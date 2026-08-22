package main

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestMain(m *testing.M) {
	if os.Getenv("DEVPORTAL_TEST_LISTEN") != "" {
		os.Exit(listenAndBlockForTest())
	}
	os.Exit(m.Run())
}

func listenAndBlockForTest() int {
	port := os.Getenv("PORT")
	if port == "" {
		return 2
	}
	ln, err := net.Listen("tcp", net.JoinHostPort(loopbackV4, port))
	if err != nil {
		return 3
	}
	defer ln.Close()
	for {
		conn, err := ln.Accept()
		if err != nil {
			return 0
		}
		_ = conn.Close()
	}
}

func TestCommandWithEnv(t *testing.T) {
	wrapped := commandWithEnv("npm run dev", 5173, []EnvVar{{Name: "HOST", Value: "127.0.0.1"}})
	want := "env HOSTNAME='127.0.0.1' HOST='127.0.0.1' PORT='5173' DEVPORTAL_PORT='5173' npm run dev"
	if wrapped != want {
		t.Fatalf("got %q want %q", wrapped, want)
	}
}

func TestCommandInjectsLoopbackEnv(t *testing.T) {
	wrapped := commandWithEnv("npm run dev", 5173, nil)
	if !strings.Contains(wrapped, "HOST='127.0.0.1'") {
		t.Fatalf("missing HOST: %q", wrapped)
	}
	if !strings.Contains(wrapped, "HOSTNAME='127.0.0.1'") {
		t.Fatalf("missing HOSTNAME: %q", wrapped)
	}
}

func TestCommandKeepsExplicitHostname(t *testing.T) {
	wrapped := commandWithEnv("npm run dev", 5173, []EnvVar{{Name: "HOSTNAME", Value: "localhost"}})
	if !strings.Contains(wrapped, "HOSTNAME='localhost'") {
		t.Fatalf("missing explicit HOSTNAME: %q", wrapped)
	}
	if strings.Contains(wrapped, "HOSTNAME='127.0.0.1'") {
		t.Fatalf("should not override HOSTNAME: %q", wrapped)
	}
	if !strings.Contains(wrapped, "HOST='127.0.0.1'") {
		t.Fatalf("missing HOST: %q", wrapped)
	}
}

func TestCommandWithBackendPort(t *testing.T) {
	wrapped := commandWithPorts(
		"npm run dev -- --port {port}",
		portInject{
			listenPort: 5173,
			appPort:    5173,
			backends:   []backendPortEnv{{Port: 8080, Env: "BACKEND_PORT"}},
		},
		[]EnvVar{{Name: "API_URL", Value: "http://127.0.0.1:{backendPort}"}},
	)
	if !strings.Contains(wrapped, "PORT='5173'") {
		t.Fatalf("missing PORT: %q", wrapped)
	}
	if !strings.Contains(wrapped, "BACKEND_PORT='8080'") {
		t.Fatalf("missing BACKEND_PORT: %q", wrapped)
	}
	if !strings.Contains(wrapped, "API_URL='http://127.0.0.1:8080'") {
		t.Fatalf("missing rewritten API_URL: %q", wrapped)
	}
	if !strings.Contains(wrapped, "--port 5173") {
		t.Fatalf("missing rewritten command port: %q", wrapped)
	}
}

func TestCommandWithCustomPortEnv(t *testing.T) {
	wrapped := commandWithPorts(
		"npm run dev",
		portInject{listenPort: 5173, appPort: 5173, listenEnv: "VITE_PORT"},
		nil,
	)
	if !strings.Contains(wrapped, "VITE_PORT='5173'") {
		t.Fatalf("missing VITE_PORT: %q", wrapped)
	}
	if strings.Contains(wrapped, " PORT='5173'") {
		t.Fatalf("should not set default PORT: %q", wrapped)
	}
	if !strings.Contains(wrapped, "DEVPORTAL_PORT='5173'") {
		t.Fatalf("missing DEVPORTAL_PORT: %q", wrapped)
	}
}

