// Package e2e は gopose のエンドツーエンドテストを提供します。
package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/harakeishi/gopose/pkg/types"
)

// testdataDir はテストデータディレクトリのパスを返します。
func testdataDir() string {
	return filepath.Join("testdata")
}

// loadTestComposeFile はテスト用 compose ファイルを読み込みます。
func loadTestComposeFile(t *testing.T, filename string) *types.ComposeConfig {
	t.Helper()
	path := filepath.Join(testdataDir(), filename)
	data, err := os.ReadFile(path)
	require.NoError(t, err, "failed to read compose file: %s", filename)

	var config types.ComposeConfig
	err = yaml.Unmarshal(data, &config)
	require.NoError(t, err, "failed to parse compose file: %s", filename)

	config.FilePath = path
	return &config
}

// createTempDir は一時ディレクトリを作成し、クリーンアップ関数を登録します。
func createTempDir(t *testing.T) string {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "gopose-e2e-*")
	require.NoError(t, err, "failed to create temp dir")
	t.Cleanup(func() {
		os.RemoveAll(tmpDir)
	})
	return tmpDir
}

// writeComposeFile は compose 設定を YAML ファイルに書き込みます。
func writeComposeFile(t *testing.T, dir string, config *types.ComposeConfig) string {
	t.Helper()
	data, err := yaml.Marshal(config)
	require.NoError(t, err, "failed to marshal compose config")

	path := filepath.Join(dir, "docker-compose.yml")
	err = os.WriteFile(path, data, 0644)
	require.NoError(t, err, "failed to write compose file")

	return path
}

// createTestComposeConfig はテスト用の ComposeConfig を生成します。
// services マップは サービス名 -> ホストポート のマッピングです。
func createTestComposeConfig(t *testing.T, services map[string]int) *types.ComposeConfig {
	t.Helper()
	config := &types.ComposeConfig{
		Services: make(map[string]types.Service),
	}

	for name, port := range services {
		config.Services[name] = types.Service{
			Name:  name,
			Image: "nginx:alpine",
			Ports: []types.PortMapping{
				{
					Host:      port,
					Container: 80,
					Protocol:  "tcp",
				},
			},
		}
	}

	return config
}

// assertPortInRange はポートが指定範囲内にあることを検証します。
func assertPortInRange(t *testing.T, port int, portRange types.PortRange, msg string) {
	t.Helper()
	if port < portRange.Start || port > portRange.End {
		t.Errorf("%s: port %d is not in range [%d, %d]",
			msg, port, portRange.Start, portRange.End)
	}
}
