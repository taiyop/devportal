package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	defaultGatewayPort uint16 = 80
	readyWait                 = 20 * time.Second
)

var fallbackGatewayPorts = []uint16{80, 7341, 8080, 8888, 18080}

type GatewayStatus struct {
	Addr          string `json:"addr"`
	Port          uint16 `json:"port"`
	PreferredPort uint16 `json:"preferredPort"`
	AllowLAN      bool   `json:"allowLan"`
	Running       bool   `json:"running"`
	Error         string `json:"error"`
	Note          string `json:"note"`
}

func preferredGatewayPort(file ConfigFile) uint16 {
	if file.GatewayPort == nil || *file.GatewayPort == 0 {
		return defaultGatewayPort
	}
	return *file.GatewayPort
}

func normalizeGatewayPort(port uint16) uint16 {
	if port == 0 {
		return defaultGatewayPort
	}
	return port
}

func listenAddrsFor(port uint16) []string {
	preferred := normalizeGatewayPort(port)
	seen := map[uint16]bool{preferred: true}
	addrs := []string{fmt.Sprintf(":%d", preferred)}
	for _, extra := range fallbackGatewayPorts {
		if seen[extra] {
			continue
		}
		seen[extra] = true
		addrs = append(addrs, fmt.Sprintf(":%d", extra))
	}
	return append(addrs, ":0")
}

func (p *Portal) gatewayConfig() (uint16, bool) {
	inner := p.lock()
	defer p.unlock()
	return preferredGatewayPort(inner.file), inner.file.GatewayAllowLAN
}

func (p *Portal) StartGateway() error {
	p.gatewayMu.Lock()
	port, allowLAN := p.gatewayConfig()
	err := p.startGatewayLocked(listenAddrsFor(port), allowLAN)
	p.gatewayMu.Unlock()
	p.emitGateway()
	p.emitAllViews()
	return err
}

func (p *Portal) SetGatewayPort(port uint16) error {
	port = normalizeGatewayPort(port)
	return p.setGatewayConfig(func(file *ConfigFile) {
		file.GatewayPort = &port
	})
}

func (p *Portal) SetGatewayAllowLAN(allow bool) error {
	return p.setGatewayConfig(func(file *ConfigFile) { file.GatewayAllowLAN = allow })
}

func (p *Portal) setGatewayConfig(update func(*ConfigFile)) error {
	p.gatewayMu.Lock()
	inner := p.lock()
	prev := inner.file
	next := prev
	update(&next)
	err := saveConfig(inner.configPath, next)
	if err == nil {
		inner.file = next
	}
	p.unlock()
	if err != nil {
		p.gatewayMu.Unlock()
		return err
	}

	port := preferredGatewayPort(next)
	changed := preferredGatewayPort(prev) != port || prev.GatewayAllowLAN != next.GatewayAllowLAN
	if changed && p.gateway != nil {
		p.stopGatewayLocked()
		err = p.startGatewayLocked(listenAddrsFor(port), next.GatewayAllowLAN)
	}
	p.gatewayMu.Unlock()
	p.emitGateway()
	p.emitAllViews()
	return err
}

func (p *Portal) startGatewayLocked(addrs []string, allowLAN bool) error {
	if p.gateway != nil {
		return nil
	}
	var lan []gatewayLANAddress
	if allowLAN {
		var err error
		lan, err = gatewayLANAddresses()
		if err != nil {
			p.gatewayErr = fmt.Sprintf("LAN のアドレスを取得できません: %v", err)
			return errString(p.gatewayErr)
		}
	}

	preferred := listenPortOf(addrs[0])
	if preferred == 0 {
		preferred = int(defaultGatewayPort)
	}
	deniedPreferred := false
	busyPreferred := false
	for i, addr := range addrs {
		candidate := listenPortOf(addr)
		listeners, err := listenGateway(strconv.Itoa(candidate), lan)
		if err != nil {
			if candidate == preferred {
				deniedPreferred = deniedPreferred || listenDenied(err)
				busyPreferred = busyPreferred || listenBusy(err)
			}
			if i < len(addrs)-1 && shouldTryNextListen(err) {
				continue
			}
			p.gatewayErr = fmt.Sprintf("%s で待ち受けできません: %v", addr, err)
			return errString(p.gatewayErr)
		}

		port := tcpListenPort(listeners[0])
		note := ""
		if int(port) != preferred {
			switch {
			case deniedPreferred && preferred == int(defaultGatewayPort):
				note = fmt.Sprintf("ポート %d は管理者権限が必要なため、ポート %d で待ち受けています。", preferred, port)
			case deniedPreferred:
				note = fmt.Sprintf("ポート %d は権限が必要なため、:%d で待ち受けています。", preferred, port)
			case busyPreferred:
				note = fmt.Sprintf("ポート %d は使用中のため、:%d で待ち受けています。", preferred, port)
			default:
				note = fmt.Sprintf(":%d で待ち受けています。", port)
			}
		}
		if allowLAN && len(lan) == 0 {
			note = strings.TrimSpace(note + " LAN 用のアドレスが見つかりません。")
		}
		p.serveGatewayLocked(listeners, port, note, allowLAN)
		p.syncInnerGatewayPort(port)
		return nil
	}

	p.gatewayErr = "リバースプロキシを開始できません"
	return errString(p.gatewayErr)
}

