import type { AppStatus } from "@/types";

export const STATUS_LABEL: Record<AppStatus, string> = {
  stopped: "停止",
  idle: "待ち受け",
  starting: "起動中",
  running: "稼働",
  error: "異常",
};

export type BoardFilter = "all" | "live" | "idle" | "off";

export function isOff(status: AppStatus) {
  return status === "stopped" || status === "error" || status === "idle";
}

export function matchesFilter(status: AppStatus, filter: BoardFilter) {
  if (filter === "all") return true;
  if (filter === "live") return status === "running" || status === "starting";
  if (filter === "idle") return status === "idle";
  return status === "stopped" || status === "error";
}
