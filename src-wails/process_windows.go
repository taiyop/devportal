//go:build windows

package main

import (
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"
)

func withParentWatchdog(command string) string {
	return command
}

func isolateProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000200}
}

func attachKeepalive(cmd *exec.Cmd) (*os.File, *os.File, error) {
	devNull, err := os.OpenFile(os.DevNull, os.O_RDONLY, 0)
	if err != nil {
		return nil, nil, err
	}
	cmd.Stdin = devNull
	return devNull, nil, nil
}

func leaderPgid(pid int) int {
	return pid
}

func signalTerm(cmd *exec.Cmd, pgid int) {
	_ = pgid
	if cmd != nil && cmd.Process != nil {
		_ = exec.Command("taskkill", "/PID", strconv.Itoa(cmd.Process.Pid), "/T").Run()
	}
}

func signalKill(cmd *exec.Cmd, pgid int) {
	_ = pgid
	if cmd != nil && cmd.Process != nil {
		_ = exec.Command("taskkill", "/PID", strconv.Itoa(cmd.Process.Pid), "/T", "/F").Run()
		_ = cmd.Process.Kill()
	}
}

func processAlive(pid uint32) bool {
	if pid == 0 {
		return false
	}
	const synchronize = 0x00100000
	h, err := syscall.OpenProcess(synchronize, false, pid)
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(h)
	status, err := syscall.WaitForSingleObject(h, 0)
	if err != nil {
		return false
	}
	return status == syscall.WAIT_TIMEOUT
}

func terminateProcess(cmd *exec.Cmd, pgid int) {
	signalTerm(cmd, pgid)
	var pid uint32
	if cmd != nil && cmd.Process != nil {
		pid = uint32(cmd.Process.Pid)
	}
	deadline := time.Now().Add(250 * time.Millisecond)
	for time.Now().Before(deadline) {
		if !processAlive(pid) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	if processAlive(pid) {
		signalKill(cmd, pgid)
	}
}

func releaseListenPort(port uint16, pgid int) {
	_ = pgid
	if port == 0 {
		return
	}
	_ = waitFree(port, 3*time.Second)
}
