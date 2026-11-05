// Rascunhos por conversa: sobrevivem a troca de conversa, abertura do painel de
// contato e navegação entre telas (sessionStorage).
const KEY = "chathm:drafts";

function load(): Record<string, string> {
  try {
    return JSON.parse(sessionStorage.getItem(KEY) ?? "{}") as Record<string, string>;
  } catch {
    return {};
  }
}

const drafts = load();

export function getDraft(id: string) {
  return drafts[id] ?? "";
}

export function setDraft(id: string, text: string) {
  if (text) drafts[id] = text;
  else delete drafts[id];
  try {
    sessionStorage.setItem(KEY, JSON.stringify(drafts));
  } catch {
    /* ignore */
  }
}
