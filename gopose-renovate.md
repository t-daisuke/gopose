# goposeの理想的な拡張機能 - 外部依存サービスを持つプロジェクトへの対応

## 背景

現在のgoposeは単純なポート競合の解決には優れているが、外部ネットワークや共有リソース（OpenSearch、MySQL等）を使用する複雑なプロジェクトには対応できない。本ドキュメントでは、goposeがこのようなプロジェクトに対応するために必要な機能拡張を提案する。

## 1. 依存サービスの自動複製機能

### 概要
外部サービスを自動的に検出し、プロジェクトごとに複製する機能。

### 設定例
```yaml
# gopose.yaml（プロジェクトルート）
gopose:
  duplicate_external_services: true
  service_mapping:
    opensearch:
      original: "opensearch:9200"
      strategy: "duplicate"  # 複製する
      prefix: "${PROJECT_NAME}_"
    mysql:
      original: "mysql:3306"
      strategy: "duplicate"
      prefix: "${PROJECT_NAME}_"
```

### 動作イメージ
```bash
# worktree1で実行
gopose up -d
# 結果:
# → opensearch_worktree1:9200 を自動起動
# → mysql_worktree1:3306 を自動起動
# → OPENSEARCH_API_BASE_URL を自動的に書き換え

# worktree2で実行（同時実行可能）
gopose up -d
# 結果:
# → opensearch_worktree2:9201 を自動起動
# → mysql_worktree2:3307 を自動起動
# → 環境変数も自動調整
```

## 2. ネームスペース分離機能

### 概要
データレベルでの分離を実現し、同一サービスを共有しながらもデータの競合を防ぐ。

### 設定例
```yaml
gopose:
  namespace_isolation: true
  shared_services:
    - name: opensearch
      isolation_strategy: "index_prefix"  # インデックス名にプレフィックス
      prefix_pattern: "${WORKTREE_NAME}_"
    - name: mysql
      isolation_strategy: "database_prefix"  # DB名にプレフィックス
      prefix_pattern: "test_${WORKTREE_NAME}_"
```

### 実行結果
```
worktree1:
  OpenSearch index: "worktree1_products_pa0100001"
  MySQL database: "test_worktree1_colorme"

worktree2:
  OpenSearch index: "worktree2_products_pa0100001"
  MySQL database: "test_worktree2_colorme"
```

## 3. 環境変数の動的書き換え

### 概要
ハードコードされた接続先を自動的に検出し、適切に書き換える。

### 処理フロー
```bash
# 元の環境変数
OPENSEARCH_API_BASE_URL="http://opensearch:9200"
DB_HOST="mysql"
DB_PORT="3306"

# goposeが自動変換
OPENSEARCH_API_BASE_URL="http://opensearch_${GOPOSE_PROJECT_ID}:${GOPOSE_OPENSEARCH_PORT}"
DB_HOST="mysql_${GOPOSE_PROJECT_ID}"
DB_PORT="${GOPOSE_MYSQL_PORT}"
```

### 実装例（Go）
```go
func (g *Gopose) RewriteEnvironmentVariables(env map[string]string) map[string]string {
    replacements := g.Config.EnvironmentMappings
    
    for key, value := range env {
        for _, rule := range replacements {
            if strings.Contains(value, rule.Pattern) {
                env[key] = strings.ReplaceAll(value, rule.Pattern, rule.Replace)
            }
        }
    }
    
    // 動的変数の展開
    env = g.ExpandVariables(env)
    
    return env
}
```

## 4. サービスディスカバリー機能

### 概要
プロキシモードを提供し、アプリケーションコードの変更なしに動的ポートに対応。

### アーキテクチャ
```
[プロキシモード構成図]

┌─────────────┐     ┌──────────────┐     ┌─────────────────┐
│   PHP App   │────▶│ gopose-proxy │────▶│   OpenSearch    │
│             │     │  (Port 9200) │     │ (Dynamic Port)  │
└─────────────┘     └──────────────┘     └─────────────────┘
                           │
                    ルーティングテーブル
                    ┌─────────────────┐
                    │ worktree1→9201  │
                    │ worktree2→9202  │
                    │ worktree3→9203  │
                    └─────────────────┘
```

### 設定例
```yaml
gopose:
  service_discovery:
    enabled: true
    mode: "proxy"  # proxy | dns | direct
    proxy:
      port: 9200  # 固定ポート
      backend_detection: "automatic"
```

## 5. テストモード対応

### 概要
CI/CDや並列テストのための一時的な独立環境を提供。

