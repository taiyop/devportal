package main

import (
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultControlAPIPort  uint16 = 17341
	maxControlAPIBodyBytes int64  = 1 << 20
)

var fallbackControlAPIPorts = []uint16{17341, 17342, 17343, 17344}

type ControlAPIStatus struct {
	URL           string `json:"url"`
	Port          uint16 `json:"port"`
	PreferredPort uint16 `json:"preferredPort"`
	Running       bool   `json:"running"`
	Token         string `json:"token"`
	InfoPath      string `json:"infoPath"`
	Error         string `json:"error"`
	Note          string `json:"note"`
}

type ControlAPIFileInfo struct {
	URL       string `json:"url"`
	Port      uint16 `json:"port"`
	Token     string `json:"token"`
	TokenPath string `json:"tokenPath"`
	PID       int    `json:"pid"`
	Version   string `json:"version"`
}

func preferredControlAPIPort(file ConfigFile) uint16 {
	if file.ControlAPIPort == nil || *file.ControlAPIPort == 0 {
		return defaultControlAPIPort
	}
	return *file.ControlAPIPort
}

func normalizeControlAPIPort(port uint16) uint16 {
	if port == 0 {
		return defaultControlAPIPort
	}
	return port
}

func controlAPIAddrsFor(port uint16) []string {
	preferred := normalizeControlAPIPort(port)
	seen := map[uint16]bool{preferred: true}
	addrs := []string{fmt.Sprintf("127.0.0.1:%d", preferred)}
	for _, extra := range fallbackControlAPIPorts {
		if seen[extra] {
			continue
		}
		seen[extra] = true
		addrs = append(addrs, fmt.Sprintf("127.0.0.1:%d", extra))
	}
	return append(addrs, "127.0.0.1:0")
}

func (p *Portal) preferredControlAPIPort() uint16 {
	inner := p.lock()
	defer p.unlock()
	return preferredControlAPIPort(inner.file)
}

func (p *Portal) controlAPIPaths() (tokenPath, infoPath string) {
	inner := p.lock()
	cfgPath := inner.configPath
	p.unlock()
	return apiTokenPathForConfig(cfgPath), apiInfoPathForConfig(cfgPath)
}

func (p *Portal) StartControlAPI() error {
	return p.listenFirstAvailableControlAPI(controlAPIAddrsFor(p.preferredControlAPIPort()))
}

func (p *Portal) listenFirstAvailableControlAPI(addrs []string) error {
	if len(addrs) == 0 {
		addrs = controlAPIAddrsFor(defaultControlAPIPort)
	}
	tokenPath, infoPath := p.controlAPIPaths()

	p.controlMu.Lock()
	if p.controlSrv != nil {
		p.controlMu.Unlock()
		return nil
	}
	token := p.controlToken
	if token == "" {
		loaded, err := loadOrCreateAPIToken(tokenPath)
		if err != nil {
			p.controlErr = err.Error()
			p.controlMu.Unlock()
			p.emitControlAPI()
			return err
		}
		token = loaded
		p.controlToken = token
	}

	preferred := listenPortOf(addrs[0])
	if preferred == 0 {
		preferred = int(defaultControlAPIPort)
	}
	var lastErr error
	busyPreferred := false
	deniedPreferred := false
	for i, addr := range addrs {
		ln, err := net.Listen("tcp4", addr)
		if err != nil {
			lastErr = err
			if listenPortOf(addr) == preferred {
				busyPreferred = busyPreferred || listenBusy(err)
				deniedPreferred = deniedPreferred || listenDenied(err)
			}
			if i < len(addrs)-1 && shouldTryNextListen(err) {
				continue
			}
			msg := fmt.Sprintf("ローカル API を %s で待ち受けできません: %v", addr, err)
			p.controlErr = msg
			p.controlMu.Unlock()
			p.emitControlAPI()
			return errString(msg)
		}

		port := tcpListenPort(ln)
		note := ""
		if int(port) != preferred {
			switch {
			case deniedPreferred:
				note = fmt.Sprintf("ポート %d は権限が必要なため、127.0.0.1:%d で待ち受けています。", preferred, port)
			case busyPreferred:
				note = fmt.Sprintf("ポート %d は使用中のため、127.0.0.1:%d で待ち受けています。", preferred, port)
			default:
				note = fmt.Sprintf("127.0.0.1:%d で待ち受けています。", port)
			}
		}

		srv := &http.Server{
			Addr:              ln.Addr().String(),
			Handler:           p.controlAPIHandler(),
			ReadHeaderTimeout: 10 * time.Second,
		}
		p.controlSrv = srv
		p.controlLn = ln
		p.controlPort = port
		p.controlNote = note
		p.controlErr = ""
		p.controlInfoPath = infoPath

		info := ControlAPIFileInfo{
			URL:       fmt.Sprintf("http://127.0.0.1:%d", port),
			Port:      port,
			Token:     token,
			TokenPath: tokenPath,
			PID:       os.Getpid(),
			Version:   runningVersion(),
		}
		_ = writeAPIInfo(infoPath, info)

		go func() { _ = srv.Serve(ln) }()
		p.controlMu.Unlock()
		p.emitControlAPI()
		return nil
	}

	msg := "ローカル API を開始できません"
	if lastErr != nil {
		msg = fmt.Sprintf("ローカル API を開始できません: %v", lastErr)
	}
	p.controlErr = msg
	p.controlMu.Unlock()
	p.emitControlAPI()
	return errString(msg)
}

