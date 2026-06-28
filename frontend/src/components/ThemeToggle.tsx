import { Moon, Sun } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useTheme } from "@/lib/theme";
import { cn } from "@/lib/utils";

export function ThemeToggle() {
  const { theme, toggle } = useTheme();
  return (
    <Button variant="ghost" size="icon" onClick={toggle} title={`Switch to ${theme === "dark" ? "light" : "dark"} mode`} aria-label="Toggle theme">
      <span className="transition-transform duration-500 ease-in-out [transform:rotate(0deg)] dark:[transform:rotate(360deg)]">
        {theme === "dark" ? <Sun className="size-4" /> : <Moon className="size-4" />}
      </span>
    </Button>
  );
}