func TestCommandWithCustomBackendPortEnv(t *testing.T) {
	wrapped := commandWithPorts(
		"npm run dev",
		portInject{
			listenPort: 5173,
			appPort:    5173,
			listenEnv:  "APP_PORT",
			backends:   []backendPortEnv{{Port: 8080, Env: "API_PORT"}},
		},
		nil,
	)
	if !strings.Contains(wrapped, "APP_PORT='5173'") {
		t.Fatalf("missing APP_PORT: %q", wrapped)
	}
	if !strings.Contains(wrapped, "API_PORT='8080'") {
		t.Fatalf("missing API_PORT: %q", wrapped)
	}
	if strings.Contains(wrapped, " PORT='5173'") {
		t.Fatalf("should not set default PORT: %q", wrapped)
	}
	if strings.Contains(wrapped, " BACKEND_PORT=") {
		t.Fatalf("should not set default BACKEND_PORT: %q", wrapped)
	}
}

func TestCommandWithTwoBackendPorts(t *testing.T) {
	wrapped := commandWithPorts(
		"npm run dev -- --api {backendPort:1} --worker {backendPort:worker}",
		portInject{
			listenPort: 5173,
			appPort:    5173,
			backends: []backendPortEnv{
				{Port: 8080, Env: "API_PORT", Name: "api"},
				{Port: 9000, Env: "WORKER_PORT", Name: "worker"},
			},
		},
		nil,
	)
	if !strings.Contains(wrapped, "API_PORT='8080'") {
		t.Fatalf("missing API_PORT: %q", wrapped)
	}
	if !strings.Contains(wrapped, "WORKER_PORT='9000'") {
		t.Fatalf("missing WORKER_PORT: %q", wrapped)
	}
	if !strings.Contains(wrapped, "--api 8080") || !strings.Contains(wrapped, "--worker 9000") {
		t.Fatalf("missing rewritten ports: %q", wrapped)
	}
	if !strings.Contains(wrapped, "DEVPORTAL_BACKEND_PORT_2='9000'") {
		t.Fatalf("missing DEVPORTAL_BACKEND_PORT_2: %q", wrapped)
	}
}

