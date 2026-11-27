import { useEffect, useState } from "react";
import { ChevronRight, Copy, ExternalLink } from "lucide-react";
import { toast } from "sonner";
import { PortalService } from "@/bindings/github.com/taiyop/devportal";
import type {
  TailnetAppSettings,
  TailnetDevice,
  TailnetSettings,
  TailnetStatus,
} from "@/bindings/github.com/taiyop/devportal";
import { Button } from "@/components/ui/button";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { FieldGroup } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Spinner } from "@/components/ui/spinner";
import { copyText } from "@/lib/clipboard";
import { asError } from "@/lib/error";
import type { AppView } from "@/types";

const defaultTailnetTag = "tag:devportal";

function resolveDisplayTag(raw: string) {
  const trimmed = raw.trim();
  if (!trimmed) return defaultTailnetTag;
  return /^tag:/i.test(trimmed) ? `tag:${trimmed.slice(4)}` : `tag:${trimmed}`;
}

function buildTagOwnersSnippet(tag: string) {
  return `"tagOwners": {\n  "${tag}": ["autogroup:admin"],\n},`;
}

const privateApp: TailnetAppSettings = {
  enabled: false,
  port: 0,
  restrictDevices: false,
  deviceIds: [],
};

function updateSelection(ids: string[], id: string, selected: boolean) {
  return selected ? [...ids, id] : ids.filter((item) => item !== id);
}

function describeStatus(status: TailnetStatus) {
  if (status.loginUrl) return "Tailscale へのログインが必要です";
  if (status.state === "Disabled") return "無効";
  if (status.error)
    return status.portalUrl ? "一部のポートで待受エラー" : "接続エラー";
  if (status.portalUrl) return "待受中";
  if (status.state === "Certifying") return "HTTPS 証明書を取得中";
  if (status.state === "NeedsMachineAuth")
    return "管理者による端末承認を待っています";
  if (status.state === "NeedsLogin") return "ログイン URL を取得中";
  return "接続中";
}

