import { ArrowDownToLine, RotateCw, Sparkles } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import {
  formatBytes,
  formatRate,
  type AppUpdater,
} from "@/lib/updater";

export function UpdateBanner({
  updater,
}: {
  updater: AppUpdater;
}) {
  const release = updater.release;
  const versionLabel = release?.version ? `v${release.version}` : "新しいバージョン";
  const size = release?.size ? formatBytes(release.size) : "";
  const progress = progressPercent(updater);

  return (
    <aside className="surface mx-auto mb-4 flex max-w-3xl flex-col gap-2.5 rounded-[14px] px-3.5 py-3">
      <div className="flex items-start gap-3">
        <span className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-[10px] bg-primary/12 text-primary">
          {updater.stage === "ready" ? (
            <RotateCw className="size-4" />
          ) : (
            <Sparkles className="size-4" />
          )}
        </span>
        <div className="min-w-0 flex-1">
          <p className="text-[13px] font-semibold tracking-tight">
            {headline(updater, versionLabel)}
          </p>
          <p className="mt-0.5 text-[12px] leading-5 text-muted-foreground">
            {subcopy(updater, size)}
          </p>
        </div>
      </div>

      {(updater.stage === "downloading" ||
        updater.stage === "verifying" ||
        updater.stage === "installing") && (
        <div
          className="h-1 overflow-hidden rounded-full bg-secondary"
          role="progressbar"
          aria-valuemin={0}
          aria-valuemax={100}
          aria-valuenow={progress ?? undefined}
        >
          <div
            className="h-full rounded-full bg-primary transition-[width] duration-200"
            style={{
              width: progress == null ? "36%" : `${progress}%`,
              animation:
                progress == null ? "pulse 1.4s ease-in-out infinite" : undefined,
            }}
          />
        </div>
      )}

      <div className="flex flex-wrap items-center gap-1.5">
        {updater.stage === "available" || updater.stage === "checking" ? (
          <>
            <Button
              size="sm"
              onClick={updater.check}
              disabled={updater.busy || updater.stage === "checking"}
            >
              {updater.busy || updater.stage === "checking" ? (
                <Spinner />
              ) : (
                <ArrowDownToLine data-icon="inline-start" />
              )}
              今すぐ更新
            </Button>
            <Button size="sm" variant="ghost" onClick={updater.remind}>
              あとで
            </Button>
            <Button size="sm" variant="ghost" onClick={updater.skip}>
              このバージョンをスキップ
            </Button>
          </>
        ) : null}
        {updater.stage === "ready" ? (
          <Button size="sm" onClick={() => void updater.restart()} disabled={updater.busy}>
            {updater.busy ? <Spinner /> : <RotateCw data-icon="inline-start" />}
            再起動して適用
          </Button>
        ) : null}
        {updater.stage === "error" ? (
          <>
            <Button size="sm" onClick={updater.check} disabled={updater.busy}>
              {updater.busy ? <Spinner /> : null}
              再試行
            </Button>
            <Button size="sm" variant="ghost" onClick={updater.remind}>
              閉じる
            </Button>
          </>
        ) : null}
        {(updater.stage === "downloading" ||
          updater.stage === "verifying" ||
          updater.stage === "installing") &&
        updater.progress?.rate ? (
          <span className="ml-auto font-mono text-[11px] text-muted-foreground">
            {formatRate(updater.progress.rate)}
          </span>
        ) : null}
      </div>
    </aside>
  );
}

function headline(updater: AppUpdater, versionLabel: string): string {
  switch (updater.stage) {
    case "checking":
      return `${versionLabel} を確認しています`;
    case "downloading":
      return `${versionLabel} をダウンロードしています`;
    case "verifying":
      return "ダウンロードを検証しています";
    case "installing":
      return "アップデートを準備しています";
    case "ready":
      return `${versionLabel} を適用できます`;
    case "error":
      return "アップデートできませんでした";
    default:
      return `${versionLabel} が利用できます`;
  }
}

function subcopy(updater: AppUpdater, size: string): string {
  if (updater.stage === "error") {
    return updater.error || "ネットワークを確認して、もう一度試してください。";
  }
  if (updater.stage === "ready") {
    return "再起動すると、今のウィンドウを閉じたあとに新しいバージョンで開きます。";
  }
  if (updater.stage === "downloading") {
    const written = updater.progress?.written ?? 0;
    const total = updater.progress?.total ?? 0;
    if (total > 0) {
      return `${formatBytes(written)} / ${formatBytes(total)}`;
    }
    return "GitHub Releases から取得しています。";
  }
  if (updater.stage === "verifying" || updater.stage === "installing") {
    return "署名とファイルの置き換え準備が終わるまで、このまま待ってください。";
  }
  const bits = ["GitHub Releases の最新版です"];
  if (size) bits.push(size);
  return bits.join(" · ");
}

function progressPercent(updater: AppUpdater): number | null {
  const total = updater.progress?.total ?? 0;
  const written = updater.progress?.written ?? 0;
  if (updater.stage === "verifying" || updater.stage === "installing") return 100;
  if (!total) return null;
  return Math.max(4, Math.min(100, Math.round((written / total) * 100)));
}
