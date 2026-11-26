package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newControlAPITestPortal(t *testing.T) (*Portal, string, string) {
	t.Helper()
	dir := t.TempDir()
	appDir := filepath.Join(dir, "app")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(dir, "apps.yml")
	if err := saveConfig(cfgPath, ConfigFile{}); err != nil {
		t.Fatal(err)
	}
	p := &Portal{inner: Inner{
		configPath:  cfgPath,
		runningPath: filepath.Join(dir, "running.yml"),
		file:        ConfigFile{Categories: []CategoryEntry{}, Apps: []AppEntry{}},
		runtime:     map[string]*Runtime{},
		logs:        map[string][]LogEvent{},
		errors:      map[string]string{},
		favicons:    map[string]string{},
	}}
	if err := p.listenFirstAvailableControlAPI([]string{"127.0.0.1:0"}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.StopControlAPI)
	st := p.ControlAPIStatus()
	return p, appDir, st.Token
}

func doControlReq(t *testing.T, handler http.Handler, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != nil {
		switch v := body.(type) {
		case string:
			reader = strings.NewReader(v)
		default:
			raw, err := json.Marshal(v)
			if err != nil {
				t.Fatal(err)
			}
			reader = bytes.NewReader(raw)
		}
	}
	req := httptest.NewRequest(method, path, reader)
	req.Host = "127.0.0.1:17341"
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestControlAPISecurity(t *testing.T) {
	p, _, token := newControlAPITestPortal(t)
	h := p.controlAPIHandler()

	// Missing token -> 401
	rec := doControlReq(t, h, http.MethodGet, "/api/v1/health", "", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("missing token code %d body %s", rec.Code, rec.Body.String())
	}

	// Wrong token -> 401
	rec = doControlReq(t, h, http.MethodGet, "/api/v1/health", "dp_wrong", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong token code %d body %s", rec.Code, rec.Body.String())
	}

	// Browser Origin header -> 403
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	req.Host = "127.0.0.1:17341"
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Origin", "https://evil.example")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("origin code %d body %s", rec.Code, rec.Body.String())
	}

	// Non-loopback Host header (DNS rebinding) -> 403
	req = httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	req.Host = "evil.example:17341"
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("host code %d body %s", rec.Code, rec.Body.String())
	}

	// Valid loopback + token -> 200
	rec = doControlReq(t, h, http.MethodGet, "/api/v1/health", token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("valid health code %d body %s", rec.Code, rec.Body.String())
	}
}

func TestControlAPIDiscoveryFileAndTokenRotation(t *testing.T) {
	p, _, firstToken := newControlAPITestPortal(t)
	st := p.ControlAPIStatus()
	if !st.Running || st.Port == 0 {
		t.Fatalf("unexpected status %+v", st)
	}
	if !strings.HasPrefix(firstToken, "dp_") {
		t.Fatalf("unexpected token format %q", firstToken)
	}

	raw, err := os.ReadFile(st.InfoPath)
	if err != nil {
		t.Fatalf("read api.json: %v", err)
	}
	var info ControlAPIFileInfo
	if err := json.Unmarshal(raw, &info); err != nil {
		t.Fatal(err)
	}
	if info.URL != st.URL || info.Token != firstToken || info.Port != st.Port {
		t.Fatalf("mismatch api.json %+v vs %+v", info, st)
	}

	// Verify real HTTP call against bound loopback listener
	req, err := http.NewRequest(http.MethodGet, info.URL+"/api/v1/schema", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+info.Token)
	resp, err := (&http.Client{Timeout: 2 * time.Second}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("schema status %d", resp.StatusCode)
	}

	// Rotate token: old token rejected, new token accepted, api.json updated
	rotated, err := p.RotateControlAPIToken()
	if err != nil {
		t.Fatal(err)
	}
	if rotated.Token == "" || rotated.Token == firstToken {
		t.Fatalf("token did not rotate: %q", rotated.Token)
	}

	h := p.controlAPIHandler()
	if rec := doControlReq(t, h, http.MethodGet, "/api/v1/health", firstToken, nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("old token should fail, got %d", rec.Code)
	}
	if rec := doControlReq(t, h, http.MethodGet, "/api/v1/health", rotated.Token, nil); rec.Code != http.StatusOK {
		t.Fatalf("new token should succeed, got %d", rec.Code)
	}

	raw, err = os.ReadFile(st.InfoPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &info); err != nil {
		t.Fatal(err)
	}
	if info.Token != rotated.Token {
		t.Fatalf("api.json token not updated: %+v", info)
	}

	// Stop removes api.json
	infoPath := st.InfoPath
	p.StopControlAPI()
	if _, err := os.Stat(infoPath); !os.IsNotExist(err) {
		t.Fatalf("api.json should be removed on stop, err=%v", err)
	}
}

