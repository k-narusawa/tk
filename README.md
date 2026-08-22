# tk

タスク・GitHub PR・routine を上下のペインに並べて見るターミナル UI。

タスクは `~/.config/tk/tasks.md`（Markdown のチェックボックス）に保存する。各タスクの詳細は `~/.config/tk/tasks/<タイトル>.md` に1ファイルずつ置く。どちらも Neovim で直接編集してよい。GitHub へのアクセスは `gh` CLI をサブプロセスで叩く。認証情報は tk 自身が一切扱わない。

routine は「気になる OSS のリリースを定期的に見に行く」ような、AI に繰り返し調べさせたい項目の一覧。`x` を押すと AI CLI が裏で走り、結果がファイルに追記される。

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
| `l` / `h` | 次 / 前のペインへフォーカスを移す（端で巡回する） |
| `j` / `k` | カーソル移動 |
| `space` | タスクの完了トグル（即 `tasks.md` へ書き戻し。タスクペインのみ） |
| `n` | 新規タスク追加（タスクペイン）／ `routines.md` を `$EDITOR` で開く（routine ペイン） |
| `e` | 選択中タスクの詳細ファイル、または routine の指示ファイルを `$EDITOR` で開く。閉じるとファイルを読み直す |
| `enter` | PR をブラウザで開く（GitHub ペインのみ） |
| `d` | `gh pr diff` を表示（GitHub ペインのみ） |
| `v` | 選択中の PR を `review.md` のプロンプト付きで AI CLI に渡す（GitHub ペインのみ） |
| `x` | 選択中の routine を裏で実行する（routine ペインのみ） |
| `a` / `A` | 選択アイテム / フォーカス中ペイン全体を AI CLI に渡す |
| `r` | フォーカス中のペインを更新（GitHub なら PR 再取得、それ以外はファイル再読み込み） |
| `R` | どのペインにいても `tasks.md` と `routines.md` を再読み込み |
| `J` / `K` | 右ペインを1行スクロール（カーソルは一覧に置いたまま） |
| `ctrl+d` / `ctrl+u` | 右ペインを半ページスクロール |
| `q` | 終了（実行中の routine があれば一度警告する。`ctrl+c` は常に即終了） |

| 環境変数 | 既定値 | 用途 |
|---|---|---|
| `TK_TASKS_FILE` | `~/.config/tk/tasks.md` | タスク一覧の保存先。詳細ディレクトリと `routines.md` はここから導出する |
| `TK_AI_CMD` | `claude` | `a` / `A` / `v` で起動する AI CLI（対話起動。画面を明け渡す） |
| `TK_ROUTINE_CMD` | `claude -p --allowedTools "WebSearch,WebFetch,Bash(gh api:*)"` | `x` で起動する AI CLI（非対話。`sh -c` 経由で走り、終了を待って標準出力を拾う） |
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

## routine

AI に繰り返し調べさせたいことの一覧。tk は routine を自動では走らせない。`x` を押したときだけ走る。定期実行が要るなら OS の cron や launchd から別に回す。

```
~/.config/tk/
  routines.md                       一覧
  routines/
    golang のリリース.md              指示（AI に渡すプロンプト）
    golang のリリース.result.md       実行結果（tk が追記する）
```

`routines.md`:

```markdown
# 監視

- golang のリリース
- 使っている OSS の脆弱性
```

チェックボックスは使わない。routine は「終わる」ものではないため。

`tasks.md` と違って tk は `routines.md` に書き戻さない。追加・削除は routine ペインで `n` を押すと `routines.md` が `$EDITOR` で開くので、そこで直接編集する。閉じると読み直す。タスクの `n`（フッタにインライン入力欄が出る）とは作法が違う。

指示は routine ごとの独立したファイルに書く。ファイル名が routine 名そのもので、変換規則はタスクの詳細ファイルと同じ。

`routines/golang のリリース.md`:

```markdown
golang/go の最新リリースを調べて、前回の実行結果から増えたぶんだけを箇条書きで出して。
破壊的変更があれば必ず触れること。
```

`x` を押すと、この指示が `$TK_ROUTINE_CMD` の標準入力に流し込まれる。tk は画面を明け渡さないので、走っている間も他のペインを操作できる。行頭の印で状態が分かる。

| 印 | 意味 |
|---|---|
| （無印） | このセッションでまだ実行していない |
| `…` | 実行中 |
| `✓` | 完了。結果が `.result.md` に追記された |
| `✗` | 失敗。理由はフッタに出る |

