import { useEffect, useState, type FormEvent } from "react";
import {
  ArrowDownToLine,
  Bot,
  Copy,
  Eye,
  EyeOff,
  FolderOpen,
  RefreshCw,
  RotateCw,
} from "lucide-react";
import { toast } from "sonner";
import type { ControlAPIStatus } from "@/api";
import { StatusLed } from "@/components/status-badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Spinner } from "@/components/ui/spinner";
import { copyText } from "@/lib/clipboard";
import { buildAgentGuidePrompt } from "@/lib/agent-prompt";
import { formatVersionLabel, type AppUpdater } from "@/lib/updater";
import type { AppStatus } from "@/types";

export function SettingsDialog({
  open,
  configPath,
  gatewayPort,
  gatewayRunning,
  savingPort,
  controlApi,
  savingControlApiPort,
  rotatingControlApiToken,
  updater,
  onOpenChange,
  onRevealConfig,
  onSaveGatewayPort,
  onSaveControlApiPort,
  onRotateControlApiToken,
}: {
  open: boolean;
  configPath: string;
  gatewayPort: number;
  gatewayRunning: boolean;
  savingPort: boolean;
  controlApi: ControlAPIStatus;
  savingControlApiPort: boolean;
  rotatingControlApiToken: boolean;
  updater: AppUpdater;
  onOpenChange: (open: boolean) => void;
  onRevealConfig: () => void;
  onSaveGatewayPort: (port: number) => void;
  onSaveControlApiPort: (port: number) => void;
  onRotateControlApiToken: () => void;
}) {
  const [draft, setDraft] = useState("80");
  const [portError, setPortError] = useState<string | null>(null);
  const [apiPortDraft, setApiPortDraft] = useState("17341");
  const [apiPortError, setApiPortError] = useState<string | null>(null);
  const [showToken, setShowToken] = useState(false);

  useEffect(() => {
    if (!open) return;
    setDraft(String(gatewayPort || 80));
    setPortError(null);
    setApiPortDraft(String(controlApi.preferredPort || 17341));
    setApiPortError(null);
    setShowToken(false);
  }, [open, gatewayPort, controlApi.preferredPort]);

  function submitPort(event: FormEvent) {
    event.preventDefault();
    const parsed = parsePort(draft, 80);
    if (parsed.error) {
      setPortError(parsed.error);
      return;
    }
    setPortError(null);
    setDraft(String(parsed.port));
    onSaveGatewayPort(parsed.port);
  }

  function submitApiPort(event: FormEvent) {
    event.preventDefault();
    const parsed = parsePort(apiPortDraft, 17341);
    if (parsed.error) {
      setApiPortError(parsed.error);
      return;
    }
    setApiPortError(null);
    setApiPortDraft(String(parsed.port));
    onSaveControlApiPort(parsed.port);
  }

  async function copy(value: string, okMessage: string) {
    const ok = await copyText(value);
    toast[ok ? "success" : "error"](ok ? okMessage : "コピーできませんでした");
  }

  const apiLedStatus: AppStatus = controlApi.running
    ? "running"
    : controlApi.error
      ? "error"
      : "stopped";

  const maskedToken = controlApi.token
    ? `${controlApi.token.slice(0, 7)}••••••••••••••••••••••••`
    : "—";

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex max-h-[calc(100dvh-1.5rem)] w-[calc(100%-1.5rem)] max-w-md flex-col gap-0 overflow-hidden p-0 sm:max-w-md">
        <DialogHeader className="shrink-0 gap-1.5 border-b border-border px-4 py-3 pr-12">
          <DialogTitle className="text-[15px] font-semibold tracking-tight">
            設定
          </DialogTitle>
          <DialogDescription className="text-[12px]">
            リバースプロキシ、ローカル API、アップデート、設定ファイルの場所です。
          </DialogDescription>
        </DialogHeader>

        <div className="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto px-4 py-4">
          <section className="flex flex-col gap-2">
            <h2 className="px-1 text-[13px] font-semibold">リバースプロキシ</h2>
            <form onSubmit={submitPort}>
              <FieldGroup className="surface settings-group gap-0 p-0">
                <Field className="gap-1.5 px-4 py-3" data-invalid={!!portError}>
                  <FieldLabel htmlFor="gateway-port">ポート</FieldLabel>
                  <div className="flex items-center gap-2">
                    <Input
                      id="gateway-port"
                      type="text"
                      inputMode="numeric"
                      placeholder="80"
                      className="font-mono"
                      value={draft}
                      aria-invalid={!!portError}
                      autoComplete="off"
                      onChange={(event) => {
                        setDraft(event.target.value);
                        setPortError(null);
                      }}
                    />
                    <Button type="submit" size="sm" disabled={savingPort}>
                      {savingPort ? <Spinner /> : null}
                      保存
                    </Button>
                  </div>
                  {portError ? (
                    <FieldError>{portError}</FieldError>
                  ) : (
                    <FieldDescription>
                      未入力なら 80 です。空いていなければ別のポートを割り当てます。
                      {gatewayRunning
                        ? " 保存するとリバースプロキシをこのポートで再起動します。"
                        : ""}
                    </FieldDescription>
                  )}
                </Field>
              </FieldGroup>
            </form>
          </section>

          <section className="flex flex-col gap-2">
            <div className="flex items-center gap-2 px-1">
              <h2 className="text-[13px] font-semibold">ローカル API（AI エージェント）</h2>
              <StatusLed status={apiLedStatus} />
            </div>
            <FieldGroup className="surface settings-group gap-0 p-0">
              <Field className="gap-1.5 px-4 py-3">
                <FieldLabel>エンドポイント</FieldLabel>
                <div className="flex items-center gap-1.5">
                  <p className="min-w-0 flex-1 truncate font-mono text-[12px]">
                    {controlApi.url}
                  </p>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-xs"
                    aria-label="エンドポイント URL をコピー"
                    disabled={!controlApi.url}
                    onClick={() =>
                      void copy(controlApi.url, "API URL をコピーしました")
                    }
                  >
                    <Copy />
                  </Button>
                </div>
                <FieldDescription>
                  {controlApi.error
                    ? controlApi.error
                    : controlApi.note ||
                      "127.0.0.1 限定。アプリの登録・編集・起動・停止を REST API から操作できます。"}
                </FieldDescription>
              </Field>

              <form onSubmit={submitApiPort}>
                <Field className="gap-1.5 px-4 py-3" data-invalid={!!apiPortError}>
                  <FieldLabel htmlFor="control-api-port">ポート</FieldLabel>
                  <div className="flex items-center gap-2">
                    <Input
                      id="control-api-port"
                      type="text"
                      inputMode="numeric"
                      placeholder="17341"
                      className="font-mono"
                      value={apiPortDraft}
                      aria-invalid={!!apiPortError}
                      autoComplete="off"
                      onChange={(event) => {
                        setApiPortDraft(event.target.value);
                        setApiPortError(null);
                      }}
                    />
                    <Button
                      type="submit"
                      size="sm"
                      disabled={savingControlApiPort}
                    >
                      {savingControlApiPort ? <Spinner /> : null}
                      保存
                    </Button>
                  </div>
                  {apiPortError ? (
                    <FieldError>{apiPortError}</FieldError>
                  ) : (
                    <FieldDescription>
                      未入力なら 17341 です。変更するとローカル API を再起動します。
                    </FieldDescription>
                  )}
                </Field>
              </form>

              <Field className="gap-1.5 px-4 py-3">
                <FieldLabel>Bearer トークン</FieldLabel>
                <div className="flex items-center gap-1">
                  <p className="min-w-0 flex-1 font-mono text-[12px] break-all text-muted-foreground">
                    {showToken ? controlApi.token || "—" : maskedToken}
                  </p>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-xs"
                    aria-label={showToken ? "トークンを隠す" : "トークンを表示"}
                    disabled={!controlApi.token}
                    onClick={() => setShowToken((v) => !v)}
                  >
                    {showToken ? <EyeOff /> : <Eye />}
                  </Button>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-xs"
                    aria-label="トークンをコピー"
                    disabled={!controlApi.token}
                    onClick={() =>
                      void copy(controlApi.token, "API トークンをコピーしました")
                    }
                  >
                    <Copy />
                  </Button>
                </div>
                {controlApi.infoPath ? (
                  <p className="font-mono text-[11px] leading-4 break-all text-muted-foreground">
                    接続情報: {controlApi.infoPath}
                  </p>
                ) : null}
                <div className="mt-1 flex flex-wrap gap-1.5">
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    disabled={!controlApi.url}
                    onClick={() =>
                      void copy(
                        buildAgentGuidePrompt(controlApi),
                        "AI エージェント向けガイドをコピーしました",
                      )
                    }
                  >
                    <Bot data-icon="inline-start" />
                    エージェント用ガイドをコピー
                  </Button>
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    disabled={rotatingControlApiToken}
                    onClick={onRotateControlApiToken}
                  >
                    {rotatingControlApiToken ? (
                      <Spinner />
                    ) : (
                      <RefreshCw data-icon="inline-start" />
                    )}
                    トークンを再生成
                  </Button>
                </div>
              </Field>
            </FieldGroup>
          </section>

          <section className="flex flex-col gap-2">
            <h2 className="px-1 text-[13px] font-semibold">このアプリ</h2>
            <FieldGroup className="surface settings-group gap-0 p-0">
              <Field className="gap-1.5 px-4 py-3">
                <FieldLabel>バージョン</FieldLabel>
                <p className="font-mono text-[13px] tabular-nums">
                  {formatVersionLabel(updater.version) || "—"}
                </p>
                <FieldDescription>{updateCopy(updater)}</FieldDescription>
                <div className="mt-1 flex flex-wrap gap-1.5">
                  {updater.stage === "ready" ? (
                    <Button
                      type="button"
                      size="sm"
                      disabled={updater.busy}
                      onClick={() => void updater.restart()}
                    >
                      {updater.busy ? <Spinner /> : <RotateCw data-icon="inline-start" />}
                      再起動して適用
                    </Button>
                  ) : (
                    <Button
                      type="button"
                      variant={updater.stage === "available" ? "default" : "outline"}
                      size="sm"
                      className="w-fit"
                      disabled={updater.busy || updater.stage === "downloading" || updater.stage === "verifying" || updater.stage === "installing"}
                      onClick={updater.check}
                    >
                      {updater.busy || updater.stage === "checking" ? (
                        <Spinner />
                      ) : (
                        <ArrowDownToLine data-icon="inline-start" />
                      )}
                      アップデートを確認
                    </Button>
                  )}
                </div>
              </Field>
            </FieldGroup>
          </section>

          <section className="flex flex-col gap-2">
            <h2 className="px-1 text-[13px] font-semibold">設定ファイル</h2>
            <FieldGroup className="surface settings-group gap-0 p-0">
              <Field className="gap-1.5 px-4 py-3">
                <FieldLabel>apps.yml</FieldLabel>
                <p className="font-mono text-[12px] leading-5 break-all text-muted-foreground">
                  {configPath || "パスを取得できませんでした"}
                </p>
                <FieldDescription>
                  Finder で保存先フォルダを開きます。
                </FieldDescription>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  className="mt-1 w-fit"
                  disabled={!configPath}
                  onClick={onRevealConfig}
                >
                  <FolderOpen data-icon="inline-start" />
                  フォルダを開く
                </Button>
              </Field>
            </FieldGroup>
          </section>
        </div>
      </DialogContent>
    </Dialog>
  );
}

function updateCopy(updater: AppUpdater): string {
  switch (updater.stage) {
    case "checking":
      return "GitHub Releases を確認しています。";
    case "available":
      return updater.release?.version
        ? `${formatVersionLabel(updater.release.version)} が利用できます。`
        : "新しいバージョンが利用できます。";
    case "downloading":
      return "新しいバージョンをダウンロードしています。";
    case "verifying":
    case "installing":
      return "ダウンロードを検証して、適用の準備をしています。";
    case "ready":
      return "再起動すると新しいバージョンに切り替わります。";
    case "up-to-date":
      return "最新のバージョンです。";
    case "error":
      return updater.error || "確認できませんでした。";
    default:
      return "GitHub Releases から、アプリ内で更新できます。";
  }
}

function parsePort(
  raw: string,
  fallback: number,
): { port: number; error: string | null } {
  const trimmed = raw.trim();
  if (trimmed === "") {
    return { port: fallback, error: null };
  }
  if (!/^\d+$/.test(trimmed)) {
    return { port: 0, error: "1 から 65535 の番号を入力してください" };
  }
  const port = Number(trimmed);
  if (port < 1 || port > 65535) {
    return { port: 0, error: "1 から 65535 の番号を入力してください" };
  }
  return { port, error: null };
}
