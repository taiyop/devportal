package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestGatewayUnknownHost(t *testing.T) {
	p := &Portal{inner: Inner{
		file:    ConfigFile{Apps: []AppEntry{{ID: "1", Hostname: "notes", WakeOnRequest: true}}},
		runtime: map[string]*Runtime{},
		errors:  map[string]string{},
	}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "missing.devportal.localhost"
	p.handleGateway(rec, req, false)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code %d body %s", rec.Code, rec.Body.String())
	}
}

func TestGatewayWakesStoppedApp(t *testing.T) {
	dir := t.TempDir()
	p := &Portal{inner: Inner{
		configPath:  filepath.Join(dir, "apps.yml"),
		runningPath: filepath.Join(dir, "running.yml"),
		file: ConfigFile{Apps: []AppEntry{{
			ID:            "1",
			Name:          "notes",
			Hostname:      "notes",
			Folder:        filepath.Join(dir, "missing"),
			Command:       "true",
			WakeOnRequest: false,
		}}},
		runtime: map[string]*Runtime{},
		errors:  map[string]string{},
		logs:    map[string][]LogEvent{},
	}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "notes.devportal.localhost"
	p.handleGateway(rec, req, false)
	if strings.Contains(rec.Body.String(), "アプリは停止中です") {
		t.Fatal("stopped apps should wake on domain access")
	}
	if rec.Code != http.StatusServiceUnavailable && rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("code %d body %s", rec.Code, rec.Body.String())
	}
}

