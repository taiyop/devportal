package main

import (
	"bytes"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"

	"gopkg.in/yaml.v3"
)

type RunningBackend struct {
	PID  uint32 `yaml:"pid"`
	PGID int    `yaml:"pgid"`
	Port uint16 `yaml:"port"`
}

type RunningGroup struct {
	ID          string           `yaml:"id"`
	PID         uint32           `yaml:"pid"`
	PGID        int              `yaml:"pgid"`
	Port        uint16           `yaml:"port"`
	Backends    []RunningBackend `yaml:"backends,omitempty"`
	BackendPID  uint32           `yaml:"backendPid,omitempty"`
	BackendPGID int              `yaml:"backendPgid,omitempty"`
	BackendPort uint16           `yaml:"backendPort,omitempty"`
}

func (g RunningGroup) backendProcs() []RunningBackend {
	if len(g.Backends) > 0 {
		return g.Backends
	}
	if g.BackendPID != 0 || g.BackendPGID != 0 {
		return []RunningBackend{{PID: g.BackendPID, PGID: g.BackendPGID, Port: g.BackendPort}}
	}
	return nil
}

type RunningFile struct {
	Groups []RunningGroup `yaml:"groups"`
}

var (
	guardPortal  atomic.Pointer[Portal]
	guardRunPath atomic.Pointer[string]
	signalsOnce  sync.Once
	// forceExit is SIGKILL on Unix. libc exit/_exit deadlock inside AppKit.
	forceExit = rawExit
)

func installGuard(portal *Portal, runningFile string) {
	guardPortal.Store(portal)
	guardRunPath.Store(&runningFile)
	startForceKiller()
	installSignals()
}

func loadRunning(path string) RunningFile {
	raw, err := os.ReadFile(path)
	if err != nil {
		return RunningFile{}
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return RunningFile{}
	}
	var file RunningFile
	if err := yaml.Unmarshal(raw, &file); err != nil {
		return RunningFile{}
	}
	return file
}

func saveRunning(path string, file RunningFile) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("稼働記録ディレクトリを作成できませんでした: %w", err)
	}
	raw, err := yaml.Marshal(file)
	if err != nil {
		return fmt.Errorf("稼働記録の書き出しに失敗しました: %w", err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return fmt.Errorf("稼働記録を保存できませんでした: %w", err)
	}
	return nil
}

func reapPersisted(path string) {
	file := loadRunning(path)
	if len(file.Groups) == 0 {
		_ = os.Remove(path)
		return
	}
	for _, group := range file.Groups {
		killGroup(group.PID, group.PGID)
		for _, backend := range group.backendProcs() {
			killGroup(backend.PID, backend.PGID)
		}
	}
	_ = os.Remove(path)
}

func registerRunning(path string, group RunningGroup) error {
	file := loadRunning(path)
	next := make([]RunningGroup, 0, len(file.Groups)+1)
	for _, item := range file.Groups {
		if item.ID != group.ID {
			next = append(next, item)
		}
	}
	next = append(next, group)
	file.Groups = next
	return saveRunning(path, file)
}

func unregisterRunning(path, id string) error {
	file := loadRunning(path)
	next := file.Groups[:0]
	for _, item := range file.Groups {
		if item.ID != id {
			next = append(next, item)
		}
	}
	if len(next) == 0 {
		_ = os.Remove(path)
		return nil
	}
	file.Groups = next
	return saveRunning(path, file)
}

func clearRunning(path string) {
	_ = os.Remove(path)
}

func emergencyStop() {
	if path := guardRunPath.Load(); path != nil {
		reapPersisted(*path)
	}
	if portal := guardPortal.Load(); portal != nil {
		portal.StopAll()
	}
}

func isDangerous(pgid int, pid uint32) bool {
	if pid == 0 || pgid <= 1 {
		return true
	}
	selfPID := uint32(os.Getpid())
	if pid == selfPID {
		return true
	}
	return isDangerousPgid(pgid)
}

func installSignals() {
	signalsOnce.Do(func() {
		ch := make(chan os.Signal, 2)
		signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
		go func() {
			sig := <-ch
			fmt.Fprintf(os.Stderr, "\n%s を受け取りました。起動中のプロセスを終了してから閉じます。\n\n", sig)
			go func() {
				<-ch
				fmt.Fprintln(os.Stderr, "2回目の信号です。強制終了します。")
				forceExit(1)
			}()
			if portal := guardPortal.Load(); portal != nil {
				portal.ShutdownFromSignal()
			} else {
				emergencyStop()
			}
			forceExit(0)
		}()
	})
}
