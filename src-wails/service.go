package main

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type PortalService struct {
	app      *application.App
	portal   *Portal
	stopPoll context.CancelFunc
}

func NewPortalService(app *application.App, portal *Portal) *PortalService {
	s := &PortalService{app: app, portal: portal}
	portal.EmitStatus = func(view AppView) {
		app.Event.Emit("app-status", view)
	}
	portal.EmitLog = func(event LogEvent) {
		app.Event.Emit("app-log", event)
	}
	portal.EmitGateway = func(status GatewayStatus) {
		app.Event.Emit("gateway-status", status)
	}
	return s
}

func (s *PortalService) ServiceStartup(_ context.Context, _ application.ServiceOptions) error {
	s.portal.ArmAllWake()
	s.portal.startIdleWatcher()
	pollCtx, cancel := context.WithCancel(context.Background())
	s.stopPoll = cancel
	go pollForUpdates(pollCtx, s.app)
	return nil
}

func (s *PortalService) StartGateway() (GatewayStatus, error) {
	err := s.portal.StartGateway()
	return s.portal.GatewayStatus(), err
}

func (s *PortalService) SetGatewayPort(port uint16) (GatewayStatus, error) {
	err := s.portal.SetGatewayPort(port)
	return s.portal.GatewayStatus(), err
}

func (s *PortalService) StopGateway() GatewayStatus {
	s.portal.StopGateway()
	return s.portal.GatewayStatus()
}

func (s *PortalService) GetGatewayStatus() GatewayStatus {
	return s.portal.GatewayStatus()
}

func (s *PortalService) SetPinned(id string, pinned bool) (AppView, error) {
	view, err := s.portal.SetPinned(id, pinned)
	if err != nil {
		return AppView{}, err
	}
	return view, nil
}

func (s *PortalService) ServiceShutdown() error {
	if s.stopPoll != nil {
		s.stopPoll()
		s.stopPoll = nil
	}
	disarmAll()
	s.portal.StopAll()
	return nil
}

func (s *PortalService) AppVersion() string {
	return runningVersion()
}

func (s *PortalService) CheckForUpdates() {
	go runCheckAndInstall(s.app)
}

func (s *PortalService) SkipUpdate(version string) {
	if s.app == nil || s.app.Updater == nil {
		return
	}
	s.app.Updater.SkipVersion(version)
}

func (s *PortalService) RestartToUpdate() error {
	if s.app == nil || s.app.Updater == nil {
		return fmt.Errorf("updater が初期化されていません")
	}
	return s.app.Updater.Restart(context.Background())
}

func (s *PortalService) ListApps() ([]AppView, error) {
	if s.portal.EmitGateway != nil {
		s.portal.EmitGateway(s.portal.GatewayStatus())
	}
	return s.portal.Views(), nil
}

func (s *PortalService) UpsertApp(input AppInput) (AppView, error) {
	entry, err := s.portal.Upsert(input)
	if err != nil {
		return AppView{}, err
	}
	s.portal.RefreshWake(entry.ID)
	inner := s.portal.lock()
	found, findErr := findEntry(inner, entry.ID)
	view := viewFor(inner, found)
	s.portal.unlock()
	if findErr != nil {
		return AppView{}, findErr
	}
	return view, nil
}

func (s *PortalService) DeleteApp(id string) error {
	return s.portal.Delete(id)
}

func (s *PortalService) StartApp(id string) (AppView, error) {
	view, err := s.portal.Start(id)
	if err != nil {
		return AppView{}, err
	}
	s.portal.emitStatus(view)
	return view, nil
}

func (s *PortalService) StopApp(id string) (AppView, error) {
	view, err := s.portal.Stop(id)
	if err != nil {
		return AppView{}, err
	}
	s.portal.emitStatus(view)
	return view, nil
}

func (s *PortalService) GetLogs(id string) ([]LogEvent, error) {
	return s.portal.LogsFor(id), nil
}

func (s *PortalService) GetConfigPath() (string, error) {
	return s.portal.ConfigPath()
}

func (s *PortalService) PickFolder() (string, error) {
	path, err := s.app.Dialog.OpenFile().
		SetTitle("対象フォルダを選択").
		CanChooseDirectories(true).
		CanChooseFiles(false).
		PromptForSingleSelection()
	if err != nil {
		return "", err
	}
	return path, nil
}

func (s *PortalService) OpenAppURL(raw string) error {
	opened, err := s.portal.PrepareHostnameURL(raw)
	if err != nil {
		return err
	}
	return s.app.Browser.OpenURL(opened)
}

func (s *PortalService) OpenConfigPath() error {
	path, err := s.portal.ConfigPath()
	if err != nil {
		return err
	}
	return openInFileManager(filepath.Dir(path))
}

func openInFileManager(dir string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", dir)
	case "windows":
		cmd = exec.Command("explorer", dir)
	default:
		cmd = exec.Command("xdg-open", dir)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("フォルダを開けませんでした: %w", err)
	}
	return nil
}
