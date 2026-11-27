package main

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"tailscale.com/ipn/ipnstate"
	"tailscale.com/types/key"
	"tailscale.com/types/views"
)

const tailnetTestHost = "devportal.test-tailnet.ts.net"

func tailnetTestLookup(_ context.Context, addr string) (string, error) {
	switch addr {
	case "100.64.0.1:50001":
		return "phone", nil
	case "100.64.0.2:50002":
		return "tablet", nil
	case "100.64.0.3:50003":
		return "guest", nil
	default:
		return "", errors.New("whois failed")
	}
}

func tailnetTestConfig() ConfigFile {
	return ConfigFile{
		Categories: []CategoryEntry{{ID: "tools", Name: "Tools"}},
		Apps: []AppEntry{
			{ID: "public", Name: "<Notes>", CategoryID: "tools", Command: "secret-command", Folder: "/private/project"},
			{ID: "phone-only", Name: "Phone app", CategoryID: "tools", BoardPinned: true},
			{ID: "local", Name: "Local app"},
		},
		Tailnet: TailnetConfig{TailnetSettings: TailnetSettings{
			Enabled:   true,
			DeviceIDs: []string{"phone", "tablet"},
			Apps: map[string]TailnetAppSettings{
				"public":     {Enabled: true, Port: 20000},
				"phone-only": {Enabled: true, Port: 20001, RestrictDevices: true, DeviceIDs: []string{"phone"}},
			},
		}},
	}
}

func TestTailnetPortalShowsOnlyAppsAvailableToPeer(t *testing.T) {
	p := &Portal{inner: Inner{file: tailnetTestConfig()}}
	portal := p.tailnetPortal(tailnetTestHost, tailnetTestLookup)

	for _, tt := range []struct {
		name, addr string
		want       int
		phoneOnly  bool
	}{
		{"allowed phone", "100.64.0.1:50001", http.StatusOK, true},
		{"allowed tablet", "100.64.0.2:50002", http.StatusOK, false},
		{"unlisted peer", "100.64.0.3:50003", http.StatusForbidden, false},
		{"unresolved peer", "100.64.0.4:50004", http.StatusForbidden, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "https://"+tailnetTestHost+"/", nil)
			request.RemoteAddr = tt.addr
			request.Header.Set("X-Forwarded-For", "100.64.0.1:50001")
			rec := httptest.NewRecorder()
			portal.ServeHTTP(rec, request)
			if rec.Code != tt.want {
				t.Fatalf("status %d, want %d", rec.Code, tt.want)
			}
			body := rec.Body.String()
			for _, hidden := range []string{"secret-command", "/private/project", "Local app"} {
				if strings.Contains(body, hidden) {
					t.Errorf("portal revealed %q", hidden)
				}
			}
			if tt.want == http.StatusOK {
				for _, visible := range []string{"Tools", "&lt;Notes&gt;", "https://" + tailnetTestHost + ":20000/"} {
					if !strings.Contains(body, visible) {
						t.Errorf("portal is missing %q", visible)
					}
				}
			}
			if visible := strings.Contains(body, "Phone app"); visible != tt.phoneOnly {
				t.Fatalf("unexpected per-app visibility: phone app visible = %v", visible)
			}
		})
	}
}

func TestTailnetDirectAppLinkRequiresAppPermission(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != r.Context().Value(http.LocalAddrContextKey).(net.Addr).String() ||
			r.Header.Get("X-Forwarded-Host") != tailnetTestHost+":20001" ||
			r.Header.Get("X-Forwarded-Proto") != "https" ||
			r.Header.Get("X-Forwarded-For") != "100.64.0.1" ||
			r.Header.Get("Tailscale-User-Login") != "" {
			t.Errorf("unexpected forwarded host %q or headers %v", r.Host, r.Header)
		}
		_, _ = io.WriteString(w, r.URL.RequestURI())
	}))
	defer backend.Close()
	backendURL, err := url.Parse(backend.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.ParseUint(backendURL.Port(), 10, 16)
	if err != nil {
		t.Fatal(err)
	}
	p := &Portal{inner: Inner{
		file:    tailnetTestConfig(),
		runtime: map[string]*Runtime{"phone-only": {ready: true, port: uint16(port)}},
	}}
	app := p.tailnetApp(tailnetTestHost, 20001, "phone-only", tailnetTestLookup)
	requestURL := "https://" + tailnetTestHost + ":20001/nested/path?check=1"

	request := httptest.NewRequest(http.MethodGet, requestURL, nil)
	request.RemoteAddr = "100.64.0.1:50001"
	request.Header.Set("X-Forwarded-For", "100.64.0.2")
	request.Header.Set("Tailscale-User-Login", "fake")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, request)
	if rec.Code != http.StatusOK || rec.Body.String() != "/nested/path?check=1" {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}

	request.Header.Set("Sec-Fetch-Site", "same-site")
	request.Header.Set("Sec-Fetch-Mode", "no-cors")
	rec = httptest.NewRecorder()
	app.ServeHTTP(rec, request)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("sibling origin subresource request returned status %d", rec.Code)
	}
	request.Header.Set("Sec-Fetch-Mode", "navigate")
	request.Header.Set("Sec-Fetch-Dest", "document")
	rec = httptest.NewRecorder()
	app.ServeHTTP(rec, request)
	if rec.Code != http.StatusOK {
		t.Fatalf("portal app navigation returned status %d", rec.Code)
	}

	request.RemoteAddr = "100.64.0.2:50002"
	rec = httptest.NewRecorder()
	app.ServeHTTP(rec, request)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("tablet direct access returned status %d", rec.Code)
	}

	settings := p.inner.file.Tailnet.Apps["phone-only"]
	settings.Enabled = false
	p.inner.file.Tailnet.Apps["phone-only"] = settings
	request.RemoteAddr = "100.64.0.1:50001"
	rec = httptest.NewRecorder()
	app.ServeHTTP(rec, request)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("unpublished direct access returned status %d", rec.Code)
	}
}

