const HOSTNAME_SUFFIX = "devportal.localhost";

export function hostnameFieldError(input: string): string | null {
  const s = normalizeHostnameInput(input);
  if (s === "") return null;
  if (s.includes(".")) {
    return `ドメインは ${HOSTNAME_SUFFIX} の直前の1ラベルだけ指定してください`;
  }
  if (!validHostnameLabel(s)) {
    return `ドメインが不正です: ${s}（英小文字・数字・ハイフン、先頭末尾はハイフン不可）`;
  }
  return null;
}

export function isHostnameError(message: string): boolean {
  return message.includes("ドメイン");
}

function normalizeHostnameInput(input: string): string {
  let s = input.trim().toLowerCase();
  s = s.replace(/^https?:\/\//, "");
  s = s.replace(/\/$/, "");
  const colon = s.lastIndexOf(":");
  if (colon > 0 && /^\d+$/.test(s.slice(colon + 1))) {
    s = s.slice(0, colon);
  }
  if (s.endsWith("." + HOSTNAME_SUFFIX)) {
    s = s.slice(0, -(HOSTNAME_SUFFIX.length + 1));
  }
  return s.replace(/^\.+|\.+$/g, "");
}

function validHostnameLabel(s: string): boolean {
  if (s.length === 0 || s.length > 63) return false;
  if (s.startsWith("-") || s.endsWith("-")) return false;
  return /^[a-z0-9-]+$/.test(s);
}
