import { Badge } from "@/components/ui/badge";
import { STATUS_LABEL } from "@/lib/status";
import { cn } from "@/lib/utils";
import type { AppStatus } from "@/types";

const LED: Record<AppStatus, string> = {
  stopped: "bg-[var(--led-off)]",
  idle: "bg-phosphor-dim led-idle",
  starting: "bg-amber led-starting",
  running: "bg-phosphor led-running",
  error: "bg-coral",
};

const PILL: Record<AppStatus, string> = {
  stopped: "bg-muted text-muted-foreground",
  idle: "bg-phosphor/12 text-phosphor-dim",
  starting: "bg-amber/15 text-amber",
  running: "bg-phosphor/12 text-phosphor",
  error: "bg-coral/12 text-coral",
};

export function StatusLed({ status, className }: { status: AppStatus; className?: string }) {
  return (
    <span
      aria-hidden="true"
      className={cn("size-2 shrink-0 rounded-full", LED[status], className)}
    />
  );
}

export function StatusBadge({ status }: { status: AppStatus }) {
  return (
    <Badge variant="ghost" className={cn("h-5 rounded-full px-2 text-[11px] font-medium", PILL[status])}>
      {STATUS_LABEL[status]}
    </Badge>
  );
}
