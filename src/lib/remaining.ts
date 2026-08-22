import { useEffect, useState } from "react";
import type { AppView } from "@/types";

export function remainingSeconds(
  idleUntil: number | null,
  nowMs = Date.now(),
): number | null {
  if (idleUntil == null) return null;
  return Math.max(0, idleUntil - Math.floor(nowMs / 1000));
}

export function formatRemaining(
  idleUntil: number | null,
  nowMs = Date.now(),
): string | null {
  const sec = remainingSeconds(idleUntil, nowMs);
  if (sec == null) return null;
  const h = Math.floor(sec / 3600);
  const m = Math.floor((sec % 3600) / 60);
  const s = sec % 60;
  if (h > 0) {
    return `${h}:${String(m).padStart(2, "0")}:${String(s).padStart(2, "0")}`;
  }
  return `${m}:${String(s).padStart(2, "0")}`;
}

export function useRemaining(idleUntil: number | null): string | null {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (idleUntil == null) return;
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [idleUntil]);
  return formatRemaining(idleUntil, now);
}

export function canKeepAlive(app: AppView) {
  return (
    (app.status === "running" || app.status === "starting") &&
    app.idleStopMin > 0
  );
}
