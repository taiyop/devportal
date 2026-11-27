package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"tailscale.com/client/local"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/tsnet"
)

const tailnetFirstAppPort = 20000

type TailnetConfig struct {
	TailnetSettings `yaml:",inline"`
	NextPort        int `yaml:"nextPort,omitempty"`
}

type TailnetSettings struct {
	Enabled   bool                          `json:"enabled" yaml:"enabled,omitempty"`
	DeviceIDs []string                      `json:"deviceIds" yaml:"deviceIds,omitempty"`
	Apps      map[string]TailnetAppSettings `json:"apps" yaml:"apps,omitempty"`
}

type TailnetAppSettings struct {
	Enabled         bool     `json:"enabled" yaml:"enabled,omitempty"`
	Port            uint16   `json:"port" yaml:"port"`
	RestrictDevices bool     `json:"restrictDevices" yaml:"restrictDevices,omitempty"`
	DeviceIDs       []string `json:"deviceIds" yaml:"deviceIds,omitempty"`
}

type TailnetDevice struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	DNS    string `json:"dns"`
	Online bool   `json:"online"`
}

type TailnetStatus struct {
	State     string          `json:"state"`
	PortalURL string          `json:"portalUrl"`
	LoginURL  string          `json:"loginUrl"`
	Error     string          `json:"error"`
	Devices   []TailnetDevice `json:"devices"`
}

func (p *Portal) tailnetSettings() TailnetSettings {
	inner := p.lock()
	defer p.unlock()
	return inner.file.Tailnet.TailnetSettings
}

func (p *Portal) setTailnetSettings(input TailnetSettings) (TailnetSettings, error) {
	inner := p.lock()
	file := inner.file
	current := file.Tailnet
	next := TailnetConfig{TailnetSettings: TailnetSettings{
		Enabled:   input.Enabled,
		DeviceIDs: normalizeDeviceIDs(input.DeviceIDs),
		Apps:      make(map[string]TailnetAppSettings),
	}, NextPort: max(current.NextPort, tailnetFirstAppPort)}
	used := make(map[uint16]bool, len(current.Apps))
	for _, app := range current.Apps {
		used[app.Port] = true
	}
	for _, entry := range file.Apps {
		app := input.Apps[entry.ID]
		app.Port = current.Apps[entry.ID].Port
		if app.Port < tailnetFirstAppPort {
			if !app.Enabled {
				continue
			}
			for next.NextPort <= 65535 && used[uint16(next.NextPort)] {
				next.NextPort++
			}
			if next.NextPort > 65535 {
				p.unlock()
				return TailnetSettings{}, errString("Tailscale 公開用のポートを割り当てられません")
			}
			app.Port = uint16(next.NextPort)
			next.NextPort++
		}
		app.DeviceIDs = normalizeDeviceIDs(app.DeviceIDs)
		next.Apps[entry.ID] = app
	}
	file.Tailnet = next
	if err := saveConfig(inner.configPath, file); err != nil {
		p.unlock()
		return TailnetSettings{}, err
	}
	inner.file = file
	inner.invalidateTailnetPolicy()
	p.unlock()
	p.syncTailnet()
	return next.TailnetSettings, nil
}

