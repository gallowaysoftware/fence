package decisionlog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWriteAndClose(t *testing.T) {
	dir := t.TempDir()
	l, err := New(filepath.Join(dir, "nested", "log"), 16, nil)
	if err != nil {
		t.Fatal(err)
	}
	l.Write("s-1", map[string]int{"n": 1})
	l.Write("s-1", map[string]int{"n": 2})
	l.Write("s-2", map[string]int{"n": 3})
	l.Close()
	l.Write("s-1", map[string]int{"n": 4})
	l.Close()

	b, err := os.ReadFile(l.Path("s-1"))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(b); got != "{\"n\":1}\n{\"n\":2}\n" {
		t.Fatalf("s-1 = %q", got)
	}
	if _, err := os.Stat(l.Path("s-2")); err != nil {
		t.Fatal(err)
	}
	if l.Dropped() != 1 {
		t.Fatalf("dropped %d, want 1 (write after close)", l.Dropped())
	}
}

func TestWriteNeverBlocks(t *testing.T) {
	l := newLogger(t.TempDir(), 2, nil) // no writer goroutine: the queue never drains
	done := make(chan struct{})
	go func() {
		for range 10 {
			l.Write("s", 1)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Write blocked on a full queue")
	}
	if l.Dropped() != 8 {
		t.Fatalf("dropped %d, want 8", l.Dropped())
	}
}

func TestUnmarshalableRecordCounted(t *testing.T) {
	l, err := New(t.TempDir(), 4, nil)
	if err != nil {
		t.Fatal(err)
	}
	l.Write("s", func() {})
	l.Close()
	if l.Dropped() != 1 {
		t.Fatalf("dropped %d", l.Dropped())
	}
}

func TestNilLoggerIsNoop(t *testing.T) {
	var l *Logger
	l.Write("s", 1)
}

func TestPath(t *testing.T) {
	l := newLogger("/logs", 1, nil)
	tests := map[string]string{
		"s-0001":        "s-0001.jsonl",
		"../../etc/pwd": "_.._.._etc_pwd.jsonl",
		"":              "_.jsonl",
		".hidden":       "_.hidden.jsonl",
	}
	for in, want := range tests {
		if got := filepath.Base(l.Path(in)); got != want || !strings.HasPrefix(l.Path(in), "/logs/") {
			t.Errorf("Path(%q) = %s, want /logs/%s", in, l.Path(in), want)
		}
	}
}

func TestManySessionsBoundedFiles(t *testing.T) {
	dir := t.TempDir()
	l, err := New(dir, 1024, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := range maxOpenFiles * 3 {
		l.Write("s-"+string(rune('a'+i%26))+string(rune('a'+i/26)), i)
	}
	// Re-write to an evicted session: it must reopen and append.
	l.Write("s-aa", "again")
	l.Close()
	if len(l.fds) > maxOpenFiles {
		t.Fatalf("%d files open", len(l.fds))
	}
	b, _ := os.ReadFile(l.Path("s-aa"))
	if got := string(b); got != "0\n\"again\"\n" {
		t.Fatalf("s-aa = %q", got)
	}
}
