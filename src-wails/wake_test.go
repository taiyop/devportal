package main

import (
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

func TestSplicesClientToBackend(t *testing.T) {
	backend, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	backendPort := uint16(backend.Addr().(*net.TCPAddr).Port)
	go func() {
		conn, err := backend.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 4)
		if _, err := io.ReadFull(conn, buf); err != nil {
			return
		}
		_, _ = conn.Write([]byte("pong"))
	}()

	proxy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Close()
	proxyAddr := proxy.Addr().String()
	go func() {
		client, err := proxy.Accept()
		if err != nil {
			return
		}
		_ = spliceToPort(client, backendPort, 2*time.Second)
	}()

	client, err := net.Dial("tcp", proxyAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if _, err := client.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 4)
	if _, err := io.ReadFull(client, buf); err != nil {
		t.Fatal(err)
	}
	if string(buf) != "pong" {
		t.Fatalf("got %q", buf)
	}
}

func TestSplicesClientToIPv6Backend(t *testing.T) {
	backend, err := net.Listen("tcp", net.JoinHostPort(loopbackV6, "0"))
	if err != nil {
		t.Skip("IPv6 loopback is not available")
	}
	defer backend.Close()
	backendPort := uint16(backend.Addr().(*net.TCPAddr).Port)
	go func() {
		conn, err := backend.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 4)
		if _, err := io.ReadFull(conn, buf); err != nil {
			return
		}
		_, _ = conn.Write([]byte("pong"))
	}()

	proxy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Close()
	proxyAddr := proxy.Addr().String()
	go func() {
		client, err := proxy.Accept()
		if err != nil {
			return
		}
		_ = spliceToPort(client, backendPort, 2*time.Second)
	}()

	client, err := net.Dial("tcp", proxyAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if _, err := client.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 4)
	if _, err := io.ReadFull(client, buf); err != nil {
		t.Fatal(err)
	}
	if string(buf) != "pong" {
		t.Fatalf("got %q", buf)
	}
}

func TestHandsOffSamePortToBackend(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := uint16(ln.Addr().(*net.TCPAddr).Port)
	addr := ln.Addr().String()

	go func() {
		client, err := ln.Accept()
		if err != nil {
			return
		}
		_ = ln.Close()
		backend, err := net.Listen("tcp", addr)
		if err != nil {
			return
		}
		defer backend.Close()
		go func() {
			conn, err := backend.Accept()
			if err != nil {
				return
			}
			defer conn.Close()
			buf := make([]byte, 4)
			if _, err := io.ReadFull(conn, buf); err != nil {
				return
			}
			_, _ = conn.Write([]byte("pong"))
		}()
		_ = spliceToPort(client, port, 2*time.Second)
	}()

	time.Sleep(80 * time.Millisecond)
	client, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if _, err := client.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 4)
	if _, err := io.ReadFull(client, buf); err != nil {
		t.Fatal(err)
	}
	if string(buf) != "pong" {
		t.Fatalf("got %q", buf)
	}
}

func TestWritesHTTPErrorWhenBackendMissing(t *testing.T) {
	proxy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Close()
	missing, err := findFreePort()
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		client, err := proxy.Accept()
		if err != nil {
			return
		}
		_ = spliceToPort(client, missing, 200*time.Millisecond)
	}()

	client, err := net.Dial("tcp", proxy.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	body, err := io.ReadAll(client)
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	if !strings.Contains(s, "503") || !strings.Contains(s, "時間切れ") {
		t.Fatalf("got %q", s)
	}
}
