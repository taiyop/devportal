package main

import (
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

const defaultGatewayPort uint16 = 80

var fallbackGatewayPorts = []uint16{80, 7341, 8080, 8888, 18080}

type GatewayStatus struct {
	Addr          string `json:"addr"`
	Port          uint16 `json:"port"`
	PreferredPort uint16 `json:"preferredPort"`
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

func (p *Portal) preferredGatewayPort() uint16 {
	inner := p.lock()
	defer p.unlock()
	return preferredGatewayPort(inner.file)
}

func (p *Portal) StartGateway() error {
	return p.listenFirstAvailable(listenAddrsFor(p.preferredGatewayPort()))
}

func (p *Portal) SetGatewayPort(port uint16) error {
	next := normalizeGatewayPort(port)
	inner := p.lock()
	prev := preferredGatewayPort(inner.file)
	stored := next
	inner.file.GatewayPort = &stored
	err := saveConfig(inner.configPath, inner.file)
	p.unlock()
	if err != nil {
		return err
	}

	p.gatewayMu.Lock()
	running := p.gateway != nil
	p.gatewayMu.Unlock()
	if running && prev != next {
		p.StopGateway()
		return p.StartGateway()
	}
	p.emitGateway()
	return nil
}

func (p *Portal) listenAndServeGateway(addr string) error {
	return p.listenFirstAvailable([]string{addr})
}

func (p *Portal) listenFirstAvailable(addrs []string) error {
	if len(addrs) == 0 {
		addrs = listenAddrsFor(defaultGatewayPort)
	}
	p.gatewayMu.Lock()
	if p.gateway != nil {
		p.gatewayMu.Unlock()
		return nil
	}

	var lastErr error
	preferred := listenPortOf(addrs[0])
	if preferred == 0 {
		preferred = int(defaultGatewayPort)
	}
	deniedPreferred := false
	busyPreferred := false
	for i, addr := range addrs {
		ln, ln4, err := listenGateway(addr)
		if err != nil {
			lastErr = err
			if listenPortOf(addr) == preferred {
				deniedPreferred = deniedPreferred || listenDenied(err)
				busyPreferred = busyPreferred || listenBusy(err)
			}
			if i < len(addrs)-1 && shouldTryNextListen(err) {
				continue
			}
			msg := fmt.Sprintf("%s で待ち受けできません: %v", addr, err)
			p.gatewayErr = msg
			p.gatewayMu.Unlock()
			p.emitGateway()
			return errString(msg)
		}

		port := tcpListenPort(ln)
		note := ""
		if int(port) != preferred {
			switch {
			case deniedPreferred && preferred == int(defaultGatewayPort):
				note = fmt.Sprintf("ポート %d は管理者権限が必要なため、:%d で待ち受けています。IPv4 と IPv6 の両方で受けます。", preferred, port)
			case deniedPreferred:
				note = fmt.Sprintf("ポート %d は権限が必要なため、:%d で待ち受けています。", preferred, port)
			case busyPreferred:
				note = fmt.Sprintf("ポート %d は使用中のため、:%d で待ち受けています。", preferred, port)
			default:
				note = fmt.Sprintf(":%d で待ち受けています。", port)
			}
		}
		err = p.serveGatewayLocked(ln, ln4, port, note)
		p.gatewayMu.Unlock()
		if err != nil {
			return err
		}
		p.syncInnerGatewayPort(port)
		p.emitGateway()
		p.emitAllViews()
		return nil
	}

	msg := "リバースプロキシを開始できません"
	if lastErr != nil {
		msg = fmt.Sprintf("リバースプロキシを開始できません: %v", lastErr)
	}
	p.gatewayErr = msg
	p.gatewayMu.Unlock()
	p.emitGateway()
	return errString(msg)
}

func (p *Portal) serveGatewayLocked(ln, ln4 net.Listener, port uint16, note string) error {
	srv := &http.Server{
		Addr:              ln.Addr().String(),
		Handler:           http.HandlerFunc(p.handleGateway),
		ReadHeaderTimeout: 10 * time.Second,
	}
	p.gateway = srv
	p.gatewayLn = ln
	p.gatewayLn4 = ln4
	p.gatewayListen = ln.Addr().String()
	p.gatewayPort = port
	p.gatewayNote = note
	p.gatewayErr = ""

	go func() { _ = srv.Serve(ln) }()
	if ln4 != nil {
		go func() { _ = srv.Serve(ln4) }()
	}
	return nil
}

func (p *Portal) StopGateway() {
	p.gatewayMu.Lock()
	srv := p.gateway
	ln := p.gatewayLn
	ln4 := p.gatewayLn4
	p.gateway = nil
	p.gatewayLn = nil
	p.gatewayLn4 = nil
	p.gatewayListen = ""
	p.gatewayPort = 0
	p.gatewayNote = ""
	p.gatewayErr = ""
	p.gatewayMu.Unlock()
	if ln != nil {
		_ = ln.Close()
	}
	if ln4 != nil {
		_ = ln4.Close()
	}
	if srv != nil {
		_ = srv.Close()
	}
	p.syncInnerGatewayPort(0)
	p.emitGateway()
	p.emitAllViews()
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

func listenGateway(addr string) (net.Listener, net.Listener, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, nil, err
	}
	ta, ok := ln.Addr().(*net.TCPAddr)
	if !ok || ta.IP.To4() != nil || !ta.IP.IsUnspecified() {
		return ln, nil, nil
	}
	ln4, err4 := net.Listen("tcp4", fmt.Sprintf("0.0.0.0:%d", ta.Port))
	if err4 != nil {
		return ln, nil, nil
	}
	return ln, ln4, nil
}

func (p *Portal) GatewayStatus() GatewayStatus {
	preferred := p.preferredGatewayPort()
	p.gatewayMu.Lock()
	defer p.gatewayMu.Unlock()
	return p.gatewaySnapshot(preferred)
}

func (p *Portal) emitGateway() {
	preferred := p.preferredGatewayPort()
	p.gatewayMu.Lock()
	status := p.gatewaySnapshot(preferred)
	p.gatewayMu.Unlock()
	if p.EmitGateway != nil {
		p.EmitGateway(status)
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

func (p *Portal) gatewaySnapshot(preferred uint16) GatewayStatus {
	preferred = normalizeGatewayPort(preferred)
	port := p.gatewayPort
	if port == 0 {
		port = preferred
	}
	return GatewayStatus{
		Addr:          "http://" + displayListenAddr(p.gatewayListen, port),
		Port:          port,
		PreferredPort: preferred,
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

func (p *Portal) handleGateway(w http.ResponseWriter, r *http.Request) {
	label := hostLabelFromRequest(r.Host)
	if label == "" {
		http.Error(w, "DevPortal: Host ヘッダが *.devportal.localhost ではありません\n", http.StatusBadRequest)
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
	if !running {
		view, err := p.start(entry.ID, false)
		if err != nil {
			http.Error(w, "DevPortal: "+err.Error()+"\n", http.StatusServiceUnavailable)
			return
		}
		p.emitStatus(view)
	}
	port, err := p.waitReady(entry.ID, readyWait)
	if err != nil {
		http.Error(w, "DevPortal: "+err.Error()+"\n", http.StatusGatewayTimeout)
		return
	}
	p.beginProxy(entry.ID)
	defer p.endProxy(entry.ID)
	backend, _ := url.Parse(loopbackURL(loopbackHost(port), port))
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(backend)
			pr.Out.Host = pr.In.Host
		},
		FlushInterval: 100 * time.Millisecond,
		ErrorHandler: func(rw http.ResponseWriter, _ *http.Request, proxyErr error) {
			http.Error(rw, "DevPortal: 転送に失敗しました\n"+proxyErr.Error()+"\n", http.StatusBadGateway)
		},
	}
	proxy.ServeHTTP(w, r)
}

func (p *Portal) waitReady(id string, timeout time.Duration) (uint16, error) {
	deadline := time.Now().Add(timeout)
	var port uint16
	for time.Now().Before(deadline) {
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
		time.Sleep(50 * time.Millisecond)
	}
	return 0, errString(fmt.Sprintf("ポート %d の起動が時間切れになりました", port))
}
