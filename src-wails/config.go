package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const appIdentifier = "devportal"

func configDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("設定ディレクトリを解決できませんでした: %w", err)
	}
	dir := filepath.Join(base, appIdentifier)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("設定ディレクトリを作成できませんでした: %w", err)
	}
	return dir, nil
}

func configPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "apps.yml"), nil
}

func runningPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "running.yml"), nil
}

func loadConfig(path string) (ConfigFile, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		empty := ConfigFile{Categories: []CategoryEntry{}, Apps: []AppEntry{}}
		if err := saveConfig(path, empty); err != nil {
			return ConfigFile{}, err
		}
		return empty, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return ConfigFile{}, fmt.Errorf("設定ファイルを読めませんでした: %w", err)
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return ConfigFile{Categories: []CategoryEntry{}, Apps: []AppEntry{}}, nil
	}
	var file ConfigFile
	if err := yaml.Unmarshal(raw, &file); err != nil {
		return ConfigFile{}, fmt.Errorf("YAML の解析に失敗しました: %w", err)
	}
	if file.Categories == nil {
		file.Categories = []CategoryEntry{}
	}
	if file.Apps == nil {
		file.Apps = []AppEntry{}
	}
	return file, nil
}

func saveConfig(path string, file ConfigFile) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("設定ディレクトリを作成できませんでした: %w", err)
	}
	if file.Categories == nil {
		file.Categories = []CategoryEntry{}
	}
	if file.Apps == nil {
		file.Apps = []AppEntry{}
	}
	raw, err := yaml.Marshal(file)
	if err != nil {
		return fmt.Errorf("YAML の書き出しに失敗しました: %w", err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return fmt.Errorf("設定ファイルを保存できませんでした: %w", err)
	}
	return nil
}
