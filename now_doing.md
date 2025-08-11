# gopose改良計画 - 複数リポジトリ環境対応

## 📖 背景
goposeは現在開発中の「--with」オプションで複数のDocker Composeファイルを統合できるようになりました。しかし、実際のプロジェクトでは以下の制限により使用できないケースがあります：
- `build`フィールドを持つサービスが統合できない
- `profiles`フィールドを持つサービスがエラーになる
- 外部ネットワークの扱いが不明確

このドキュメントでは、これらの制限を段階的に解消し、実プロジェクトで使えるようにする計画を示します。

## 🎯 目標
メインアプリケーション、リバースプロキシ、データベースフィクスチャなど、複数リポジトリに分散したDocker Compose環境を`gopose`で統合管理できるようにする

## 📍 現状の構成例

以下のような複数リポジトリ構成を想定します：

```
main-app/ (メインプロジェクト)
├── compose.yaml
│   ├── web-nginx (build: ./docker/production/nginx/Dockerfile) ← 問題点①
│   └── app-server (build: ./docker/production/php-fpm/Dockerfile) ← 問題点①
│   └── networks: shared_network (external: true) ← 問題点③
│
reverse-proxy/ (共有インフラ)
├── docker-compose.yml
│   ├── proxy (image: ghcr.io/example/nginx:stable) ✓ OK
│   ├── mail-catcher (image: schickling/mailcatcher) ✓ OK
│   ├── cache (image: containers.example.com/images/memcached:1.6) ✓ OK
│   ├── search (profiles: ["search"]) ← 問題点②
│   ├── storage (image: minio/minio:latest) ✓ OK
│   └── networks: shared_network (external: true) ← 問題点③
│
db-fixtures/ (テストデータ)
├── compose.yaml
│   ├── fixtures (image: containers.example.com/fixtures:latest) ✓ OK
│   ├── schema-tool (image: ghcr.io/k1low/tbls:v1.81.0) ✓ OK
│   ├── database (image: containers.example.com/db:mysql8.0) ✓ OK
│   └── networks: shared_network (external: true) ← 問題点③
```

### 期待する使い方
```bash
# 理想: これが動くようにしたい
cd main-app
gopose up -f compose.yaml \
  --with ../reverse-proxy/docker-compose.yml \
  --with ../db-fixtures/compose.yaml
```

## 🚫 現在の障害と影響

### 問題点① **buildフィールド** (main-app)
- **現象**: `--with`オプションで指定したファイルに`build`があるとエラー
- **影響**: メインアプリケーションが統合できない
- **エラー例**: `依存ファイル compose.yaml のサービス 'web-nginx' にbuildフィールドが含まれています`

### 問題点② **profilesフィールド** (reverse-proxy)
- **現象**: `profiles`を持つサービスがあるとエラー
- **影響**: オプショナルなサービスを含むファイルが統合できない
- **エラー例**: `サービス 'search' にprofilesが含まれています。profilesは現在サポートされていません`

### 問題点③ **外部ネットワーク** (全リポジトリ)
- **現象**: 外部ネットワークの扱いが不明確
- **影響**: ネットワーク衝突解決機能が正しく動作しない可能性
- **懸念**: 既存の外部ネットワークとの接続が保証されない

## 📈 インクリメンタル改良計画

### Phase 1: profilesエラーを警告に変更 (工数: 1時間)
**目的:** profilesを持つファイルを統合可能にする
**理由:** 最も簡単な修正で、影響範囲が最小

```go
// cmd/up.go validateWithServices関数内 L374-376
// 変更前:
if _, hasProfiles := service["profiles"]; hasProfiles {
    return fmt.Errorf("依存ファイル %s のサービス '%s' にprofilesが含まれています", withFile, serviceName)
}

// 変更後:
if _, hasProfiles := service["profiles"]; hasProfiles {
    logger.Warn(ctx, "profilesフィールドは無視されます", 
        types.Field{Key: "service", Value: serviceName},
        types.Field{Key: "file", Value: withFile})
    // エラーを返さずに処理を継続
}
```

**効果:** 
- reverse-proxyが統合可能になる（searchサービスは無視される）
- 既存の動作への影響なし

### Phase 2: メインファイルのbuild許可 (工数: 2時間)
**目的:** buildフィールドを持つファイルをメインとして使えるようにする
**理由:** メインプロジェクトは通常buildを使うため、これが最重要

```go
// cmd/up.go mergeComposeFiles関数を修正
func mergeComposeFiles(ctx context.Context, mainFile string, withFiles []string, logger logger.Logger) (*types.ComposeConfig, error) {
    // ... 既存の処理 ...
    
    // 重要: メインファイルは検証対象外とする
    // withファイルのみバリデーション
    for _, withFile := range withFiles {
        // withFileに対してのみvalidateWithServicesを実行
        if err := validateSingleFile(withFile); err != nil {
            return nil, err
        }
    }
    
    return config, nil
}
```

**効果:**
- main-appをメインファイルとして使用可能
- `gopose up -f compose.yaml --with ../db-fixtures/compose.yaml` が動作

### Phase 3: 外部ネットワーク対応 (工数: 1時間)
**目的:** 外部ネットワークを適切に処理する
**理由:** 全リポジトリが外部ネットワークを使用しているため必須

