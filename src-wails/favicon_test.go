package main

import (
	"encoding/base64"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
)

var png1x1 = mustDecode("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==")

const svgIcon = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16"><rect width="16" height="16" fill="#007aff"/></svg>`

func mustDecode(s string) []byte {
	raw, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return raw
}

func TestParseIconHrefsPrefersAppleTouchAndSvg(t *testing.T) {
	html := `
<link rel="stylesheet" href="/app.css">
<link rel="icon" href="/favicon.ico" sizes="16x16">
<link rel="icon" type="image/svg+xml" href="/icon.svg">
<link rel="apple-touch-icon" sizes="180x180" href="/apple.png">
<link rel="mask-icon" href="/mask.svg">
`
	refs := parseIconHrefs(html)
	if len(refs) != 3 {
		t.Fatalf("got %d refs: %+v", len(refs), refs)
	}
	sortRefs := parseIconHrefs(html)
	best := sortRefs[0]
	for _, ref := range sortRefs[1:] {
		if ref.score > best.score {
			best = ref
		}
	}
	if best.href != "/apple.png" && best.href != "/icon.svg" {
		t.Fatalf("unexpected best href %q score %d (%+v)", best.href, best.score, sortRefs)
	}
}

func TestParseIconHrefsShortcutAndQuotedAttrs(t *testing.T) {
	html := `<LINK REL='shortcut icon' HREF="/logo.png" TYPE="image/png">`
	refs := parseIconHrefs(html)
	if len(refs) != 1 || refs[0].href != "/logo.png" {
		t.Fatalf("got %+v", refs)
	}
	if refs[0].score <= 0 {
		t.Fatalf("expected positive score, got %d", refs[0].score)
	}
}

func TestSniffFaviconRejectsHTML(t *testing.T) {
	if _, ok := sniffFavicon([]byte("<!doctype html><html>nope</html>")); ok {
		t.Fatal("html must not look like an icon")
	}
	if mime, ok := sniffFavicon(png1x1); !ok || mime != "image/png" {
		t.Fatalf("png sniff failed: %s %v", mime, ok)
	}
	if mime, ok := sniffFavicon([]byte(svgIcon)); !ok || mime != "image/svg+xml" {
		t.Fatalf("svg sniff failed: %s %v", mime, ok)
	}
}

func TestDiscoverLocalFaviconFromPublicFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "public"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "public", "favicon.ico"), png1x1, 0o644); err != nil {
		t.Fatal(err)
	}
	data, ok := discoverLocalFavicon(dir)
	if !ok {
		t.Fatal("expected local favicon")
	}
	if !strings.HasPrefix(data, "data:image/png;base64,") {
		t.Fatalf("got %q", data)
	}
}

func TestDiscoverLocalFaviconFromIndexHTML(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "public"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "public", "vite.svg"), []byte(svgIcon), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "public", "favicon.ico"), png1x1, 0o644); err != nil {
		t.Fatal(err)
	}
	html := `<!doctype html><html><head><link rel="icon" type="image/svg+xml" href="/vite.svg"></head></html>`
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(html), 0o644); err != nil {
		t.Fatal(err)
	}
	data, ok := discoverLocalFavicon(dir)
	if !ok {
		t.Fatal("expected html-linked favicon")
	}
	if !strings.HasPrefix(data, "data:image/svg+xml;base64,") {
		t.Fatalf("expected svg from index.html, got %q", data[:min(48, len(data))])
	}
}

func TestDiscoverLocalFaviconIgnoresPathTraversal(t *testing.T) {
	root := t.TempDir()
	secret := filepath.Join(root, "secret.png")
	if err := os.WriteFile(secret, png1x1, 0o644); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "app")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	html := `<link rel="icon" href="../secret.png">`
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(html), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := discoverLocalFavicon(dir); ok {
		t.Fatal("must not read files outside the app folder")
	}
}

