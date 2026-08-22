import { useEffect, useState, type FormEvent } from "react";
import { FolderOpen } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Spinner } from "@/components/ui/spinner";

export function SettingsDialog({
  open,
  configPath,
  gatewayPort,
  gatewayRunning,
  savingPort,
  onOpenChange,
  onRevealConfig,
  onSaveGatewayPort,
}: {
  open: boolean;
  configPath: string;
  gatewayPort: number;
  gatewayRunning: boolean;
  savingPort: boolean;
  onOpenChange: (open: boolean) => void;
  onRevealConfig: () => void;
  onSaveGatewayPort: (port: number) => void;
}) {
  const [draft, setDraft] = useState("80");
  const [portError, setPortError] = useState<string | null>(null);

  useEffect(() => {
    if (!open) return;
    setDraft(String(gatewayPort || 80));
    setPortError(null);
  }, [open, gatewayPort]);

  function submitPort(event: FormEvent) {
    event.preventDefault();
    const parsed = parseGatewayPort(draft);
    if (parsed.error) {
      setPortError(parsed.error);
      return;
    }
    setPortError(null);
    setDraft(String(parsed.port));
    onSaveGatewayPort(parsed.port);
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex max-h-[calc(100dvh-1.5rem)] w-[calc(100%-1.5rem)] max-w-md flex-col gap-0 overflow-hidden p-0 sm:max-w-md">
        <DialogHeader className="shrink-0 gap-1.5 border-b border-border px-4 py-3 pr-12">
          <DialogTitle className="text-[15px] font-semibold tracking-tight">
            設定
          </DialogTitle>
          <DialogDescription className="text-[12px]">
            リバースプロキシのポートと、設定ファイルの場所です。
          </DialogDescription>
        </DialogHeader>

        <div className="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto px-4 py-4">
          <section className="flex flex-col gap-2">
            <h2 className="px-1 text-[13px] font-semibold">リバースプロキシ</h2>
            <form onSubmit={submitPort}>
              <FieldGroup className="surface settings-group gap-0 p-0">
                <Field className="gap-1.5 px-4 py-3" data-invalid={!!portError}>
                  <FieldLabel htmlFor="gateway-port">ポート</FieldLabel>
                  <div className="flex items-center gap-2">
                    <Input
                      id="gateway-port"
                      type="text"
                      inputMode="numeric"
                      placeholder="80"
                      className="font-mono"
                      value={draft}
                      aria-invalid={!!portError}
                      autoComplete="off"
                      onChange={(event) => {
                        setDraft(event.target.value);
                        setPortError(null);
                      }}
                    />
                    <Button type="submit" size="sm" disabled={savingPort}>
                      {savingPort ? <Spinner /> : null}
                      保存
                    </Button>
                  </div>
                  {portError ? (
                    <FieldError>{portError}</FieldError>
                  ) : (
                    <FieldDescription>
                      未入力なら 80 です。空いていなければ別のポートを割り当てます。
                      {gatewayRunning
                        ? " 保存するとリバースプロキシをこのポートで再起動します。"
                        : ""}
                    </FieldDescription>
                  )}
                </Field>
              </FieldGroup>
            </form>
          </section>

          <section className="flex flex-col gap-2">
            <h2 className="px-1 text-[13px] font-semibold">設定ファイル</h2>
            <FieldGroup className="surface settings-group gap-0 p-0">
              <Field className="gap-1.5 px-4 py-3">
                <FieldLabel>apps.yml</FieldLabel>
                <p className="font-mono text-[12px] leading-5 break-all text-muted-foreground">
                  {configPath || "パスを取得できませんでした"}
                </p>
                <FieldDescription>
                  Finder で保存先フォルダを開きます。
                </FieldDescription>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  className="mt-1 w-fit"
                  disabled={!configPath}
                  onClick={onRevealConfig}
                >
                  <FolderOpen data-icon="inline-start" />
                  フォルダを開く
                </Button>
              </Field>
            </FieldGroup>
          </section>
        </div>
      </DialogContent>
    </Dialog>
  );
}

function parseGatewayPort(raw: string): { port: number; error: string | null } {
  const trimmed = raw.trim();
  if (trimmed === "") {
    return { port: 80, error: null };
  }
  if (!/^\d+$/.test(trimmed)) {
    return { port: 0, error: "1 から 65535 の番号を入力してください" };
  }
  const port = Number(trimmed);
  if (port < 1 || port > 65535) {
    return { port: 0, error: "1 から 65535 の番号を入力してください" };
  }
  return { port, error: null };
}
