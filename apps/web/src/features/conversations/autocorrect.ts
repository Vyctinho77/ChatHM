/**
 * Corretor de acentos do campo de mensagem: quem escreve rápido digita "voce", "nao", "orcamento" e o app põe o acento ao fechar a palavra
 * (espaço ou pontuação). Só corrige o que não é ambíguo: a forma sem acento NÃO é outra palavra comum ("esta"/"está", "e"/"é", "pais"/"país",
 * "negocio"/"negócio" ficam de fora). Respeita maiúsculas ("Voce" → "Você", "NAO" → "NÃO"), e-mails, links, @menções e caminhos.
 * Voltar com Backspace logo em seguida desfaz a correção daquela palavra.
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
`;

export const DICTIONARY: ReadonlyMap<string, string> = new Map(
  WORDS.split(/\s+/).filter(Boolean).map((pair) => {
    const [from, to] = pair.split(":") as [string, string];
    return [from, to];
  }),
);

// Terminações seguras (a palavra sem acento não existe): informação, decisões, possível, responsável.
const SUFFIXES: [RegExp, string][] = [
  [/cao$/, "ção"], [/coes$/, "ções"],
  [/(s)ao$/, "$1ão"], [/soes$/, "sões"], [/stao$/, "stão"], [/stoes$/, "stões"],
  [/zoes$/, "zões"], [/toes$/, "tões"],
  [/ivel$/, "ível"], [/iveis$/, "íveis"], [/avel$/, "ável"], [/aveis$/, "áveis"],
];

const MIN_SUFFIX_LEN = 5; // "cao", "sao" curtos ficam por conta do dicionário

function accentedForm(lower: string): string | null {
  const hit = DICTIONARY.get(lower);
  if (hit) return hit;
  if (lower.length < MIN_SUFFIX_LEN) return lower === "sao" ? "são" : null;
  for (const [re, to] of SUFFIXES) if (re.test(lower)) return lower.replace(re, to);
  return null;
}

/** A palavra com acento, mantendo as maiúsculas ("Voce" → "Você", "NAO" → "NÃO"); null se não há o que corrigir. */
export function correctWord(word: string): string | null {
  if (!/^\p{L}+$/u.test(word) || /[\u00c0-\u024f]/.test(word)) return null; // só letras; quem já acentuou não é mexido
  const lower = word.toLowerCase();
  const fixed = accentedForm(lower);
  if (!fixed) return null;
  if (word === lower) return fixed;
  if (word.length > 1 && word === word.toUpperCase()) return fixed.toUpperCase();
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
export function fixWordEndingAt(text: string, end: number, rejected?: Rejected): Fix | null {
  let start = end;
  while (start > 0 && /\p{L}/u.test(text.charAt(start - 1))) start--;
  if (start === end) return null;
  if (start > 0 && BLOCKED_BEFORE.includes(text.charAt(start - 1))) return null;
  if (start > 0 && /\p{N}/u.test(text.charAt(start - 1))) return null; // "2nao"
  const from = text.slice(start, end);
  if (rejected && rejected.start === start && rejected.word === from.toLowerCase()) return null;
  const to = correctWord(from);
  return to && to !== from ? { start, end, from, to } : null;
}

/** Na hora de enviar: a última palavra ainda não foi "fechada" por espaço ou pontuação. */
export function fixTail(text: string, rejected?: Rejected): string {
  const fix = fixWordEndingAt(text, text.length, rejected);
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
