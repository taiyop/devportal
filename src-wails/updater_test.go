package main

import (
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/updater"
	"github.com/wailsapp/wails/v3/pkg/updater/providers/github"
)

func TestCurrentVersionHasNoVPrefix(t *testing.T) {
	if strings.HasPrefix(currentVersion, "v") {
		t.Fatalf("currentVersion must not include a leading v: %q", currentVersion)
	}
	if currentVersion == "" {
		t.Fatal("currentVersion must not be empty")
	}
}

func TestZipFirstMatcherPrefersZipOverDmg(t *testing.T) {
	assets := []github.ReleaseAsset{
		{Name: "DevPortal-darwin-arm64.dmg"},
		{Name: "SHA256SUMS"},
		{Name: "DevPortal-darwin-arm64.zip"},
	}
	got := zipFirstMatcher(updater.CheckRequest{Platform: "darwin", Arch: "arm64"}, assets)
	if got != 2 {
		t.Fatalf("wanted zip at index 2, got %d", got)
	}
}

func TestZipFirstMatcherFallsBackWhenNoZip(t *testing.T) {
	assets := []github.ReleaseAsset{
		{Name: "notes.txt"},
		{Name: "DevPortal-darwin-arm64"},
	}
	got := zipFirstMatcher(updater.CheckRequest{Platform: "darwin", Arch: "arm64"}, assets)
	if got != 1 {
		t.Fatalf("wanted binary at index 1, got %d", got)
	}
}

func TestZipFirstMatcherSkipsWrongArch(t *testing.T) {
	assets := []github.ReleaseAsset{
		{Name: "DevPortal-darwin-amd64.zip"},
		{Name: "DevPortal-darwin-arm64.zip"},
	}
	got := zipFirstMatcher(updater.CheckRequest{Platform: "darwin", Arch: "arm64"}, assets)
	if got != 1 {
		t.Fatalf("wanted arm64 zip at index 1, got %d", got)
	}
}

func TestRunningVersionStripsPrefix(t *testing.T) {
	if got := runningVersion(); got != currentVersion {
		t.Fatalf("got %q want %q", got, currentVersion)
	}
}

func TestNormalizeAppVersion(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"0.1.2", "0.1.2"},
		{"v0.1.2", "0.1.2"},
		{"V0.1.2", "0.1.2"},
		{"  v0.1.2  ", "0.1.2"},
		{"v", "0.1.0"},
		{"", "0.1.0"},
		{"  ", "0.1.0"},
	}
	for _, c := range cases {
		if got := normalizeAppVersion(c.in); got != c.want {
			t.Errorf("normalizeAppVersion(%q)=%q want %q", c.in, got, c.want)
		}
	}
}

