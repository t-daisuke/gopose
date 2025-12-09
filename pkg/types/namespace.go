package types

import "fmt"

// NamespaceConfig は namespace の設定を表します。
// namespace は複数の開発者/環境間でポート範囲を分離するための機能です。
type NamespaceConfig struct {
	// Name は namespace の識別名です
	Name string `yaml:"name" json:"name"`

	// PortOffset は namespace 間のポート間隔です
	// 例: PortOffset=1000 の場合、namespace "dev" は 8000-8999、
	//     namespace "staging" は 9000-9999 を使用
	PortOffset int `yaml:"port_offset" json:"port_offset"`

	// PortRange は この namespace で使用するポート範囲です
	PortRange PortRange `yaml:"port_range" json:"port_range"`
}

// Validate は NamespaceConfig の妥当性を検証します。
func (c *NamespaceConfig) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("namespace name is required")
	}

	if c.PortRange.Start > c.PortRange.End {
		return fmt.Errorf("invalid port range: start (%d) > end (%d)",
			c.PortRange.Start, c.PortRange.End)
	}

	if c.PortRange.Start < 1 || c.PortRange.End > 65535 {
		return fmt.Errorf("port range must be between 1 and 65535")
	}

	return nil
}

// Contains はポートが namespace の範囲内かどうかを確認します。
func (c *NamespaceConfig) Contains(port int) bool {
	return port >= c.PortRange.Start && port <= c.PortRange.End
}

// IsEmpty は NamespaceConfig が空（未設定）かどうかを確認します。
func (c *NamespaceConfig) IsEmpty() bool {
	return c.Name == "" && c.PortOffset == 0 && c.PortRange.Start == 0 && c.PortRange.End == 0
}
