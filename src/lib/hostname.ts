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

export function slugHostname(input: string): string {
  const source = input.trim().toLowerCase();
  let out = "";
  let lastDash = false;
  for (const ch of source) {
    const alnum = (ch >= "a" && ch <= "z") || (ch >= "0" && ch <= "9");
    if (alnum) {
      out += ch;
      lastDash = false;
      continue;
    }
    if (out.length > 0 && !lastDash) {
      out += "-";
      lastDash = true;
    }
  }
  let result = out.replace(/^-+|-+$/g, "");
  if (result.length > 63) {
    result = result.slice(0, 63).replace(/-+$/g, "");
  }
  return result === "" ? "app" : result;
}

export function uniqueHostname(
  taken: readonly string[],
  preferred: string,
): string {
  if (preferred === "") return "";
  const used = new Set(taken);
  let candidate = preferred;
  let n = 2;
  while (used.has(candidate)) {
    const suffix = `-${n}`;
    let base = preferred;
    if (base.length + suffix.length > 63) {
      base = base.slice(0, 63 - suffix.length).replace(/-+$/g, "");
    }
    candidate = `${base}${suffix}`;
    n += 1;
  }
  return candidate;
}