func (p *Portal) serveGatewayLocked(listeners []net.Listener, port uint16, note string, allowLAN bool) {
	ctx, cancel := context.WithCancel(context.Background())
	srv := &http.Server{
		Addr: listeners[0].Addr().String(),
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p.handleGateway(w, r, allowLAN)
		}),
		BaseContext:       func(net.Listener) context.Context { return ctx },
		ReadHeaderTimeout: 10 * time.Second,
	}
	p.gateway = srv
	p.gatewayListeners = listeners
	p.gatewayCancel = cancel
	p.gatewayListen = srv.Addr
	p.gatewayPort = port
	p.gatewayNote = note
	p.gatewayErr = ""

	for _, listener := range listeners {
		go func() { _ = srv.Serve(listener) }()
	}
}

func (p *Portal) StopGateway() {
	p.gatewayMu.Lock()
	p.stopGatewayLocked()
	p.gatewayMu.Unlock()
	p.emitGateway()
	p.emitAllViews()
}

func (p *Portal) stopGatewayLocked() {
	if p.gatewayCancel != nil {
		p.gatewayCancel()
	}
	for _, listener := range p.gatewayListeners {
		_ = listener.Close()
	}
	if p.gateway != nil {
		_ = p.gateway.Close()
	}
	p.gateway = nil
	p.gatewayListeners = nil
	p.gatewayCancel = nil
	p.gatewayListen = ""
	p.gatewayPort = 0
	p.gatewayNote = ""
	p.gatewayErr = ""
	p.syncInnerGatewayPort(0)
}

func (p *Portal) PrepareHostnameURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw, nil
	}
	host := strings.ToLower(u.Hostname())
	if hostLabelFromRequest(host) == "" {
		return raw, nil
	}
	if !p.GatewayStatus().Running {
		if startErr := p.StartGateway(); startErr != nil {
			return "", startErr
		}
	}
	st := p.GatewayStatus()
	if !st.Running {
		return "", errString("リバースプロキシが起動していません")
	}
	if st.Port == 0 || st.Port == 80 {
		u.Host = host
	} else {
		u.Host = net.JoinHostPort(host, strconv.Itoa(int(st.Port)))
	}
	return u.String(), nil
}

func (p *Portal) GatewayStatus() GatewayStatus {
	p.gatewayMu.Lock()
	defer p.gatewayMu.Unlock()
	preferred, allowLAN := p.gatewayConfig()
	return p.gatewaySnapshot(preferred, allowLAN)
}

func (p *Portal) emitGateway() {
	if p.EmitGateway != nil {
		p.EmitGateway(p.GatewayStatus())
	}
}

func (p *Portal) emitAllViews() {
	if p.EmitStatus == nil {
		return
	}
	for _, view := range p.Views() {
		p.emitStatus(view)
	}
}

func (p *Portal) syncInnerGatewayPort(port uint16) {
	inner := p.lock()
	inner.gatewayPort = port
	p.unlock()
}

func (p *Portal) gatewaySnapshot(preferred uint16, allowLAN bool) GatewayStatus {
	preferred = normalizeGatewayPort(preferred)
	port := p.gatewayPort
	if port == 0 {
		port = preferred
	}
	return GatewayStatus{
		Addr:          "http://" + displayListenAddr(p.gatewayListen, port),
		Port:          port,
		PreferredPort: preferred,
		AllowLAN:      allowLAN,
		Running:       p.gateway != nil && p.gatewayErr == "",
		Error:         p.gatewayErr,
		Note:          p.gatewayNote,
	}
}

func displayListenAddr(listen string, port uint16) string {
	if port == 0 {
		port = 80
	}
	host := "127.0.0.1"
	if h, _, err := net.SplitHostPort(listen); err == nil {
		h = strings.Trim(h, "[]")
		if ip := net.ParseIP(h); ip != nil && !ip.IsUnspecified() {
			if ip.To4() == nil {
				host = "[" + ip.String() + "]"
			} else {
				host = ip.String()
			}
		}
	}
	return fmt.Sprintf("%s:%d", host, port)
}

