import {
  FormEvent,
  useEffect,
  useMemo,
  useRef,
  useState,
  type DragEvent,
  type KeyboardEvent,
  type ReactNode,
} from "react";
import {
  AppWindow,
  CirclePlay,
  FolderCog,
  Inbox,
  Moon,
  Plus,
  Power,
  Search,
  Settings,
} from "lucide-react";
import { toast } from "sonner";
import {
  deleteApp,
  deleteCategory,
  getConfigPath,
  getGatewayStatus,
  getLogs,
  idleGateway,
  isBrowserPreview,
  isWails,
  listApps,
  listCategories,
  onAppLog,
  onAppStatus,
  onGatewayStatus,
  openAppUrl,
  openConfigPath,
  pickFolder,
  reorderApps,
  reorderCategories,
  setGatewayPort,
  setPinned,
  startApp,
  startGateway,
  stopApp,
  stopGateway,
  upsertApp,
  upsertCategory,
  type GatewayStatus,
} from "./api";
import { AppCard } from "@/components/app-card";
import { AppDetailsDialog } from "@/components/app-details-dialog";
import { AppFormPage } from "@/components/app-form-page";
import { AppShell } from "@/components/app-shell";
import { CategoriesPage } from "@/components/categories-page";
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
import { moveById } from "@/lib/reorder";
import { navigate, useAppRoute } from "@/lib/route";
import { isOff, matchesFilter, type BoardFilter } from "@/lib/status";
import { promptVisible, useAppUpdater } from "@/lib/updater";
import { cn } from "@/lib/utils";
import { persistTheme, readTheme, type Theme } from "./theme";
import {
  AppInput,
  AppView,
  Category,
  duplicateForm,
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
  const [categories, setCategories] = useState<Category[]>([]);
  const [sidebarDropId, setSidebarDropId] = useState<string | null>(null);
  const [selectedCategory, setSelectedCategory] = useState<string | null>(
    null,
  );
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
  const [dragId, setDragId] = useState<string | null>(null);
  const [dropHint, setDropHint] = useState<{
    id: string;
    after: boolean;
  } | null>(null);
  const [gateway, setGateway] = useState<GatewayStatus>(() => idleGateway());
  const [gatewayBusy, setGatewayBusy] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [savingGatewayPort, setSavingGatewayPort] = useState(false);
  const updater = useAppUpdater(preview);
  const formSeed = useRef<string | null>(null);
  const pendingCategoryScroll = useRef<string | null>(null);
  const updateWaiting = promptVisible(updater);

  useEffect(() => {
    persistTheme(theme);
  }, [theme]);

  useEffect(() => {
    let cancelled = false;

    if (preview) {
      setApps(previewApps());
      setCategories(previewCategories());
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
        const [nextApps, path, nextGateway, nextCategories] =
          await Promise.all([
            listApps(),
            getConfigPath(),
            getGatewayStatus(),
            listCategories(),
          ]);
        if (cancelled) return;
        setApps(nextApps);
        setConfigPath(path);
        setGateway(nextGateway);
        setCategories(nextCategories);
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

  const categoriesById = useMemo(
    () => new Map(categories.map((cat) => [cat.id, cat])),
    [categories],
  );

  const categoryCounts = useMemo(() => {
    const counts: Record<string, number> = {};
    for (const app of apps) {
      const key = categoriesById.has(app.categoryId) ? app.categoryId : "";
      counts[key] = (counts[key] ?? 0) + 1;
    }
    return counts;
  }, [apps, categoriesById]);

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
        categoriesById.get(app.categoryId)?.name ?? "",
        ...(app.backends ?? []).flatMap((backend) => [
          backend.folder,
          backend.command,
          backend.name,
        ]),
      ].some((value) => value.toLowerCase().includes(needle));
    });
  }, [apps, filter, query, categoriesById]);

  const groupedApps = useMemo(() => {
    if (categories.length === 0) {
      return [{ id: "", name: "", apps: visibleApps }];
    }
    const groups = categories.map((cat) => ({
      id: cat.id,
      name: cat.name,
      apps: [] as AppView[],
    }));
    const byId = new Map(groups.map((group) => [group.id, group]));
    const uncategorized = { id: "", name: "未分類", apps: [] as AppView[] };
    for (const app of visibleApps) {
      (byId.get(app.categoryId) ?? uncategorized).apps.push(app);
    }
    return [
      ...groups.filter((group) => group.apps.length > 0),
      ...(uncategorized.apps.length > 0 ? [uncategorized] : []),
    ];
  }, [visibleApps, categories]);

  const orderedApps = useMemo(
    () => groupedApps.flatMap((group) => group.apps),
    [groupedApps],
  );

  const confirmApp = apps.find((app) => app.id === confirmId) ?? null;
  const logApp = apps.find((app) => app.id === logAppId) ?? null;
  const detailApp = apps.find((app) => app.id === detailAppId) ?? null;
  const filteredEmpty = apps.length > 0 && visibleApps.length === 0;
  const editApp =
    route.page === "edit"
      ? (apps.find((app) => app.id === route.id) ?? null)
      : null;

  useEffect(() => {
    if (route.page === "board" || route.page === "categories") {
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

  function scrollToCategory(id: string) {
    if (route.page !== "board") {
      pendingCategoryScroll.current = id;
      navigate({ page: "board", preview: route.preview });
      return;
    }
    scrollCategoryIntoView(id);
  }

  function selectCategory(id: string) {
    if (selectedCategory === id) {
      setSelectedCategory(null);
      return;
    }
    setSelectedCategory(id);
    scrollToCategory(id);
  }

  useEffect(() => {
    if (
      route.page !== "board" ||
      !ready ||
      pendingCategoryScroll.current === null
    ) {
      return;
    }
    const target = pendingCategoryScroll.current;
    pendingCategoryScroll.current = null;
    scrollCategoryIntoView(target);
  }, [route.page, ready]);

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

  function openDuplicate(app: AppView) {
    setForm(duplicateForm(app, apps));
    setFormError(null);
    formSeed.current = "new";
    navigate({ page: "new", preview: route.preview });
    toast.info(
      `${app.name} の内容を複製しました。ドメインと環境変数を調整して保存してください。`,
    );
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
          categoryId: form.categoryId,
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

  function clearDrag() {
    setDragId(null);
    setDropHint(null);
    setSidebarDropId(null);
  }

  async function commitOrder(next: AppView[]) {
    const unchanged =
      next.length === apps.length &&
      next.every((app, index) => app.id === apps[index].id);
    if (unchanged) return;
    setApps(next);
    if (preview) return;
    try {
      const views = await reorderApps(next.map((app) => app.id));
      setApps((current) => {
        const byId = new Map(current.map((app) => [app.id, app]));
        const known = new Set(views.map((view) => view.id));
        return [
          ...views.map((view) => byId.get(view.id) ?? view),
          ...current.filter((app) => !known.has(app.id)),
        ];
      });
    } catch (error) {
      toast.error(asError(error));
      try {
        setApps(await listApps());
      } catch {
        // keep current
      }
    }
  }

  function dropOn(targetId: string, after: boolean) {
    const id = dragId;
    clearDrag();
    if (!id || id === targetId) return;
    const dragged = apps.find((item) => item.id === id);
    const target = apps.find((item) => item.id === targetId);
    if (!dragged || !target) return;
    let next = apps;
    if (dragged.categoryId !== target.categoryId) {
      next = apps.map((item) =>
        item.id === id ? { ...item, categoryId: target.categoryId } : item,
      );
      setApps(next);
      toast.success(
        `${dragged.name} を ${categoriesById.get(target.categoryId)?.name ?? "未分類"} に移動しました`,
      );
      void persistCategory(dragged, target.categoryId);
    }
    void commitOrder(moveById(next, id, targetId, after));
  }

  async function persistCategory(app: AppView, categoryId: string) {
    if (preview) return;
    try {
      const saved = await upsertApp({ ...formFromApp(app), categoryId });
      setApps((current) => mergeApp(current, saved));
    } catch (error) {
      toast.error(asError(error));
      try {
        setApps(await listApps());
      } catch {
        // keep current
      }
    }
  }

  async function assignCategory(appId: string, categoryId: string) {
    const app = apps.find((item) => item.id === appId);
    if (!app || app.categoryId === categoryId) return;
    setApps((current) =>
      current.map((item) =>
        item.id === appId ? { ...item, categoryId } : item,
      ),
    );
    toast.success(
      `${app.name} を ${categoriesById.get(categoryId)?.name ?? "未分類"} に移動しました`,
    );
    await persistCategory(app, categoryId);
  }

  async function createCategory(name: string): Promise<boolean> {
    if (preview) {
      setCategories((current) => [
        ...current,
        { id: `cat-${Date.now()}`, name, color: "" },
      ]);
      return true;
    }
    try {
      const cat = await upsertCategory(null, name, "");
      setCategories((current) => [...current, cat]);
      return true;
    } catch (error) {
      toast.error(asError(error));
      return false;
    }
  }

  async function renameCategory(id: string, name: string): Promise<boolean> {
    const previous = categories;
    setCategories((current) =>
      current.map((cat) => (cat.id === id ? { ...cat, name } : cat)),
    );
    if (preview) return true;
    const color =
      categories.find((cat) => cat.id === id)?.color ?? "";
    try {
      const saved = await upsertCategory(id, name, color);
      setCategories((current) =>
        current.map((cat) => (cat.id === id ? saved : cat)),
      );
      return true;
    } catch (error) {
      toast.error(asError(error));
      setCategories(previous);
      return false;
    }
  }

  async function setCategoryColor(
    id: string,
    color: string,
  ): Promise<boolean> {
    const cat = categories.find((item) => item.id === id);
    if (!cat || cat.color === color) return true;
    const previous = categories;
    setCategories((current) =>
      current.map((item) => (item.id === id ? { ...item, color } : item)),
    );
    if (preview) return true;
    try {
      const saved = await upsertCategory(id, cat.name, color);
      setCategories((current) =>
        current.map((item) => (item.id === id ? saved : item)),
      );
      return true;
    } catch (error) {
      toast.error(asError(error));
      setCategories(previous);
      return false;
    }
  }

  async function removeCategory(id: string): Promise<void> {
    const previousCategories = categories;
    const previousApps = apps;
    setCategories((current) => current.filter((cat) => cat.id !== id));
    setApps((current) =>
      current.map((app) =>
        app.categoryId === id ? { ...app, categoryId: "" } : app,
      ),
    );
    if (preview) {
      toast.success("カテゴリを削除しました");
      return;
    }
    try {
      await deleteCategory(id);
      toast.success("カテゴリを削除しました");
    } catch (error) {
      toast.error(asError(error));
      setCategories(previousCategories);
      setApps(previousApps);
    }
  }

  async function commitCategoryOrder(next: Category[]) {
    const unchanged =
      next.length === categories.length &&
      next.every((cat, index) => cat.id === categories[index].id);
    if (unchanged) return;
    setCategories(next);
    if (preview) return;
    try {
      setCategories(await reorderCategories(next.map((cat) => cat.id)));
    } catch (error) {
      toast.error(asError(error));
      try {
        setCategories(await listCategories());
      } catch {
        // keep current
      }
    }
  }

  function reorderKey(
    app: AppView,
    event: KeyboardEvent<HTMLButtonElement>,
  ) {
    const backward = event.key === "ArrowUp" || event.key === "ArrowLeft";
    const forward = event.key === "ArrowDown" || event.key === "ArrowRight";
    if (!backward && !forward) return;
    event.preventDefault();
    const index = orderedApps.findIndex((item) => item.id === app.id);
    const neighbor = backward
      ? orderedApps[index - 1]
      : orderedApps[index + 1];
    if (!neighbor) return;
    void commitOrder(moveById(apps, app.id, neighbor.id, forward));
  }

  function listDragOver(event: DragEvent<HTMLUListElement>) {
    event.preventDefault();
    if (!dragId) return;
    event.dataTransfer.dropEffect = "move";
    const hint = dropTargetAt(
      event.currentTarget,
      event.clientX,
      event.clientY,
    );
    const next = hint && hint.id !== dragId ? hint : null;
    setDropHint((current) =>
      current?.id === next?.id && current?.after === next?.after
        ? current
        : next,
    );
  }

  function listDrop(event: DragEvent<HTMLUListElement>) {
    event.preventDefault();
    if (!dragId) return;
    const hint = dropTargetAt(
      event.currentTarget,
      event.clientX,
      event.clientY,
    );
    if (hint) {
      dropOn(hint.id, hint.after);
    } else {
      clearDrag();
    }
  }

  function listDragLeave(event: DragEvent<HTMLUListElement>) {
    const related = event.relatedTarget as Node | null;
    if (!related || !event.currentTarget.contains(related)) {
      setDropHint(null);
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

          <div className="mt-4">
            <div className="flex items-center justify-between px-2 pb-1">
              <p className="text-[11px] font-medium text-muted-foreground">
                カテゴリ
              </p>
              <button
                type="button"
                className="inline-flex size-5 items-center justify-center rounded-md text-muted-foreground/70 outline-none hover:bg-sidebar-accent hover:text-muted-foreground focus-visible:ring-2 focus-visible:ring-ring/60"
                aria-label="カテゴリを管理"
                aria-current={route.page === "categories" ? "page" : undefined}
                onClick={() =>
                  navigate({ page: "categories", preview: route.preview })
                }
              >
                <FolderCog className="size-3.5" />
              </button>
            </div>
            {categories.length > 0 ? (
              <nav
                className="flex flex-col gap-0.5"
                aria-label="カテゴリへ移動"
              >
                {categories.map((cat) => (
                  <button
                    key={cat.id}
                    type="button"
                    aria-pressed={selectedCategory === cat.id}
                    className={cn(
                      "nav-item",
                      sidebarDropId === cat.id &&
                        "bg-sidebar-accent ring-1 ring-primary/50",
                      selectedCategory === cat.id && "bg-sidebar-accent",
                    )}
                    style={
                      selectedCategory === cat.id && cat.color
                        ? {
                            backgroundColor: `color-mix(in srgb, ${cat.color} 14%, transparent)`,
                          }
                        : undefined
                    }
                    onClick={() => selectCategory(cat.id)}
                    onDragOver={(event) => {
                      if (!dragId) return;
                      event.preventDefault();
                      event.dataTransfer.dropEffect = "move";
                      setSidebarDropId(cat.id);
                    }}
                    onDragLeave={() =>
                      setSidebarDropId((current) =>
                        current === cat.id ? null : current,
                      )
                    }
                    onDrop={(event) => {
                      event.preventDefault();
                      const id = dragId;
                      setSidebarDropId(null);
                      if (id) void assignCategory(id, cat.id);
                    }}
                  >
                    <span
                      aria-hidden="true"
                      className="nav-icon size-2 shrink-0 rounded-full"
                      style={{
                        backgroundColor: cat.color || "var(--border)",
                      }}
                    />
                    <span className="min-w-0 flex-1 truncate">{cat.name}</span>
                    <span className="tabular-nums text-[11px] text-muted-foreground">
                      {categoryCounts[cat.id] ?? 0}
                    </span>
                  </button>
                ))}
                <button
                  type="button"
                  className={cn(
                    "nav-item",
                    sidebarDropId === "" &&
                      "bg-sidebar-accent ring-1 ring-primary/50",
                  )}
                  onClick={() => scrollToCategory("")}
                  onDragOver={(event) => {
                    if (!dragId) return;
                    event.preventDefault();
                    event.dataTransfer.dropEffect = "move";
                    setSidebarDropId("");
                  }}
                  onDragLeave={() =>
                    setSidebarDropId((current) =>
                      current === "" ? null : current,
                    )
                  }
                  onDrop={(event) => {
                    event.preventDefault();
                    const id = dragId;
                    setSidebarDropId(null);
                    if (id) void assignCategory(id, "");
                  }}
                >
                  <Inbox className="nav-icon size-4" />
                  <span className="min-w-0 flex-1 truncate">未分類</span>
                  <span className="tabular-nums text-[11px] text-muted-foreground">
                    {categoryCounts[""] ?? 0}
                  </span>
                </button>
              </nav>
            ) : null}
          </div>

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
                categoryName={
                  detailApp
                    ? (categoriesById.get(detailApp.categoryId)?.name ?? null)
                    : null
                }
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
                onDuplicate={() => {
                  if (!detailApp) return;
                  setDetailAppId(null);
                  openDuplicate(detailApp);
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
          <Toaster theme={theme} position="top-right" />
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
              <div className="mx-auto grid w-full max-w-6xl grid-cols-[repeat(auto-fill,minmax(240px,1fr))] gap-3">
                <div className="surface h-40 animate-pulse rounded-[14px]" />
                <div className="surface h-40 animate-pulse rounded-[14px]" />
                <div className="surface h-40 animate-pulse rounded-[14px]" />
                <div className="surface h-40 animate-pulse rounded-[14px]" />
                <div className="surface h-40 animate-pulse rounded-[14px]" />
                <div className="surface h-40 animate-pulse rounded-[14px]" />
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
              <div className="mx-auto flex w-full max-w-6xl flex-col gap-6">
                <ul
                  className="grid w-full list-none grid-cols-[repeat(auto-fill,minmax(240px,1fr))] gap-3 p-0"
                  onDragOver={listDragOver}
                  onDrop={listDrop}
                  onDragLeave={listDragLeave}
                >
                  {orderedApps.map((app, index) => (
                    <li
                      key={app.id}
                      data-app-id={app.id}
                      data-category-section={
                        categoriesById.has(app.categoryId)
                          ? app.categoryId
                          : "__none"
                      }
                      className={cn(
                        "relative flex min-w-0 scroll-mt-4",
                        dragId === app.id && "opacity-45",
                      )}
                    >
                      {dropHint?.id === app.id ? (
                        <div
                          aria-hidden="true"
                          className={cn(
                            "pointer-events-none absolute inset-y-0 z-10 w-0.5 rounded-full bg-primary",
                            dropHint.after
                              ? "-right-[7px]"
                              : "-left-[7px]",
                          )}
                        />
                      ) : null}
                      <AppCard
                        app={app}
                        index={index}
                        category={
                          categoriesById.get(app.categoryId) ?? null
                        }
                        tintColor={
                          app.categoryId &&
                          app.categoryId === selectedCategory
                            ? categoriesById.get(app.categoryId)?.color ||
                              "var(--primary)"
                            : null
                        }
                        busy={busyId === app.id}
                        reorder={
                          apps.length > 1
                            ? {
                                onDragStart: (event) => {
                                  event.dataTransfer.setData(
                                    "text/plain",
                                    app.id,
                                  );
                                  event.dataTransfer.effectAllowed = "move";
                                  setDragId(app.id);
                                },
                                onDragEnd: clearDrag,
                                onKeyDown: (event) =>
                                  reorderKey(app, event),
                              }
                            : undefined
                        }
                        onToggle={() => toggle(app)}
                        onEdit={() => openEdit(app)}
                        onDuplicate={() => openDuplicate(app)}
                        onDetails={() => setDetailAppId(app.id)}
                        onOpenUrl={() => openInBrowser(app)}
                        onSetPinned={(pinned) => void pinApp(app, pinned)}
                      />
                    </li>
                  ))}
                </ul>
              </div>
            )}
          </main>
        </>
      ) : route.page === "categories" ? (
        <CategoriesPage
          categories={categories}
          appCounts={categoryCounts}
          onBack={goBoard}
          onCreate={createCategory}
          onRename={renameCategory}
          onSetColor={setCategoryColor}
          onDelete={removeCategory}
          onReorder={(next) => void commitCategoryOrder(next)}
        />
      ) : (
        <AppFormPage
          mode={route.page === "edit" ? "edit" : "create"}
          form={form}
          categories={categories}
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
    categoryId: "",
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

function previewCategories(): Category[] {
  return [
    { id: "cat-docs", name: "ドキュメント", color: "#3e63dd" },
    { id: "cat-tools", name: "開発ツール", color: "#30a46c" },
    { id: "cat-internal", name: "社内", color: "#8e4ec6" },
  ];
}

function previewApps(): AppView[] {
  return [
    previewBase({
      id: "markdown-preview",
      name: "markdown_preview",
      description: "Markdown をブラウザでリアルタイムプレビューする",
      hostname: "markdown-preview",
      categoryId: "cat-docs",
      command: "npm run dev",
      port: 5173,
      favicon: previewFavicon("#34c759", "M"),
    }),
    previewBase({
      id: "storybook",
      name: "Storybook",
      description: "UI コンポーネントをカタログとして確認する",
      hostname: "storybook",
      categoryId: "cat-tools",
      folder: "/Users/you/repos/webapp",
      command: "npm run storybook",
      port: 6006,
      favicon: previewFavicon("#ff4785", "S"),
    }),
    previewBase({
      id: "mailhog",
      name: "mailhog",
      hostname: "mailhog",
      categoryId: "cat-tools",
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
      categoryId: "cat-internal",
      command: "bun run dev",
      port: 3000,
      favicon: previewFavicon("#007aff", "I"),
    }),
    previewBase({
      id: "swagger-ui",
      name: "swagger-ui",
      hostname: "swagger-ui",
      categoryId: "cat-docs",
      folder: "/Users/you/repos/api",
      command: "npx swagger-ui-watcher",
      port: 8080,
      favicon: previewFavicon("#49cc90", "A"),
    }),
    previewBase({
      id: "running",
      name: "社内Wiki",
      hostname: "wiki",
      categoryId: "cat-internal",
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

function scrollCategoryIntoView(id: string) {
  const el = document.querySelector(
    `[data-category-section="${CSS.escape(id || "__none")}"]`,
  );
  el?.scrollIntoView({ behavior: "smooth", block: "start" });
}

function dropTargetAt(
  listEl: HTMLElement,
  x: number,
  y: number,
): { id: string; after: boolean } | null {
  const listWidth = listEl.getBoundingClientRect().width;
  let lastId: string | null = null;
  for (const item of Array.from(listEl.children)) {
    const id = (item as HTMLElement).dataset.appId;
    if (!id) continue;
    lastId = id;
    const rect = item.getBoundingClientRect();
    if (rect.width >= listWidth - 2) {
      if (y < rect.top + rect.height / 2) return { id, after: false };
      continue;
    }
    if (y < rect.top) return { id, after: false };
    if (y <= rect.bottom && x < rect.left + rect.width / 2) {
      return { id, after: false };
    }
  }
  return lastId ? { id: lastId, after: true } : null;
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