func normalizeDeviceIDs(ids []string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id := strings.TrimSpace(id); id != "" {
			out = append(out, id)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

func (inner *Inner) invalidateTailnetPolicy() {
	if inner.cancelTailnetPolicy != nil {
		inner.cancelTailnetPolicy()
	}
	inner.tailnetPolicy, inner.cancelTailnetPolicy = context.WithCancel(context.Background())
}

type tailnetSession struct {
	cancel context.CancelFunc
	done   chan struct{}
	reload chan struct{}
	mu     sync.Mutex
	status TailnetStatus
}

type tailnetListener struct {
	listener net.Listener
	server   *http.Server
	cancel   context.CancelFunc
}

func (l tailnetListener) close() {
	l.cancel()
	_ = l.listener.Close()
	_ = l.server.Close()
}

func (p *Portal) syncTailnet() {
	p.tailnetMu.Lock()
	defer p.tailnetMu.Unlock()

	inner := p.lock()
	enabled := inner.file.Tailnet.Enabled
	dir := filepath.Join(filepath.Dir(inner.configPath), "tailscale")
	p.unlock()
	if !enabled || p.tailnetClosed {
		p.closeTailnetSession()
		return
	}
	if p.tailnet != nil {
		select {
		case p.tailnet.reload <- struct{}{}:
		default:
		}
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	session := &tailnetSession{
		cancel: cancel,
		done:   make(chan struct{}),
		reload: make(chan struct{}, 1),
		status: TailnetStatus{State: "Starting"},
	}
	p.tailnet = session
	go p.runTailnet(ctx, session, dir)
}

// The caller holds tailnetMu. The session worker only takes its own mutex and p.mu.
func (p *Portal) closeTailnetSession() {
	if session := p.tailnet; session != nil {
		session.cancel()
		<-session.done
		p.tailnet = nil
	}
}

func (p *Portal) stopTailnet() {
	p.tailnetMu.Lock()
	defer p.tailnetMu.Unlock()
	p.tailnetClosed = true
	inner := p.lock()
	if inner.cancelTailnetPolicy != nil {
		inner.cancelTailnetPolicy()
	}
	p.unlock()
	p.closeTailnetSession()
}

func (p *Portal) tailnetStatus() TailnetStatus {
	p.tailnetMu.Lock()
	defer p.tailnetMu.Unlock()
	if p.tailnet == nil {
		return TailnetStatus{State: "Disabled"}
	}
	p.tailnet.mu.Lock()
	defer p.tailnet.mu.Unlock()
	return p.tailnet.status
}

func (s *tailnetSession) setStatus(status TailnetStatus) {
	s.mu.Lock()
	s.status = status
	s.mu.Unlock()
}

func (p *Portal) runTailnet(ctx context.Context, session *tailnetSession, dir string) {
	defer close(session.done)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		session.setStatus(TailnetStatus{State: "Error", Error: err.Error()})
		return
	}
	node := &tsnet.Server{
		Dir:      dir,
		Hostname: "devportal",
		// The Local API reports the login URL to the settings window.
		UserLogf: func(string, ...any) {},
		Logf:     func(string, ...any) {},
	}
	defer node.Close()
	client, err := node.LocalClient()
	if err != nil {
		session.setStatus(TailnetStatus{State: "Error", Error: err.Error()})
		return
	}

	servers := make(map[uint16]tailnetListener)
	defer closeTailnetServers(servers)
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	var currentHost string
	var certError error
	var certRetry time.Time
	for ctx.Err() == nil {
		status, host, suffix := readTailnetStatus(ctx, client)
		if host != currentHost {
			closeTailnetServers(servers)
			currentHost = host
			certRetry = time.Time{}
			certError = nil
		}
		if host != "" && ctx.Err() == nil {
			if certRetry.IsZero() || (certError != nil && time.Now().After(certRetry)) {
				connecting := status
				connecting.State = "Certifying"
				session.setStatus(connecting)
				certError = prepareTailnetCertificate(ctx, client, host)
				certRetry = time.Now().Add(30 * time.Second)
			}
			if certError != nil {
				status.Error = fmt.Sprintf("HTTPS 証明書を取得できませんでした: %v", certError)
			} else if ctx.Err() == nil {
				status.PortalURL, status.Error = p.syncTailnetListeners(ctx, node, client, host, suffix, servers)
			}
		}
		session.setStatus(status)
		select {
		case <-ctx.Done():
		case <-session.reload:
		case <-ticker.C:
		}
	}
}

func prepareTailnetCertificate(ctx context.Context, client *local.Client, host string) error {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	_, _, err := client.CertPair(ctx, host)
	return err
}

func readTailnetStatus(ctx context.Context, client *local.Client) (TailnetStatus, string, string) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	st, err := client.Status(ctx)
	if err != nil {
		return TailnetStatus{State: "Error", Error: err.Error()}, "", ""
	}
	status := TailnetStatus{State: st.BackendState, LoginURL: st.AuthURL}
	if status.State != "Running" || st.CurrentTailnet == nil || st.Self == nil {
		return status, "", ""
	}
	suffix := strings.Trim(strings.ToLower(st.CurrentTailnet.MagicDNSSuffix), ".")
	status.Devices = tailnetDevices(st, suffix)
	host := strings.TrimSuffix(strings.ToLower(st.Self.DNSName), ".")
	if !st.CurrentTailnet.MagicDNSEnabled || !slices.Contains(st.CertDomains, host) || !tailnetMemberName(host, suffix) {
		status.Error = "Tailscale 管理コンソールで MagicDNS と HTTPS 証明書を有効にしてください"
		return status, "", ""
	}
	return status, host, suffix
}

func tailnetDevices(st *ipnstate.Status, suffix string) []TailnetDevice {
	devices := make([]TailnetDevice, 0, len(st.Peer))
	for _, peer := range st.Peer {
		if peer == nil || !peer.InNetworkMap || peer.ShareeNode || peer.ID == "" || !tailnetMemberName(peer.DNSName, suffix) {
			continue
		}
		name := peer.HostName
		if name == "" {
			name, _, _ = strings.Cut(peer.DNSName, ".")
		}
		devices = append(devices, TailnetDevice{
			ID: string(peer.ID), Name: name, DNS: strings.TrimSuffix(peer.DNSName, "."), Online: peer.Online,
		})
	}
	sort.Slice(devices, func(i, j int) bool { return devices[i].DNS < devices[j].DNS })
	return devices
}

func tailnetMemberName(name, suffix string) bool {
	label, domain, ok := strings.Cut(strings.TrimSuffix(strings.ToLower(name), "."), ".")
	return ok && label != "" && suffix != "" && domain == suffix
}

func resolveTailnetDevice(ctx context.Context, client *local.Client, remoteAddr, suffix string) (string, error) {
	addr, err := netip.ParseAddrPort(remoteAddr)
	if err != nil {
		return "", err
	}
	who, err := client.WhoIs(ctx, remoteAddr)
	if err != nil {
		return "", err
	}
	if who.Node != nil && who.Node.StableID != "" && tailnetMemberName(who.Node.Name, suffix) &&
		(!who.Node.Hostinfo.Valid() || !who.Node.Hostinfo.ShareeNode()) {
		for _, prefix := range who.Node.Addresses {
			if prefix.IsSingleIP() && prefix.Addr().Unmap() == addr.Addr().Unmap() {
				return string(who.Node.StableID), nil
			}
		}
	}
	return "", errors.New("tailnet device could not be verified")
}

func closeTailnetServers(servers map[uint16]tailnetListener) {
	for port, server := range servers {
		server.close()
		delete(servers, port)
	}
}

func (p *Portal) syncTailnetListeners(ctx context.Context, node *tsnet.Server, client *local.Client, host, suffix string, servers map[uint16]tailnetListener) (string, string) {
	settings := p.tailnetSettings()
	if !settings.Enabled {
		closeTailnetServers(servers)
		return "", ""
	}
	wanted := map[uint16]string{443: ""}
	for id, app := range settings.Apps {
		if app.Enabled && app.Port >= tailnetFirstAppPort {
			wanted[app.Port] = id
		}
	}
	for port, server := range servers {
		if _, keep := wanted[port]; !keep {
			server.close()
			delete(servers, port)
		}
	}

	lookup := func(ctx context.Context, addr string) (string, error) {
		return resolveTailnetDevice(ctx, client, addr, suffix)
	}
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12, GetCertificate: client.GetCertificate}
	var listenErr error
	for port, id := range wanted {
		if _, exists := servers[port]; exists {
			continue
		}
		ln, err := node.Listen("tcp", ":"+strconv.Itoa(int(port)))
		if err != nil {
			listenErr = errors.Join(listenErr, fmt.Errorf("ポート %d: %w", port, err))
			continue
		}
		var handler http.Handler
		if port == 443 {
			handler = p.tailnetPortal(host, lookup)
		} else {
			handler = p.tailnetApp(host, port, id, lookup)
		}
		listenerCtx, cancel := context.WithCancel(ctx)
		server := &http.Server{
			Handler:           handler,
			ReadHeaderTimeout: 10 * time.Second,
			IdleTimeout:       60 * time.Second,
			BaseContext:       func(net.Listener) context.Context { return listenerCtx },
		}
		servers[port] = tailnetListener{listener: ln, server: server, cancel: cancel}
		go func() {
			defer ln.Close()
			_ = server.Serve(tls.NewListener(ln, tlsConfig))
		}()
	}
	var portalURL, message string
	if _, exists := servers[443]; exists {
		portalURL = (&url.URL{Scheme: "https", Host: host}).String()
	}
	if listenErr != nil {
		message = listenErr.Error()
	}
	return portalURL, message
}