func TestGatewayProxiesRunningApp(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != "notes.devportal.localhost" {
			t.Errorf("backend host %q", r.Host)
		}
		w.Header().Set("X-From", "backend")
		_, _ = io.WriteString(w, "ok")
	}))
	defer backend.Close()
	u, err := url.Parse(backend.URL)
	if err != nil {
		t.Fatal(err)
	}
	port64, err := strconv.ParseUint(u.Port(), 10, 16)
	if err != nil {
		t.Fatal(err)
	}
	port := uint16(port64)
	p := &Portal{inner: Inner{
		file: ConfigFile{Apps: []AppEntry{{ID: "1", Hostname: "notes", WakeOnRequest: true, Port: &port}}},
		runtime: map[string]*Runtime{
			"1": {port: port, ready: true},
		},
		errors: map[string]string{},
	}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/hello", nil)
	req.Host = "notes.devportal.localhost"
	p.handleGateway(rec, req, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("code %d body %s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "ok" {
		t.Fatalf("body %q", rec.Body.String())
	}
	if rec.Header().Get("X-From") != "backend" {
		t.Fatalf("missing backend header")
	}
}

func TestGatewayProxiesIPv6Loopback(t *testing.T) {
	ln, err := net.Listen("tcp", net.JoinHostPort(loopbackV6, "0"))
	if err != nil {
		t.Skip("IPv6 loopback is not available")
	}
	backend := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != "notes.devportal.localhost" {
			t.Errorf("backend host %q", r.Host)
		}
		_, _ = io.WriteString(w, "v6ok")
	}))
	backend.Listener = ln
	backend.Start()
	defer backend.Close()
	u, err := url.Parse(backend.URL)
	if err != nil {
		t.Fatal(err)
	}
	port64, err := strconv.ParseUint(u.Port(), 10, 16)
	if err != nil {
		t.Fatal(err)
	}
	port := uint16(port64)
	p := &Portal{inner: Inner{
		file: ConfigFile{Apps: []AppEntry{{ID: "1", Hostname: "notes", WakeOnRequest: true, Port: &port}}},
		runtime: map[string]*Runtime{
			"1": {port: port, ready: true, bindHost: loopbackV6},
		},
		errors: map[string]string{},
	}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/hello", nil)
	req.Host = "notes.devportal.localhost"
	p.handleGateway(rec, req, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("code %d body %s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "v6ok" {
		t.Fatalf("body %q", rec.Body.String())
	}
}

func TestGatewayDoesNotListenUntilStarted(t *testing.T) {
	p := newTestPortal()
	st := p.GatewayStatus()
	if st.Running {
		t.Fatal("gateway should not run until started")
	}
	if st.Error != "" {
		t.Fatalf("unexpected error %q", st.Error)
	}
}

func TestGatewayStartStop(t *testing.T) {
	p := newTestPortal()
	addr := freeLocalAddr(t)
	if err := p.listenAndServeGateway(addr); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.StopGateway)

	st := p.GatewayStatus()
	if !st.Running {
		t.Fatalf("expected running, status %+v", st)
	}
	if st.Error != "" {
		t.Fatalf("unexpected error %q", st.Error)
	}

	client := &http.Client{Timeout: 2 * time.Second}
	var resp *http.Response
	deadline := time.Now().Add(2 * time.Second)
	for {
		var err error
		resp, err = client.Get(st.Addr + "/")
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal(err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("code %d", resp.StatusCode)
	}

	if err := p.listenAndServeGateway(addr); err != nil {
		t.Fatalf("second start: %v", err)
	}

	p.StopGateway()
	st = p.GatewayStatus()
	if st.Running {
		t.Fatal("expected stopped")
	}

	deadline = time.Now().Add(2 * time.Second)
	for {
		ln, err := net.Listen("tcp", addr)
		if err == nil {
			_ = ln.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("port still in use after stop: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestGatewayStartEmitsStatus(t *testing.T) {
	p := newTestPortal()
	got := make(chan GatewayStatus, 2)
	p.EmitGateway = func(status GatewayStatus) {
		got <- status
	}
	addr := freeLocalAddr(t)
	if err := p.listenAndServeGateway(addr); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.StopGateway)

	select {
	case st := <-got:
		if !st.Running {
			t.Fatalf("emit %+v", st)
		}
	case <-time.After(time.Second):
		t.Fatal("no start event")
	}

	p.StopGateway()
	select {
	case st := <-got:
		if st.Running {
			t.Fatalf("stop emit %+v", st)
		}
	case <-time.After(time.Second):
		t.Fatal("no stop event")
	}
}

func TestGatewayFallsBackWhenFirstPortBusy(t *testing.T) {
	hold, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer hold.Close()
	busy := hold.Addr().String()
	free := freeLocalAddr(t)
	p := newTestPortal()
	if err := p.listenFirstAvailable([]string{busy, free}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.StopGateway)
	st := p.GatewayStatus()
	if !st.Running {
		t.Fatalf("status %+v", st)
	}
	_, want, err := net.SplitHostPort(free)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprintf("%d", st.Port) != want {
		t.Fatalf("port %d want %s status %+v", st.Port, want, st)
	}
}

func TestStartGatewaySkipsPort80WithoutFailing(t *testing.T) {
	p := newTestPortal()
	if err := p.StartGateway(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.StopGateway)
	st := p.GatewayStatus()
	if !st.Running || st.Error != "" {
		t.Fatalf("status %+v", st)
	}
	if st.Port == 0 {
		t.Fatal("missing port")
	}
	if st.Port == 80 && st.Note != "" {
		t.Fatalf("unexpected note on port 80: %q", st.Note)
	}
	if st.Port != 80 && (st.Note == "" || !strings.Contains(st.Note, fmt.Sprintf("%d", st.Port))) {
		t.Fatalf("expected fallback note, got %+v", st)
	}
	if st.Port != 80 {
		return
	}
	client := &http.Client{Timeout: 2 * time.Second}
	req, err := http.NewRequest(http.MethodGet, "http://notes.devportal.localhost/", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("hostname via :80 code %d", resp.StatusCode)
	}
}

func TestPreferredGatewayPortIs80(t *testing.T) {
	addrs := listenAddrsFor(0)
	if addrs[0] != ":80" {
		t.Fatalf("preferred %q", addrs[0])
	}
	if addrs[len(addrs)-1] != ":0" {
		t.Fatalf("auto fallback %q", addrs[len(addrs)-1])
	}
	if listenPortOf(":80") != 80 || listenPortOf(":7341") != 7341 {
		t.Fatal("port parse")
	}
}

func TestListenAddrsForUsesConfiguredPortFirst(t *testing.T) {
	addrs := listenAddrsFor(8080)
	if addrs[0] != ":8080" {
		t.Fatalf("preferred %q", addrs[0])
	}
	seen80 := false
	for _, addr := range addrs[1:] {
		if addr == ":8080" {
			t.Fatalf("duplicate %v", addrs)
		}
		if addr == ":80" {
			seen80 = true
		}
	}
	if !seen80 {
		t.Fatalf("expected :80 fallback, got %v", addrs)
	}
}

func TestSetGatewayPortPersistsAndListens(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "apps.yml")
	p := newTestPortal()
	p.inner.configPath = path
	if err := saveConfig(path, ConfigFile{}); err != nil {
		t.Fatal(err)
	}

	hold, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := uint16(hold.Addr().(*net.TCPAddr).Port)
	if err := hold.Close(); err != nil {
		t.Fatal(err)
	}

	if err := p.SetGatewayPort(port); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if preferredGatewayPort(loaded) != port {
		t.Fatalf("saved %+v", loaded.GatewayPort)
	}

	if err := p.StartGateway(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.StopGateway)
	st := p.GatewayStatus()
	if !st.Running || st.PreferredPort != port {
		t.Fatalf("status %+v", st)
	}
	if st.Port != port && (st.Note == "" || !strings.Contains(st.Note, fmt.Sprintf("%d", port))) {
		t.Fatalf("expected fallback note, got %+v", st)
	}
}

func TestSetGatewayPortEmptyIs80(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "apps.yml")
	p := newTestPortal()
	p.inner.configPath = path
	if err := saveConfig(path, ConfigFile{}); err != nil {
		t.Fatal(err)
	}
	if err := p.SetGatewayPort(0); err != nil {
		t.Fatal(err)
	}
	st := p.GatewayStatus()
	if st.PreferredPort != 80 {
		t.Fatalf("status %+v", st)
	}
}

func TestGatewayStatusPreferredPortWhenStopped(t *testing.T) {
	p := newTestPortal()
	port := uint16(8888)
	p.inner.file.GatewayPort = &port
	st := p.GatewayStatus()
	if st.Running {
		t.Fatal("expected stopped")
	}
	if st.PreferredPort != 8888 || st.Port != 8888 {
		t.Fatalf("status %+v", st)
	}
}

func TestGatewayAcceptsIPv4AndIPv6(t *testing.T) {
	p := newTestPortal()
	if err := p.listenAndServeGateway(":0"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.StopGateway)
	st := p.GatewayStatus()
	if st.AllowLAN {
		t.Fatal("LAN access is enabled by default")
	}
	if !st.Running {
		t.Fatalf("status %+v", st)
	}
	client := &http.Client{Timeout: 2 * time.Second}
	for _, raw := range []string{
		fmt.Sprintf("http://127.0.0.1:%d/", st.Port),
		fmt.Sprintf("http://[%s]:%d/", net.IPv6loopback.String(), st.Port),
	} {
		req, err := http.NewRequest(http.MethodGet, raw, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Host = "notes.devportal.localhost"
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("%s code %d", raw, resp.StatusCode)
		}
	}
}

func TestGatewayLANSettingPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "apps.yml")
	if err := os.WriteFile(path, []byte("apps: []\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	p := newTestPortal()
	p.inner.configPath = path
	p.inner.file = file
	if p.GatewayStatus().AllowLAN {
		t.Fatal("existing config should default to loopback")
	}
	for _, allow := range []bool{true, false} {
		if err := p.SetGatewayAllowLAN(allow); err != nil {
			t.Fatal(err)
		}
		loaded, err := loadConfig(path)
		if err != nil {
			t.Fatal(err)
		}
		if status := p.GatewayStatus(); status.AllowLAN != allow || status.Running || loaded.GatewayAllowLAN != allow {
			t.Fatalf("allow=%t, status=%+v, saved=%t", allow, status, loaded.GatewayAllowLAN)
		}
	}
}

func TestGatewayLANListenerAllowsOnlyItsPrivateSubnet(t *testing.T) {
	for _, test := range []struct {
		subnet, allowed, denied string
	}{
		{"192.168.50.0/24", "192.168.50.10", "192.168.51.10"},
		{"fd00:50::/64", "fd00:50::10", "fd00:51::10"},
		{"10.0.0.0/4", "10.1.0.10", "12.1.0.10"},
	} {
		_, network, err := net.ParseCIDR(test.subnet)
		if err != nil {
			t.Fatal(err)
		}
		listener := gatewayLANListener{subnet: network}
		if !listener.allows(net.ParseIP(test.allowed)) || listener.allows(net.ParseIP(test.denied)) {
			t.Fatalf("incorrect allow/deny for %s", test.subnet)
		}
	}
}

func TestGatewayToggleLANChangesLiveListeners(t *testing.T) {
	addresses, err := gatewayLANAddresses()
	if err != nil {
		t.Fatal(err)
	}
	if len(addresses) == 0 {
		t.Skip("no LAN interface")
	}
	p := newTestPortal()
	p.inner.configPath = filepath.Join(t.TempDir(), "apps.yml")
	port := uint16(listenPortOf(freeLocalAddr(t)))
	p.inner.file.GatewayPort = &port
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer backend.Close()
	backendPort := tcpListenPort(backend.Listener)
	p.inner.file.Apps = []AppEntry{{ID: "notes", Hostname: "notes", Port: &backendPort}}
	p.inner.runtime["notes"] = &Runtime{port: backendPort, ready: true}
	if err := p.StartGateway(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.StopGateway)

	client := &http.Client{Timeout: 2 * time.Second}
	defer client.CloseIdleConnections()
	for _, allow := range []bool{false, true, false} {
		if err := p.SetGatewayAllowLAN(allow); err != nil {
			t.Fatal(err)
		}
		status := p.GatewayStatus()
		if !status.Running || status.AllowLAN != allow {
			t.Fatalf("allow=%t, status=%+v", allow, status)
		}
		lanAddr := net.JoinHostPort(addresses[0].host, itoaPort(status.Port))
		if !allow {
			if conn, err := net.DialTimeout("tcp", lanAddr, time.Second); err == nil {
				_ = conn.Close()
				t.Fatalf("LAN connection to %s while disabled", lanAddr)
			}
			continue
		}
		target := url.URL{Scheme: "http", Host: lanAddr, Path: "/"}
		req, err := http.NewRequest(http.MethodGet, target.String(), nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Host = "notes.devportal.home.arpa"
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent {
			t.Fatalf("LAN hostname routing: status %d", resp.StatusCode)
		}
	}
}

func TestPrepareHostnameURLStartsGateway(t *testing.T) {
	p := newTestPortal()
	t.Cleanup(p.StopGateway)
	got, err := p.PrepareHostnameURL("http://notes.devportal.localhost/path")
	if err != nil {
		t.Fatal(err)
	}
	st := p.GatewayStatus()
	if !st.Running {
		t.Fatal("gateway should start")
	}
	u, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	if u.Hostname() != "notes.devportal.localhost" {
		t.Fatalf("host %q", u.Hostname())
	}
	if st.Port != 80 && u.Port() != fmt.Sprintf("%d", st.Port) {
		t.Fatalf("url %q status %+v", got, st)
	}
}

func newTestPortal() *Portal {
	return &Portal{inner: Inner{
		file:    ConfigFile{},
		runtime: map[string]*Runtime{},
		errors:  map[string]string{},
	}}
}

func (p *Portal) listenAndServeGateway(addr string) error {
	return p.listenFirstAvailable([]string{addr})
}

func (p *Portal) listenFirstAvailable(addrs []string) error {
	p.gatewayMu.Lock()
	err := p.startGatewayLocked(addrs, false)
	p.gatewayMu.Unlock()
	p.emitGateway()
	p.emitAllViews()
	return err
}

func freeLocalAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}
	return addr
}
