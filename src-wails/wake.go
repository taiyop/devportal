package main

import (
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

type wakeSlot struct {
	stop atomic.Bool
	port uint16
	ln   net.Listener
}

var (
	wakeMu    sync.Mutex
	wakeSlots = map[string]*wakeSlot{}
)

const readyWait = 20 * time.Second

func armedPort(id string) (uint16, bool) {
	wakeMu.Lock()
	defer wakeMu.Unlock()
	slot, ok := wakeSlots[id]
	if !ok {
		return 0, false
	}
	return slot.port, true
}

func disarm(id string) {
	wakeMu.Lock()
	slot, ok := wakeSlots[id]
	if ok {
		delete(wakeSlots, id)
	}
	wakeMu.Unlock()
	if ok {
		slot.stop.Store(true)
		_ = slot.ln.Close()
	}
}

func disarmAll() {
	wakeMu.Lock()
	slots := wakeSlots
	wakeSlots = map[string]*wakeSlot{}
	wakeMu.Unlock()
	for _, slot := range slots {
		slot.stop.Store(true)
		_ = slot.ln.Close()
	}
}

func arm(portal *Portal, id string, port uint16) error {
	disarm(id)
	if !waitFree(port, 2*time.Second) {
		return errString(fmt.Sprintf("ポート %d を待ち受け用に確保できませんでした（他のプロセスが使用中）", port))
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return fmt.Errorf("ポート %d で待ち受けできません: %w", port, err)
	}
	slot := &wakeSlot{port: port, ln: ln}
	wakeMu.Lock()
	wakeSlots[id] = slot
	wakeMu.Unlock()
	go runListener(portal, id, ln, slot)
	return nil
}

func runListener(portal *Portal, id string, ln net.Listener, slot *wakeSlot) {
	for {
		if slot.stop.Load() {
			return
		}
		if tcp, ok := ln.(*net.TCPListener); ok {
			_ = tcp.SetDeadline(time.Now().Add(40 * time.Millisecond))
		}
		conn, err := ln.Accept()
		if err != nil {
			if slot.stop.Load() {
				return
			}
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			wakeMu.Lock()
			if wakeSlots[id] == slot {
				delete(wakeSlots, id)
			}
			wakeMu.Unlock()
			return
		}
		pending := []net.Conn{conn}
		if tcp, ok := ln.(*net.TCPListener); ok {
			_ = tcp.SetDeadline(time.Now().Add(20 * time.Millisecond))
			for {
				extra, extraErr := ln.Accept()
				if extraErr != nil {
					break
				}
				pending = append(pending, extra)
			}
		}
		wakeMu.Lock()
		delete(wakeSlots, id)
		wakeMu.Unlock()
		_ = ln.Close()
		portal.StartAndForward(id, pending)
		return
	}
}

func forward(client net.Conn, port uint16) {
	go func() {
		_ = spliceToPort(client, port, readyWait)
	}()
}

func respondUnavailable(client net.Conn, message string) {
	body := "DevPortal: アプリを起動できませんでした\n" + message + "\n"
	response := fmt.Sprintf(
		"HTTP/1.1 503 Service Unavailable\r\nContent-Type: text/plain; charset=utf-8\r\nConnection: close\r\nContent-Length: %d\r\n\r\n%s",
		len(body),
		body,
	)
	if tcp, ok := client.(*net.TCPConn); ok {
		_ = tcp.SetNoDelay(true)
	}
	_, _ = io.WriteString(client, response)
	_ = client.Close()
}

func spliceToPort(client net.Conn, port uint16, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var server net.Conn
	for {
		conn, _, err := openLoopback(port, 50*time.Millisecond)
		if err == nil {
			server = conn
			break
		}
		if time.Now().After(deadline) {
			respondUnavailable(client, fmt.Sprintf("ポート %d の起動が時間切れになりました", port))
			return errString("timeout")
		}
		time.Sleep(50 * time.Millisecond)
	}
	if tcp, ok := client.(*net.TCPConn); ok {
		_ = tcp.SetNoDelay(true)
	}
	if tcp, ok := server.(*net.TCPConn); ok {
		_ = tcp.SetNoDelay(true)
	}
	splice(client, server)
	return nil
}

func splice(left, right net.Conn) {
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(right, left)
		closeWrite(right)
		close(done)
	}()
	_, _ = io.Copy(left, right)
	closeWrite(left)
	<-done
	_ = left.Close()
	_ = right.Close()
}

func closeWrite(conn net.Conn) {
	if tcp, ok := conn.(*net.TCPConn); ok {
		_ = tcp.CloseWrite()
		return
	}
	_ = conn.Close()
}