func TestUpdateInProgress(t *testing.T) {
	busy := []updater.State{
		updater.StateChecking,
		updater.StateDownloading,
		updater.StateVerifying,
		updater.StateInstalling,
		updater.StateReady,
	}
	for _, state := range busy {
		if !updateInProgress(state) {
			t.Errorf("updateInProgress(%s) = false, want true", state)
		}
	}
	idle := []updater.State{
		updater.StateIdle,
		updater.StateAvailable,
		updater.StateUpToDate,
		updater.StateError,
	}
	for _, state := range idle {
		if updateInProgress(state) {
			t.Errorf("updateInProgress(%s) = true, want false", state)
		}
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("", "  ", "token"); got != "token" {
		t.Fatalf("got %q", got)
	}
	if got := firstNonEmpty("", ""); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestNormalizeAndParseUpdateChannel(t *testing.T) {
	if got := normalizeUpdateChannel(""); got != UpdateChannelStable {
		t.Fatalf("empty channel normalized to %q, want stable", got)
	}
	if got := normalizeUpdateChannel("BETA"); got != UpdateChannelBeta {
		t.Fatalf("BETA normalized to %q, want beta", got)
	}
	if got := normalizeUpdateChannel("unknown"); got != UpdateChannelStable {
		t.Fatalf("unknown normalized to %q, want stable", got)
	}

	for _, tc := range []struct {
		in   string
		want UpdateChannel
	}{
		{"", UpdateChannelStable},
		{"stable", UpdateChannelStable},
		{"  STABLE ", UpdateChannelStable},
		{"beta", UpdateChannelBeta},
		{" Beta ", UpdateChannelBeta},
	} {
		got, err := parseUpdateChannel(tc.in)
		if err != nil {
			t.Fatalf("parseUpdateChannel(%q) unexpected error: %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("parseUpdateChannel(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}

	if _, err := parseUpdateChannel("nightly"); err == nil {
		t.Fatal("expected error for unsupported channel")
	}
}

type stubUpdateProvider struct {
	name          string
	checkRelease  *updater.Release
	checkCalls    int
	downloadCalls int
}

func (s *stubUpdateProvider) Name() string { return s.name }

func (s *stubUpdateProvider) Check(_ context.Context, _ updater.CheckRequest) (*updater.Release, error) {
	s.checkCalls++
	return s.checkRelease, nil
}

func (s *stubUpdateProvider) Download(_ context.Context, _ *updater.Release, _ io.Writer, _ func(written, total int64)) error {
	s.downloadCalls++
	return nil
}

func TestChannelAwareProviderRoutesByActiveChannel(t *testing.T) {
	stable := &stubUpdateProvider{
		name:         "github-stable",
		checkRelease: &updater.Release{Version: "0.1.9", Channel: "stable"},
	}
	beta := &stubUpdateProvider{
		name:         "github-beta",
		checkRelease: &updater.Release{Version: "0.2.0-beta.1", Channel: "prerelease"},
	}
	current := UpdateChannelStable
	p := &channelAwareProvider{
		stable: stable,
		beta:   beta,
		channel: func() UpdateChannel {
			return current
		},
	}

	rel, err := p.Check(context.Background(), updater.CheckRequest{CurrentVersion: "0.1.8"})
	if err != nil {
		t.Fatalf("stable check failed: %v", err)
	}
	if rel.Version != "0.1.9" || stable.checkCalls != 1 || beta.checkCalls != 0 {
		t.Fatalf("expected stable provider, got rel=%+v stableCalls=%d betaCalls=%d", rel, stable.checkCalls, beta.checkCalls)
	}

	current = UpdateChannelBeta
	rel, err = p.Check(context.Background(), updater.CheckRequest{CurrentVersion: "0.1.8"})
	if err != nil {
		t.Fatalf("beta check failed: %v", err)
	}
	if rel.Version != "0.2.0-beta.1" || stable.checkCalls != 1 || beta.checkCalls != 1 {
		t.Fatalf("expected beta provider, got rel=%+v stableCalls=%d betaCalls=%d", rel, stable.checkCalls, beta.checkCalls)
	}

	// Downloading a prerelease always routes to beta even if channel switched back.
	current = UpdateChannelStable
	if err := p.Download(context.Background(), rel, io.Discard, nil); err != nil {
		t.Fatalf("beta download failed: %v", err)
	}
	if beta.downloadCalls != 1 || stable.downloadCalls != 0 {
		t.Fatalf("expected beta download, got stable=%d beta=%d", stable.downloadCalls, beta.downloadCalls)
	}
}

func TestPortalSetUpdateChannelPersists(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "apps.yml")
	portal := &Portal{
		inner: Inner{
			configPath: cfgPath,
			file:       ConfigFile{Categories: []CategoryEntry{}, Apps: []AppEntry{}},
			runtime:    map[string]*Runtime{},
			logs:       map[string][]LogEvent{},
			errors:     map[string]string{},
			favicons:   map[string]string{},
		},
	}

	if got := portal.UpdateChannel(); got != UpdateChannelStable {
		t.Fatalf("initial channel = %q, want stable", got)
	}
	got, err := portal.SetUpdateChannel("beta")
	if err != nil {
		t.Fatalf("SetUpdateChannel(beta) failed: %v", err)
	}
	if got != UpdateChannelBeta || portal.UpdateChannel() != UpdateChannelBeta {
		t.Fatalf("channel after beta = %q / %q", got, portal.UpdateChannel())
	}

	loaded, err := loadConfig(cfgPath)
	if err != nil {
		t.Fatalf("loadConfig failed: %v", err)
	}
	if loaded.UpdateChannel != UpdateChannelBeta {
		t.Fatalf("persisted channel = %q, want beta", loaded.UpdateChannel)
	}

	if _, err := portal.SetUpdateChannel("invalid"); err == nil {
		t.Fatal("expected error for invalid channel")
	}
	if portal.UpdateChannel() != UpdateChannelBeta {
		t.Fatalf("channel changed after invalid input: %q", portal.UpdateChannel())
	}
}
