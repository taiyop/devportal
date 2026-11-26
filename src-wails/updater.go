package main

import (
	"context"
	"io"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/updater"
	"github.com/wailsapp/wails/v3/pkg/updater/providers/github"
)

// currentVersion is the running build (no "v" prefix).
// Release CI overrides it from the git tag:
//
//	-ldflags "-X main.currentVersion=1.2.3"
var currentVersion = "0.1.0"

const (
	updateRepository        = "taiyop/devportal"
	updateChecksumAsset     = "SHA256SUMS"
	updateCheckStartupDelay = 8 * time.Second
	updateCheckInterval     = 6 * time.Hour
	updateCheckTimeout      = 45 * time.Second
	updateInstallTimeout    = 15 * time.Minute
)

func (p *Portal) UpdateChannel() UpdateChannel {
	if p == nil {
		return UpdateChannelStable
	}
	inner := p.lock()
	defer p.unlock()
	return normalizeUpdateChannel(inner.file.UpdateChannel)
}

func (p *Portal) SetUpdateChannel(raw string) (UpdateChannel, error) {
	next, err := parseUpdateChannel(raw)
	if err != nil {
		return "", err
	}
	inner := p.lock()
	prev := inner.file.UpdateChannel
	inner.file.UpdateChannel = next
	if err := saveConfig(inner.configPath, inner.file); err != nil {
		inner.file.UpdateChannel = prev
		p.unlock()
		return "", err
	}
	p.unlock()
	return next, nil
}

type channelAwareProvider struct {
	stable  updater.Provider
	beta    updater.Provider
	channel func() UpdateChannel
}

func (p *channelAwareProvider) Name() string {
	return p.active().Name()
}

func (p *channelAwareProvider) Check(ctx context.Context, req updater.CheckRequest) (*updater.Release, error) {
	return p.active().Check(ctx, req)
}

func (p *channelAwareProvider) Download(ctx context.Context, r *updater.Release, dst io.Writer, onProgress func(written, total int64)) error {
	if r != nil && r.Channel == "prerelease" {
		return p.beta.Download(ctx, r, dst, onProgress)
	}
	return p.active().Download(ctx, r, dst, onProgress)
}

func (p *channelAwareProvider) active() updater.Provider {
	if p.channel != nil && p.channel() == UpdateChannelBeta {
		return p.beta
	}
	return p.stable
}

func configureUpdater(app *application.App, portal *Portal) error {
	token := firstNonEmpty(
		os.Getenv("DEVPORTAL_GITHUB_TOKEN"),
		os.Getenv("GITHUB_TOKEN"),
		os.Getenv("GH_TOKEN"),
	)
	stable, err := github.New(github.Config{
		Repository:    updateRepository,
		Token:         token,
		Prerelease:    false,
		ChecksumAsset: updateChecksumAsset,
		AssetMatcher:  zipFirstMatcher,
	})
	if err != nil {
		return err
	}
	beta, err := github.New(github.Config{
		Repository:    updateRepository,
		Token:         token,
		Prerelease:    true,
		ChecksumAsset: updateChecksumAsset,
		AssetMatcher:  zipFirstMatcher,
	})
	if err != nil {
		return err
	}
	provider := &channelAwareProvider{
		stable: stable,
		beta:   beta,
		channel: func() UpdateChannel {
			return portal.UpdateChannel()
		},
	}
	return app.Updater.Init(updater.Config{
		CurrentVersion: runningVersion(),
		Providers:      []updater.Provider{provider},
		Window: &updater.BuiltinWindow{
			CSS: updaterWindowCSS,
			Options: updater.WindowOptions{
				Title:  "DevPortal のアップデート",
				Width:  520,
				Height: 540,
			},
		},
	})
}