```go
// internal/scanner/network_detector.go
func (d *DockerNetworkDetector) DetectNetworkConflicts(ctx context.Context, config *types.ComposeConfig) ([]types.NetworkConflict, error) {
    for name, network := range config.Networks {
        if network.External {
            // 外部ネットワークは衝突検出の対象外
            logger.Debug(ctx, "外部ネットワークをスキップ", 
                types.Field{Key: "network", Value: name})
            continue
        }
        // 内部ネットワークのみ衝突検出
    }
}
```

**効果:**
- 外部ネットワーク使用時のエラーを回避
- 既存の外部ネットワークへの接続を維持

### Phase 4: profilesの完全サポート (工数: 3日) - オプショナル
**目的:** profilesを活用した柔軟な構成管理
**優先度:** 低（Phase 1で基本動作は可能なため）

```go
// --profileフラグを追加してDocker Composeに渡す
var activeProfiles []string
upCmd.Flags().StringSliceVar(&activeProfiles, "profile", []string{}, "有効にするprofile")

// docker compose configコマンドに--profileを追加
if len(activeProfiles) > 0 {
    for _, profile := range activeProfiles {
        args = append(args, "--profile", profile)
    }
}
```

### Phase 5: --withでのbuildサポート (工数: 5日) - オプショナル
**目的:** 依存ファイルでもbuildを許可する
**優先度:** 低（イメージのプリビルドで回避可能なため）

```go
// 新しいフラグを追加
var allowBuildInWith bool
upCmd.Flags().BoolVar(&allowBuildInWith, "allow-build-in-with", false, "依存ファイルでbuildを許可")
```

## 🔧 今すぐ使える回避策（goposeを修正せずに）

### 方法1: イメージのプリビルドとラッパーファイル
buildフィールドを回避するため、事前にイメージをビルドしてラッパーファイルを作成します。

```bash
# 1. main-appのイメージをビルド
cd main-app
docker compose build
docker tag main-app-web-nginx:latest app-nginx:latest
docker tag main-app-app-server:latest app-server:latest

# 2. imageベースのラッパーファイルを作成
cat > app-wrapper.yml << 'EOF'
version: '3.8'
services:
  web-nginx:
    image: app-nginx:latest
    volumes:  # 元のcompose.yamlからコピー
      - ./:/var/www/app
    networks:
      - shared_network
  app-server:
    image: app-server:latest
    volumes:  # 元のcompose.yamlからコピー
      - ./:/var/www/app
    networks:
      - shared_network
networks:
  shared_network:
    external: true
EOF

# 3. goposeで実行（buildフィールドがないので動作する）
gopose up -f app-wrapper.yml --with ../db-fixtures/compose.yaml
```

### 方法2: 問題のあるフィールドを除外
profilesを持つサービスを別ファイルに分離します。

```bash
# profilesを使わないサービスだけを抽出
cd reverse-proxy
# searchサービス以外を新しいファイルにコピー
cat docker-compose.yml | yq 'del(.services.search)' > proxy-no-profiles.yml

# goposeで実行
gopose up -f ../main-app/app-wrapper.yml \
  --with proxy-no-profiles.yml \
  --with ../db-fixtures/compose.yaml
```

## 🚀 推奨実装順序とその理由

### 優先度HIGH（今週中に実装）
1. **Phase 1: profiles警告化** 
   - 工数: 1時間
   - 理由: 最小の変更で大きな効果
   - リスク: ほぼなし

2. **Phase 2: メインファイルbuild許可**
   - 工数: 2時間
   - 理由: メインプロジェクトの統合に必須
   - リスク: 低（メインファイルのみの変更）

3. **Phase 3: 外部ネットワーク対応**
   - 工数: 1時間
   - 理由: 多くのプロジェクトで使用される
   - リスク: 低

### 優先度LOW（必要に応じて）
4. **Phase 4: profiles完全サポート**
   - 工数: 3日
   - 理由: Phase 1で基本機能は動作するため

5. **Phase 5: --withでbuild許可**
   - 工数: 5日
   - 理由: ラッパーファイルで回避可能

## 📝 段階的リリース戦略

### ステップ1: 最速で動かす（今日中）
```bash
# 現在のブランチで2箇所だけ修正
git checkout add-with-compose-option-v0

# 1. profilesを警告に (L374-376)
# 2. buildチェックをコメントアウト (L364-366)

# テスト実行
make test
cd /path/to/project
./gopose up -f compose.yaml --with ../other/compose.yaml
```

### ステップ2: 個別PR作成（今週中）
各修正を独立したPRとして作成：

1. **PR #1**: "fix: make profiles field warning instead of error"
   - 最小の変更
   - すぐにマージ可能

2. **PR #2**: "feat: allow build field in main compose file"
   - メインファイルのみ許可
   - 段階的な緩和

3. **PR #3**: "fix: skip external networks in conflict detection"
   - ネットワーク機能の修正
   - 独立した変更

## 📌 今すぐ実行できるアクション

### 🔥 最速パス（1時間で完了）
```bash
# 1. ブランチで修正
cd /path/to/gopose
git checkout add-with-compose-option-v0

# 2. 2箇所のみ修正
# cmd/up.go L374-376: profilesエラーを警告に
# cmd/up.go L364-366: buildチェックをスキップ

# 3. ビルドとテスト
make build
./build/gopose up --help

# 4. 実環境で確認
cd /path/to/main-app
/path/to/gopose up -f compose.yaml \
  --with ../reverse-proxy/docker-compose.yml \
  --with ../db-fixtures/compose.yaml
```

これで今日中に動作確認ができ、段階的にPRを作成していけます。