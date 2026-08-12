# tk — タスク / GitHub のタブ分割 設計

作成日: 2026-08-12

## 目的

タスクと PR を1本のリストに混ぜている現状をやめ、2つのタブに分ける。
`[` / `]` で切り替える。

混ぜたことで起きている問題は2つ。タスクを片付けたいときに PR が視界に入り、
PR をさばきたいときにタスクが挟まる。もう1つは、`space` / `enter` のように
片方の Kind でしか意味を持たないキーが、リストのどこにカーソルがあるかで
効いたり効かなかったりすること。

## スコープ

**含む**

- タブ2つ（タスク / GitHub）と `[` / `]` による切り替え
- タブごとのカーソル位置の保持
- タブに応じたキーの有効・無効
- 画面最上部のタブ行
- GitHub タブが空のときの状態表示
- README の更新

**含まない（意図的に外した）**

- 統合ビュー（「すべて」タブ）。分けるのが目的なので残さない。
- 3つ目以降のタブ。GitHub タブを review / mine に分けるのも今はやらない。
  Role による絞り込みが欲しくなってから考える。
- タブ状態の永続化。起動時は常にタスクタブ。
- タブごとの並び替え設定。並び順は引き続き固定。

## 設計

### domain

`SortInbox(tasks, prs)` を削除し、2本に割る。

```go
func SortTasks(tasks []Item) []Item  // 未完了 → 完了
func SortPRs(prs []Item) []Item      // review → mine
```

並び順は固定でユーザーが変更する手段を用意しない、という現行の方針は
そのまま両方に引き継ぐ。`MergePRs` は変更しない。

### usecase

`Inbox.Items()` を `Tasks()` / `PRs()` の2本に分ける。それぞれ
`SortTasks` / `SortPRs` を通す。

`Inbox.Find` は本体のどこからも呼ばれていない（テストのみ）ので削除する。

`Refresh` / `Toggle` / `Add` / `Detail` は変更しない。タブは表示上の
概念であり、usecase より下には持ち込まない。

### adapter/tui

`Model` に3つ足す。

```go
tab         tabID // tabTask | tabGitHub
otherCursor int   // 非表示タブのカーソル位置
prLoaded    bool  // Refresh が1度でも完了したか
```

`m.items` は「現在のタブの一覧」に意味が変わる。`selected` / `listView` /
`aiExec` は `m.items` を見ているだけなので手を入れない。

`reload()` はタブを見て `inbox.Tasks()` か `inbox.PRs()` を入れる。

タブ切り替えはカーソルの入れ替えとリロードだけ。

```go
func (m *Model) switchTab(t tab) {
    if m.tab == t {
        return
    }
    m.tab, m.cursor, m.otherCursor = t, m.otherCursor, m.cursor
    m.reload()
    m.syncDetail()
}
```

`[` はタスクタブ、`]` は GitHub タブへの**絶対移動**にする。トグルに
しないのは、タスクタブで `[` を押しても何も起きない方が意図に合うため。
2タブの間はトグルと結果が同じだが、キーの意味は「左のタブ」「右のタブ」。

### キー

| キー | タスクタブ | GitHub タブ |
|---|---|---|
| `[` `]` | タブ移動 | タブ移動 |
| `j` `k` | 有効 | 有効 |
| `ctrl+d` `ctrl+u` | 有効 | 有効 |
| `a` `A` | 有効（現タブのみ） | 有効（現タブのみ） |
| `r` `R` `q` | 有効 | 有効 |
| `space` `n` | 有効 | **無効** |
| `enter` `d` | 無効 | 有効 |

`enter` / `d` の無効化は既存の `it.Kind != domain.KindPR` ガードが
そのまま効くので、コードの追加は要らない。`n` は GitHub タブで押すと
見えない場所にタスクが増えて混乱するので、明示的に塞ぐ。

