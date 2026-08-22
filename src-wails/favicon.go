package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
)

const (
	maxFaviconBytes = 256 * 1024
	maxHTMLBytes    = 256 * 1024
)

var faviconClient = &http.Client{
	Timeout: 4 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 4 {
			return errString("too many redirects")
		}
		if req.URL == nil || (req.URL.Scheme != "http" && req.URL.Scheme != "https") {
			return errString("unsupported redirect")
		}
		return nil
	},
}

var (
	linkTagRe = regexp.MustCompile(`(?is)<link\b([^>]*?)>`)
	attrRe    = regexp.MustCompile(`(?i)([^\s=/>]+)\s*=\s*("[^"]*"|'[^']*'|[^\s>]+)`)
)

var wellKnownFaviconFiles = []string{
	"favicon.ico",
	"favicon.png",
	"favicon.svg",
	"apple-touch-icon.png",
	"apple-touch-icon.ico",
	"public/favicon.ico",
	"public/favicon.png",
	"public/favicon.svg",
	"public/apple-touch-icon.png",
	"public/vite.svg",
	"static/favicon.ico",
	"static/favicon.png",
	"static/favicon.svg",
	"src/favicon.ico",
	"src/favicon.svg",
	"src/favicon.png",
	"app/favicon.ico",
	"app/icon.png",
	"app/icon.ico",
	"app/icon.svg",
	"app/apple-icon.png",
}

var htmlIndexCandidates = []string{
	"index.html",
	"public/index.html",
	"src/index.html",
}

type iconRef struct {
	href  string
	score int
}

func attachFavicon(inner *Inner, view AppView) AppView {
	if inner == nil || inner.favicons == nil {
		return view
	}
	if data := inner.favicons[view.ID]; data != "" {
		view.Favicon = &data
	}
	return view
}

func (p *Portal) hydrateMissingFavicons() {
	inner := p.lock()
	defer p.unlock()
	if inner.favicons == nil {
		inner.favicons = map[string]string{}
	}
	for _, app := range inner.file.Apps {
		if inner.favicons[app.ID] != "" {
			continue
		}
		data, ok := discoverLocalFavicon(app.Folder)
		if !ok {
			continue
		}
		inner.favicons[app.ID] = data
		persistFavicon(inner.configPath, app.ID, data)
	}
}

func (p *Portal) captureLocalFaviconLocked(entry AppEntry, folderChanged bool) {
	if inner := &p.inner; inner.favicons == nil {
		inner.favicons = map[string]string{}
	}
	if folderChanged {
		delete(p.inner.favicons, entry.ID)
		removeFaviconFile(p.inner.configPath, entry.ID)
	}
	if p.inner.favicons[entry.ID] != "" {
		return
	}
	data, ok := discoverLocalFavicon(entry.Folder)
	if !ok {
		return
	}
	p.inner.favicons[entry.ID] = data
	persistFavicon(p.inner.configPath, entry.ID, data)
}

func (p *Portal) beginFaviconFetch(id string) bool {
	p.faviconMu.Lock()
	defer p.faviconMu.Unlock()
	if p.faviconInflight == nil {
		p.faviconInflight = map[string]struct{}{}
	}
	if _, busy := p.faviconInflight[id]; busy {
		return false
	}
	p.faviconInflight[id] = struct{}{}
	return true
}

func (p *Portal) endFaviconFetch(id string) {
	p.faviconMu.Lock()
	delete(p.faviconInflight, id)
	p.faviconMu.Unlock()
}

func (p *Portal) liveFaviconOrigin(id string) (string, bool) {
	inner := p.lock()
	defer p.unlock()
	rt, ok := inner.runtime[id]
	if !ok || !rt.ready || rt.port == 0 {
		return "", false
	}
	host := rt.bindHost
	if host == "" {
		host = loopbackV4
	}
	return loopbackURL(host, rt.port), true
}

func (p *Portal) storeFavicon(id, data string) bool {
	if strings.TrimSpace(data) == "" {
		return false
	}
	inner := p.lock()
	defer p.unlock()
	if inner.favicons == nil {
		inner.favicons = map[string]string{}
	}
	if inner.favicons[id] == data {
		return false
	}
	inner.favicons[id] = data
	persistFavicon(inner.configPath, id, data)
	return true
}

func (p *Portal) emitFaviconView(id string) {
	inner := p.lock()
	entry, err := findEntry(inner, id)
	var view AppView
	if err == nil {
		view = viewFor(inner, entry)
	}
	p.unlock()
	if err == nil {
		p.emitStatus(view)
	}
}

