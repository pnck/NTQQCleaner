package classify

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"

	"qqcleaner/internal/media"
)

// Options controls a Scan. Progress is invoked from worker goroutines once
// per file; callers must throttle (e.g. ≥100ms or every N files) themselves.
// K 是必需的（qq 知识实现：遍历白名单/跳过目录/分类/命名解析）。
type Options struct {
	K        Classifier
	OnlyBizs []string        // empty = K 的全部白名单
	SkipDirs map[string]bool // 额外的跳过目录（nil = K 的默认）
	MinSize  int64           // ignore files smaller than this
	Workers  int             // 0 = NumCPU, capped at len(BizDirs)
	Progress func(stage string, done, total uint64)
	// DetectAnimated 开启动图嗅探（对图片类文件按内容魔数判定 gif/
	// webp/APNG 动画，每次多一次 open+读头）。显示层才需要：由上层按
	// 平台政策决定（Windows 照片墙静态化动图）；CLI manifest 不需要，
	// 关闭可免掉扫描期额外 I/O。
	DetectAnimated bool
}

// Scan walks the whitelisted biz directories under ntData and returns every
// classified file. It performs a quick pre-count pass first so progress can
// report a meaningful total (dir reads are cheap; per-file stats dominate).
func Scan(ctx context.Context, ntData string, opts Options) ([]FileEntry, error) {
	if opts.K == nil {
		return nil, fmt.Errorf("classify: knowledge (Options.K) is required")
	}
	skip := opts.SkipDirs
	if skip == nil {
		skip = opts.K.SkipDirs()
	}
	roots, err := walkRoots(opts.K, ntData, opts.OnlyBizs)
	if err != nil {
		return nil, err
	}

	var total uint64
	for _, r := range roots {
		n, err := countFiles(ctx, r, skip)
		if err != nil {
			return nil, err
		}
		total += n
	}

	var (
		entries []FileEntry
		mu      sync.Mutex
		done    uint64
	)
	jobs := make(chan string)
	workers := opts.Workers
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	if workers > len(roots) {
		workers = len(roots)
	}
	if workers < 1 {
		workers = 1
	}

	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	errOnce := sync.Once{}
	var firstErr error
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for root := range jobs {
				found, err := walkRoot(ctx, opts.K, root, skip, opts.MinSize, opts.DetectAnimated, func() {
					atomic.AddUint64(&done, 1)
					if opts.Progress != nil {
						opts.Progress(root, atomic.LoadUint64(&done), total)
					}
				})
				if err != nil && !errors.Is(err, context.Canceled) {
					errOnce.Do(func() { firstErr = err })
					cancel()
					return
				}
				mu.Lock()
				entries = append(entries, found...)
				mu.Unlock()
			}
		}()
	}
	for _, r := range roots {
		jobs <- r
	}
	close(jobs)
	wg.Wait()

	if firstErr != nil {
		return nil, firstErr
	}
	if ctx.Err() != nil {
		return entries, ctx.Err()
	}
	return entries, nil
}

// isImageExt 判定扩展名是否属于图片类（动图嗅探的粗筛门：QQ 缓存里
// 扩展名与实际内容常不一致——personal_emoji 的 gif 存成 .jpg、jpg 实
// 为 webp——所以门控只按「看起来是图片」粗筛省 I/O，真伪由内容魔数
// 决定）。含手机互传常见格式（avif/heic——探测覆盖到，动画解码能力
// 以 media 包为准）。
func isImageExt(ext string) bool {
	switch ext {
	case "jpg", "jpeg", "png", "gif", "webp", "bmp", "avif", "heic":
		return true
	}
	return false
}

// walkRoots resolves the whitelisted biz dirs that exist under ntData.
func walkRoots(k Classifier, ntData string, only []string) ([]string, error) {
	bizs := k.BizDirs()
	if len(only) > 0 {
		bizs = only
	}
	var roots []string
	for _, b := range bizs {
		p := filepath.Join(ntData, b)
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			roots = append(roots, p)
		}
	}
	if len(roots) == 0 {
		return nil, fs.ErrNotExist
	}
	return roots, nil
}

func countFiles(ctx context.Context, root string, skip map[string]bool) (uint64, error) {
	var n uint64
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skip[d.Name()] && path != root {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type().IsRegular() {
			n++
		}
		return nil
	})
	return n, err
}

func walkRoot(ctx context.Context, k Classifier, root string, skip map[string]bool, minSize int64, detectAnimated bool, onFile func()) ([]FileEntry, error) {
	var out []FileEntry
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if d.IsDir() {
			if skip[d.Name()] && path != root {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil // never follow symlinks
		}
		info, err := d.Info()
		if err != nil {
			return nil // unreadable entry: skip, never touch
		}
		if info.Size() < minSize {
			return nil
		}
		e := newEntry(k, filepath.Dir(root), path, info.Size(), info.ModTime().Unix())
		// 动图标记（media 层按内容判定，不信任扩展名）：isImageExt 只做
		// 粗筛省 I/O（跳过视频/数据库等），真伪由内容魔数决定。
		if detectAnimated && isImageExt(e.Ext) {
			e.Animated = media.IsAnimated(path)
		}
		out = append(out, e)
		if onFile != nil {
			onFile()
		}
		return nil
	})
	return out, err
}