`A` は現在のタブのアイテムだけを AI CLI に渡す。画面に見えているものと
渡るものを一致させる。実装上は `m.items` がすでに現タブのリストなので
`aiExec(m.items)` のままでよい。

`r` は GitHub タブにいなくても PR を再取得する。起動時の `Refresh` も
今まで通り走らせ、タスクタブを見ている間に裏で取り終わっているようにする。

### 画面

最上部にタブ行を1行足す。選択中を角括弧で囲む。

```
 [ タスク ]  GitHub
╭─────────────────╮ ╭──────────╮
│ > □ 設計を詰める │ │ 設計を詰める │
│   ▣ README 更新  │ │ state: 未完了│
╰─────────────────╯ ╰──────────╯
 j/k:移動 space:完了 n:追加 enter:開く d:diff ...
```

本文の高さは `m.height-4` から `m.height-5` に変わる。`WindowSizeMsg` の
viewport 寸法計算も同じ値を使っているので、両方を直す。

ヘルプ行はタブによらず固定にする。`[/]:タブ` を先頭に足す。端末幅で
`MaxWidth` により切り詰める現行の処理はそのまま残す。

タブ行に色は使わない。現行の view.go は枠線以外にスタイルを持っておらず、
角括弧だけで選択状態は伝わる。

### 詳細ペイン

タブ切り替え時に `syncDetail` と `detailCmd` を呼ぶ。タスクタブにいる間は
`detailCmd` が `Kind != KindPR` で早期 return するので、PR 詳細の HTTP は
発生しない。現行の遅延取得がそのまま活きる。

### GitHub タブが空のとき

起動直後は `gh` の取得が終わるまで必ず空になる。何も出さないと故障に
見えるので、一覧に1行だけ状態を出す。

- `prLoaded == false` → `（PR を取得中…）`
- `prLoaded == true` かつ 0 件 → `（PR なし）`

`prLoaded` は `prLoadedMsg` を受けた時点で（エラーの有無によらず）true に
する。エラーの場合はフッタに `errMsg` が出るので、一覧側は「PR なし」で
矛盾しない。

タスクタブが空のときは何も出さない。`tasks.md` が空なのはユーザーが知って
いる状態であり、非同期の取得も挟まらない。

## テスト

`domain`

- `SortTasks` が未完了を先に出す
- `SortPRs` が review を先に出す

`adapter/tui`

- `[` / `]` で `m.items` がタスク一覧 / PR 一覧に入れ替わる
- タブを往復してもカーソル位置がタブごとに保たれる
- GitHub タブで `n` と `space` が何もしない
- `A` が現在のタブのアイテムだけを渡す
- `prLoaded` の状態で空表示の文言が変わる
- タブ行を足した状態でも、描画結果が端末の幅と高さに収まる

既存の `model_test.go` は統合リストを前提にしたケースが多いので、タブを
指定する形に直す。

## 影響範囲

| ファイル | 変更 |
|---|---|
| `internal/domain/inbox.go` | `SortInbox` 削除、`SortTasks` / `SortPRs` 追加 |
| `internal/domain/inbox_test.go` | 上記に合わせる |
| `internal/usecase/inbox.go` | `Items` → `Tasks` / `PRs`、`Find` 削除 |
| `internal/usecase/inbox_test.go` | 上記に合わせる |
| `internal/adapter/tui/model.go` | `tab` / `otherCursor` / `prLoaded`、`reload` がタブを見る |
| `internal/adapter/tui/update.go` | `[` / `]`、`n` のガード、高さ計算、`prLoaded` |
| `internal/adapter/tui/view.go` | タブ行、高さ計算、空表示、ヘルプ文言 |
| `internal/adapter/tui/model_test.go` | 上記に合わせる |
| `README.md` | 冒頭の説明、キー表、レイアウト |

`internal/adapter/gh` `internal/adapter/ai` `internal/adapter/markdown`
`main.go` は変更しない。
