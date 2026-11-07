import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Bot, Pencil, SendHorizontal, X } from "lucide-react";
import { Button } from "@/components/ui";
import { errorMessage, get, post } from "@/lib/api";
import { DEMO } from "@/lib/env";
import { conversationKeys } from "./store.api";

export type AiSuggestion = { id: string; conversationId: string; intent: "draft" | "send"; text: string; replyTo?: string; createdAt: string; expiresAt: string };
const suggestionsKey = (conversationId: string) => ["ai", "suggestions", conversationId] as const;

/**
 * Rascunhos e pedidos de envio deixados pela IA (MCP) para esta conversa. A IA nunca envia: a pessoa revisa e
 * aprova aqui (Enviar), leva o texto para o campo e edita (Editar) ou descarta.
 */
export function AiSuggestions({
  conversationId,
  canSend,
  sendBlockedReason,
  onUse,
}: {
  conversationId: string;
  canSend: boolean;
  sendBlockedReason?: string;
  /** Coloca o texto no campo de mensagem para a pessoa editar. */
  onUse: (text: string) => void;
}) {
  const qc = useQueryClient();
  const [error, setError] = useState("");
  const { data } = useQuery({
    queryKey: suggestionsKey(conversationId),
    enabled: !DEMO,
    queryFn: async () => (await get<{ suggestions: AiSuggestion[] }>(`/ai/suggestions?conversationId=${conversationId}`)).suggestions,
  });
  const refresh = () => {
    void qc.invalidateQueries({ queryKey: suggestionsKey(conversationId) });
    void qc.invalidateQueries({ queryKey: conversationKeys.messages(conversationId) });
    void qc.invalidateQueries({ queryKey: conversationKeys.list });
  };
  const send = useMutation({
    mutationFn: (id: string) => post(`/ai/suggestions/${id}/send`, {}),
    onSuccess: refresh,
    onError: (e) => setError(errorMessage(e)),
  });
  const use = useMutation({
    mutationFn: (id: string) => post<AiSuggestion>(`/ai/suggestions/${id}/use`),
    onSuccess: (s) => {
      refresh();
      onUse(s.text);
    },
    onError: (e) => setError(errorMessage(e)),
  });
  const discard = useMutation({
    mutationFn: (id: string) => post<void>(`/ai/suggestions/${id}/discard`),
    onSuccess: refresh,
    onError: (e) => setError(errorMessage(e)),
  });

  if (!data?.length) return null;
  const busy = send.isPending || use.isPending || discard.isPending;
  return (
    <div className="px-3 pt-2 sm:px-4" aria-live="polite">
      {data.map((s) => (
        <section key={s.id} aria-label="Sugestão da IA" className="mb-1.5 rounded-2xl border border-line bg-panel p-3 shadow-sm">
          <p className="flex items-center gap-1.5 text-xs font-semibold text-brand-ink">
            <Bot className="size-4" /> {s.intent === "send" ? "A IA pediu para enviar esta mensagem" : "Rascunho da IA"}
            <span className="font-normal text-muted">· nada foi enviado ao cliente</span>
          </p>
          <p className="mt-1.5 max-h-40 overflow-y-auto whitespace-pre-wrap break-words text-[13.5px]">{s.text}</p>
          {error && <p role="alert" className="mt-1.5 text-xs text-danger">{error}</p>}
          <div className="mt-2 flex flex-wrap items-center justify-end gap-1.5">
            <Button type="button" variant="ghost" disabled={busy} onClick={() => (setError(""), discard.mutate(s.id))}>
              <X className="size-4" /> Descartar
            </Button>
            <Button type="button" variant="secondary" disabled={busy} onClick={() => (setError(""), use.mutate(s.id))}>
              <Pencil className="size-4" /> Editar no campo
            </Button>
            <Button type="button" loading={send.isPending && send.variables === s.id} disabled={busy || !canSend} title={canSend ? undefined : sendBlockedReason} onClick={() => (setError(""), send.mutate(s.id))}>
              <SendHorizontal className="size-4" /> Enviar
            </Button>
          </div>
        </section>
      ))}
    </div>
  );
}
