// Package testutil builds deterministic fake QQ data roots for tests,
// modeled on docs/05_sample_data.md §6. Mtimes are fixed so reason/关联
// assertions are reproducible.
package testutil

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Fixed test clock: everything is aged relative to this instant.
var Now = time.Date(2026, 8, 10, 12, 0, 0, 0, time.Local)

// Synthetic test values; structure mirrors the docs/05 samples.
const (
	HashA   = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb1" // 32 hex，格式同 nt_qq_<hash>
	HashB   = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb2"
	QQA     = "10001"
	QQB     = "10002"
	MD5A    = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa01"
	MD5B    = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa02"
	MD5C    = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa03"
	MD5D    = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa04"
	MD5E    = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa05"
	MD5Temp = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa06"
	MD5F    = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa07"
	MD5G    = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa08"
)

// Fixture is a fake QQ data root plus the paths tests commonly need.
type Fixture struct {
	Root    string
	NtDataA string
	NtDataB string
}

// BuildQQTree creates the fixture layout (docs/05 §6):
//
//	global/nt_data/mmkv/mmkv.default          (hash ↔ QQ mappings)
//	global/nt_data/Login/.10001                 (source 3)
//	nt_qq_<A>/nt_data/
//	  Pic/2023-01/{Ori,Thumb}/<md5A>...        old pic pair (same md5)
//	  Pic/2026-07/Thumb/<md5A>_720.jpg         recent thumb of same md5
//	  Pic/2026-08/Ori/<md5E>.jpg               fresh (1 day old)
//	  Pic/2026-07/OriTemp/<md5Temp>.tmp        temp residue (4 days old)
//	  Pic/2023-01/Thumb/<md5G>_720.jpg         60KB, 与 md5D/md5F 字节相同
//	  Pic/2026-07/Thumb/<md5F>_720.jpg         60KB, 同内容不同名（内容重复）
//	  Emoji/emoji-recv/2024-05/Thumb/<md5B>_720.png
//	  Emoji/marketface/123/x.png               10KB（与 my.png 同大小不同内容）
//	  File/Thumb/<md5C>.png                    File is a cautious unit
//	  UnitedConfig/<QQA>/                      (source 2)
//	  mmkv/, nt_db/                            never scanned
//	nt_qq_<B>/nt_data/Pic/2022-01/Thumb/<md5D>_720.jpg  60KB, 与 md5F/md5G 字节相同
func BuildQQTree(t *testing.T) *Fixture {
	t.Helper()
	root := t.TempDir()
	f := &Fixture{Root: root}

	// --- global (account identification sources) ---
	mmkvContent := []byte(
		"\x00\x10mmkv" +
			"k_/data/nt_qq_" + HashA + "/nt_data/flashfransfer+" + QQA + "_flash_transfer_document_cache_key" +
			"\x00\x01v" +
			"k_/data/nt_qq_" + HashB + "/nt_data/flashfransfer+" + QQB + "_flash_file_download_dir_key" +
			"\x00\x02junk",
	)
	mkFile(t, filepath.Join(root, "global", "nt_data", "mmkv", "mmkv.default"), mmkvContent, Now.AddDate(0, 0, -30))
	mkFile(t, filepath.Join(root, "global", "nt_data", "Login", "."+QQA), nil, Now.AddDate(0, 0, -60))
	mkFile(t, filepath.Join(root, "global", "nt_data", "Login", "."+QQB), nil, Now.AddDate(0, 0, -300))

	// --- account A ---
	f.NtDataA = filepath.Join(root, "nt_qq_"+HashA, "nt_data")
	mkFile(t, filepath.Join(f.NtDataA, "UnitedConfig", QQA, "10002", "config"), []byte("x"), Now.AddDate(0, 0, -10))

	mkFile(t, filepath.Join(f.NtDataA, "Pic", "2023-01", "Ori", MD5A+".jpg"), bytesN(2<<20), nowDate(2023, 1, 15))
	mkFile(t, filepath.Join(f.NtDataA, "Pic", "2023-01", "Thumb", MD5A+"_720.jpg"), bytesN(80<<10), nowDate(2023, 1, 15))
	mkFile(t, filepath.Join(f.NtDataA, "Pic", "2026-07", "Thumb", MD5A+"_720.jpg"), bytesN(80<<10), nowDate(2026, 7, 10))
	mkFile(t, filepath.Join(f.NtDataA, "Pic", "2026-08", "Ori", MD5E+".jpg"), bytesN(100<<10), Now.AddDate(0, 0, -1))
	mkFile(t, filepath.Join(f.NtDataA, "Pic", "2026-07", "OriTemp", MD5Temp+".tmp"), bytesN(1<<10), Now.AddDate(0, 0, -4))
	// 内容重复（同字节、不同名）：60KB 内容 ×2（与 B 账号的 md5D 同为
	// 一组 3 份的字节级副本）。
	mkFile(t, filepath.Join(f.NtDataA, "Pic", "2023-01", "Thumb", MD5G+"_720.jpg"), bytesN(60<<10), nowDate(2023, 1, 15))
	mkFile(t, filepath.Join(f.NtDataA, "Pic", "2026-07", "Thumb", MD5F+"_720.jpg"), bytesN(60<<10), nowDate(2026, 7, 10))

	mkFile(t, filepath.Join(f.NtDataA, "Emoji", "emoji-recv", "2024-05", "Thumb", MD5B+"_720.png"), bytesN(50<<10), nowDate(2024, 5, 1))
	mkFile(t, filepath.Join(f.NtDataA, "Emoji", "marketface", "123", "x.png"), bytesN(10<<10), nowDate(2025, 1, 1))
	// 与 marketface/x.png 同大小（10KB）但内容不同：验证「先按大小分组、
	// 再按哈希细分」不会误判为重复。
	personal := bytesN(10 << 10)
	personal[0] = 0x01
	mkFile(t, filepath.Join(f.NtDataA, "Emoji", "personal_emoji", "Ori", "my.png"), personal, nowDate(2024, 1, 1))
	mkFile(t, filepath.Join(f.NtDataA, "File", "Thumb", MD5C+".png"), bytesN(30<<10), nowDate(2022, 6, 1))
	mkFile(t, filepath.Join(f.NtDataA, "File", "file_assistant", "doc.txt"), []byte("x"), nowDate(2022, 6, 1))
	// dataline/.tmp 的 NFC 传输残留：已知可清结构（clean_temp 门控），
	// 必须进入索引（用户实测布局：dataline/.tmp/output(1).mp4.<hash>.NFC）。
	// 3KB 大小唯一，不触发内容哈希。
	mkFile(t, filepath.Join(f.NtDataA, "dataline", ".tmp", "output(1).mp4."+MD5G+".NFC"), bytesN(3<<10), Now.AddDate(0, 0, -5))
	// 高级 opt-in 类别（默认关，设置里勾选后扫描+清理才覆盖）：
	// 运行日志（QQ 自动重建）、头像缓存（重新拉取）。
	mkFile(t, filepath.Join(f.NtDataA, "log", "2026-08.log"), bytesN(2), Now.AddDate(0, 0, -10))
	mkFile(t, filepath.Join(f.NtDataA, "log-cache", "x"), bytesN(4), Now.AddDate(0, 0, -10))
	mkFile(t, filepath.Join(f.NtDataA, "avatar", "a.png"), bytesN(5), Now.AddDate(0, 0, -10))

	// must never be scanned or cleaned
	mkFile(t, filepath.Join(f.NtDataA, "mmkv", "mmkv.default"), []byte("x"), Now)
	mkFile(t, filepath.Join(f.NtDataA, "nt_db", "bc.db"), []byte("x"), Now)
	mkFile(t, filepath.Join(root, "nt_qq_"+HashA, "nt_db", "message.db"), []byte("x"), Now)

	// --- account B ---
	f.NtDataB = filepath.Join(root, "nt_qq_"+HashB, "nt_data")
	mkFile(t, filepath.Join(f.NtDataB, "UnitedConfig", "000", "config"), []byte("x"), Now.AddDate(0, 0, -10))
	mkFile(t, filepath.Join(f.NtDataB, "Pic", "2022-01", "Thumb", MD5D+"_720.jpg"), bytesN(60<<10), nowDate(2022, 1, 5))
	return f
}

// mkFile writes a file with fixed content and mtime.
func mkFile(t *testing.T, path string, content []byte, mtime time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}

func nowDate(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 12, 0, 0, 0, time.Local)
}

func bytesN(n int) []byte { return make([]byte, n) }
