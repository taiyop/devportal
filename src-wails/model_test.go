package main

import "testing"

func TestNormalizeFolder(t *testing.T) {
	if got := normalizeFolder("  file:///Users/me/my%20app  "); got != "/Users/me/my app" {
		t.Fatalf("got %q", got)
	}
	if got := normalizeFolder("file://localhost/Users/me/app"); got != "/Users/me/app" {
		t.Fatalf("got %q", got)
	}
	if got := normalizeFolder("/plain/path"); got != "/plain/path" {
		t.Fatalf("got %q", got)
	}
}

func TestInjectPort(t *testing.T) {
	if got := injectPort("vite --port {port}", 4173); got != "vite --port 4173" {
		t.Fatalf("got %q", got)
	}
}

func TestInjectPorts(t *testing.T) {
	got := injectPorts("vite --port {port} --api {backendPort}", 5173, 8080)
	if got != "vite --port 5173 --api 8080" {
		t.Fatalf("got %q", got)
	}
	if got := injectPorts("keep {backendPort}", 1, 0); got != "keep {backendPort}" {
		t.Fatalf("got %q", got)
	}
}

func TestNormalizeBackendEmpty(t *testing.T) {
	got, err := normalizeBackend(&BackendSpec{}, "/tmp/app", 0)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("expected nil backend, got %+v", got)
	}
}

func TestNormalizeBackendRequiresCommand(t *testing.T) {
	_, err := normalizeBackend(&BackendSpec{Folder: "/tmp"}, "/tmp/app", 0)
	if err == nil {
		t.Fatal("expected command error")
	}
}

func TestInjectPortMapNamedAndIndexed(t *testing.T) {
	got := injectPortMap(
		"api {backendPort:api} worker {backendPort:2} first {backendPort}",
		portMap{app: 5173, backends: []uint16{8080, 9000}, names: []string{"api", "worker"}},
	)
	if got != "api 8080 worker 9000 first 8080" {
		t.Fatalf("got %q", got)
	}
}

func TestDefaultNameFromFolder(t *testing.T) {
	if got := defaultNameFromFolder("/tmp/demo-app"); got != "demo-app" {
		t.Fatalf("got %q", got)
	}
}

func TestNormalizePortEnv(t *testing.T) {
	got, err := normalizePortEnv("", defaultListenPortEnv, "ポート")
	if err != nil {
		t.Fatal(err)
	}
	if got != "PORT" {
		t.Fatalf("got %q", got)
	}
	got, err = normalizePortEnv("  VITE_PORT  ", defaultListenPortEnv, "ポート")
	if err != nil {
		t.Fatal(err)
	}
	if got != "VITE_PORT" {
		t.Fatalf("got %q", got)
	}
	if _, err := normalizePortEnv("1BAD", defaultListenPortEnv, "ポート"); err == nil {
		t.Fatal("expected invalid env name")
	}
}

func TestNormalizeBackendKeepsPortEnv(t *testing.T) {
	got, err := normalizeBackend(&BackendSpec{
		Command:    "go run .",
		PortEnv:    "API_PORT",
		AppPortEnv: "VITE_API_PORT",
	}, "/tmp", 0)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.PortEnv != "API_PORT" || got.AppPortEnv != "VITE_API_PORT" {
		t.Fatalf("got %+v", got)
	}
}

func TestListenPortEnvDefaults(t *testing.T) {
	entry := AppEntry{}
	if entry.listenPortEnv() != "PORT" {
		t.Fatalf("got %q", entry.listenPortEnv())
	}
	entry.PortEnv = "APP_PORT"
	if entry.listenPortEnv() != "APP_PORT" {
		t.Fatalf("got %q", entry.listenPortEnv())
	}
	spec := BackendSpec{}
	if spec.appPortEnv(0) != "BACKEND_PORT" {
		t.Fatalf("got %q", spec.appPortEnv(0))
	}
	if spec.appPortEnv(1) != "BACKEND_PORT_2" {
		t.Fatalf("got %q", spec.appPortEnv(1))
	}
}

func TestNormalizeEnv(t *testing.T) {
	vars, err := normalizeEnv([]EnvVar{
		{Name: "  HOST  ", Value: "127.0.0.1"},
		{Name: "", Value: ""},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(vars) != 1 || vars[0].Name != "HOST" {
		t.Fatalf("got %+v", vars)
	}
	if _, err := normalizeEnv([]EnvVar{{Name: "1BAD", Value: "x"}}); err == nil {
		t.Fatal("expected invalid env name")
	}
}