func (p *Portal) StopControlAPI() {
	p.controlMu.Lock()
	srv := p.controlSrv
	ln := p.controlLn
	infoPath := p.controlInfoPath
	p.controlSrv = nil
	p.controlLn = nil
	p.controlPort = 0
	p.controlNote = ""
	p.controlErr = ""
	p.controlInfoPath = ""
	p.controlMu.Unlock()

	if ln != nil {
		_ = ln.Close()
	}
	if srv != nil {
		_ = srv.Close()
	}
	removeAPIInfo(infoPath)
	p.emitControlAPI()
}

func (p *Portal) SetControlAPIPort(port uint16) error {
	next := normalizeControlAPIPort(port)
	inner := p.lock()
	prev := preferredControlAPIPort(inner.file)
	stored := next
	inner.file.ControlAPIPort = &stored
	err := saveConfig(inner.configPath, inner.file)
	p.unlock()
	if err != nil {
		return err
	}

	p.controlMu.Lock()
	running := p.controlSrv != nil
	p.controlMu.Unlock()
	if running && prev != next {
		p.StopControlAPI()
		return p.StartControlAPI()
	}
	p.emitControlAPI()
	return nil
}

func (p *Portal) RotateControlAPIToken() (ControlAPIStatus, error) {
	tokenPath, infoPath := p.controlAPIPaths()
	nextToken, err := rotateAPIToken(tokenPath)
	if err != nil {
		return p.ControlAPIStatus(), err
	}

	p.controlMu.Lock()
	p.controlToken = nextToken
	running := p.controlSrv != nil
	port := p.controlPort
	if running {
		if p.controlInfoPath != "" {
			infoPath = p.controlInfoPath
		}
		_ = writeAPIInfo(infoPath, ControlAPIFileInfo{
			URL:       fmt.Sprintf("http://127.0.0.1:%d", port),
			Port:      port,
			Token:     nextToken,
			TokenPath: tokenPath,
			PID:       os.Getpid(),
			Version:   runningVersion(),
		})
	}
	p.controlMu.Unlock()

	p.emitControlAPI()
	return p.ControlAPIStatus(), nil
}

func (p *Portal) ControlAPIStatus() ControlAPIStatus {
	preferred := p.preferredControlAPIPort()
	_, infoPath := p.controlAPIPaths()
	p.controlMu.Lock()
	defer p.controlMu.Unlock()
	return p.controlAPISnapshotLocked(preferred, infoPath)
}

func (p *Portal) emitControlAPI() {
	status := p.ControlAPIStatus()
	if p.EmitControlAPI != nil {
		p.EmitControlAPI(status)
	}
}

func (p *Portal) controlAPISnapshotLocked(preferred uint16, defaultInfoPath string) ControlAPIStatus {
	preferred = normalizeControlAPIPort(preferred)
	port := p.controlPort
	if port == 0 {
		port = preferred
	}
	infoPath := p.controlInfoPath
	if infoPath == "" {
		infoPath = defaultInfoPath
	}
	return ControlAPIStatus{
		URL:           fmt.Sprintf("http://127.0.0.1:%d", port),
		Port:          port,
		PreferredPort: preferred,
		Running:       p.controlSrv != nil && p.controlErr == "",
		Token:         p.controlToken,
		InfoPath:      infoPath,
		Error:         p.controlErr,
		Note:          p.controlNote,
	}
}

func (p *Portal) controlAPIToken() string {
	p.controlMu.Lock()
	defer p.controlMu.Unlock()
	return p.controlToken
}

