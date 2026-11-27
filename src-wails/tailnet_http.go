package main

import (
	"context"
	_ "embed"
	"html/template"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

type tailnetLookup func(context.Context, string) (string, error)

//go:embed tailnet_portal.html
var tailnetPortalHTML string

var tailnetPortalTemplate = template.Must(template.New("tailnet").Parse(tailnetPortalHTML))

func (p *Portal) authorizeTailnet(authority string, port uint16, appID string, lookup tailnetLookup, next http.HandlerFunc) http.HandlerFunc {
	origin := "https://" + authority
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		host := strings.ToLower(r.Host)
		if host != authority && (port != 443 || host != authority+":443") {
			http.Error(w, "別の URL を使用してください", http.StatusMisdirectedRequest)
			return
		}
		requestOrigin := r.Header.Get("Origin")
		safe := r.Method == http.MethodGet || r.Method == http.MethodHead
		fetchSite := r.Header.Get("Sec-Fetch-Site")
		otherOrigin := fetchSite == "cross-site" || fetchSite == "same-site"
		navigation := safe && r.Header.Get("Sec-Fetch-Mode") == "navigate" && r.Header.Get("Sec-Fetch-Dest") == "document"
		if (otherOrigin && !navigation) || (requestOrigin != "" && requestOrigin != origin) ||
			(port == 443 && !safe && requestOrigin != origin) {
			http.Error(w, "アクセスが許可されていません", http.StatusForbidden)
			return
		}

		lookupCtx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		device, err := lookup(lookupCtx, r.RemoteAddr)
		cancel()
		if err != nil || device == "" {
			http.Error(w, "アクセスが許可されていません", http.StatusForbidden)
			return
		}
		inner := p.lock()
		settings := inner.file.Tailnet.TailnetSettings
		allowed := settings.Enabled && slices.Contains(settings.DeviceIDs, device)
		if allowed && appID != "" {
			_, err := findEntry(inner, appID)
			app := settings.Apps[appID]
			allowed = err == nil && app.Port == port && app.allows(device)
		}
		if allowed && inner.tailnetPolicy == nil {
			inner.invalidateTailnetPolicy()
		}
		policy := inner.tailnetPolicy
		p.unlock()
		if !allowed {
			http.Error(w, "アクセスが許可されていません", http.StatusForbidden)
			return
		}

		// Policy changes also cancel already-authorized streams and WebSocket proxies.
		ctx, cancel := context.WithCancel(policy)
		stop := context.AfterFunc(r.Context(), cancel)
		defer stop()
		defer cancel()
		if ctx.Err() != nil || r.Context().Err() != nil {
			http.Error(w, "アクセスが許可されていません", http.StatusForbidden)
			return
		}
		ctx = context.WithValue(ctx, tailnetDeviceKey{}, device)
		next(w, r.WithContext(ctx))
	}
}

type tailnetDeviceKey struct{}

func (app TailnetAppSettings) allows(device string) bool {
	return app.Enabled && (!app.RestrictDevices || slices.Contains(app.DeviceIDs, device))
}

func (p *Portal) tailnetApp(host string, port uint16, id string, lookup tailnetLookup) http.HandlerFunc {
	authority := net.JoinHostPort(host, itoaPort(port))
	return p.authorizeTailnet(authority, port, id, lookup, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodConnect {
			http.Error(w, "この HTTP メソッドには対応していません", http.StatusMethodNotAllowed)
			return
		}
		inner := p.lock()
		_, running := inner.runtime[id]
		p.unlock()
		p.proxyApp(w, r, id, running, true)
	})
}

