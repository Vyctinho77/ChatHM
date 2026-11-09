import { cn } from "@/lib/cn";

/** Aviso sobre o chat enquanto se arrasta um arquivo. */
export function DropOverlay({ blocked }: { blocked: string | null }) {
  return (
    <div aria-hidden className="pointer-events-none absolute inset-0 z-40 flex items-center justify-center bg-app/80 p-6 backdrop-blur-sm">
      <div className={cn("max-w-sm rounded-2xl border-2 border-dashed px-8 py-6 text-center", blocked ? "border-danger/60" : "border-brand")}>
        <p className="text-[15px] font-semibold">{blocked ? "Não dá para enviar agora" : "Solte para enviar"}</p>
        <p className="mt-1 text-[13px] text-muted">{blocked ?? "Documentos (PDF, Word, Excel…) e fotos. Até 10 arquivos de 25 MB."}</p>
      </div>
    </div>
  );
}
