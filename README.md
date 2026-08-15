# tk

タスクと GitHub PR を上下のペインに並べて見るターミナル UI。

タスクは `~/.config/tk/tasks.md`（Markdown のチェックボックス）に保存する。各タスクの詳細は `~/.config/tk/tasks/<タイトル>.md` に1ファイルずつ置く。どちらも Neovim で直接編集してよい。GitHub へのアクセスは `gh` CLI をサブプロセスで叩く。認証情報は tk 自身が一切扱わない。

## 使う

```sh
mise use -g github:k-narusawa/tk # ビルド済みバイナリ（Go 不要）
mise use -g go:github.com/k-narusawa/tk  # ソースからビルド（Go 必要）
go install github.com/k-narusawa/tk@latest
tk
```

前提: `gh` CLI がログイン済み（PR 機能を使う場合のみ）。ソースからビルドする場合は Go 1.26 以降。

| キー | 動作 |
|---|---|
| `h` / `l` | ペインのフォーカスを移す（2つなので、どちらのキーでも往復する） |
| `j` / `k` | カーソル移動 |
| `space` | タスクの完了トグル（即 `tasks.md` へ書き戻し。タスクペインのみ） |
| `n` | 新規タスク追加（タスクペインのみ） |
| `e` | 選択中タスクの詳細ファイルを `$EDITOR` で開く（タスクペインのみ）。閉じると `tasks.md` を読み直す |
| `enter` | PR をブラウザで開く（GitHub ペインのみ） |
| `d` | `gh pr diff` を表示（GitHub ペインのみ） |
| `a` / `A` | 選択アイテム / フォーカス中ペイン全体を AI CLI に渡す |
| `r` | フォーカス中のペインを更新（タスクなら `tasks.md` 再読み込み、GitHub なら PR 再取得） |
| `R` | どちらのペインにいても `tasks.md` を再読み込み |
| `ctrl+d` / `ctrl+u` | 右ペインのスクロール |
| `q` | 終了 |

| 環境変数 | 既定値 | 用途 |
|---|---|---|
| `TK_TASKS_FILE` | `~/.config/tk/tasks.md` | タスク一覧の保存先。詳細ディレクトリはここから導出する（`tasks.md` → `tasks/`） |
| `TK_AI_CMD` | `claude` | `a` / `A` で起動する AI CLI |
| `TK_EDITOR` | `$VISUAL` → `$EDITOR` → `vi` | `e` で起動するエディタ |

## タスクの詳細

詳細はタスクごとの独立したファイルに置く。ファイル名がタスクのタイトルそのもの。

```
~/.config/tk/
  tasks.md                          一覧（順番・完了・タグ）
  tasks/
    認証まわりのリファクタ.md          詳細（本文だけ）
```

`tasks.md`:

```markdown
- [ ] 認証まわりのリファクタ @today
- [x] 別のタスク
```

`tasks/認証まわりのリファクタ.md`:

```markdown
Cookie の SameSite を Lax に

RFC を読み直すこと
```

詳細ファイルの中身に tk が課す規則はない。frontmatter も要らない。書いた Markdown がそのまま右ペインに出る。

書くのは `e`（そのタスクの詳細ファイルが `$EDITOR` で開く）か、直接ファイルを編集する。**tk 自身は詳細を書き込まない。** ファイルが無ければエディタが新規作成する。tk がやるのは `tasks/` ディレクトリを作ることだけ。

いくつか決めごとがある。

- ファイル名はタイトルから作る。`/` は `-` に、先頭の `.` は `-` に置換し、255 バイトで切り詰める。**タグは含めない**ので、タグを付け替えても詳細は迷子にならない
- 同じタイトルのタスクが2つあれば、同じ詳細ファイルを見る
- **tk は詳細ファイルを削除しない。** `tasks.md` から行を消しても詳細は残る。「完了して消した」のか「一時的にコメントアウトした」のかを区別できないので、書いたメモを勝手に消さない。溜まったら手で消す
- 詳細はキャッシュしない。カーソルが動くたびに読み直すので、エディタで書き換えれば次に選んだ瞬間に反映される

## 0.x からの移行

保存先が `~/tasks.md` から `~/.config/tk/tasks.md` に変わった。自動移行はしないので手で移す。

```sh
mkdir -p ~/.config/tk
mv ~/tasks.md ~/.config/tk/tasks.md
```

`~/tasks.md` を使い続けたい場合は `TK_TASKS_FILE=~/tasks.md` を指定する。

以前はチェックボックス行の直後のインデント行が詳細だった。この解釈はやめたので、既存のインデント行は右ペインに出なくなる（`tasks.md` からは消えない。tk は非チェックボックス行を原文のまま保つ）。右ペインに出したければ `~/.config/tk/tasks/<タイトル>.md` に手で移す。

`~/.config` を dotfiles リポジトリで管理している場合、タスクの中身がリポジトリに入る。`.gitignore` に `tk/` を足しておくこと。

フォーカス中のペインは枠が緑になり、フォーカスしていない側は枠線だけに潰れてタイトルに件数（`GitHub (3)`）が出る。

`a` は選択中のアイテム、`A` はフォーカス中のペインに出ているアイテム全部を `$TK_AI_CMD` に渡す。tk が端末の外へデータを出す唯一の経路なので、機密を含むタスク名がある場合は注意すること。

## 開発

Go のバージョンは `mise.toml` で固定している。`mise install` で入れる。

