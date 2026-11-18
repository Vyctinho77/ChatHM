/** Raias da IA (docs/mcp-raia.md): tipos e regras puras da tela. As regras de verdade são do servidor; aqui só se descreve e se confere o formulário. */

export type Lane = {
  key: string; label: string; enabled: boolean; mode: "shadow" | "live"; canInitiate: boolean;
  segmentId: string; handoffSegmentId: string;
  maxTurns: number; maxConsecutive: number; contactDaily: number; humanQuietMinutes: number; staleHours: number;
  requireSources: boolean; allowedDomains: string[]; guidelines: string; updatedAt: string;
};
export type LaneDraft = {
  id: string; lane: string; conversationId: string; contactId: string; contactName: string; body: string; createdAt: string;
  outcome?: "approved" | "edited" | "discarded" | ""; reviewedAt?: string; provider: string; agent: string; finalBody?: string;
};
export type Handoff = { id: string; conversationId: string; lane: string; reason: string; summary: string; source: "ai" | "server"; createdAt: string; answered: boolean };

export type LaneState = "off" | "shadow" | "live";
export const laneState = (l: Pick<Lane, "enabled" | "mode">): LaneState => (!l.enabled ? "off" : l.mode === "live" ? "live" : "shadow");

export const STATE_LABEL: Record<LaneState, string> = { off: "Desligada", shadow: "Em sombra", live: "Valendo" };
export const STATE_HINT: Record<LaneState, string> = {
  off: "A IA não envia nada por esta raia.",
  shadow: "A IA escreve, o ChatHM guarda o rascunho e nada vai ao cliente.",
  live: "A IA envia de verdade aos clientes.",
};

/** Só pede confirmação quando a mudança faz a IA passar a enviar de verdade. */
export const needsLiveConfirm = (before: Pick<Lane, "enabled" | "mode">, after: Pick<Lane, "enabled" | "mode">) => laneState(after) === "live" && laneState(before) !== "live";

/** "gov.br, asaas.com" → ["gov.br", "asaas.com"] (minúsculas, sem repetição). */
export function parseDomains(text: string): string[] {
  const out: string[] = [];
  for (const d of text.split(/[\s,;]+/)) {
    const v = d.trim().toLowerCase();
    if (v && !out.includes(v)) out.push(v);
  }
  return out;
}

const HOST = /^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$/;
export type LaneErrors = Partial<Record<"label" | "maxTurns" | "maxConsecutive" | "contactDaily" | "humanQuietMinutes" | "staleHours" | "allowedDomains" | "guidelines", string>>;

/** Os mesmos limites do servidor, para avisar antes de salvar. */
export function validateLane(l: Lane): LaneErrors {
  const e: LaneErrors = {};
  const range = (k: "maxTurns" | "maxConsecutive" | "contactDaily" | "humanQuietMinutes" | "staleHours", min: number, max: number, msg: string) => {
    const v = l[k];
    if (!Number.isInteger(v) || v < min || v > max) e[k] = msg;
  };
  if (!l.label.trim() || l.label.trim().length > 60) e.label = "O nome deve ter de 1 a 60 caracteres.";
  range("maxTurns", 1, 50, "De 1 a 50 rodadas.");
  range("maxConsecutive", 1, 10, "De 1 a 10 mensagens.");
  range("contactDaily", 1, 50, "De 1 a 50 mensagens.");
  range("humanQuietMinutes", 0, 1440, "De 0 a 1440 minutos.");
  range("staleHours", 0, 720, "De 0 a 720 horas.");
  const bad = l.allowedDomains.find((d) => !HOST.test(d));
  if (bad) e.allowedDomains = `Use só o domínio, como asaas.com (sem http:// nem caminho): ${bad}`;
  if (l.guidelines.length > 6000) e.guidelines = "No máximo 6000 caracteres.";
  return e;
}

/** Resumo de uma linha do que a raia faz hoje. */
export function laneSummary(l: Lane, segmentName?: string): string {
  const parts = [laneState(l) === "off" ? "Desligada" : l.mode === "live" ? "Valendo" : "Em sombra"];
  parts.push(l.canInitiate ? "inicia conversas" : "só responde");
  parts.push(segmentName ? `fila ${segmentName}` : "qualquer fila");
  if (l.staleHours > 0) parts.push(`conversas paradas há ${l.staleHours} h`);
  return parts.join(" · ");
}

const REASON: Record<string, string> = {
  documento: "Cliente mandou foto ou documento", pediu_humano: "Cliente pediu uma pessoa ou reclamou", turnos: "Limite de rodadas da IA",
  dados: "Dados do cliente", reclamacao: "Reclamação", preco: "Preço", pagamento: "Pagamento", juridico: "Assunto jurídico", duvida: "A IA ficou em dúvida", outro: "Outro motivo",
};
export const reasonLabel = (r: string) => REASON[r] ?? (r ? r.charAt(0).toUpperCase() + r.slice(1).replace(/_/g, " ") : "Outro motivo");

export const MAX_DRAFT_CHARS = 1500; // limite do servidor para uma mensagem do atendente

/** Rascunho com mais de 1 dia: a conversa pode ter mudado, a tela pede para conferir antes de enviar. */
export const isOldDraft = (createdAt: string, now = Date.now()) => now - Date.parse(createdAt) > 24 * 3600 * 1000;

export const OUTCOME_LABEL: Record<string, string> = { approved: "Enviado como a IA escreveu", edited: "Enviado com edição", discarded: "Descartado" };

/** O texto editado pode ser enviado? (não vazio, dentro do limite e diferente de espaços) */
export const draftTextError = (text: string): string => {
  const t = text.trim();
  if (!t) return "Escreva a mensagem.";
  if (t.length > MAX_DRAFT_CHARS) return `No máximo ${MAX_DRAFT_CHARS} caracteres (${t.length}).`;
  return "";
};
