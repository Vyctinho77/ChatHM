/**
 * Corretor de ERROS DE DIGITAÇÃO do campo de mensagem (interno, sem tela). Só corrige com muita confiança: a palavra digitada não é palavra,
 * e existe UMA palavra frequente a um destes enganos, os mais comuns de quem digita rápido:
 *   tr   duas letras vizinhas trocadas        (voecê → você)           a partir de 4 letras
 *   ins  faltou uma letra                     (obrigdo → obrigado)     a partir de 5
 *   dup  letra repetida sem querer            (obrigaddo → obrigado)   a partir de 5
 *   key  letra no lugar da tecla vizinha      (obrigavo → obrigado)    a partir de 6
 * Fora disso (palavras raras, nomes, siglas, gírias, estrangeirismos) não mexe: o navegador sublinha. É o mesmo algoritmo de
 * scripts/gen-pt-typos.py (typos.fixture.json garante que as duas implementações concordam); ver docs/corretor.md.
 *
 * Dados (carregados sob demanda, em segundo plano): ptWords.data.ts (~30 mil palavras frequentes) e ptTypoExceptions.data.ts
 * (~26 mil palavras raras VÁLIDAS que o algoritmo trocaria por engano; nunca são corrigidas).
 */

const ALPHA = "abcdefghijklmnopqrstuvwxyz";
const MIN = { tr: 4, ins: 5, dup: 5, key: 6 } as const;
type Kind = keyof typeof MIN;
const DOMINANCE = 20; // o melhor candidato precisa ser 20x mais frequente que o segundo
const MIN_WORD = 4;

/** Teclado ABNT2/QWERTY: a tecla j encosta nas j-1 e j da fileira de baixo. */
const NEIGH: ReadonlyMap<string, ReadonlySet<string>> = (() => {
  const rows = ["qwertyuiop", "asdfghjkl", "zxcvbnm"];
  const map = new Map<string, Set<string>>();
  const link = (a: string, b: string) => {
    if (!map.has(a)) map.set(a, new Set());
    map.get(a)!.add(b);
  };
  rows.forEach((row, r) => {
    [...row].forEach((ch, j) => {
      if (j > 0) link(ch, row[j - 1]!);
      if (j + 1 < row.length) link(ch, row[j + 1]!);
      const below = rows[r + 1];
      if (below) for (const k of [j - 1, j]) if (k >= 0 && k < below.length) (link(ch, below[k]!), link(below[k]!, ch));
    });
  });
  return map;
})();

/** Risadas e interjeições de chat: nunca são "erros". */
const LAUGH = /^(k+|(rs)+|(ha)+h?|(he)+h?|(hi)+|(ah)+h?|(eh)+|(oh)+|(ue)+|(u+h)|(a+h+))$/;

/** Siglas e termos do dia a dia de um escritório de contabilidade e de certificação digital: não são erros. */
const DOMAIN = new Set(
  `cnpj cnpjs nfse nfce icms simples presumido esocial fgts caged cnae cfop darf dctf sped irpj irpf csll cofins whatsapp asaas boleto pdfs xmls token
certificado certificados contabil contabeis fiscal fiscais tributo tributos folha holerite pronampe sebrae anvisa receita federal serpro renavam`
    .split(/\s+/).filter(Boolean),
);

export const fold = (w: string) => w.normalize("NFD").replace(/\p{M}/gu, "");

type Dict = { variants: Map<string, string[]>; rank: Map<string, number>; exceptions: ReadonlySet<string> };
let dict: Dict | null = null;
let loading: Promise<void> | null = null;

/** `words` em ordem de frequência (acentuadas); `exceptions` sem acento. */
export function setTypoDictionary(words: readonly string[] | null, exceptions: Iterable<string> = []) {
  if (!words) return void (dict = null);
  const variants = new Map<string, string[]>();
  const rank = new Map<string, number>();
  words.forEach((w, i) => {
    rank.set(w, i);
    const f = fold(w);
    const list = variants.get(f);
    if (list) list.push(w);
    else variants.set(f, [w]);
  });
  dict = { variants, rank, exceptions: new Set(exceptions) };
}
export const isTypoDictionaryLoaded = () => dict !== null;

/** Carrega os dados (dois arquivos separados, ~150 KB comprimidos no total). Chamar de novo não baixa de novo. */
export function loadTypoDictionary(): Promise<void> {
  loading ??= Promise.all([import("./ptWords.data"), import("./ptTypoExceptions.data")])
    .then(([w, e]) => setTypoDictionary(w.PT_WORDS.split(" "), e.PT_TYPO_EXCEPTIONS.split(" ")))
    .catch(() => {
      loading = null; // sem rede agora: tenta de novo na próxima vez
    });
  return loading;
}

function candidates(f: string, d: Dict): Set<string> {
  const found = new Set<string>();
  const n = f.length;
  const take = (e: string, kind: Kind) => {
    if (n >= MIN[kind] && d.variants.has(e)) found.add(e);
  };
  for (let i = 0; i < n - 1; i++) if (f[i] !== f[i + 1]) take(f.slice(0, i) + f[i + 1] + f[i] + f.slice(i + 2), "tr");
  for (let i = 1; i <= n; i++) for (const c of ALPHA) take(f.slice(0, i) + c + f.slice(i), "ins");
  for (let i = 1; i < n; i++) if (f[i] === f[i - 1]) take(f.slice(0, i) + f.slice(i + 1), "dup");
  for (let i = 0; i < n; i++) for (const c of NEIGH.get(f[i]!) ?? []) take(f.slice(0, i) + c + f.slice(i + 1), "key");
  return found;
}

/** O melhor candidato (acentuado) para a palavra digitada em minúsculas, sem acento; null = não mexe. */
export function fixFolded(f: string): string | null {
  const d = dict;
  if (!d || !/^[a-z]+$/.test(f) || f.length < MIN_WORD || LAUGH.test(f) || DOMAIN.has(f) || d.variants.has(f) || d.exceptions.has(f)) return null;
  const found = candidates(f, d);
  if (found.size === 0) return null;
  const best = [...found].map((e) => d.variants.get(e)![0]!).sort((a, b) => d.rank.get(a)! - d.rank.get(b)!);
  const top = best[0]!;
  if (best.length > 1 && !(d.rank.get(best[1]!)! > DOMINANCE * (d.rank.get(top)! + 50))) return null;
  const forms = d.variants.get(fold(top))!;
  if (forms.length > 1 && !(d.rank.get(forms[1]!)! > DOMINANCE * (d.rank.get(forms[0]!)! + 50))) return null; // pais/país
  return top;
}

/**
 * A palavra digitada com o erro corrigido, mantendo a capitalização. Palavras com inicial maiúscula só entram no começo da frase
 * (no meio do texto costumam ser nomes); TUDO EM MAIÚSCULAS e palavras com acento não são mexidas.
 */
export function correctTypo(word: string, ctx: { sentenceStart: boolean }): string | null {
  if (!/^[a-zA-Z]+$/.test(word)) return null; // só letras sem acento: quem acentuou, sabe o que escreveu
  const lower = word.toLowerCase();
  const capital = word !== lower;
  if (capital && !(word === lower.charAt(0).toUpperCase() + lower.slice(1) && ctx.sentenceStart)) return null;
  const fixed = fixFolded(lower);
  if (!fixed) return null;
  return capital ? fixed.charAt(0).toUpperCase() + fixed.slice(1) : fixed;
}
