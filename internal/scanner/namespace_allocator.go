package scanner

import (
	"context"
	"fmt"

	"github.com/harakeishi/gopose/internal/errors"
	"github.com/harakeishi/gopose/internal/logger"
	"github.com/harakeishi/gopose/pkg/types"
)

// NamespaceAwarePortAllocator は namespace を考慮したポート割り当てを行う実装です。
type NamespaceAwarePortAllocator struct {
	detector  PortDetector
	logger    logger.Logger
	namespace types.NamespaceConfig
}

// NewNamespaceAwarePortAllocator は新しい NamespaceAwarePortAllocator を作成します。
func NewNamespaceAwarePortAllocator(detector PortDetector, logger logger.Logger, namespace types.NamespaceConfig) *NamespaceAwarePortAllocator {
	return &NamespaceAwarePortAllocator{
		detector:  detector,
		logger:    logger,
		namespace: namespace,
	}
}

// AllocatePort は namespace 範囲内で利用可能なポートを1つ割り当てます。
func (n *NamespaceAwarePortAllocator) AllocatePort(ctx context.Context, config types.PortConfig) (int, error) {
	// namespace 範囲と config 範囲の交差を計算
	effectiveRange := n.calculateEffectiveRange(config.Range)
	if effectiveRange.Start > effectiveRange.End {
		return 0, &errors.AppError{
			Code:    errors.ErrPortRangeInvalid,
			Message: fmt.Sprintf("指定された範囲 [%d-%d] は namespace [%s] の範囲 [%d-%d] と重なりません",
				config.Range.Start, config.Range.End,
				n.namespace.Name, n.namespace.PortRange.Start, n.namespace.PortRange.End),
		}
	}

	// 使用中ポートを取得
	usedPorts, err := n.detector.DetectUsedPortsInRange(ctx, effectiveRange)
	if err != nil {
		return 0, err
	}

	// 除外リストを作成
	excludePorts := make(map[int]bool)
	for _, port := range usedPorts {
		excludePorts[port] = true
	}
	for _, port := range config.Reserved {
		excludePorts[port] = true
	}

	// 特権ポートを除外
	if config.ExcludePrivileged {
		for i := 1; i <= 1023; i++ {
			excludePorts[i] = true
		}
	}

	// 利用可能なポートを順次検索
	for port := effectiveRange.Start; port <= effectiveRange.End; port++ {
		if !excludePorts[port] {
			n.logger.Debug(ctx, "namespace 対応ポート割り当て成功",
				types.Field{Key: "namespace", Value: n.namespace.Name},
				types.Field{Key: "allocated_port", Value: port})
			return port, nil
		}
	}

	return 0, &errors.AppError{
		Code:    errors.ErrPortUnavailable,
		Message: fmt.Sprintf("namespace [%s] の範囲 [%d-%d] に利用可能なポートがありません",
			n.namespace.Name, effectiveRange.Start, effectiveRange.End),
		Fields: map[string]interface{}{
			"namespace":   n.namespace.Name,
			"range_start": effectiveRange.Start,
			"range_end":   effectiveRange.End,
		},
	}
}