func TestTailnetPortalAllowsOnlySameOriginStartStop(t *testing.T) {
	rt := &Runtime{}
	p := &Portal{inner: Inner{file: tailnetTestConfig(), runtime: map[string]*Runtime{"public": rt}}}
	portal := p.tailnetPortal(tailnetTestHost, tailnetTestLookup)
	origin := "https://" + tailnetTestHost

	for _, tt := range []struct {
		method, path, origin string
		want                 int
	}{
		{"GET", "/apps/public/start", "", http.StatusMethodNotAllowed},
		{"POST", "/apps/public/start", "", http.StatusForbidden},
		{"POST", "/apps/public/start", origin + ":20000", http.StatusForbidden},
		{"POST", "/apps/phone-only/start", origin, http.StatusNotFound},
		{"POST", "/apps/local/start", origin, http.StatusNotFound},
		{"POST", "/apps/public/update", origin, http.StatusNotFound},
		{"GET", "/v1/apps", "", http.StatusNotFound},
		{"POST", "/apps/public/start", origin, http.StatusSeeOther},
	} {
		request := httptest.NewRequest(tt.method, origin+tt.path, nil)
		request.RemoteAddr = "100.64.0.2:50002"
		request.Header.Set("Origin", tt.origin)
		rec := httptest.NewRecorder()
		portal.ServeHTTP(rec, request)
		if rec.Code != tt.want {
			t.Fatalf("%s %s (origin %q) returned %d, want %d", tt.method, tt.path, tt.origin, rec.Code, tt.want)
		}
		if rt.pinned != (tt.want == http.StatusSeeOther) {
			t.Fatalf("pin changed after %s %s returned %d", tt.method, tt.path, rec.Code)
		}
	}
}

func TestTailnetSavePersistsPorts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "apps.yml")
	file := tailnetTestConfig()
	file.Tailnet.Enabled = false
	file.Tailnet.Apps = nil
	p := &Portal{inner: Inner{configPath: path, file: file}}
	input := TailnetSettings{
		DeviceIDs: []string{"phone", "tablet", " phone "},
		Apps: map[string]TailnetAppSettings{
			"phone-only": {Enabled: true, Port: 443, RestrictDevices: true, DeviceIDs: []string{"phone"}},
		},
	}
	updated, err := p.setTailnetSettings(input)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(updated.DeviceIDs, []string{"phone", "tablet"}) || updated.Apps["phone-only"].Port != tailnetFirstAppPort {
		t.Fatalf("unexpected settings %#v", updated)
	}

	app := updated.Apps["phone-only"]
	app.Enabled = false
	updated.Apps = map[string]TailnetAppSettings{"phone-only": app}
	if _, err := p.setTailnetSettings(updated); err != nil {
		t.Fatal(err)
	}
	app.Enabled = true
	updated.Apps = map[string]TailnetAppSettings{"phone-only": app}
	if _, err := p.setTailnetSettings(updated); err != nil {
		t.Fatal(err)
	}
	reloaded, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	got := reloaded.Tailnet.Apps["phone-only"]
	if !got.Enabled || !got.RestrictDevices || got.Port != tailnetFirstAppPort || !slices.Equal(got.DeviceIDs, []string{"phone"}) {
		t.Fatalf("unexpected reloaded app policy %#v", got)
	}
}

func TestTailnetRevocationCancelsAuthorizedRequest(t *testing.T) {
	p := &Portal{inner: Inner{
		configPath: filepath.Join(t.TempDir(), "apps.yml"),
		file:       tailnetTestConfig(),
	}}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	entered := make(chan struct{})
	finished := make(chan struct{})
	app := p.authorizeTailnet(tailnetTestHost+":20000", 20000, "public", tailnetTestLookup, func(_ http.ResponseWriter, r *http.Request) {
		close(entered)
		<-r.Context().Done()
	})
	request := httptest.NewRequestWithContext(ctx, http.MethodGet, "https://"+tailnetTestHost+":20000/", nil)
	request.RemoteAddr = "100.64.0.1:50001"
	go func() {
		defer close(finished)
		app.ServeHTTP(httptest.NewRecorder(), request)
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("request never reached the authorized handler")
	}
	if _, err := p.setTailnetSettings(TailnetSettings{}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-finished:
	case <-ctx.Done():
		t.Fatal("request continued after access was revoked")
	}
}