実行結果は `.result.md` に日時見出し付きで積まれる。**新しいものが先頭**なので、右ペインを開いた時点で最新が見える。上書きしないのは、前回から何が変わったかを AI 自身に読ませたいため。指示の中で「前回の実行結果から増えたぶん」と書けるのはこれが理由。溜まりすぎたら手で消す（tk は消さない）。

いくつか決めごとがある。

- **実行中に tk を終了すると、走っている routine も道連れで死ぬ。** 結果ファイルは途中まで書かれた状態で残る。`q` は実行中があれば一度だけ警告して止まるが、`ctrl+c` は常に即終了する
- 同じ routine の二重起動はしない。実行中にもう一度 `x` を押しても無視される
- 指示ファイルが空なら実行しない。押した時点でフッタに理由が出る
- **非対話ではツールの承認ダイアログを出せない。** 許可を渡さないと調べ物のツールが全部拒否され、「取得できませんでした」という文章がそのまま結果ファイルに追記される（終了コードは 0 なので tk からは `✓` に見える）。既定の `--allowedTools` は読み取り系だけを通している。他のツールを使わせたいなら `TK_ROUTINE_CMD` ごと書き換える
- `TK_ROUTINE_CMD` は `sh -c` に渡される。クォートもパイプも使える

`TK_AI_CMD` と分けているのは、あちらが対話起動（画面を明け渡して人が読む）なのに対し、こちらは終了を待って標準出力を拾う必要があるため。既定は `claude -p --allowedTools "WebSearch,WebFetch,Bash(gh api:*)"` だが、環境変数だけで他の CLI に乗り換えられる。

## 0.x からの移行

保存先が `~/tasks.md` から `~/.config/tk/tasks.md` に変わった。自動移行はしないので手で移す。

```sh
mkdir -p ~/.config/tk
mv ~/tasks.md ~/.config/tk/tasks.md
```

`mkdir -p` は保険。保存先が無ければ `Store.Save` が初回保存時に自動で作るので必須ではない。

`~/tasks.md` を使い続けたい場合は `TK_TASKS_FILE=~/tasks.md` を指定する。その場合、詳細ファイルの保存先も `~/.config/tk/tasks/` ではなく `~/tasks/` になる。

以前はチェックボックス行の直後のインデント行が詳細だった。この解釈はやめたので、既存のインデント行は右ペインに出なくなる（`tasks.md` からは消えない。tk は非チェックボックス行を原文のまま保つ）。右ペインに出したければ `~/.config/tk/tasks/<タイトル>.md` に手で移す。

`~/.config` を dotfiles リポジトリで管理している場合、タスクの中身がリポジトリに入る。`.gitignore` に `tk/` を足しておくこと。

フォーカス中のペインは枠が緑になり、フォーカスしていない側は枠線だけに潰れてタイトルに件数（`GitHub (3)`）が出る。

`a` は選択中のアイテム、`A` はフォーカス中のペインに出ているアイテム全部を `$TK_AI_CMD` に渡す。tk が端末の外へデータを出す唯一の経路なので、機密を含むタスク名がある場合は注意すること。

## PR を AI にレビューさせる

レビュー用のプロンプトを `~/.config/tk/review.md` に置いておくと、GitHub ペインで PR を選んで `v` を押すだけでそれを使ったレビューが始まる。

```markdown
この PR をレビューして。gh pr diff で差分を取ること。

観点:
- 設計とレイヤの整合
- エラー処理の握り潰し
- テストの漏れ
```

tk は `review.md` の中身に対象 PR を添えて一時ファイルに書き、`$TK_AI_CMD <path>` を起動する。渡るのは repo / number / url だけで、差分は AI 自身に `gh` で取らせる。巨大な PR でもプロンプトが膨らまない。

```markdown
<review.md の中身>

## 対象 PR
- repo: k-narusawa/tk
- number: 25
- url: https://github.com/k-narusawa/tk/pull/25
```

`{{repo}}` `{{number}}` `{{url}}` は本文中で使える。書かなくても末尾のブロックで対象は伝わるので、位置を指定したいときだけ使う。

- **プロンプトは毎回読み直す。** 書き換えたら tk を再起動せずに次の `v` から効く
- `review.md` が無ければ、置くべきパスをエラーに出す。tk がテンプレートを勝手に作ることはしない
- 保存先は `TK_TASKS_FILE` から導出する（`tasks.md` → 同じディレクトリの `review.md`）。専用の環境変数は無い
- 対象は PR なら何でもよい。レビュー依頼が来ているものに限らず、自分の PR にも使える

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
