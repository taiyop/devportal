import { Moon, Sun } from "lucide-react";
import { Button } from "@/components/ui/button";
import { ButtonGroup } from "@/components/ui/button-group";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import type { Theme } from "@/theme";

export function ThemeToggle({
  theme,
  onChange,
}: {
  theme: Theme;
  onChange: (theme: Theme) => void;
}) {
  return (
    <ButtonGroup aria-label="表示モード" className="rounded-full bg-muted p-0.5">
      <Tooltip>
        <TooltipTrigger
          render={
            <Button
              type="button"
              variant={theme === "light" ? "secondary" : "ghost"}
              size="icon-xs"
              className="rounded-full"
              aria-pressed={theme === "light"}
              onClick={() => onChange("light")}
            />
          }
        >
          <Sun />
          <span className="sr-only">ライト</span>
        </TooltipTrigger>
        <TooltipContent>ライト</TooltipContent>
      </Tooltip>
      <Tooltip>
        <TooltipTrigger
          render={
            <Button
              type="button"
              variant={theme === "dark" ? "secondary" : "ghost"}
              size="icon-xs"
              className="rounded-full"
              aria-pressed={theme === "dark"}
              onClick={() => onChange("dark")}
            />
          }
        >
          <Moon />
          <span className="sr-only">ダーク</span>
        </TooltipTrigger>
        <TooltipContent>ダーク</TooltipContent>
      </Tooltip>
    </ButtonGroup>
  );
}
