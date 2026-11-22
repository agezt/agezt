import { useEffect, useState } from "react";
import { Loader2 } from "lucide-react";
import { fetchRawBlob, type ArtifactEntry } from "@/lib/artifacts";

// BlobArtifact renders bytes that a plain <img src>/<iframe src> cannot fetch:
// the raw route needs a bearer header, so the blob is fetched then handed to the
// element as an object URL (revoked on unmount).
//
// The component lives in the shared layer because four views render an artifact
// (the gallery, the inbox, the channels view and the file manager) — and the
// domain it draws on lives in `lib/artifacts.ts` beside it (W5.3).
export function BlobArtifact({ entry, kind, alt, title, className }: { entry: ArtifactEntry; kind: "image" | "pdf"; alt?: string; title?: string; className?: string }) {
  const [href, setHref] = useState<string | null>(null);
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    let objectURL = "";
    setHref(null);
    setErr(null);
    fetchRawBlob(entry)
      .then((blob) => {
        if (cancelled) return;
        objectURL = URL.createObjectURL(blob);
        setHref(objectURL);
      })
      .catch((e) => !cancelled && setErr((e as Error).message));
    return () => {
      cancelled = true;
      if (objectURL) URL.revokeObjectURL(objectURL);
    };
  }, [entry]);

  if (err) return <p className="py-6 text-center text-sm text-muted">{err}</p>;
  if (!href) {
    return (
      <p className="flex items-center justify-center gap-2 py-6 text-sm text-muted">
        <Loader2 className="size-4 animate-spin" /> loading preview…
      </p>
    );
  }
  if (kind === "pdf")
    return (
      <iframe
        src={href}
        sandbox=""
        referrerPolicy="no-referrer"
        title={title || entry.name || "pdf"}
        className={className}
      />
    );
  return <img src={href} alt={alt || entry.caption || entry.name || "image"} className={className} loading="lazy" />;
}
