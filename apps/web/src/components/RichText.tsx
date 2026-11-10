import { useState } from "react";
import { useNavigate } from "react-router";
import { Highlight } from "@/components/Highlight";
import { FormattedText } from "@/components/FormattedText";
import { linkify } from "@/lib/linkify";
import { useContactOpener } from "@/features/conversations/store";

const linkClass = "text-sky-300 underline decoration-sky-300/50 underline-offset-2 hover:decoration-sky-300 focus-visible:decoration-sky-300";

/**
 * Texto com *negrito*, _itálico_ e sites, e-mails e telefones clicáveis (azul): site e e-mail abrem fora do app;
 * telefone abre (ou inicia) a conversa com aquele número no ChatHM. Trechos da busca continuam destacados.
 */
export function RichText({ text, term }: { text: string; term?: string }) {
  const opener = useContactOpener();
  const navigate = useNavigate();
  const [failed, setFailed] = useState("");

  return (
    <>
      <FormattedText text={text} term={term} renderText={(part) => linkify(part).map((p, i) => {
        if (p.type === "text") return <Highlight key={i} text={p.text} term={term} />;
        if (p.type === "phone") {
          return (
            <button
              key={i}
              type="button"
              title="Abrir conversa com este número"
              onClick={(e) => {
                e.stopPropagation();
                setFailed("");
                opener
                  .openPhone(p.digits)
                  .then((c) => navigate(`/conversas/${c.id}`))
                  .catch(() => setFailed(p.digits));
              }}
              className={`${linkClass} inline cursor-pointer p-0 text-left`}
            >
              <Highlight text={p.text} term={term} />
            </button>
          );
        }
        return (
          <a
            key={i}
            href={p.href}
            target={p.type === "url" ? "_blank" : undefined}
            rel="noopener noreferrer"
            onClick={(e) => e.stopPropagation()}
            className={`${linkClass} break-all`}
          >
            <Highlight text={p.text} term={term} />
          </a>
        );
      })} />
      {failed && <span role="alert" className="ml-1 text-xs text-danger">Não foi possível abrir a conversa.</span>}
    </>
  );
}