func (p *Portal) controlAPIHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", p.apiHealth)
	mux.HandleFunc("GET /api/v1/schema", p.apiSchema)

	mux.HandleFunc("GET /api/v1/apps", p.apiListApps)
	mux.HandleFunc("POST /api/v1/apps", p.apiCreateApp)
	mux.HandleFunc("PUT /api/v1/apps/order", p.apiReorderApps)
	mux.HandleFunc("GET /api/v1/apps/{id}", p.apiGetApp)
	mux.HandleFunc("PUT /api/v1/apps/{id}", p.apiPutApp)
	mux.HandleFunc("PATCH /api/v1/apps/{id}", p.apiPatchApp)
	mux.HandleFunc("DELETE /api/v1/apps/{id}", p.apiDeleteApp)
	mux.HandleFunc("POST /api/v1/apps/{id}/start", p.apiStartApp)
	mux.HandleFunc("POST /api/v1/apps/{id}/stop", p.apiStopApp)
	mux.HandleFunc("POST /api/v1/apps/{id}/restart", p.apiRestartApp)
	mux.HandleFunc("PUT /api/v1/apps/{id}/pinned", p.apiSetPinned)
	mux.HandleFunc("GET /api/v1/apps/{id}/logs", p.apiGetLogs)

	mux.HandleFunc("GET /api/v1/categories", p.apiListCategories)
	mux.HandleFunc("POST /api/v1/categories", p.apiCreateCategory)
	mux.HandleFunc("PUT /api/v1/categories/order", p.apiReorderCategories)
	mux.HandleFunc("GET /api/v1/categories/{id}", p.apiGetCategory)
	mux.HandleFunc("PUT /api/v1/categories/{id}", p.apiPutCategory)
	mux.HandleFunc("PATCH /api/v1/categories/{id}", p.apiPatchCategory)
	mux.HandleFunc("DELETE /api/v1/categories/{id}", p.apiDeleteCategory)

	mux.HandleFunc("GET /api/v1/gateway", p.apiGetGateway)
	mux.HandleFunc("POST /api/v1/gateway/start", p.apiStartGateway)
	mux.HandleFunc("POST /api/v1/gateway/stop", p.apiStopGateway)
	mux.HandleFunc("PUT /api/v1/gateway/port", p.apiSetGatewayPort)

	return p.withControlAPISecurity(mux)
}