```sh
go test ./...          # 全テスト
go test -race ./...    # 競合検出（TUI と Refresh の並行処理があるので PR 前に必ず）
go vet ./...
mise run dev            # 手元で起動確認（サブディレクトリからでも可）
TK_TASKS_FILE=/tmp/t.md go run .   # 一覧も詳細も /tmp/t/ に隔離して試す
```

テストは標準の `testing` のみ。フレームワークは足さない。

### 層と依存の向き

依存は **adapter → usecase → domain の一方向**で、逆流させない。

| ディレクトリ | 責務 | import してよいもの |
|---|---|---|
| `internal/domain/` | `Item` の構造、Markdown の解釈と生成、並び順。純粋関数のみ | 標準ライブラリのみ（実質 `strings` / `fmt`） |
| `internal/usecase/` | アプリの手順。ポートの interface を自分で定義し、実装は知らない | `internal/domain` + 標準ライブラリ |
| `internal/adapter/tui` | bubbletea / lipgloss / bubbles への依存はここだけ | 全部 |
| `internal/adapter/markdown` | `tasks.md` の読み書き（tmp + `os.Rename` で atomic）と、詳細ファイルの読み込み | |
| `internal/adapter/gh` | `gh` サブプロセス。`*exec.Cmd` を返すだけで実行しない | |
| `internal/adapter/ai` | AI CLI 用の一時ファイル生成と `*exec.Cmd` 組み立て | |
| `internal/adapter/editor` | エディタ起動の `*exec.Cmd` 組み立て。実行しない | |
| `main.go` | 環境変数読み → DI 配線 → `tea.NewProgram`。唯一全層を知る | |

外部プロセスを `tea.ExecProcess` で包むのは `internal/adapter/tui` の仕事。`internal/adapter/ai` と `internal/adapter/gh` は `*exec.Cmd` を返すだけにしてあるので、TUI を起動せずにコマンド引数をテストできる。

新しい外部サービス（Jira 等）を足すときは、`internal/usecase.PRSource` と同じ形のポートを1つ追加して `internal/adapter/` に実装を置く。

### 変更するときに踏み外しやすい点

- **bubbletea は v2**。v1 の記憶で書くとコンパイルが通らない（`View() tea.View`、`tea.KeyPressMsg`、スペースキーは `msg.String() == "space"`）。差分は [docs/superpowers/specs/2026-08-11-tk-design.md](docs/superpowers/specs/2026-08-11-tk-design.md) に一覧がある。
- **モジュールパスは `charm.land/...`**。旧 `github.com/charmbracelet/...` では `go get` が通らない。
- **`tasks.md` の非チェックボックス行（見出し・自由記述）は原文のまま保持する。** `Parse` → `Render` がバイト一致することを domain のテストで守っている。
- **詳細ファイルは tk が書かない。** 読むのと親ディレクトリの `mkdir` だけ。書き込みを足すと、エディタで開いている最中の外部変更と競合する。`Store` が `mtime` / `size` で守っているのと同じ問題を、詳細ファイルぶん抱え込むことになる。
- **保存前に外部変更を検知したら上書きせずエラーを返す。** `ID` が行番号ベースなので、ずれると別のタスクを完了にしてしまう。自動マージはしない。
- **レイアウト計算は実測する。** lipgloss の `Width`/`Height` は枠線込みかつ最小値であって上限ではない。「端末の幅・高さに収まる」形でテストを書く。
- **寸法の計算は `view.go` の `newLayout` に集約してある。** `View` と `update.go` の `WindowSizeMsg` の両方が同じ値を要るので、直接計算を書き足さないこと。片方だけずれると右ペインが枠から溢れる。
- **枠の中身は行数も幅も自分で切り詰める。** `box.Height` / `Width` は最小値なので、一覧に末尾改行を付けたり枠幅を超える行を折り返させたりすると、枠がその分伸びて下のペインを押し出す。
- **枠の高さは `paneView` / `detailView` を直接測って検証する。** レンダリング結果から枠の閉じ位置（`╰` や `╯`）を探す形だと、左カラムが伸びても最終行は「左の下端 ++ 右の下端」のままなので、ズレを検出できない。

### ドキュメント

- [docs/superpowers/specs/2026-08-11-tk-design.md](docs/superpowers/specs/2026-08-11-tk-design.md) — 設計。スコープ外にしたものと、その理由
- [docs/superpowers/specs/2026-08-12-tk-tabs-design.md](docs/superpowers/specs/2026-08-12-tk-tabs-design.md) — タスク / GitHub の分割（当時はタブ）
- [docs/superpowers/specs/2026-08-13-tk-panes-design.md](docs/superpowers/specs/2026-08-13-tk-panes-design.md) — タブをやめて lazygit 風の上下ペインへ
- [docs/superpowers/specs/2026-08-13-tk-task-detail-design.md](docs/superpowers/specs/2026-08-13-tk-task-detail-design.md) — タスクの詳細メモ
- [docs/superpowers/specs/2026-08-15-tk-task-files-design.md](docs/superpowers/specs/2026-08-15-tk-task-files-design.md) — 詳細を1タスク1ファイルに分離。SQLite を採らなかった理由
- [docs/known-issues.md](docs/known-issues.md) — 既知の課題、設計判断の記録、GitHub issue との対応表

「なぜそうしなかったか」は known-issues に書いてある。設計を変えたくなったらまずそこを読むこと。
