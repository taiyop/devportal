import { useEffect, useState } from "react";
import { Copy, ExternalLink } from "lucide-react";
import { toast } from "sonner";
import { PortalService } from "@/bindings/github.com/taiyop/devportal";
import type {
  TailnetAppSettings,
  TailnetDevice,
  TailnetSettings,
  TailnetStatus,
} from "@/bindings/github.com/taiyop/devportal";
import { Button } from "@/components/ui/button";
import { FieldGroup } from "@/components/ui/field";
import { Spinner } from "@/components/ui/spinner";
import { copyText } from "@/lib/clipboard";
import { asError } from "@/lib/error";
import type { AppView } from "@/types";

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

  async function copyURL(url: string) {
    const copied = await copyText(url);
    toast[copied ? "success" : "error"](
      copied ? "URL をコピーしました" : "コピーできませんでした",
    );
  }

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
                onClick={() => void copyURL(status.portalUrl)}
              >
                <Copy />
              </Button>
            </div>
          ) : null}
        </div>

        {draft?.enabled ? (
          <>
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
