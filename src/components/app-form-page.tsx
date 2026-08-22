import { FormEvent, useEffect, useRef } from "react";
import { ChevronLeft, FolderOpen, Plus, Trash2 } from "lucide-react";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from "@/components/ui/empty";
import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
  FieldLegend,
  FieldSet,
} from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
} from "@/components/ui/input-group";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { Spinner } from "@/components/ui/spinner";
import { isHostnameError } from "@/lib/hostname";
import {
  DEFAULT_PORT_ENV,
  defaultAppPortEnv,
  emptyBackend,
  emptyEnvRow,
  resolvedPortEnv,
  type AppInput,
  type BackendConfig,
  type EnvVar,
  type PortMode,
} from "@/types";

export function AppFormPage({
  mode,
  form,
  saving,
  error,
  pickingFolder,
  loading,
  missing,
  onBack,
  onFormChange,
  onPickFolder,
  onSubmit,
}: {
  mode: "create" | "edit";
  form: AppInput;
  saving: boolean;
  error: string | null;
  pickingFolder: boolean;
  loading: boolean;
  missing: boolean;
  onBack: () => void;
  onFormChange: (next: AppInput) => void;
  onPickFolder: (target: "app" | number) => void;
  onSubmit: (event: FormEvent) => void;
}) {
  const editing = mode === "edit";
  const errorRef = useRef<HTMLDivElement>(null);
  const hostnameInvalid = Boolean(error && isHostnameError(error));
  const title = editing ? "アプリを編集" : "アプリを登録";

  useEffect(() => {
    if (!error) return;
    errorRef.current?.scrollIntoView({ behavior: "smooth", block: "center" });
  }, [error]);

  return (
    <>
      <header className="toolbar">
        <Button type="button" variant="ghost" size="sm" onClick={onBack}>
          <ChevronLeft data-icon="inline-start" />
          戻る
        </Button>
      </header>

      {loading ? (
        <div className="stage-body px-6 py-6">
          <div className="mx-auto flex max-w-2xl flex-col gap-4">
            <div className="surface h-16 animate-pulse rounded-[14px]" />
            <div className="surface h-[28rem] animate-pulse rounded-[14px]" />
          </div>
        </div>
      ) : missing ? (
        <div className="stage-body flex items-center justify-center px-6">
          <Empty className="max-w-md py-16">
            <EmptyHeader>
              <EmptyTitle>アプリが見つかりません</EmptyTitle>
              <EmptyDescription>
                削除されたか、アドレスが違います。ボードから開き直してください。
              </EmptyDescription>
            </EmptyHeader>
            <EmptyContent>
              <Button type="button" onClick={onBack}>
                ボードに戻る
              </Button>
            </EmptyContent>
          </Empty>
        </div>
      ) : (
        <form onSubmit={onSubmit} className="stage-body">
          <div className="mx-auto flex max-w-2xl flex-col gap-7 px-6 py-7 pb-10">
            <div>
              <h1 className="text-[22px] font-semibold tracking-tight">{title}</h1>
              <p className="mt-1 text-[13px] text-muted-foreground">
                フォルダと起動コマンドを登録すると、画面から起こして止められます。API
                サーバーなどが必要なら、バックエンドも別プロセスで同時起動できます。
              </p>
            </div>

            {error && (
              <Alert ref={errorRef} variant="destructive">
                <AlertTitle>保存できません</AlertTitle>
                <AlertDescription>{error}</AlertDescription>
              </Alert>
            )}

            <section className="flex flex-col gap-2">
              <h2 className="px-1 text-[13px] font-semibold">基本</h2>
              <FieldGroup className="surface settings-group gap-0 p-0">
                <Field className="gap-1.5 px-4 py-3">
                  <FieldLabel htmlFor="app-name">表示名</FieldLabel>
                  <Input
                    id="app-name"
                    value={form.name}
                    placeholder="未入力ならフォルダ名"
                    onChange={(event) =>
                      onFormChange({ ...form, name: event.target.value })
                    }
                  />
                </Field>

                <Field className="gap-1.5 px-4 py-3">
                  <FieldLabel htmlFor="app-description">解説</FieldLabel>
                  <Textarea
                    id="app-description"
                    value={form.description}
                    placeholder="このサービスが何をするか"
                    rows={4}
                    onChange={(event) =>
                      onFormChange({
                        ...form,
                        description: event.target.value,
                      })
                    }
                  />
                  <FieldDescription>任意です。一覧に表示されます。</FieldDescription>
                </Field>

                <Field className="gap-1.5 px-4 py-3" data-invalid={hostnameInvalid || undefined}>
                  <FieldLabel htmlFor="app-hostname">ドメイン</FieldLabel>
                  <InputGroup className="h-8">
                    <InputGroupInput
                      id="app-hostname"
                      className="font-mono"
                      aria-invalid={hostnameInvalid || undefined}
                      value={form.hostname}
                      placeholder="notes"
                      onChange={(event) =>
                        onFormChange({ ...form, hostname: event.target.value })
                      }
                    />
                    <InputGroupAddon align="inline-end" className="font-mono text-[11px]">
                      .devportal.localhost
                    </InputGroupAddon>
                  </InputGroup>
                  {hostnameInvalid ? (
                    <FieldError>{error}</FieldError>
                  ) : (
                    <FieldDescription>
                      英小文字・数字・ハイフンのみです。アンダースコアは使えません。
                      ボードでリバースプロキシを開始すると、http://名前.devportal.localhost でこのアプリに届きます。
                      未入力なら表示名から自動で付けます。
                    </FieldDescription>
                  )}
                </Field>
              </FieldGroup>
            </section>

            <section className="flex flex-col gap-2">
              <h2 className="px-1 text-[13px] font-semibold">起動</h2>
              <FieldGroup className="surface settings-group gap-0 p-0">
                <Field className="gap-1.5 px-4 py-3">
                  <FieldLabel htmlFor="app-folder">対象フォルダ</FieldLabel>
                  <InputGroup className="h-8">
                    <InputGroupInput
                      id="app-folder"
                      required
                      value={form.folder}
                      placeholder="/Users/you/project"
                      onChange={(event) =>
                        onFormChange({ ...form, folder: event.target.value })
                      }
                    />
                    <InputGroupAddon align="inline-end">
                      <InputGroupButton
                        type="button"
                        onClick={() => onPickFolder("app")}
                        disabled={pickingFolder}
                      >
                        {pickingFolder ? <Spinner /> : <FolderOpen />}
                        {pickingFolder ? "選択中" : "選ぶ"}
                      </InputGroupButton>
                    </InputGroupAddon>
                  </InputGroup>
                </Field>

                <Field className="gap-1.5 px-4 py-3">
                  <FieldLabel htmlFor="app-command">起動コマンド</FieldLabel>
                  <Input
                    id="app-command"
                    required
                    className="font-mono"
                    value={form.command}
                    placeholder="npm run dev"
                    onChange={(event) =>
                      onFormChange({ ...form, command: event.target.value })
                    }
                  />
                  <FieldDescription>
                    待受番号は環境変数{" "}
                    <code>{resolvedPortEnv(form.portEnv)}</code> に入ります。
                    <code>{"{port}"}</code> でも埋め込めます。
                    {form.backends.length > 0 ? (
                      <>
                        {" "}
                        バックエンドがあるときは <code>{"{backendPort}"}</code>
                        {" "}
                        （1つ目）、<code>{"{backendPort:1}"}</code>、名前を付けたときは{" "}
                        <code>{"{backendPort:api}"}</code> も使えます。
                      </>
                    ) : null}
                  </FieldDescription>
                </Field>

                <EnvVarFields
                  rows={form.env}
                  onChange={(env) => onFormChange({ ...form, env })}
                />
              </FieldGroup>
            </section>

            <section className="flex flex-col gap-2">
              <div className="flex items-center justify-between gap-2 px-1">
                <h2 className="text-[13px] font-semibold">バックエンド</h2>
                <Button
                  type="button"
                  id="add-backend"
                  variant="outline"
                  size="sm"
                  onClick={() =>
                    onFormChange({
                      ...form,
                      backends: [
                        ...form.backends,
                        emptyBackend(form.backends.length),
                      ],
                    })
                  }
                >
                  <Plus data-icon="inline-start" />
                  追加
                </Button>
              </div>
              {form.backends.length === 0 ? (
                <FieldGroup className="surface settings-group gap-0 p-0">
                  <Field className="gap-1.5 px-4 py-3">
                    <FieldDescription>
                      API サーバーなどを別プロセスで同時に起動できます。必要な数だけ追加してください。
                    </FieldDescription>
                  </Field>
                </FieldGroup>
              ) : (
                form.backends.map((backend, index) => (
                  <BackendFields
                    key={index}
                    index={index}
                    backend={backend}
                    pickingFolder={pickingFolder}
                    onPickFolder={() => onPickFolder(index)}
                    onChange={(next) =>
                      onFormChange({
                        ...form,
                        backends: form.backends.map((item, itemIndex) =>
                          itemIndex === index ? next : item,
                        ),
                      })
                    }
                    onRemove={() =>
                      onFormChange({
                        ...form,
                        backends: form.backends.filter(
                          (_, itemIndex) => itemIndex !== index,
                        ),
                      })
                    }
                  />
                ))
              )}
            </section>

            <section className="flex flex-col gap-2">
              <h2 className="px-1 text-[13px] font-semibold">ポート</h2>
              <FieldGroup className="surface settings-group gap-0 p-0">
                <PortModeFields
                  idPrefix="app"
                  legend="割り当て"
                  value={form.portMode}
                  port={form.port}
                  defaultPort={5173}
                  envName={form.portEnv}
                  envPlaceholder={DEFAULT_PORT_ENV}
                  envDescription="このプロセスへ待受番号を渡す環境変数名です。多くの dev server は PORT を読みます。"
                  onChange={(portMode, port) =>
                    onFormChange({ ...form, portMode, port })
                  }
                  onEnvNameChange={(portEnv) =>
                    onFormChange({ ...form, portEnv })
                  }
                />

                <Field className="gap-1.5 px-4 py-3">
                  <FieldLabel htmlFor="idle-stop-min">
                    アクセスがなくなったら止める
                  </FieldLabel>
                  <FieldDescription>
                    ドメインへの最後のアクセスからこの時間が経つと自動で止まります。
                    一覧の Keep alive がオンの間は止まりません。画面の「起動」は Keep alive オンで始まります。
                  </FieldDescription>
                  <div className="flex flex-wrap gap-1.5">
                    {([
                      [0, "しない"],
                      [5, "5分"],
                      [15, "15分"],
                      [30, "30分"],
                      [60, "1時間"],
                    ] as const).map(([value, label]) => (
                      <Button
                        key={value}
                        type="button"
                        size="sm"
                        variant={form.idleStopMin === value ? "default" : "outline"}
                        onClick={() =>
                          onFormChange({ ...form, idleStopMin: value })
                        }
                      >
                        {label}
                      </Button>
                    ))}
                  </div>
                  <Input
                    id="idle-stop-min"
                    type="number"
                    min={0}
                    max={1440}
                    value={form.idleStopMin}
                    onChange={(event) =>
                      onFormChange({
                        ...form,
                        idleStopMin: event.target.value
                          ? Math.max(0, Number(event.target.value))
                          : 0,
                      })
                    }
                  />
                  <FieldDescription>0 は自動終了しません。最大 1440 分です。</FieldDescription>
                </Field>
              </FieldGroup>
            </section>

            <div className="flex justify-end gap-2 pt-1">
              <Button type="button" variant="outline" onClick={onBack}>
                キャンセル
              </Button>
              <Button type="submit" disabled={saving}>
                {saving && <Spinner />}
                {saving ? "保存中" : "保存"}
              </Button>
            </div>
          </div>
        </form>
      )}
    </>
  );
}

