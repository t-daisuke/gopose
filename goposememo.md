# goposeとgit worktreeでのDocker Compose運用に関する詳細ガイド

## 目次
1. [goposeとは何か](#goposeとは何か)
2. [なぜこのプロジェクトでgoposeが使えないのか](#なぜこのプロジェクトでgoposeが使えないのか)
3. [git worktreeでの正しい運用方法](#git-worktreeでの正しい運用方法)
4. [具体的なコマンド例](#具体的なコマンド例)
5. [トラブルシューティング](#トラブルシューティング)

---

## goposeとは何か

goposeは、Docker Composeのポート競合を自動的に検出・解決するGoで書かれたツールです。

### 通常の使用例
```bash
# プロジェクトA（ポート8080を使用）
cd project-a
docker compose up -d  # 8080で起動

# プロジェクトB（同じく8080を使いたい）
cd project-b
docker compose up -d  # エラー！ポート8080は既に使用中

# goposeを使うと...
gopose up -d  # 自動的に8081などの空いているポートに変更して起動
```

### goposeの動作原理
1. システムで使用中のポートをスキャン
2. 競合を検出したら、空いているポートを探す
3. `docker-compose.override.yml`を自動生成
4. 変更されたポートでコンテナを起動

---

## なぜこのプロジェクトでgoposeが使えないのか

### 1. プロジェクトのアーキテクチャ

```yaml
# compose.yaml の重要な部分
networks:
  default:
    external: true
    name: colorme_default  # ← 外部ネットワークを使用

environment:
  OPENSEARCH_API_BASE_URL: "http://opensearch:9200"  # ← 固定のホスト名とポート
```

### 2. 共有リソースの存在

```
[colorme_default ネットワーク上のサービス構成]

┌─────────────────────────────────────────────────┐
│           colorme_default Network               │
├─────────────────────────────────────────────────┤
│                                                 │
│  ┌──────────────┐    ┌──────────────┐         │
│  │  OpenSearch  │    │  Fixtures DB  │         │
│  │   Port:9200  │    │   Port:3306   │         │
│  └──────────────┘    └──────────────┘         │
│         ▲                    ▲                 │
│         │                    │                 │
│    接続先は固定          接続先は固定          │
│         │                    │                 │
│  ┌──────────────────────────────────┐         │
│  │     entrance-php-fpm             │         │
│  │  OPENSEARCH_API_BASE_URL:        │         │
│  │  "http://opensearch:9200"        │         │
│  └──────────────────────────────────┘         │
│                                                 │
└─────────────────────────────────────────────────┘
```

### 3. goposeを使った場合の問題

#### 問題1: ポート変更が意味をなさない

```bash
# goposeでworktree1を起動
cd colorme-user
gopose up -d  # nginx: 8080 → 8080（変更なし）

# goposeでworktree2を起動
cd colorme-user-cat2
gopose up -d  # nginx: 8080 → 8081（ポート変更）

# しかし、内部通信は変わらない！
# worktree1のPHP → opensearch:9200
# worktree2のPHP → opensearch:9200  # 同じOpenSearchインスタンス！
```

#### 問題2: テストの競合

```php
// worktree1のテスト
public function testSearchProducts() {
    $this->createTestData('test_product_1');  // OpenSearchに書き込み
    $result = $this->search('test_product_1');
    $this->assertEquals(1, count($result));
}

// worktree2のテスト（同時実行）
public function testSearchProducts() {
    $this->cleanupAllData();  // 全データ削除！
    $this->createTestData('test_product_2');
    // worktree1のテストが失敗する！
}
```

#### 問題3: データベースの汚染

```
worktree1とworktree2が同じDBを使用
→ マイグレーションの競合
→ テストデータの混在
→ トランザクションの競合
→ 予期しないテスト失敗
```

---

## git worktreeでの正しい運用方法

### 解決策: プロジェクト名を統一する

```bash
# すべてのworktreeで同じプロジェクト名を使用
docker compose -p colorme-user up -d
```

### なぜこれが動作するのか

```
[Docker イメージとボリュームマウントの関係]

┌──────────────────────────────────────┐
│     Dockerイメージ（3週間前）        │
│  colorme-user-entrance-php-fpm       │
│                                      │
│  - PHP 7.4                          │
│  - Composer packages                │
│  - System packages                  │
│  - PHP extensions                   │
└──────────────────────────────────────┘
                 ▲
                 │
         イメージを再利用
                 │
┌──────────────────────────────────────┐
│        実行時のマウント              │
├──────────────────────────────────────┤
│                                      │
│  /var/www/app ← ./（現在のworktree） │
│                                      │
│  colorme-user-cat2/                 │
│    ├── app/                         │
│    ├── tests/                       │
│    └── index.php                    │
│                                      │
│  ↑ このコードが実行される           │
└──────────────────────────────────────┘
```

### できること・できないこと

#### ✅ できること（コードレベルの変更）

| 種類 | 例 | 理由 |
|------|-----|------|
| PHPコードの変更 | `app/models/Product.php`の修正 | ボリュームマウントで即反映 |
| 新規ファイルの追加 | `tests/NewTest.php`を作成 | ディレクトリがマウントされている |
| テンプレートの変更 | `templates/*.tpl`の編集 | ファイルシステム経由で読み込まれる |
| 設定ファイルの変更 | `app/configs/config.php`の修正 | PHPが実行時に読み込む |
| JavaScriptの変更 | `js/*.js`の編集 | 静的ファイルとして配信される |

#### ❌ できないこと（イメージレベルの変更）

| 種類 | 例 | 理由 | 対処法 |
|------|-----|------|--------|
| Composerパッケージの追加 | `composer require aws/aws-sdk-php` | vendor/はイメージ内に固定 | イメージの再ビルドが必要 |
| PHP拡張の追加 | `pecl install redis` | コンテナ起動時には遅い | Dockerfileを修正して再ビルド |
| システムパッケージ | `apt-get install imagemagick` | イメージ作成時にインストール | Dockerfileを修正して再ビルド |
| PHPバージョン変更 | PHP 7.4 → 8.0 | ベースイメージの変更が必要 | 新しいイメージをビルド |

---

## 具体的なコマンド例

### 基本的な使用方法

```bash
# 1. worktreeを作成
git worktree add ../colorme-user-feature-x feature-branch

# 2. worktreeに移動
cd ../colorme-user-feature-x

# 3. コンテナを起動（重要: -p colorme-user を忘れずに！）
docker compose -p colorme-user up -d

# 4. テストを実行
docker compose -p colorme-user exec entrance-php-fpm bin/phpunit tests/

# 5. コードを修正
vim app/models/Product.php

# 6. 修正が即座に反映される（再起動不要）
docker compose -p colorme-user exec entrance-php-fpm bin/phpunit tests/app/models/ProductTest.php

# 7. 作業終了時
docker compose -p colorme-user down
```

### エイリアスを使った効率化

```bash
# ~/.zshrc または ~/.bashrc に追加
alias dcu='docker compose -p colorme-user'

# 使用例
dcu up -d
dcu exec entrance-php-fpm bash
dcu exec entrance-php-fpm bin/phpunit tests/
dcu logs -f entrance-php-fpm
dcu down
```

### 複数worktreeの切り替え

```bash
# worktree 1で作業
cd ~/colorme-user-feature-a
dcu up -d
# ... 作業 ...
dcu down  # 必ず停止

# worktree 2に切り替え
cd ~/colorme-user-feature-b
dcu up -d  # 同じイメージ、異なるコード
# ... 作業 ...
dcu down
```

---

## トラブルシューティング

### Q1: なぜDNSエラーが発生するのか？

**症状:**
```
failed to resolve source metadata for containers.git.pepabo.com/...
```

**原因:**
- 新しいworktreeは異なるプロジェクト名を持つ
- 既存のイメージがないため、ビルドしようとする
- 社内レジストリやDocker Hubへのアクセスに失敗

**解決策:**
```bash
# 既存のプロジェクト名を使用
docker compose -p colorme-user up -d
```

### Q2: 複数のworktreeを同時に起動できないか？

**答え:** できません。理由は以下の通り：

1. **同じコンテナ名の競合**
   - `colorme-user-entrance-php-fpm-1`が既に存在

2. **共有リソースの問題**
   - 同じOpenSearch/DBを使用
   - テストデータが混在する

3. **推奨される運用**
   ```bash
   # worktree 1を停止してから
   docker compose -p colorme-user down
   
   # worktree 2を起動
   cd ../other-worktree
   docker compose -p colorme-user up -d
   ```

### Q3: composer.jsonを変更したらどうすればいい？

**手順:**

1. **メインのworktreeで作業**
   ```bash
   cd ~/colorme-user  # メインのworktree
   ```

2. **イメージを再ビルド**
   ```bash
   # DNSが正常な環境で
   docker compose build --no-cache entrance-php-fpm
   ```

3. **新しいイメージで起動**
   ```bash
   docker compose up -d
   ```

4. **他のworktreeでも新しいイメージを使用**
   ```bash
   cd ~/colorme-user-feature-x
   docker compose -p colorme-user up -d  # 新しいイメージが使われる
   ```

### Q4: どのworktreeが現在起動中か確認したい

```bash
# 起動中のコンテナを確認
docker compose -p colorme-user ps

# マウントされているディレクトリを確認
docker compose -p colorme-user exec entrance-php-fpm pwd
docker compose -p colorme-user exec entrance-php-fpm ls -la | head -5

# より詳細な情報
docker inspect colorme-user-entrance-php-fpm-1 | grep -A5 "Mounts"
```

---

## まとめ

### 重要なポイント

1. **goposeは使わない** - 共有リソースの競合問題があるため
2. **`-p colorme-user`を常に使用** - 既存イメージを再利用
3. **コード変更は即反映** - ボリュームマウントのおかげ
4. **イメージ変更は再ビルド必要** - Composerやシステムパッケージの追加時
5. **同時起動は避ける** - 1つずつworktreeを切り替えて使用

### 推奨ワークフロー

```
1. git worktree add で新しいworktreeを作成
2. docker compose -p colorme-user up -d で起動
3. コードを修正・テスト
4. docker compose -p colorme-user down で停止
5. 別のworktreeに切り替えて繰り返し
```

この方法により、DNSエラーを回避し、効率的にgit worktreeで開発を進めることができます。