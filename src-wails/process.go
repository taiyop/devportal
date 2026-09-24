package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const maxLogs = 200

type Runtime struct {
	cmd        *exec.Cmd
	port       uint16
	ready      bool
	bindHost   string
	pgid       int
	keepalive  *os.File
	pinned     bool
	active     int
	lastAccess time.Time
	waitOnce   sync.Once
	waitCh     chan struct{}
	waitErr    error
	backends   []*Runtime
}

func (rt *Runtime) pid() uint32 {
	if rt.cmd == nil || rt.cmd.Process == nil {
		return 0
	}
	return uint32(rt.cmd.Process.Pid)
}

func (rt *Runtime) wait() error {
	rt.waitOnce.Do(func() {
		if rt.cmd != nil {
			rt.waitErr = rt.cmd.Wait()
		}
		close(rt.waitCh)
	})
	<-rt.waitCh
	return rt.waitErr
}

type Inner struct {
	configPath  string
	runningPath string
	file        ConfigFile
	runtime     map[string]*Runtime
	logs        map[string][]LogEvent
	errors      map[string]string
	gatewayPort uint16
	favicons    map[string]string
}

type Portal struct {
	mu              sync.Mutex
	inner           Inner
	gatewayMu       sync.Mutex
	gateway         *http.Server
	gatewayLn       net.Listener
	gatewayLn4      net.Listener
	gatewayListen   string
	gatewayPort     uint16
	gatewayNote     string
	gatewayErr      string
	EmitStatus      func(AppView)
	EmitLog         func(LogEvent)
	EmitGateway     func(GatewayStatus)
	shutdownOnce    sync.Once
	startingMu      sync.Mutex
	starting        map[string]struct{}
	faviconMu       sync.Mutex
	faviconInflight map[string]struct{}
}

func LoadPortal() (*Portal, error) {
	cfgPath, err := configPath()
	if err != nil {
		return nil, err
	}
	runPath, err := runningPath()
	if err != nil {
		return nil, err
	}
	reapPersisted(runPath)
	file, err := loadConfig(cfgPath)
	if err != nil {
		return nil, err
	}
	portal := &Portal{
		inner: Inner{
			configPath:  cfgPath,
			runningPath: runPath,
			file:        file,
			runtime:     map[string]*Runtime{},
			logs:        map[string][]LogEvent{},
			errors:      map[string]string{},
			favicons:    loadFaviconCache(cfgPath),
		},
	}
	portal.hydrateMissingFavicons()
	return portal, nil
}

func (p *Portal) lock() *Inner {
	p.mu.Lock()
	return &p.inner
}

func (p *Portal) unlock() {
	p.mu.Unlock()
}

func (p *Portal) emitStatus(view AppView) {
	if p.EmitStatus != nil {
		p.EmitStatus(view)
	}
}

func (p *Portal) emitLog(event LogEvent) {
	if p.EmitLog != nil {
		p.EmitLog(event)
	}
}

func (p *Portal) Views() []AppView {
	inner := p.lock()
	defer p.unlock()
	out := make([]AppView, 0, len(inner.file.Apps))
	for _, entry := range inner.file.Apps {
		out = append(out, viewFor(inner, entry))
	}
	return out
}

func viewFor(inner *Inner, entry AppEntry) AppView {
	if rt, ok := inner.runtime[entry.ID]; ok {
		status := AppStatusStarting
		if rt.ready {
			status = AppStatusRunning
		}
		pid := rt.pid()
		port := rt.port
		var errMsg *string
		if msg, exists := inner.errors[entry.ID]; exists {
			errMsg = &msg
		}
		view := viewFromEntry(entry, status, &pid, &port, errMsg, inner.gatewayPort, rt.bindHost)
		pids := make([]uint32, len(view.Backends))
		for i, backend := range rt.backends {
			if i < len(view.Backends) {
				backendPort := backend.port
				view.Backends[i].Port = &backendPort
			}
			if i < len(pids) {
				pids[i] = backend.pid()
			}
		}
		if len(rt.backends) > len(pids) {
			for i := len(pids); i < len(rt.backends); i++ {
				pids = append(pids, rt.backends[i].pid())
			}
		}
		view.BackendPIDs = pids
		return attachFavicon(inner, decorateIdle(view, entry, rt))
	}
	var errMsg *string
	status := AppStatusStopped
	if msg, exists := inner.errors[entry.ID]; exists {
		errMsg = &msg
		status = AppStatusError
	} else if entry.Hostname != "" && inner.gatewayPort != 0 {
		status = AppStatusIdle
	}
	return attachFavicon(inner, decorateIdle(viewFromEntry(entry, status, nil, nil, errMsg, inner.gatewayPort, ""), entry, nil))
}

