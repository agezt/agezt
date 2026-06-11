import { Fragment, useState } from "react";
import { Check, Copy } from "lucide-react";
import { parseInline, parseMarkdown, type Inline } from "@/lib/markdown";
import { DataView } from "@/components/DataView";

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
            return <CodeBlock key={i} code={b.v} lang={b.lang} />;
          case "json":
            return <DataView key={i} data={b.data} />;
          case "table":
            return (
              <div key={i} className="my-2 overflow-x-auto rounded-lg border border-border">
                <table className="w-full border-collapse text-xs">
                  <thead>
                    <tr className="bg-panel/70">
                      {b.header.map((h, j) => (
                        <th
                          key={j}
                          className="border-b border-border px-2.5 py-1.5 text-left font-semibold text-foreground"
                        >
                          {renderInline(parseInline(h))}
                        </th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {b.rows.map((row, r) => (
                      <tr key={r} className={r % 2 ? "bg-panel/20" : ""}>
                        {row.map((c, j) => (
                          <td key={j} className="border-t border-border px-2.5 py-1.5 align-top">
                            {renderInline(parseInline(c))}
                          </td>
                        ))}
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            );
          case "quote":
            return (
              <blockquote
                key={i}
                className="my-2 border-l-2 border-accent/50 pl-3 italic text-muted whitespace-pre-wrap break-words"
              >
                {renderInline(parseInline(b.v))}
              </blockquote>
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

// CodeBlock renders a fenced block with a hover Copy button — the snippet/command
// an agent hands you is usually the thing you want to grab, so make it one click.
function CodeBlock({ code, lang }: { code: string; lang: string }) {
  const [copied, setCopied] = useState(false);
  async function copy() {
    try {
      await navigator.clipboard.writeText(code);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1200);
    } catch {
      /* clipboard unavailable (non-secure context) — silently no-op */
    }
  }
  return (
    <div className="group relative my-2">
      <pre className="overflow-auto rounded-md border border-border bg-panel/70 p-2.5 font-mono text-[12px] leading-snug">
        <code>{code}</code>
      </pre>
      {lang && (
        <span className="pointer-events-none absolute left-2 top-1 select-none text-[10px] uppercase tracking-wide text-muted/60">
          {lang}
        </span>
      )}
      <button
        onClick={copy}
        title={copied ? "Copied" : "Copy"}
        className="absolute right-1.5 top-1.5 inline-flex items-center gap-1 rounded border border-border bg-card px-1.5 py-0.5 text-[11px] text-muted opacity-0 transition-opacity hover:text-foreground focus-visible:opacity-100 group-hover:opacity-100"
      >
        {copied ? <Check className="size-3 text-good" /> : <Copy className="size-3" />}
        {copied ? "Copied" : "Copy"}
      </button>
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
      case "del":
        return <del key={i} className="text-muted">{tok.v}</del>;
      case "link":
        return (
          <a
            key={i}
            href={tok.href}
            target="_blank"
            rel="noopener noreferrer nofollow"
            className="text-accent underline underline-offset-2 hover:opacity-80"
          >
            {tok.v}
          </a>
        );
      default:
        return <Fragment key={i}>{tok.v}</Fragment>;
    }
  });
}
