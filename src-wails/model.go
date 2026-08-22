package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type PortMode string

const (
	PortModeAuto   PortMode = "auto"
	PortModeManual PortMode = "manual"
)

type AppStatus string

const (
	AppStatusStopped  AppStatus = "stopped"
	AppStatusIdle     AppStatus = "idle"
	AppStatusStarting AppStatus = "starting"
	AppStatusRunning  AppStatus = "running"
	AppStatusError    AppStatus = "error"
)

type EnvVar struct {
	Name  string `json:"name" yaml:"name"`
	Value string `json:"value" yaml:"value"`
}

type BackendSpec struct {
	Name       string   `json:"name" yaml:"name,omitempty"`
	Folder     string   `json:"folder" yaml:"folder"`
	Command    string   `json:"command" yaml:"command"`
	PortMode   PortMode `json:"portMode" yaml:"portMode"`
	Port       *uint16  `json:"port" yaml:"port"`
	PortEnv    string   `json:"portEnv" yaml:"portEnv,omitempty"`
	AppPortEnv string   `json:"appPortEnv" yaml:"appPortEnv,omitempty"`
	Env        []EnvVar `json:"env" yaml:"env"`
}

type AppEntry struct {
	ID            string        `json:"id" yaml:"id"`
	Name          string        `json:"name" yaml:"name"`
	Description   string        `json:"description" yaml:"description"`
	Hostname      string        `json:"hostname" yaml:"hostname"`
	Folder        string        `json:"folder" yaml:"folder"`
	Command       string        `json:"command" yaml:"command"`
	PortMode      PortMode      `json:"portMode" yaml:"portMode"`
	Port          *uint16       `json:"port" yaml:"port"`
	PortEnv       string        `json:"portEnv" yaml:"portEnv,omitempty"`
	WakeOnRequest bool          `json:"wakeOnRequest" yaml:"wakeOnRequest"`
	IdleStopMin   int           `json:"idleStopMin" yaml:"idleStopMin"`
	Env           []EnvVar      `json:"env" yaml:"env"`
	Backends      []BackendSpec `json:"backends,omitempty" yaml:"backends,omitempty"`
}

type appEntryYAML struct {
	ID             string        `yaml:"id"`
	Name           string        `yaml:"name"`
	Description    string        `yaml:"description"`
	Hostname       string        `yaml:"hostname"`
	Folder         string        `yaml:"folder"`
	Command        string        `yaml:"command"`
	PortMode       PortMode      `yaml:"portMode"`
	Port           *uint16       `yaml:"port"`
	PortEnv        string        `yaml:"portEnv"`
	BackendPortEnv string        `yaml:"backendPortEnv"`
	WakeOnRequest  *bool         `yaml:"wakeOnRequest"`
	IdleStopMin    int           `yaml:"idleStopMin"`
	Env            []EnvVar      `yaml:"env"`
	Backend        *BackendSpec  `yaml:"backend"`
	Backends       []BackendSpec `yaml:"backends"`
}

func (e *AppEntry) UnmarshalYAML(unmarshal func(any) error) error {
	var raw appEntryYAML
	if err := unmarshal(&raw); err != nil {
		return err
	}
	e.ID = raw.ID
	e.Name = raw.Name
	e.Description = strings.TrimSpace(raw.Description)
	e.Hostname = strings.ToLower(strings.TrimSpace(raw.Hostname))
	e.Folder = raw.Folder
	e.Command = raw.Command
	e.PortMode = raw.PortMode
	if e.PortMode == "" {
		e.PortMode = PortModeAuto
	}
	e.Port = raw.Port
	e.PortEnv = strings.TrimSpace(raw.PortEnv)
	e.WakeOnRequest = true
	if raw.WakeOnRequest != nil {
		e.WakeOnRequest = *raw.WakeOnRequest
	}
	e.IdleStopMin = clampIdleStopMin(raw.IdleStopMin)
	e.Env = raw.Env
	if e.Env == nil {
		e.Env = []EnvVar{}
	}
	e.Backends = mergeLoadedBackends(raw.Backends, raw.Backend, raw.BackendPortEnv)
	return nil
}

