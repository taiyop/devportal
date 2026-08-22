import { Pin, Timer } from "lucide-react";
import { Button } from "@/components/ui/button";
import { canKeepAlive, remainingSeconds, useRemaining } from "@/lib/remaining";
import { cn } from "@/lib/utils";
import type { AppView } from "@/types";

export function KeepAliveControls({
  app,
  disabled,
  className,
  onSetPinned,
}: {
  app: AppView;
  disabled?: boolean;
  className?: string;
  onSetPinned: (pinned: boolean) => void;
}) {
  const remaining = useRemaining(app.pinned ? null : app.idleUntil);
  const low =
    !app.pinned && (remainingSeconds(app.idleUntil) ?? Number.POSITIVE_INFINITY) < 60;

  if (!canKeepAlive(app)) return null;

  return (
    <div className={cn("flex min-w-0 flex-wrap items-center gap-2", className)}>
      {app.pinned ? (
        <span className="text-[12px] text-muted-foreground">永続</span>
      ) : (
        <span
          className={cn(
            "inline-flex items-center gap-1 font-mono text-[12px] tabular-nums",
            low ? "text-coral" : "text-muted-foreground",
          )}
          title="アクセスがなければ自動停止するまでの残り時間"
        >
          <Timer className="size-3" />
          残り {remaining ?? `${app.idleStopMin}:00`}
        </span>
      )}
      <Button
        type="button"
        size="sm"
        variant={app.pinned ? "default" : "outline"}
        aria-pressed={app.pinned}
        aria-label="Keep alive"
        disabled={disabled}
        onClick={() => onSetPinned(!app.pinned)}
      >
        <Pin data-icon="inline-start" />
        Keep alive
      </Button>
    </div>
  );
}
