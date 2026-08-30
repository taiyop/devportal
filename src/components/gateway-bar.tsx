import { Label } from "@/components/ui/label";
import { Spinner } from "@/components/ui/spinner";
import { Switch } from "@/components/ui/switch";
import { StatusLed } from "@/components/status-badge";
import { cn } from "@/lib/utils";
import type { GatewayStatus } from "@/api";
import type { AppStatus } from "@/types";

export function GatewayBar({
  status,
  busy,
  onToggle,
}: {
  status: GatewayStatus;
  busy: boolean;
  onToggle: (running: boolean) => void;
}) {
  const kind: AppStatus = status.running
    ? "running"
    : status.error
      ? "error"
      : "stopped";
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
      aria-busy={busy || undefined}
    >
      <div className="flex items-center gap-2">
        <StatusLed status={kind} />
        <Label
          htmlFor="gateway-toggle"
          className="min-w-0 flex-1 cursor-pointer text-[13px] font-medium"
        >
          リバースプロキシ
        </Label>
        {busy ? <Spinner className="size-3.5 text-muted-foreground" /> : null}
        <Switch
          id="gateway-toggle"
          checked={status.running}
          disabled={busy}
          className="data-checked:bg-phosphor"
          onCheckedChange={onToggle}
        />
      </div>
      <p className="mt-1 truncate font-mono text-[11px] text-muted-foreground" title={host}>
        {host}
      </p>
      <p
        className={cn(
          "mt-1.5 text-[11px] leading-4",
          status.error ? "text-coral" : "text-muted-foreground",
        )}
      >
        {status.error
          ? status.error
          : status.running
            ? status.note || "ホスト名 URL を受けています。"
            : "ホスト名 URL を使うときに開始します。"}
      </p>
    </section>
  );
}
