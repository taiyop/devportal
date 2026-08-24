//go:build unix

package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type forceKiller struct {
	helperPid int
}

var (
	killerOnce    sync.Once
	sessionKiller *forceKiller
)

func startForceKiller() {
	killerOnce.Do(func() {
		self := os.Getpid()
		k, err := newForceKiller(self, sessionKillTargets(self)...)
		if err != nil {
			fmt.Fprintf(os.Stderr, "force killer を起動できません: %v\n", err)
			return
		}
		sessionKiller = k
	})
}

// newForceKiller starts a sibling in its own process group. It waits for
// SIGUSR1 (or SIGTERM), then SIGKILLs watchPid, extras, and those process
// trees from outside this process.
//
// Triggering with kill(helper, SIGUSR1) is the same class of call as
// terminating a registered app (kill other pid). kill(self), libc exit,
// unlink, and pipe close all deadlock once AppKit owns the main thread.
func newForceKiller(watchPid int, extras ...int) (*forceKiller, error) {
	if watchPid <= 1 {
		return nil, errString("force killer has no target")
	}
	args := []string{"-c", forceKillerScript, "devportal-killer", strconv.Itoa(watchPid)}
	for _, pid := range extras {
		if pid > 1 && pid != watchPid {
			args = append(args, strconv.Itoa(pid))
		}
	}
	cmd := exec.Command("/bin/sh", args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	go func() { _ = cmd.Wait() }()
	fmt.Fprintf(os.Stderr, "force killer pid=%d watch=%d extras=%v\n", cmd.Process.Pid, watchPid, extras)
	_ = os.WriteFile(filepath.Join(os.TempDir(), fmt.Sprintf("devportal-killer-%d", watchPid)), []byte(strconv.Itoa(cmd.Process.Pid)+"\n"), 0o600)
	return &forceKiller{helperPid: cmd.Process.Pid}, nil
}

const forceKillerScript = `
trap '' HUP
got=0
trap 'got=1' USR1 INT TERM
watch=$1
shift
while [ "$got" = 0 ] && /bin/kill -0 "$watch" 2>/dev/null; do
  /bin/sleep 0.05
done
kill_tree() {
  _pid=$1
  [ -n "$_pid" ] || return 0
  case "$_pid" in
    ''|*[!0-9]*) return 0 ;;
  esac
  if [ "$_pid" -le 1 ]; then
    return 0
  fi
  for _c in $(pgrep -P "$_pid" 2>/dev/null); do
    kill_tree "$_c"
  done
  _pgid=$(ps -o pgid= -p "$_pid" 2>/dev/null | tr -d ' ')
  /bin/kill -9 "$_pid" 2>/dev/null || :
  # Only signal a process group when this pid is the group leader. Killing
  # -$pgid for a process that inherited the terminal/harness group would
  # take down unrelated jobs.
  if [ -n "$_pgid" ] && [ "$_pgid" = "$_pid" ] && [ "$_pgid" -gt 1 ]; then
    /bin/kill -9 -"$_pgid" 2>/dev/null || :
  fi
}
kill_tree "$watch"
for _t in "$@"; do
  kill_tree "$_t"
done
`

func (k *forceKiller) trip() {
	if k == nil || k.helperPid <= 1 {
		return
	}
	_ = syscall.Kill(k.helperPid, syscall.SIGUSR1)
}

func rawExit(code int) {
	_ = code
	fmt.Fprintln(os.Stderr, "プロセスを終了します。")
	if sessionKiller == nil {
		fmt.Fprintln(os.Stderr, "force killer が起動していません")
	} else {
		sessionKiller.trip()
	}
	for {
		time.Sleep(100 * time.Millisecond)
	}
}

func sessionKillTargets(self int) []int {
	seen := map[int]bool{self: true, 0: true, 1: true}
	var extras []int
	pid := parentPid(self)
	for i := 0; i < 12 && pid > 1 && !seen[pid]; i++ {
		seen[pid] = true
		cmd := procCommand(pid)
		if cmd == "" || isStopAncestor(cmd) {
			break
		}
		if shouldKillAncestor(cmd) || i == 0 {
			extras = append(extras, pid)
		} else {
			break
		}
		pid = parentPid(pid)
	}
	return extras
}

func parentPid(pid int) int {
	out, err := exec.Command("ps", "-o", "ppid=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return 0
	}
	n, convErr := strconv.Atoi(strings.TrimSpace(string(out)))
	if convErr != nil {
		return 0
	}
	return n
}

func procCommand(pid int) string {
	out, err := exec.Command("ps", "-o", "command=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return ""
	}
	return string(bytes.TrimSpace(out))
}

func isStopAncestor(cmd string) bool {
	fields := strings.Fields(cmd)
	if len(fields) == 0 {
		return true
	}
	base := strings.ToLower(filepath.Base(fields[0]))
	if strings.HasPrefix(base, "-") {
		return true
	}
	switch base {
	case "zsh", "fish", "login", "sshd":
		return true
	default:
		return false
	}
}

func shouldKillAncestor(cmd string) bool {
	c := strings.ToLower(cmd)
	if strings.Contains(c, "wails") || strings.Contains(c, "devportal") || strings.Contains(c, "vite") {
		return true
	}
	fields := strings.Fields(c)
	if len(fields) == 0 {
		return false
	}
	base := filepath.Base(fields[0])
	switch base {
	case "bun", "node", "task":
		return true
	default:
		return false
	}
}
