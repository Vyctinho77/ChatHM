import { useQuery, useQueryClient } from "@tanstack/react-query";
import { get, patch } from "@/lib/api";
import { DEMO } from "@/lib/env";
import { useConversationActions, useConversations } from "@/features/conversations/store";
import { conversationKeys } from "@/features/conversations/store.api";
import type { ContactRecord } from "@/features/conversations/types";

/**
 * Contatos que a pessoa pode ver (o administrador vê o diretório importado inteiro). A lista é grande (centenas de contatos):
 * `enabled: false` não a busca, e ela só é atualizada de tempos em tempos enquanto alguém está usando.
 */
export function useContacts({ enabled = true }: { enabled?: boolean } = {}) {
  const demo = useConversations();
  const query = useQuery({
    queryKey: ["contacts"],
    enabled: !DEMO && enabled,
    queryFn: async () => (await get<{ contacts: ContactRecord[] }>("/contacts")).contacts,
    staleTime: 30_000,
    refetchInterval: 60_000, // só roda enquanto a consulta está ativa; mudanças grandes chegam por WebSocket (useRealtime)
  });
  const contacts: ContactRecord[] = DEMO
    ? [...new Map((demo.data ?? []).map((c) => [c.contact.id, { ...c.contact, conversationId: c.id }])).values()]
    : (query.data ?? []);
  return { contacts, isPending: DEMO ? demo.isPending : query.isPending, error: DEMO ? demo.error : query.error };
}

/** Renomeia um contato (nome vazio volta a seguir o nome do WhatsApp). */
export function useRenameContact() {
  const qc = useQueryClient();
  const actions = useConversationActions();
  const convs = useConversations();
  return async (contact: ContactRecord, name: string) => {
    if (DEMO) {
      const conv = (convs.data ?? []).find((c) => c.contact.id === contact.id);
      if (conv) await actions.setName(conv.id, name);
      return;
    }
    await patch(`/contacts/${contact.id}`, { name });
    await Promise.all([qc.invalidateQueries({ queryKey: ["contacts"] }), qc.invalidateQueries({ queryKey: conversationKeys.list })]);
  };
}
