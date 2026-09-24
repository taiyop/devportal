import {
  Copy,
  CopyPlus,
  ExternalLink,
  GripVertical,
  Info,
  Pencil,
  Play,
  Square,
} from "lucide-react";
import type { DragEvent, KeyboardEvent } from "react";
import { toast } from "sonner";
import { KeepAliveControls } from "@/components/keep-alive-controls";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { copyText } from "@/lib/clipboard";
import { isOff } from "@/lib/status";
import type { AppView } from "@/types";
import { AppFavicon } from "@/components/app-favicon";
import { StatusBadge } from "@/components/status-badge";

export interface AppCardReorder {
  onDragStart: (event: DragEvent<HTMLButtonElement>) => void;
  onDragEnd: () => void;
  onKeyDown: (event: KeyboardEvent<HTMLButtonElement>) => void;
}

export function AppCard({
  app,
  index,
  busy,
  reorder,
  onToggle,
  onEdit,
  onDuplicate,
  onDetails,
  onOpenUrl,
  onSetPinned,
}: {
  app: AppView;
  index: number;
  busy: boolean;
  reorder?: AppCardReorder;
  onToggle: () => void;
  onEdit: () => void;
  onDuplicate: () => void;
  onDetails: () => void;
  onOpenUrl: () => void;
  onSetPinned: (pinned: boolean) => void;
}) {
  const off = isOff(app.status);

  async function copyUrl() {
    if (!app.url) return;
    const ok = await copyText(app.url);
    toast[ok ? "success" : "error"](
      ok ? "URL をコピーしました" : "コピーできませんでした",
    );
  }

  return (
    <article
      className="surface relative flex w-full flex-col gap-2 rounded-[14px] p-3 animate-rise"
      style={{ animationDelay: `${index * 40}ms` }}
    >
      <div className="flex items-start gap-2.5">
        {reorder ? (
          <Tooltip>
            <TooltipTrigger
              render={
                <button
                  type="button"
                  draggable
                  aria-label={`${app.name} を並べ替え（矢印キーでも移動できます）`}
                  className="mt-1.5 -ml-1 inline-flex size-5 shrink-0 cursor-grab items-center justify-center rounded-md text-muted-foreground/60 outline-none hover:bg-muted hover:text-muted-foreground focus-visible:ring-2 focus-visible:ring-ring/60 active:cursor-grabbing"
                  onDragStart={reorder.onDragStart}
                  onDragEnd={reorder.onDragEnd}
                  onKeyDown={reorder.onKeyDown}
                />
              }
            >
              <GripVertical />
            </TooltipTrigger>
            <TooltipContent>ドラッグまたは矢印キーで並べ替え</TooltipContent>
          </Tooltip>
        ) : null}
        <AppFavicon name={app.name} favicon={app.favicon} status={app.status} />
        <div className="min-w-0 flex-1">
          <h2 className="truncate text-[15px] leading-tight font-semibold tracking-tight">
            {app.name}
          </h2>
          <div className="mt-1 flex">
            <StatusBadge status={app.status} />
          </div>
        </div>
      </div>

      {app.description ? (
        <p className="line-clamp-2 text-[13px] leading-5 text-muted-foreground">
          {app.description}
        </p>
      ) : null}
      {app.url ? (
        <div className="flex min-w-0 items-center gap-1.5">
          <button
            type="button"
            className="flex min-w-0 items-center gap-1 text-left font-mono text-[12px] text-primary hover:underline"
            title={`${app.url} を開く`}
            onClick={onOpenUrl}
          >
            <span className="min-w-0 truncate">{app.url}</span>
            <ExternalLink className="size-3 shrink-0" />
          </button>
          <Tooltip>
            <TooltipTrigger
              render={
                <Button
                  type="button"
                  variant="ghost"
                  size="icon-xs"
                  className="shrink-0 text-muted-foreground"
                  aria-label="URLをコピー"
                  onClick={copyUrl}
                />
              }
            >
              <Copy />
            </TooltipTrigger>
            <TooltipContent>URLをコピー</TooltipContent>
          </Tooltip>
        </div>
      ) : null}
      <KeepAliveControls
        app={app}
        disabled={busy}
        onSetPinned={onSetPinned}
      />

      <div className="mt-auto -mx-3 flex items-center gap-1 border-t px-3 pt-2">
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

        <div className="ml-auto flex items-center gap-1">
          <Tooltip>
            <TooltipTrigger
              render={
                <Button
                  type="button"
                  variant="outline"
                  size="icon-sm"
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
                  size="icon-sm"
                  aria-label={`${app.name} を複製`}
                  onClick={onDuplicate}
                />
              }
            >
              <CopyPlus />
            </TooltipTrigger>
            <TooltipContent>複製</TooltipContent>
          </Tooltip>

          <Tooltip>
            <TooltipTrigger
              render={
                <Button
                  type="button"
                  variant="outline"
                  size="icon-sm"
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
      </div>
    </article>
  );
}