function BackendFields({
  index,
  backend,
  pickingFolder,
  onPickFolder,
  onChange,
  onRemove,
}: {
  index: number;
  backend: BackendConfig;
  pickingFolder: boolean;
  onPickFolder: () => void;
  onChange: (next: BackendConfig) => void;
  onRemove: () => void;
}) {
  const label = backend.name.trim() || `バックエンド${index + 1}`;
  const idPrefix = `backend-${index}`;
  return (
    <FieldGroup className="surface settings-group gap-0 p-0">
      <div className="flex items-center justify-between gap-2 px-4 py-3">
        <h3 className="text-[13px] font-semibold">{label}</h3>
        <Button type="button" variant="ghost" size="icon" onClick={onRemove}>
          <Trash2 />
          <span className="sr-only">{label}を削除</span>
        </Button>
      </div>

      <Field className="gap-1.5 px-4 py-3">
        <FieldLabel htmlFor={`${idPrefix}-name`}>名前</FieldLabel>
        <Input
          id={`${idPrefix}-name`}
          className="font-mono"
          value={backend.name}
          placeholder="api"
          onChange={(event) =>
            onChange({ ...backend, name: event.target.value })
          }
        />
        <FieldDescription>
          任意です。コマンドでは <code>{`{backendPort:${backend.name.trim() || "api"}}`}</code>{" "}
          として埋め込めます。
        </FieldDescription>
      </Field>

      <Field className="gap-1.5 px-4 py-3">
        <FieldLabel htmlFor={`${idPrefix}-folder`}>対象フォルダ</FieldLabel>
        <InputGroup className="h-8">
          <InputGroupInput
            id={`${idPrefix}-folder`}
            value={backend.folder}
            placeholder="未入力ならアプリと同じ"
            onChange={(event) =>
              onChange({ ...backend, folder: event.target.value })
            }
          />
          <InputGroupAddon align="inline-end">
            <InputGroupButton
              type="button"
              onClick={onPickFolder}
              disabled={pickingFolder}
            >
              {pickingFolder ? <Spinner /> : <FolderOpen />}
              {pickingFolder ? "選択中" : "選ぶ"}
            </InputGroupButton>
          </InputGroupAddon>
        </InputGroup>
      </Field>

      <Field className="gap-1.5 px-4 py-3">
        <FieldLabel htmlFor={`${idPrefix}-command`}>起動コマンド</FieldLabel>
        <Input
          id={`${idPrefix}-command`}
          required
          className="font-mono"
          value={backend.command}
          placeholder="npm run server"
          onChange={(event) =>
            onChange({ ...backend, command: event.target.value })
          }
        />
        <FieldDescription>
          待受は <code>{resolvedPortEnv(backend.portEnv)}</code> と{" "}
          <code>{`{backendPort:${index + 1}}`}</code> です。アプリ側のポートは{" "}
          <code>{"{port}"}</code> です。
        </FieldDescription>
      </Field>

      <EnvVarFields
        rows={backend.env}
        namePlaceholder="DATABASE_URL"
        valuePlaceholder="postgres://127.0.0.1/app"
        onChange={(env) => onChange({ ...backend, env })}
      />

      <PortModeFields
        idPrefix={idPrefix}
        legend="ポート"
        value={backend.portMode}
        port={backend.port}
        defaultPort={8080}
        envName={backend.portEnv}
        envPlaceholder={DEFAULT_PORT_ENV}
        envDescription="このバックエンドプロセスへ待受番号を渡す環境変数名です。"
        onChange={(portMode, port) => onChange({ ...backend, portMode, port })}
        onEnvNameChange={(portEnv) => onChange({ ...backend, portEnv })}
      />

      <Field className="gap-1.5 px-4 py-3">
        <FieldLabel htmlFor={`${idPrefix}-app-port-env`}>
          アプリ側に渡す変数名
        </FieldLabel>
        <Input
          id={`${idPrefix}-app-port-env`}
          className="font-mono"
          value={backend.appPortEnv}
          placeholder={defaultAppPortEnv(index)}
          onChange={(event) =>
            onChange({ ...backend, appPortEnv: event.target.value })
          }
        />
        <FieldDescription>
          アプリ本体へ、このバックエンドの待受番号をこの名前で渡します。未入力なら{" "}
          <code>{defaultAppPortEnv(index)}</code> です。
        </FieldDescription>
      </Field>
    </FieldGroup>
  );
}

