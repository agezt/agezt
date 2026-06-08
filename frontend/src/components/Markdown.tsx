import { Fragment } from "react";
import { parseInline, parseMarkdown, type Inline } from "@/lib/markdown";

// Markdown renders an agent answer from the tiny AST in lib/markdown. Every leaf
// is plain React text (React escapes it) — there is no raw-HTML path — so it is
// safe to render model output directly under the strict CSP.
export function Markdown({ source, className }: { source: string; className?: string }) {
  const blocks = parseMarkdown(source);
  return (
    <div className={className}>
      {blocks.map((b, i) => {
        switch (b.t) {
          case "code":
            return (
              <pre
                key={i}
                className="my-2 overflow-auto rounded-md border border-border bg-panel/70 p-2.5 font-mono text-[12px] leading-snug"
              >
                <code>{b.v}</code>
              </pre>
            );
          case "ul":
            return (
              <ul key={i} className="my-1.5 list-disc space-y-0.5 pl-5">
                {b.items.map((it, j) => (
                  <li key={j}>{renderInline(parseInline(it))}</li>
                ))}
              </ul>
            );
          case "ol":
            return (
              <ol key={i} className="my-1.5 list-decimal space-y-0.5 pl-5">
                {b.items.map((it, j) => (
                  <li key={j}>{renderInline(parseInline(it))}</li>
                ))}
              </ol>
            );
          case "h": {
            const size = b.level <= 1 ? "text-base" : b.level === 2 ? "text-sm" : "text-sm";
            return (
              <p key={i} className={`mt-2 mb-1 font-semibold ${size}`}>
                {renderInline(parseInline(b.v))}
              </p>
            );
          }
          default:
            return (
              <p key={i} className="my-1.5 whitespace-pre-wrap break-words leading-relaxed">
                {renderInline(parseInline(b.v))}
              </p>
            );
        }
      })}
    </div>
  );
}

function renderInline(tokens: Inline[]) {
  return tokens.map((tok, i) => {
    switch (tok.t) {
      case "code":
        return (
          <code key={i} className="rounded bg-panel/70 px-1 py-0.5 font-mono text-[0.85em]">
            {tok.v}
          </code>
        );
      case "strong":
        return <strong key={i} className="font-semibold">{tok.v}</strong>;
      case "em":
        return <em key={i}>{tok.v}</em>;
      default:
        return <Fragment key={i}>{tok.v}</Fragment>;
    }
  });
}
