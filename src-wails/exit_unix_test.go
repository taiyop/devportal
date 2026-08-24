//go:build unix

package main

import (
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

func TestForceKillerKillsTarget(t *testing.T) {
	cmd := exec.Command("/bin/sleep", "30")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pid := cmd.Process.Pid
	defer func() { _ = syscall.Kill(-pid, syscall.SIGKILL) }()

	killer, err := newForceKiller(pid)
	if err != nil {
		t.Fatal(err)
	}
	if killer.helperPid <= 1 {
		t.Fatal("helper pid missing")
	}
	time.Sleep(100 * time.Millisecond)
	killer.trip()

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("force killer did not SIGKILL the target after SIGUSR1")
	}
	if processAlive(uint32(pid)) {
		t.Fatalf("pid %d still alive", pid)
	}
}

func TestForceKillerKillsExtrasAndTarget(t *testing.T) {
	startSleep := func() *exec.Cmd {
		cmd := exec.Command("/bin/sleep", "30")
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		return cmd
	}
	target := startSleep()
	extra := startSleep()
	defer func() {
		_ = syscall.Kill(-target.Process.Pid, syscall.SIGKILL)
		_ = syscall.Kill(-extra.Process.Pid, syscall.SIGKILL)
	}()

	killer, err := newForceKiller(target.Process.Pid, extra.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	killer.trip()

	waitDone := func(cmd *exec.Cmd) {
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatalf("pid %d still alive after trip", cmd.Process.Pid)
		}
	}
	waitDone(target)
	waitDone(extra)
	if processAlive(uint32(target.Process.Pid)) || processAlive(uint32(extra.Process.Pid)) {
		t.Fatal("target or extra still alive")
	}
}

func TestForceKillerReapsExtrasWhenWatchDies(t *testing.T) {
	startSleep := func() *exec.Cmd {
		cmd := exec.Command("/bin/sleep", "30")
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		return cmd
	}
	watch := startSleep()
	extra := startSleep()
	defer func() {
		_ = syscall.Kill(-watch.Process.Pid, syscall.SIGKILL)
		_ = syscall.Kill(-extra.Process.Pid, syscall.SIGKILL)
	}()
	if _, err := newForceKiller(watch.Process.Pid, extra.Process.Pid); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	if err := syscall.Kill(watch.Process.Pid, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	_, _ = watch.Process.Wait()
	done := make(chan error, 1)
	go func() { done <- extra.Wait() }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("extra still alive after watch died without trip")
	}
	if processAlive(uint32(extra.Process.Pid)) {
		t.Fatal("extra still alive")
	}
}

func TestForceKillerLeavesTargetUntilTrip(t *testing.T) {
	cmd := exec.Command("/bin/sleep", "30")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pid := cmd.Process.Pid
	defer func() { _ = syscall.Kill(-pid, syscall.SIGKILL) }()

	if _, err := newForceKiller(pid); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	if !processAlive(uint32(pid)) {
		t.Fatal("helper killed the target before trip")
	}
}

func TestForceKillerIgnoresInitPid(t *testing.T) {
	if _, err := newForceKiller(1); err == nil {
		t.Fatal("expected error for init target")
	}
	if _, err := newForceKiller(0); err == nil {
		t.Fatal("expected error for pid 0")
	}
}

func TestShouldKillAncestorSelectsWailsSession(t *testing.T) {
	if !shouldKillAncestor("wails3 dev") {
		t.Fatal("wails3 dev should be killed")
	}
	if !shouldKillAncestor("/bin/bash -c cd src-wails && wails3 dev") {
		t.Fatal("wails3 wrapper bash should be killed")
	}
	if shouldKillAncestor("/bin/bash /path/to/verify-sigint.sh") {
		t.Fatal("unrelated bash must not be killed")
	}
	if !shouldKillAncestor("bun run wails:dev") {
		t.Fatal("bun should be killed")
	}
	if isStopAncestor("wails3 dev") {
		t.Fatal("wails3 is not a stop ancestor")
	}
	if !isStopAncestor("/bin/zsh") {
		t.Fatal("interactive zsh must not be killed")
	}
	if !isStopAncestor("-zsh") {
		t.Fatal("login zsh must not be killed")
	}
}

func TestSessionKillTargetsSkipsSelf(t *testing.T) {
	self := os.Getpid()
	for _, pid := range sessionKillTargets(self) {
		if pid == self || pid <= 1 {
			t.Fatalf("unexpected target %d", pid)
		}
	}
}