function EnvVarFields({
  rows,
  onChange,
  namePlaceholder = "HOST",
  valuePlaceholder = "127.0.0.1",
}: {
  rows: EnvVar[];
  onChange: (rows: EnvVar[]) => void;
  namePlaceholder?: string;
  valuePlaceholder?: string;
}) {
  return (
    <FieldSet className="gap-2 px-4 py-3">
      <FieldLegend variant="label">環境変数</FieldLegend>
      <FieldDescription>
        1行が1変数です。値の <code>{"{port}"}</code> と{" "}
        <code>{"{backendPort}"}</code> は実際の番号に置換されます。
      </FieldDescription>
      <div className="flex flex-col gap-2">
        {rows.map((row, index) => (
          <div
            key={index}
            className="grid grid-cols-[minmax(0,0.9fr)_auto_minmax(0,1.2fr)_auto] items-center gap-1.5"
          >
            <Input
              className="font-mono"
              placeholder={namePlaceholder}
              value={row.name}
              onChange={(event) =>
                onChange(
                  rows.map((item, itemIndex) =>
                    itemIndex === index
                      ? { ...item, name: event.target.value }
                      : item,
                  ),
                )
              }
            />
            <span className="text-muted-foreground">=</span>
            <Input
              className="font-mono"
              placeholder={valuePlaceholder}
              value={row.value}
              onChange={(event) =>
                onChange(
                  rows.map((item, itemIndex) =>
                    itemIndex === index
                      ? { ...item, value: event.target.value }
                      : item,
                  ),
                )
              }
            />
            <Button
              type="button"
              variant="ghost"
              size="icon"
              onClick={() =>
                onChange(
                  rows.length <= 1
                    ? [emptyEnvRow()]
                    : rows.filter((_, itemIndex) => itemIndex !== index),
                )
              }
            >
              <Trash2 />
              <span className="sr-only">変数を削除</span>
            </Button>
          </div>
        ))}
      </div>
      <Button
        type="button"
        variant="outline"
        size="sm"
        className="w-fit"
        onClick={() => onChange([...rows, emptyEnvRow()])}
      >
        <Plus data-icon="inline-start" />
        変数を追加
      </Button>
    </FieldSet>
  );
}

