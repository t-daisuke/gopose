// Package e2e は gopose のエンドツーエンドテストを提供します。
package e2e

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/harakeishi/gopose/internal/generator"
	"github.com/harakeishi/gopose/internal/logger"
	"github.com/harakeishi/gopose/internal/scanner"
	"github.com/harakeishi/gopose/pkg/types"
)

// mockPortDetector はテスト用のモックポート検出器です。
type mockPortDetector struct {
	usedPorts []int
}

func newMockPortDetector(usedPorts []int) *mockPortDetector {
	return &mockPortDetector{usedPorts: usedPorts}
}

func (m *mockPortDetector) DetectUsedPorts(ctx context.Context) ([]int, error) {
	return m.usedPorts, nil
}

func (m *mockPortDetector) DetectUsedPortsInRange(ctx context.Context, portRange types.PortRange) ([]int, error) {
	var result []int
	for _, port := range m.usedPorts {
		if port >= portRange.Start && port <= portRange.End {
			result = append(result, port)
		}
	}
	return result, nil
}

func (m *mockPortDetector) IsPortInUse(ctx context.Context, port int) (bool, error) {
	for _, p := range m.usedPorts {
		if p == port {
			return true, nil
		}
	}
	return false, nil
}

// mockNetworkDetector はテスト用のモックネットワーク検出器です。
type mockNetworkDetector struct {
	networks []scanner.NetworkInfo
}

func newMockNetworkDetector(networks []scanner.NetworkInfo) *mockNetworkDetector {
	return &mockNetworkDetector{networks: networks}
}

func (m *mockNetworkDetector) DetectNetworks(ctx context.Context) ([]scanner.NetworkInfo, error) {
	return m.networks, nil
}

func TestBasicPortConflictDetection(t *testing.T) {
	// テスト: ポート衝突検知が正しく動作すること
	ctx := context.Background()
	nopLogger := &logger.NopLogger{}

	// システムで使用中のポート: 8080
	mockDetector := newMockPortDetector([]int{8080})
	mockNetDetector := newMockNetworkDetector(nil)

	unifiedDetector := scanner.NewUnifiedConflictDetectorImpl(mockDetector, mockNetDetector, nopLogger)

	// テスト用 compose ファイルを作成
	config := createTestComposeConfig(t, map[string]int{
		"web": 8080, // システムポートと衝突
		"api": 3000, // 衝突なし
	})

	// 衝突検知を実行
	conflictInfo, err := unifiedDetector.DetectConflicts(ctx, config, "test-project")
	require.NoError(t, err)

	// 衝突が検出されることを確認
	assert.True(t, conflictInfo.HasConflicts(), "conflicts should be detected")
	assert.True(t, conflictInfo.HasPortConflicts(), "port conflicts should be detected")
	assert.Len(t, conflictInfo.PortConflicts, 1, "should detect 1 port conflict")

	// 衝突の詳細を確認
	conflict := conflictInfo.PortConflicts[0]
	assert.Equal(t, 8080, conflict.Port)
	assert.Equal(t, types.ConflictTypeSystem, conflict.Type)
	assert.Equal(t, "web", conflict.ServiceName)
}

func TestComposeInternalPortConflictDetection(t *testing.T) {
	// テスト: Compose 内での重複ポート検知
	ctx := context.Background()
	nopLogger := &logger.NopLogger{}

	// システムで使用中のポートはなし
	mockDetector := newMockPortDetector([]int{})
	mockNetDetector := newMockNetworkDetector(nil)

	unifiedDetector := scanner.NewUnifiedConflictDetectorImpl(mockDetector, mockNetDetector, nopLogger)

	// conflict-compose.yml と同等の設定（同じポートを使う2つのサービス）
	config := &types.ComposeConfig{
		Services: map[string]types.Service{
			"web1": {
				Name:  "web1",
				Image: "nginx:alpine",
				Ports: []types.PortMapping{{Host: 8080, Container: 80, Protocol: "tcp"}},
			},
			"web2": {
				Name:  "web2",
				Image: "nginx:alpine",
				Ports: []types.PortMapping{{Host: 8080, Container: 80, Protocol: "tcp"}},
			},
		},
	}

	// 衝突検知を実行
	conflictInfo, err := unifiedDetector.DetectConflicts(ctx, config, "test-project")
	require.NoError(t, err)

	// Compose 内部での衝突が検出されることを確認
	assert.True(t, conflictInfo.HasPortConflicts(), "compose internal port conflicts should be detected")
	assert.Len(t, conflictInfo.PortConflicts, 1, "should detect 1 internal port conflict")

	conflict := conflictInfo.PortConflicts[0]
	assert.Equal(t, 8080, conflict.Port)
	assert.Equal(t, types.ConflictTypeCompose, conflict.Type)
}

