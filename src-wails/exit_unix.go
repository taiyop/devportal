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
	killerOnce         sync.Once
	sessionKiller      *forceKiller
	startupKillTargets []int
	startupTty         string
	startupTargetsOnce sync.Once
)

func init() {
	// Capture before application.New() / AppKit. GUI init can reparent
	// this process to launchd, which would make parent-walking empty.
	// bun/wails3 also sit in a different pgid than the .app, so parent
	// walking alone misses them — include everyone on the same tty.
	startupTargetsOnce.Do(func() {
		self := os.Getpid()
		startupTty = procTty(self)
		startupKillTargets = uniquePids(append(sessionKillTargets(self), ttySessionTargets(self)...))
	})
}

func startForceKiller() {
	killerOnce.Do(func() {
		self := os.Getpid()
		extras := startupKillTargets
		if len(extras) == 0 {
			extras = uniquePids(append(sessionKillTargets(self), ttySessionTargets(self)...))
		}
		tty := startupTty
		if tty == "" {
			tty = procTty(self)
		}
		k, err := spawnForceKiller(self, tty, extras)
		if err != nil {
			fmt.Fprintf(os.Stderr, "force killer を起動できません: %v\n", err)
			return
		}
		sessionKiller = k
	})
}

// newForceKiller starts a detached helper in its own process group, then
// reparents it to launchd so it is not a child of DevPortal. It waits for
// SIGUSR1 (or SIGTERM), then SIGKILLs watchPid, extras, and those process
// trees from outside this process.
//
// It must not stay a child: wails3's Ctrl+C handler kills the primary tree
// and would reap a child helper, leaving bun/wails3 holding the tty.
// Triggering with kill(helper, SIGUSR1) is the same class of call as
// terminating a registered app (kill other pid). kill(self), libc exit,
// unlink, and pipe close all deadlock once AppKit owns the main thread.
func newForceKiller(watchPid int, extras ...int) (*forceKiller, error) {
	return spawnForceKiller(watchPid, "", extras)
}

func spawnForceKiller(watchPid int, tty string, extras []int) (*forceKiller, error) {
	if watchPid <= 1 {
		return nil, errString("force killer has no target")
	}
	scriptFile := filepath.Join(os.TempDir(), fmt.Sprintf("devportal-killer-script-%d.sh", watchPid))
	pidFile := filepath.Join(os.TempDir(), fmt.Sprintf("devportal-killer-%d", watchPid))
	if err := os.WriteFile(scriptFile, []byte(strings.TrimSpace(forceKillerScript)+"\n"), 0o700); err != nil {
		return nil, err
	}
	args := []string{
		"-c",
		`script=$1
pidfile=$2
shift 2
export DP_TTY
# Start the helper and exit so launchd reparents it. If the helper stayed
# a child of DevPortal, wails3's Ctrl+C cleanup would SIGKILL it with the
# app and leave bun/wails3 holding the tty.
if command -v perl >/dev/null 2>&1; then
  perl -e 'setpgrp; exec { $ARGV[0] } @ARGV' /bin/sh "$script" "$@" </dev/null >/dev/null 2>&1 &
else
  /bin/sh "$script" "$@" </dev/null >/dev/null 2>&1 &
fi
echo $! > "$pidfile"`,
		"launcher",
		scriptFile,
		pidFile,
		strconv.Itoa(watchPid),
	}
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
	if tty != "" && tty != "?" && tty != "??" {
		cmd.Env = append(os.Environ(), "DP_TTY="+tty)
	}
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(pidFile)
	if err != nil {
		return nil, err
	}
	helperPid, convErr := strconv.Atoi(strings.TrimSpace(string(raw)))
	if convErr != nil || helperPid <= 1 {
		return nil, errString("force killer pid missing")
	}
	if err := syscall.Kill(helperPid, 0); err != nil {
		return nil, errString("force killer did not stay alive")
	}
	fmt.Fprintf(os.Stderr, "force killer pid=%d watch=%d extras=%v tty=%s\n", helperPid, watchPid, extras, tty)
	return &forceKiller{helperPid: helperPid}, nil
}