func TestSpawnAndTerminateSleep(t *testing.T) {
	cmd, keepalive, pgid, stdout, stderr, err := spawnCommand(os.TempDir(), "sleep 30", 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer stdout.Close()
	defer stderr.Close()
	if cmd.Process == nil || cmd.Process.Pid <= 0 {
		t.Fatal("expected pid")
	}
	rt := &Runtime{cmd: cmd, pgid: pgid, keepalive: keepalive, waitCh: make(chan struct{})}
	go func() { _ = rt.wait() }()
	terminateRuntime(rt)
	select {
	case <-rt.waitCh:
	case <-time.After(3 * time.Second):
		t.Fatal("child process should be gone after terminate")
	}
}

func TestClosingKeepaliveKillsProcessGroup(t *testing.T) {
	if isWindows() {
		t.Skip("keepalive is unix-only")
	}
	cmd, keepalive, pgid, stdout, stderr, err := spawnCommand(os.TempDir(), "sleep 30", 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer stdout.Close()
	defer stderr.Close()
	rt := &Runtime{cmd: cmd, pgid: pgid, keepalive: keepalive, waitCh: make(chan struct{})}
	go func() { _ = rt.wait() }()
	_ = keepalive.Close()
	rt.keepalive = nil
	select {
	case <-rt.waitCh:
	case <-time.After(4 * time.Second):
		terminateRuntime(rt)
		t.Fatal("child should exit after parent keepalive closes")
	}
}

func TestTerminateRuntimeReleasesListeningPort(t *testing.T) {
	if isWindows() {
		t.Skip("port release is unix-oriented")
	}
	helper, err := filepath.Abs(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	port, err := findFreePort()
	if err != nil {
		t.Fatal(err)
	}
	// Re-exec this test binary as the child. python3 -m http.server calls
	// socket.getfqdn() on bind, which hangs for a minute-plus on GitHub
	// Actions macOS 15+ runners, so the 3s ready wait always times out.
	cmd, keepalive, pgid, stdout, stderr, err := spawnCommand(
		os.TempDir(),
		shSingleQuote(helper),
		port,
		[]EnvVar{{Name: "DEVPORTAL_TEST_LISTEN", Value: "1"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer stdout.Close()
	defer stderr.Close()
	rt := &Runtime{cmd: cmd, port: port, pgid: pgid, keepalive: keepalive, waitCh: make(chan struct{})}
	go func() { _ = rt.wait() }()
	deadline := time.Now().Add(3 * time.Second)
	for !isOpen(port) {
		select {
		case <-rt.waitCh:
			terminateRuntime(rt)
			t.Fatal("child exited before listening")
		default:
		}
		if time.Now().After(deadline) {
			terminateRuntime(rt)
			t.Fatal("child listener did not listen")
		}
		time.Sleep(30 * time.Millisecond)
	}
	terminateRuntime(rt)
	if !isFree(port) {
		t.Fatal("listen port should be free after terminate")
	}
}

func TestPersistedPgidIsReaped(t *testing.T) {
	if isWindows() {
		t.Skip("pgid reap is unix-oriented")
	}
	dir := filepath.Join(os.TempDir(), "devportal-reap-"+uuid.NewString())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "running.yml")
	cmd, keepalive, pgid, stdout, stderr, err := spawnCommand(os.TempDir(), "sleep 30", 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer stdout.Close()
	defer stderr.Close()
	rt := &Runtime{cmd: cmd, pgid: pgid, keepalive: keepalive, waitCh: make(chan struct{})}
	go func() { _ = rt.wait() }()
	if err := registerRunning(path, RunningGroup{
		ID:   "sleep",
		PID:  uint32(cmd.Process.Pid),
		PGID: pgid,
		Port: 1,
	}); err != nil {
		t.Fatal(err)
	}
	reapPersisted(path)
	select {
	case <-rt.waitCh:
	case <-time.After(3 * time.Second):
		terminateRuntime(rt)
		t.Fatal("persisted process group should be killed on reap")
	}
}

func TestUpsertBackendAndRejectPortClash(t *testing.T) {
	dir := t.TempDir()
	appDir := filepath.Join(dir, "app")
	backDir := filepath.Join(dir, "api")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(backDir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := &Portal{inner: Inner{
		configPath:  filepath.Join(dir, "apps.yml"),
		runningPath: filepath.Join(dir, "running.yml"),
		file:        ConfigFile{Apps: []AppEntry{}},
		runtime:     map[string]*Runtime{},
		errors:      map[string]string{},
		logs:        map[string][]LogEvent{},
	}}
	id := "app-1"
	port := uint16(5173)
	bport := uint16(8080)
	entry, err := p.Upsert(AppInput{
		ID:       &id,
		Name:     "demo",
		Folder:   appDir,
		Command:  "npm run dev",
		PortMode: PortModeManual,
		Port:     &port,
		Backends: []BackendSpec{{
			Folder:   backDir,
			Command:  "go run .",
			PortMode: PortModeManual,
			Port:     &bport,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(entry.Backends) != 1 || entry.Backends[0].Command != "go run ." {
		t.Fatalf("backends %+v", entry.Backends)
	}
	if entry.Backends[0].Folder != backDir {
		t.Fatalf("folder %q", entry.Backends[0].Folder)
	}
	_, err = p.Upsert(AppInput{
		ID:       &id,
		Name:     "demo",
		Folder:   appDir,
		Command:  "npm run dev",
		PortMode: PortModeManual,
		Port:     &port,
		Backends: []BackendSpec{{
			Folder:   backDir,
			Command:  "go run .",
			PortMode: PortModeManual,
			Port:     &port,
		}},
	})
	if err == nil {
		t.Fatal("expected port clash")
	}
}

func TestUpsertRejectsSamePortEnvNames(t *testing.T) {
	dir := t.TempDir()
	appDir := filepath.Join(dir, "app")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := &Portal{inner: Inner{
		configPath:  filepath.Join(dir, "apps.yml"),
		runningPath: filepath.Join(dir, "running.yml"),
		file:        ConfigFile{Apps: []AppEntry{}},
		runtime:     map[string]*Runtime{},
		errors:      map[string]string{},
		logs:        map[string][]LogEvent{},
	}}
	id := "app-1"
	_, err := p.Upsert(AppInput{
		ID:      &id,
		Name:    "demo",
		Folder:  appDir,
		Command: "npm run dev",
		PortEnv: "PORT",
		Backends: []BackendSpec{{
			Folder:     appDir,
			Command:    "go run .",
			AppPortEnv: "PORT",
		}},
	})
	if err == nil {
		t.Fatal("expected env name clash")
	}
}

func TestStartAndStopBackendProcess(t *testing.T) {
	if isWindows() {
		t.Skip("process groups are unix-oriented")
	}
	dir := t.TempDir()
	appDir := filepath.Join(dir, "app")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatal(err)
	}
	id := "app-1"
	p := &Portal{inner: Inner{
		configPath:  filepath.Join(dir, "apps.yml"),
		runningPath: filepath.Join(dir, "running.yml"),
		file: ConfigFile{Apps: []AppEntry{{
			ID:       id,
			Name:     "demo",
			Folder:   appDir,
			Command:  "sleep 30",
			PortMode: PortModeAuto,
			Backends: []BackendSpec{{
				Folder:   appDir,
				Command:  "sleep 30",
				PortMode: PortModeAuto,
			}},
		}}},
		runtime: map[string]*Runtime{},
		logs:    map[string][]LogEvent{},
		errors:  map[string]string{},
	}}
	view, err := p.Start(id)
	if err != nil {
		t.Fatal(err)
	}
	if view.PID == nil || *view.PID == 0 {
		t.Fatal("expected frontend pid")
	}
	if len(view.BackendPIDs) != 1 || view.BackendPIDs[0] == 0 {
		t.Fatal("expected backend pid")
	}
	frontendPID := *view.PID
	backendPID := view.BackendPIDs[0]
	if frontendPID == backendPID {
		t.Fatal("expected distinct pids")
	}
	if !processAlive(frontendPID) || !processAlive(backendPID) {
		_, _ = p.Stop(id)
		t.Fatal("both processes should be alive")
	}
	if _, err := p.Stop(id); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for processAlive(frontendPID) || processAlive(backendPID) {
		if time.Now().After(deadline) {
			t.Fatal("both processes should be gone after stop")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestStartAndStopTwoBackendProcesses(t *testing.T) {
	if isWindows() {
		t.Skip("process groups are unix-oriented")
	}
	dir := t.TempDir()
	appDir := filepath.Join(dir, "app")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatal(err)
	}
	id := "app-1"
	p := &Portal{inner: Inner{
		configPath:  filepath.Join(dir, "apps.yml"),
		runningPath: filepath.Join(dir, "running.yml"),
		file: ConfigFile{Apps: []AppEntry{{
			ID:       id,
			Name:     "demo",
			Folder:   appDir,
			Command:  "sleep 30",
			PortMode: PortModeAuto,
			Backends: []BackendSpec{
				{Name: "api", Folder: appDir, Command: "sleep 30", PortMode: PortModeAuto},
				{Name: "worker", Folder: appDir, Command: "sleep 30", PortMode: PortModeAuto},
			},
		}}},
		runtime: map[string]*Runtime{},
		logs:    map[string][]LogEvent{},
		errors:  map[string]string{},
	}}
	view, err := p.Start(id)
	if err != nil {
		t.Fatal(err)
	}
	if view.PID == nil || *view.PID == 0 {
		t.Fatal("expected frontend pid")
	}
	if len(view.BackendPIDs) != 2 || view.BackendPIDs[0] == 0 || view.BackendPIDs[1] == 0 {
		t.Fatalf("backend pids %+v", view.BackendPIDs)
	}
	if view.BackendPIDs[0] == view.BackendPIDs[1] {
		t.Fatal("expected distinct backend pids")
	}
	pids := []uint32{*view.PID, view.BackendPIDs[0], view.BackendPIDs[1]}
	for _, pid := range pids {
		if !processAlive(pid) {
			_, _ = p.Stop(id)
			t.Fatalf("pid %d should be alive", pid)
		}
	}
	if _, err := p.Stop(id); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		alive := false
		for _, pid := range pids {
			if processAlive(pid) {
				alive = true
			}
		}
		if !alive {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("all processes should be gone after stop")
		}
		time.Sleep(20 * time.Millisecond)
	}
}
