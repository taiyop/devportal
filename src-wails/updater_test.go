package main

import (
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

func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("", "  ", "token"); got != "token" {
		t.Fatalf("got %q", got)
	}
	if got := firstNonEmpty("", ""); got != "" {
		t.Fatalf("got %q", got)
	}
}