func TestFetchOriginFaviconFromLinkAndFallback(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><head><link rel="icon" type="image/svg+xml" href="/mark.svg"></head></html>`))
	})
	mux.HandleFunc("/mark.svg", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/svg+xml")
		_, _ = w.Write([]byte(svgIcon))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	data, ok := fetchOriginFavicon(srv.URL)
	if !ok {
		t.Fatal("expected remote favicon")
	}
	if !strings.HasPrefix(data, "data:image/svg+xml;base64,") {
		t.Fatalf("got %q", data[:min(48, len(data))])
	}

	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/favicon.ico" {
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(png1x1)
			return
		}
		if r.URL.Path == "/" {
			http.NotFound(w, r)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(fallback.Close)
	data, ok = fetchOriginFavicon(fallback.URL)
	if !ok {
		t.Fatal("expected /favicon.ico fallback")
	}
	if !strings.HasPrefix(data, "data:image/png;base64,") {
		t.Fatalf("got %q", data[:min(40, len(data))])
	}
}

func TestFetchOriginFaviconRejectsHTMLDisguisedAsIco(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/x-icon")
		_, _ = w.Write([]byte("<!doctype html><html>404</html>"))
	}))
	t.Cleanup(srv.Close)
	if _, ok := fetchOriginFavicon(srv.URL); ok {
		t.Fatal("html body must not be cached as a favicon")
	}
}

func TestRefreshLiveFaviconUpdatesViewAndCache(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><link rel="icon" href="/favicon.ico"></html>`))
	})
	mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(png1x1)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, portStr, err := net.SplitHostPort(u.Host)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	id := uuid.NewString()
	var got AppView
	p := &Portal{
		inner: Inner{
			configPath: filepath.Join(dir, "apps.yml"),
			file:       ConfigFile{Apps: []AppEntry{{ID: id, Name: "demo"}}},
			runtime: map[string]*Runtime{
				id: {ready: true, port: uint16(port), bindHost: "127.0.0.1"},
			},
			favicons: map[string]string{},
			errors:   map[string]string{},
		},
		EmitStatus: func(view AppView) { got = view },
	}
	p.refreshLiveFavicon(id)
	if got.Favicon == nil || !strings.HasPrefix(*got.Favicon, "data:image/png;base64,") {
		t.Fatalf("expected emitted favicon, got %+v", got.Favicon)
	}
	cached := filepath.Join(dir, "favicons", id+".data")
	raw, err := os.ReadFile(cached)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(raw), "data:image/png;base64,") {
		t.Fatalf("cache %q", raw)
	}

	reloaded := loadFaviconCache(p.inner.configPath)
	if reloaded[id] != *got.Favicon {
		t.Fatal("cache roundtrip mismatch")
	}
}

func TestViewForIncludesCachedFavicon(t *testing.T) {
	id := "app-1"
	data := encodeFavicon("image/png", png1x1)
	inner := Inner{
		file:     ConfigFile{Apps: []AppEntry{{ID: id, Name: "notes"}}},
		runtime:  map[string]*Runtime{},
		errors:   map[string]string{},
		favicons: map[string]string{id: data},
	}
	view := viewFor(&inner, inner.file.Apps[0])
	if view.Favicon == nil || *view.Favicon != data {
		t.Fatalf("expected cached favicon on view, got %+v", view.Favicon)
	}
}

func TestSafeFaviconIDRejectsTraversal(t *testing.T) {
	if safeFaviconID("../etc") != "" {
		t.Fatal("expected empty")
	}
	if safeFaviconID("abc-123_Z") == "" {
		t.Fatal("expected uuid-like id")
	}
}

func TestNormalizeDataURL(t *testing.T) {
	encoded := encodeFavicon("image/png", png1x1)
	got, ok := normalizeDataURL(encoded)
	if !ok || got != encoded {
		t.Fatalf("got %v %q", ok, got)
	}
	svgData := "data:image/svg+xml," + url.PathEscape(svgIcon)
	got, ok = normalizeDataURL(svgData)
	if !ok || !strings.HasPrefix(got, "data:image/svg+xml;base64,") {
		t.Fatalf("svg data url: %v %q", ok, got)
	}
}
