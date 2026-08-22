import { useEffect, useState } from "react";
import { StatusLed } from "@/components/status-badge";
import { cn } from "@/lib/utils";
import type { AppStatus } from "@/types";

export function AppFavicon({
  name,
  favicon,
  status,
  className,
}: {
  name: string;
  favicon: string | null;
  status: AppStatus;
  className?: string;
}) {
  const [broken, setBroken] = useState(false);

  useEffect(() => {
    setBroken(false);
  }, [favicon]);

  const showImage = Boolean(favicon) && !broken;
  const letter = firstLetter(name);
  const hue = hueFromName(name);

  return (
    <span
      className={cn("relative mt-0.5 inline-flex size-9 shrink-0", className)}
      aria-hidden="true"
    >
      <span
        className="flex size-9 items-center justify-center overflow-hidden rounded-[10px] shadow-[inset_0_0_0_0.5px_var(--hairline)]"
        style={
          showImage
            ? { background: "var(--muted)" }
            : {
                background: `linear-gradient(160deg, hsl(${hue} 32% 52%), hsl(${hue} 38% 38%))`,
              }
        }
      >
        {showImage ? (
          <img
            src={favicon ?? ""}
            alt=""
            className="size-full object-contain p-[3px]"
            onError={() => setBroken(true)}
          />
        ) : (
          <span className="text-[13px] font-semibold tracking-tight text-white">
            {letter}
          </span>
        )}
      </span>
      <StatusLed
        status={status}
        className="absolute right-0 bottom-0 mt-0 ring-2 ring-[var(--card)]"
      />
    </span>
  );
}

function firstLetter(name: string): string {
  const trimmed = name.trim();
  if (!trimmed) return "?";
  return trimmed[0]!.toUpperCase();
}

function hueFromName(name: string): number {
  let hash = 0;
  for (let i = 0; i < name.length; i += 1) {
    hash = (hash * 33 + name.charCodeAt(i)) >>> 0;
  }
  return hash % 360;
}