func decorateIdle(view AppView, entry AppEntry, rt *Runtime) AppView {
	view.IdleStopMin = entry.IdleStopMin
	if rt != nil {
		view.Pinned = rt.pinned
		view.IdleUntil = idleUntilUnix(rt.pinned, entry.IdleStopMin, rt.lastAccess)
	}
	return view
}

func (p *Portal) Upsert(input AppInput) (AppEntry, error) {
	inner := p.lock()
	defer p.unlock()
	oldFolder := ""
	if input.ID != nil {
		if id := strings.TrimSpace(*input.ID); id != "" {
			if found, err := findEntry(inner, id); err == nil {
				oldFolder = found.Folder
			}
		}
	}
	entry, err := upsert(inner, input)
	if err != nil {
		return AppEntry{}, err
	}
	p.captureLocalFaviconLocked(entry, oldFolder != "" && oldFolder != entry.Folder)
	return entry, nil
}

func upsert(inner *Inner, input AppInput) (AppEntry, error) {
	folder := normalizeFolder(input.Folder)
	if folder == "" {
		return AppEntry{}, errString("対象フォルダを入力してください")
	}
	info, err := os.Stat(folder)
	if err != nil || !info.IsDir() {
		return AppEntry{}, errString("フォルダが見つかりません: " + folder)
	}
	command := strings.TrimSpace(input.Command)
	if command == "" {
		return AppEntry{}, errString("起動コマンドを入力してください")
	}
	if input.PortMode == PortModeManual {
		if input.Port == nil || *input.Port == 0 {
			return AppEntry{}, errString("手動モードでは 1〜65535 のポート番号が必要です")
		}
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = defaultNameFromFolder(folder)
	}
	description := strings.TrimSpace(input.Description)
	hostname, err := normalizeHostname(input.Hostname)
	if err != nil {
		return AppEntry{}, err
	}
	id := uuid.NewString()
	if input.ID != nil && strings.TrimSpace(*input.ID) != "" {
		id = strings.TrimSpace(*input.ID)
	}
	env, err := normalizeEnv(input.Env)
	if err != nil {
		return AppEntry{}, err
	}
	if hostname == "" {
		hostname = uniqueHostname(inner.file.Apps, id, slugHostname(name))
	} else {
		for _, app := range inner.file.Apps {
			if app.ID != id && app.Hostname == hostname {
				return AppEntry{}, errString("ドメイン " + fullHostname(hostname) + " は " + app.Name + " に割り当て済みです")
			}
		}
	}
	backends, err := normalizeBackends(input.Backends, folder)
	if err != nil {
		return AppEntry{}, err
	}
	portEnv, err := normalizePortEnv(input.PortEnv, defaultListenPortEnv, "ポート")
	if err != nil {
		return AppEntry{}, err
	}
	for i, backend := range backends {
		appEnv := backend.appPortEnv(i)
		if portEnv == appEnv {
			return AppEntry{}, errString("アプリのポートと" + backendLabel(i, backend.Name) + "に同じ環境変数名は使えません: " + portEnv)
		}
	}
	categoryID := strings.TrimSpace(input.CategoryID)
	if categoryID != "" && !categoryExists(inner, categoryID) {
		categoryID = ""
	}
	entry := AppEntry{
		ID:            id,
		Name:          name,
		Description:   description,
		Hostname:      hostname,
		CategoryID:    categoryID,
		Folder:        folder,
		Command:       command,
		PortMode:      input.PortMode,
		Port:          input.Port,
		PortEnv:       portEnv,
		WakeOnRequest: input.wakeOnRequest(),
		IdleStopMin:   input.idleStopMin(),
		Env:           env,
		Backends:      backends,
	}
	used := occupiedPorts(inner.file.Apps, entry.ID)
	if entry.Port != nil {
		if owner, ok := used[*entry.Port]; ok {
			return AppEntry{}, errString(fmt.Sprintf("ポート %d は %s に割り当て済みです", *entry.Port, owner))
		}
		used[*entry.Port] = "アプリ本体"
	}
	for i, backend := range entry.Backends {
		if backend.Port == nil {
			continue
		}
		label := backendLabel(i, backend.Name)
		if owner, ok := used[*backend.Port]; ok {
			return AppEntry{}, errString(fmt.Sprintf("ポート %d は %s と %s で重複しています", *backend.Port, owner, label))
		}
		used[*backend.Port] = label
	}
	replaced := false
	for i := range inner.file.Apps {
		if inner.file.Apps[i].ID == entry.ID {
			inner.file.Apps[i] = entry
			replaced = true
			break
		}
	}
	if !replaced {
		inner.file.Apps = append(inner.file.Apps, entry)
	}
	if err := saveConfig(inner.configPath, inner.file); err != nil {
		return AppEntry{}, err
	}
	return entry, nil
}

