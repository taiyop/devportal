package main

import "testing"

func TestNormalizeHostname(t *testing.T) {
	got, err := normalizeHostname(" Notes.DevPortal.Localhost ")
	if err != nil {
		t.Fatal(err)
	}
	if got != "notes" {
		t.Fatalf("got %q", got)
	}
	got, err = normalizeHostname("http://wiki.devportal.localhost:80/")
	if err != nil {
		t.Fatal(err)
	}
	if got != "wiki" {
		t.Fatalf("got %q", got)
	}
	if _, err := normalizeHostname("bad_name"); err == nil {
		t.Fatal("expected invalid")
	}
	if _, err := normalizeHostname("a.b.devportal.localhost"); err == nil {
		t.Fatal("expected extra label rejected")
	}
}

func TestHostLabelFromRequest(t *testing.T) {
	if got := hostLabelFromRequest("notes.devportal.localhost:80"); got != "notes" {
		t.Fatalf("got %q", got)
	}
	if got := hostLabelFromRequest("example.com"); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestUniqueHostname(t *testing.T) {
	apps := []AppEntry{{ID: "1", Hostname: "notes"}}
	if got := uniqueHostname(apps, "2", "notes"); got != "notes-2" {
		t.Fatalf("got %q", got)
	}
	if got := uniqueHostname(apps, "1", "notes"); got != "notes" {
		t.Fatalf("got %q", got)
	}
}

func TestSlugHostname(t *testing.T) {
	if got := slugHostname("SNS Post Editor"); got != "sns-post-editor" {
		t.Fatalf("got %q", got)
	}
}

func TestPublicAppURLGatewayPort(t *testing.T) {
	entry := AppEntry{Hostname: "notes"}
	got := publicAppURL(entry, nil, AppStatusStopped, 0, "")
	if got == nil || *got != "http://notes.devportal.localhost" {
		t.Fatalf("got %v", got)
	}
	got = publicAppURL(entry, nil, AppStatusStopped, 80, "")
	if got == nil || *got != "http://notes.devportal.localhost" {
		t.Fatalf("got %v", got)
	}
	got = publicAppURL(entry, nil, AppStatusStopped, 7341, "")
	if got == nil || *got != "http://notes.devportal.localhost:7341" {
		t.Fatalf("got %v", got)
	}
}

func TestPublicAppURLLoopbackBind(t *testing.T) {
	entry := AppEntry{}
	port := uint16(3000)
	got := publicAppURL(entry, &port, AppStatusRunning, 0, "")
	if got == nil || *got != "http://127.0.0.1:3000" {
		t.Fatalf("got %v", got)
	}
	got = publicAppURL(entry, &port, AppStatusRunning, 0, "::1")
	if got == nil || *got != "http://[::1]:3000" {
		t.Fatalf("got %v", got)
	}
}
