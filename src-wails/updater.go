package main

import (
	"context"
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
// Release builds override it with:
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

func configureUpdater(app *application.App) error {
	token := firstNonEmpty(
		os.Getenv("DEVPORTAL_GITHUB_TOKEN"),
		os.Getenv("GITHUB_TOKEN"),
		os.Getenv("GH_TOKEN"),
	)
	gh, err := github.New(github.Config{
		Repository:    updateRepository,
		Token:         token,
		ChecksumAsset: updateChecksumAsset,
		AssetMatcher:  zipFirstMatcher,
	})
	if err != nil {
		return err
	}
	return app.Updater.Init(updater.Config{
		CurrentVersion: runningVersion(),
		Providers:      []updater.Provider{gh},
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

func promptIfUpdateAvailable(ctx context.Context, app *application.App) {
	if app == nil || app.Updater == nil {
		return
	}
	switch app.Updater.State() {
	case updater.StateChecking, updater.StateDownloading, updater.StateVerifying, updater.StateInstalling, updater.StateReady:
		return
	}

	checkCtx, cancel := context.WithTimeout(ctx, updateCheckTimeout)
	rel, err := app.Updater.Check(checkCtx)
	cancel()
	if err != nil {
		logUpdate(app, "silent update check failed", err)
		return
	}
	if rel == nil {
		return
	}
	if ctx.Err() != nil {
		return
	}
	runCheckAndInstall(app)
}

func pollForUpdates(ctx context.Context, app *application.App) {
	timer := time.NewTimer(updateCheckStartupDelay)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			promptIfUpdateAvailable(ctx, app)
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
	version := strings.TrimPrefix(strings.TrimSpace(currentVersion), "v")
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