func (p *Portal) refreshLiveFavicon(id string) {
	if !p.beginFaviconFetch(id) {
		return
	}
	defer p.endFaviconFetch(id)

	for i := 0; i < 6; i++ {
		if i > 0 {
			time.Sleep(400 * time.Millisecond)
		}
		origin, ok := p.liveFaviconOrigin(id)
		if !ok {
			return
		}
		data, found := fetchOriginFavicon(origin)
		if !found {
			continue
		}
		if p.storeFavicon(id, data) {
			p.emitFaviconView(id)
		}
		return
	}
}

func discoverLocalFavicon(folder string) (string, bool) {
	folder = strings.TrimSpace(folder)
	if folder == "" {
		return "", false
	}
	info, err := os.Stat(folder)
	if err != nil || !info.IsDir() {
		return "", false
	}
	for _, rel := range htmlIndexCandidates {
		path := filepath.Join(folder, filepath.FromSlash(rel))
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			continue
		}
		if data, ok := pickLocalIcon(folder, filepath.Dir(path), raw); ok {
			return data, true
		}
	}
	for _, rel := range wellKnownFaviconFiles {
		path := filepath.Join(folder, filepath.FromSlash(rel))
		if data, ok := readFaviconFile(path); ok {
			return data, true
		}
	}
	return "", false
}

func pickLocalIcon(folder, htmlDir string, html []byte) (string, bool) {
	refs := parseIconHrefs(string(clipBytes(html, maxHTMLBytes)))
	sort.SliceStable(refs, func(i, j int) bool { return refs[i].score > refs[j].score })
	for _, ref := range refs {
		href := strings.TrimSpace(ref.href)
		if href == "" {
			continue
		}
		if strings.HasPrefix(strings.ToLower(href), "data:") {
			if data, ok := normalizeDataURL(href); ok {
				return data, true
			}
			continue
		}
		if looksRemote(href) {
			continue
		}
		for _, candidate := range localIconPaths(folder, htmlDir, href) {
			if data, ok := readFaviconFile(candidate); ok {
				return data, true
			}
		}
	}
	return "", false
}

func localIconPaths(folder, htmlDir, href string) []string {
	href = strings.TrimSpace(href)
	href = strings.SplitN(href, "?", 2)[0]
	href = strings.SplitN(href, "#", 2)[0]
	if href == "" || looksRemote(href) {
		return nil
	}
	rel := filepath.FromSlash(strings.TrimPrefix(href, "/"))
	var out []string
	add := func(base string) {
		joined := filepath.Join(base, rel)
		confined, ok := confinedPath(folder, joined)
		if ok {
			out = append(out, confined)
		}
	}
	if strings.HasPrefix(href, "/") {
		add(folder)
		add(filepath.Join(folder, "public"))
		add(filepath.Join(folder, "static"))
		add(htmlDir)
	} else {
		add(htmlDir)
		add(folder)
		add(filepath.Join(folder, "public"))
	}
	return uniqueStrings(out)
}

func confinedPath(root, target string) (string, bool) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", false
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return "", false
	}
	sep := string(os.PathSeparator)
	if absTarget == absRoot {
		return absTarget, true
	}
	if strings.HasPrefix(absTarget, absRoot+sep) {
		return absTarget, true
	}
	return "", false
}

func uniqueStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func looksRemote(href string) bool {
	s := strings.ToLower(strings.TrimSpace(href))
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "//")
}

func readFaviconFile(path string) (string, bool) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Size() <= 0 || info.Size() > maxFaviconBytes {
		return "", false
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	mime, ok := sniffFavicon(raw)
	if !ok {
		return "", false
	}
	return encodeFavicon(mime, raw), true
}

func fetchOriginFavicon(origin string) (string, bool) {
	origin = strings.TrimRight(strings.TrimSpace(origin), "/")
	if origin == "" {
		return "", false
	}
	base, err := url.Parse(origin + "/")
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") {
		return "", false
	}
	html, pageURL, ok := fetchHTML(base)
	if ok && pageURL != nil {
		refs := parseIconHrefs(html)
		sort.SliceStable(refs, func(i, j int) bool { return refs[i].score > refs[j].score })
		for _, ref := range refs {
			if data, found := fetchIconURL(pageURL, ref.href); found {
				return data, true
			}
		}
	}
	for _, path := range []string{"/favicon.ico", "/favicon.svg", "/favicon.png", "/apple-touch-icon.png", "/vite.svg"} {
		if data, found := fetchIconURL(base, path); found {
			return data, true
		}
	}
	return "", false
}

