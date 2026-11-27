//go:build unix

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func withParentWatchdog(command string) string {
	return "set +m 2>/dev/null || :; exec 3<&0; exec < /dev/null; _DEVPORTAL_PGID=$$; (cat <&3 >/dev/null 2>/dev/null; /bin/kill -TERM -- -$_DEVPORTAL_PGID 2>/dev/null; sleep 1; /bin/kill -KILL -- -$_DEVPORTAL_PGID 2>/dev/null) >/dev/null 2>&1 & _DEVPORTAL_WATCHDOG=$!; exec 3<&-\n" +
		command + "\n" +
		"_DEVPORTAL_STATUS=$?\n" +
		"/bin/kill -TERM \"$_DEVPORTAL_WATCHDOG\" 2>/dev/null || :\n" +
		"exit \"$_DEVPORTAL_STATUS\""
}

func isolateProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func attachKeepalive(cmd *exec.Cmd) (*os.File, *os.File, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return nil, nil, errString("keepalive pipe を作成できませんでした")
	}
	cmd.Stdin = r
	return r, w, nil
}

func leaderPgid(pid int) int {
	pgid, err := syscall.Getpgid(pid)
	if err != nil || pgid <= 0 {
		return pid
	}
	return pgid
}

func signalTerm(cmd *exec.Cmd, pgid int) {
	if pgid > 1 {
		_ = syscall.Kill(-pgid, syscall.SIGTERM)
		return
	}
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Signal(syscall.SIGTERM)
	}
}

func signalKill(cmd *exec.Cmd, pgid int) {
	if pgid > 1 {
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
	}
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}

func processAlive(pid uint32) bool {
	if pid == 0 {
		return false
	}
	err := syscall.Kill(int(pid), 0)
	return err != syscall.ESRCH
}

func processGroupAlive(pgid int) bool {
	if pgid <= 1 {
		return false
	}
	err := syscall.Kill(-pgid, 0)
	return err != syscall.ESRCH
}

func terminateProcess(cmd *exec.Cmd, pgid int) {
	signalTerm(cmd, pgid)
	var pid uint32
	if cmd != nil && cmd.Process != nil {
		pid = uint32(cmd.Process.Pid)
	}
	deadline := time.Now().Add(250 * time.Millisecond)
	for time.Now().Before(deadline) {
		if !processAlive(pid) && !processGroupAlive(pgid) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	if processAlive(pid) || processGroupAlive(pgid) {
		signalKill(cmd, pgid)
	}
}

func releaseListenPort(port uint16, pgid int) {
	if port == 0 {
		return
	}
	if waitFree(port, 2*time.Second) {
		return
	}
	if pgid > 1 {
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
	}
	if waitFree(port, 2*time.Second) {
		return
	}
	killPortListeners(port)
	_ = waitFree(port, 2*time.Second)
}

func killPortListeners(port uint16) {
	cmd := exec.Command("lsof", "-nP", "-t", fmt.Sprintf("-iTCP:%d", port), "-sTCP:LISTEN")
	out, err := cmd.Output()
	if err != nil {
		return
	}
	self := os.Getpid()
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		pid, convErr := strconv.Atoi(line)
		if convErr != nil || pid <= 1 || pid == self {
			continue
		}
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
}