// AllocatePorts は namespace 範囲内で指定された数のポートを割り当てます。
func (n *NamespaceAwarePortAllocator) AllocatePorts(ctx context.Context, count int, config types.PortConfig) ([]int, error) {
	if count <= 0 {
		return []int{}, nil
	}

	effectiveRange := n.calculateEffectiveRange(config.Range)
	if effectiveRange.Start > effectiveRange.End {
		return nil, &errors.AppError{
			Code:    errors.ErrPortRangeInvalid,
			Message: fmt.Sprintf("指定された範囲は namespace [%s] の範囲と重なりません", n.namespace.Name),
		}
	}

	usedPorts, err := n.detector.DetectUsedPortsInRange(ctx, effectiveRange)
	if err != nil {
		return nil, err
	}

	excludePorts := make(map[int]bool)
	for _, port := range usedPorts {
		excludePorts[port] = true
	}
	for _, port := range config.Reserved {
		excludePorts[port] = true
	}

	if config.ExcludePrivileged {
		for i := 1; i <= 1023; i++ {
			excludePorts[i] = true
		}
	}

	allocatedPorts := make([]int, 0, count)
	for port := effectiveRange.Start; port <= effectiveRange.End && len(allocatedPorts) < count; port++ {
		if !excludePorts[port] {
			allocatedPorts = append(allocatedPorts, port)
			excludePorts[port] = true
		}
	}

	if len(allocatedPorts) < count {
		return allocatedPorts, &errors.AppError{
			Code:    errors.ErrPortUnavailable,
			Message: fmt.Sprintf("namespace [%s] で要求された数のポートを割り当てできません。要求: %d, 割り当て可能: %d",
				n.namespace.Name, count, len(allocatedPorts)),
		}
	}

	n.logger.Info(ctx, "namespace 対応複数ポート割り当て完了",
		types.Field{Key: "namespace", Value: n.namespace.Name},
		types.Field{Key: "allocated_count", Value: len(allocatedPorts)},
		types.Field{Key: "ports", Value: allocatedPorts})

	return allocatedPorts, nil
}

// AllocatePortsForServices はサービス別に namespace 範囲内でポートを割り当てます。
func (n *NamespaceAwarePortAllocator) AllocatePortsForServices(ctx context.Context, services []types.Service, config types.PortConfig) (map[string]int, error) {
	servicesNeedingPorts := 0
	for _, service := range services {
		if len(service.Ports) > 0 {
			servicesNeedingPorts++
		}
	}

	if servicesNeedingPorts == 0 {
		return make(map[string]int), nil
	}

	effectiveRange := n.calculateEffectiveRange(config.Range)
	if effectiveRange.Start > effectiveRange.End {
		return nil, &errors.AppError{
			Code:    errors.ErrPortRangeInvalid,
			Message: fmt.Sprintf("指定された範囲は namespace [%s] の範囲と重なりません", n.namespace.Name),
		}
	}

	usedPorts, err := n.detector.DetectUsedPortsInRange(ctx, effectiveRange)
	if err != nil {
		return nil, err
	}

	excludePorts := make(map[int]bool)
	for _, port := range usedPorts {
		excludePorts[port] = true
	}
	for _, port := range config.Reserved {
		excludePorts[port] = true
	}

	if config.ExcludePrivileged {
		for i := 1; i <= 1023; i++ {
			excludePorts[i] = true
		}
	}

	result := make(map[string]int)
	currentPort := effectiveRange.Start

	for _, service := range services {
		if len(service.Ports) == 0 {
			continue
		}

		for currentPort <= effectiveRange.End {
			if !excludePorts[currentPort] {
				result[service.Name] = currentPort
				excludePorts[currentPort] = true
				currentPort++
				break
			}
			currentPort++
		}

		if _, found := result[service.Name]; !found {
			return result, fmt.Errorf("namespace [%s] でサービス %s のポート割り当てに失敗: 利用可能なポートがありません",
				n.namespace.Name, service.Name)
		}
	}

	n.logger.Info(ctx, "namespace 対応サービス別ポート割り当て完了",
		types.Field{Key: "namespace", Value: n.namespace.Name},
		types.Field{Key: "service_count", Value: len(result)},
		types.Field{Key: "allocations", Value: result})

	return result, nil
}

// calculateEffectiveRange は config 範囲と namespace 範囲の交差を計算します。
func (n *NamespaceAwarePortAllocator) calculateEffectiveRange(configRange types.PortRange) types.PortRange {
	start := configRange.Start
	end := configRange.End

	// namespace 範囲との交差を計算
	if start < n.namespace.PortRange.Start {
		start = n.namespace.PortRange.Start
	}
	if end > n.namespace.PortRange.End {
		end = n.namespace.PortRange.End
	}

	return types.PortRange{Start: start, End: end}
}

// GetNamespace は現在の namespace 設定を返します。
func (n *NamespaceAwarePortAllocator) GetNamespace() types.NamespaceConfig {
	return n.namespace
}