func fetchHTML(base *url.URL) (string, *url.URL, bool) {
	req, err := http.NewRequest(http.MethodGet, base.String(), nil)
	if err != nil {
		return "", nil, false
	}
	req.Header.Set("User-Agent", "DevPortal/0.1 (favicon)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml;q=0.9,*/*;q=0.8")
	resp, err := faviconClient.Do(req)
	if err != nil {
		return "", nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return "", nil, false
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxHTMLBytes+1))
	if err != nil || len(raw) == 0 {
		return "", nil, false
	}
	if len(raw) > maxHTMLBytes {
		raw = raw[:maxHTMLBytes]
	}
	final := base
	if resp.Request != nil && resp.Request.URL != nil {
		final = resp.Request.URL
	}
	return string(raw), final, true
}

func fetchIconURL(base *url.URL, href string) (string, bool) {
	href = strings.TrimSpace(href)
	if href == "" || base == nil {
		return "", false
	}
	if strings.HasPrefix(strings.ToLower(href), "data:") {
		return normalizeDataURL(href)
	}
	resolved, err := base.Parse(href)
	if err != nil {
		return "", false
	}
	if resolved.Scheme != "http" && resolved.Scheme != "https" {
		return "", false
	}
	req, err := http.NewRequest(http.MethodGet, resolved.String(), nil)
	if err != nil {
		return "", false
	}
	req.Header.Set("User-Agent", "DevPortal/0.1 (favicon)")
	req.Header.Set("Accept", "image/avif,image/webp,image/svg+xml,image/*,*/*;q=0.8")
	resp, err := faviconClient.Do(req)
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", false
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxFaviconBytes+1))
	if err != nil || len(raw) == 0 || len(raw) > maxFaviconBytes {
		return "", false
	}
	mime, ok := sniffFavicon(raw)
	if !ok {
		return "", false
	}
	return encodeFavicon(mime, raw), true
}

func parseIconHrefs(html string) []iconRef {
	var out []iconRef
	for _, m := range linkTagRe.FindAllStringSubmatch(html, 40) {
		if len(m) < 2 {
			continue
		}
		attrs := parseAttrs(m[1])
		href := strings.TrimSpace(attrs["href"])
		if href == "" {
			continue
		}
		score := iconScore(attrs["rel"], attrs["type"], attrs["sizes"])
		if score < 0 {
			continue
		}
		out = append(out, iconRef{href: href, score: score})
	}
	return out
}

func parseAttrs(s string) map[string]string {
	out := map[string]string{}
	for _, m := range attrRe.FindAllStringSubmatch(s, 24) {
		if len(m) < 3 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(m[1]))
		val := strings.TrimSpace(m[2])
		if len(val) >= 2 && (val[0] == '"' || val[0] == '\'') {
			val = val[1 : len(val)-1]
		}
		out[key] = htmlUnescape(val)
	}
	return out
}

func iconScore(rel, typ, sizes string) int {
	rel = strings.ToLower(strings.TrimSpace(rel))
	typ = strings.ToLower(strings.TrimSpace(typ))
	if rel == "" {
		return -1
	}
	if strings.Contains(rel, "mask-icon") || strings.Contains(rel, "manifest") {
		return -1
	}
	hasIcon := false
	apple := false
	for _, part := range strings.Fields(rel) {
		if part == "icon" || part == "shortcut" {
			hasIcon = true
		}
		if strings.Contains(part, "icon") {
			hasIcon = true
		}
		if strings.HasPrefix(part, "apple-touch-icon") {
			apple = true
			hasIcon = true
		}
	}
	if !hasIcon {
		return -1
	}
	score := 10
	if apple {
		score += 30
	}
	switch {
	case strings.Contains(typ, "svg"):
		score += 40
	case strings.Contains(typ, "png"):
		score += 20
	case strings.Contains(typ, "icon"):
		score += 5
	}
	score += sizeScore(sizes)
	return score
}

func sizeScore(sizes string) int {
	sizes = strings.ToLower(strings.TrimSpace(sizes))
	if sizes == "" || sizes == "any" {
		return 8
	}
	best := 0
	for _, part := range strings.Fields(sizes) {
		var w, h int
		if _, err := fmt.Sscanf(part, "%dx%d", &w, &h); err != nil {
			continue
		}
		side := w
		if h > side {
			side = h
		}
		if side > best {
			best = side
		}
	}
	switch {
	case best >= 180:
		return 25
	case best >= 64:
		return 20
	case best >= 32:
		return 15
	case best >= 16:
		return 5
	default:
		return 0
	}
}

