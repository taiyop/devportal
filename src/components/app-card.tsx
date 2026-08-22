import { ExternalLink, Info, Pencil, Play, Square } from "lucide-react";
import { KeepAliveControls } from "@/components/keep-alive-controls";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { isOff } from "@/lib/status";
import type { AppView } from "@/types";
import { AppFavicon } from "@/components/app-favicon";
import { StatusBadge } from "@/components/status-badge";

export function AppCard({
  app,
  index,
  busy,
  onToggle,
  onEdit,
  onDetails,
  onOpenUrl,
  onSetPinned,
}: {
  app: AppView;
  index: number;
  busy: boolean;
  onToggle: () => void;
  onEdit: () => void;
  onDetails: () => void;
  onOpenUrl: () => void;
  onSetPinned: (pinned: boolean) => void;
}) {
  const off = isOff(app.status);

  return (
    <article
      className="surface relative flex w-full items-start gap-3 rounded-[14px] px-3 py-2.5 animate-rise"
      style={{ animationDelay: `${index * 40}ms` }}
    >
      <AppFavicon name={app.name} favicon={app.favicon} status={app.status} />
      <div className="min-w-0 flex-1">
        <div className="flex min-w-0 items-center gap-2">
          <h2 className="truncate text-[15px] leading-tight font-semibold tracking-tight">
            {app.name}
          </h2>
          <StatusBadge status={app.status} />
        </div>
        {app.description ? (
          <p className="mt-1 line-clamp-2 text-[13px] leading-5 text-muted-foreground">
            {app.description}
          </p>
        ) : null}
        {app.url ? (
          <button
            type="button"
            className="mt-0.5 flex max-w-full items-center gap-1 text-left font-mono text-[12px] text-primary hover:underline"
            title={`${app.url} を開く`}
            onClick={onOpenUrl}
          >
            <span className="truncate">{app.url}</span>
            <ExternalLink className="size-3 shrink-0" />
          </button>
        ) : null}
        <KeepAliveControls
          app={app}
          className="mt-1.5"
          disabled={busy}
          onSetPinned={onSetPinned}
        />
      </div>

      <div className="flex shrink-0 items-center gap-1.5">
        <Button
          variant={off ? "default" : "destructive"}
          size="sm"
          disabled={busy}
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

        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                type="button"
                variant="outline"
                size="icon"
                aria-label={`${app.name} の詳細`}
                onClick={onDetails}
              />
            }
          >
            <Info />
          </TooltipTrigger>
          <TooltipContent>詳細</TooltipContent>
        </Tooltip>

        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                type="button"
                variant="outline"
                size="icon"
                aria-label={`${app.name} を編集`}
                onClick={onEdit}
              />
            }
          >
            <Pencil />
          </TooltipTrigger>
          <TooltipContent>編集</TooltipContent>
        </Tooltip>
      </div>
    </article>
  );
}
