import { useEffect, useState } from "react";

export type AppRoute =
  | { page: "board"; preview: boolean }
  | { page: "new"; preview: boolean }
  | { page: "categories"; preview: boolean }
  | { page: "edit"; id: string; preview: boolean };

export function parseHash(hash: string): AppRoute {
  let raw = hash.startsWith("#") ? hash.slice(1) : hash;
  raw = raw.replace(/^\/+/, "");
  const preview = raw === "preview" || raw.startsWith("preview/");
  let path = preview ? raw.slice("preview".length) : raw;
  path = path.replace(/^\/+/, "");

  if (path === "new") return { page: "new", preview };
  if (path === "categories") return { page: "categories", preview };
  if (path.startsWith("edit/")) {
    const id = decodeURIComponent(path.slice("edit/".length));
    if (id) return { page: "edit", id, preview };
  }
  return { page: "board", preview };
}

export function hashFor(route: AppRoute): string {
  if (route.page === "board") return route.preview ? "#preview" : "";
  if (route.page === "new") return route.preview ? "#preview/new" : "#/new";
  if (route.page === "categories")
    return route.preview ? "#preview/categories" : "#/categories";
  const id = encodeURIComponent(route.id);
  return route.preview ? `#preview/edit/${id}` : `#/edit/${id}`;
}

export function navigate(route: AppRoute): void {
  const next = hashFor(route);
  const current = window.location.hash === "#" ? "" : window.location.hash;
  if (current === next) return;

  if (!next) {
    history.pushState(
      null,
      "",
      `${window.location.pathname}${window.location.search}`,
    );
    window.dispatchEvent(new HashChangeEvent("hashchange"));
    return;
  }
  window.location.hash = next.slice(1);
}

export function useAppRoute(): AppRoute {
  const [route, setRoute] = useState<AppRoute>(() =>
    parseHash(typeof window === "undefined" ? "" : window.location.hash),
  );

  useEffect(() => {
    const sync = () => setRoute(parseHash(window.location.hash));
    window.addEventListener("hashchange", sync);
    window.addEventListener("popstate", sync);
    return () => {
      window.removeEventListener("hashchange", sync);
      window.removeEventListener("popstate", sync);
    };
  }, []);

  return route;
}
