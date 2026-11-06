import { useQuery } from "@tanstack/react-query";
import { get } from "@/lib/api";
import { DEMO } from "@/lib/env";
import { useConversations } from "@/features/conversations/store";
import type { ContactRecord } from "@/features/conversations/types";

/** Contatos que a pessoa pode ver (o administrador vê o diretório importado inteiro). */
export function useContacts() {
  const demo = useConversations();
  const query = useQuery({
    queryKey: ["contacts"],
    enabled: !DEMO,
    queryFn: async () => (await get<{ contacts: ContactRecord[] }>("/contacts")).contacts,
    refetchInterval: 10_000,
  });
  const contacts: ContactRecord[] = DEMO
    ? [...new Map((demo.data ?? []).map((c) => [c.contact.id, { ...c.contact, conversationId: c.id }])).values()]
    : (query.data ?? []);
  return { contacts, isPending: DEMO ? demo.isPending : query.isPending, error: DEMO ? demo.error : query.error };
}