### コマンド例
```bash
# 独立環境でテスト実行
gopose test --isolated --cleanup-after
# → 完全に独立した環境を作成
# → テスト実行
# → 自動的にクリーンアップ

# 並列テスト実行
gopose test --parallel --max-instances=4
# → 最大4つの独立環境を作成
# → テストを並列実行
```

### CI/CD設定例（GitHub Actions）
```yaml
- name: Run parallel tests
  run: |
    gopose test \
      --parallel \
      --ephemeral \
      --report-format=junit \
      --output=test-results.xml
```

## 6. 詳細な設定ファイル仕様

### .gopose.yml（完全版）
```yaml
version: "1.0"

# 分離レベルの設定
isolation:
  level: "full"  # full | partial | none
  cleanup_on_exit: true

# サービスごとの設定
services:
  opensearch:
    duplicate: true
    port_range: [9200, 9299]
    health_check:
      endpoint: "/_cluster/health"
      timeout: 30s
    data_isolation:
      type: "index_prefix"
      prefix: "${WORKTREE_NAME}_"
      cleanup: true
    resources:
      memory_limit: "512m"
      cpu_limit: "0.5"
  
  mysql:
    duplicate: false  # 共有する
    data_isolation:
      type: "database"
      prefix: "test_${WORKTREE_NAME}_"
      cleanup: true
      init_script: "./tests/init.sql"
    connection_pool:
      max_connections: 10

# 環境変数のマッピング
environment_mapping:
  - pattern: "http://opensearch:9200"
    replace: "http://opensearch_${INSTANCE_ID}:${OPENSEARCH_PORT}"
  - pattern: "mysql:3306"
    replace: "mysql_${INSTANCE_ID}:${MYSQL_PORT}"
  - pattern: "redis://redis:6379"
    replace: "redis://redis_${INSTANCE_ID}:${REDIS_PORT}"

# ネットワーク設定
network:
  isolation: true
  name_pattern: "${PROJECT_NAME}_${BRANCH_NAME}_network"
  driver: "bridge"
  ipam:
    subnet: "172.${SUBNET_ID}.0.0/16"

# フック
hooks:
  pre_up:
    - "echo 'Starting isolated environment for ${PROJECT_NAME}'"
    - "./scripts/prepare-test-data.sh"
  post_up:
    - "./scripts/wait-for-services.sh"
  pre_down:
    - "./scripts/backup-test-results.sh"
  post_down:
    - "echo 'Cleanup completed'"

# リソース制限
resources:
  max_instances: 5
  total_memory_limit: "4G"
  auto_cleanup_after: "2h"
```

## 7. CLIコマンドの拡張

### 基本コマンド
```bash
# 独立環境で起動
gopose up --isolated [--detach]

# 特定の設定ファイルを使用
gopose -f .gopose.prod.yml up

# ドライラン（変更内容を確認）
gopose up --dry-run
```

### 管理コマンド
```bash
# リソース使用状況の確認
gopose resources [--format=table|json]
# 出力例:
# PROJECT              CPU    MEMORY   SERVICES           UPTIME
# feature-payment      0.3    512MB    opensearch,mysql   2h30m
# feature-search       0.2    384MB    opensearch         45m

# 共有リソースの状況確認
gopose status --show-shared-resources
# 出力例:
# Shared Resources:
# - mysql (shared): 3 connections from 2 projects
# - redis (shared): idle
# 
# Isolated Resources:
# - opensearch_payment:9201 (150MB) - feature-payment
# - opensearch_search:9202 (145MB) - feature-search

# インスタンスの管理
gopose instances list
gopose instances stop feature-payment
gopose instances remove --all --older-than=24h
```

### デバッグコマンド
```bash
# 設定の検証
gopose config validate

# 接続テスト
gopose test-connection opensearch

# ログの確認
gopose logs [service] [--tail=100] [--follow]

# トラブルシューティング
gopose doctor
# 出力例:
# ✓ Docker daemon is running
# ✓ Required networks are available
# ✓ Port ranges are not blocked
# ⚠ Low disk space (< 5GB available)
# ✓ All required images are present
```

## 8. 実装の技術的詳細