function PortModeFields({
  idPrefix,
  legend,
  value,
  port,
  defaultPort,
  envName,
  envPlaceholder,
  envDescription,
  onChange,
  onEnvNameChange,
}: {
  idPrefix: string;
  legend: string;
  value: PortMode;
  port: number | null;
  defaultPort: number;
  envName: string;
  envPlaceholder: string;
  envDescription: string;
  onChange: (portMode: PortMode, port: number | null) => void;
  onEnvNameChange: (name: string) => void;
}) {
  return (
    <FieldSet className="gap-2 px-4 py-3">
      <FieldLegend variant="label">{legend}</FieldLegend>
      <RadioGroup
        value={value}
        className="grid grid-cols-2 gap-2"
        onValueChange={(next) => {
          if (next === "auto") {
            onChange("auto", port);
          }
          if (next === "manual") {
            onChange("manual", port ?? defaultPort);
          }
        }}
      >
        <FieldLabel className="flex w-full cursor-pointer flex-col items-start gap-1 rounded-[10px] border border-border bg-background/50 p-3 has-data-checked:border-primary has-data-checked:bg-accent">
          <span className="flex items-center gap-2">
            <RadioGroupItem value="auto" id={`${idPrefix}-port-auto`} />
            自動
          </span>
          <span className="pl-6 text-[11px] font-normal text-muted-foreground">
            空いている番号を割り当て
          </span>
        </FieldLabel>
        <FieldLabel className="flex w-full cursor-pointer flex-col items-start gap-1 rounded-[10px] border border-border bg-background/50 p-3 has-data-checked:border-primary has-data-checked:bg-accent">
          <span className="flex items-center gap-2">
            <RadioGroupItem value="manual" id={`${idPrefix}-port-manual`} />
            手動
          </span>
          <span className="pl-6 text-[11px] font-normal text-muted-foreground">
            番号を自分で決める
          </span>
        </FieldLabel>
      </RadioGroup>
      {value === "manual" && (
        <Input
          id={`${idPrefix}-port`}
          type="number"
          min={1}
          max={65535}
          required
          value={port ?? ""}
          onChange={(event) =>
            onChange(
              "manual",
              event.target.value ? Number(event.target.value) : null,
            )
          }
        />
      )}
      <Field className="gap-1.5">
        <FieldLabel htmlFor={`${idPrefix}-port-env`}>環境変数名</FieldLabel>
        <Input
          id={`${idPrefix}-port-env`}
          className="font-mono"
          value={envName}
          placeholder={envPlaceholder}
          onChange={(event) => onEnvNameChange(event.target.value)}
        />
        <FieldDescription>{envDescription}</FieldDescription>
      </Field>
    </FieldSet>
  );
}