type ConfigFile struct {
	GatewayPort *uint16    `json:"gatewayPort" yaml:"gatewayPort,omitempty"`
	Apps        []AppEntry `json:"apps" yaml:"apps"`
}

type AppInput struct {
	ID            *string       `json:"id"`
	Name          string        `json:"name"`
	Description   string        `json:"description"`
	Hostname      string        `json:"hostname"`
	Folder        string        `json:"folder"`
	Command       string        `json:"command"`
	PortMode      PortMode      `json:"portMode"`
	Port          *uint16       `json:"port"`
	PortEnv       string        `json:"portEnv"`
	WakeOnRequest *bool         `json:"wakeOnRequest"`
	IdleStopMin   *int          `json:"idleStopMin"`
	Env           []EnvVar      `json:"env"`
	Backends      []BackendSpec `json:"backends"`
}

func (in AppInput) idleStopMin() int {
	if in.IdleStopMin == nil {
		return 15
	}
	return clampIdleStopMin(*in.IdleStopMin)
}

func (in AppInput) wakeOnRequest() bool {
	if in.WakeOnRequest == nil {
		return true
	}
	return *in.WakeOnRequest
}

type AppView struct {
	ID            string        `json:"id"`
	Name          string        `json:"name"`
	Description   string        `json:"description"`
	Hostname      string        `json:"hostname"`
	Folder        string        `json:"folder"`
	Command       string        `json:"command"`
	PortMode      PortMode      `json:"portMode"`
	Port          *uint16       `json:"port"`
	PortEnv       string        `json:"portEnv"`
	WakeOnRequest bool          `json:"wakeOnRequest"`
	IdleStopMin   int           `json:"idleStopMin"`
	Pinned        bool          `json:"pinned"`
	IdleUntil     *int64        `json:"idleUntil"`
	Env           []EnvVar      `json:"env"`
	Backends      []BackendSpec `json:"backends"`
	BackendPIDs   []uint32      `json:"backendPids"`
	Status        AppStatus     `json:"status"`
	URL           *string       `json:"url"`
	Favicon       *string       `json:"favicon"`
	PID           *uint32       `json:"pid"`
	Error         *string       `json:"error"`
}

type LogEvent struct {
	ID     string `json:"id"`
	Stream string `json:"stream"`
	Line   string `json:"line"`
}

func viewFromEntry(entry AppEntry, status AppStatus, pid *uint32, livePort *uint16, errMsg *string, gatewayPort uint16, bindHost string) AppView {
	port := livePort
	if port == nil {
		port = entry.Port
	}
	env := entry.Env
	if env == nil {
		env = []EnvVar{}
	}
	backends := cloneBackends(entry.Backends)
	return AppView{
		ID:            entry.ID,
		Name:          entry.Name,
		Description:   entry.Description,
		Hostname:      entry.Hostname,
		Folder:        entry.Folder,
		Command:       entry.Command,
		PortMode:      entry.PortMode,
		Port:          port,
		PortEnv:       entry.listenPortEnv(),
		WakeOnRequest: entry.WakeOnRequest,
		IdleStopMin:   entry.IdleStopMin,
		Env:           env,
		Backends:      backends,
		BackendPIDs:   []uint32{},
		Status:        status,
		URL:           publicAppURL(entry, port, status, gatewayPort, bindHost),
		PID:           pid,
		Error:         errMsg,
	}
}

func itoaPort(port uint16) string {
	if port == 0 {
		return "0"
	}
	n := int(port)
	buf := [5]byte{}
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

func normalizeEnv(vars []EnvVar) ([]EnvVar, error) {
	out := make([]EnvVar, 0, len(vars))
	for _, v := range vars {
		name := strings.TrimSpace(v.Name)
		if name == "" && v.Value == "" {
			continue
		}
		if name == "" {
			return nil, errString("環境変数名を入力してください")
		}
		if !isValidEnvName(name) {
			return nil, errString("環境変数名が不正です: " + name + "（英数字と _ のみ、先頭は数字不可）")
		}
		out = append(out, EnvVar{Name: name, Value: v.Value})
	}
	return out, nil
}

const (
	defaultListenPortEnv  = "PORT"
	defaultBackendPortEnv = "BACKEND_PORT"
)

func firstEnvName(name, fallback string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return fallback
	}
	return name
}

