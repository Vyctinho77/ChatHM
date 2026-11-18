/**
 * Corretor de acentos do campo de mensagem: quem escreve rápido digita "voce", "nao", "orcamento" e o app põe o acento ao fechar a palavra
 * (espaço ou pontuação). Só corrige o que não é ambíguo: a forma sem acento NÃO é outra palavra comum ("esta"/"está", "e"/"é", "pais"/"país",
 * "negocio"/"negócio" ficam de fora). Respeita maiúsculas ("Voce" → "Você", "NAO" → "NÃO"), e-mails, links, @menções e caminhos.
 * Voltar com Backspace logo em seguida desfaz a correção daquela palavra.
 *
 * Três camadas, da mais forte para a mais fraca: a lista curada abaixo (sempre vale, inclusive gírias de chat como "ta" → "tá"), o dicionário
 * estendido (ptAccents.data.ts, ~4.700 palavras geradas do VERO pt-BR + frequência de conversa; carrega sob demanda) e as terminações
 * seguras (-ção, -são, -ível...). Abreviações de chat ("vc", "tb", "obg") só com a opção ligada. Palavras que a pessoa mandou ignorar
 * (ou que ela desfez duas vezes) nunca são corrigidas.
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

/** Abreviações de chat (só com a opção ligada). Sempre com 2+ letras: "n" e "q" sozinhos são ambíguos demais. */
export const ABBREVIATIONS: ReadonlyMap<string, string> = new Map(
  `vc:você vcs:vocês tb:também tbm:também obg:obrigado obgd:obrigado obgda:obrigada blz:beleza msg:mensagem msgs:mensagens hj:hoje
ctz:certeza cmg:comigo ctg:contigo mt:muito mto:muito mta:muita td:tudo tds:todos qdo:quando qto:quanto dps:depois pfv:por-favor vlw:valeu`
    .split(/\s+/).filter(Boolean).map((pair) => {
      const [from, to] = pair.split(":") as [string, string];
      return [from, to.replace("-", " ")];
    }),
);

export type CorrectOptions = {
  /** Põe acentos (padrão: sim). Falso = só abreviações. */
  accents?: boolean;
  /** Expande abreviações de chat (vc → você). Desligado por padrão. */
  abbreviations?: boolean;
  /** Palavras (minúsculas) que a pessoa mandou nunca corrigir. */
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

function accentedForm(lower: string): string | null {
  const hit = DICTIONARY.get(lower) ?? extended?.get(lower);
  if (hit) return hit;
  if (lower.length < MIN_SUFFIX_LEN) return lower === "sao" ? "são" : null;
  for (const [re, to] of SUFFIXES) if (re.test(lower)) return lower.replace(re, to);
  return null;
}

/** A palavra com acento, mantendo as maiúsculas ("Voce" → "Você", "NAO" → "NÃO"); null se não há o que corrigir. */
export function correctWord(word: string, opts: CorrectOptions = {}): string | null {
  if (!/^\p{L}+$/u.test(word) || /[\u00c0-\u024f]/.test(word)) return null; // só letras; quem já acentuou não é mexido
  const lower = word.toLowerCase();
  if (opts.ignored?.has(lower)) return null;
  const upper = word.length > 1 && word === word.toUpperCase();
  if (upper && word.length <= 2) return null; // "PE", "AI", "VC": sigla
  const fixed = (opts.abbreviations ? ABBREVIATIONS.get(lower) : undefined) ?? (opts.accents === false ? null : accentedForm(lower));
  if (!fixed) return null;
  if (word === lower) return fixed;
  if (upper) return fixed.toUpperCase();
  if (word === lower.charAt(0).toUpperCase() + lower.slice(1)) return fixed.charAt(0).toUpperCase() + fixed.slice(1);
  return null; // maiúsculas misturadas ("nAo"): provavelmente é de propósito
}

/** Digitar um destes fecha a palavra. */
export const TRIGGERS = " \n\t.,;:!?)]";
export const isTrigger = (ch: string) => ch.length === 1 && TRIGGERS.includes(ch);

// Antes da palavra: e-mail, link, caminho, @menção, #tag, variável {x}, nome com hífen ou ponto.
const BLOCKED_BEFORE = "@/\\:#._-{<[+=~$%&*";

export type Fix = { start: number; end: number; from: string; to: string };

/** A correção que a pessoa desfez com Backspace: aquela palavra, naquele lugar, não é corrigida de novo (as outras continuam sendo). */
export type Rejected = { start: number; word: string } | null;

/** A palavra que termina em `end` (exclusive) merece correção? */
export function fixWordEndingAt(text: string, end: number, rejected?: Rejected, opts: CorrectOptions = {}): Fix | null {
  let start = end;
  while (start > 0 && /\p{L}/u.test(text.charAt(start - 1))) start--;
  if (start === end) return null;
  if (start > 0 && BLOCKED_BEFORE.includes(text.charAt(start - 1))) return null;
  if (start > 0 && /\p{N}/u.test(text.charAt(start - 1))) return null; // "2nao"
  const from = text.slice(start, end);
  if (rejected && rejected.start === start && rejected.word === from.toLowerCase()) return null;
  const to = correctWord(from, opts);
  return to && to !== from ? { start, end, from, to } : null;
}

/** Na hora de enviar: a última palavra ainda não foi "fechada" por espaço ou pontuação. */
export function fixTail(text: string, rejected?: Rejected, opts: CorrectOptions = {}): string {
  const fix = fixWordEndingAt(text, text.length, rejected, opts);
  return fix ? text.slice(0, fix.start) + fix.to + text.slice(fix.end) : text;
}

/** Aplica uma correção ao texto; devolve o novo texto e para onde o cursor vai (`caret`, antes da correção, na mesma posição relativa). */
export function applyFix(text: string, fix: Fix, caret: number): { text: string; caret: number } {
  return { text: text.slice(0, fix.start) + fix.to + text.slice(fix.end), caret: caret + fix.to.length - (fix.end - fix.start) };
}

/** Desfaz: volta a palavra original (Backspace logo depois da correção). */
export function undoFix(text: string, fix: Fix, caret: number): { text: string; caret: number } {
  return { text: text.slice(0, fix.start) + fix.from + text.slice(fix.start + fix.to.length), caret: caret - (fix.to.length - fix.from.length) };
}
