# DevPortal

ローカルで作ったちょい便利ツールを、ターミナルを開かずに起動・終了するためのランチャーです。

フォルダと起動コマンドを登録すると、画面からプロセスを起こし、`http://127.0.0.1:<port>` で開けます。設定は YAML に保存されます。

## できること

- 対象フォルダをパス入力、またはフォルダ選択ダイアログで登録
- 起動コマンドを指定（例: `npm run dev` / `bun run dev`）
- バックエンドを複数、別プロセスとして同時起動できる（フォルダ・コマンド・ポート・環境変数）
- ポートは自動割り当て、または手動指定。待受番号を渡す環境変数名もアプリごとに変えられる
- 環境変数を1行1変数で登録
- 起動 / 終了で子プロセスごと止める
- 稼働中の URL をブラウザで開く
- 起動先の favicon を一覧に表示する（登録時はフォルダ内のアイコン、起動後は実際に配信されているもの）
- アプリごとに `名前.devportal.localhost` を割り当て、ボードからリバースプロキシを開始すると指定ポート（未指定なら `:80`）で転送。空いていなければ自動で別ポートを割り当てる
- 指定ドメインへアクセスすると、停止中のアプリを自動起動する
- ドメイン経由で起こしたアプリは、最終アクセスから一定時間で自動終了（Keep alive がオン、または画面から起動したものは永続）

## ドメインとリバースプロキシ

リバースプロキシはアプリ起動時には始まりません。ボードの「Reverse Proxy」から開始・終了します。開始すると、設定のポート（未指定なら `:80`、すべてのインターフェース）で待ち受けます。空いていなければ 7341 などへ自動で切り替えます。ブラウザから次のように開くと対応するアプリへ転送します。

```txt
http://my-tool.devportal.localhost
```

`*.localhost` は OS がループバックに解決します。hosts の編集は不要です。リバースプロキシが動いていれば、停止中でもこの URL で起動します。`http://127.0.0.1:ポート` へのアクセスでは起動しません。

ポートは設定画面で変更できます。未入力・未指定は 80 です。macOS では `127.0.0.1:80` は管理者権限が必要ですが、`:80`（`0.0.0.0` / `::`）なら一般ユーザーでも待受できます。指定ポートが使用中などのときは別のポートへ自動で切り替え、詳細の URL も `http://my-tool.devportal.localhost:7341` のようになります。
LAN からも 80 番は見えるので、Host が `*.devportal.localhost` 以外なら拒否します。

ドメイン URL 経由で起こしたアプリは、最後のアクセスから設定した時間が経つと自動で止まります。画面の「起動」と Keep alive オンは永続稼働で、自動では止まりません。一覧の残り時間と Keep alive ボタンで切り替えられます。

## 動作要件 (Prerequisites)

- **OS**: macOS (Apple Silicon / Intel)
- **Go**: 1.25 以上
- **Bun** (推奨) または **Node.js**: 20 以上
- **Wails CLI v3**: `go install github.com/wailsapp/wails/v3/cmd/wails3@latest`

## 起動・開発方法

Wails 3（Go）で開発サーバーを起動します。

```bash
bun install
bun run wails:dev
```

リリースビルド:

```bash
bun run wails:build
```

macOS の `.app` と、Applications にドラッグして入れる DMG:

```bash
bun run wails:dmg
```

成果物は `src-wails/bin/` に出ます。

- `DevPortal.app` — アプリ本体
- `DevPortal.dmg` — インストーラ（アプリを Applications へドラッグ）

DMG を開いて `DevPortal` を `Applications` にドラッグすると、Launchpad や Spotlight から起動できます。Developer ID で署名していないビルドは、初回に「壊れている」と出ることがあります。そのときは次で隔離属性を外してください。

```bash
xattr -cr /Applications/DevPortal.app
```

Developer ID で署名・公証する場合は `wails3 setup` のあと `wails3 task darwin:sign:notarize` です。`src-wails` ディレクトリで実行してください。

```bash
cd src-wails
wails3 setup
wails3 task darwin:sign:notarize
```

`bun build wails` は使えません。`bun build` は Bun のバンドラなので、`wails` という JS エントリを探しにいきます。Wails アプリをビルドするときは `bun run wails:build` か `bun run wails build` を使ってください。

`src-tauri` の Tauri 2 実装も残してあります。そちらで起動する場合は `bun run tauri dev` です。

## 設定ファイル

`apps.yml` は OS のアプリ設定ディレクトリに保存されます。

- macOS: `~/Library/Application Support/devportal/apps.yml`
- 起動中の PID / PGID は同じ場所の `running.yml` に残し、次回起動時に回収します。
- 起動先から取得した favicon は同じ場所の `favicons/` にキャッシュします。

```yaml
gatewayPort: 80
apps:
  - id: "..."
    name: my-tool
    description: ローカルで動かすメモ用サーバー
    hostname: my-tool
    folder: /Users/you/repos/my-tool
    command: npm run dev
    portMode: auto
    port: 5173
    portEnv: PORT
    env:
      - name: HOST
        value: 127.0.0.1
    backends:
      - name: api
        folder: /Users/you/repos/my-tool/api
        command: go run .
        portMode: auto
        port: 8080
        portEnv: PORT
        appPortEnv: API_PORT
        env:
          - name: DATABASE_URL
            value: postgres://127.0.0.1/app
      - name: worker
        command: go run ./cmd/worker
        portMode: auto
        appPortEnv: WORKER_PORT
```

`backends` は任意です。あるとアプリの起動時に別プロセスとして同時に始まり、終了時も一緒に止まります。フォルダを省略するとアプリと同じフォルダです。古い `backend:`（単体）も読み込みます。

## ポートの渡し方

起動時に待受番号を環境変数へ渡します。変数名はアプリ設定で変えられます。未指定なら本体は `PORT`、1つ目のバックエンドを本体へ渡すときは `BACKEND_PORT`、2つ目以降は `BACKEND_PORT_2` です。各バックエンド自身の待受も、未指定なら `PORT` です。内部用に `DEVPORTAL_PORT` / `DEVPORTAL_BACKEND_PORT` / `DEVPORTAL_BACKEND_PORT_1` も入ります。

コマンドと環境変数の値にある `{port}` はアプリ本体の番号、`{backendPort}` は1つ目のバックエンド、`{backendPort:1}` `{backendPort:2}` は番号、`{backendPort:api}` は名前です。指定した環境変数は各プロセスの待受番号で上書きされます。

```text
vite --port {port} --host 127.0.0.1
```

```text
API_URL=http://127.0.0.1:{backendPort:api}
```

## ドメインで起動

リバースプロキシが動いているあいだ、`http://my-tool.devportal.localhost` にアクセスすると:

1. 対応するアプリの起動コマンドを実行する
2. ポートが開くまで待ってからリクエストを転送する

`http://127.0.0.1:ポート` へのアクセスでは起動しません。アプリが止まっているときは、指定ドメイン経由で起こします。

DevPortal 自体が落ちていると、ドメインでの自動起動はできません。
 
+## コントリビューション
+
+バグ報告、機能提案、Pull Request を歓迎しています。開発手順やガイドラインについては [CONTRIBUTING.md](CONTRIBUTING.md) をご覧ください。
+
+## ライセンス
+
+[MIT License](LICENSE)
+