func normalizePortEnv(name, fallback, label string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return fallback, nil
	}
	if !isValidEnvName(name) {
		return "", errString(label + "の環境変数名が不正です: " + name + "（英数字と _ のみ、先頭は数字不可）")
	}
	return name, nil
}

func (e AppEntry) listenPortEnv() string {
	return firstEnvName(e.PortEnv, defaultListenPortEnv)
}

func defaultAppPortEnv(index int) string {
	if index <= 0 {
		return defaultBackendPortEnv
	}
	return fmt.Sprintf("%s_%d", defaultBackendPortEnv, index+1)
}

func (b BackendSpec) listenPortEnv() string {
	return firstEnvName(b.PortEnv, defaultListenPortEnv)
}

func (b BackendSpec) appPortEnv(index int) string {
	return firstEnvName(b.AppPortEnv, defaultAppPortEnv(index))
}

func (e AppEntry) hasBackends() bool {
	return len(e.Backends) > 0
}

func (e AppEntry) backendFolder(index int) string {
	if index < 0 || index >= len(e.Backends) {
		return e.Folder
	}
	folder := strings.TrimSpace(e.Backends[index].Folder)
	if folder == "" {
		return e.Folder
	}
	return folder
}

func (e AppEntry) backendNames() []string {
	names := make([]string, len(e.Backends))
	for i, spec := range e.Backends {
		names[i] = strings.TrimSpace(spec.Name)
	}
	return names
}

func isValidEnvName(name string) bool {
	if name == "" {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		if i == 0 {
			if !isASCIIAlpha(c) && c != '_' {
				return false
			}
			continue
		}
		if !isASCIIAlpha(c) && !isASCIIDigit(c) && c != '_' {
			return false
		}
	}
	return true
}

func isASCIIAlpha(c byte) bool {
	return c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z'
}

func isASCIIDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

func normalizeFolder(input string) string {
	trimmed := strings.TrimSpace(input)
	withoutScheme := trimmed
	if rest, ok := strings.CutPrefix(trimmed, "file://"); ok {
		withoutScheme = rest
		if rest2, ok := strings.CutPrefix(withoutScheme, "localhost"); ok {
			withoutScheme = rest2
		}
	}
	return percentDecode(withoutScheme)
}

func injectPort(command string, port uint16) string {
	return injectPortMap(command, portMap{app: port})
}

func injectPorts(s string, port, backendPort uint16) string {
	m := portMap{app: port}
	if backendPort != 0 {
		m.backends = []uint16{backendPort}
	}
	return injectPortMap(s, m)
}

type portMap struct {
	app      uint16
	backends []uint16
	names    []string
}

func injectPortMap(s string, m portMap) string {
	s = strings.ReplaceAll(s, "{port}", itoaPort(m.app))
	if len(m.backends) > 0 && m.backends[0] != 0 {
		s = strings.ReplaceAll(s, "{backendPort}", itoaPort(m.backends[0]))
	}
	for i, port := range m.backends {
		if port == 0 {
			continue
		}
		n := itoaPort(port)
		s = strings.ReplaceAll(s, fmt.Sprintf("{backendPort:%d}", i+1), n)
		if i < len(m.names) {
			name := strings.TrimSpace(m.names[i])
			if name != "" {
				s = strings.ReplaceAll(s, "{backendPort:"+name+"}", n)
			}
		}
	}
	return s
}

func backendLabel(index int, name string) string {
	name = strings.TrimSpace(name)
	if name != "" {
		return "バックエンド " + name
	}
	return fmt.Sprintf("バックエンド%d", index+1)
}

