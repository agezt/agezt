// help.ts — the in-app manual. Aggregates the 6 topic sections
// (converse, monitor, agents, automation, knowledge, system)
// into the single HELP record, plus the dispatcher and fallback.
// Day 5b split: each section now lives in its own file under
// app/help/ so the 64-topic dictionary isn't one 2864-line
// god-file anymore. The public API is unchanged — HelpTopic +
// HELP + FALLBACK_TOPIC + helpTopicFor still import from
// @/app/help just as before.

import { Converse } from "./converse";
import { Monitor } from "./monitor";
import { Agents } from "./agents";
import { Automation } from "./automation";
import { Knowledge } from "./knowledge";
import { System } from "./system";
import type { HelpTopic } from "./types";

export type { HelpTopic } from "./types";

export const HELP: Record<string, HelpTopic> = {
  ...Converse,
  ...Monitor,
  ...Agents,
  ...Automation,
  ...Knowledge,
  ...System,
};

/** Fallback topic for a view id with no entry (should be caught by tests). */
export const FALLBACK_TOPIC: HelpTopic = {
  title: "Help",
  intro: "No detailed guide has been written for this page yet.",
  sections: [
    {
      heading: "General navigation",
      paragraphs: [
        "Use the sidebar to move between views, or press ⌘K / Ctrl+K for the command palette. Most pages update live from the daemon's event stream — no manual refreshing needed.",
      ],
    },
  ],
};
export function helpTopicFor(viewId: string): HelpTopic {
  return HELP[viewId] ?? FALLBACK_TOPIC;
}
