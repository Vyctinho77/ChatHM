// "Há uma nova versão": o index.html do servidor aponta para o pacote atual (assets/index-HASH.js). Se for diferente do que esta
// aba carregou, a aba está com uma versão antiga: quem deixa o app aberto o dia todo continuaria nela até recarregar.
const BUNDLE = /\/assets\/index-[A-Za-z0-9_-]+\.js/;

/** O pacote principal citado por um index.html (null se não achar). */
export function bundleFromHtml(html: string): string | null {
  return BUNDLE.exec(html)?.[0] ?? null;
}

/** O pacote que esta aba está rodando (null em desenvolvimento, onde não há hash). */
export function runningBundle(doc: Pick<Document, "querySelectorAll"> = document): string | null {
  for (const s of Array.from(doc.querySelectorAll<HTMLScriptElement>('script[type="module"][src]'))) {
    const src = s.getAttribute("src") ?? "";
    const m = BUNDLE.exec(src);
    if (m) return m[0];
  }
  return null;
}

/** Avisa quando o servidor tem um pacote diferente do que roda aqui, a não ser que a pessoa já tenha dispensado justamente esse. */
export function shouldAnnounce(running: string | null, latest: string | null, dismissed: string | null): boolean {
  return !!running && !!latest && running !== latest && latest !== dismissed;
}
