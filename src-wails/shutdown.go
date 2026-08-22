package main

import (
	"fmt"
	"io"
	"os"
	"sort"
	"time"
)

const (
	shutdownTermWait = 8 * time.Second
	shutdownKillWait = 2 * time.Second
)

type shutdownItem struct {
	id   string
	name string
	pid  uint32
	pgid int
	port uint16
	rt   *Runtime
}

func (p *Portal) ShutdownFromSignal() {
	p.shutdownOnce.Do(func() {
		p.shutdownAll(os.Stderr)
	})
}

func (p *Portal) shutdownAll(w io.Writer) {
	if w == nil {
		w = io.Discard
	}
	gw := p.GatewayStatus()
	if gw.Running {
		shutdownLog(w, "リバースプロキシ %s を停止します。", gw.Addr)
	}
	p.StopGateway()

	items := p.takeRunning()
	if len(items) == 0 {
		shutdownLog(w, "起動中のプロセスはありません。")
		return
	}

	shutdownLog(w, "起動中のプロセス %d 件:", len(items))
	for _, item := range items {
		shutdownLog(w, "  • %-24s  pid %-7d  pgid %-7d  port %d", item.name, item.pid, item.pgid, item.port)
	}
	shutdownLog(w, "")
	shutdownLog(w, "終了を待っています...")

	for _, item := range items {
		go func(rt *Runtime) { _ = rt.wait() }(item.rt)
		if item.rt.keepalive != nil {
			_ = item.rt.keepalive.Close()
			item.rt.keepalive = nil
		}
		signalTerm(item.rt.cmd, item.rt.pgid)
		shutdownLog(w, "  [..] %-24s  終了を要求しました", item.name)
	}

	pending := make([]*shutdownItem, 0, len(items))
	for i := range items {
		pending = append(pending, &items[i])
	}

	pending = waitPending(w, pending, shutdownTermWait, false)
	if len(pending) > 0 {
		shutdownLog(w, "")
		shutdownLog(w, "まだ終了していないプロセスを強制終了します...")
		for _, item := range pending {
			signalKill(item.rt.cmd, item.rt.pgid)
			shutdownLog(w, "  [!!] %-24s  強制終了を送りました", item.name)
		}
		pending = waitPending(w, pending, shutdownKillWait, true)
	}

	if len(pending) == 0 {
		shutdownLog(w, "")
		shutdownLog(w, "すべてのプロセスが終了しました。")
		return
	}
	shutdownLog(w, "")
	shutdownLog(w, "終了を確認できないプロセスが残っています:")
	for _, item := range pending {
		shutdownLog(w, "  [xx] %-24s  pid %d", item.name, item.pid)
	}
}

func (p *Portal) takeRunning() []shutdownItem {
	inner := p.lock()
	runPath := inner.runningPath
	runtimes := inner.runtime
	inner.runtime = map[string]*Runtime{}
	items := make([]shutdownItem, 0, len(runtimes))
	for id, rt := range runtimes {
		name := id
		if entry, err := findEntry(inner, id); err == nil && entry.Name != "" {
			name = entry.Name
		}
		backends := rt.backends
		rt.backends = nil
		for i, backend := range backends {
			items = append(items, shutdownItem{
				id:   fmt.Sprintf("%s:backend:%d", id, i+1),
				name: fmt.Sprintf("%s (backend %d)", name, i+1),
				pid:  backend.pid(),
				pgid: backend.pgid,
				port: backend.port,
				rt:   backend,
			})
		}
		items = append(items, shutdownItem{
			id:   id,
			name: name,
			pid:  rt.pid(),
			pgid: rt.pgid,
			port: rt.port,
			rt:   rt,
		})
		_ = unregisterRunning(runPath, id)
	}
	p.unlock()
	clearRunning(runPath)
	sort.Slice(items, func(i, j int) bool {
		if items[i].name == items[j].name {
			return items[i].id < items[j].id
		}
		return items[i].name < items[j].name
	})
	return items
}

func waitPending(w io.Writer, pending []*shutdownItem, timeout time.Duration, forced bool) []*shutdownItem {
	deadline := time.Now().Add(timeout)
	for len(pending) > 0 && time.Now().Before(deadline) {
		next := pending[:0]
		for _, item := range pending {
			if item.rt.finished() || !processAlive(item.pid) {
				if forced {
					shutdownLog(w, "  [ok] %-24s  強制終了を確認しました", item.name)
				} else {
					shutdownLog(w, "  [ok] %-24s  終了しました", item.name)
				}
				continue
			}
			next = append(next, item)
		}
		pending = next
		if len(pending) == 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	return pending
}

func shutdownLog(w io.Writer, format string, args ...any) {
	if w == nil || w == io.Discard {
		return
	}
	fmt.Fprintf(w, format+"\n", args...)
}
