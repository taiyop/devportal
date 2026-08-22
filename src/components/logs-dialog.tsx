import { useEffect, useRef } from "react";
import { Copy, ScrollText } from "lucide-react";
import { toast } from "sonner";
import { StatusBadge, StatusLed } from "@/components/status-badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { ScrollArea } from "@/components/ui/scroll-area";
import { copyText } from "@/lib/clipboard";
import type { AppView, LogEvent } from "@/types";

export function LogsDialog({
  app,
  logs,
  onOpenChange,
}: {
  app: AppView | null;
  logs: LogEvent[];
  onOpenChange: (open: boolean) => void;
}) {
  const logEndRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (app) {
      logEndRef.current?.scrollIntoView({ block: "end" });
    }
  }, [app, logs]);

  async function copyLogs() {
    const text =
      logs.length === 0
        ? ""
        : logs
            .map((line) => {
              const meta = streamMeta(line.stream);
              const mark = meta.kind === "stderr" ? "!" : " ";
              const source = meta.source === "backend" ? "b" : " ";
              return `${source}${mark} ${line.line}`;
            })
            .join("\n");
    if (!text) {
      toast.error("コピーするログがありません");
      return;
    }
    const ok = await copyText(text);
    toast[ok ? "success" : "error"](
      ok ? "ログをコピーしました" : "コピーできませんでした",
    );
  }

  return (
    <Dialog open={app !== null} onOpenChange={onOpenChange}>
      <DialogContent className="flex w-[calc(100%-1.5rem)] max-w-3xl flex-col gap-0 overflow-hidden p-0 sm:max-w-3xl">
        <DialogHeader className="gap-1.5 border-b border-border px-4 py-3 pr-12">
          <div className="flex flex-wrap items-center gap-2">
            {app && <StatusLed status={app.status} />}
            <DialogTitle className="truncate text-[15px] font-semibold tracking-tight">
              {app?.name ?? "ログ"}
            </DialogTitle>
            {app && <StatusBadge status={app.status} />}
          </div>
          <DialogDescription className="truncate font-mono text-[12px]">
            {app
              ? app.backends.length > 0
                ? `$ ${app.command}  +  ${app.backends.map((backend) => `$ ${backend.command}`).join("  +  ")}`
                : `$ ${app.command}`
              : "標準出力"}
          </DialogDescription>
        </DialogHeader>

        <ScrollArea className="h-[min(28rem,calc(100vh-16rem))] bg-log text-log-foreground">
          <div className="p-4 font-mono text-[12.5px] leading-relaxed">
            {logs.length === 0 ? (
              <p className="flex items-center gap-2 text-log-foreground/55">
                <ScrollText className="size-3.5" />
                ログはまだありません
              </p>
            ) : (
              logs.map((line, index) => {
                const meta = streamMeta(line.stream);
                return (
                  <p
                    key={`${index}-${line.line}`}
                    className={
                      meta.kind === "stderr"
                        ? "whitespace-pre-wrap text-red-400"
                        : "whitespace-pre-wrap"
                    }
                  >
                    <span className="mr-2 select-none text-log-foreground/35">
                      {meta.source === "backend"
                        ? "b"
                        : meta.kind === "stderr"
                          ? "!"
                          : "·"}
                    </span>
                    {line.line}
                  </p>
                );
              })
            )}
            <div ref={logEndRef} />
          </div>
        </ScrollArea>

        <div className="flex items-center justify-between gap-2 border-t border-border bg-muted/40 px-4 py-2.5">
          <span className="text-[11px] text-muted-foreground">
            {logs.length} 行
          </span>
          <Button type="button" variant="outline" size="sm" onClick={copyLogs}>
            <Copy data-icon="inline-start" />
            コピー
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}

function streamMeta(stream: string): {
  kind: "stdout" | "stderr";
  source: "app" | "backend";
} {
  if (stream === "backend-err" || stream.startsWith("backend-err:")) {
    return { kind: "stderr", source: "backend" };
  }
  if (stream === "backend" || stream.startsWith("backend:")) {
    return { kind: "stdout", source: "backend" };
  }
  return {
    kind: stream === "stderr" ? "stderr" : "stdout",
    source: "app",
  };
}
