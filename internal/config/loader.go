package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/harakeishi/gopose/pkg/types"
)

// LoadFromFile は指定されたパスから設定ファイルを読み込みます。
func LoadFromFile(path string) (*types.AppConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("設定ファイルの読み込みに失敗: %w", err)
	}

	return LoadFromBytes(data)
}

// LoadFromBytes はバイト配列から設定を読み込みます。
func LoadFromBytes(data []byte) (*types.AppConfig, error) {
	config := DefaultConfig()

	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("設定ファイルの解析に失敗: %w", err)
	}

	return config, nil
}

// LoadWithDefaults は設定ファイルを読み込み、未設定の値にはデフォルト値を適用します。
func LoadWithDefaults(path string) (*types.AppConfig, error) {
	config, err := LoadFromFile(path)
	if err != nil {
		return nil, err
	}

	// デフォルト値とマージ
	defaults := DefaultConfig()

	// 各設定項目がゼロ値の場合はデフォルト値を使用
	if config.Port.Range.Start == 0 && config.Port.Range.End == 0 {
		config.Port.Range = defaults.Port.Range
	}

	if config.File.ComposeFile == "" {
		config.File.ComposeFile = defaults.File.ComposeFile
	}

	if config.File.OverrideFile == "" {
		config.File.OverrideFile = defaults.File.OverrideFile
	}

	if config.Log.Level == "" {
		config.Log.Level = defaults.Log.Level
	}

	if config.Log.Format == "" {
		config.Log.Format = defaults.Log.Format
	}

	return config, nil
}

// FindConfigFile は設定ファイルを検索します。
// 以下の順序で検索します:
// 1. .gopose.yaml（カレントディレクトリ）
// 2. .gopose.yml（カレントディレクトリ）
// 3. gopose.yaml（カレントディレクトリ）
// 4. gopose.yml（カレントディレクトリ）
func FindConfigFile(dir string) (string, error) {
	candidates := []string{
		".gopose.yaml",
		".gopose.yml",
		"gopose.yaml",
		"gopose.yml",
	}

	for _, candidate := range candidates {
		path := dir + "/" + candidate
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("設定ファイルが見つかりません")
}
