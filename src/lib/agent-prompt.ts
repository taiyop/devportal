import type { ControlAPIStatus } from "@/api";

const DEFAULT_API_INFO_PATH =
  "~/Library/Application Support/devportal/api.json";

export function buildRegisterProjectPrompt(
  controlApi: ControlAPIStatus,
): string {
  const infoPath = controlApi.infoPath || DEFAULT_API_INFO_PATH;
  const fallbackUrl = controlApi.url || "http://127.0.0.1:17341";

  return [
    "このプロジェクトを DevPortal に新規登録してください。",
    "",
    "1. このリポジトリの構成（README / package.json / 起動スクリプト等）を確認し、適切な設定値を決めてください。",
    "   - folder: このプロジェクトの絶対パス",
    "   - name / description / hostname（英小文字・数字・ハイフンのみ）",
    "   - command（必要なら backends や env も設定）",
    '   - portMode: ポートを環境変数 PORT や {port} で渡せるなら "auto"、固定ポートなら "manual" にして port を指定',
    "   - wakeOnRequest: true / idleStopMin: 15（必要に応じて調整）",
    "",
    "2. DevPortal のローカル管理 API 接続情報（url と token）を以下から読み取ってください。",
    `   ${infoPath}`,
    `   （現在のエンドポイント: ${fallbackUrl}）`,
    "",
    "3. すべてのリクエストに `Authorization: Bearer <token>` を付け、必要に応じて `GET /api/v1/schema`・`GET /api/v1/apps`・`GET /api/v1/categories` で仕様や既存設定を確認してください。",
    "",
    "4. `POST /api/v1/apps` で登録してください（既存アプリを更新する場合は `PATCH /api/v1/apps/{id}`）。",
    "",
    "5. 登録結果（ID・ホスト名 URL・起動コマンド・ポート設定）を報告してください。",
  ].join("\n");
}

export function buildAgentGuidePrompt(controlApi: ControlAPIStatus): string {
  const lines = [
    "DevPortal はローカル管理 API（REST / JSON）を公開しています。",
    `- Base URL: ${controlApi.url || "http://127.0.0.1:17341"}`,
    `- 接続情報ファイル（URL と Bearer トークン）: ${controlApi.infoPath || DEFAULT_API_INFO_PATH}`,
    "- 認証ヘッダ: Authorization: Bearer <token>",
    `- API スキーマ確認: GET ${controlApi.url || "http://127.0.0.1:17341"}/api/v1/schema`,
    "- 主なエンドポイント:",
    "  - GET /api/v1/apps, POST /api/v1/apps, PUT /api/v1/apps/{id}, PATCH /api/v1/apps/{id}, DELETE /api/v1/apps/{id}",
    "  - POST /api/v1/apps/{id}/start, POST /api/v1/apps/{id}/stop, POST /api/v1/apps/{id}/restart, PUT /api/v1/apps/{id}/pinned, GET /api/v1/apps/{id}/logs",
    "  - GET/POST /api/v1/categories, PUT/PATCH/DELETE /api/v1/categories/{id}",
    "  - GET /api/v1/gateway, POST /api/v1/gateway/start, POST /api/v1/gateway/stop, PUT /api/v1/gateway/port",
    '- portMode は "auto" または "manual"（固定ポート時は "manual" と port を指定）です。',
  ];
  return lines.join("\n");
}