func (p *Portal) withControlAPISecurity(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")

		if !isLoopbackHostHeader(r.Host) {
			writeAPIError(w, http.StatusForbidden, "forbidden host header")
			return
		}
		if strings.TrimSpace(r.Header.Get("Origin")) != "" {
			writeAPIError(w, http.StatusForbidden, "cross-origin browser requests are not allowed")
			return
		}

		expected := p.controlAPIToken()
		provided, ok := extractBearerToken(r.Header.Get("Authorization"))
		if expected == "" || !ok || subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="DevPortal"`)
			writeAPIError(w, http.StatusUnauthorized, "missing or invalid bearer token")
			return
		}

		next.ServeHTTP(w, r)
	})
}

func isLoopbackHostHeader(host string) bool {
	h := strings.TrimSpace(strings.ToLower(host))
	if h == "" {
		return true
	}
	if parsed, _, err := net.SplitHostPort(h); err == nil {
		h = parsed
	}
	h = strings.Trim(h, "[]")
	return h == "127.0.0.1" || h == "localhost" || h == "::1"
}

func extractBearerToken(header string) (string, bool) {
	header = strings.TrimSpace(header)
	if len(header) < len("Bearer ") {
		return "", false
	}
	if !strings.EqualFold(header[:7], "Bearer ") {
		return "", false
	}
	token := strings.TrimSpace(header[7:])
	return token, token != ""
}

func writeAPIJSON(w http.ResponseWriter, status int, payload any) {
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(payload)
}

func writeAPIError(w http.ResponseWriter, status int, msg string) {
	writeAPIJSON(w, status, map[string]string{"error": msg})
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, dst any, allowEmpty bool) error {
	if r.Body == nil {
		if allowEmpty {
			return nil
		}
		return errAPIStatus{status: http.StatusBadRequest, msg: "request body is required"}
	}
	limited := http.MaxBytesReader(w, r.Body, maxControlAPIBodyBytes)
	raw, err := io.ReadAll(limited)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return errAPIStatus{status: http.StatusRequestEntityTooLarge, msg: "request body is too large"}
		}
		return errAPIStatus{status: http.StatusBadRequest, msg: "failed to read request body"}
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		if allowEmpty {
			return nil
		}
		return errAPIStatus{status: http.StatusBadRequest, msg: "request body is required"}
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return errAPIStatus{status: http.StatusBadRequest, msg: fmt.Sprintf("invalid JSON: %v", err)}
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		return errAPIStatus{status: http.StatusBadRequest, msg: "request body must contain a single JSON value"}
	}
	return nil
}

type errAPIStatus struct {
	status int
	msg    string
}

func (e errAPIStatus) Error() string { return e.msg }

func writeDecodedError(w http.ResponseWriter, err error) {
	var apiErr errAPIStatus
	if errors.As(err, &apiErr) {
		writeAPIError(w, apiErr.status, apiErr.msg)
		return
	}
	writeAPIError(w, http.StatusBadRequest, err.Error())
}

func statusForPortalError(err error) int {
	if err == nil {
		return http.StatusOK
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "見つかりません"):
		if strings.Contains(msg, "フォルダが見つかりません") {
			return http.StatusBadRequest
		}
		return http.StatusNotFound
	case strings.Contains(msg, "実行中のアプリは削除できません"),
		strings.Contains(msg, "割り当て済み"),
		strings.Contains(msg, "重複"),
		strings.Contains(msg, "同じ名前のカテゴリ"),
		strings.Contains(msg, "すでに起動しています"),
		strings.Contains(msg, "アプリは起動していません"):
		return http.StatusConflict
	default:
		return http.StatusBadRequest
	}
}

func (p *Portal) apiHealth(w http.ResponseWriter, _ *http.Request) {
	inner := p.lock()
	cfgPath := inner.configPath
	p.unlock()
	if cfgPath == "" {
		cfgPath, _ = p.ConfigPath()
	}
	ctrl := p.ControlAPIStatus()
	writeAPIJSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"version":    runningVersion(),
		"configPath": cfgPath,
		"gateway":    p.GatewayStatus(),
		"controlApi": map[string]any{
			"url":           ctrl.URL,
			"port":          ctrl.Port,
			"preferredPort": ctrl.PreferredPort,
			"running":       ctrl.Running,
			"infoPath":      ctrl.InfoPath,
		},
	})
}

func (p *Portal) apiSchema(w http.ResponseWriter, _ *http.Request) {
	writeAPIJSON(w, http.StatusOK, controlAPISchemaDocument())
}

func (p *Portal) apiListApps(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	hostFilter := strings.ToLower(strings.TrimSpace(q.Get("hostname")))
	if hostFilter != "" {
		if normalized, err := normalizeHostname(hostFilter); err == nil && normalized != "" {
			hostFilter = normalized
		}
	}
	catFilter := strings.TrimSpace(q.Get("categoryId"))
	statusFilter := strings.ToLower(strings.TrimSpace(q.Get("status")))

	all := p.Views()
	filtered := make([]AppView, 0, len(all))
	for _, app := range all {
		if hostFilter != "" && strings.ToLower(app.Hostname) != hostFilter {
			continue
		}
		if catFilter != "" && app.CategoryID != catFilter {
			continue
		}
		if statusFilter != "" && string(app.Status) != statusFilter {
			continue
		}
		filtered = append(filtered, app)
	}
	writeAPIJSON(w, http.StatusOK, map[string]any{"apps": filtered})
}

func (p *Portal) apiGetApp(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	view, err := p.ViewByID(id)
	if err != nil {
		writeAPIError(w, http.StatusNotFound, err.Error())
		return
	}
	writeAPIJSON(w, http.StatusOK, view)
}

func (p *Portal) apiCreateApp(w http.ResponseWriter, r *http.Request) {
	var input AppInput
	if err := decodeJSONBody(w, r, &input, false); err != nil {
		writeDecodedError(w, err)
		return
	}
	if input.ID != nil {
		if id := strings.TrimSpace(*input.ID); id != "" {
			if _, err := p.ViewByID(id); err == nil {
				writeAPIError(w, http.StatusConflict, "同じ ID のアプリがすでに存在します: "+id)
				return
			}
		}
	}
	entry, err := p.Upsert(input)
	if err != nil {
		writeAPIError(w, statusForPortalError(err), err.Error())
		return
	}
	p.emitView(entry.ID)
	view, err := p.ViewByID(entry.ID)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeAPIJSON(w, http.StatusCreated, view)
}

func (p *Portal) apiPutApp(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeAPIError(w, http.StatusBadRequest, "app id is required")
		return
	}
	var input AppInput
	if err := decodeJSONBody(w, r, &input, false); err != nil {
		writeDecodedError(w, err)
		return
	}
	_, existedErr := p.ViewByID(id)
	existed := existedErr == nil
	input.ID = &id

	entry, err := p.Upsert(input)
	if err != nil {
		writeAPIError(w, statusForPortalError(err), err.Error())
		return
	}
	p.emitView(entry.ID)
	view, err := p.ViewByID(entry.ID)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	status := http.StatusOK
	if !existed {
		status = http.StatusCreated
	}
	writeAPIJSON(w, status, view)
}

type optionalUint16 struct {
	Set   bool
	Value *uint16
}

func (o *optionalUint16) UnmarshalJSON(data []byte) error {
	o.Set = true
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		o.Value = nil
		return nil
	}
	var v uint16
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	o.Value = &v
	return nil
}

type AppPatchInput struct {
	Name          *string        `json:"name"`
	Description   *string        `json:"description"`
	Hostname      *string        `json:"hostname"`
	CategoryID    *string        `json:"categoryId"`
	Folder        *string        `json:"folder"`
	Command       *string        `json:"command"`
	PortMode      *PortMode      `json:"portMode"`
	Port          optionalUint16 `json:"port"`
	PortEnv       *string        `json:"portEnv"`
	WakeOnRequest *bool          `json:"wakeOnRequest"`
	IdleStopMin   *int           `json:"idleStopMin"`
	Env           *[]EnvVar      `json:"env"`
	Backends      *[]BackendSpec `json:"backends"`
}

func (p *Portal) apiPatchApp(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	inner := p.lock()
	existing, err := findEntry(inner, id)
	p.unlock()
	if err != nil {
		writeAPIError(w, http.StatusNotFound, err.Error())
		return
	}

	var patch AppPatchInput
	if err := decodeJSONBody(w, r, &patch, false); err != nil {
		writeDecodedError(w, err)
		return
	}

	wake := existing.WakeOnRequest
	idle := existing.IdleStopMin
	var portCopy *uint16
	if existing.Port != nil {
		v := *existing.Port
		portCopy = &v
	}
	envCopy := append([]EnvVar(nil), existing.Env...)
	backendsCopy := cloneBackends(existing.Backends)

	input := AppInput{
		ID:            &existing.ID,
		Name:          existing.Name,
		Description:   existing.Description,
		Hostname:      existing.Hostname,
		CategoryID:    existing.CategoryID,
		Folder:        existing.Folder,
		Command:       existing.Command,
		PortMode:      existing.PortMode,
		Port:          portCopy,
		PortEnv:       existing.PortEnv,
		WakeOnRequest: &wake,
		IdleStopMin:   &idle,
		Env:           envCopy,
		Backends:      backendsCopy,
	}

	if patch.Name != nil {
		input.Name = *patch.Name
	}
	if patch.Description != nil {
		input.Description = *patch.Description
	}
	if patch.Hostname != nil {
		input.Hostname = *patch.Hostname
	}
	if patch.CategoryID != nil {
		input.CategoryID = *patch.CategoryID
	}
	if patch.Folder != nil {
		input.Folder = *patch.Folder
	}
	if patch.Command != nil {
		input.Command = *patch.Command
	}
	if patch.PortMode != nil {
		input.PortMode = *patch.PortMode
	}
	if patch.Port.Set {
		input.Port = patch.Port.Value
	}
	if patch.PortEnv != nil {
		input.PortEnv = *patch.PortEnv
	}
	if patch.WakeOnRequest != nil {
		v := *patch.WakeOnRequest
		input.WakeOnRequest = &v
	}
	if patch.IdleStopMin != nil {
		v := *patch.IdleStopMin
		input.IdleStopMin = &v
	}
	if patch.Env != nil {
		input.Env = *patch.Env
	}
	if patch.Backends != nil {
		input.Backends = *patch.Backends
	}

	entry, err := p.Upsert(input)
	if err != nil {
		writeAPIError(w, statusForPortalError(err), err.Error())
		return
	}
	p.emitView(entry.ID)
	view, err := p.ViewByID(entry.ID)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeAPIJSON(w, http.StatusOK, view)
}

func (p *Portal) apiDeleteApp(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	force := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("force")), "true") || r.URL.Query().Get("force") == "1"
	if force {
		if _, err := p.ViewByID(id); err != nil {
			writeAPIError(w, http.StatusNotFound, err.Error())
			return
		}
		_, _ = p.Stop(id)
	}
	if err := p.Delete(id); err != nil {
		writeAPIError(w, statusForPortalError(err), err.Error())
		return
	}
	writeAPIJSON(w, http.StatusOK, map[string]any{"deleted": true, "id": id})
}

type orderRequest struct {
	IDs []string `json:"ids"`
}

func (p *Portal) apiReorderApps(w http.ResponseWriter, r *http.Request) {
	var req orderRequest
	if err := decodeJSONBody(w, r, &req, false); err != nil {
		writeDecodedError(w, err)
		return
	}
	views, err := p.Reorder(req.IDs)
	if err != nil {
		writeAPIError(w, statusForPortalError(err), err.Error())
		return
	}
	writeAPIJSON(w, http.StatusOK, map[string]any{"apps": views})
}

type startAppRequest struct {
	Pinned *bool `json:"pinned"`
}

func (p *Portal) apiStartApp(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	var req startAppRequest
	if err := decodeJSONBody(w, r, &req, true); err != nil {
		writeDecodedError(w, err)
		return
	}
	pinned := true
	if req.Pinned != nil {
		pinned = *req.Pinned
	}
	view, err := p.start(id, pinned)
	if err != nil {
		writeAPIError(w, statusForPortalError(err), err.Error())
		return
	}
	p.emitStatus(view)
	writeAPIJSON(w, http.StatusOK, view)
}

func (p *Portal) apiStopApp(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	view, err := p.Stop(id)
	if err != nil {
		writeAPIError(w, statusForPortalError(err), err.Error())
		return
	}
	p.emitStatus(view)
	writeAPIJSON(w, http.StatusOK, view)
}

func (p *Portal) apiRestartApp(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	var req startAppRequest
	if err := decodeJSONBody(w, r, &req, true); err != nil {
		writeDecodedError(w, err)
		return
	}

	inner := p.lock()
	if _, err := findEntry(inner, id); err != nil {
		p.unlock()
		writeAPIError(w, http.StatusNotFound, err.Error())
		return
	}
	pinned := true
	if rt, ok := inner.runtime[id]; ok {
		pinned = rt.pinned
	}
	p.unlock()
	if req.Pinned != nil {
		pinned = *req.Pinned
	}

	if _, err := p.Stop(id); err != nil {
		writeAPIError(w, statusForPortalError(err), err.Error())
		return
	}
	view, err := p.start(id, pinned)
	if err != nil {
		writeAPIError(w, statusForPortalError(err), err.Error())
		return
	}
	p.emitStatus(view)
	writeAPIJSON(w, http.StatusOK, view)
}

type pinnedRequest struct {
	Pinned *bool `json:"pinned"`
}

func (p *Portal) apiSetPinned(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if _, err := p.ViewByID(id); err != nil {
		writeAPIError(w, http.StatusNotFound, err.Error())
		return
	}
	var req pinnedRequest
	if err := decodeJSONBody(w, r, &req, false); err != nil {
		writeDecodedError(w, err)
		return
	}
	if req.Pinned == nil {
		writeAPIError(w, http.StatusBadRequest, "pinned (boolean) is required")
		return
	}
	view, err := p.SetPinned(id, *req.Pinned)
	if err != nil {
		writeAPIError(w, statusForPortalError(err), err.Error())
		return
	}
	writeAPIJSON(w, http.StatusOK, view)
}

func (p *Portal) apiGetLogs(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if _, err := p.ViewByID(id); err != nil {
		writeAPIError(w, http.StatusNotFound, err.Error())
		return
	}
	streamFilter := strings.TrimSpace(r.URL.Query().Get("stream"))
	limit := 0
	if rawLimit := strings.TrimSpace(r.URL.Query().Get("limit")); rawLimit != "" {
		n, err := strconv.Atoi(rawLimit)
		if err != nil || n < 0 {
			writeAPIError(w, http.StatusBadRequest, "limit must be a non-negative integer")
			return
		}
		limit = n
	}

	lines := p.LogsFor(id)
	if streamFilter != "" {
		filtered := make([]LogEvent, 0, len(lines))
		for _, line := range lines {
			if line.Stream == streamFilter {
				filtered = append(filtered, line)
			}
		}
		lines = filtered
	}
	if limit > 0 && len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}
	writeAPIJSON(w, http.StatusOK, map[string]any{"logs": lines})
}

func (p *Portal) apiListCategories(w http.ResponseWriter, _ *http.Request) {
	writeAPIJSON(w, http.StatusOK, map[string]any{"categories": p.Categories()})
}

func (p *Portal) apiGetCategory(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	for _, cat := range p.Categories() {
		if cat.ID == id {
			writeAPIJSON(w, http.StatusOK, cat)
			return
		}
	}
	writeAPIError(w, http.StatusNotFound, "カテゴリが見つかりません")
}

type categoryRequest struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

func (p *Portal) apiCreateCategory(w http.ResponseWriter, r *http.Request) {
	var req categoryRequest
	if err := decodeJSONBody(w, r, &req, false); err != nil {
		writeDecodedError(w, err)
		return
	}
	if id := strings.TrimSpace(req.ID); id != "" {
		for _, cat := range p.Categories() {
			if cat.ID == id {
				writeAPIError(w, http.StatusConflict, "同じ ID のカテゴリがすでに存在します: "+id)
				return
			}
		}
	}
	entry, err := p.UpsertCategory(req.ID, req.Name, req.Color)
	if err != nil {
		writeAPIError(w, statusForPortalError(err), err.Error())
		return
	}
	writeAPIJSON(w, http.StatusCreated, entry)
}

func (p *Portal) apiPutCategory(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeAPIError(w, http.StatusBadRequest, "category id is required")
		return
	}
	var req categoryRequest
	if err := decodeJSONBody(w, r, &req, false); err != nil {
		writeDecodedError(w, err)
		return
	}
	existed := false
	for _, cat := range p.Categories() {
		if cat.ID == id {
			existed = true
			break
		}
	}
	entry, err := p.UpsertCategory(id, req.Name, req.Color)
	if err != nil {
		writeAPIError(w, statusForPortalError(err), err.Error())
		return
	}
	status := http.StatusOK
	if !existed {
		status = http.StatusCreated
	}
	writeAPIJSON(w, status, entry)
}

type categoryPatchRequest struct {
	Name  *string `json:"name"`
	Color *string `json:"color"`
}

func (p *Portal) apiPatchCategory(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	var current CategoryEntry
	found := false
	for _, cat := range p.Categories() {
		if cat.ID == id {
			current = cat
			found = true
			break
		}
	}
	if !found {
		writeAPIError(w, http.StatusNotFound, "カテゴリが見つかりません")
		return
	}
	var patch categoryPatchRequest
	if err := decodeJSONBody(w, r, &patch, false); err != nil {
		writeDecodedError(w, err)
		return
	}
	name := current.Name
	color := current.Color
	if patch.Name != nil {
		name = *patch.Name
	}
	if patch.Color != nil {
		color = *patch.Color
	}
	entry, err := p.UpsertCategory(id, name, color)
	if err != nil {
		writeAPIError(w, statusForPortalError(err), err.Error())
		return
	}
	writeAPIJSON(w, http.StatusOK, entry)
}

func (p *Portal) apiDeleteCategory(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if err := p.DeleteCategory(id); err != nil {
		writeAPIError(w, statusForPortalError(err), err.Error())
		return
	}
	writeAPIJSON(w, http.StatusOK, map[string]any{"deleted": true, "id": id})
}

func (p *Portal) apiReorderCategories(w http.ResponseWriter, r *http.Request) {
	var req orderRequest
	if err := decodeJSONBody(w, r, &req, false); err != nil {
		writeDecodedError(w, err)
		return
	}
	cats, err := p.ReorderCategories(req.IDs)
	if err != nil {
		writeAPIError(w, statusForPortalError(err), err.Error())
		return
	}
	writeAPIJSON(w, http.StatusOK, map[string]any{"categories": cats})
}

func (p *Portal) apiGetGateway(w http.ResponseWriter, _ *http.Request) {
	writeAPIJSON(w, http.StatusOK, p.GatewayStatus())
}

func (p *Portal) apiStartGateway(w http.ResponseWriter, _ *http.Request) {
	if err := p.StartGateway(); err != nil {
		writeAPIError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeAPIJSON(w, http.StatusOK, p.GatewayStatus())
}

func (p *Portal) apiStopGateway(w http.ResponseWriter, _ *http.Request) {
	p.StopGateway()
	writeAPIJSON(w, http.StatusOK, p.GatewayStatus())
}

type gatewayPortRequest struct {
	Port uint16 `json:"port"`
}

func (p *Portal) apiSetGatewayPort(w http.ResponseWriter, r *http.Request) {
	var req gatewayPortRequest
	if err := decodeJSONBody(w, r, &req, false); err != nil {
		writeDecodedError(w, err)
		return
	}
	if err := p.SetGatewayPort(req.Port); err != nil {
		writeAPIError(w, statusForPortalError(err), err.Error())
		return
	}
	writeAPIJSON(w, http.StatusOK, p.GatewayStatus())
}

func controlAPISchemaDocument() map[string]any {
	return map[string]any{
		"name":    "DevPortal Local Control API",
		"version": "v1",
		"auth": map[string]any{
			"type":   "bearer",
			"header": "Authorization: Bearer <token>",
			"notes":  "Loopback only (127.0.0.1). Cross-origin browser requests with an Origin header are rejected.",
		},
		"endpoints": []map[string]any{
			{"method": "GET", "path": "/api/v1/health", "summary": "Health, version, config path, gateway and control API status"},
			{"method": "GET", "path": "/api/v1/schema", "summary": "Self-describing API schema and field reference"},
			{"method": "GET", "path": "/api/v1/apps", "summary": "List apps (optional query: hostname, categoryId, status)"},
			{"method": "POST", "path": "/api/v1/apps", "summary": "Register a new app"},
			{"method": "PUT", "path": "/api/v1/apps/order", "summary": "Reorder apps with {ids: string[]}"},
			{"method": "GET", "path": "/api/v1/apps/{id}", "summary": "Get a single app by ID"},
			{"method": "PUT", "path": "/api/v1/apps/{id}", "summary": "Create or fully replace an app by ID"},
			{"method": "PATCH", "path": "/api/v1/apps/{id}", "summary": "Partially update an existing app"},
			{"method": "DELETE", "path": "/api/v1/apps/{id}", "summary": "Delete an app (optional ?force=true to stop first)"},
			{"method": "POST", "path": "/api/v1/apps/{id}/start", "summary": "Start an app (optional body: {pinned?: boolean}, default true)"},
			{"method": "POST", "path": "/api/v1/apps/{id}/stop", "summary": "Stop an app"},
			{"method": "POST", "path": "/api/v1/apps/{id}/restart", "summary": "Restart an app (optional body: {pinned?: boolean})"},
			{"method": "PUT", "path": "/api/v1/apps/{id}/pinned", "summary": "Toggle Keep Alive pin on a running app ({pinned: boolean})"},
			{"method": "GET", "path": "/api/v1/apps/{id}/logs", "summary": "Get buffered logs (optional query: limit, stream)"},
			{"method": "GET", "path": "/api/v1/categories", "summary": "List categories"},
			{"method": "POST", "path": "/api/v1/categories", "summary": "Create a category ({name, color?, id?})"},
			{"method": "PUT", "path": "/api/v1/categories/order", "summary": "Reorder categories with {ids: string[]}"},
			{"method": "GET", "path": "/api/v1/categories/{id}", "summary": "Get a single category"},
			{"method": "PUT", "path": "/api/v1/categories/{id}", "summary": "Create or replace a category"},
			{"method": "PATCH", "path": "/api/v1/categories/{id}", "summary": "Partially update a category"},
			{"method": "DELETE", "path": "/api/v1/categories/{id}", "summary": "Delete a category"},
			{"method": "GET", "path": "/api/v1/gateway", "summary": "Get reverse proxy gateway status"},
			{"method": "POST", "path": "/api/v1/gateway/start", "summary": "Start reverse proxy gateway"},
			{"method": "POST", "path": "/api/v1/gateway/stop", "summary": "Stop reverse proxy gateway"},
			{"method": "PUT", "path": "/api/v1/gateway/port", "summary": "Set preferred gateway port ({port: uint16}, 0 resets to 80)"},
		},
		"models": map[string]any{
			"AppInput": map[string]any{
				"id":            "string (optional; UUID generated if omitted)",
				"name":          "string (optional; defaults to folder basename)",
				"description":   "string (optional)",
				"hostname":      "string (optional; single DNS label before .devportal.localhost, auto-generated from name if omitted)",
				"categoryId":    "string (optional; existing category ID)",
				"folder":        "string (required; existing directory path)",
				"command":       "string (required; shell command, supports {port}, {backendPort}, {backendPort:<name>})",
				"portMode":      `"auto" | "manual" (optional; default "auto". Use "manual" for a fixed port)`,
				"port":          "uint16 | null (required when portMode is \"manual\")",
				"portEnv":       "string (optional; default \"PORT\")",
				"wakeOnRequest": "boolean (optional; default true)",
				"idleStopMin":   "int 0..1440 (optional; default 15, 0 disables auto-stop)",
				"env":           `Array<{name: string, value: string}> (optional)`,
				"backends":      `Array<{name?, folder?, command, portMode?: "auto"|"manual", port?, portEnv?, appPortEnv?, env?}> (optional)`,
			},
		},
	}
}