func mergeLoadedBackends(list []BackendSpec, legacy *BackendSpec, legacyAppPortEnv string) []BackendSpec {
	specs := list
	if len(specs) == 0 {
		if loaded := normalizeLoadedBackend(legacy); loaded != nil {
			specs = []BackendSpec{*loaded}
		}
	} else {
		out := make([]BackendSpec, 0, len(specs))
		for i := range specs {
			if loaded := normalizeLoadedBackend(&specs[i]); loaded != nil {
				out = append(out, *loaded)
			}
		}
		specs = out
	}
	if strings.TrimSpace(legacyAppPortEnv) != "" && len(specs) > 0 && strings.TrimSpace(specs[0].AppPortEnv) == "" {
		specs[0].AppPortEnv = strings.TrimSpace(legacyAppPortEnv)
	}
	if specs == nil {
		return []BackendSpec{}
	}
	return specs
}

func normalizeLoadedBackend(spec *BackendSpec) *BackendSpec {
	if spec == nil {
		return nil
	}
	out := *spec
	out.Name = strings.TrimSpace(out.Name)
	out.Command = strings.TrimSpace(out.Command)
	out.Folder = strings.TrimSpace(out.Folder)
	if out.PortMode == "" {
		out.PortMode = PortModeAuto
	}
	out.PortEnv = strings.TrimSpace(out.PortEnv)
	out.AppPortEnv = strings.TrimSpace(out.AppPortEnv)
	if out.Env == nil {
		out.Env = []EnvVar{}
	}
	if out.Command == "" && out.Folder == "" && out.Name == "" && (out.Port == nil || *out.Port == 0) && len(out.Env) == 0 && out.AppPortEnv == "" {
		return nil
	}
	return &out
}

func cloneBackends(specs []BackendSpec) []BackendSpec {
	if specs == nil {
		return []BackendSpec{}
	}
	out := make([]BackendSpec, 0, len(specs))
	for i := range specs {
		out = append(out, cloneBackend(specs[i]))
	}
	return out
}

func cloneBackend(spec BackendSpec) BackendSpec {
	out := spec
	if spec.Port != nil {
		port := *spec.Port
		out.Port = &port
	}
	if spec.Env != nil {
		out.Env = append([]EnvVar(nil), spec.Env...)
	} else {
		out.Env = []EnvVar{}
	}
	return out
}

func isValidBackendName(name string) bool {
	if name == "" {
		return false
	}
	allDigits := true
	for i := 0; i < len(name); i++ {
		c := name[i]
		if i == 0 && !isASCIIAlpha(c) && c != '_' {
			return false
		}
		if !isASCIIAlpha(c) && !isASCIIDigit(c) && c != '_' && c != '-' {
			return false
		}
		if !isASCIIDigit(c) {
			allDigits = false
		}
	}
	return !allDigits
}

func normalizeBackends(input []BackendSpec, appFolder string) ([]BackendSpec, error) {
	out := make([]BackendSpec, 0, len(input))
	usedNames := map[string]int{}
	usedAppEnvs := map[string]int{}
	for i := range input {
		spec, err := normalizeBackend(&input[i], appFolder, len(out))
		if err != nil {
			return nil, err
		}
		if spec == nil {
			continue
		}
		if spec.Name != "" {
			key := strings.ToLower(spec.Name)
			if prev, ok := usedNames[key]; ok {
				return nil, errString(fmt.Sprintf("%s の名前が %s と重複しています", backendLabel(i, spec.Name), backendLabel(prev, out[prev].Name)))
			}
			usedNames[key] = len(out)
		}
		appEnv := spec.appPortEnv(len(out))
		if prev, ok := usedAppEnvs[appEnv]; ok {
			return nil, errString(fmt.Sprintf("アプリ側に渡す環境変数名 %s が %s と %s で重複しています", appEnv, backendLabel(prev, out[prev].Name), backendLabel(i, spec.Name)))
		}
		usedAppEnvs[appEnv] = len(out)
		out = append(out, *spec)
	}
	return out, nil
}

