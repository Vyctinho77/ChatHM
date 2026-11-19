/**
 * Corretor de ERROS DE DIGITAÇÃO do campo de mensagem (interno, sem tela). Só corrige com muita confiança: a palavra digitada não é palavra,
 * e existe UMA palavra frequente a um destes enganos, os mais comuns de quem digita rápido:
 *   tr   duas letras vizinhas trocadas        (voecê → você)           a partir de 4 letras
 *   ins  faltou uma letra                     (obrigdo → obrigado)     a partir de 5
 *   dup  letra repetida sem querer            (obrigaddo → obrigado)   a partir de 5
 *   key  letra no lugar da tecla vizinha      (obrigafo → obrigado)    a partir de 6
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
      if (below) {
        for (const k of [j - 1, j]) {
          if (k < 0 || k >= below.length) continue;
          link(ch, below[k]!);
          link(below[k]!, ch);
        }
      }
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

const ASCII = /^[\x00-\x7f]*$/; // eslint-disable-line no-control-regex
/** Sem acentos e sem cedilha. Palavra só com ASCII (a maioria) passa direto, sem normalizar. */
export const fold = (w: string) => (ASCII.test(w) ? w : w.normalize("NFD").replace(/\p{M}/gu, ""));

type Dict = { variants: Map<string, string[]>; rank: Map<string, number>; exceptions: ReadonlySet<string>; keys: string[] };
let dict: Dict | null = null;
let loading: Promise<void> | null = null;

const SLICE = 4000; // palavras por fatia ao montar o vocabulário sem travar a tela
const yieldToMain = () => new Promise<void>((resolve) => setTimeout(resolve, 0));

function indexWords(words: readonly string[], from: number, to: number, variants: Map<string, string[]>, rank: Map<string, number>) {
  for (let i = from; i < Math.min(to, words.length); i++) {
    const w = words[i]!;
    rank.set(w, i);
    const f = fold(w);
    const list = variants.get(f);
    if (list) list.push(w);
    else variants.set(f, [w]);
  }
}

/** `words` em ordem de frequência (acentuadas); `exceptions` sem acento. Monta tudo de uma vez (testes). */
export function setTypoDictionary(words: readonly string[] | null, exceptions: Iterable<string> = []) {
  if (!words) return void (dict = null);
  const variants = new Map<string, string[]>();
  const rank = new Map<string, number>();
  indexWords(words, 0, words.length, variants, rank);
  dict = { variants, rank, exceptions: new Set(exceptions), keys: [...variants.keys()] };
}

/** O mesmo, em fatias que devolvem o controle à tela entre si: o app não congela enquanto o vocabulário é montado. */
async function buildTypoDictionary(words: readonly string[], exceptions: readonly string[]) {
  const variants = new Map<string, string[]>();
  const rank = new Map<string, number>();
  for (let i = 0; i < words.length; i += SLICE) {
    indexWords(words, i, i + SLICE, variants, rank);
    await yieldToMain();
  }
  dict = { variants, rank, exceptions: new Set(exceptions), keys: [...variants.keys()] };
}

export const isTypoDictionaryLoaded = () => dict !== null;

/** As palavras frequentes (para o verificador de palavras): `has` = é uma delas (como escrita), `variants` = sem acento → acentuadas, `rank` = frequência. */
export function getTypoVocabulary(): { has: (w: string) => boolean; variants: ReadonlyMap<string, string[]>; rank: ReadonlyMap<string, number>; keys: readonly string[] } | null {
  const d = dict;
  return d ? { has: (w) => d.rank.has(w), variants: d.variants, rank: d.rank, keys: d.keys } : null;
}

/** Carrega os dados (dois arquivos separados, ~150 KB comprimidos no total). Chamar de novo não baixa de novo. */
export function loadTypoDictionary(): Promise<void> {
  loading ??= Promise.all([import("./ptWords.data"), import("./ptTypoExceptions.data")])
    .then(([w, e]) => buildTypoDictionary(w.PT_WORDS.split(" "), e.PT_TYPO_EXCEPTIONS.split(" ")))
    .catch(() => {
      loading = null; // sem rede agora: tenta de novo na próxima vez
    });
  return loading;
}

function candidates(f: string, d: Dict): Map<string, Kind> {
  const found = new Map<string, Kind>();
  const n = f.length;
  const take = (e: string, kind: Kind) => {
    if (n >= MIN[kind] && d.variants.has(e) && !found.has(e)) found.set(e, kind);
  };
  for (let i = 0; i < n - 1; i++) if (f[i] !== f[i + 1]) take(f.slice(0, i) + f[i + 1] + f[i] + f.slice(i + 2), "tr");
  for (let i = 1; i <= n; i++) for (const c of ALPHA) take(f.slice(0, i) + c + f.slice(i), "ins");
  for (let i = 1; i < n; i++) if (f[i] === f[i - 1]) take(f.slice(0, i) + f.slice(i + 1), "dup");
  for (let i = 0; i < n; i++) for (const c of NEIGH.get(f[i]!) ?? []) take(f.slice(0, i) + c + f.slice(i + 1), "key");
  return found;
}

export type TypoHit = { word: string; kind: Kind };

/** O melhor candidato (acentuado) e o tipo de engano, para a palavra digitada em minúsculas, sem acento; null = não mexe. */
export function fixFoldedKind(f: string): TypoHit | null {
  const d = dict;
  if (!d || !/^[a-z]+$/.test(f) || f.length < MIN_WORD || LAUGH.test(f) || DOMAIN.has(f) || d.variants.has(f) || d.exceptions.has(f)) return null;
  const found = candidates(f, d);
  if (found.size === 0) return null;
  const best = [...found.keys()].map((e) => d.variants.get(e)![0]!).sort((a, b) => d.rank.get(a)! - d.rank.get(b)!);
  const top = best[0]!;
  if (best.length > 1 && !(d.rank.get(best[1]!)! > DOMINANCE * (d.rank.get(top)! + 50))) return null;
  const forms = d.variants.get(fold(top))!;
  if (forms.length > 1 && !(d.rank.get(forms[1]!)! > DOMINANCE * (d.rank.get(forms[0]!)! + 50))) return null; // pais/país
  return { word: top, kind: found.get(fold(top))! };
}

/** Só a palavra (para quem não precisa do tipo). */
export const fixFolded = (f: string): string | null => fixFoldedKind(f)?.word ?? null;

/**
 * A palavra digitada com o erro corrigido, mantendo a capitalização. Palavras com inicial maiúscula só entram no começo da frase
 * (no meio do texto costumam ser nomes); TUDO EM MAIÚSCULAS e palavras com acento não são mexidas.
 */
export function correctTypoLayer(word: string, ctx: { sentenceStart: boolean }): { to: string; layer: `typo_${Kind}` } | null {
  if (!/^[a-zA-Z]+$/.test(word)) return null; // só letras sem acento: quem acentuou, sabe o que escreveu
  const lower = word.toLowerCase();
  const capital = word !== lower;
  if (capital && !(word === lower.charAt(0).toUpperCase() + lower.slice(1) && ctx.sentenceStart)) return null;
  const hit = fixFoldedKind(lower);
  if (!hit) return null;
  return { to: capital ? hit.word.charAt(0).toUpperCase() + hit.word.slice(1) : hit.word, layer: `typo_${hit.kind}` };
}

export const correctTypo = (word: string, ctx: { sentenceStart: boolean }): string | null => correctTypoLayer(word, ctx)?.to ?? null;
