// Package e2e は gopose のエンドツーエンドテストを提供します。
package e2e

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/harakeishi/gopose/internal/config"
	"github.com/harakeishi/gopose/internal/logger"
	"github.com/harakeishi/gopose/internal/scanner"
	"github.com/harakeishi/gopose/pkg/types"
)

func TestNamespacePortIsolation(t *testing.T) {
	// テスト: 異なる namespace でポート範囲が分離されること
	ctx := context.Background()
	nopLogger := &logger.NopLogger{}

	devNamespace := types.NamespaceConfig{
		Name:       "dev",
		PortOffset: 0,
		PortRange:  types.PortRange{Start: 8000, End: 8999},
	}

	stagingNamespace := types.NamespaceConfig{
		Name:       "staging",
		PortOffset: 1000,
		PortRange:  types.PortRange{Start: 9000, End: 9999},
	}

	// バリデーションが通ることを確認
	require.NoError(t, devNamespace.Validate())
	require.NoError(t, stagingNamespace.Validate())

	// モックで特定のポートを使用中とする
	usedPorts := []int{8080, 9080} // 両方の namespace 範囲でそれぞれ使用中
	mockDetector := newMockPortDetector(usedPorts)

	// dev namespace でポート割り当て
	devAllocator := scanner.NewNamespaceAwarePortAllocator(mockDetector, nopLogger, devNamespace)
	devPort, err := devAllocator.AllocatePort(ctx, types.PortConfig{
		Range:             devNamespace.PortRange,
		ExcludePrivileged: true,
	})
	require.NoError(t, err)
	assert.True(t, devNamespace.Contains(devPort),
		"dev port %d should be in dev namespace range [%d-%d]",
		devPort, devNamespace.PortRange.Start, devNamespace.PortRange.End)

	// staging namespace でポート割り当て
	stagingAllocator := scanner.NewNamespaceAwarePortAllocator(mockDetector, nopLogger, stagingNamespace)
	stagingPort, err := stagingAllocator.AllocatePort(ctx, types.PortConfig{
		Range:             stagingNamespace.PortRange,
		ExcludePrivileged: true,
	})
	require.NoError(t, err)
	assert.True(t, stagingNamespace.Contains(stagingPort),
		"staging port %d should be in staging namespace range [%d-%d]",
		stagingPort, stagingNamespace.PortRange.Start, stagingNamespace.PortRange.End)

	// ポートが異なることを確認
	assert.NotEqual(t, devPort, stagingPort,
		"dev and staging should have different ports")
}

func TestNamespaceConfigLoading(t *testing.T) {
	// テスト: .gopose.yaml から namespace 設定が読み込めること
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".gopose.yaml")

	configContent := `
port:
  range:
    start: 8000
    end: 9999
  exclude_privileged: true
namespace:
  name: dev
  port_offset: 1000
  port_range:
    start: 8000
    end: 8999
`
	require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0644))

	// 設定読み込み
	cfg, err := config.LoadFromFile(configPath)
	require.NoError(t, err)

	// namespace 設定の検証
	assert.Equal(t, "dev", cfg.Namespace.Name)
	assert.Equal(t, 1000, cfg.Namespace.PortOffset)
	assert.Equal(t, 8000, cfg.Namespace.PortRange.Start)
	assert.Equal(t, 8999, cfg.Namespace.PortRange.End)
}

func TestMultipleNamespaceConflictResolution(t *testing.T) {
	// テスト: 複数 namespace 間での衝突解決
	ctx := context.Background()
	nopLogger := &logger.NopLogger{}

	// namespace 設定
	namespaces := []types.NamespaceConfig{
		{Name: "dev", PortRange: types.PortRange{Start: 8000, End: 8999}},
		{Name: "staging", PortRange: types.PortRange{Start: 9000, End: 9999}},
	}

	// 同じサービス構成
	services := map[string]int{
		"web": 8080,
		"api": 3000,
	}

	// 使用中ポート（両方の namespace に影響）
	usedPorts := []int{8080, 8081, 9080, 9081}
	mockDetector := newMockPortDetector(usedPorts)
	mockNetDetector := newMockNetworkDetector(nil)

	for _, ns := range namespaces {
		t.Run(ns.Name, func(t *testing.T) {
			// Compose 設定を作成
			composeConfig := createTestComposeConfig(t, services)

			// 衝突検知
			detector := scanner.NewUnifiedConflictDetectorImpl(mockDetector, mockNetDetector, nopLogger)
			conflicts, err := detector.DetectConflicts(ctx, composeConfig, ns.Name)
			require.NoError(t, err)

			// namespace 対応アロケータで衝突解決
			allocator := scanner.NewNamespaceAwarePortAllocator(mockDetector, nopLogger, ns)

			// 各衝突を解決
			for i := range conflicts.PortConflicts {
				conflict := &conflicts.PortConflicts[i]

				// namespace 範囲内でポート割り当て
				resolvedPort, err := allocator.AllocatePort(ctx, types.PortConfig{
					Range:             ns.PortRange,
					ExcludePrivileged: true,
				})
				require.NoError(t, err)

				// 解決されたポートが namespace 範囲内であることを確認
				assert.True(t, ns.Contains(resolvedPort),
					"resolved port %d should be in namespace %s range [%d-%d]",
					resolvedPort, ns.Name, ns.PortRange.Start, ns.PortRange.End)

				conflict.Resolution = &types.PortResolutionInfo{
					ResolvedPort: resolvedPort,
					Strategy:     types.StrategyMinimalChange,
					Reason:       "namespace aware allocation",
				}
			}
		})
	}
}

func TestNamespaceAwareAllocatorRespectsBoundaries(t *testing.T) {
	// テスト: NamespaceAwarePortAllocator が namespace 境界を尊重すること
	ctx := context.Background()
	nopLogger := &logger.NopLogger{}

	ns := types.NamespaceConfig{
		Name:       "test",
		PortOffset: 0,
		PortRange:  types.PortRange{Start: 8000, End: 8010}, // 小さい範囲
	}

	// ほとんどのポートが使用中
	usedPorts := []int{8000, 8001, 8002, 8003, 8004, 8005, 8006, 8007, 8008}
	mockDetector := newMockPortDetector(usedPorts)

	allocator := scanner.NewNamespaceAwarePortAllocator(mockDetector, nopLogger, ns)

	// 利用可能なポート（8009, 8010）を割り当て
	port1, err := allocator.AllocatePort(ctx, types.PortConfig{
		Range:             ns.PortRange,
		ExcludePrivileged: true,
	})
	require.NoError(t, err)
	assert.True(t, ns.Contains(port1))

	port2, err := allocator.AllocatePort(ctx, types.PortConfig{
		Range:             ns.PortRange,
		Reserved:          []int{port1}, // 前に割り当てたポートを除外
		ExcludePrivileged: true,
	})
	require.NoError(t, err)
	assert.True(t, ns.Contains(port2))
	assert.NotEqual(t, port1, port2)

	// 3つ目の割り当ては失敗するはず（全ポートが使用中または割り当て済み）
	_, err = allocator.AllocatePort(ctx, types.PortConfig{
		Range:             ns.PortRange,
		Reserved:          []int{port1, port2},
		ExcludePrivileged: true,
	})
	assert.Error(t, err, "should fail when no ports available in namespace")
}
