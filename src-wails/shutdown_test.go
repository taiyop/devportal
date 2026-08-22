package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestShutdownAllEmpty(t *testing.T) {
	p := newTestPortal()
	var buf bytes.Buffer
	p.shutdownAll(&buf)
	if !strings.Contains(buf.String(), "起動中のプロセスはありません") {
		t.Fatalf("got %q", buf.String())
	}
}

func TestShutdownAllListsAndWaits(t *testing.T) {
	if isWindows() {
		t.Skip("process group shutdown is unix-oriented")
	}
	dir := filepath.Join(os.TempDir(), "devportal-shutdown-"+uuid.NewString())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	p := &Portal{inner: Inner{
		runningPath: filepath.Join(dir, "running.yml"),
		file: ConfigFile{Apps: []AppEntry{
			{ID: "a", Name: "alpha"},
			{ID: "b", Name: "beta"},
		}},
		runtime: map[string]*Runtime{},
		errors:  map[string]string{},
		logs:    map[string][]LogEvent{},
	}}

	for _, id := range []string{"a", "b"} {
		cmd, keepalive, pgid, stdout, stderr, err := spawnCommand(os.TempDir(), "sleep 30", 1, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer stdout.Close()
		defer stderr.Close()
		p.inner.runtime[id] = &Runtime{
			cmd:       cmd,
			pgid:      pgid,
			keepalive: keepalive,
			waitCh:    make(chan struct{}),
			port:      1,
		}
	}

	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		p.shutdownAll(&buf)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(6 * time.Second):
		t.Fatalf("shutdown timed out\n%s", buf.String())
	}

	out := buf.String()
	if !strings.Contains(out, "起動中のプロセス 2 件") {
		t.Fatalf("missing list:\n%s", out)
	}
	if !strings.Contains(out, "alpha") || !strings.Contains(out, "beta") {
		t.Fatalf("missing names:\n%s", out)
	}
	if strings.Count(out, "終了しました") < 2 {
		t.Fatalf("missing exit confirmations:\n%s", out)
	}
	if !strings.Contains(out, "すべてのプロセスが終了しました") {
		t.Fatalf("missing completion:\n%s", out)
	}
	if len(p.inner.runtime) != 0 {
		t.Fatal("runtimes should be cleared before waiting")
	}
}