func (p *Portal) Delete(id string) error {
	inner := p.lock()
	defer p.unlock()
	if _, ok := inner.runtime[id]; ok {
		return errString("実行中のアプリは削除できません。先に終了してください。")
	}
	before := len(inner.file.Apps)
	next := inner.file.Apps[:0]
	for _, app := range inner.file.Apps {
		if app.ID != id {
			next = append(next, app)
		}
	}
	inner.file.Apps = next
	if len(inner.file.Apps) == before {
		return errString("アプリが見つかりません")
	}
	delete(inner.logs, id)
	delete(inner.errors, id)
	delete(inner.favicons, id)
	removeFaviconFile(inner.configPath, id)
	return saveConfig(inner.configPath, inner.file)
}

func (p *Portal) Reorder(ids []string) ([]AppView, error) {
	inner := p.lock()
	defer p.unlock()
	byID := make(map[string]AppEntry, len(inner.file.Apps))
	for _, app := range inner.file.Apps {
		byID[app.ID] = app
	}
	next := make([]AppEntry, 0, len(inner.file.Apps))
	seen := make(map[string]bool, len(ids))
	for _, raw := range ids {
		id := strings.TrimSpace(raw)
		if id == "" || seen[id] {
			continue
		}
		if app, ok := byID[id]; ok {
			next = append(next, app)
			seen[id] = true
		}
	}
	for _, app := range inner.file.Apps {
		if !seen[app.ID] {
			next = append(next, app)
		}
	}
	inner.file.Apps = next
	if err := saveConfig(inner.configPath, inner.file); err != nil {
		return nil, err
	}
	out := make([]AppView, 0, len(next))
	for _, entry := range next {
		out = append(out, viewFor(inner, entry))
	}
	return out, nil
}

func (p *Portal) Start(id string) (AppView, error) {
	return p.start(id, true)
}