func TestOverrideYmlGeneration(t *testing.T) {
	// テスト: override.yml が正しく生成されること
	ctx := context.Background()
	nopLogger := &logger.NopLogger{}

	// システムで使用中のポート: 8080
	mockDetector := newMockPortDetector([]int{8080})
	mockNetDetector := newMockNetworkDetector(nil)

	unifiedDetector := scanner.NewUnifiedConflictDetectorImpl(mockDetector, mockNetDetector, nopLogger)
	allocator := scanner.NewPortAllocatorImpl(mockDetector, nopLogger)
	overrideGen := generator.NewUnifiedOverrideGeneratorImpl(allocator, nopLogger)

	// 衝突のある compose 設定
	config := createTestComposeConfig(t, map[string]int{
		"web": 8080,
	})

	// 衝突検知
	conflictInfo, err := unifiedDetector.DetectConflicts(ctx, config, "test-project")
	require.NoError(t, err)
	require.True(t, conflictInfo.HasConflicts())

	// 衝突解決
	portConfig := types.PortConfig{
		Range:             types.PortRange{Start: 8000, End: 9999},
		ExcludePrivileged: true,
	}
	err = overrideGen.ResolveConflicts(ctx, conflictInfo, types.StrategyMinimalChange, portConfig)
	require.NoError(t, err)

	// override.yml 生成
	override, err := overrideGen.GenerateFromConflicts(ctx, config, conflictInfo)
	require.NoError(t, err)
	require.NotNil(t, override)

	// override.yml の内容を検証
	assert.Contains(t, override.Services, "web", "web service should be in override")
	webOverride := override.Services["web"]
	assert.Len(t, webOverride.Ports, 1, "web service should have 1 port mapping")
	assert.NotEqual(t, 8080, webOverride.Ports[0].Host, "port should be changed from 8080")
}

func TestResolvedPortValidity(t *testing.T) {
	// テスト: 解決後のポートが妥当であること
	ctx := context.Background()
	nopLogger := &logger.NopLogger{}

	// システムで使用中のポート
	mockDetector := newMockPortDetector([]int{8080, 8081, 8082})
	mockNetDetector := newMockNetworkDetector(nil)

	unifiedDetector := scanner.NewUnifiedConflictDetectorImpl(mockDetector, mockNetDetector, nopLogger)
	allocator := scanner.NewPortAllocatorImpl(mockDetector, nopLogger)
	overrideGen := generator.NewUnifiedOverrideGeneratorImpl(allocator, nopLogger)

	// 複数の衝突を持つ compose 設定
	config := createTestComposeConfig(t, map[string]int{
		"web":   8080,
		"api":   8081,
		"admin": 8082,
	})

	// 衝突検知
	conflictInfo, err := unifiedDetector.DetectConflicts(ctx, config, "test-project")
	require.NoError(t, err)
	require.True(t, conflictInfo.HasConflicts())

	// ポート範囲を指定して衝突解決
	portRange := types.PortRange{Start: 8000, End: 9999}
	portConfig := types.PortConfig{
		Range:             portRange,
		ExcludePrivileged: true,
	}
	err = overrideGen.ResolveConflicts(ctx, conflictInfo, types.StrategyMinimalChange, portConfig)
	require.NoError(t, err)

	// 解決されたポートを収集
	resolvedPorts := make(map[int]bool)
	for _, conflict := range conflictInfo.PortConflicts {
		require.NotNil(t, conflict.Resolution, "conflict for port %d should have resolution", conflict.Port)

		resolvedPort := conflict.Resolution.ResolvedPort

		// 範囲内であることを確認
		assert.GreaterOrEqual(t, resolvedPort, portRange.Start,
			"resolved port %d should be >= %d", resolvedPort, portRange.Start)
		assert.LessOrEqual(t, resolvedPort, portRange.End,
			"resolved port %d should be <= %d", resolvedPort, portRange.End)

		// 重複がないことを確認
		assert.False(t, resolvedPorts[resolvedPort],
			"resolved port %d should not be duplicated", resolvedPort)
		resolvedPorts[resolvedPort] = true
	}
}

func TestNoConflictScenario(t *testing.T) {
	// テスト: 衝突がない場合は空の衝突情報が返されること
	ctx := context.Background()
	nopLogger := &logger.NopLogger{}

	// システムで使用中のポートはなし
	mockDetector := newMockPortDetector([]int{})
	mockNetDetector := newMockNetworkDetector(nil)

	unifiedDetector := scanner.NewUnifiedConflictDetectorImpl(mockDetector, mockNetDetector, nopLogger)

	config := createTestComposeConfig(t, map[string]int{
		"web": 8080,
		"api": 3000,
	})

	conflictInfo, err := unifiedDetector.DetectConflicts(ctx, config, "test-project")
	require.NoError(t, err)

	assert.False(t, conflictInfo.HasConflicts(), "no conflicts should be detected")
	assert.Empty(t, conflictInfo.PortConflicts)
}
