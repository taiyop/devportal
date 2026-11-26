package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

func apiTokenPathForConfig(cfgPath string) string {
	if strings.TrimSpace(cfgPath) != "" {
		return filepath.Join(filepath.Dir(cfgPath), "api-token")
	}
	dir, err := configDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "api-token")
}

func apiInfoPathForConfig(cfgPath string) string {
	if strings.TrimSpace(cfgPath) != "" {
		return filepath.Join(filepath.Dir(cfgPath), "api.json")
	}
	dir, err := configDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "api.json")
}

func generateAPIToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("API トークンを生成できませんでした: %w", err)
	}
	return "dp_" + hex.EncodeToString(buf), nil
}

func writeSecretFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("設定ディレクトリを作成できませんでした: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("ファイルを書き出せませんでした: %w", err)
	}
	_ = os.Chmod(tmp, 0o600)
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("ファイルを保存できませんでした: %w", err)
	}
	_ = os.Chmod(path, 0o600)
	return nil
}

func loadOrCreateAPIToken(path string) (string, error) {
	if path != "" {
		if raw, err := os.ReadFile(path); err == nil {
			if token := strings.TrimSpace(string(raw)); token != "" {
				return token, nil
			}
		}
	}
	return rotateAPIToken(path)
}

func rotateAPIToken(path string) (string, error) {
	token, err := generateAPIToken()
	if err != nil {
		return "", err
	}
	if path != "" {
		if err := writeSecretFile(path, []byte(token+"\n")); err != nil {
			return "", err
		}
	}
	return token, nil
}

func writeAPIInfo(path string, info ControlAPIFileInfo) error {
	if path == "" {
		return nil
	}
	raw, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return fmt.Errorf("API 接続情報を書き出せませんでした: %w", err)
	}
	return writeSecretFile(path, append(raw, '\n'))
}

func removeAPIInfo(path string) {
	if path == "" {
		return
	}
	_ = os.Remove(path)
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