func (p *Portal) start(id string, pinned bool) (AppView, error) {
	inner := p.lock()
	if rt, ok := inner.runtime[id]; ok {
		if pinned {
			rt.pinned = true
		}
		entry, err := findEntry(inner, id)
		view := viewFor(inner, entry)
		p.unlock()
		if err != nil {
			return AppView{}, err
		}
		return view, nil
	}
	var knownPort uint16
	var knownBackendPorts []uint16
	if entry, err := findEntry(inner, id); err == nil {
		if entry.Port != nil {
			knownPort = *entry.Port
		}
		for _, backend := range entry.Backends {
			if backend.Port != nil && *backend.Port != 0 {
				knownBackendPorts = append(knownBackendPorts, *backend.Port)
			}
		}
	}
	p.unlock()

	p.setStarting(id, true)
	defer p.setStarting(id, false)

	if knownPort != 0 {
		_ = waitFree(knownPort, 5*time.Second)
	}
	for _, backendPort := range knownBackendPorts {
		if backendPort != knownPort {
			_ = waitFree(backendPort, 5*time.Second)
		}
	}

	inner = p.lock()
	if rt, ok := inner.runtime[id]; ok {
		if pinned {
			rt.pinned = true
		}
		entry, err := findEntry(inner, id)
		view := viewFor(inner, entry)
		p.unlock()
		if err != nil {
			return AppView{}, err
		}
		return view, nil
	}
	entry, err := findEntry(inner, id)
	if err != nil {
		p.unlock()
		return AppView{}, err
	}
	port, err := resolvePort(entry)
	if err != nil {
		p.unlock()
		return AppView{}, err
	}
	exclude := []uint16{port}
	backendPorts := make([]uint16, len(entry.Backends))
	for i, spec := range entry.Backends {
		mode := spec.PortMode
		if mode == "" {
			mode = PortModeAuto
		}
		backendPort, resolveErr := resolveListenPort(mode, spec.Port, exclude...)
		if resolveErr != nil {
			p.unlock()
			return AppView{}, errString(backendLabel(i, spec.Name) + "の" + resolveErr.Error())
		}
		backendPorts[i] = backendPort
		exclude = append(exclude, backendPort)
	}
	entry.Port = &port
	for i := range entry.Backends {
		assigned := backendPorts[i]
		entry.Backends[i].Port = &assigned
	}
	for i := range inner.file.Apps {
		if inner.file.Apps[i].ID == id {
			inner.file.Apps[i].Port = &port
			if len(inner.file.Apps[i].Backends) == len(backendPorts) {
				for j := range inner.file.Apps[i].Backends {
					assigned := backendPorts[j]
					inner.file.Apps[i].Backends[j].Port = &assigned
				}
			} else {
				inner.file.Apps[i].Backends = cloneBackends(entry.Backends)
			}
		}
	}
	if err := saveConfig(inner.configPath, inner.file); err != nil {
		p.unlock()
		return AppView{}, err
	}
	delete(inner.errors, id)
	inner.logs[id] = []LogEvent{}
	p.unlock()

	ports := portInject{
		listenPort: port,
		appPort:    port,
		listenEnv:  entry.listenPortEnv(),
		backends:   backendInjects(entry.Backends, backendPorts),
	}
	pmap := ports.portMap()

	var backendRts []*Runtime
	stopStarted := func() {
		for i := len(backendRts) - 1; i >= 0; i-- {
			terminateRuntime(backendRts[i])
		}
	}
	for i, spec := range entry.Backends {
		backendCommand := injectPortMap(spec.Command, pmap)
		bcmd, bkeep, bpgid, bout, berr, spawnErr := spawnCommandWithPorts(
			entry.backendFolder(i),
			backendCommand,
			portInject{
				listenPort: backendPorts[i],
				appPort:    port,
				listenEnv:  spec.listenPortEnv(),
				backends:   ports.backends,
			},
			spec.Env,
		)
		if spawnErr != nil {
			stopStarted()
			return AppView{}, errString(backendLabel(i, spec.Name) + "を起動できませんでした: " + spawnErr.Error())
		}
		p.spawnLogReader(id, backendLogStream(i, spec.Name, false), bout)
		p.spawnLogReader(id, backendLogStream(i, spec.Name, true), berr)
		backendRt := &Runtime{
			cmd:       bcmd,
			port:      backendPorts[i],
			pgid:      bpgid,
			keepalive: bkeep,
			waitCh:    make(chan struct{}),
		}
		if !processAlive(backendRt.pid()) {
			terminateRuntime(backendRt)
			stopStarted()
			return AppView{}, errString(backendLabel(i, spec.Name) + "が起動直後に終了しました")
		}
		backendRts = append(backendRts, backendRt)
	}

	command := injectPortMap(entry.Command, pmap)
	cmd, keepalive, pgid, stdout, stderr, err := spawnCommandWithPorts(
		entry.Folder,
		command,
		ports,
		entry.Env,
	)
	if err != nil {
		stopStarted()
		return AppView{}, err
	}
	p.spawnLogReader(id, "stdout", stdout)
	p.spawnLogReader(id, "stderr", stderr)

	inner = p.lock()
	if _, exists := inner.runtime[id]; exists {
		p.unlock()
		terminateRuntime(&Runtime{cmd: cmd, pgid: pgid, keepalive: keepalive, waitCh: make(chan struct{})})
		stopStarted()
		return AppView{}, errString("アプリはすでに起動しています")
	}
	runningBackends := make([]RunningBackend, 0, len(backendRts))
	for i, backendRt := range backendRts {
		runningBackends = append(runningBackends, RunningBackend{
			PID:  backendRt.pid(),
			PGID: backendRt.pgid,
			Port: backendPorts[i],
		})
	}
	_ = registerRunning(inner.runningPath, RunningGroup{
		ID:       id,
		PID:      uint32(cmd.Process.Pid),
		PGID:     pgid,
		Port:     port,
		Backends: runningBackends,
	})
	rt := &Runtime{
		cmd:        cmd,
		port:       port,
		ready:      false,
		pgid:       pgid,
		keepalive:  keepalive,
		pinned:     pinned,
		lastAccess: time.Now(),
		waitCh:     make(chan struct{}),
		backends:   backendRts,
	}
	inner.runtime[id] = rt
	updated, _ := findEntry(inner, id)
	view := viewFor(inner, updated)
	p.unlock()

	go func() {
		_ = rt.wait()
		p.handleChildExit(id)
	}()
	for i, backendRt := range backendRts {
		label := backendLabel(i, entry.Backends[i].Name)
		go func(companion *Runtime, name string) {
			_ = companion.wait()
			p.handleCompanionExit(id, companion, name)
		}(backendRt, label)
	}
	go p.watchReady(id, port)
	return view, nil
}

func (p *Portal) SetPinned(id string, pinned bool) (AppView, error) {
	inner := p.lock()
	rt, ok := inner.runtime[id]
	if !ok {
		p.unlock()
		return AppView{}, errString("アプリは起動していません")
	}
	rt.pinned = pinned
	if !pinned {
		rt.lastAccess = time.Now()
	}
	entry, err := findEntry(inner, id)
	view := viewFor(inner, entry)
	p.unlock()
	if err != nil {
		return AppView{}, err
	}
	p.emitStatus(view)
	return view, nil
}

func (p *Portal) beginProxy(id string) {
	inner := p.lock()
	rt, ok := inner.runtime[id]
	if !ok {
		p.unlock()
		return
	}
	rt.active++
	rt.lastAccess = time.Now()
	entry, err := findEntry(inner, id)
	view := viewFor(inner, entry)
	p.unlock()
	if err == nil {
		p.emitStatus(view)
	}
}