func TestControlAPIAppsCRUDPatchAndEvents(t *testing.T) {
	p, appDir, token := newControlAPITestPortal(t)
	h := p.controlAPIHandler()

	var statusEvents []AppView
	var appsChangedEvents []AppsChangedEvent
	p.EmitStatus = func(v AppView) { statusEvents = append(statusEvents, v) }
	p.EmitAppsChanged = func(ev AppsChangedEvent) { appsChangedEvents = append(appsChangedEvents, ev) }

	// Reject invalid portMode (e.g. "fixed")
	rec := doControlReq(t, h, http.MethodPost, "/api/v1/apps", token, map[string]any{
		"name":     "agent2api",
		"folder":   appDir,
		"command":  "sleep 30",
		"portMode": "fixed",
		"port":     8080,
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid portMode code %d body %s", rec.Code, rec.Body.String())
	}

	// Reject unknown JSON fields so AI agents catch typos immediately
	rec = doControlReq(t, h, http.MethodPost, "/api/v1/apps", token, map[string]any{
		"name":          "agent2api",
		"folder":        appDir,
		"command":       "sleep 30",
		"idle_stop_min": 15,
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown field code %d body %s", rec.Code, rec.Body.String())
	}

	// Create valid app with manual port
	rec = doControlReq(t, h, http.MethodPost, "/api/v1/apps", token, map[string]any{
		"id":            "agent2api-local",
		"name":          "agent2api",
		"hostname":      "agent2api",
		"folder":        appDir,
		"command":       "sleep 30",
		"portMode":      "manual",
		"port":          18080,
		"wakeOnRequest": true,
		"idleStopMin":   15,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create code %d body %s", rec.Code, rec.Body.String())
	}
	var created AppView
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID != "agent2api-local" || created.Hostname != "agent2api" || created.Port == nil || *created.Port != 18080 {
		t.Fatalf("created %+v", created)
	}
	if len(statusEvents) == 0 {
		t.Fatal("expected app-status emit on create")
	}

	// Duplicate ID on POST -> 409
	rec = doControlReq(t, h, http.MethodPost, "/api/v1/apps", token, map[string]any{
		"id":      "agent2api-local",
		"name":    "dup",
		"folder":  appDir,
		"command": "sleep 30",
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("dup id code %d body %s", rec.Code, rec.Body.String())
	}

	// List with ?hostname= filter
	rec = doControlReq(t, h, http.MethodGet, "/api/v1/apps?hostname=agent2api.devportal.localhost", token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list code %d body %s", rec.Code, rec.Body.String())
	}
	var listResp struct {
		Apps []AppView `json:"apps"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listResp); err != nil {
		t.Fatal(err)
	}
	if len(listResp.Apps) != 1 || listResp.Apps[0].ID != "agent2api-local" {
		t.Fatalf("filtered list %+v", listResp.Apps)
	}

	// PATCH partial update (only idleStopMin & description)
	rec = doControlReq(t, h, http.MethodPatch, "/api/v1/apps/agent2api-local", token, map[string]any{
		"description": "Agent API wrapper",
		"idleStopMin": 30,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("patch code %d body %s", rec.Code, rec.Body.String())
	}
	var patched AppView
	if err := json.Unmarshal(rec.Body.Bytes(), &patched); err != nil {
		t.Fatal(err)
	}
	if patched.Description != "Agent API wrapper" || patched.IdleStopMin != 30 || patched.Port == nil || *patched.Port != 18080 || patched.Command != "sleep 30" {
		t.Fatalf("patched %+v", patched)
	}

	// PUT upsert a second app
	rec = doControlReq(t, h, http.MethodPut, "/api/v1/apps/second-app", token, map[string]any{
		"name":    "second",
		"folder":  appDir,
		"command": "sleep 30",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("put create code %d body %s", rec.Code, rec.Body.String())
	}

	// Reorder apps
	rec = doControlReq(t, h, http.MethodPut, "/api/v1/apps/order", token, map[string]any{
		"ids": []string{"second-app", "agent2api-local"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("reorder code %d body %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listResp); err != nil {
		t.Fatal(err)
	}
	if len(listResp.Apps) != 2 || listResp.Apps[0].ID != "second-app" {
		t.Fatalf("reordered %+v", listResp.Apps)
	}
	if len(appsChangedEvents) == 0 {
		t.Fatal("expected apps-changed emit on reorder")
	}

	// Delete app
	rec = doControlReq(t, h, http.MethodDelete, "/api/v1/apps/second-app", token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete code %d body %s", rec.Code, rec.Body.String())
	}
	if rec := doControlReq(t, h, http.MethodGet, "/api/v1/apps/second-app", token, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("deleted app should 404, got %d", rec.Code)
	}
}

func TestControlAPIAppLifecycleLogsAndForceDelete(t *testing.T) {
	if isWindows() {
		t.Skip("unix shell command test")
	}
	p, appDir, token := newControlAPITestPortal(t)
	h := p.controlAPIHandler()

	rec := doControlReq(t, h, http.MethodPost, "/api/v1/apps", token, map[string]any{
		"id":      "runner",
		"name":    "runner",
		"folder":  appDir,
		"command": "sh -c 'echo hello-from-api; sleep 30'",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create code %d body %s", rec.Code, rec.Body.String())
	}

	// Start with pinned=false
	rec = doControlReq(t, h, http.MethodPost, "/api/v1/apps/runner/start", token, map[string]any{"pinned": false})
	if rec.Code != http.StatusOK {
		t.Fatalf("start code %d body %s", rec.Code, rec.Body.String())
	}
	var started AppView
	if err := json.Unmarshal(rec.Body.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	if started.PID == nil || *started.PID == 0 || started.Pinned {
		t.Fatalf("started %+v", started)
	}
	t.Cleanup(func() { _, _ = p.Stop("runner") })

	// Pin via PUT /pinned
	rec = doControlReq(t, h, http.MethodPut, "/api/v1/apps/runner/pinned", token, map[string]any{"pinned": true})
	if rec.Code != http.StatusOK {
		t.Fatalf("pin code %d body %s", rec.Code, rec.Body.String())
	}
	var pinnedView AppView
	if err := json.Unmarshal(rec.Body.Bytes(), &pinnedView); err != nil {
		t.Fatal(err)
	}
	if !pinnedView.Pinned {
		t.Fatalf("expected pinned=true, got %+v", pinnedView)
	}

	// Wait briefly for stdout log line
	deadline := time.Now().Add(3 * time.Second)
	gotLog := false
	for time.Now().Before(deadline) {
		rec = doControlReq(t, h, http.MethodGet, "/api/v1/apps/runner/logs?stream=stdout&limit=10", token, nil)
		if rec.Code == http.StatusOK && strings.Contains(rec.Body.String(), "hello-from-api") {
			gotLog = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !gotLog {
		t.Fatalf("expected stdout log line, last body: %s", rec.Body.String())
	}

	// Delete without force -> 409
	rec = doControlReq(t, h, http.MethodDelete, "/api/v1/apps/runner", token, nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("delete running code %d body %s", rec.Code, rec.Body.String())
	}

	// Delete with ?force=true -> 200
	rec = doControlReq(t, h, http.MethodDelete, "/api/v1/apps/runner?force=true", token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("force delete code %d body %s", rec.Code, rec.Body.String())
	}
}

func TestControlAPICategoriesAndGateway(t *testing.T) {
	p, _, token := newControlAPITestPortal(t)
	h := p.controlAPIHandler()
	t.Cleanup(p.StopGateway)

	var catEvents []CategoriesChangedEvent
	p.EmitCategoriesChanged = func(ev CategoriesChangedEvent) { catEvents = append(catEvents, ev) }

	// Create category
	rec := doControlReq(t, h, http.MethodPost, "/api/v1/categories", token, map[string]any{
		"name":  "AI Tools",
		"color": "#3e63dd",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create category code %d body %s", rec.Code, rec.Body.String())
	}
	var cat CategoryEntry
	if err := json.Unmarshal(rec.Body.Bytes(), &cat); err != nil {
		t.Fatal(err)
	}
	if cat.Name != "AI Tools" || cat.Color != "#3e63dd" || cat.ID == "" {
		t.Fatalf("category %+v", cat)
	}

	// PATCH category
	rec = doControlReq(t, h, http.MethodPatch, "/api/v1/categories/"+cat.ID, token, map[string]any{
		"name": "Agents",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("patch category code %d body %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &cat); err != nil {
		t.Fatal(err)
	}
	if cat.Name != "Agents" || cat.Color != "#3e63dd" {
		t.Fatalf("patched category %+v", cat)
	}

	// Delete category
	rec = doControlReq(t, h, http.MethodDelete, "/api/v1/categories/"+cat.ID, token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete category code %d body %s", rec.Code, rec.Body.String())
	}
	if len(catEvents) < 3 {
		t.Fatalf("expected 3 category events, got %d", len(catEvents))
	}

	// Gateway port + start + stop
	hold, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	freePort := uint16(hold.Addr().(*net.TCPAddr).Port)
	_ = hold.Close()

	rec = doControlReq(t, h, http.MethodPut, "/api/v1/gateway/port", token, map[string]any{"port": freePort})
	if rec.Code != http.StatusOK {
		t.Fatalf("set gateway port code %d body %s", rec.Code, rec.Body.String())
	}
	rec = doControlReq(t, h, http.MethodPost, "/api/v1/gateway/start", token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("start gateway code %d body %s", rec.Code, rec.Body.String())
	}
	var gw GatewayStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &gw); err != nil {
		t.Fatal(err)
	}
	if !gw.Running {
		t.Fatalf("expected running gateway %+v", gw)
	}

	rec = doControlReq(t, h, http.MethodPost, "/api/v1/gateway/stop", token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("stop gateway code %d body %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &gw); err != nil {
		t.Fatal(err)
	}
	if gw.Running {
		t.Fatalf("expected stopped gateway %+v", gw)
	}
}

func TestSetControlAPIPortPersistsAndRestarts(t *testing.T) {
	p, _, _ := newControlAPITestPortal(t)

	hold, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	nextPort := uint16(hold.Addr().(*net.TCPAddr).Port)
	_ = hold.Close()

	if err := p.SetControlAPIPort(nextPort); err != nil {
		t.Fatal(err)
	}
	st := p.ControlAPIStatus()
	if !st.Running || st.PreferredPort != nextPort || st.Port != nextPort {
		t.Fatalf("unexpected status after port change: %+v", st)
	}
	loaded, err := loadConfig(p.inner.configPath)
	if err != nil {
		t.Fatal(err)
	}
	if preferredControlAPIPort(loaded) != nextPort {
		t.Fatalf("persisted port %+v want %d", loaded.ControlAPIPort, nextPort)
	}
}
