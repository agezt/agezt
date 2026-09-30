// app/help/types.ts — shared types for the in-app manual.
// Day 5b split (see scripts/dev/split-help-topics.py).

// HelpItem and HelpSection are used by HelpTopic below, not imported from here,
// so they stay package-private. Exporting them made them look like public API
// that nothing consumed.
interface HelpItem {
  /** Short bold lead-in — a control, concept, or column on the page. */
  term: string;
  /** What it does / how to use it. */
  desc: string;
}

interface HelpSection {
  heading: string;
  paragraphs?: string[];
  items?: HelpItem[];
}

export interface HelpTopic {
  title: string;
  /** One- or two-sentence orientation: what this page is for. */
  intro: string;
  sections: HelpSection[];
  /** Practical "did you know" pointers, rendered as callouts. */
  tips?: string[];
  /** Other views that complete the story; chips navigate there. */
  related?: { id: string; label: string }[];
}
