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
				found, err := walkRoot(ctx, opts.K, root, skip, opts.MinSize, func() {
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

func walkRoot(ctx context.Context, k Classifier, root string, skip map[string]bool, minSize int64, onFile func()) ([]FileEntry, error) {
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
		out = append(out, newEntry(k, filepath.Dir(root), path, info.Size(), info.ModTime().Unix()))
		if onFile != nil {
			onFile()
		}
		return nil
	})
	return out, err
}