func TestNormalizeTailnetTag(t *testing.T) {
	for _, tt := range []struct {
		in   string
		want string
	}{
		{"", defaultTailnetTag},
		{"   ", defaultTailnetTag},
		{"devportal-work", "tag:devportal-work"},
		{"tag:devportal", "tag:devportal"},
		{"TAG:devportal-2", "tag:devportal-2"},
		{"  tag:MacStudio-1  ", "tag:MacStudio-1"},
	} {
		got, err := normalizeTailnetTag(tt.in)
		if err != nil {
			t.Fatalf("normalizeTailnetTag(%q) error: %v", tt.in, err)
		}
		if got != tt.want {
			t.Fatalf("normalizeTailnetTag(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}

	for _, invalid := range []string{"tag:", "123", "tag:123", "tag:foo_bar", "tag:a b", "tag:日本語"} {
		if _, err := normalizeTailnetTag(invalid); err == nil {
			t.Fatalf("normalizeTailnetTag(%q) succeeded, want error", invalid)
		}
	}
}

func TestTailnetSaveNormalizesAndPersistsTag(t *testing.T) {
	path := filepath.Join(t.TempDir(), "apps.yml")
	p := &Portal{inner: Inner{configPath: path, file: tailnetTestConfig()}}
	p.inner.file.Tailnet.Enabled = false

	if got := p.tailnetSettings().Tag; got != defaultTailnetTag {
		t.Fatalf("default tag = %q, want %q", got, defaultTailnetTag)
	}

	if _, err := p.setTailnetSettings(TailnetSettings{Tag: "tag:invalid_tag"}); err == nil {
		t.Fatal("expected error for invalid tag")
	}

	updated, err := p.setTailnetSettings(TailnetSettings{Tag: "devportal-mini"})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Tag != "tag:devportal-mini" {
		t.Fatalf("updated tag = %q, want %q", updated.Tag, "tag:devportal-mini")
	}

	reloaded, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Tailnet.Tag != "tag:devportal-mini" {
		t.Fatalf("reloaded tag = %q, want %q", reloaded.Tailnet.Tag, "tag:devportal-mini")
	}
}

func TestTailnetStatusFromIPNRequiresConfiguredTag(t *testing.T) {
	peerKey := key.NewNode().Public()
	baseStatus := func(tags []string, authURL string) *ipnstate.Status {
		var tagView *views.Slice[string]
		if len(tags) > 0 {
			s := views.SliceOf(tags)
			tagView = &s
		}
		return &ipnstate.Status{
			BackendState: "Running",
			AuthURL:      authURL,
			CertDomains:  []string{tailnetTestHost},
			CurrentTailnet: &ipnstate.TailnetStatus{
				MagicDNSSuffix:  "test-tailnet.ts.net",
				MagicDNSEnabled: true,
			},
			Self: &ipnstate.PeerStatus{
				DNSName: tailnetTestHost + ".",
				Tags:    tagView,
			},
			Peer: map[key.NodePublic]*ipnstate.PeerStatus{
				peerKey: {
					ID:           "phone",
					HostName:     "phone",
					DNSName:      "phone.test-tailnet.ts.net.",
					InNetworkMap: true,
					Online:       true,
				},
			},
		}
	}

	t.Run("untagged node requires reauth", func(t *testing.T) {
		st, host, suffix, needsReauth := tailnetStatusFromIPN(baseStatus(nil, "https://login.tailscale.com/a/1"), "tag:devportal")
		if host != "" || suffix != "" || !needsReauth {
			t.Fatalf("got host=%q suffix=%q needsReauth=%v", host, suffix, needsReauth)
		}
		if st.LoginURL != "https://login.tailscale.com/a/1" || st.Error == "" || len(st.Devices) != 1 {
			t.Fatalf("unexpected status %#v", st)
		}
	})

	t.Run("mismatched tag requires reauth", func(t *testing.T) {
		st, host, _, needsReauth := tailnetStatusFromIPN(baseStatus([]string{"tag:other"}, ""), "tag:devportal")
		if host != "" || !needsReauth {
			t.Fatalf("got host=%q needsReauth=%v", host, needsReauth)
		}
		if !slices.Equal(st.Tags, []string{"tag:other"}) || st.Error == "" {
			t.Fatalf("unexpected status %#v", st)
		}
	})

	t.Run("matching tag clears stale auth url and serves", func(t *testing.T) {
		st, host, suffix, needsReauth := tailnetStatusFromIPN(baseStatus([]string{"tag:devportal", "tag:extra"}, "https://login.tailscale.com/stale"), "tag:DevPortal")
		if host != tailnetTestHost || suffix != "test-tailnet.ts.net" || needsReauth {
			t.Fatalf("got host=%q suffix=%q needsReauth=%v", host, suffix, needsReauth)
		}
		if st.LoginURL != "" || st.Error != "" || !slices.Equal(st.Tags, []string{"tag:devportal", "tag:extra"}) {
			t.Fatalf("unexpected status %#v", st)
		}
	})
}
