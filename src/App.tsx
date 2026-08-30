import { FormEvent, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import {
  AppWindow,
  CirclePlay,
  Moon,
  Plus,
  Power,
  Search,
  Settings,
} from "lucide-react";
import { toast } from "sonner";
import {
  deleteApp,
  getConfigPath,
  getGatewayStatus,
  getLogs,
  idleGateway,
  isBrowserPreview,
  isWails,
  listApps,
  onAppLog,
  onAppStatus,
  onGatewayStatus,
  openAppUrl,
  openConfigPath,
  pickFolder,
  setGatewayPort,
  setPinned,
  startApp,
  startGateway,
  stopApp,
  stopGateway,
  upsertApp,
  type GatewayStatus,
} from "./api";
import { AppCard } from "@/components/app-card";
import { AppDetailsDialog } from "@/components/app-details-dialog";
import { AppFormPage } from "@/components/app-form-page";
import { AppShell } from "@/components/app-shell";
import { DeleteAppDialog } from "@/components/delete-app-dialog";
import { EmptyBoard } from "@/components/empty-board";
import { GatewayBar } from "@/components/gateway-bar";
import { LogsDialog } from "@/components/logs-dialog";
import { SettingsDialog } from "@/components/settings-dialog";
import { ThemeToggle } from "@/components/theme-toggle";
import { UpdateBanner } from "@/components/update-banner";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from "@/components/ui/input-group";
import { Toaster } from "@/components/ui/sonner";
import { asError } from "@/lib/error";
import { hostnameFieldError } from "@/lib/hostname";
import { navigate, useAppRoute } from "@/lib/route";
import { isOff, matchesFilter, type BoardFilter } from "@/lib/status";
import { promptVisible, useAppUpdater } from "@/lib/updater";
import { persistTheme, readTheme, type Theme } from "./theme";
import {
  AppInput,
  AppView,
  emptyForm,
  formFromApp,
  LogEvent,
  resolvedAppPortEnv,
  resolvedPortEnv,
} from "./types";

const FILTERS: { id: BoardFilter; label: string; icon: ReactNode }[] = [
  { id: "all", label: "すべて", icon: <AppWindow className="nav-icon size-4" /> },
  { id: "live", label: "稼働", icon: <CirclePlay className="nav-icon size-4" /> },
  { id: "idle", label: "待ち受け", icon: <Moon className="nav-icon size-4" /> },
  { id: "off", label: "停止", icon: <Power className="nav-icon size-4" /> },
];

export default function App() {
  const route = useAppRoute();
  const preview = route.preview || isBrowserPreview();
  const [apps, setApps] = useState<AppView[]>([]);
  const [logs, setLogs] = useState<Record<string, LogEvent[]>>({});
  const [logAppId, setLogAppId] = useState<string | null>(null);
  const [detailAppId, setDetailAppId] = useState<string | null>(null);
  const [configPath, setConfigPath] = useState("");
  const [ready, setReady] = useState(false);
  const [bootError, setBootError] = useState<string | null>(null);
  const [form, setForm] = useState<AppInput>(emptyForm());
  const [busyId, setBusyId] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);
  const [confirmId, setConfirmId] = useState<string | null>(null);
  const [pickingFolder, setPickingFolder] = useState(false);
  const [theme, setTheme] = useState<Theme>(() => readTheme());
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState<BoardFilter>("all");
  const [gateway, setGateway] = useState<GatewayStatus>(() => idleGateway());
  const [gatewayBusy, setGatewayBusy] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [savingGatewayPort, setSavingGatewayPort] = useState(false);
  const updater = useAppUpdater(preview);
  const formSeed = useRef<string | null>(null);
  const updateWaiting = promptVisible(updater);

  useEffect(() => {
    persistTheme(theme);
  }, [theme]);

  useEffect(() => {
    let cancelled = false;

    if (preview) {
      setApps(previewApps());
      setLogs(previewLogs());
      setConfigPath(
        "/Users/you/Library/Application Support/devportal/apps.yml",
      );
      setGateway({
        addr: "http://127.0.0.1:80",
        port: 80,
        preferredPort: 80,
        running: true,
        error: "",
        note: "",
      });
      setReady(true);
      setBootError(null);
      return;
    }

    (async () => {
      try {
        const [nextApps, path, nextGateway] = await Promise.all([
          listApps(),
          getConfigPath(),
          getGatewayStatus(),
        ]);
        if (cancelled) return;
        setApps(nextApps);
        setConfigPath(path);
        setGateway(nextGateway);
      } catch (error) {
        if (!cancelled) {
          setBootError(
            isWails()
              ? asError(error)
              : "この画面は Wails アプリとして起動してください。`bun run wails:dev`",
          );
        }
      } finally {
        if (!cancelled) setReady(true);
      }
    })();

    const unlistenStatus = onAppStatus((app) => {
      setApps((current) => mergeApp(current, app));
    });
    const unlistenLog = onAppLog((event) => {
      setLogs((current) => {
        const lines = [...(current[event.id] ?? []), event];
        return { ...current, [event.id]: lines.slice(-200) };
      });
    });
    const unlistenGateway = onGatewayStatus((status) => {
      setGateway(status);
    });

    return () => {
      cancelled = true;
      unlistenStatus();
      unlistenLog();
      unlistenGateway();
    };
  }, [preview]);

  const counts = useMemo(() => {
    const live = apps.filter(
      (app) => app.status === "running" || app.status === "starting",
    ).length;
    return {
      all: apps.length,
      live,
      idle: apps.filter((app) => app.status === "idle").length,
      off: apps.filter((app) => app.status === "stopped" || app.status === "error").length,
    };
  }, [apps]);

  const visibleApps = useMemo(() => {
    const needle = query.trim().toLowerCase();
    return apps.filter((app) => {
      if (!matchesFilter(app.status, filter)) return false;
      if (!needle) return true;
      return [
        app.name,
        app.description,
        app.hostname,
        app.folder,
        app.command,
        ...(app.backends ?? []).flatMap((backend) => [
          backend.folder,
          backend.command,
          backend.name,
        ]),
      ].some((value) => value.toLowerCase().includes(needle));
    });
  }, [apps, filter, query]);

  const confirmApp = apps.find((app) => app.id === confirmId) ?? null;
  const logApp = apps.find((app) => app.id === logAppId) ?? null;
  const detailApp = apps.find((app) => app.id === detailAppId) ?? null;
  const filteredEmpty = apps.length > 0 && visibleApps.length === 0;
  const editApp =
    route.page === "edit"
      ? (apps.find((app) => app.id === route.id) ?? null)
      : null;

  useEffect(() => {
    if (route.page === "board") {
      formSeed.current = null;
      return;
    }
    if (route.page === "new") {
      if (formSeed.current !== "new") {
        setForm(emptyForm());
        setFormError(null);
        formSeed.current = "new";
      }
      return;
    }
    const key = `edit:${route.id}`;
    if (formSeed.current === key) return;
    const app = apps.find((item) => item.id === route.id);
    if (!app) return;
    setForm(formFromApp(app));
    setFormError(null);
    formSeed.current = key;
  }, [route, apps]);

  function goBoard() {
    navigate({ page: "board", preview: route.preview });
  }

  function selectFilter(id: BoardFilter) {
    setFilter(id);
    if (route.page !== "board") {
      navigate({ page: "board", preview: route.preview });
    }
  }

  function openCreate() {
    setForm(emptyForm());
    setFormError(null);
    formSeed.current = "new";
    navigate({ page: "new", preview: route.preview });
  }

  function openEdit(app: AppView) {
    setForm(formFromApp(app));
    setFormError(null);
    formSeed.current = `edit:${app.id}`;
    navigate({ page: "edit", id: app.id, preview: route.preview });
  }

  async function chooseFolder(target: "app" | number = "app") {
    if (pickingFolder) return;
    setPickingFolder(true);
    setFormError(null);
    try {
      const folder = await pickFolder();
      if (folder) {
        setForm((current) => {
          if (target !== "app") {
            return {
              ...current,
              backends: current.backends.map((backend, index) =>
                index === target ? { ...backend, folder } : backend,
              ),
            };
          }
          return {
            ...current,
            folder,
            name:
              current.name ||
              folder.split(/[\\/]/).filter(Boolean).pop() ||
              "",
          };
        });
      }
    } catch (error) {
      setFormError(asError(error));
    } finally {
      setPickingFolder(false);
    }
  }

  async function saveForm(event: FormEvent) {
    event.preventDefault();
    setSaving(true);
    setFormError(null);
    const editing = Boolean(form.id);
    const hostnameError = hostnameFieldError(form.hostname);
    if (hostnameError) {
      setFormError(hostnameError);
      setSaving(false);
      return;
    }
    {
      const listenEnv = resolvedPortEnv(form.portEnv);
      const seen = new Set<string>([listenEnv]);
      let clash: string | null = null;
      form.backends.forEach((backend, index) => {
        if (clash || !backend.command.trim()) return;
        const appEnv = resolvedAppPortEnv(backend.appPortEnv, index);
        if (seen.has(appEnv)) {
          clash = `アプリのポートとバックエンドポートに同じ環境変数名は使えません: ${appEnv}`;
          return;
        }
        seen.add(appEnv);
      });
      if (clash) {
        setFormError(clash);
        setSaving(false);
        return;
      }
    }
    try {
      if (preview) {
        const id = form.id || `preview-${Date.now()}`;
        const name =
          form.name ||
          form.folder.split(/[\\/]/).filter(Boolean).pop() ||
          "untitled";
        const saved: AppView = {
          id,
          name,
          description: form.description,
          hostname: form.hostname || name.toLowerCase().replace(/[^a-z0-9-]+/g, "-"),
          folder: form.folder,
          command: form.command,
          portMode: form.portMode,
          port: form.port,
          portEnv: form.portEnv,
          wakeOnRequest: form.wakeOnRequest,
          idleStopMin: form.idleStopMin,
          pinned: false,
          idleUntil: null,
          env: form.env.filter((row) => row.name),
          backends: form.backends
            .filter((backend) => backend.command.trim())
            .map((backend) => ({
              ...backend,
              env: backend.env.filter((row) => row.name),
            })),
          backendPids: [],
          status: "stopped",
          url: null,
          favicon:
            (form.id
              ? apps.find((item) => item.id === form.id)?.favicon
              : null) ?? null,
          pid: null,
          error: null,
        };
        setApps((current) => mergeApp(current, saved));
        goBoard();
        toast.success(
          editing ? `${saved.name} を更新しました` : `${saved.name} を登録しました`,
        );
        return;
      }
      const saved = await upsertApp(form);
      setApps((current) => mergeApp(current, saved));
      goBoard();
      toast.success(
        editing ? `${saved.name} を更新しました` : `${saved.name} を登録しました`,
      );
    } catch (error) {
      setFormError(asError(error));
    } finally {
      setSaving(false);
    }
  }

  async function toggle(app: AppView) {
    setBusyId(app.id);
    try {
      if (preview) {
        const starting = isOff(app.status);
        const next: AppView = starting
          ? {
              ...app,
              status: "running",
              pid: app.pid ?? 40000,
              backendPids: app.backends.map((_, index) => 50000 + index),
              error: null,
              url: app.url ?? `http://127.0.0.1:${app.port ?? 5173}`,
            }
          : {
              ...app,
              status: app.hostname ? "idle" : "stopped",
              pid: null,
              backendPids: [],
              pinned: false,
              idleUntil: null,
            };
        setApps((current) => mergeApp(current, next));
        return;
      }
      const next = isOff(app.status) ? await startApp(app.id) : await stopApp(app.id);
      setApps((current) => mergeApp(current, next));
    } catch (error) {
      toast.error(asError(error));
    } finally {
      setBusyId(null);
    }
  }

  async function remove(app: AppView) {
    setBusyId(app.id);
    try {
      if (preview) {
        setApps((current) => current.filter((item) => item.id !== app.id));
        setConfirmId(null);
        toast.success(`${app.name} を削除しました`);
        return;
      }
      await deleteApp(app.id);
      setApps((current) => current.filter((item) => item.id !== app.id));
      setConfirmId(null);
      toast.success(`${app.name} を削除しました`);
    } catch (error) {
      toast.error(asError(error));
    } finally {
      setBusyId(null);
    }
  }

  async function revealLogs(app: AppView) {
    setLogAppId(app.id);
    if (logs[app.id]) return;
    try {
      const history = await getLogs(app.id);
      setLogs((current) => ({ ...current, [app.id]: history }));
    } catch {
      // keep empty
    }
  }

  async function pinApp(app: AppView, pinned: boolean) {
    if (preview) {
      const next: AppView = {
        ...app,
        pinned,
        idleUntil: pinned
          ? null
          : Math.floor(Date.now() / 1000) + Math.max(app.idleStopMin, 1) * 60,
      };
      setApps((current) => mergeApp(current, next));
      toast.success(
        next.pinned
          ? `${next.name} の Keep alive をオンにしました`
          : `${next.name} の Keep alive をオフにしました`,
      );
      return;
    }
    try {
      const next = await setPinned(app.id, pinned);
      setApps((current) => mergeApp(current, next));
      toast.success(
        next.pinned
          ? `${next.name} の Keep alive をオンにしました`
          : `${next.name} の Keep alive をオフにしました`,
      );
    } catch (error) {
      toast.error(asError(error));
    }
  }

  async function openInBrowser(app: AppView) {
    if (!app.url) return;
    if (!isWails() || preview) {
      window.open(app.url, "_blank", "noopener,noreferrer");
      return;
    }
    try {
      await openAppUrl(app.url);
    } catch (error) {
      toast.error(asError(error));
    }
  }

  async function revealConfig() {
    if (!configPath) return;
    if (preview) {
      return;
    }
    try {
      await openConfigPath();
    } catch (error) {
      toast.error(asError(error) || "設定フォルダを開けませんでした");
    }
  }

  async function saveGatewayPort(port: number) {
    if (savingGatewayPort) return;
    if (preview) {
      setGateway((current) => ({
        ...current,
        preferredPort: port,
        port,
        addr: `http://127.0.0.1:${port}`,
        note: "",
        error: "",
      }));
      toast.success("リバースプロキシのポートを保存しました");
      return;
    }
    setSavingGatewayPort(true);
    try {
      const status = await setGatewayPort(port);
      setGateway(status);
      toast.success(
        status.running && status.port !== status.preferredPort
          ? `ポート ${status.preferredPort} は使えなかったため、:${status.port} で待ち受けています`
          : "リバースプロキシのポートを保存しました",
      );
    } catch (error) {
      toast.error(asError(error) || "ポートを保存できませんでした");
      try {
        setGateway(await getGatewayStatus());
      } catch {
        // keep current
      }
    } finally {
      setSavingGatewayPort(false);
    }
  }

  async function toggleGateway(nextRunning: boolean) {
    if (gatewayBusy) return;
    if (preview) {
      const port = gateway.preferredPort || 80;
      setGateway({
        addr: `http://127.0.0.1:${port}`,
        port,
        preferredPort: port,
        running: nextRunning,
        error: "",
        note: "",
      });
      return;
    }
    setGatewayBusy(true);
    try {
      const status = nextRunning ? await startGateway() : await stopGateway();
      setGateway(status);
      toast.success(
        status.running
          ? "リバースプロキシを開始しました"
          : "リバースプロキシを終了しました",
      );
    } catch (error) {
      toast.error(asError(error));
      try {
        setGateway(await getGatewayStatus());
      } catch {
        // keep current
      }
    } finally {
      setGatewayBusy(false);
    }
  }

  return (
    <AppShell
      nativeChrome={isWails() && !preview}
      sidebar={
        <>
          <div className="window-drag px-2 pb-4">
            <p className="text-[15px] font-semibold tracking-tight">DevPortal</p>
            <p className="mt-0.5 text-[11px] text-muted-foreground">
              ローカルランチャー
            </p>
          </div>

          <nav className="flex flex-col gap-0.5" aria-label="状態で絞り込み">
            {FILTERS.map((item) => (
              <button
                key={item.id}
                type="button"
                className="nav-item"
                data-active={filter === item.id && route.page === "board"}
                onClick={() => selectFilter(item.id)}
              >
                {item.icon}
                <span className="flex-1">{item.label}</span>
                <span className="tabular-nums text-[11px] text-muted-foreground">
                  {counts[item.id]}
                </span>
              </button>
            ))}
          </nav>

          <GatewayBar
            status={gateway}
            busy={gatewayBusy}
            onToggle={(running) => void toggleGateway(running)}
          />

          <div className="mt-3 flex items-center justify-between gap-2 px-1">
            <ThemeToggle theme={theme} onChange={setTheme} />
            <Button
              type="button"
              variant="ghost"
              size="icon-xs"
              className="relative"
              aria-label="設定"
              aria-haspopup="dialog"
              aria-expanded={settingsOpen}
              onClick={() => setSettingsOpen(true)}
            >
              <Settings />
              {updateWaiting ? (
                <span className="absolute top-0.5 right-0.5 size-1.5 rounded-full bg-primary ring-2 ring-[var(--sidebar)]" />
              ) : null}
            </Button>
          </div>
        </>
      }
      overlays={
        <>
          <SettingsDialog
            open={settingsOpen}
            configPath={configPath}
            gatewayPort={gateway.preferredPort || 80}
            gatewayRunning={gateway.running}
            savingPort={savingGatewayPort}
            updater={updater}
            onOpenChange={setSettingsOpen}
            onRevealConfig={() => void revealConfig()}
            onSaveGatewayPort={(port) => void saveGatewayPort(port)}
          />
          {route.page === "board" && (
            <>
              <AppDetailsDialog
                app={detailApp}
                busy={busyId === detailApp?.id}
                onOpenChange={(open) => {
                  if (!open) setDetailAppId(null);
                }}
                onToggle={() => {
                  if (detailApp) void toggle(detailApp);
                }}
                onEdit={() => {
                  if (!detailApp) return;
                  setDetailAppId(null);
                  openEdit(detailApp);
                }}
                onDelete={() => {
                  if (!detailApp) return;
                  setDetailAppId(null);
                  setConfirmId(detailApp.id);
                }}
                onOpenUrl={() => {
                  if (detailApp) void openInBrowser(detailApp);
                }}
                onOpenLogs={() => {
                  if (!detailApp) return;
                  const app = detailApp;
                  setDetailAppId(null);
                  void revealLogs(app);
                }}
                onSetPinned={(pinned) => {
                  if (detailApp) void pinApp(detailApp, pinned);
                }}
              />
              <LogsDialog
                app={logApp}
                logs={logApp ? (logs[logApp.id] ?? []) : []}
                onOpenChange={(open) => {
                  if (!open) setLogAppId(null);
                }}
              />
              <DeleteAppDialog
                app={confirmApp}
                busy={busyId === confirmApp?.id}
                onOpenChange={(open) => {
                  if (!open) setConfirmId(null);
                }}
                onConfirm={() => {
                  if (confirmApp) void remove(confirmApp);
                }}
              />
            </>
          )}
          <Toaster theme={theme} position="bottom-right" />
        </>
      }
    >
      {route.page === "board" ? (
        <>
          <header className="toolbar">
            <InputGroup className="h-8 min-w-0 max-w-md flex-1 bg-background/70">
              <InputGroupAddon>
                <Search />
              </InputGroupAddon>
              <InputGroupInput
                value={query}
                placeholder="名前・フォルダ・コマンドで探す"
                onChange={(event) => setQuery(event.target.value)}
              />
            </InputGroup>
            <div className="ml-auto">
              <Button size="sm" onClick={openCreate}>
                <Plus data-icon="inline-start" />
                アプリを登録
              </Button>
            </div>
          </header>

          <main className="stage-body px-5 py-5">
            {bootError && (
              <Alert variant="destructive" className="mb-4">
                <AlertTitle>起動できません</AlertTitle>
                <AlertDescription>{bootError}</AlertDescription>
              </Alert>
            )}

            {updateWaiting ? <UpdateBanner updater={updater} /> : null}

            {!ready ? (
              <div className="mx-auto flex max-w-3xl flex-col gap-2">
                <div className="surface h-20 animate-pulse rounded-[14px]" />
                <div className="surface h-20 animate-pulse rounded-[14px]" />
                <div className="surface h-20 animate-pulse rounded-[14px]" />
              </div>
            ) : apps.length === 0 && !bootError ? (
              <EmptyBoard
                filtered={false}
                onCreate={openCreate}
                onResetFilter={() => {
                  setQuery("");
                  setFilter("all");
                }}
              />
            ) : filteredEmpty ? (
              <EmptyBoard
                filtered
                onCreate={openCreate}
                onResetFilter={() => {
                  setQuery("");
                  setFilter("all");
                }}
              />
            ) : (
              <ul className="mx-auto flex max-w-3xl list-none flex-col gap-2 p-0">
                {visibleApps.map((app, index) => (
                  <li key={app.id}>
                    <AppCard
                      app={app}
                      index={index}
                      busy={busyId === app.id}
                      onToggle={() => toggle(app)}
                      onEdit={() => openEdit(app)}
                      onDetails={() => setDetailAppId(app.id)}
                      onOpenUrl={() => openInBrowser(app)}
                      onSetPinned={(pinned) => void pinApp(app, pinned)}
                    />
                  </li>
                ))}
              </ul>
            )}
          </main>
        </>
      ) : (
        <AppFormPage
          mode={route.page === "edit" ? "edit" : "create"}
          form={form}
          saving={saving}
          error={formError}
          pickingFolder={pickingFolder}
          loading={route.page === "edit" && !ready}
          missing={route.page === "edit" && ready && !editApp}
          onBack={goBoard}
          onFormChange={(next) => {
            setForm(next);
            if (formError) setFormError(null);
          }}
          onPickFolder={chooseFolder}
          onSubmit={saveForm}
        />
      )}
    </AppShell>
  );
}

