import { Link } from "react-router";
import { Lock, MessageSquareText } from "lucide-react";
import { ApiError } from "@/lib/api";
import { contactTitle, useSharedConversation, type SharedConversation } from "./api";

const statusLabel = (c: SharedConversation) =>
  c.status === "closed" ? "Atendimento encerrado" : c.status === "new" ? "Aguardando atendimento" : c.assignee ? `Em atendimento por ${c.assignee}` : "Em atendimento";

/**
 * Cartão de uma conversa de cliente no chat interno. A mensagem guarda só o id: o contato é pedido ao servidor para
 * quem está olhando, e quem não pode ver aquela conversa não descobre quem é o cliente (nem que ela existe).
 */
export function SharedConversationCard({ id }: { id: string }) {
  const q = useSharedConversation(id);
  const box = "flex w-64 max-w-full items-center gap-2.5 rounded-md border border-black/10 bg-black/[.06] p-2 text-left";

  if (q.isPending) {
    return <div className={box} aria-busy="true"><MessageSquareText className="size-5 shrink-0 opacity-60" /><span className="text-[12.5px] opacity-70">Carregando conversa…</span></div>;
  }
  if (q.isError) {
    const denied = q.error instanceof ApiError && q.error.status === 404;
    return (
      <div className={box}>
        <Lock className="size-5 shrink-0 opacity-60" aria-hidden />
        <span className="min-w-0 text-[12.5px]">
          <span className="block font-semibold">Conversa de um cliente</span>
          <span className="block opacity-75">{denied ? "Você não tem acesso a esta conversa." : "Não foi possível carregar."}</span>
          {!denied && <button type="button" onClick={() => void q.refetch()} className="font-medium underline-offset-2 hover:underline">Tentar de novo</button>}
        </span>
      </div>
    );
  }
  const c = q.data;
  const open = c.status !== "closed"; // conversa encerrada some das listas: não há para onde abrir
  const body = (
    <>
      <MessageSquareText className="size-5 shrink-0 opacity-70" aria-hidden />
      <span className="min-w-0 flex-1 text-[12.5px]">
        <span className="block truncate font-semibold">{contactTitle(c.contact)}</span>
        <span className="block truncate opacity-75">{statusLabel(c)}</span>
        {open && <span className="block font-medium underline-offset-2 group-hover/card:underline">Abrir conversa</span>}
      </span>
    </>
  );
  return open ? (
    <Link to={`/conversas/${c.id}`} aria-label={`Abrir a conversa de ${contactTitle(c.contact)}`} className={`${box} group/card hover:bg-black/10`}>{body}</Link>
  ) : (
    <div className={box}>{body}</div>
  );
}
