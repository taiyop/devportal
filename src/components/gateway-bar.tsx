import { Play, Square } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import { StatusLed } from "@/components/status-badge";
import { cn } from "@/lib/utils";
import type { GatewayStatus } from "@/api";
import type { AppStatus } from "@/types";

export function GatewayBar({
  status,
  busy,
  onStart,
  onStop,
}: {
  status: GatewayStatus;
  busy: boolean;
  onStart: () => void;
  onStop: () => void;
}) {
  const kind: AppStatus = status.running
    ? "running"
    : status.error
      ? "error"
      : "stopped";
  const label = status.running ? "稼働中" : status.error ? "開始できません" : "停止中";
  const host =
    status.port && status.port !== 80
      ? `*.devportal.localhost:${status.port}`
      : "*.devportal.localhost";

  return (
    <section
      className={cn(
        "surface mt-auto rounded-[12px] px-3 py-3",
        status.error && "ring-1 ring-coral/30",
      )}
      aria-label="リバースプロキシ"
    >
      <div className="flex items-center gap-2">
        <StatusLed status={kind} />
        <p className="text-[13px] font-medium">リバースプロキシ</p>
      </div>
      <p className="mt-1 truncate font-mono text-[11px] text-muted-foreground" title={host}>
        {host}
      </p>
      <p className="mt-1.5 text-[11px] leading-4 text-muted-foreground">
        {status.error
          ? status.error
          : status.running
            ? status.note || "ホスト名 URL を受けています。"
            : "ホスト名 URL を使うときに開始します。"}
      </p>
      <div className="mt-2.5 flex items-center justify-between gap-2">
        <span
          className={cn(
            "text-[11px] font-medium",
            status.running && "text-phosphor",
            status.error && "text-coral",
            !status.running && !status.error && "text-muted-foreground",
          )}
        >
          {label}
        </span>
        {status.running ? (
          <Button type="button" variant="outline" size="xs" disabled={busy} onClick={onStop}>
            {busy ? <Spinner /> : <Square data-icon="inline-start" />}
            終了
          </Button>
        ) : (
          <Button type="button" size="xs" disabled={busy} onClick={onStart}>
            {busy ? <Spinner /> : <Play data-icon="inline-start" />}
            {status.error ? "再試行" : "開始"}
          </Button>
        )}
      </div>
    </section>
  );
}
