// app/help/types.ts — shared types for the in-app manual.
// Day 5b split (see scripts/dev/split-help-topics.py).

export interface HelpItem {
  /** Short bold lead-in — a control, concept, or column on the page. */
  term: string;
  /** What it does / how to use it. */
  desc: string;
}

export interface HelpSection {
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
