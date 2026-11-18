import { correctTypoLayer } from "./typos";

/**
 * Corretor de acentos do campo de mensagem: quem escreve rápido digita "voce", "nao", "orcamento" e o app põe o acento ao fechar a palavra
 * (espaço ou pontuação). Só corrige o que não é ambíguo: a forma sem acento NÃO é outra palavra comum ("esta"/"está", "e"/"é", "pais"/"país",
 * "negocio"/"negócio" ficam de fora). Respeita maiúsculas ("Voce" → "Você", "NAO" → "NÃO"), e-mails, links, @menções e caminhos.
 * Voltar com Backspace logo em seguida desfaz a correção daquela palavra.
 *
 * Três camadas, da mais forte para a mais fraca: a lista curada abaixo (sempre vale, inclusive gírias de chat como "ta" → "tá"), o dicionário
 * estendido (ptAccents.data.ts, ~4.700 palavras geradas do VERO pt-BR + frequência de conversa; carrega sob demanda) e as terminações
 * seguras (-ção, -são, -ível...). Palavras cuja correção a pessoa desfez duas vezes deixam de ser corrigidas (por um tempo). Não há tela
 * de configuração: o corretor é interno e só aparece nas caixas de texto.
 */

// sem acento → com acento. Uma linha por assunto, só para ficar legível.
const WORDS = `
nao:não voce:você voces:vocês tambem:também entao:então ja:já ate:até so:só la:lá ola:olá alo:alô ninguem:ninguém alguem:alguém porem:porém alem:além apos:após
atraves:através mes:mês tres:três mae:mãe maes:mães pao:pão paes:pães mao:mão maos:mãos irmao:irmão irmaos:irmãos orgao:órgão orgaos:órgãos cidadao:cidadão cartao:cartão cartoes:cartões
estao:estão vao:vão irao:irão farao:farão serao:serão terao:terão darao:darão poderao:poderão ficarao:ficarão estara:estará podera:poderá tera:terá fara:fará dara:dará ficara:ficará
numero:número numeros:números proximo:próximo proxima:próxima proximos:próximos proximas:próximas ultimo:último ultima:última ultimos:últimos ultimas:últimas
unico:único unica:única unicos:únicos unicas:únicas pagina:página paginas:páginas codigo:código codigos:códigos util:útil uteis:úteis facil:fácil faceis:fáceis dificil:difícil dificeis:difíceis
rapido:rápido rapida:rápida agil:ágil otimo:ótimo otima:ótima pessimo:péssimo pessima:péssima area:área areas:áreas agencia:agência agencias:agências
sabado:sábado terca:terça preco:preço precos:preços servico:serviço servicos:serviços endereco:endereço enderecos:endereços orcamento:orçamento orcamentos:orçamentos
cobranca:cobrança cobrancas:cobranças presenca:presença diferenca:diferença licenca:licença confianca:confiança financas:finanças
audiencia:audiência competencia:competência experiencia:experiência ausencia:ausência emergencia:emergência referencia:referência frequencia:frequência
escritorio:escritório cartorio:cartório obrigatorio:obrigatório obrigatoria:obrigatória relatorio:relatório laboratorio:laboratório territorio:território
horario:horário horarios:horários salario:salário salarios:salários diario:diário necessario:necessário necessaria:necessária bancario:bancário bancaria:bancária
juridico:jurídico juridica:jurídica tecnico:técnico tecnica:técnica contabil:contábil historico:histórico periodo:período periodos:períodos tramite:trâmite tramites:trâmites
familia:família historia:história memoria:memória politica:política reuniao:reunião reunioes:reuniões opiniao:opinião uniao:união regiao:região regioes:regiões
milhao:milhão milhoes:milhões razao:razão portugues:português ingles:inglês
ta:tá ne:né cade:cadê sera:será amanha:amanhã manha:manhã historia:história musica:música medico:médico video:vídeo
`;

export const DICTIONARY: ReadonlyMap<string, string> = new Map(
  WORDS.split(/\s+/).filter(Boolean).map((pair) => {
    const [from, to] = pair.split(":") as [string, string];
    return [from, to];
  }),
);

// Dicionário estendido (gerado): carrega uma vez, em segundo plano. Até carregar vale só a lista curada.
let extended: ReadonlyMap<string, string> | null = null;
let loading: Promise<void> | null = null;

export function setExtendedDictionary(map: ReadonlyMap<string, string> | null) {
  extended = map;
}
export const isExtendedLoaded = () => extended !== null;

/** Carrega o dicionário estendido (um arquivo separado, ~30 KB comprimido). Chamar de novo não baixa de novo. */
export function loadExtendedDictionary(): Promise<void> {
  loading ??= import("./ptAccents.data")
    .then(({ PT_ACCENTS }) => {
      extended = new Map(PT_ACCENTS.split(" ").map((pair) => pair.split(":") as [string, string]));
    })
    .catch(() => {
      loading = null; // sem rede agora: tenta de novo na próxima vez
    });
  return loading;
}

export type CorrectOptions = {
  /** Palavras (minúsculas) que o corretor aprendeu a não corrigir (a pessoa desfez a correção duas vezes). */
  ignored?: ReadonlySet<string>;
};

// Terminações seguras (a palavra sem acento não existe): informação, decisões, possível, responsável.
const SUFFIXES: [RegExp, string][] = [
  [/cao$/, "ção"], [/coes$/, "ções"],
  [/(s)ao$/, "$1ão"], [/soes$/, "sões"], [/stao$/, "stão"], [/stoes$/, "stões"],
  [/zoes$/, "zões"], [/toes$/, "tões"],
  [/ivel$/, "ível"], [/iveis$/, "íveis"], [/avel$/, "ável"], [/aveis$/, "áveis"],
];

