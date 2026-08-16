package logring

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLogfAndDump(t *testing.T) {
	Logf("hello %d", 1)
	Logf("world")
	var buf bytes.Buffer
	Dump(&buf)
	out := buf.String()
	if !strings.Contains(out, "hello 1") || !strings.Contains(out, "world") {
		t.Fatalf("dump missing entries:\n%s", out)
	}
}

// TestRecoverDumpsRing：panic 经 Recover 重新抛出，环形缓冲已写入
// 崩溃文件（与运行时崩溃转储同文件的机制验证）。
func TestRecoverDumpsRing(t *testing.T) {
	path := EnableCrashLog(t.TempDir())
	if path == "" {
		t.Fatal("EnableCrashLog failed")
	}
	Logf("prelude-to-crash")
	func() {
		defer func() {
			if v := recover(); v == nil {
				t.Error("expected the panic to be re-raised")
			}
		}()
		func() {
			defer Recover()
			panic("boom")
		}()
	}()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if !strings.Contains(s, "prelude-to-crash") || !strings.Contains(s, "panic recovered for ring dump: boom") {
		t.Fatalf("crash file missing ring dump:\n%s", s)
	}
}

// TestCrashFileNaming：崩溃文件按时间戳命名，落在指定目录。
func TestCrashFileNaming(t *testing.T) {
	dir := t.TempDir()
	path := EnableCrashLog(dir)
	if path == "" {
		t.Fatal("EnableCrashLog failed")
	}
	if filepath.Dir(path) != dir || !strings.Contains(filepath.Base(path), "crash-") {
		t.Fatalf("unexpected crash path: %s", path)
	}
}

// TestCrumbLandsImmediately：面包屑在进程存活期间即落盘（docs/09
// §3.2）——外部击毙时崩溃文件不为 0 字节、最后一根面包屑定位死点。
func TestCrumbLandsImmediately(t *testing.T) {
	path := EnableCrashLog(t.TempDir())
	if path == "" {
		t.Fatal("EnableCrashLog failed")
	}
	// 启用即落首行（crash watcher armed）。
	first, err := os.ReadFile(path)
	if err != nil || len(first) == 0 {
		t.Fatalf("crash file empty right after EnableCrashLog: %v", err)
	}
	Crumb("progress 1000/5000")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if !strings.Contains(s, "crash watcher armed") || !strings.Contains(s, "progress 1000/5000") {
		t.Fatalf("crumb missing from crash file:\n%s", s)
	}
}