func (p *Portal) tailnetPortal(host string, lookup tailnetLookup) http.HandlerFunc {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		device := r.Context().Value(tailnetDeviceKey{}).(string)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; img-src data:; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		_ = tailnetPortalTemplate.Execute(w, p.tailnetGroups(host, device))
	})
	mux.HandleFunc("POST /apps/{id}/{action}", func(w http.ResponseWriter, r *http.Request) {
		id, action := r.PathValue("id"), r.PathValue("action")
		device := r.Context().Value(tailnetDeviceKey{}).(string)
		inner := p.lock()
		_, err := findEntry(inner, id)
		settings := inner.file.Tailnet.TailnetSettings
		app := settings.Apps[id]
		allowed := err == nil && settings.Enabled && slices.Contains(settings.DeviceIDs, device) && app.Port >= tailnetFirstAppPort && app.allows(device)
		p.unlock()
		if !allowed || (action != "start" && action != "stop") {
			http.NotFound(w, r)
			return
		}
		var view AppView
		switch action {
		case "start":
			view, err = p.startWithContext(r.Context(), id, true)
		case "stop":
			view, err = p.stopWithContext(r.Context(), id)
		}
		if err != nil {
			http.Error(w, "アプリを操作できませんでした。詳細は DevPortal のデスクトップ画面で確認してください。", http.StatusServiceUnavailable)
			return
		}
		p.emitStatus(view)
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})
	return p.authorizeTailnet(host, 443, "", lookup, mux.ServeHTTP)
}

type tailnetGroup struct {
	Name string
	Apps []tailnetCard
}

type tailnetCard struct {
	Name, Description, URL, ActionURL, ActionLabel, StatusLabel string
	Status                                                      AppStatus
	Favicon                                                     template.URL
	Pinned                                                      bool
}

func (p *Portal) tailnetGroups(host, device string) []tailnetGroup {
	inner := p.lock()
	defer p.unlock()
	settings := inner.file.Tailnet.TailnetSettings
	if !settings.Enabled || !slices.Contains(settings.DeviceIDs, device) {
		return nil
	}
	groups := make([]tailnetGroup, 0, len(inner.file.Categories)+1)
	indices := make(map[string]int, len(inner.file.Categories))
	for _, category := range inner.file.Categories {
		indices[category.ID] = len(groups)
		groups = append(groups, tailnetGroup{Name: category.Name})
	}
	uncategorized := len(groups)
	groups = append(groups, tailnetGroup{Name: "カテゴリなし"})
	for _, entry := range inner.file.Apps {
		app := settings.Apps[entry.ID]
		if app.Port < tailnetFirstAppPort || !app.allows(device) {
			continue
		}
		view := viewFor(inner, entry)
		link := url.URL{Scheme: "https", Host: net.JoinHostPort(host, itoaPort(app.Port)), Path: "/"}
		card := tailnetCard{
			Name: entry.Name, Description: entry.Description, URL: link.String(),
			ActionURL: "/apps/" + url.PathEscape(entry.ID) + "/start", ActionLabel: "起動",
			Status: view.Status, StatusLabel: "停止中", Pinned: entry.BoardPinned,
		}
		switch view.Status {
		case AppStatusRunning:
			card.StatusLabel = "稼働中"
		case AppStatusStarting:
			card.StatusLabel = "起動中"
		case AppStatusError:
			card.StatusLabel = "エラー"
		}
		if _, running := inner.runtime[entry.ID]; running {
			card.ActionURL = "/apps/" + url.PathEscape(entry.ID) + "/stop"
			card.ActionLabel = "停止"
		}
		if view.Favicon != nil && strings.HasPrefix(*view.Favicon, "data:image/") {
			card.Favicon = template.URL(*view.Favicon)
		}
		i, ok := indices[entry.CategoryID]
		if !ok {
			i = uncategorized
		}
		groups[i].Apps = append(groups[i].Apps, card)
	}
	groups = slices.DeleteFunc(groups, func(group tailnetGroup) bool { return len(group.Apps) == 0 })
	for _, group := range groups {
		slices.SortStableFunc(group.Apps, func(a, b tailnetCard) int {
			if a.Pinned == b.Pinned {
				return 0
			}
			if a.Pinned {
				return -1
			}
			return 1
		})
	}
	return groups
}
