import * as React from "react";
import * as TabsPrimitive from "@radix-ui/react-tabs";
import { cn } from "@/lib/utils";

// TabNav — a proper icon+label tab system. Uses Radix UI Tabs under the hood.
// Each tab has an icon, label, optional count badge, and a pill active indicator.
// Tabs are visually compact and scannable — designed to replace ad-hoc chip-rows
// and basic text-button toggles throughout the UI.
export function TabNav({
  tabs,
  value,
  onValueChange,
  className,
}: {
  tabs: TabDef[];
  value?: string;
  onValueChange?: (value: string) => void;
  className?: string;
}) {
  // When used uncontrolled (no `value`/`onValueChange`), default to the first
  // tab so content is visible — a Radix Tabs.Root with neither value nor
  // defaultValue selects nothing and renders an empty panel.
  const uncontrolled = value === undefined;
  return (
    <TabsPrimitive.Root
      value={value}
      defaultValue={uncontrolled ? tabs[0]?.id : undefined}
      onValueChange={onValueChange}
      className={cn("flex flex-col gap-3", className)}
    >
      <TabsPrimitive.List
        className={cn(
          "flex flex-wrap items-center gap-1 rounded-xl border border-border bg-panel/50 p-1",
        )}
        aria-label="View tabs"
      >
        {tabs.map((tab) => (
          <TabTrigger key={tab.id} tab={tab} value={value} />
        ))}
      </TabsPrimitive.List>
      {tabs.map((tab) => (
        <TabsPrimitive.Content
          key={tab.id}
          value={tab.id}
          className="min-h-0 flex-1 animate-in fade-in-0 slide-in-from-top-1 duration-200"
        >
          {tab.content}
        </TabsPrimitive.Content>
      ))}
    </TabsPrimitive.Root>
  );
}

function TabTrigger({
  tab,
  value,
}: {
  tab: TabDef;
  value?: string;
}) {
  const isActive = value === tab.id;
  return (
    <TabsPrimitive.Trigger
      value={tab.id}
      className={cn(
        "inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-xs font-medium",
        "transition-all duration-150",
        "outline-none focus-visible:ring-2 focus-visible:ring-accent/50",
        "data-[state=active]:bg-gradient-to-br data-[state=active]:from-accent/20 data-[state=active]:to-accent2/10 data-[state=active]:text-accent",
        "data-[state=active]:shadow-sm data-[state=active]:ring-1 data-[state=active]:ring-inset data-[state=active]:ring-accent/25",
        "data-[state=inactive]:text-muted data-[state=inactive]:hover:text-foreground",
      )}
    >
      {tab.icon && <tab.icon className="size-3.5" aria-hidden />}
      {tab.label}
      {tab.count !== undefined && (
        <span
          className={cn(
            "inline-flex min-w-4 items-center justify-center rounded-full px-1 text-xs tabular-nums",
            isActive
              ? "bg-accent/20 text-accent"
              : "bg-panel text-muted",
          )}
        >
          {tab.count}
        </span>
      )}
    </TabsPrimitive.Trigger>
  );
}

export type TabDef = {
  id: string;
  label: string;
  icon?: React.ComponentType<{ className?: string }>;
  count?: number | string;
  content: React.ReactNode;
};
