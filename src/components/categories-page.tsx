import {
  FormEvent,
  useRef,
  useState,
  type DragEvent,
  type KeyboardEvent,
} from "react";
import { ChevronLeft, GripVertical, Plus, Trash2 } from "lucide-react";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogMedia,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Spinner } from "@/components/ui/spinner";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { CATEGORY_COLORS } from "@/lib/category-colors";
import { moveById } from "@/lib/reorder";
import { cn } from "@/lib/utils";
import type { Category } from "@/types";

export function CategoriesPage({
  categories,
  appCounts,
  onBack,
  onCreate,
  onRename,
  onSetColor,
  onDelete,
  onReorder,
}: {
  categories: Category[];
  appCounts: Record<string, number>;
  onBack: () => void;
  onCreate: (name: string) => Promise<boolean>;
  onRename: (id: string, name: string) => Promise<boolean>;
  onSetColor: (id: string, color: string) => Promise<boolean>;
  onDelete: (id: string) => Promise<void>;
  onReorder: (next: Category[]) => void;
}) {
  const [newName, setNewName] = useState("");
  const [drafts, setDrafts] = useState<Record<string, string>>({});
  const [creating, setCreating] = useState(false);
  const [confirmId, setConfirmId] = useState<string | null>(null);
  const [deleting, setDeleting] = useState(false);
  const [dragId, setDragId] = useState<string | null>(null);
  const [dropHint, setDropHint] = useState<{
    id: string;
    after: boolean;
  } | null>(null);
  const cancelledEdits = useRef<Set<string>>(new Set());

  const confirmCategory =
    categories.find((cat) => cat.id === confirmId) ?? null;
  const confirmCount = confirmCategory
    ? (appCounts[confirmCategory.id] ?? 0)
    : 0;

  async function submitNew(event: FormEvent) {
    event.preventDefault();
    const name = newName.trim();
    if (!name || creating) return;
    setCreating(true);
    try {
      if (await onCreate(name)) setNewName("");
    } finally {
      setCreating(false);
    }
  }

  function draftOf(cat: Category): string {
    return drafts[cat.id] ?? cat.name;
  }

  async function commitRename(cat: Category) {
    if (cancelledEdits.current.delete(cat.id)) {
      setDrafts((current) => {
        const { [cat.id]: _, ...rest } = current;
        return rest;
      });
      return;
    }
    const next = (drafts[cat.id] ?? cat.name).trim();
    if (next === cat.name) {
      setDrafts((current) => {
        const { [cat.id]: _, ...rest } = current;
        return rest;
      });
      return;
    }
    const ok = await onRename(cat.id, next);
    setDrafts((current) => {
      const { [cat.id]: _, ...rest } = current;
      return ok ? rest : { ...current };
    });
  }

  function renameKey(cat: Category, event: KeyboardEvent<HTMLInputElement>) {
    if (event.key === "Enter") {
      event.preventDefault();
      event.currentTarget.blur();
    } else if (event.key === "Escape") {
      cancelledEdits.current.add(cat.id);
      event.currentTarget.blur();
    }
  }

  function clearDrag() {
    setDragId(null);
    setDropHint(null);
  }

  function listDragOver(event: DragEvent<HTMLUListElement>) {
    event.preventDefault();
    if (!dragId) return;
    event.dataTransfer.dropEffect = "move";
    const hint = dropTargetAt(event.currentTarget, event.clientY);
    const next = hint && hint.id !== dragId ? hint : null;
    setDropHint((current) =>
      current?.id === next?.id && current?.after === next?.after
        ? current
        : next,
    );
  }

  function listDrop(event: DragEvent<HTMLUListElement>) {
    event.preventDefault();
    const id = dragId;
    const hint = dropTargetAt(event.currentTarget, event.clientY);
    clearDrag();
    if (!id || !hint || hint.id === id) return;
    onReorder(moveById(categories, id, hint.id, hint.after));
  }

  function listDragLeave(event: DragEvent<HTMLUListElement>) {
    const related = event.relatedTarget as Node | null;
    if (!related || !event.currentTarget.contains(related)) {
      setDropHint(null);
    }
  }

  function reorderKey(cat: Category, event: KeyboardEvent<HTMLButtonElement>) {
    if (event.key !== "ArrowUp" && event.key !== "ArrowDown") return;
    event.preventDefault();
    const index = categories.findIndex((item) => item.id === cat.id);
    const neighbor =
      event.key === "ArrowUp" ? categories[index - 1] : categories[index + 1];
    if (!neighbor) return;
    onReorder(
      moveById(categories, cat.id, neighbor.id, event.key === "ArrowDown"),
    );
  }

  return (
    <>
      <header className="toolbar">
        <Button type="button" variant="ghost" size="sm" onClick={onBack}>
          <ChevronLeft data-icon="inline-start" />
          戻る
        </Button>
      </header>

      <div className="stage-body">
        <div className="mx-auto flex max-w-2xl flex-col gap-7 px-6 py-7 pb-10">
          <div>
            <h1 className="text-[22px] font-semibold tracking-tight">
              カテゴリ
            </h1>
            <p className="mt-1 text-[13px] text-muted-foreground">
              ボードのアプリをカテゴリでまとめて表示できます。行をドラッグ、またはグリップで
              ↑↓ キーを押すと並べ替えられます。名前はクリックして直接編集、色は丸いスウォッチで選べます。
            </p>
          </div>

          <section className="flex flex-col gap-2">
            <form
              onSubmit={submitNew}
              className="flex items-center gap-2 px-1"
            >
              <Input
                value={newName}
                placeholder="新しいカテゴリ名"
                aria-label="新しいカテゴリ名"
                onChange={(event) => setNewName(event.target.value)}
              />
              <Button type="submit" size="sm" disabled={creating || !newName.trim()}>
                {creating ? <Spinner /> : <Plus data-icon="inline-start" />}
                追加
              </Button>
            </form>

            {categories.length === 0 ? (
              <div className="surface settings-group px-4 py-6 text-center text-[13px] text-muted-foreground">
                カテゴリはまだありません。上の欄から追加してください。
              </div>
            ) : (
              <ul
                className="surface settings-group list-none gap-0 p-0"
                onDragOver={listDragOver}
                onDrop={listDrop}
                onDragLeave={listDragLeave}
              >
                {categories.map((cat) => (
                  <li
                    key={cat.id}
                    data-cat-id={cat.id}
                    className={cn(
                      "relative flex items-center gap-2 px-3 py-2",
                      dragId === cat.id && "opacity-45",
                    )}
                  >
                    {dropHint?.id === cat.id ? (
                      <div
                        aria-hidden="true"
                        className={cn(
                          "pointer-events-none absolute inset-x-2 z-10 h-0.5 rounded-full bg-primary",
                          dropHint.after ? "-bottom-[3px]" : "-top-[3px]",
                        )}
                      />
                    ) : null}
                    <Tooltip>
                      <TooltipTrigger
                        render={
                          <button
                            type="button"
                            draggable
                            aria-label={`${cat.name} を並べ替え（上下キーでも移動できます）`}
                            className="inline-flex size-5 shrink-0 cursor-grab items-center justify-center rounded-md text-muted-foreground/60 outline-none hover:bg-muted hover:text-muted-foreground focus-visible:ring-2 focus-visible:ring-ring/60 active:cursor-grabbing"
                            onDragStart={(event) => {
                              event.dataTransfer.setData("text/plain", cat.id);
                              event.dataTransfer.effectAllowed = "move";
                              setDragId(cat.id);
                            }}
                            onDragEnd={clearDrag}
                            onKeyDown={(event) => reorderKey(cat, event)}
                          />
                          }
                        >
                          <GripVertical />
                        </TooltipTrigger>
                        <TooltipContent>
                          ドラッグまたは ↑↓ キーで並べ替え
                        </TooltipContent>
                      </Tooltip>
                    <Input
                      value={draftOf(cat)}
                      aria-label={`${cat.name} のカテゴリ名`}
                      className="h-7 flex-1 border-transparent bg-transparent px-1.5 font-medium shadow-none hover:bg-muted/60 focus-visible:bg-transparent dark:bg-transparent dark:hover:bg-input/40"
                      onChange={(event) =>
                        setDrafts((current) => ({
                          ...current,
                          [cat.id]: event.target.value,
                        }))
                      }
                      onBlur={() => void commitRename(cat)}
                      onKeyDown={(event) => renameKey(cat, event)}
                    />
                    <div
                      className="flex shrink-0 items-center gap-1"
                      role="group"
                      aria-label={`${cat.name} の色`}
                    >
                      <button
                        type="button"
                        title="色なし"
                        aria-label="色なし"
                        aria-pressed={!cat.color}
                        className={cn(
                          "relative size-3.5 rounded-full border border-muted-foreground/40 transition hover:scale-110",
                          !cat.color &&
                            "ring-2 ring-foreground/40 ring-offset-1 ring-offset-card",
                        )}
                        onClick={() => void onSetColor(cat.id, "")}
                      >
                        <span
                          aria-hidden="true"
                          className="absolute top-1/2 left-1/2 h-px w-3 -translate-x-1/2 -translate-y-1/2 rotate-45 bg-muted-foreground/50"
                        />
                      </button>
                      {CATEGORY_COLORS.map((color) => (
                        <button
                          key={color}
                          type="button"
                          title={color}
                          aria-label={`色 ${color}`}
                          aria-pressed={cat.color === color}
                          className={cn(
                            "size-3.5 rounded-full transition hover:scale-110",
                            cat.color === color &&
                              "ring-2 ring-foreground/40 ring-offset-1 ring-offset-card",
                          )}
                          style={{ backgroundColor: color }}
                          onClick={() => void onSetColor(cat.id, color)}
                        />
                      ))}
                    </div>
                    <span className="shrink-0 tabular-nums text-[11px] text-muted-foreground">
                      {appCounts[cat.id] ?? 0}
                    </span>
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon-xs"
                      className="shrink-0 text-muted-foreground hover:text-destructive"
                      aria-label={`${cat.name} を削除`}
                      onClick={() => setConfirmId(cat.id)}
                    >
                      <Trash2 />
                    </Button>
                  </li>
                ))}
              </ul>
            )}
          </section>
        </div>
      </div>

      <AlertDialog
        open={confirmCategory !== null}
        onOpenChange={(open) => {
          if (!open) setConfirmId(null);
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogMedia className="bg-destructive/10 text-destructive">
              <Trash2 />
            </AlertDialogMedia>
            <AlertDialogTitle>このカテゴリを削除する？</AlertDialogTitle>
            <AlertDialogDescription>
              {confirmCategory ? (
                <>
                  <span className="font-medium text-foreground">
                    {confirmCategory.name}
                  </span>{" "}
                  を削除します。
                  {confirmCount > 0
                    ? `含まれる ${confirmCount} 個のアプリは未分類になります。`
                    : "含まれるアプリはありません。"}
                </>
              ) : (
                "カテゴリを削除します。"
              )}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleting}>やめる</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              disabled={deleting}
              onClick={async () => {
                if (!confirmCategory) return;
                setDeleting(true);
                try {
                  await onDelete(confirmCategory.id);
                  setConfirmId(null);
                } finally {
                  setDeleting(false);
                }
              }}
            >
              {deleting && <Spinner />}
              削除
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}

function dropTargetAt(
  listEl: HTMLElement,
  y: number,
): { id: string; after: boolean } | null {
  for (const item of Array.from(listEl.children)) {
    const id = (item as HTMLElement).dataset.catId;
    if (!id) continue;
    const rect = item.getBoundingClientRect();
    if (y < rect.top + rect.height / 2) return { id, after: false };
    if (y <= rect.bottom) return { id, after: true };
  }
  const last = listEl.lastElementChild as HTMLElement | null;
  const id = last?.dataset.catId;
  return id ? { id, after: true } : null;
}
