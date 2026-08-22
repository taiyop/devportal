package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

func TestYAMLRoundtrip(t *testing.T) {
	dir := filepath.Join(os.TempDir(), "devportal-test-"+uuid.NewString())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "apps.yml")
	port := uint16(5173)
	file := ConfigFile{
		Apps: []AppEntry{{
			ID:            "abc",
			Name:          "demo",
			Description:   "demo tool",
			Hostname:      "demo",
			Folder:        "/tmp/demo",
			Command:       "npm run dev",
			PortMode:      PortModeAuto,
			Port:          &port,
			WakeOnRequest: true,
			Env:           []EnvVar{{Name: "HOST", Value: "127.0.0.1"}},
		}},
	}
	if err := saveConfig(path, file); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Apps) != 1 || loaded.Apps[0].Name != "demo" {
		t.Fatalf("got %+v", loaded)
	}
	if loaded.Apps[0].Description != "demo tool" {
		t.Fatalf("description %+v", loaded.Apps[0].Description)
	}
	if loaded.Apps[0].Hostname != "demo" {
		t.Fatalf("hostname %+v", loaded.Apps[0].Hostname)
	}
	if loaded.Apps[0].Env[0].Name != "HOST" {
		t.Fatalf("env %+v", loaded.Apps[0].Env)
	}
	if !loaded.Apps[0].WakeOnRequest {
		t.Fatal("expected wake on request")
	}
	if preferredGatewayPort(loaded) != 80 {
		t.Fatalf("expected default gateway port 80, got %+v", loaded.GatewayPort)
	}
}

func TestGatewayPortRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "apps.yml")
	port := uint16(8080)
	if err := saveConfig(path, ConfigFile{GatewayPort: &port, Apps: []AppEntry{}}); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if preferredGatewayPort(loaded) != 8080 {
		t.Fatalf("gateway port %+v", loaded.GatewayPort)
	}
}

func TestWakeOnRequestDefaultsTrue(t *testing.T) {
	var loaded ConfigFile
	err := yaml.Unmarshal([]byte(`
apps:
  - id: abc
    name: demo
    folder: /tmp/demo
    command: npm run dev
`), &loaded)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.Apps[0].WakeOnRequest {
		t.Fatal("expected default true")
	}
	if loaded.Apps[0].Description != "" {
		t.Fatalf("expected empty description, got %q", loaded.Apps[0].Description)
	}
	if preferredGatewayPort(loaded) != 80 {
		t.Fatalf("expected default gateway port 80")
	}
}

func TestYAMLBackendRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "apps.yml")
	port := uint16(5173)
	bport := uint16(8080)
	file := ConfigFile{
		Apps: []AppEntry{{
			ID:       "abc",
			Name:     "demo",
			Folder:   "/tmp/demo",
			Command:  "npm run dev",
			PortMode: PortModeAuto,
			Port:     &port,
			PortEnv:  "VITE_PORT",
			Backends: []BackendSpec{{
				Name:       "api",
				Folder:     "/tmp/demo/api",
				Command:    "go run .",
				PortMode:   PortModeManual,
				Port:       &bport,
				PortEnv:    "LISTEN_PORT",
				AppPortEnv: "API_PORT",
				Env:        []EnvVar{{Name: "DATABASE_URL", Value: "postgres://localhost/app"}},
			}},
		}},
	}
	if err := saveConfig(path, file); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Apps[0].Backends) != 1 {
		t.Fatalf("backends %+v", loaded.Apps[0].Backends)
	}
	backend := loaded.Apps[0].Backends[0]
	if backend.Command != "go run ." || backend.Name != "api" {
		t.Fatalf("backend %+v", backend)
	}
	if backend.Port == nil || *backend.Port != 8080 {
		t.Fatalf("backend port %+v", backend.Port)
	}
	if len(backend.Env) != 1 || backend.Env[0].Name != "DATABASE_URL" {
		t.Fatalf("backend env %+v", backend.Env)
	}
	if loaded.Apps[0].PortEnv != "VITE_PORT" {
		t.Fatalf("portEnv %q", loaded.Apps[0].PortEnv)
	}
	if backend.AppPortEnv != "API_PORT" {
		t.Fatalf("appPortEnv %q", backend.AppPortEnv)
	}
	if backend.PortEnv != "LISTEN_PORT" {
		t.Fatalf("backend portEnv %q", backend.PortEnv)
	}
}

func TestYAMLLegacyBackendStillLoads(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "apps.yml")
	raw := []byte(`apps:
  - id: abc
    name: demo
    folder: /tmp/demo
    command: npm run dev
    backendPortEnv: API_PORT
    backend:
      folder: /tmp/demo/api
      command: go run .
`)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Apps[0].Backends) != 1 {
		t.Fatalf("backends %+v", loaded.Apps[0].Backends)
	}
	if loaded.Apps[0].Backends[0].Command != "go run ." {
		t.Fatalf("command %q", loaded.Apps[0].Backends[0].Command)
	}
	if loaded.Apps[0].Backends[0].AppPortEnv != "API_PORT" {
		t.Fatalf("appPortEnv %q", loaded.Apps[0].Backends[0].AppPortEnv)
	}
}