func htmlUnescape(s string) string {
	replacer := strings.NewReplacer(
		"&amp;", "&",
		"&quot;", `"`,
		"&#39;", "'",
		"&#x27;", "'",
		"&apos;", "'",
		"&lt;", "<",
		"&gt;", ">",
	)
	return replacer.Replace(s)
}

func sniffFavicon(raw []byte) (string, bool) {
	if len(raw) < 4 {
		return "", false
	}
	switch {
	case bytes.HasPrefix(raw, []byte("\x89PNG\r\n\x1a\n")):
		return "image/png", true
	case bytes.HasPrefix(raw, []byte{0xff, 0xd8}):
		return "image/jpeg", true
	case bytes.HasPrefix(raw, []byte("GIF87a")) || bytes.HasPrefix(raw, []byte("GIF89a")):
		return "image/gif", true
	case len(raw) >= 12 && bytes.HasPrefix(raw, []byte("RIFF")) && bytes.Equal(raw[8:12], []byte("WEBP")):
		return "image/webp", true
	case bytes.HasPrefix(raw, []byte{0x00, 0x00, 0x01, 0x00}) || bytes.HasPrefix(raw, []byte{0x00, 0x00, 0x02, 0x00}):
		return "image/x-icon", true
	}
	trimmed := bytes.TrimSpace(raw)
	lower := bytes.ToLower(trimmed)
	if bytes.Contains(lower, []byte("<html")) || bytes.Contains(lower, []byte("<!doctype html")) {
		return "", false
	}
	if bytes.HasPrefix(lower, []byte("<svg")) || (bytes.HasPrefix(lower, []byte("<?xml")) && bytes.Contains(lower, []byte("<svg"))) {
		return "image/svg+xml", true
	}
	return "", false
}

func encodeFavicon(mime string, raw []byte) string {
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(raw)
}

func normalizeDataURL(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(strings.ToLower(s), "data:") {
		return "", false
	}
	comma := strings.IndexByte(s, ',')
	if comma < 0 {
		return "", false
	}
	header := s[:comma]
	payload := s[comma+1:]
	meta := strings.TrimPrefix(strings.ToLower(header), "data:")
	parts := strings.Split(meta, ";")
	isBase64 := false
	for _, part := range parts[1:] {
		if strings.TrimSpace(part) == "base64" {
			isBase64 = true
		}
	}
	var raw []byte
	if isBase64 {
		decoded, err := base64.StdEncoding.DecodeString(payload)
		if err != nil {
			decoded, err = base64.RawStdEncoding.DecodeString(strings.TrimRight(payload, "="))
			if err != nil {
				return "", false
			}
		}
		raw = decoded
	} else {
		decoded, err := url.QueryUnescape(payload)
		if err != nil {
			decoded = payload
		}
		raw = []byte(decoded)
	}
	mime, ok := sniffFavicon(raw)
	if !ok {
		return "", false
	}
	return encodeFavicon(mime, raw), true
}

func clipBytes(raw []byte, n int) []byte {
	if len(raw) > n {
		return raw[:n]
	}
	return raw
}

func faviconCacheDir(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), "favicons")
}

func safeFaviconID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	for _, r := range id {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' {
			continue
		}
		return ""
	}
	return id
}

func loadFaviconCache(configPath string) map[string]string {
	out := map[string]string{}
	if strings.TrimSpace(configPath) == "" {
		return out
	}
	entries, err := os.ReadDir(faviconCacheDir(configPath))
	if err != nil {
		return out
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".data") {
			continue
		}
		id := safeFaviconID(strings.TrimSuffix(entry.Name(), ".data"))
		if id == "" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(faviconCacheDir(configPath), entry.Name()))
		if err != nil {
			continue
		}
		data := strings.TrimSpace(string(raw))
		if !strings.HasPrefix(data, "data:image/") {
			continue
		}
		out[id] = data
	}
	return out
}

func persistFavicon(configPath, id, data string) {
	id = safeFaviconID(id)
	if strings.TrimSpace(configPath) == "" || id == "" || !strings.HasPrefix(data, "data:image/") {
		return
	}
	dir := faviconCacheDir(configPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(dir, id+".data"), []byte(data), 0o644)
}

func removeFaviconFile(configPath, id string) {
	id = safeFaviconID(id)
	if strings.TrimSpace(configPath) == "" || id == "" {
		return
	}
	_ = os.Remove(filepath.Join(faviconCacheDir(configPath), id+".data"))
}
