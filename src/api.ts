import { Events } from "@wailsio/runtime";
import { PortalService } from "./bindings/github.com/taiyop/devportal";
import type { AppInput, AppView, Category, LogEvent } from "./types";
import type {
  AppInput as BoundAppInput,
  AppView as BoundAppView,
  CategoryEntry as BoundCategoryEntry,
  GatewayStatus as BoundGatewayStatus,
  LogEvent as BoundLogEvent,
} from "./bindings/github.com/taiyop/devportal";

export const isWails = () => {
  if (typeof window === "undefined") return false;
  const win = window as Window & {
    chrome?: { webview?: { postMessage?: unknown } };
    webkit?: { messageHandlers?: { external?: { postMessage?: unknown } } };
    wails?: { invoke?: unknown };
  };
  // @wailsio/runtime always creates window._wails in a browser, so that
  // cannot mean the desktop webview is actually present.
  return Boolean(
    win.chrome?.webview?.postMessage ||
      win.webkit?.messageHandlers?.external?.postMessage ||
      win.wails?.invoke,
  );
};

export const isBrowserPreview = () => import.meta.env.DEV && !isWails();

export async function listApps(): Promise<AppView[]> {
  return ((await PortalService.ListApps()) ?? []).map(asAppView);
}

export async function upsertApp(input: AppInput): Promise<AppView> {
  const payload: BoundAppInput = {
    id: input.id ?? null,
    name: input.name,
    description: input.description,
    hostname: input.hostname,
    categoryId: input.categoryId,
    folder: input.folder,
    command: input.command,
    portMode: input.portMode as BoundAppInput["portMode"],
    port: input.port,
    portEnv: input.portEnv,
    wakeOnRequest: input.wakeOnRequest,
    idleStopMin: input.idleStopMin,
    env: input.env,
    backends: input.backends.map((backend) => ({
      name: backend.name,
      folder: backend.folder,
      command: backend.command,
      portMode: backend.portMode as BoundAppInput["portMode"],
      port: backend.port,
      portEnv: backend.portEnv,
      appPortEnv: backend.appPortEnv,
      env: backend.env,
    })),
  };
  return asAppView(await PortalService.UpsertApp(payload));
}

export async function deleteApp(id: string): Promise<void> {
  await PortalService.DeleteApp(id);
}

export async function reorderApps(ids: string[]): Promise<AppView[]> {
  return ((await PortalService.ReorderApps(ids)) ?? []).map(asAppView);
}

export async function listCategories(): Promise<Category[]> {
  return ((await PortalService.ListCategories()) ?? []).map(asCategory);
}

export async function upsertCategory(
  id: string | null,
  name: string,
  color: string,
): Promise<Category> {
  return asCategory(
    await PortalService.UpsertCategory(id ?? "", name, color),
  );
}

export async function deleteCategory(id: string): Promise<void> {
  await PortalService.DeleteCategory(id);
}

export async function reorderCategories(ids: string[]): Promise<Category[]> {
  return ((await PortalService.ReorderCategories(ids)) ?? []).map(asCategory);
}

export async function startApp(id: string): Promise<AppView> {
  return asAppView(await PortalService.StartApp(id));
}

export async function setPinned(id: string, pinned: boolean): Promise<AppView> {
  return asAppView(await PortalService.SetPinned(id, pinned));
}

export async function stopApp(id: string): Promise<AppView> {
  return asAppView(await PortalService.StopApp(id));
}

export async function getLogs(id: string): Promise<LogEvent[]> {
  return ((await PortalService.GetLogs(id)) ?? []).map(asLogEvent);
}

export async function getConfigPath(): Promise<string> {
  return PortalService.GetConfigPath();
}

export async function pickFolder(): Promise<string | null> {
  const selected = await PortalService.PickFolder();
  if (!selected) return null;
  return selected;
}

export async function openAppUrl(url: string): Promise<void> {
  await PortalService.OpenAppURL(url);
}

export async function openConfigPath(): Promise<void> {
  await PortalService.OpenConfigPath();
}

