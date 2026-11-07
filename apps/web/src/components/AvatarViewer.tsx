import { useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { X } from "lucide-react";
import { Avatar } from "@/components/Avatar";

/** URL da versão em tamanho grande da foto (o servidor busca no WhatsApp; sem ela, devolve a miniatura). */
const fullAvatarUrl = (url: string) => `${url}${url.includes("?") ? "&" : "?"}full=1`;

/** Foto de perfil em tela cheia, como no WhatsApp: nome no canto, fechar com X, Esc ou clique fora. */
export function AvatarViewer({ name, src, onClose }: { name: string; src: string; onClose: () => void }) {
  const [loaded, setLoaded] = useState(false);
  const closeRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", onKey);
    closeRef.current?.focus();
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  return createPortal(
    <div
      role="dialog"
      aria-modal="true"
      aria-label={`Foto de ${name}`}
      className="fixed inset-0 z-[70] flex flex-col bg-black/92 text-white"
      onClick={onClose}
    >
      <header className="flex h-16 shrink-0 items-center gap-3 px-4" onClick={(e) => e.stopPropagation()}>
        <Avatar name={name} src={src} size="md" />
        <p className="min-w-0 flex-1 truncate text-[15px] font-medium">{name}</p>
        <button
          ref={closeRef}
          type="button"
          aria-label="Fechar"
          onClick={onClose}
          className="inline-flex size-10 items-center justify-center rounded-full hover:bg-white/10"
        >
          <X className="size-6" />
        </button>
      </header>
      <div className="relative flex min-h-0 flex-1 items-center justify-center p-4 pb-8">
        {/* A miniatura aparece de imediato; a foto grande a substitui ao carregar. */}
        <img src={src} alt="" aria-hidden className="absolute max-h-full max-w-full object-contain blur-sm" style={{ opacity: loaded ? 0 : 1 }} />
        <img
          src={fullAvatarUrl(src)}
          alt={`Foto de perfil de ${name}`}
          onLoad={() => setLoaded(true)}
          onClick={(e) => e.stopPropagation()}
          className="relative max-h-full max-w-full object-contain transition-opacity"
          style={{ opacity: loaded ? 1 : 0 }}
        />
      </div>
    </div>,
    document.body,
  );
}

/** Avatar que abre a foto em tela cheia ao clicar (só quando há foto). */
export function ViewableAvatar({
  name,
  src,
  group,
  size,
  className,
}: {
  name: string;
  src?: string;
  group?: boolean;
  size?: "sm" | "list" | "md" | "lg";
  className?: string;
}) {
  const [open, setOpen] = useState(false);
  if (!src) return <Avatar name={name} group={group} size={size} className={className} />;
  return (
    <>
      <button
        type="button"
        aria-label={`Ver foto de ${name}`}
        onClick={(e) => (e.stopPropagation(), setOpen(true))}
        className="shrink-0 cursor-zoom-in rounded-full transition-opacity hover:opacity-90 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-ink"
      >
        <Avatar name={name} src={src} group={group} size={size} className={className} />
      </button>
      {open && <AvatarViewer name={name} src={src} onClose={() => setOpen(false)} />}
    </>
  );
}
