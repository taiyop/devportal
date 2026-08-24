package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

func TestRunningFileRoundtrip(t *testing.T) {
	dir := filepath.Join(os.TempDir(), "devportal-running-"+uuid.NewString())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "running.yml")
	file := RunningFile{
		Groups: []RunningGroup{{
			ID:   "app-1",
			PID:  42,
			PGID: 42,
			Port: 3000,
		}},
	}
	if err := saveRunning(path, file); err != nil {
		t.Fatal(err)
	}
	loaded := loadRunning(path)
	if loaded.Groups[0].ID != "app-1" || loaded.Groups[0].PGID != 42 {
		t.Fatalf("got %+v", loaded)
	}
}

func TestRefusesToKillSelfOrInit(t *testing.T) {
	selfPID := uint32(os.Getpid())
	if !isDangerous(1, 1) {
		t.Fatal("pid 1 should be dangerous")
	}
	if !isDangerous(0, 0) {
		t.Fatal("pid 0 should be dangerous")
	}
	if !isDangerous(1, selfPID) && isDangerousPgid(os.Getppid()) {
		// self pid is always dangerous regardless of pgid
	}
	if !isDangerous(999, selfPID) {
		t.Fatal("self pid should be dangerous")
	}
}

func TestForceExitDefaultsToRawExit(t *testing.T) {
	if fmt.Sprintf("%p", forceExit) != fmt.Sprintf("%p", rawExit) {
		t.Fatal("forceExit should be rawExit, not libc exit/os.Exit")
	}
}