const updaterWindowCSS = `
:root {
  --accent: #007aff;
  --radius: 12px;
  --font: -apple-system, BlinkMacSystemFont, "SF Pro Text", "Hiragino Sans",
          "Hiragino Kaku Gothic ProN", system-ui, sans-serif;
}
@media (prefers-color-scheme: dark) {
  :root { --accent: #0a84ff; }
}
`

func installApplicationMenu(app *application.App) {
	menu := app.Menu.New()
	if runtime.GOOS == "darwin" {
		appMenu := menu.AddSubmenu("DevPortal")
		appMenu.AddRole(application.About)
		appMenu.AddSeparator()
		appMenu.Add("アップデートを確認…").OnClick(func(*application.Context) {
			go runCheckAndInstall(app)
		})
		appMenu.AddSeparator()
		appMenu.AddRole(application.ServicesMenu)
		appMenu.AddSeparator()
		appMenu.AddRole(application.Hide)
		appMenu.AddRole(application.HideOthers)
		appMenu.AddRole(application.UnHide)
		appMenu.AddSeparator()
		appMenu.AddRole(application.Quit)
		menu.AddRole(application.EditMenu)
		menu.AddRole(application.WindowMenu)
	} else {
		appMenu := menu.AddSubmenu("DevPortal")
		appMenu.Add("アップデートを確認…").OnClick(func(*application.Context) {
			go runCheckAndInstall(app)
		})
		appMenu.AddSeparator()
		appMenu.AddRole(application.Quit)
	}
	app.Menu.SetApplicationMenu(menu)
}

func runCheckAndInstall(app *application.App) {
	if app == nil || app.Updater == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), updateInstallTimeout)
	defer cancel()
	if err := app.Updater.CheckAndInstall(ctx); err != nil {
		logUpdate(app, "update check failed", err)
	}
}

func checkForUpdatesSilently(ctx context.Context, app *application.App) {
	if app == nil || app.Updater == nil {
		return
	}
	if updateInProgress(app.Updater.State()) {
		return
	}

	checkCtx, cancel := context.WithTimeout(ctx, updateCheckTimeout)
	defer cancel()
	if _, err := app.Updater.Check(checkCtx); err != nil {
		logUpdate(app, "silent update check failed", err)
	}
}

func updateInProgress(state updater.State) bool {
	switch state {
	case updater.StateChecking, updater.StateDownloading, updater.StateVerifying, updater.StateInstalling, updater.StateReady:
		return true
	default:
		return false
	}
}

func pollForUpdates(ctx context.Context, app *application.App) {
	timer := time.NewTimer(updateCheckStartupDelay)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			checkForUpdatesSilently(ctx, app)
			timer.Reset(updateCheckInterval)
		}
	}
}

func logUpdate(app *application.App, msg string, err error) {
	if app != nil && app.Logger != nil {
		app.Logger.Error(msg, slog.Any("error", err))
		return
	}
	slog.Error(msg, slog.Any("error", err))
}

func runningVersion() string {
	return normalizeAppVersion(currentVersion)
}

func normalizeAppVersion(version string) string {
	version = strings.TrimSpace(version)
	if strings.HasPrefix(version, "v") || strings.HasPrefix(version, "V") {
		version = strings.TrimSpace(version[1:])
	}
	if version == "" {
		return "0.1.0"
	}
	return version
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func zipFirstMatcher(req updater.CheckRequest, assets []github.ReleaseAsset) int {
	zips := make([]github.ReleaseAsset, 0, len(assets))
	index := make([]int, 0, len(assets))
	for i, asset := range assets {
		name := strings.ToLower(asset.Name)
		if strings.HasSuffix(name, ".zip") || strings.HasSuffix(name, ".tar.gz") || strings.HasSuffix(name, ".tgz") {
			zips = append(zips, asset)
			index = append(index, i)
		}
	}
	if len(zips) > 0 {
		if picked := github.DefaultAssetMatcher(req, zips); picked >= 0 {
			return index[picked]
		}
	}
	return github.DefaultAssetMatcher(req, assets)
}