func (p *Portal) endProxy(id string) {
	inner := p.lock()
	rt, ok := inner.runtime[id]
	if !ok {
		p.unlock()
		return
	}
	if rt.active > 0 {
		rt.active--
	}
	rt.lastAccess = time.Now()
	entry, err := findEntry(inner, id)
	view := viewFor(inner, entry)
	p.unlock()
	if err == nil {
		p.emitStatus(view)
	}
}

func (p *Portal) startIdleWatcher() {
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for range ticker.C {
			p.reapIdle()
		}
	}()
}

func (p *Portal) reapIdle() {
	inner := p.lock()
	now := time.Now()
	var toStop []string
	for id, rt := range inner.runtime {
		entry, err := findEntry(inner, id)
		if err != nil {
			continue
		}
		if shouldIdleStop(rt.pinned, rt.active, entry.IdleStopMin, rt.lastAccess, now) {
			toStop = append(toStop, id)
		}
	}
	p.unlock()
	for _, id := range toStop {
		view, err := p.Stop(id)
		if err == nil {
			p.emitStatus(view)
		}
	}
}

func (p *Portal) Stop(id string) (AppView, error) {
	inner := p.lock()
	if rt, ok := inner.runtime[id]; ok {
		delete(inner.runtime, id)
		_ = unregisterRunning(inner.runningPath, id)
		p.unlock()
		terminateRuntime(rt)
	} else {
		p.unlock()
	}
	inner = p.lock()
	delete(inner.errors, id)
	p.unlock()
	p.emitView(id)
	inner = p.lock()
	entry, err := findEntry(inner, id)
	view := viewFor(inner, entry)
	p.unlock()
	if err != nil {
		return AppView{}, err
	}
	return view, nil
}

func (p *Portal) StopAll() {
	p.shutdownOnce.Do(func() {
		p.shutdownAll(os.Stderr)
	})
}

func (rt *Runtime) finished() bool {
	if rt == nil || rt.waitCh == nil {
		return true
	}
	select {
	case <-rt.waitCh:
		return true
	default:
		return false
	}
}

func (p *Portal) pushLog(event LogEvent) {
	inner := p.lock()
	lines := inner.logs[event.ID]
	if len(lines) >= maxLogs {
		lines = lines[1:]
	}
	lines = append(lines, event)
	inner.logs[event.ID] = lines
	p.unlock()
	p.emitLog(event)
}

func (p *Portal) LogsFor(id string) []LogEvent {
	inner := p.lock()
	defer p.unlock()
	lines := inner.logs[id]
	if lines == nil {
		return []LogEvent{}
	}
	out := make([]LogEvent, len(lines))
	copy(out, lines)
	return out
}

func (p *Portal) ConfigPath() (string, error) {
	return configPath()
}

func (p *Portal) handleChildExit(id string) {
	inner := p.lock()
	rt, ok := inner.runtime[id]
	if !ok {
		p.unlock()
		return
	}
	delete(inner.runtime, id)
	_ = unregisterRunning(inner.runningPath, id)
	message := processExitMessage("プロセス", rt)
	inner.errors[id] = message
	backends := rt.backends
	rt.backends = nil
	if rt.keepalive != nil {
		_ = rt.keepalive.Close()
		rt.keepalive = nil
	}
	p.unlock()
	for _, backend := range backends {
		terminateRuntime(backend)
	}
	p.emitView(id)
}

func (p *Portal) handleCompanionExit(id string, companion *Runtime, label string) {
	inner := p.lock()
	rt, ok := inner.runtime[id]
	if !ok {
		p.unlock()
		return
	}
	stillTracked := false
	next := rt.backends[:0]
	for _, backend := range rt.backends {
		if backend == companion {
			stillTracked = true
			continue
		}
		next = append(next, backend)
	}
	if !stillTracked {
		p.unlock()
		return
	}
	rt.backends = next
	delete(inner.runtime, id)
	_ = unregisterRunning(inner.runningPath, id)
	inner.errors[id] = processExitMessage(label, companion)
	if companion != nil && companion.keepalive != nil {
		_ = companion.keepalive.Close()
		companion.keepalive = nil
	}
	p.unlock()
	terminateRuntime(rt)
	p.emitView(id)
}

func processExitMessage(label string, rt *Runtime) string {
	if rt == nil {
		return label + "が終了しました"
	}
	if rt.waitErr != nil {
		if rt.cmd != nil && rt.cmd.ProcessState != nil && !rt.cmd.ProcessState.Success() {
			return fmt.Sprintf("%sが異常終了しました (code: %v)", label, rt.cmd.ProcessState.ExitCode())
		}
		return fmt.Sprintf("%sの監視に失敗しました: %v", label, rt.waitErr)
	}
	if rt.cmd != nil && rt.cmd.ProcessState != nil && !rt.cmd.ProcessState.Success() {
		return fmt.Sprintf("%sが異常終了しました (code: %v)", label, rt.cmd.ProcessState.ExitCode())
	}
	return label + "が終了しました"
}