### サービス複製のアルゴリズム
```go
type ServiceDuplicator struct {
    config     *Config
    docker     *DockerClient
    portFinder *PortFinder
}

func (sd *ServiceDuplicator) DuplicateService(original Service) (*Service, error) {
    // 1. 元のサービスの設定を取得
    originalConfig := sd.docker.InspectService(original.Name)
    
    // 2. 新しいサービス名を生成
    newName := fmt.Sprintf("%s_%s", original.Name, sd.config.ProjectID)
    
    // 3. 利用可能なポートを探す
    newPort := sd.portFinder.FindAvailable(
        original.Port,
        sd.config.PortRange,
    )
    
    // 4. 新しいサービスの設定を作成
    newConfig := sd.cloneConfig(originalConfig)
    newConfig.Name = newName
    newConfig.Ports = []Port{{Host: newPort, Container: original.Port}}
    
    // 5. ボリュームを分離
    if sd.config.IsolateVolumes {
        newConfig.Volumes = sd.isolateVolumes(originalConfig.Volumes)
    }
    
    // 6. サービスを起動
    return sd.docker.CreateService(newConfig)
}
```

### 環境変数の動的解決
```go
type EnvironmentResolver struct {
    mappings []EnvironmentMapping
    vars     map[string]string
}

func (er *EnvironmentResolver) Resolve(env []string) []string {
    resolved := make([]string, 0, len(env))
    
    for _, e := range env {
        parts := strings.SplitN(e, "=", 2)
        if len(parts) != 2 {
            resolved = append(resolved, e)
            continue
        }
        
        key, value := parts[0], parts[1]
        
        // マッピングルールを適用
        for _, mapping := range er.mappings {
            value = strings.ReplaceAll(value, mapping.Pattern, mapping.Replace)
        }
        
        // 変数を展開
        value = os.Expand(value, er.getVar)
        
        resolved = append(resolved, fmt.Sprintf("%s=%s", key, value))
    }
    
    return resolved
}
```

## 9. 実用的な利用シナリオ

### シナリオ1: 機能開発の並列作業
```bash
# 開発者A（決済機能）
cd ~/projects/colorme-user-payment
gopose up --isolated
# → 完全に独立した環境で決済機能を開発
# → opensearch_payment:9201
# → mysql_payment:3307

# 開発者B（検索機能）- 同時作業可能
cd ~/projects/colorme-user-search
gopose up --isolated
# → 検索機能を独立環境で開発
# → opensearch_search:9202
# → mysql_search:3308

# お互いの作業が干渉しない
```

### シナリオ2: CI/CDパイプライン
```yaml
# .github/workflows/test.yml
name: Parallel Testing
on: [push, pull_request]

jobs:
  test:
    strategy:
      matrix:
        test_suite: [unit, integration, e2e]
    steps:
      - uses: actions/checkout@v2
      
      - name: Run tests in isolated environment
        run: |
          gopose test \
            --suite=${{ matrix.test_suite }} \
            --isolated \
            --cleanup-after \
            --junit-output=results-${{ matrix.test_suite }}.xml
      
      - name: Upload test results
        uses: actions/upload-artifact@v2
        with:
          name: test-results
          path: results-*.xml
```

### シナリオ3: レビュー環境の自動構築
```bash
# PRごとに独立環境を作成
PR_NUMBER=123
BRANCH_NAME="feature/new-search"

gopose up \
  --isolated \
  --name="review-pr-${PR_NUMBER}" \
  --env="ENVIRONMENT=review" \
  --expose-ports  # 外部からアクセス可能にする

# レビュー用URLを生成
echo "Review URL: https://pr-${PR_NUMBER}.review.example.com"
```

## 10. まとめ

### 実装による効果

1. **完全な環境分離**
   - 各開発者が独立した環境で作業可能
   - テストデータの競合を完全に排除

2. **並列開発の実現**
   - 複数のworktreeを同時に起動・テスト
   - チーム開発の効率が大幅に向上

3. **CI/CDの高速化**
   - 並列テスト実行が可能
   - テスト時間を大幅に短縮

4. **既存プロジェクトへの導入容易性**
   - docker-compose.yamlの変更不要
   - 設定ファイルの追加のみで対応

5. **リソース管理の自動化**
   - 不要な環境の自動クリーンアップ
   - リソース使用量の可視化

### 実装の優先順位

1. **Phase 1: 基本機能**
   - ポート競合の解決（既存）
   - 環境変数の動的書き換え

2. **Phase 2: サービス複製**
   - 外部サービスの自動複製
   - 基本的な分離機能

3. **Phase 3: 高度な機能**
   - プロキシモード
   - ネームスペース分離
   - リソース管理

4. **Phase 4: エンタープライズ機能**
   - CI/CD統合
   - モニタリング
   - クラスタ対応

これらの機能が実装されれば、goposeは単なるポート競合解決ツールから、**開発環境の完全な分離と管理を実現する強力なツール**へと進化し、大規模なマイクロサービスプロジェクトでも活用できるようになる。
