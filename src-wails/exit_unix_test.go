//go:build unix

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
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

func TestForceKillerDetachesFromCaller(t *testing.T) {
	cmd := exec.Command("/bin/sleep", "30")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }()

	killer, err := newForceKiller(cmd.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = syscall.Kill(killer.helperPid, syscall.SIGKILL) }()
	ppid := parentPid(killer.helperPid)
	if ppid == os.Getpid() {
		t.Fatalf("helper ppid=%d is still the caller; wails3 would reap it with DevPortal", ppid)
	}
	if !processAlive(uint32(killer.helperPid)) {
		t.Fatal("helper died after detach")
	}
}

func TestForceKillerKillsExtrasWhenHelperIsChildOfWatch(t *testing.T) {
	if os.Getenv("DEVPORTAL_BE_WATCH") == "1" {
		extra, err := strconv.Atoi(os.Getenv("DEVPORTAL_EXTRA_PID"))
		if err != nil || extra <= 1 {
			os.Exit(1)
		}
		k, err := newForceKiller(os.Getpid(), extra)
		if err != nil {
			os.Exit(1)
		}
		status := strconv.Itoa(k.helperPid) + " " + strconv.Itoa(parentPid(k.helperPid))
		if err := os.WriteFile(os.Getenv("DEVPORTAL_HELPER_PID_FILE"), []byte(status+"\n"), 0o600); err != nil {
			os.Exit(1)
		}
		time.Sleep(30 * time.Second)
		os.Exit(0)
	}

	startSleep := func() *exec.Cmd {
		cmd := exec.Command("/bin/sleep", "30")
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		return cmd
	}
	extra := startSleep()
	defer func() { _ = syscall.Kill(-extra.Process.Pid, syscall.SIGKILL) }()

	pidFile := filepath.Join(t.TempDir(), "helper.pid")
	watch := exec.Command(os.Args[0], "-test.run=^TestForceKillerKillsExtrasWhenHelperIsChildOfWatch$", "-test.count=1")
	watch.Env = append(os.Environ(),
		"DEVPORTAL_BE_WATCH=1",
		"DEVPORTAL_EXTRA_PID="+strconv.Itoa(extra.Process.Pid),
		"DEVPORTAL_HELPER_PID_FILE="+pidFile,
	)
	watch.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := watch.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = syscall.Kill(-watch.Process.Pid, syscall.SIGKILL) }()

	var helperPid, helperPpid int
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		raw, err := os.ReadFile(pidFile)
		if err == nil {
			fields := strings.Fields(string(raw))
			if len(fields) >= 2 {
				n, err1 := strconv.Atoi(fields[0])
				p, err2 := strconv.Atoi(fields[1])
				if err1 == nil && err2 == nil && n > 1 {
					helperPid = n
					helperPpid = p
					break
				}
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	if helperPid <= 1 {
		t.Fatal("watch process did not report helper pid")
	}
	if helperPpid == watch.Process.Pid {
		t.Fatalf("helper ppid=%d is still the watch process; Ctrl+C would reap it", helperPpid)
	}

	if err := syscall.Kill(helperPid, syscall.SIGUSR1); err != nil {
		t.Fatal(err)
	}

	waitDone := func(cmd *exec.Cmd, name string) {
		t.Helper()
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatalf("%s pid %d still alive; helper suicided before extras", name, cmd.Process.Pid)
		}
	}
	waitDone(watch, "watch")
	waitDone(extra, "extra")
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

func TestShouldKillSessionSelectsWailsTty(t *testing.T) {
	if !shouldKillSession("bun run wails:dev") {
		t.Fatal("bun run wails:dev should be killed")
	}
	if !shouldKillSession("wails3 dev") {
		t.Fatal("wails3 should be killed")
	}
	if shouldKillSession("/bin/zsh") {
		t.Fatal("zsh must not be killed")
	}
	if shouldKillSession("-/bin/zsh") {
		t.Fatal("login zsh must not be killed")
	}
	if shouldKillSession("/usr/bin/login -flp taiyop") {
		t.Fatal("login must not be killed")
	}
	if shouldKillSession("grok") {
		t.Fatal("unrelated command must not be killed")
	}
}

func TestTtySessionTargetsSkipsSelfAndShells(t *testing.T) {
	self := os.Getpid()
	for _, pid := range ttySessionTargets(self) {
		if pid == self || pid <= 1 {
			t.Fatalf("unexpected pid %d", pid)
		}
		cmd := procCommand(pid)
		if isStopAncestor(cmd) {
			t.Fatalf("shell pid %d %q", pid, cmd)
		}
		if !shouldKillSession(cmd) {
			t.Fatalf("non-session pid %d %q", pid, cmd)
		}
	}
}

func TestUniquePidsDropsInitAndDupes(t *testing.T) {
	got := uniquePids([]int{0, 1, 8, 8, 9, 1})
	if len(got) != 2 || got[0] != 8 || got[1] != 9 {
		t.Fatalf("got %v", got)
	}
}
