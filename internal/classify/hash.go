package classify

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
)

// HashDuplicates 是扫描的第二遍：为「与其它文件字节数完全相同」的文件
// 计算 SHA-256 内容哈希，写入 entries[i].ContentHash。
//
// 依据：内容相同的文件大小必然相同，所以大小唯一的文件不可能有内容
// 孪生——只对大小冲突组内的文件做全量读取，把 I/O 开销压到最低
// （docs/04 §4.2 二次扫描）。只读不写，dry-run 安全。ctx 可取消；
// progress 每完成一个文件回调一次（调用方负责节流）。
func HashDuplicates(ctx context.Context, entries []*FileEntry, progress func(done, total uint64)) error {
	sizeCount := make(map[int64]int, len(entries))
	for _, e := range entries {
		sizeCount[e.Size]++
	}
	var cand []int
	for i, e := range entries {
		if sizeCount[e.Size] >= 2 {
			cand = append(cand, i)
		}
	}
	if len(cand) == 0 || ctx.Err() != nil {
		return ctx.Err()
	}

	workers := runtime.GOMAXPROCS(0)
	if workers > 16 {
		workers = 16 // 小文件为主，syscall 墙：NVMe/APFS 随机小文件读 16 路仍有余量（docs/07 §4.3）
	}
	jobs := make(chan int)
	var done atomic.Uint64
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case i, ok := <-jobs:
					if !ok {
						return
					}
					hashFile(ctx, entries[i])
					if progress != nil {
						progress(done.Add(1), uint64(len(cand)))
					}
				}
			}
		}()
	}
feed:
	for _, i := range cand {
		select {
		case <-ctx.Done():
			break feed
		case jobs <- i:
		}
	}
	close(jobs)
	wg.Wait()
	return ctx.Err()
}

// emptySHA256 是零字节文件的 SHA-256 常量：跳过 open/read，直接赋值
// （SHA-256 空串是数学常量，精确而非启发式）。
var emptySHA256 = hex.EncodeToString(sha256.New().Sum(nil))

// hashFile 计算单文件 SHA-256；读取失败或取消时保持 ContentHash 空串
// （该文件不进入任何内容组——宁可漏报，不可误报）。
func hashFile(ctx context.Context, e *FileEntry) {
	if e.Size == 0 {
		e.ContentHash = emptySHA256
		return
	}
	f, err := os.Open(e.Path)
	if err != nil {
		return
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, ctxReader{ctx: ctx, r: f}); err != nil {
		return
	}
	e.ContentHash = hex.EncodeToString(h.Sum(nil))
}

// ctxReader aborts reads as soon as the context is cancelled.
type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c ctxReader) Read(p []byte) (int, error) {
	select {
	case <-c.ctx.Done():
		return 0, c.ctx.Err()
	default:
		return c.r.Read(p)
	}
}