func (p *Portal) watchReady(id string, port uint16) {
	for i := 0; i < 80; i++ {
		time.Sleep(250 * time.Millisecond)
		inner := p.lock()
		rt, ok := inner.runtime[id]
		if !ok || rt.port != port {
			p.unlock()
			return
		}
		p.unlock()
		if isOpen(port) {
			p.markReady(id, port)
			return
		}
	}
	if isOpen(port) {
		p.markReady(id, port)
		return
	}
	inner := p.lock()
	if _, ok := inner.runtime[id]; ok {
		msg := fmt.Sprintf("プロセスは残っていますが、ポート %d で待受を確認できませんでした", port)
		inner.errors[id] = msg
		entry, err := findEntry(inner, id)
		var view AppView
		if err == nil {
			view = viewFromEntry(entry, AppStatusError, nil, &port, &msg, inner.gatewayPort, "")
		}
		p.unlock()
		if err == nil {
			p.emitStatus(view)
		}
		return
	}
	p.unlock()
}

func (p *Portal) markReady(id string, port uint16) {
	host := loopbackHost(port)
	inner := p.lock()
	rt, ok := inner.runtime[id]
	if !ok || rt.port != port {
		p.unlock()
		return
	}
	rt.ready = true
	rt.bindHost = host
	entry, err := findEntry(inner, id)
	var view AppView
	if err == nil {
		view = viewFor(inner, entry)
	}
	p.unlock()
	if err == nil {
		p.emitStatus(view)
	}
	go p.refreshLiveFavicon(id)
}

func resolvePort(entry AppEntry) (uint16, error) {
	return resolveListenPort(entry.PortMode, entry.Port)
}

type backendPortEnv struct {
	Port uint16
	Env  string
	Name string
}

type portInject struct {
	listenPort uint16
	appPort    uint16
	listenEnv  string
	backends   []backendPortEnv
}

func (p portInject) resolved() portInject {
	if p.appPort == 0 {
		p.appPort = p.listenPort
	}
	p.listenEnv = firstEnvName(p.listenEnv, defaultListenPortEnv)
	return p
}

func (p portInject) portMap() portMap {
	p = p.resolved()
	m := portMap{app: p.appPort, backends: make([]uint16, len(p.backends)), names: make([]string, len(p.backends))}
	for i, backend := range p.backends {
		m.backends[i] = backend.Port
		m.names[i] = backend.Name
	}
	return m
}

func backendInjects(specs []BackendSpec, ports []uint16) []backendPortEnv {
	out := make([]backendPortEnv, 0, len(specs))
	for i, spec := range specs {
		port := uint16(0)
		if i < len(ports) {
			port = ports[i]
		}
		out = append(out, backendPortEnv{
			Port: port,
			Env:  spec.appPortEnv(i),
			Name: strings.TrimSpace(spec.Name),
		})
	}
	return out
}

func backendLogStream(index int, name string, stderr bool) string {
	prefix := "backend"
	if stderr {
		prefix = "backend-err"
	}
	name = strings.TrimSpace(name)
	if name != "" {
		return prefix + ":" + name
	}
	if index == 0 {
		return prefix
	}
	return fmt.Sprintf("%s:%d", prefix, index+1)
}

func spawnCommand(folder, command string, port uint16, envVars []EnvVar) (*exec.Cmd, *os.File, int, io.ReadCloser, io.ReadCloser, error) {
	return spawnCommandWithPorts(folder, command, portInject{listenPort: port, appPort: port}, envVars)
}