export function TailnetSettingsPanel({
  apps,
  preview,
}: {
  apps: AppView[];
  preview: boolean;
}) {
  const [draft, setDraft] = useState<TailnetSettings | null>(null);
  const [status, setStatus] = useState<TailnetStatus | null>(null);
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);
  const [dirty, setDirty] = useState(false);

  useEffect(() => {
    if (preview) return;
    let active = true;
    let refreshing = false;
    const refresh = async () => {
      if (refreshing) return;
      refreshing = true;
      try {
        const next = await PortalService.GetTailnetStatus();
        if (active) setStatus(next);
      } catch {
        if (active) setStatus(null);
      } finally {
        refreshing = false;
      }
    };
    void PortalService.GetTailnetSettings().then(
      (settings) => {
        if (active) setDraft(settings);
      },
      (error) => {
        if (active) setError(asError(error) || "設定を読み込めませんでした");
      },
    );
    void refresh();
    const timer = window.setInterval(() => void refresh(), 2000);
    return () => {
      active = false;
      window.clearInterval(timer);
    };
  }, [preview]);

  function change(next: TailnetSettings) {
    setDraft(next);
    setDirty(true);
  }

  async function save() {
    if (!draft || saving || !dirty) return;
    setSaving(true);
    setError("");
    try {
      setDraft(await PortalService.SetTailnetSettings(draft));
      setDirty(false);
      toast.success("Tailscale の設定を保存しました");
    } catch (error) {
      setError(asError(error) || "設定を保存できませんでした");
    } finally {
      setSaving(false);
    }
  }

  async function openURL(login: boolean) {
    try {
      await PortalService.OpenTailnetURL(login);
    } catch (error) {
      toast.error(asError(error) || "URL を開けませんでした");
    }
  }

  async function copySnippet(text: string, okMessage: string) {
    const copied = await copyText(text);
    toast[copied ? "success" : "error"](
      copied ? okMessage : "コピーできませんでした",
    );
  }

  const activeTag = resolveDisplayTag(draft?.tag ?? "");
  const tagOwnersSnippet = buildTagOwnersSnippet(activeTag);
  const deviceIds = draft?.deviceIds ?? [];
  const devices = status?.devices ?? [];
  const roster = new Map(devices.map((device) => [device.id, device]));
  for (const id of deviceIds) {
    if (!roster.has(id))
      roster.set(id, {
        id,
        name: "現在検出されていない端末",
        dns: id,
        online: false,
      });
  }

  return (
    <section className="flex flex-col gap-2">
      <h2 className="px-1 text-[13px] font-semibold">Tailscale</h2>
      <FieldGroup className="surface settings-group gap-0 p-0">
        <div className="space-y-2 px-4 py-3 text-[13px]">
          {preview ? (
            <p className="text-muted-foreground">
              デスクトップアプリで設定できます。
            </p>
          ) : draft ? (
            <label className="flex w-fit items-center gap-2 font-medium">
              <input
                type="checkbox"
                checked={draft.enabled}
                disabled={saving}
                onChange={(event) =>
                  change({ ...draft, enabled: event.target.checked })
                }
              />
              Tailnet で公開する
            </label>
          ) : !error ? (
            <Spinner aria-label="設定を読み込み中" />
          ) : null}
          {status && status.state !== "Disabled" ? (
            <p role="status" className="text-[12px] text-muted-foreground">
              {describeStatus(status)}
            </p>
          ) : null}
          {status?.tags?.length ? (
            <p className="font-mono text-[11px] text-muted-foreground">
              適用中のタグ: {status.tags.join(", ")}
            </p>
          ) : null}
          {status?.error ? (
            <p className="break-words text-[12px] text-destructive">
              {status.error}
            </p>
          ) : null}
          {status?.loginUrl ? (
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => void openURL(true)}
            >
              <ExternalLink /> Tailscale にログイン
            </Button>
          ) : null}
          {status?.portalUrl ? (
            <div className="flex items-center gap-1.5">
              <button
                type="button"
                className="min-w-0 flex-1 break-all text-left font-mono text-[12px] underline"
                onClick={() => void openURL(false)}
              >
                {status.portalUrl}
              </button>
              <Button
                type="button"
                size="icon-xs"
                variant="ghost"
                aria-label="ポータル URL をコピー"
                onClick={() =>
                  void copySnippet(status.portalUrl, "URL をコピーしました")
                }
              >
                <Copy />
              </Button>
            </div>
          ) : null}
        </div>

        {draft?.enabled ? (
          <>
            <div className="space-y-1.5 border-t border-border px-4 py-3">
              <label
                htmlFor="tailnet-tag"
                className="block text-[13px] font-medium"
              >
                マシンのタグ
              </label>
              <Input
                id="tailnet-tag"
                type="text"
                placeholder="tag:devportal"
                className="font-mono"
                value={draft.tag}
                disabled={saving}
                autoComplete="off"
                onChange={(event) =>
                  change({ ...draft, tag: event.target.value })
                }
              />
              <p className="text-[12px] text-muted-foreground">
                未入力時は <code className="font-mono">tag:devportal</code> です。事前に Tailscale 管理コンソールの Access Controls（<code className="font-mono">tagOwners</code>）でこのタグを定義してください。変更後は再ログインが必要です。
              </p>
              <Collapsible className="pt-0.5">
                <CollapsibleTrigger className="group inline-flex items-center gap-1 text-[12px] font-medium text-muted-foreground transition-colors hover:text-foreground">
                  <ChevronRight className="size-3.5 transition-transform group-data-[panel-open]:rotate-90" />
                  Tailscale 側の tagOwners 設定例を見る
                </CollapsibleTrigger>
                <CollapsibleContent className="mt-2 space-y-2 rounded-md border border-border bg-muted/40 p-2.5 text-[12px] text-muted-foreground">
                  <p>
                    Tailscale 管理コンソールの{" "}
                    <strong className="font-medium text-foreground">
                      Access Controls
                    </strong>
                    （JSON エディタ）に以下を追加して保存します。
                  </p>
                  <div className="relative rounded-md border border-border bg-background/80">
                    <pre className="overflow-x-auto p-2.5 pr-9 font-mono text-[11px] leading-relaxed text-foreground">
                      <code>{tagOwnersSnippet}</code>
                    </pre>
                    <Button
                      type="button"
                      size="icon-xs"
                      variant="ghost"
                      className="absolute top-1.5 right-1.5"
                      aria-label="tagOwners の設定例をコピー"
                      onClick={() =>
                        void copySnippet(
                          tagOwnersSnippet,
                          "tagOwners の設定例をコピーしました",
                        )
                      }
                    >
                      <Copy />
                    </Button>
                  </div>
                  <p className="text-[11px] leading-relaxed">
                    すでに <code className="font-mono">tagOwners</code>{" "}
                    がある場合は、その中に{" "}
                    <code className="font-mono">
                      &quot;{activeTag}&quot;: [&quot;autogroup:admin&quot;],
                    </code>{" "}
                    の行だけを追加してください。管理者以外のユーザーでログインする場合は{" "}
                    <code className="font-mono">&quot;autogroup:admin&quot;</code>{" "}
                    の代わりにそのメールアドレスを指定します。
                  </p>
                </CollapsibleContent>
              </Collapsible>
            </div>

            <fieldset
              disabled={saving}
              className="space-y-1 border-t border-border px-4 py-3"
            >
              <legend className="float-left mb-1 w-full text-[13px] font-medium">
                アクセスを許可する端末
              </legend>
              <p className="clear-both text-[12px] text-muted-foreground">
                ポータルを開ける端末を選びます。未選択の状態ではアクセスできません。
              </p>
              {roster.size ? (
                <ul className="max-h-40 space-y-1.5 overflow-y-auto">
                  {Array.from(roster.values(), (device) => (
                    <li key={device.id}>
                      <DeviceOption
                        device={device}
                        checked={deviceIds.includes(device.id)}
                        onChange={(checked) =>
                          change({
                            ...draft,
                            deviceIds: updateSelection(
                              deviceIds,
                              device.id,
                              checked,
                            ),
                          })
                        }
                      />
                    </li>
                  ))}
                </ul>
              ) : (
                <p className="text-[12px] text-muted-foreground">
                  検出された端末はありません。
                </p>
              )}
            </fieldset>

            <fieldset
              disabled={saving}
              className="space-y-2 border-t border-border px-4 py-3"
            >
              <legend className="float-left mb-1 w-full text-[13px] font-medium">
                公開するアプリ
              </legend>
              <ul className="clear-both max-h-64 space-y-2 overflow-y-auto">
                {apps.map((app) => {
                  const settings =
                    draft.apps &&
                    Object.prototype.hasOwnProperty.call(draft.apps, app.id)
                      ? (draft.apps[app.id] ?? privateApp)
                      : privateApp;
                  const update = (next: TailnetAppSettings) =>
                    change({
                      ...draft,
                      apps: { ...draft.apps, [app.id]: next },
                    });
                  const appDevices = settings.deviceIds ?? [];
                  return (
                    <li
                      key={app.id}
                      className="space-y-1.5 rounded-md border border-border p-2"
                    >
                      <label className="flex items-center gap-2 text-[13px] font-medium">
                        <input
                          type="checkbox"
                          checked={settings.enabled}
                          onChange={(event) =>
                            update({
                              ...settings,
                              enabled: event.target.checked,
                            })
                          }
                        />
                        <span className="min-w-0 flex-1 break-words">
                          {app.name}
                        </span>
                        {settings.enabled && settings.port ? (
                          <span className="font-mono text-[11px] text-muted-foreground">
                            :{settings.port}
                          </span>
                        ) : null}
                      </label>
                      {settings.enabled ? (
                        <div className="space-y-1 pl-5">
                          <label className="flex items-center gap-2 text-[12px]">
                            <input
                              type="checkbox"
                              checked={settings.restrictDevices}
                              onChange={(event) =>
                                update({
                                  ...settings,
                                  restrictDevices: event.target.checked,
                                })
                              }
                            />
                            このアプリの許可端末を絞る
                          </label>
                          {settings.restrictDevices ? (
                            <ul className="space-y-1.5 pl-4">
                              {deviceIds.map((id) => {
                                const device = roster.get(id);
                                return device ? (
                                  <li key={id}>
                                    <DeviceOption
                                      device={device}
                                      checked={appDevices.includes(id)}
                                      onChange={(checked) =>
                                        update({
                                          ...settings,
                                          deviceIds: updateSelection(
                                            appDevices,
                                            id,
                                            checked,
                                          ),
                                        })
                                      }
                                    />
                                  </li>
                                ) : null;
                              })}
                              <li className="text-[11px] text-muted-foreground">
                                選択した端末にのみ公開します。
                              </li>
                            </ul>
                          ) : null}
                        </div>
                      ) : null}
                    </li>
                  );
                })}
                {!apps.length ? (
                  <li className="text-[12px] text-muted-foreground">
                    登録されたアプリはありません。
                  </li>
                ) : null}
              </ul>
            </fieldset>
          </>
        ) : null}

        {error ? (
          <p
            role="alert"
            className="break-words px-4 pb-2 text-[12px] text-destructive"
          >
            {error}
          </p>
        ) : null}
        {draft ? (
          <div className="flex justify-end border-t border-border px-4 py-2">
            <Button
              type="button"
              size="sm"
              disabled={saving || !dirty}
              onClick={() => void save()}
            >
              {saving ? <Spinner /> : null} 保存
            </Button>
          </div>
        ) : null}
      </FieldGroup>
    </section>
  );
}

function DeviceOption({
  device,
  checked,
  onChange,
}: {
  device: TailnetDevice;
  checked: boolean;
  onChange: (checked: boolean) => void;
}) {
  return (
    <label className="flex items-start gap-2 text-[12px]">
      <input
        className="mt-1"
        type="checkbox"
        checked={checked}
        onChange={(event) => onChange(event.target.checked)}
      />
      <span className="min-w-0 break-all">
        {device.name}{" "}
        {device.online ? (
          <span className="text-muted-foreground">・オンライン</span>
        ) : null}
        <span className="block text-[11px] text-muted-foreground">
          {device.dns}
        </span>
      </span>
    </label>
  );
}
