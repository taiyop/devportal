import { useEffect, useState } from "react";
import { Events, Updater } from "@wailsio/runtime";
import {
  appVersion,
  checkForUpdates,
  restartToUpdate,
  skipUpdate,
} from "@/api";

export type UpdateStage =
  | "idle"
  | "checking"
  | "available"
  | "downloading"
  | "verifying"
  | "installing"
  | "ready"
  | "up-to-date"
  | "error";

export interface UpdateRelease {
  version: string;
  name: string;
  notes: string;
  filename: string;
  size: number;
}

export interface UpdateProgress {
  written: number;
  total: number;
  rate: number;
}

export interface AppUpdater {
  version: string;
  stage: UpdateStage;
  release: UpdateRelease | null;
  progress: UpdateProgress | null;
  error: string | null;
  dismissed: boolean;
  busy: boolean;
  check: () => void;
  skip: () => void;
  remind: () => void;
  restart: () => Promise<void>;
}

export function useAppUpdater(preview: boolean): AppUpdater {
  const [version, setVersion] = useState("0.1.0");
  const [stage, setStage] = useState<UpdateStage>(preview ? "available" : "idle");
  const [release, setRelease] = useState<UpdateRelease | null>(
    preview ? previewRelease() : null,
  );
  const [progress, setProgress] = useState<UpdateProgress | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [dismissed, setDismissed] = useState(false);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (preview) {
      setVersion("0.1.0");
      setStage("available");
      setRelease(previewRelease());
      return;
    }

    let cancelled = false;
    void appVersion()
      .then((next) => {
        const normalized = normalizeVersion(next);
        if (!cancelled && normalized) setVersion(normalized);
      })
      .catch(() => {});

    const unsubs = [
      listen(Updater.Events.CheckStarted, () => {
        setStage("checking");
        setError(null);
        setProgress(null);
      }),
      listen(Updater.Events.UpdateAvailable, (event) => {
        const next = asRelease(event.data);
        if (next) setRelease(next);
        setStage("available");
        setDismissed(false);
        setError(null);
      }),
      listen(Updater.Events.NoUpdate, () => {
        setStage("up-to-date");
        setRelease(null);
        setProgress(null);
        setError(null);
      }),
      listen(Updater.Events.DownloadStarted, (event) => {
        const next = asRelease(event.data);
        if (next) setRelease(next);
        setStage("downloading");
        setDismissed(false);
        setProgress({ written: 0, total: 0, rate: 0 });
      }),
      listen(Updater.Events.DownloadProgress, (event) => {
        setProgress(asProgress(event.data));
        setStage("downloading");
      }),
      listen(Updater.Events.DownloadComplete, () => {
        setStage("verifying");
      }),
      listen(Updater.Events.Verifying, () => {
        setStage("verifying");
      }),
      listen(Updater.Events.Installing, () => {
        setStage("installing");
      }),
      listen(Updater.Events.UpdateReady, (event) => {
        const next = asRelease(event.data);
        if (next) setRelease(next);
        setStage("ready");
        setDismissed(false);
        setBusy(false);
      }),
      listen(Updater.Events.Error, (event) => {
        const info = asErrorInfo(event.data);
        setStage("error");
        setError(info);
        setBusy(false);
      }),
      listen(Updater.Events.Meta, (event) => {
        const meta = asMeta(event.data);
        if (meta.currentVersion) setVersion(normalizeVersion(meta.currentVersion));
      }),
    ];

    return () => {
      cancelled = true;
      unsubs.forEach((off) => off());
    };
  }, [preview]);

  function check() {
    if (preview) {
      setStage("checking");
      window.setTimeout(() => {
        setStage("available");
        setRelease(previewRelease());
        setDismissed(false);
      }, 420);
      return;
    }
    setBusy(true);
    setStage("checking");
    checkForUpdates();
    window.setTimeout(() => setBusy(false), 1200);
  }

  function skip() {
    const versionToSkip = release?.version ?? "";
    if (!preview && versionToSkip) skipUpdate(versionToSkip);
    setDismissed(true);
    setStage("idle");
  }

  function remind() {
    setDismissed(true);
  }

  async function restart() {
    if (preview) return;
    setBusy(true);
    try {
      await restartToUpdate();
    } finally {
      setBusy(false);
    }
  }

  return {
    version,
    stage,
    release,
    progress,
    error,
    dismissed,
    busy,
    check,
    skip,
    remind,
    restart,
  };
}

function listen(
  name: string,
  handler: (event: { data?: unknown }) => void,
): () => void {
  try {
    return Events.On(name, handler);
  } catch {
    return () => {};
  }
}

function previewRelease(): UpdateRelease {
  return {
    version: "0.2.0",
    name: "DevPortal 0.2.0",
    notes: "起動先の favicon 表示と、アプリ内アップデートの案内を追加しました。",
    filename: "DevPortal-darwin-arm64.zip",
    size: 12 * 1024 * 1024,
  };
}

function asRelease(data: unknown): UpdateRelease | null {
  if (!data || typeof data !== "object") return null;
  const raw = data as {
    version?: string;
    name?: string;
    notes?: string;
    artifact?: { filename?: string; size?: number };
  };
  const version = normalizeVersion(String(raw.version ?? ""));
  if (!version) return null;
  return {
    version,
    name: String(raw.name ?? "").trim(),
    notes: String(raw.notes ?? ""),
    filename: String(raw.artifact?.filename ?? ""),
    size: Number(raw.artifact?.size) || 0,
  };
}

function asProgress(data: unknown): UpdateProgress {
  const raw = (data ?? {}) as { written?: number; total?: number; rate?: number };
  return {
    written: Number(raw.written) || 0,
    total: Number(raw.total) || 0,
    rate: Number(raw.rate) || 0,
  };
}

function asErrorInfo(data: unknown): string {
  if (!data || typeof data !== "object") return "アップデートに失敗しました";
  const raw = data as { message?: string; stage?: string };
  const message = String(raw.message ?? "").trim();
  return message || "アップデートに失敗しました";
}

function asMeta(data: unknown): { currentVersion: string } {
  if (!data || typeof data !== "object") return { currentVersion: "" };
  const raw = data as { currentVersion?: string };
  return { currentVersion: normalizeVersion(String(raw.currentVersion ?? "")) };
}

export function normalizeVersion(version: string): string {
  const trimmed = version.trim();
  return trimmed.replace(/^[vV]/, "").trim();
}

export function formatVersionLabel(version: string): string {
  const normalized = normalizeVersion(version);
  return normalized ? `v${normalized}` : "";
}

export function formatBytes(bytes: number): string {
  if (!bytes || bytes < 0) return "";
  if (bytes < 1024) return `${bytes} B`;
  const units = ["KB", "MB", "GB"];
  let value = bytes / 1024;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit += 1;
  }
  const digits = value >= 10 || unit === 0 ? 0 : 1;
  return `${value.toFixed(digits)} ${units[unit]}`;
}

export function formatRate(bytesPerSec: number): string {
  if (!bytesPerSec) return "";
  return `${formatBytes(bytesPerSec)}/s`;
}

export function promptVisible(updater: AppUpdater): boolean {
  if (updater.dismissed) return false;
  if (updater.stage === "checking") return updater.release != null;
  return (
    updater.stage === "available" ||
    updater.stage === "downloading" ||
    updater.stage === "verifying" ||
    updater.stage === "installing" ||
    updater.stage === "ready" ||
    updater.stage === "error"
  );
}