function previewBase(partial: Partial<AppView> & Pick<AppView, "id" | "name" | "hostname">): AppView {
  const hostname = partial.hostname;
  return {
    description: "",
    folder: `/Users/you/repos/${hostname}`,
    command: "npm run dev",
    portMode: "auto",
    port: 5173,
    portEnv: "PORT",
    wakeOnRequest: true,
    idleStopMin: 15,
    pinned: false,
    idleUntil: null,
    env: [],
    backends: [],
    backendPids: [],
    status: "idle",
    url: `http://${hostname}.devportal.localhost`,
    favicon: null,
    pid: null,
    error: null,
    ...partial,
  };
}

function previewApps(): AppView[] {
  return [
    previewBase({
      id: "markdown-preview",
      name: "markdown_preview",
      description: "Markdown をブラウザでリアルタイムプレビューする",
      hostname: "markdown-preview",
      command: "npm run dev",
      port: 5173,
      favicon: previewFavicon("#34c759", "M"),
    }),
    previewBase({
      id: "storybook",
      name: "Storybook",
      description: "UI コンポーネントをカタログとして確認する",
      hostname: "storybook",
      folder: "/Users/you/repos/webapp",
      command: "npm run storybook",
      port: 6006,
      favicon: previewFavicon("#ff4785", "S"),
    }),
    previewBase({
      id: "mailhog",
      name: "mailhog",
      hostname: "mailhog",
      folder: "/Users/you/tools/mailhog",
      command: "mailhog",
      portMode: "manual",
      port: 8025,
      favicon: previewFavicon("#af52de", "H"),
    }),
    previewBase({
      id: "invoice-maker",
      name: "Invoice Maker",
      description: "請求書の下書きを作って PDF で確認する社内ツール",
      hostname: "invoice-maker",
      command: "bun run dev",
      port: 3000,
      favicon: previewFavicon("#007aff", "I"),
    }),
    previewBase({
      id: "swagger-ui",
      name: "swagger-ui",
      hostname: "swagger-ui",
      folder: "/Users/you/repos/api",
      command: "npx swagger-ui-watcher",
      port: 8080,
      favicon: previewFavicon("#49cc90", "A"),
    }),
    previewBase({
      id: "running",
      name: "社内Wiki",
      hostname: "wiki",
      folder: "/Users/you/repos/wiki",
      command: "bun run dev",
      portMode: "manual",
      port: 3333,
      idleUntil: Math.floor(Date.now() / 1000) + 4 * 60 + 49,
      env: [{ name: "HOST", value: "127.0.0.1" }],
      backends: [
        {
          name: "api",
          folder: "/Users/you/repos/wiki",
          command: "go run ./cmd/api",
          portMode: "auto",
          port: 8088,
          portEnv: "PORT",
          appPortEnv: "VITE_API_PORT",
          env: [{ name: "DATABASE_URL", value: "postgres://127.0.0.1/wiki" }],
          pid: 48240,
        },
        {
          name: "worker",
          folder: "/Users/you/repos/wiki",
          command: "go run ./cmd/worker",
          portMode: "auto",
          port: 8090,
          portEnv: "PORT",
          appPortEnv: "WORKER_PORT",
          env: [],
          pid: 48255,
        },
      ],
      backendPids: [48240, 48255],
      status: "running",
      favicon: previewFavicon("#5ac8fa", "W"),
      pid: 48211,
    }),
  ];
}

