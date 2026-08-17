package oplog

import (
	"io"
	"os"
	"strings"
	"testing"
)

// TestPrintfWhenEnabled：启用后时间戳行输出到 stdout。
func TestPrintfWhenEnabled(t *testing.T) {
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() { os.Stdout = old }()
	Enable()
	defer Disable()
	Printf("clean op: remove %s", "/x/y")
	w.Close()
	data, _ := io.ReadAll(r)
	if !strings.Contains(string(data), "clean op: remove /x/y") {
		t.Fatalf("missing line: %q", data)
	}
}

// TestPrintfDisabledNoop：未启用时零输出。
func TestPrintfDisabledNoop(t *testing.T) {
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() { os.Stdout = old }()
	Printf("should not appear")
	w.Close()
	data, _ := io.ReadAll(r)
	if len(data) != 0 {
		t.Fatalf("expected no output, got %q", data)
	}
}