const forceKillerScript = `
trap '' HUP TSTP TTIN TTOU
got=0
trap 'got=1' USR1 INT TERM
self=$$
watch=$1
shift
tty=${DP_TTY:-}
while [ "$got" = 0 ] && /bin/kill -0 "$watch" 2>/dev/null; do
  /bin/sleep 0.05
done
kill_tree() {
  _pid=$1
  [ -n "$_pid" ] || return 0
  case "$_pid" in
    ''|*[!0-9]*) return 0 ;;
  esac
  if [ "$_pid" -le 1 ] || [ "$_pid" = "$self" ]; then
    return 0
  fi
  # ps, not pgrep: macOS pgrep -P from a descendant omits processes we need.
  for _c in $(ps -ax -o pid=,ppid= 2>/dev/null | awk -v p="$_pid" '$2+0 == p+0 { print $1+0 }'); do
    kill_tree "$_c"
  done
  _pgid=$(ps -o pgid= -p "$_pid" 2>/dev/null | tr -d ' ')
  /bin/kill -9 "$_pid" 2>/dev/null || :
  # Only signal a process group when this pid is the group leader. Killing
  # -$pgid for a process that inherited the terminal/harness group would
  # take down unrelated jobs.
  if [ -n "$_pgid" ] && [ "$_pgid" = "$_pid" ] && [ "$_pgid" -gt 1 ] && [ "$_pgid" != "$self" ]; then
    /bin/kill -9 -"$_pgid" 2>/dev/null || :
  fi
}
# Watchers first so the tty is released even if killing DevPortal races.
for _t in "$@"; do
  kill_tree "$_t"
done
kill_tree "$watch"
# bun/wails3 live in a different pgid than the .app. Parent walking misses
# them after launchd reparent; sweep the recorded tty as a last pass.
if [ -n "$tty" ] && [ "$tty" != "?" ] && [ "$tty" != "??" ]; then
  ps -t "$tty" -o pid=,command= 2>/dev/null | while IFS= read -r _line; do
    _pid=$(echo "$_line" | awk '{print $1}')
    _cmd=$(echo "$_line" | sed 's/^ *[0-9][0-9]* *//')
    case "$_pid" in
      ''|*[!0-9]*) continue ;;
    esac
    if [ "$_pid" -le 1 ] || [ "$_pid" = "$self" ]; then
      continue
    fi
    _lc=$(echo "$_cmd" | tr '[:upper:]' '[:lower:]')
    _kill=0
    case "$_lc" in
      *wails*|*devportal*|*vite*) _kill=1 ;;
    esac
    _base=$(echo "$_lc" | awk '{print $1}')
    _base=${_base##*/}
    case "$_base" in
      bun|task) _kill=1 ;;
    esac
    if [ "$_kill" = 1 ]; then
      /bin/kill -9 "$_pid" 2>/dev/null || :
    fi
  done
fi
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
	reapStartupTargets()
	if sessionKiller == nil {
		fmt.Fprintln(os.Stderr, "force killer が起動していません")
	} else {
		sessionKiller.trip()
	}
	for {
		time.Sleep(100 * time.Millisecond)
	}
}

func reapStartupTargets() {
	self := os.Getpid()
	selfPgid := syscall.Getpgrp()
	for _, pid := range startupKillTargets {
		if pid <= 1 || pid == self {
			continue
		}
		cmd := procCommand(pid)
		if cmd == "" || isStopAncestor(cmd) || !shouldKillSession(cmd) {
			continue
		}
		if pgid, err := syscall.Getpgid(pid); err == nil && pgid == pid && pgid > 1 && pgid != selfPgid {
			_ = syscall.Kill(-pgid, syscall.SIGKILL)
		}
		_ = syscall.Kill(pid, syscall.SIGKILL)
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

func shouldKillSession(cmd string) bool {
	if isStopAncestor(cmd) {
		return false
	}
	c := strings.ToLower(cmd)
	if strings.Contains(c, "wails") || strings.Contains(c, "devportal") || strings.Contains(c, "vite") {
		return true
	}
	fields := strings.Fields(c)
	if len(fields) == 0 {
		return false
	}
	switch filepath.Base(fields[0]) {
	case "bun", "task":
		return true
	default:
		return false
	}
}

func procTty(pid int) string {
	out, err := exec.Command("ps", "-o", "tty=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return ""
	}
	tty := strings.TrimSpace(string(out))
	if tty == "" || tty == "?" || tty == "??" {
		if link, linkErr := os.Readlink("/dev/fd/0"); linkErr == nil {
			tty = strings.TrimPrefix(link, "/dev/")
		}
	}
	if tty == "?" || tty == "??" {
		return ""
	}
	return tty
}

func ttySessionTargets(self int) []int {
	tty := procTty(self)
	if tty == "" {
		return nil
	}
	out, err := exec.Command("ps", "-t", tty, "-o", "pid=,command=").Output()
	if err != nil || len(bytes.TrimSpace(out)) == 0 {
		if !strings.HasPrefix(tty, "tty") {
			out, err = exec.Command("ps", "-t", "tty"+tty, "-o", "pid=,command=").Output()
		}
		if err != nil {
			return nil
		}
	}
	var pids []int
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		pid, convErr := strconv.Atoi(fields[0])
		if convErr != nil || pid <= 1 || pid == self {
			continue
		}
		cmd := strings.TrimSpace(line[len(fields[0]):])
		if shouldKillSession(cmd) {
			pids = append(pids, pid)
		}
	}
	return pids
}

func uniquePids(pids []int) []int {
	seen := map[int]bool{0: true, 1: true}
	out := make([]int, 0, len(pids))
	for _, pid := range pids {
		if pid <= 1 || seen[pid] {
			continue
		}
		seen[pid] = true
		out = append(out, pid)
	}
	return out
}
