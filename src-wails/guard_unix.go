//go:build unix

package main

import (
	"syscall"
	"time"
)

func isDangerousPgid(pgid int) bool {
	return pgid == syscall.Getpgrp()
}

func groupMatches(pid uint32, pgid int) bool {
	err := syscall.Kill(int(pid), 0)
	if err != nil {
		if err == syscall.ESRCH {
			return false
		}
	}
	current, err := syscall.Getpgid(int(pid))
	if err != nil {
		return false
	}
	return current == pgid
}

func killGroup(pid uint32, pgid int) {
	if isDangerous(pgid, pid) {
		return
	}
	if !groupMatches(pid, pgid) {
		return
	}
	_ = syscall.Kill(-pgid, syscall.SIGTERM)
	time.Sleep(250 * time.Millisecond)
	if groupMatches(pid, pgid) {
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
	}
}
