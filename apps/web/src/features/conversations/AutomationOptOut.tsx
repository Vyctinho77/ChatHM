import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, get } from "@/lib/api";
import { DEMO } from "@/lib/env";
import { useMe } from "@/features/auth/api";

type State = { optOut: boolean; optOutAt?: string };

/** "Não enviar avisos automáticos": o cliente pediu para parar (PARAR) ou a equipe marcou. Só administradores e gestores. */
export function AutomationOptOut({ contactId }: { contactId: string }) {
  const { data: me } = useMe();
  const allowed = !DEMO && (me?.user.role === "admin" || me?.user.role === "manager");
  const qc = useQueryClient();
  const key = ["automation", "contact", contactId];
  const { data } = useQuery({ queryKey: key, enabled: allowed, queryFn: () => get<State>(`/automation/contacts/${contactId}`) });
  const save = useMutation({
    mutationFn: (optOut: boolean) => api<void>("PUT", `/automation/contacts/${contactId}/opt-out`, { optOut }),
    onSuccess: () => void qc.invalidateQueries({ queryKey: key }),
  });
  if (!allowed || !data) return null;
  return (
    <section className="border-b border-line px-5 py-4">
      <label className="flex cursor-pointer items-center justify-between gap-4">
        <span className="min-w-0">
          <span className="block text-[13px] font-medium">Não enviar avisos automáticos</span>
          <span className="mt-0.5 block text-xs text-muted">
            {data.optOut ? "A IA não inclui este cliente em envios automáticos." : "A IA pode propor avisos (DAS, cobrança, prazos) a este cliente."}
          </span>
        </span>
        <input type="checkbox" role="switch" aria-label="Não enviar avisos automáticos" checked={data.optOut} disabled={save.isPending} onChange={(e) => save.mutate(e.target.checked)} className="size-4 shrink-0 accent-[var(--c-accent)]" />
      </label>
    </section>
  );
}
