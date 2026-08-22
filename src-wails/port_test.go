package main

import (
	"net"
	"testing"
	"time"
)

func listenLoopback(t *testing.T, host string) (net.Listener, uint16) {
	t.Helper()
	ln, err := net.Listen("tcp", net.JoinHostPort(host, "0"))
	if err != nil {
		if host == loopbackV6 {
			t.Skip("IPv6 loopback is not available")
		}
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatal("expected TCP addr")
	}
	return ln, uint16(addr.Port)
}

func TestFindFreePort(t *testing.T) {
	port, err := findFreePort()
	if err != nil {
		t.Fatal(err)
	}
	if port == 0 {
		t.Fatal("expected non-zero port")
	}
}

func TestWaitFreeAfterDrop(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := uint16(ln.Addr().(*net.TCPAddr).Port)
	if isFree(port) {
		t.Fatal("port should be in use")
	}
	_ = ln.Close()
	if !waitFree(port, time.Second) {
		t.Fatal("port should become free")
	}
}

func TestIsOpenSeesIPv4Listen(t *testing.T) {
	_, port := listenLoopback(t, loopbackV4)
	if !isOpen(port) {
		t.Fatal("expected IPv4 listener to count as open")
	}
	if isFree(port) {
		t.Fatal("expected IPv4 listener to occupy the port")
	}
	if host := loopbackHost(port); host != loopbackV4 {
		t.Fatalf("host %q", host)
	}
}

func TestIsOpenSeesIPv6OnlyListen(t *testing.T) {
	_, port := listenLoopback(t, loopbackV6)
	if !isOpen(port) {
		t.Fatal("expected IPv6-only listener to count as open")
	}
	if isFree(port) {
		t.Fatal("expected IPv6-only listener to occupy the port")
	}
	host := loopbackHost(port)
	if host != loopbackV6 {
		t.Fatalf("host %q", host)
	}
	wantURL := "http://[" + loopbackV6 + "]:" + itoaPort(port)
	if loopbackURL(host, port) != wantURL {
		t.Fatalf("url %s", loopbackURL(host, port))
	}
}
