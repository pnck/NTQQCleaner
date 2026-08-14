package classify

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

// TestHashDuplicates：只有大小冲突组的文件才计算哈希；同大小不同内容
// 不会被误分到同一组；唯一大小的文件保持空哈希。
func TestHashDuplicates(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, content []byte) string {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, content, 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	identical := []byte("same content")
	other := []byte("diff content") // 与 identical 同长度、不同字节
	entries := []*FileEntry{
		{Path: write("a.bin", identical), Size: int64(len(identical))}, // 冲突组，内容相同
		{Path: write("b.bin", identical), Size: int64(len(identical))}, // 与 a.bin 字节级重复
		{Path: write("c.bin", other), Size: int64(len(other))},         // 同大小不同内容 → 不同哈希
		{Path: write("d.bin", []byte("unique")), Size: 6},              // 大小唯一 → 跳过
	}
	var total uint64
	if err := HashDuplicates(context.Background(), entries, func(done, totalDone uint64) {
		total = totalDone
	}); err != nil {
		t.Fatal(err)
	}
	if total != 3 {
		t.Fatalf("hashed %d files want 3 (only size-collision candidates)", total)
	}
	if entries[0].ContentHash == "" || entries[0].ContentHash != entries[1].ContentHash {
		t.Errorf("byte-identical files must share a hash: %q vs %q", entries[0].ContentHash, entries[1].ContentHash)
	}
	if entries[2].ContentHash == "" || entries[2].ContentHash == entries[0].ContentHash {
		t.Errorf("same-size different-content must not share a hash: %q vs %q", entries[2].ContentHash, entries[0].ContentHash)
	}
	if entries[3].ContentHash != "" {
		t.Errorf("unique-size file must stay un-hashed, got %q", entries[3].ContentHash)
	}
}

// TestHashDuplicatesCancelled：取消后立即返回 ctx.Err()，且不写入任何哈希。
func TestHashDuplicatesCancelled(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.bin")
	if err := os.WriteFile(p, []byte("xx"), 0o644); err != nil {
		t.Fatal(err)
	}
	entries := []*FileEntry{{Path: p, Size: 2}, {Path: p, Size: 2}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := HashDuplicates(ctx, entries, nil); err == nil {
		t.Fatal("cancelled context must return ctx.Err()")
	}
	if entries[0].ContentHash != "" {
		t.Errorf("cancelled run must not hash, got %q", entries[0].ContentHash)
	}
}

// TestHashDuplicatesEmptyFiles：零字节文件走常量短路（不 open），且与
// 全量计算的 SHA-256 空串一致（精确而非启发式）。
func TestHashDuplicatesEmptyFiles(t *testing.T) {
	dir := t.TempDir()
	write := func(name string) string {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	entries := []*FileEntry{
		{Path: write("e1.bin"), Size: 0},
		{Path: write("e2.bin"), Size: 0},
	}
	if err := HashDuplicates(context.Background(), entries, nil); err != nil {
		t.Fatal(err)
	}
	empty := hex.EncodeToString(sha256.New().Sum(nil))
	if entries[0].ContentHash != empty || entries[1].ContentHash != empty {
		t.Errorf("empty files must share the constant SHA-256 of empty input, got %q / %q",
			entries[0].ContentHash, entries[1].ContentHash)
	}
}