function previewLogs(): Record<string, LogEvent[]> {
  return {
    running: [
      { id: "running", stream: "stdout", line: "$ bun run dev" },
      { id: "running", stream: "backend:api", line: "$ go run ./cmd/api" },
      { id: "running", stream: "backend:api", line: "api listening on 127.0.0.1:8088" },
      { id: "running", stream: "backend:worker", line: "$ go run ./cmd/worker" },
      { id: "running", stream: "backend:worker", line: "worker listening on 127.0.0.1:8090" },
      { id: "running", stream: "stdout", line: "started wiki on 127.0.0.1:3333" },
      { id: "running", stream: "stdout", line: "GET / 200  12ms" },
      { id: "running", stream: "stdout", line: "GET /pages 200  4ms" },
      { id: "running", stream: "stderr", line: "warn: sqlite busy, retrying" },
      { id: "running", stream: "stdout", line: "GET /pages/home 200  3ms" },
    ],
  };
}

function mergeApp(list: AppView[], next: AppView): AppView[] {
  const index = list.findIndex((item) => item.id === next.id);
  if (index === -1) return [...list, next];
  return list.map((item) => (item.id === next.id ? next : item));
}

function previewFavicon(color: string, glyph: string): string {
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32"><rect width="32" height="32" rx="8" fill="${color}"/><text x="16" y="21" text-anchor="middle" font-size="13" font-family="-apple-system,system-ui,sans-serif" fill="#fff">${glyph}</text></svg>`;
  return `data:image/svg+xml;base64,${btoa(svg)}`;
}
