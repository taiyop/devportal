//go:build windows

package main

import (
	"os/exec"
	"strconv"
	"strings"
)

func isDangerousPgid(pgid int) bool {
	return false
}

func groupMatches(pid uint32, pgid int) bool {
	_ = pgid
	out, err := exec.Command("tasklist", "/FI", "PID eq "+strconv.FormatUint(uint64(pid), 10), "/NH").Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), strconv.FormatUint(uint64(pid), 10))
}

func killGroup(pid uint32, pgid int) {
	if isDangerous(pgid, pid) {
		return
	}
	if !groupMatches(pid, pgid) {
		return
	}
	_ = exec.Command("taskkill", "/PID", strconv.FormatUint(uint64(pid), 10), "/T", "/F").Run()
}