export async function appVersion(): Promise<string> {
  return PortalService.AppVersion();
}

export function checkForUpdates(): void {
  void PortalService.CheckForUpdates();
}

export function skipUpdate(version: string): void {
  void PortalService.SkipUpdate(version);
}

export async function restartToUpdate(): Promise<void> {
  await PortalService.RestartToUpdate();
}

export function onAppStatus(handler: (app: AppView) => void): () => void {
  try {
    return Events.On("app-status", (event: { data: BoundAppView }) =>
      handler(asAppView(event.data)),
    );
  } catch {
    return () => {};
  }
}

export interface GatewayStatus {
  addr: string;
  port: number;
  preferredPort: number;
  running: boolean;
  error: string;
  note: string;
}

export const idleGateway = (): GatewayStatus => ({
  addr: "http://127.0.0.1:80",
  port: 80,
  preferredPort: 80,
  running: false,
  error: "",
  note: "",
});

export async function getGatewayStatus(): Promise<GatewayStatus> {
  return asGatewayStatus(await PortalService.GetGatewayStatus());
}

export async function startGateway(): Promise<GatewayStatus> {
  return asGatewayStatus(await PortalService.StartGateway());
}

export async function stopGateway(): Promise<GatewayStatus> {
  return asGatewayStatus(await PortalService.StopGateway());
}

export async function setGatewayPort(port: number): Promise<GatewayStatus> {
  return asGatewayStatus(await PortalService.SetGatewayPort(port));
}

export function onGatewayStatus(
  handler: (status: GatewayStatus) => void,
): () => void {
  try {
    return Events.On("gateway-status", (event: { data: GatewayStatus }) =>
      handler(asGatewayStatus(event.data)),
    );
  } catch {
    return () => {};
  }
}

function asGatewayStatus(status: BoundGatewayStatus | GatewayStatus): GatewayStatus {
  const preferredPort = Number(status.preferredPort) || 80;
  const port = Number(status.port) || preferredPort;
  return {
    addr: status.addr || `http://127.0.0.1:${port}`,
    port,
    preferredPort,
    running: Boolean(status.running),
    error: status.error ?? "",
    note: status.note ?? "",
  };
}

export function onAppLog(handler: (log: LogEvent) => void): () => void {
  try {
    return Events.On("app-log", (event: { data: BoundLogEvent }) =>
      handler(asLogEvent(event.data)),
    );
  } catch {
    return () => {};
  }
}

function asAppView(app: BoundAppView): AppView {
  return {
    id: app.id,
    name: app.name,
    description: app.description ?? "",
    hostname: app.hostname ?? "",
    categoryId: app.categoryId ?? "",
    folder: app.folder,
    command: app.command,
    portMode: app.portMode === "manual" ? "manual" : "auto",
    port: app.port,
    portEnv: app.portEnv ?? "",
    wakeOnRequest: app.wakeOnRequest,
    idleStopMin: app.idleStopMin ?? 0,
    pinned: Boolean(app.pinned),
    idleUntil: app.idleUntil ?? null,
    env: app.env ?? [],
    backends: (app.backends ?? []).map((backend, index) => ({
      name: backend.name ?? "",
      folder: backend.folder ?? "",
      command: backend.command ?? "",
      portMode: backend.portMode === "manual" ? "manual" : "auto",
      port: backend.port ?? null,
      portEnv: backend.portEnv ?? "",
      appPortEnv: backend.appPortEnv ?? "",
      env: backend.env ?? [],
      pid: app.backendPids?.[index] ?? null,
    })),
    backendPids: app.backendPids ?? [],
    status: (app.status || "stopped") as AppView["status"],
    url: app.url,
    favicon: app.favicon ?? null,
    pid: app.pid,
    error: app.error,
  };
}

function asCategory(entry: BoundCategoryEntry): Category {
  return {
    id: entry.id,
    name: entry.name ?? "",
    color: entry.color ?? "",
  };
}

function asLogEvent(event: BoundLogEvent): LogEvent {
  return {
    id: event.id,
    stream: event.stream,
    line: event.line,
  };
}