func normalizeBackend(input *BackendSpec, appFolder string, index int) (*BackendSpec, error) {
	if input == nil {
		return nil, nil
	}
	label := backendLabel(index, input.Name)
	command := strings.TrimSpace(input.Command)
	folder := normalizeFolder(input.Folder)
	name := strings.TrimSpace(input.Name)
	env, err := normalizeEnv(input.Env)
	if err != nil {
		return nil, errString(label + "の" + err.Error())
	}
	emptyPort := input.Port == nil || *input.Port == 0
	portEnv := strings.TrimSpace(input.PortEnv)
	appPortEnv := strings.TrimSpace(input.AppPortEnv)
	if command == "" && folder == "" && name == "" && emptyPort && len(env) == 0 && (input.PortMode == "" || input.PortMode == PortModeAuto) && (portEnv == "" || portEnv == defaultListenPortEnv) && (appPortEnv == "" || appPortEnv == defaultAppPortEnv(index)) {
		return nil, nil
	}
	if command == "" {
		return nil, errString(label + "の起動コマンドを入力してください")
	}
	if name != "" && !isValidBackendName(name) {
		return nil, errString(label + "の名前が不正です（英数字・ハイフン・_。先頭は数字不可、数字のみは不可）")
	}
	if folder == "" {
		folder = appFolder
	} else {
		info, statErr := os.Stat(folder)
		if statErr != nil || !info.IsDir() {
			return nil, errString(label + "のフォルダが見つかりません: " + folder)
		}
	}
	mode := input.PortMode
	if mode == "" {
		mode = PortModeAuto
	}
	if mode != PortModeAuto && mode != PortModeManual {
		return nil, errString(label + "のポート割り当てが不正です")
	}
	var port *uint16
	if input.Port != nil && *input.Port != 0 {
		copied := *input.Port
		port = &copied
	}
	if mode == PortModeManual && port == nil {
		return nil, errString(label + "の手動モードでは 1〜65535 のポート番号が必要です")
	}
	normalizedPortEnv, err := normalizePortEnv(input.PortEnv, defaultListenPortEnv, label+"のポート")
	if err != nil {
		return nil, err
	}
	normalizedAppPortEnv, err := normalizePortEnv(input.AppPortEnv, defaultAppPortEnv(index), label+"をアプリへ渡すポート")
	if err != nil {
		return nil, err
	}
	return &BackendSpec{
		Name:       name,
		Folder:     folder,
		Command:    command,
		PortMode:   mode,
		Port:       port,
		PortEnv:    normalizedPortEnv,
		AppPortEnv: normalizedAppPortEnv,
		Env:        env,
	}, nil
}

func occupiedPorts(apps []AppEntry, exceptID string) map[uint16]string {
	used := map[uint16]string{}
	add := func(port *uint16, label string) {
		if port == nil || *port == 0 {
			return
		}
		if _, exists := used[*port]; !exists {
			used[*port] = label
		}
	}
	for _, app := range apps {
		if app.ID == exceptID {
			continue
		}
		name := app.Name
		if name == "" {
			name = app.ID
		}
		add(app.Port, name)
		for i, backend := range app.Backends {
			add(backend.Port, name+" の"+backendLabel(i, backend.Name))
		}
	}
	return used
}

func defaultNameFromFolder(folder string) string {
	name := filepath.Base(folder)
	if name == "" || name == "." || name == string(filepath.Separator) {
		return "untitled"
	}
	return name
}

func percentDecode(input string) string {
	bytes := []byte(input)
	out := make([]byte, 0, len(bytes))
	for i := 0; i < len(bytes); {
		if bytes[i] == '%' && i+2 < len(bytes) {
			hi, hok := fromHex(bytes[i+1])
			lo, lok := fromHex(bytes[i+2])
			if hok && lok {
				out = append(out, (hi<<4)|lo)
				i += 3
				continue
			}
		}
		out = append(out, bytes[i])
		i++
	}
	return string(out)
}

func fromHex(b byte) (byte, bool) {
	switch {
	case b >= '0' && b <= '9':
		return b - '0', true
	case b >= 'a' && b <= 'f':
		return b - 'a' + 10, true
	case b >= 'A' && b <= 'F':
		return b - 'A' + 10, true
	default:
		return 0, false
	}
}

type stringError string

func (e stringError) Error() string { return string(e) }

func errString(msg string) error { return stringError(msg) }
