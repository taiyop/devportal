package main

import (
	"fmt"
	"net"
	"time"
)

const (
	loopbackV4 = "127.0.0.1"
	loopbackV6 = "::1"
)

func loopbackHosts() []string {
	return []string{loopbackV4, loopbackV6}
}

func loopbackAddr(host string, port uint16) string {
	return net.JoinHostPort(host, itoaPort(port))
}

func loopbackURL(host string, port uint16) string {
	if host == "" {
		host = loopbackV4
	}
	return "http://" + loopbackAddr(host, port)
}

func openLoopback(port uint16, timeout time.Duration) (net.Conn, string, error) {
	var last error
	for _, host := range loopbackHosts() {
		conn, err := net.DialTimeout("tcp", loopbackAddr(host, port), timeout)
		if err == nil {
			return conn, host, nil
		}
		last = err
	}
	if last == nil {
		last = errString("loopback is closed")
	}
	return nil, "", last
}

func loopbackHost(port uint16) string {
	conn, host, err := openLoopback(port, 120*time.Millisecond)
	if err != nil {
		return ""
	}
	_ = conn.Close()
	return host
}

func loopbackListenFree(host string, port uint16) (free bool, available bool) {
	ln, err := net.Listen("tcp", loopbackAddr(host, port))
	if err != nil {
		if listenBusy(err) {
			return false, true
		}
		return true, false
	}
	_ = ln.Close()
	return true, true
}

func isFree(port uint16) bool {
	v4free, v4ok := loopbackListenFree(loopbackV4, port)
	if !v4ok || !v4free {
		return false
	}
	v6free, v6ok := loopbackListenFree(loopbackV6, port)
	if v6ok && !v6free {
		return false
	}
	return true
}

func waitFree(port uint16, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		if isFree(port) {
			return true
		}
		if time.Now().After(deadline) || time.Now().Equal(deadline) {
			return isFree(port)
		}
		time.Sleep(30 * time.Millisecond)
	}
}

func findFreePort() (uint16, error) {
	return findFreePortExcept()
}

func findFreePortExcept(exclude ...uint16) (uint16, error) {
	blocked := map[uint16]struct{}{}
	for _, port := range exclude {
		if port != 0 {
			blocked[port] = struct{}{}
		}
	}
	for i := 0; i < 48; i++ {
		ln, err := net.Listen("tcp", net.JoinHostPort(loopbackV4, "0"))
		if err != nil {
			return 0, fmt.Errorf("空きポートを取得できませんでした: %w", err)
		}
		addr, ok := ln.Addr().(*net.TCPAddr)
		if !ok {
			_ = ln.Close()
			return 0, errString("空きポートを取得できませんでした")
		}
		port := uint16(addr.Port)
		if _, skip := blocked[port]; skip {
			_ = ln.Close()
			continue
		}
		v6free, v6ok := loopbackListenFree(loopbackV6, port)
		_ = ln.Close()
		if v6ok && !v6free {
			continue
		}
		return port, nil
	}
	return 0, errString("空きポートを取得できませんでした")
}

func portExcluded(port uint16, exclude []uint16) bool {
	for _, item := range exclude {
		if item != 0 && item == port {
			return true
		}
	}
	return false
}

func resolveListenPort(mode PortMode, preferred *uint16, exclude ...uint16) (uint16, error) {
	switch mode {
	case PortModeManual:
		if preferred == nil || *preferred == 0 {
			return 0, errString("手動モードではポート番号が必要です")
		}
		port := *preferred
		if portExcluded(port, exclude) {
			return 0, errString(fmt.Sprintf("ポート %d はすでに使用中です", port))
		}
		if !isFree(port) && !waitFree(port, 5*time.Second) {
			return 0, errString(fmt.Sprintf("ポート %d はすでに使用中です", port))
		}
		return port, nil
	default:
		if preferred != nil && *preferred != 0 && !portExcluded(*preferred, exclude) && (isFree(*preferred) || waitFree(*preferred, 5*time.Second)) {
			return *preferred, nil
		}
		return findFreePortExcept(exclude...)
	}
}

func isOpen(port uint16) bool {
	conn, _, err := openLoopback(port, 120*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
