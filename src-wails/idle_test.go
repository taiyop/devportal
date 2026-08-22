package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestShouldIdleStop(t *testing.T) {
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	last := now.Add(-15 * time.Minute)
	if !shouldIdleStop(false, 0, 15, last, now) {
		t.Fatal("expected stop after idle")
	}
	if shouldIdleStop(true, 0, 15, last, now) {
		t.Fatal("pinned must not stop")
	}
	if shouldIdleStop(false, 1, 15, last, now) {
		t.Fatal("active proxy must not stop")
	}
	if shouldIdleStop(false, 0, 0, last, now) {
		t.Fatal("zero timeout means never")
	}
	if shouldIdleStop(false, 0, 15, now.Add(-14*time.Minute), now) {
		t.Fatal("not yet idle")
	}
}

func TestSetPinnedUpdatesView(t *testing.T) {
	id := "app-1"
	idle := 15
	p := &Portal{inner: Inner{
		file: ConfigFile{Apps: []AppEntry{{
			ID:          id,
			Name:        "notes",
			IdleStopMin: idle,
		}}},
		runtime: map[string]*Runtime{
			id: {ready: true, port: 3333, lastAccess: time.Now()},
		},
		errors: map[string]string{},
	}}
	view, err := p.SetPinned(id, true)
	if err != nil {
		t.Fatal(err)
	}
	if !view.Pinned {
		t.Fatal("expected pinned view")
	}
	if view.IdleUntil != nil {
		t.Fatal("pinned view should not have idleUntil")
	}

	view, err = p.SetPinned(id, false)
	if err != nil {
		t.Fatal(err)
	}
	if view.Pinned {
		t.Fatal("expected unpinned view")
	}
	if view.IdleUntil == nil {
		t.Fatal("unpinned view should have idleUntil")
	}
}

func TestIdleStopDoesNotArmPortSentry(t *testing.T) {
	if isWindows() {
		t.Skip("wake re-arm is unix-oriented")
	}
	port, err := findFreePort()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(os.TempDir(), "devportal-idle-"+uuid.NewString())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	id := "idle-app"
	p := &Portal{inner: Inner{
		runningPath: filepath.Join(dir, "running.yml"),
		file: ConfigFile{Apps: []AppEntry{{
			ID:            id,
			Name:          "idle-app",
			WakeOnRequest: true,
			Port:          &port,
			IdleStopMin:   5,
			PortMode:      PortModeManual,
		}}},
		runtime: map[string]*Runtime{},
		errors:  map[string]string{},
		logs:    map[string][]LogEvent{},
	}}
	cmd, keepalive, pgid, stdout, stderr, err := spawnCommand(os.TempDir(), "sleep 30", port, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer stdout.Close()
	defer stderr.Close()
	rt := &Runtime{
		cmd:        cmd,
		port:       port,
		ready:      true,
		pgid:       pgid,
		keepalive:  keepalive,
		lastAccess: time.Now().Add(-10 * time.Minute),
		waitCh:     make(chan struct{}),
	}
	p.inner.runtime[id] = rt
	go func() { _ = rt.wait() }()
	p.reapIdle()
	t.Cleanup(func() { disarm(id) })
	if _, ok := armedPort(id); ok {
		t.Fatal("port sentry should not re-arm after idle stop")
	}
	if _, running := p.inner.runtime[id]; running {
		t.Fatal("runtime should be gone")
	}
}

func TestViewIdleWhenGatewayRunning(t *testing.T) {
	inner := Inner{
		file:        ConfigFile{Apps: []AppEntry{{ID: "1", Name: "notes", Hostname: "notes"}}},
		runtime:     map[string]*Runtime{},
		errors:      map[string]string{},
		gatewayPort: 80,
	}
	view := viewFor(&inner, inner.file.Apps[0])
	if view.Status != AppStatusIdle {
		t.Fatalf("status %s", view.Status)
	}
}

func TestViewStoppedWhenGatewayOff(t *testing.T) {
	inner := Inner{
		file:    ConfigFile{Apps: []AppEntry{{ID: "1", Name: "notes", Hostname: "notes"}}},
		runtime: map[string]*Runtime{},
		errors:  map[string]string{},
	}
	view := viewFor(&inner, inner.file.Apps[0])
	if view.Status != AppStatusStopped {
		t.Fatalf("status %s", view.Status)
	}
}

func TestViewForRunningIncludesPinned(t *testing.T) {
	id := "app-1"
	inner := Inner{
		file: ConfigFile{Apps: []AppEntry{{ID: id, Name: "notes", IdleStopMin: 15}}},
		runtime: map[string]*Runtime{
			id: {ready: true, port: 3333, pinned: true, lastAccess: time.Now()},
		},
		errors: map[string]string{},
	}
	view := viewFor(&inner, inner.file.Apps[0])
	if !view.Pinned {
		t.Fatal("running view discarded pinned")
	}
}