const MIN_SUFFIX_LEN = 5; // "cao", "sao" curtos ficam por conta do dicionário

/** Qual camada do corretor fez a correção (para medir onde ele acerta e onde a pessoa desfaz). */
export type Layer = "curated" | "extended" | "suffix" | "typo_tr" | "typo_ins" | "typo_dup" | "typo_key";

function accentedForm(lower: string): { to: string; layer: Layer } | null {
  const curated = DICTIONARY.get(lower);
  if (curated) return { to: curated, layer: "curated" };
  const wide = extended?.get(lower);
  if (wide) return { to: wide, layer: "extended" };
  if (lower.length < MIN_SUFFIX_LEN) return lower === "sao" ? { to: "são", layer: "curated" } : null;
  for (const [re, to] of SUFFIXES) if (re.test(lower)) return { to: lower.replace(re, to), layer: "suffix" };
  return null;
}

/** A palavra com acento, mantendo as maiúsculas ("Voce" → "Você", "NAO" → "NÃO"); null se não há o que corrigir. */
export const correctWord = (word: string, opts: CorrectOptions = {}): string | null => correctWordLayer(word, opts)?.to ?? null;

/** O mesmo que correctWord, dizendo também qual camada corrigiu. */
export function correctWordLayer(word: string, opts: CorrectOptions = {}): { to: string; layer: Layer } | null {
  if (!/^\p{L}+$/u.test(word) || /[\u00c0-\u024f]/.test(word)) return null; // só letras; quem já acentuou não é mexido
  const lower = word.toLowerCase();
  if (opts.ignored?.has(lower)) return null;
  const upper = word.length > 1 && word === word.toUpperCase();
  if (upper && word.length <= 2) return null; // "PE", "AI", "VC": sigla
  const hit = accentedForm(lower);
  if (!hit) return null;
  const { to: fixed, layer } = hit;
  if (word === lower) return { to: fixed, layer };
  if (upper) return { to: fixed.toUpperCase(), layer };
  if (word === lower.charAt(0).toUpperCase() + lower.slice(1)) return { to: fixed.charAt(0).toUpperCase() + fixed.slice(1), layer };
  return null; // maiúsculas misturadas ("nAo"): provavelmente é de propósito
}

/** Digitar um destes fecha a palavra. */
export const TRIGGERS = " \n\t.,;:!?)]";
export const isTrigger = (ch: string) => ch.length === 1 && TRIGGERS.includes(ch);

// Antes da palavra: e-mail, link, caminho, @menção, #tag, variável {x}, nome com hífen ou ponto.
const BLOCKED_BEFORE = "@/\\:#._-{<[+=~$%&*";

export type Fix = { start: number; end: number; from: string; to: string; layer: Layer };

/** A correção que a pessoa desfez com Backspace: aquela palavra, naquele lugar, não é corrigida de novo (as outras continuam sendo). */
export type Rejected = { start: number; word: string } | null;

/** O texto antes de `start` termina uma frase (ou não há texto)? Só então uma palavra com inicial maiúscula pode ser corrigida como erro de digitação. */
const atSentenceStart = (text: string, start: number) => /(^|[.!?…]\s+|\n)\s*$/.test(text.slice(0, start));

/** A palavra que termina em `end` (exclusive) merece correção? */
export function fixWordEndingAt(text: string, end: number, rejected?: Rejected, opts: CorrectOptions = {}): Fix | null {
  let start = end;
  while (start > 0 && /\p{L}/u.test(text.charAt(start - 1))) start--;
  if (start === end) return null;
  if (start > 0 && BLOCKED_BEFORE.includes(text.charAt(start - 1))) return null;
  if (start > 0 && /\p{N}/u.test(text.charAt(start - 1))) return null; // "2nao"
  const from = text.slice(start, end);
  if (rejected && rejected.start === start && rejected.word === from.toLowerCase()) return null;
  // 1º acentos (palavra conhecida sem acento); 2º erro de digitação (a palavra não existe e há uma muito parecida e muito mais comum)
  const hit = correctWordLayer(from, opts) ?? (opts.ignored?.has(from.toLowerCase()) ? null : correctTypoLayer(from, { sentenceStart: atSentenceStart(text, start) }));
  return hit && hit.to !== from ? { start, end, from, to: hit.to, layer: hit.layer } : null;
}

/** Na hora de enviar: a última palavra ainda não foi "fechada" por espaço ou pontuação. */
export function fixTail(text: string, rejected?: Rejected, opts: CorrectOptions = {}): string {
  return fixTailWithFix(text, rejected, opts).text;
}

/** O mesmo que fixTail, devolvendo também a correção feita (para contar). */
export function fixTailWithFix(text: string, rejected?: Rejected, opts: CorrectOptions = {}): { text: string; fix: Fix | null } {
  const fix = fixWordEndingAt(text, text.length, rejected, opts);
  return { text: fix ? text.slice(0, fix.start) + fix.to + text.slice(fix.end) : text, fix };
}

/** Aplica uma correção ao texto; devolve o novo texto e para onde o cursor vai (`caret`, antes da correção, na mesma posição relativa). */
export function applyFix(text: string, fix: Fix, caret: number): { text: string; caret: number } {
  return { text: text.slice(0, fix.start) + fix.to + text.slice(fix.end), caret: caret + fix.to.length - (fix.end - fix.start) };
}

/** Desfaz: volta a palavra original (Backspace logo depois da correção). */
export function undoFix(text: string, fix: Fix, caret: number): { text: string; caret: number } {
  return { text: text.slice(0, fix.start) + fix.from + text.slice(fix.start + fix.to.length), caret: caret - (fix.to.length - fix.from.length) };
}
