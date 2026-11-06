import { highlightParts } from "@/lib/search";

/** Texto com as ocorrências da busca em destaque (sem acento/maiúscula). */
export function Highlight({ text, term }: { text: string; term?: string }) {
  if (!term?.trim()) return <>{text}</>;
  return (
    <>
      {highlightParts(text, term).map((p, i) =>
        p.hit ? (
          <mark key={i} className="rounded-sm bg-yellow-300/70 px-px text-inherit dark:bg-yellow-400/40">
            {p.text}
          </mark>
        ) : (
          <span key={i}>{p.text}</span>
        ),
      )}
    </>
  );
}
