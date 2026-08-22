export function asError(error: unknown): string {
  const raw = collectErrorText(error)
    .map(stripBindingPrefix)
    .find((text) => text && !isBindingWrapper(text));
  return raw || stripBindingPrefix(collectErrorText(error)[0] ?? "") || "予期しないエラーが発生しました";
}

function collectErrorText(error: unknown, depth = 0): string[] {
  if (error == null || depth > 4) return [];
  if (typeof error === "string") return error.trim() ? [error.trim()] : [];
  if (error instanceof Error) {
    const cause = "cause" in error ? (error as Error & { cause?: unknown }).cause : undefined;
    return [...collectErrorText(cause, depth + 1), error.message.trim()].filter(Boolean);
  }
  if (typeof error === "object") {
    const obj = error as Record<string, unknown>;
    return [
      ...collectErrorText(obj.cause, depth + 1),
      ...collectErrorText(obj.message, depth + 1),
      ...collectErrorText(obj.error, depth + 1),
    ];
  }
  const text = String(error).trim();
  return text ? [text] : [];
}

function isBindingWrapper(text: string): boolean {
  return /^(ERR\s+)?(Binding call failed|Bound method returned an error)\b/i.test(text) &&
    !text.includes("：") &&
    !text.includes(":");
}

function stripBindingPrefix(text: string): string {
  return text
    .replace(/^ERR\s+/i, "")
    .replace(/^Binding call failed:\s*/i, "")
    .replace(/^Bound method returned an error:\s*/i, "")
    .trim();
}
