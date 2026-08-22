export type PortMode = "auto" | "manual";
export type AppStatus = "stopped" | "idle" | "starting" | "running" | "error";

export const DEFAULT_PORT_ENV = "PORT";
export const DEFAULT_BACKEND_PORT_ENV = "BACKEND_PORT";

export interface EnvVar {
  name: string;
  value: string;
}

export interface BackendConfig {
  name: string;
  folder: string;
  command: string;
  portMode: PortMode;
  port: number | null;
  portEnv: string;
  appPortEnv: string;
  env: EnvVar[];
  pid?: number | null;
}

export interface AppView {
  id: string;
  name: string;
  description: string;
  hostname: string;
  folder: string;
  command: string;
  portMode: PortMode;
  port: number | null;
  portEnv: string;
  wakeOnRequest: boolean;
  idleStopMin: number;
  pinned: boolean;
  idleUntil: number | null;
  env: EnvVar[];
  backends: BackendConfig[];
  backendPids: number[];
  status: AppStatus;
  url: string | null;
  favicon: string | null;
  pid: number | null;
  error: string | null;
}

export interface AppInput {
  id?: string | null;
  name: string;
  description: string;
  hostname: string;
  folder: string;
  command: string;
  portMode: PortMode;
  port: number | null;
  portEnv: string;
  wakeOnRequest: boolean;
  idleStopMin: number;
  env: EnvVar[];
  backends: BackendConfig[];
}

export function resolvedPortEnv(name: string | null | undefined): string {
  const trimmed = name?.trim() ?? "";
  return trimmed || DEFAULT_PORT_ENV;
}

export function defaultAppPortEnv(index: number): string {
  return index <= 0
    ? DEFAULT_BACKEND_PORT_ENV
    : `${DEFAULT_BACKEND_PORT_ENV}_${index + 1}`;
}

export function resolvedAppPortEnv(
  name: string | null | undefined,
  index: number,
): string {
  const trimmed = name?.trim() ?? "";
  return trimmed || defaultAppPortEnv(index);
}

export interface LogEvent {
  id: string;
  stream: string;
  line: string;
}

const emptyEnvRow = (): EnvVar => ({ name: "", value: "" });

export const emptyBackend = (index = 0): BackendConfig => ({
  name: "",
  folder: "",
  command: "",
  portMode: "auto",
  port: null,
  portEnv: DEFAULT_PORT_ENV,
  appPortEnv: defaultAppPortEnv(index),
  env: [emptyEnvRow()],
});

export const emptyForm = (): AppInput => ({
  id: null,
  name: "",
  description: "",
  hostname: "",
  folder: "",
  command: "npm run dev",
  portMode: "auto",
  port: null,
  portEnv: DEFAULT_PORT_ENV,
  wakeOnRequest: true,
  idleStopMin: 15,
  env: [emptyEnvRow()],
  backends: [],
});

function copyEnv(rows: EnvVar[]): EnvVar[] {
  return rows.length > 0 ? rows.map((row) => ({ ...row })) : [emptyEnvRow()];
}

export const formFromApp = (app: AppView): AppInput => ({
  id: app.id,
  name: app.name,
  description: app.description,
  hostname: app.hostname,
  folder: app.folder,
  command: app.command,
  portMode: app.portMode,
  port: app.port,
  portEnv: resolvedPortEnv(app.portEnv),
  wakeOnRequest: app.wakeOnRequest,
  idleStopMin: app.idleStopMin,
  env: copyEnv(app.env),
  backends: (app.backends ?? []).map((backend, index) => ({
    name: backend.name ?? "",
    folder: backend.folder === app.folder ? "" : backend.folder,
    command: backend.command,
    portMode: backend.portMode === "manual" ? "manual" : "auto",
    port: backend.port,
    portEnv: resolvedPortEnv(backend.portEnv),
    appPortEnv: resolvedAppPortEnv(backend.appPortEnv, index),
    env: copyEnv(backend.env),
  })),
});

export { emptyEnvRow };
