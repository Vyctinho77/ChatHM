/**
 * Abas da página "Atendimento por IA". Só aparece o que tem uso: "Para aprovar" quando há pedidos aguardando (assim, se um assunto
 * voltar para "com aprovação", nada fica preso sem tela) e "Fila e histórico" quando há fila, envio de hoje ou falha para acompanhar.
 * Cobranças saiu da tela: o Asaas é consultado pela própria IA, fora do ChatHM.
 */
export const TABS = [["aprovar", "Para aprovar"], ["fila", "Fila e histórico"], ["assuntos", "Assuntos"], ["raias", "Raias"], ["controles", "Controles"], ["docs", "Documentos"]] as const;
export type Tab = (typeof TABS)[number][0];

export type Counts = { pending: number; queued: number; sentToday: number; failed: number };

/** Abas que aparecem agora. Sem o resumo (carregando ou com erro) só as fixas aparecem, para a tela não "piscar" abas. */
export function visibleTabs(o?: Counts): (typeof TABS)[number][] {
  const active = !!o && o.pending + o.queued + o.sentToday + o.failed > 0;
  return TABS.filter(([k]) => (k === "aprovar" ? !!o && o.pending > 0 : k === "fila" ? active : true));
}

/** A aba a mostrar: a escolhida, se ainda existir; senão a de aprovação (se houver pedidos) ou "Assuntos". */
export function currentTab(chosen: Tab | null, o?: Counts): Tab {
  const list = visibleTabs(o);
  if (chosen && list.some(([k]) => k === chosen)) return chosen;
  return o && o.pending > 0 ? "aprovar" : "assuntos";
}
