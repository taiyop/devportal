import { type ReactNode } from "react";
import {
  Copy,
  CopyPlus,
  ExternalLink,
  Pencil,
  Play,
  ScrollText,
  Square,
  Trash2,
} from "lucide-react";
import { toast } from "sonner";
import { AppFavicon } from "@/components/app-favicon";
import { KeepAliveControls } from "@/components/keep-alive-controls";
import { StatusBadge } from "@/components/status-badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Spinner } from "@/components/ui/spinner";
import { copyText } from "@/lib/clipboard";
import { isOff } from "@/lib/status";
import { resolvedAppPortEnv, resolvedPortEnv, type AppView } from "@/types";

export function AppDetailsDialog({
  app,
  busy,
  onOpenChange,
  onToggle,
  onEdit,
  onDuplicate,
  onDelete,
  onOpenUrl,
  onOpenLogs,
  onSetPinned,
}: {
  app: AppView | null;
  busy: boolean;
  onOpenChange: (open: boolean) => void;
  onToggle: () => void;
  onEdit: () => void;
  onDuplicate: () => void;
  onDelete: () => void;
  onOpenUrl: () => void;
  onOpenLogs: () => void;
  onSetPinned: (pinned: boolean) => void;
}) {
  const off = app ? isOff(app.status) : true;
  const locked = app?.status === "running" || app?.status === "starting";

  async function copy(value: string, okMessage: string) {
    const ok = await copyText(value);
    toast[ok ? "success" : "error"](ok ? okMessage : "コピーできませんでした");
  }

  const host =
    app?.hostname ? `${app.hostname}.devportal.localhost` : "";
  const envLine =
    app && app.env.filter((row) => row.name).length > 0
      ? app.env
          .filter((row) => row.name)
          .map((row) => `${row.name}=${row.value}`)
          .join("\n")
      : "";
  const backends = app?.backends ?? [];

  return (
    <Dialog open={app !== null} onOpenChange={onOpenChange}>
      <DialogContent className="flex max-h-[calc(100dvh-1.5rem)] w-[calc(100%-1.5rem)] max-w-lg flex-col gap-0 overflow-hidden p-0 sm:max-w-lg">
        <DialogHeader className="shrink-0 gap-1.5 border-b border-border px-4 py-3 pr-12">
          <div className="flex min-w-0 flex-wrap items-center gap-2">
            {app && (
              <AppFavicon
                name={app.name}
                favicon={app.favicon}
                status={app.status}
                className="mt-0"
              />
            )}
            <DialogTitle className="truncate text-[15px] font-semibold tracking-tight">
              {app?.name ?? "詳細"}
            </DialogTitle>
            {app && <StatusBadge status={app.status} />}
          </div>
          <DialogDescription className="sr-only">
            {app ? `${app.name} の登録内容` : "アプリの詳細"}
          </DialogDescription>
        </DialogHeader>

        <div className="min-h-0 flex-1 overflow-y-auto">
          {app && (
            <dl className="settings-group">
              {app.description ? (
                <DetailRow label="説明">{app.description}</DetailRow>
              ) : null}
              {host ? (
                <DetailRow label="ホスト" mono>
                  {host}
                </DetailRow>
              ) : null}
              {app.url ? (
                <DetailRow
                  label="URL"
                  action={
                    <div className="flex items-center gap-0.5">
                      <Button
                        type="button"
                        variant="ghost"
                        size="icon-xs"
                        aria-label="URLをコピー"
                        onClick={() => copy(app.url!, "URL をコピーしました")}
                      >
                        <Copy />
                      </Button>
                      <Button
                        type="button"
                        variant="ghost"
                        size="icon-xs"
                        aria-label="URLを開く"
                        onClick={onOpenUrl}
                      >
                        <ExternalLink />
                      </Button>
                    </div>
                  }
                >
                  <span className="font-mono text-[12px] break-all">{app.url}</span>
                </DetailRow>
              ) : null}
              <DetailRow label="フォルダ" mono>
                {app.folder}
              </DetailRow>
              <DetailRow label="コマンド">
                <span className="rounded-md bg-command px-2 py-1 font-mono text-[12px] text-command-foreground">
                  $ {app.command}
                </span>
              </DetailRow>
              {envLine ? (
                <DetailRow label="環境変数" mono>
                  <span className="whitespace-pre-wrap">{envLine}</span>
                </DetailRow>
              ) : null}
              <DetailRow label="ポート">
                {app.portMode === "auto" ? "自動" : "手動"}
                {app.port != null ? ` · ${app.port}` : " · 未割当"}
                {` · ${resolvedPortEnv(app.portEnv)}`}
              </DetailRow>
              {backends.map((backend, index) => {
                const title = backend.name.trim() || `BE${index + 1}`;
                const backendEnv = backend.env
                  .filter((row) => row.name)
                  .map((row) => `${row.name}=${row.value}`)
                  .join("\n");
                return (
                  <div key={`${title}-${index}`}>
                    <DetailRow label={`${title} フォルダ`} mono>
                      {backend.folder}
                    </DetailRow>
                    <DetailRow label={`${title} コマンド`}>
                      <span className="rounded-md bg-command px-2 py-1 font-mono text-[12px] text-command-foreground">
                        $ {backend.command}
                      </span>
                    </DetailRow>
                    {backendEnv ? (
                      <DetailRow label={`${title} 環境変数`} mono>
                        <span className="whitespace-pre-wrap">{backendEnv}</span>
                      </DetailRow>
                    ) : null}
                    <DetailRow label={`${title} ポート`}>
                      {backend.portMode === "auto" ? "自動" : "手動"}
                      {backend.port != null ? ` · ${backend.port}` : " · 未割当"}
                      {` · ${resolvedPortEnv(backend.portEnv)}`}
                      {` · アプリへ ${resolvedAppPortEnv(backend.appPortEnv, index)}`}
                    </DetailRow>
                  </div>
                );
              })}
              <DetailRow label="起動">
                {[
                  app.hostname ? "ドメインで起動" : "手動起動",
                  app.idleStopMin > 0
                    ? `アイドル ${app.idleStopMin}分で停止`
                    : "自動停止なし",
                  app.pinned ? "Keep alive" : null,
                ]
                  .filter(Boolean)
                  .join(" · ")}
              </DetailRow>
              {app.pid ? (
                <DetailRow label="PID">{app.pid}</DetailRow>
              ) : null}
              {backends.map((backend, index) => {
                const pid = backend.pid ?? app.backendPids[index];
                if (!pid) return null;
                const title = backend.name.trim() || `BE${index + 1}`;
                return (
                  <DetailRow key={`${title}-pid`} label={`${title} PID`}>
                    {pid}
                  </DetailRow>
                );
              })}
              {app.status === "idle" ? (
                <DetailRow label="待ち受け">
                  このドメインにアクセスすると起動します
                  {app.idleStopMin > 0
                    ? ` · ${app.idleStopMin}分アクセスがなければ停止`
                    : ""}
                </DetailRow>
              ) : null}
              {(app.status === "running" || app.status === "starting") &&
                app.idleStopMin > 0 && (
                  <DetailRow label="稼働">
                    <KeepAliveControls
                      app={app}
                      disabled={busy}
                      onSetPinned={onSetPinned}
                    />
                  </DetailRow>
                )}
              {app.error ? (
                <DetailRow label="エラー">
                  <span className="text-coral">{app.error}</span>
                </DetailRow>
              ) : null}
            </dl>
          )}
        </div>

        <div className="flex shrink-0 flex-wrap items-center gap-1.5 border-t border-border bg-muted/40 px-3 py-2.5">
          <Button
            variant={off ? "default" : "destructive"}
            size="sm"
            disabled={!app || busy}
            onClick={onToggle}
          >
            {busy ? (
              <Spinner />
            ) : off ? (
              <Play data-icon="inline-start" />
            ) : (
              <Square data-icon="inline-start" />
            )}
            {busy ? "処理中" : off ? "起動" : "終了"}
          </Button>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            disabled={!app}
            onClick={onOpenLogs}
          >
            <ScrollText data-icon="inline-start" />
            ログ
          </Button>
          <div className="ml-auto flex items-center gap-1">
            <Button
              type="button"
              variant="outline"
              size="icon"
              aria-label="複製"
              disabled={!app}
              onClick={onDuplicate}
            >
              <CopyPlus />
            </Button>
            <Button
              type="button"
              variant="outline"
              size="icon"
              aria-label="編集"
              disabled={!app}
              onClick={onEdit}
            >
              <Pencil />
            </Button>
            {app && !locked && (
              <Button
                type="button"
                variant="ghost"
                size="icon"
                className="text-destructive hover:bg-destructive/10 hover:text-destructive"
                aria-label="削除"
                disabled={busy}
                onClick={onDelete}
              >
                <Trash2 />
              </Button>
            )}
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}

function DetailRow({
  label,
  children,
  mono,
  action,
}: {
  label: string;
  children: ReactNode;
  mono?: boolean;
  action?: ReactNode;
}) {
  return (
    <div className="flex items-start gap-3 px-4 py-2.5">
      <dt className="w-24 shrink-0 pt-0.5 text-[11px] text-muted-foreground">
        {label}
      </dt>
      <dd
        className={
          mono
            ? "min-w-0 flex-1 font-mono text-[12px] leading-5 break-all"
            : "min-w-0 flex-1 text-[13px] leading-5"
        }
      >
        {children}
      </dd>
      {action ? <div className="shrink-0">{action}</div> : null}
    </div>
  );
}
