# tk

タスクと GitHub PR を1つのインボックスにまとめるターミナル UI。

タスクは `~/tasks.md`（Markdown のチェックボックス）に保存する。Neovim で直接編集してもよい。GitHub へのアクセスは `gh` CLI をサブプロセスで叩く。認証情報は tk 自身が一切扱わない。

## 使う

```sh
mise use -g ubi:k-narusawa/tk    # ビルド済みバイナリ（Go 不要）
mise use -g go:github.com/k-narusawa/tk  # ソースからビルド（Go 必要）
go install github.com/k-narusawa/tk@latest
tk
```

前提: `gh` CLI がログイン済み（PR 機能を使う場合のみ）。ソースからビルドする場合は Go 1.26 以降。

| キー | 動作 |
|---|---|
| `j` / `k` | カーソル移動 |
| `space` | タスクの完了トグル（即 `tasks.md` へ書き戻し） |
| `n` | 新規タスク追加 |
| `enter` | PR をブラウザで開く |
| `d` | `gh pr diff` を表示 |
| `a` / `A` | 選択アイテム / インボックス全体を AI CLI に渡す |
| `r` / `R` | PR 再取得 / `tasks.md` 再読み込み |
| `ctrl+d` / `ctrl+u` | 右ペインのスクロール |
| `q` | 終了 |

| 環境変数 | 既定値 | 用途 |
|---|---|---|
| `TK_TASKS_FILE` | `~/tasks.md` | タスクの保存先 |
| `TK_AI_CMD` | `claude` | `a` / `A` で起動する AI CLI |

`a` / `A` はタスク名と PR 情報を `$TK_AI_CMD` に渡す。tk が端末の外へデータを出す唯一の経路なので、機密を含むタスク名がある場合は注意すること。

## 開発

```sh
go test ./...          # 全テスト
go test -race ./...    # 競合検出（TUI と Refresh の並行処理があるので PR 前に必ず）
go vet ./...
go build -o tk . && ./tk   # 手元で起動確認
TK_TASKS_FILE=/tmp/t.md ./tk   # 自分の tasks.md を汚さずに試す
```

テストは標準の `testing` のみ。フレームワークは足さない。

### 層と依存の向き

依存は **adapter → usecase → domain の一方向**で、逆流させない。

| ディレクトリ | 責務 | import してよいもの |
|---|---|---|
| `internal/domain/` | `Item` の構造、Markdown の解釈と生成、並び順。純粋関数のみ | 標準ライブラリのみ（実質 `strings` / `fmt`） |
| `internal/usecase/` | アプリの手順。ポートの interface を自分で定義し、実装は知らない | `internal/domain` + 標準ライブラリ |
| `internal/adapter/tui` | bubbletea / lipgloss / bubbles への依存はここだけ | 全部 |
| `internal/adapter/markdown` | `tasks.md` の読み書き（tmp + `os.Rename` で atomic） | |
| `internal/adapter/gh` | `gh` サブプロセス。`*exec.Cmd` を返すだけで実行しない | |
| `internal/adapter/ai` | AI CLI 用の一時ファイル生成と `*exec.Cmd` 組み立て | |
| `main.go` | 環境変数読み → DI 配線 → `tea.NewProgram`。唯一全層を知る | |

外部プロセスを `tea.ExecProcess` で包むのは `internal/adapter/tui` の仕事。`internal/adapter/ai` と `internal/adapter/gh` は `*exec.Cmd` を返すだけにしてあるので、TUI を起動せずにコマンド引数をテストできる。

新しい外部サービス（Jira 等）を足すときは、`internal/usecase.PRSource` と同じ形のポートを1つ追加して `internal/adapter/` に実装を置く。

### 変更するときに踏み外しやすい点

- **bubbletea は v2**。v1 の記憶で書くとコンパイルが通らない（`View() tea.View`、`tea.KeyPressMsg`、スペースキーは `msg.String() == "space"`）。差分は [docs/superpowers/specs/2026-08-11-tk-design.md](docs/superpowers/specs/2026-08-11-tk-design.md) に一覧がある。
- **モジュールパスは `charm.land/...`**。旧 `github.com/charmbracelet/...` では `go get` が通らない。
- **`tasks.md` の非チェックボックス行（見出し・自由記述）は原文のまま保持する。** `Parse` → `Render` がバイト一致することを domain のテストで守っている。
- **保存前に外部変更を検知したら上書きせずエラーを返す。** `ID` が行番号ベースなので、ずれると別のタスクを完了にしてしまう。自動マージはしない。
- **レイアウト計算は実測する。** lipgloss の `Width`/`Height` は枠線込みかつ最小値であって上限ではない。「端末の幅・高さに収まる」形でテストを書く。

### ドキュメント

- [docs/superpowers/specs/2026-08-11-tk-design.md](docs/superpowers/specs/2026-08-11-tk-design.md) — 設計。スコープ外にしたものと、その理由
- [docs/known-issues.md](docs/known-issues.md) — 既知の課題、設計判断の記録、GitHub issue との対応表

「なぜそうしなかったか」は known-issues に書いてある。設計を変えたくなったらまずそこを読むこと。