func spawnCommandWithPorts(folder, command string, ports portInject, envVars []EnvVar) (*exec.Cmd, *os.File, int, io.ReadCloser, io.ReadCloser, error) {
	ports = ports.resolved()
	wrapped := commandWithPorts(command, ports, envVars)
	wrapped = withParentWatchdog(wrapped)
	cmd := shellCommand(wrapped)
	cmd.Dir = folder
	baseEnv, fromLogin := loginShellEnviron(folder)
	cmd.Env = appendLoopbackEnv(baseEnv, envVars)
	pmap := ports.portMap()
	for _, v := range envVars {
		cmd.Env = append(cmd.Env, v.Name+"="+injectPortMap(v.Value, pmap))
	}
	cmd.Env = append(cmd.Env, portEnvAssignments(ports)...)
	if !fromLogin {
		enrichPath(cmd)
	}
	isolateProcessGroup(cmd)
	stdinR, keepalive, err := attachKeepalive(cmd)
	if err != nil {
		return nil, nil, 0, nil, nil, err
	}
	closeStdinR := func() {
		if stdinR != nil {
			_ = stdinR.Close()
		}
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		closeStdinR()
		closeKeepalive(keepalive)
		return nil, nil, 0, nil, nil, fmt.Errorf("起動に失敗しました: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		closeStdinR()
		closeKeepalive(keepalive)
		return nil, nil, 0, nil, nil, fmt.Errorf("起動に失敗しました: %w", err)
	}
	if err := cmd.Start(); err != nil {
		closeStdinR()
		closeKeepalive(keepalive)
		return nil, nil, 0, nil, nil, fmt.Errorf("起動に失敗しました: %w", err)
	}
	closeStdinR()
	pgid := leaderPgid(cmd.Process.Pid)
	return cmd, keepalive, pgid, stdout, stderr, nil
}

func closeKeepalive(f *os.File) {
	if f != nil {
		_ = f.Close()
	}
}

func hasEnv(vars []EnvVar, name string) bool {
	for _, v := range vars {
		if v.Name == name {
			return true
		}
	}
	return false
}

func loopbackEnvAssignments(envVars []EnvVar) []string {
	out := make([]string, 0, 2)
	if !hasEnv(envVars, "HOST") {
		out = append(out, "HOST="+shSingleQuote(loopbackV4))
	}
	if !hasEnv(envVars, "HOSTNAME") {
		out = append(out, "HOSTNAME="+shSingleQuote(loopbackV4))
	}
	return out
}

func appendLoopbackEnv(env []string, envVars []EnvVar) []string {
	if !hasEnv(envVars, "HOST") {
		env = append(env, "HOST="+loopbackV4)
	}
	if !hasEnv(envVars, "HOSTNAME") {
		env = append(env, "HOSTNAME="+loopbackV4)
	}
	return env
}

func commandWithEnv(command string, port uint16, envVars []EnvVar) string {
	return commandWithPorts(command, portInject{listenPort: port, appPort: port}, envVars)
}

func commandWithPorts(command string, ports portInject, envVars []EnvVar) string {
	ports = ports.resolved()
	pmap := ports.portMap()
	assignments := make([]string, 0, len(envVars)+6+len(ports.backends)*2)
	assignments = append(assignments, loopbackEnvAssignments(envVars)...)
	for _, v := range envVars {
		assignments = append(assignments, v.Name+"="+shSingleQuote(injectPortMap(v.Value, pmap)))
	}
	for _, pair := range portEnvAssignments(ports) {
		name, value, ok := strings.Cut(pair, "=")
		if !ok {
			continue
		}
		assignments = append(assignments, name+"="+shSingleQuote(value))
	}
	return "env " + strings.Join(assignments, " ") + " " + injectPortMap(command, pmap)
}

func portEnvAssignments(ports portInject) []string {
	ports = ports.resolved()
	out := []string{
		ports.listenEnv + "=" + strconv.Itoa(int(ports.listenPort)),
		"DEVPORTAL_PORT=" + strconv.Itoa(int(ports.appPort)),
	}
	for i, backend := range ports.backends {
		if backend.Port == 0 {
			continue
		}
		value := strconv.Itoa(int(backend.Port))
		if backend.Env != "" {
			out = append(out, backend.Env+"="+value)
		}
		if i == 0 {
			out = append(out, "DEVPORTAL_BACKEND_PORT="+value)
		}
		out = append(out, fmt.Sprintf("DEVPORTAL_BACKEND_PORT_%d=%s", i+1, value))
	}
	return out
}

func shSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

// GUI apps do not source .zshrc/.bashrc, so version managers like nodenv are
// missing from PATH. We load the user's login+interactive shell once per spawn
// to copy that environment, then still run the command under /bin/sh so the
// POSIX parent-watchdog (background job + exec) is not subject to zsh job control.
const loginShellDump = "command -v printenv >/dev/null 2>&1 && printenv || env"

func shellCommand(command string) *exec.Cmd {
	if isWindows() {
		return exec.Command("cmd", "/C", command)
	}
	return exec.Command("/bin/sh", "-c", command)
}

func loginShellEnviron(dir string) ([]string, bool) {
	if isWindows() {
		return os.Environ(), false
	}
	captured, err := captureLoginShellEnv(dir)
	if err != nil || len(captured) == 0 {
		return os.Environ(), false
	}
	return overlayEnv(os.Environ(), captured), true
}

func captureLoginShellEnv(dir string) ([]string, error) {
	shell := userShell()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, shell, append(loginInteractiveArgs(shell), loginShellDump)...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = append(os.Environ(), "TERM=dumb")
	cmd.Stdin = nil
	cmd.Stderr = io.Discard
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	parsed := parseExportedEnv(out)
	if len(parsed) == 0 {
		return nil, errString("login shell produced no environment")
	}
	return parsed, nil
}

func parseExportedEnv(out []byte) []string {
	lines := strings.Split(string(out), "\n")
	env := make([]string, 0, len(lines))
	for _, line := range lines {
		name, _, ok := strings.Cut(line, "=")
		if !ok || name == "" {
			continue
		}
		switch name {
		case "_", "PWD", "OLDPWD", "SHLVL", "SHELL_SESSION_ID":
			continue
		}
		env = append(env, line)
	}
	return env
}

func overlayEnv(base, overlay []string) []string {
	index := make(map[string]int, len(base)+len(overlay))
	out := make([]string, 0, len(base)+len(overlay))
	for _, kv := range base {
		name, _, _ := strings.Cut(kv, "=")
		if name == "" {
			continue
		}
		if i, ok := index[name]; ok {
			out[i] = kv
			continue
		}
		index[name] = len(out)
		out = append(out, kv)
	}
	for _, kv := range overlay {
		name, _, _ := strings.Cut(kv, "=")
		if name == "" {
			continue
		}
		if i, ok := index[name]; ok {
			out[i] = kv
			continue
		}
		index[name] = len(out)
		out = append(out, kv)
	}
	return out
}

func userShell() string {
	if s := strings.TrimSpace(os.Getenv("SHELL")); s != "" {
		return s
	}
	if runtime.GOOS == "darwin" {
		if info, err := os.Stat("/bin/zsh"); err == nil && !info.IsDir() {
			return "/bin/zsh"
		}
	}
	return "/bin/sh"
}

func loginInteractiveArgs(shell string) []string {
	switch shellBaseName(shell) {
	case "fish":
		return []string{"-l", "-i", "-c"}
	default:
		return []string{"-lic"}
	}
}

func shellBaseName(shell string) string {
	base := strings.ToLower(filepath.Base(strings.TrimSpace(shell)))
	return strings.TrimSuffix(base, ".exe")
}

func isWindows() bool {
	return os.PathSeparator == '\\' && os.PathListSeparator == ';'
}

func enrichPath(cmd *exec.Cmd) {
	home, _ := os.UserHomeDir()
	current := os.Getenv("PATH")
	extras := []string{
		"/opt/homebrew/bin",
		"/usr/local/bin",
		filepath.Join(home, ".bun/bin"),
		filepath.Join(home, ".local/bin"),
		filepath.Join(home, ".cargo/bin"),
		"/opt/homebrew/opt/node/bin",
	}
	parts := make([]string, 0, len(extras)+1)
	for _, p := range extras {
		if p != "" {
			parts = append(parts, p)
		}
	}
	parts = append(parts, current)
	path := strings.Join(parts, string(os.PathListSeparator))
	if cmd.Env == nil {
		cmd.Env = os.Environ()
	}
	replaced := false
	for i, kv := range cmd.Env {
		if strings.HasPrefix(kv, "PATH=") {
			cmd.Env[i] = "PATH=" + path
			replaced = true
			break
		}
	}
	if !replaced {
		cmd.Env = append(cmd.Env, "PATH="+path)
	}
}

func terminateRuntime(rt *Runtime) {
	if rt == nil {
		return
	}
	backends := rt.backends
	rt.backends = nil
	for _, backend := range backends {
		terminateRuntime(backend)
	}
	if rt.keepalive != nil {
		_ = rt.keepalive.Close()
		rt.keepalive = nil
	}
	terminateProcess(rt.cmd, rt.pgid)
	_ = rt.wait()
	releaseListenPort(rt.port, rt.pgid)
}

func (p *Portal) spawnLogReader(id, stream string, reader io.ReadCloser) {
	go func() {
		defer reader.Close()
		scanner := bufio.NewScanner(reader)
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, 1024*1024)
		for scanner.Scan() {
			p.pushLog(LogEvent{ID: id, Stream: stream, Line: scanner.Text()})
		}
	}()
}

func (p *Portal) emitView(id string) {
	if p.isStarting(id) {
		return
	}
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

func (p *Portal) setStarting(id string, on bool) {
	p.startingMu.Lock()
	defer p.startingMu.Unlock()
	if p.starting == nil {
		p.starting = map[string]struct{}{}
	}
	if on {
		p.starting[id] = struct{}{}
		return
	}
	delete(p.starting, id)
}

func (p *Portal) isStarting(id string) bool {
	p.startingMu.Lock()
	defer p.startingMu.Unlock()
	_, ok := p.starting[id]
	return ok
}

func findEntry(inner *Inner, id string) (AppEntry, error) {
	for _, app := range inner.file.Apps {
		if app.ID == id {
			return app, nil
		}
	}
	return AppEntry{}, errString("アプリが見つかりません")
}
