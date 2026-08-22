import type { ReactNode } from "react";

export function AppShell({
  nativeChrome,
  sidebar,
  children,
  overlays,
}: {
  nativeChrome: boolean;
  sidebar: ReactNode;
  children: ReactNode;
  overlays?: ReactNode;
}) {
  return (
    <div className="shell">
      <aside className="sidebar">
        <div className="sidebar-titlebar">
          {!nativeChrome && (
            <div className="traffic-lights" aria-hidden="true">
              <span className="tl-close" />
              <span className="tl-min" />
              <span className="tl-zoom" />
            </div>
          )}
        </div>
        {sidebar}
      </aside>
      <div className="stage">{children}</div>
      {overlays}
    </div>
  );
}