func listenPortOf(addr string) int {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return 0
	}
	n, convErr := strconv.Atoi(port)
	if convErr != nil {
		return 0
	}
	return n
}

func tcpListenPort(ln net.Listener) uint16 {
	ta, ok := ln.Addr().(*net.TCPAddr)
	if !ok || ta.Port <= 0 || ta.Port > 65535 {
		return 0
	}
	return uint16(ta.Port)
}

func shouldTryNextListen(err error) bool {
	return listenDenied(err) || listenBusy(err)
}

func listenDenied(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, syscall.EACCES) || errors.Is(err, syscall.EPERM) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "permission denied") || strings.Contains(msg, "access is denied")
}

func listenBusy(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, syscall.EADDRINUSE) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "address already in use") ||
		strings.Contains(msg, "only one usage of each socket address")
}

func (p *Portal) handleGateway(w http.ResponseWriter, r *http.Request, allowLAN bool) {
	label := hostLabelFromRequest(r.Host)
	if label == "" && allowLAN {
		label = hostLabelForSuffix(r.Host, lanHostnameSuffix)
	}
	if label == "" {
		http.Error(w, "DevPortal: ホスト名が無効です", http.StatusBadRequest)
		return
	}
	inner := p.lock()
	var entry AppEntry
	found := false
	running := false
	for _, app := range inner.file.Apps {
		if app.Hostname == label {
			entry = app
			found = true
			_, running = inner.runtime[app.ID]
			break
		}
	}
	p.unlock()
	if !found {
		http.Error(w, "DevPortal: このホストに対応するアプリがありません\n", http.StatusNotFound)
		return
	}
	p.proxyApp(w, r, entry.ID, running, false)
}

func (p *Portal) proxyApp(w http.ResponseWriter, r *http.Request, id string, running, tailnet bool) {
	if !running {
		view, err := p.startWithContext(r.Context(), id, false)
		if err != nil {
			proxyAppError(w, "アプリを起動できませんでした", err, http.StatusServiceUnavailable, tailnet)
			return
		}
		p.emitStatus(view)
	}
	port, err := p.waitReady(r.Context(), id, readyWait)
	if err != nil {
		proxyAppError(w, "アプリの起動を確認できませんでした", err, http.StatusGatewayTimeout, tailnet)
		return
	}
	p.beginProxy(id)
	defer p.endProxy(id)
	backend, _ := url.Parse(loopbackURL(loopbackHost(port), port))
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(backend)
			if tailnet {
				// Pass the public URL in forwarding headers so a local dev server can
				// keep its loopback Host checks enabled.
				pr.SetXForwarded()
				for header := range pr.Out.Header {
					if strings.HasPrefix(strings.ToLower(header), "tailscale-") {
						delete(pr.Out.Header, header)
					}
				}
			} else {
				pr.Out.Host = pr.In.Host
			}
		},
		FlushInterval: 100 * time.Millisecond,
		ErrorHandler: func(rw http.ResponseWriter, _ *http.Request, proxyErr error) {
			proxyAppError(rw, "転送に失敗しました", proxyErr, http.StatusBadGateway, tailnet)
		},
	}
	if tailnet {
		proxy.ModifyResponse = func(response *http.Response) error {
			location, err := url.Parse(response.Header.Get("Location"))
			if err == nil && location.Scheme == backend.Scheme && location.Host == backend.Host && location.User == nil {
				location.Scheme = "https"
				location.Host = r.Host
				response.Header.Set("Location", location.String())
			}
			return nil
		}
	}
	proxy.ServeHTTP(w, r)
}

func proxyAppError(w http.ResponseWriter, message string, err error, status int, tailnet bool) {
	if !tailnet {
		if status == http.StatusBadGateway {
			message += "\n" + err.Error() + "\n"
		} else {
			message = err.Error() + "\n"
		}
	}
	http.Error(w, "DevPortal: "+message, status)
}

func (p *Portal) waitReady(ctx context.Context, id string, timeout time.Duration) (uint16, error) {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	var port uint16
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		inner := p.lock()
		rt, ok := inner.runtime[id]
		errMsg := inner.errors[id]
		if !ok {
			p.unlock()
			if errMsg != "" {
				return 0, errString(errMsg)
			}
			return 0, errString("プロセスが起動していません")
		}
		port = rt.port
		ready := rt.ready
		p.unlock()
		if ready || isOpen(port) {
			return port, nil
		}
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-ticker.C:
		}
	}
	return 0, errString(fmt.Sprintf("ポート %d の起動が時間切れになりました", port))
}
