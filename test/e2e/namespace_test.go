// Package e2e は gopose のエンドツーエンドテストを提供します。
package e2e

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/harakeishi/gopose/pkg/types"
)

func TestNamespaceConfigStructure(t *testing.T) {
	// テスト: NamespaceConfig 構造体が正しく定義されていること
	ns := types.NamespaceConfig{
		Name:       "dev",
		PortOffset: 1000,
		PortRange: types.PortRange{
			Start: 8000,
			End:   8999,
		},
	}

	assert.Equal(t, "dev", ns.Name)
	assert.Equal(t, 1000, ns.PortOffset)
	assert.Equal(t, 8000, ns.PortRange.Start)
	assert.Equal(t, 8999, ns.PortRange.End)
}

func TestNamespaceConfigValidation(t *testing.T) {
	// テスト: NamespaceConfig のバリデーションが機能すること
	tests := []struct {
		name    string
		config  types.NamespaceConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: types.NamespaceConfig{
				Name:       "dev",
				PortOffset: 1000,
				PortRange:  types.PortRange{Start: 8000, End: 8999},
			},
			wantErr: false,
		},
		{
			name: "empty name",
			config: types.NamespaceConfig{
				Name:       "",
				PortOffset: 1000,
				PortRange:  types.PortRange{Start: 8000, End: 8999},
			},
			wantErr: true,
		},
		{
			name: "invalid port range - start > end",
			config: types.NamespaceConfig{
				Name:       "dev",
				PortOffset: 1000,
				PortRange:  types.PortRange{Start: 9000, End: 8000},
			},
			wantErr: true,
		},
		{
			name: "invalid port range - out of valid range (low)",
			config: types.NamespaceConfig{
				Name:       "dev",
				PortOffset: 1000,
				PortRange:  types.PortRange{Start: 0, End: 8999},
			},
			wantErr: true,
		},
		{
			name: "invalid port range - out of valid range (high)",
			config: types.NamespaceConfig{
				Name:       "dev",
				PortOffset: 1000,
				PortRange:  types.PortRange{Start: 8000, End: 70000},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestNamespaceConfigContains(t *testing.T) {
	// テスト: Contains メソッドが正しく動作すること
	ns := types.NamespaceConfig{
		Name:       "dev",
		PortOffset: 1000,
		PortRange:  types.PortRange{Start: 8000, End: 8999},
	}

	tests := []struct {
		name     string
		port     int
		expected bool
	}{
		{"port at start", 8000, true},
		{"port at end", 8999, true},
		{"port in middle", 8500, true},
		{"port below range", 7999, false},
		{"port above range", 9000, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ns.Contains(tt.port)
			assert.Equal(t, tt.expected, result)
		})
	}
}
