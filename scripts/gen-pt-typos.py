#!/usr/bin/env python3
"""
Gera os dados do corretor de ERROS DE DIGITAÇÃO (apps/web/src/features/conversations/):
  ptWords.data.ts           as palavras frequentes de conversa que o VERO conhece (lista-alvo, em ordem de frequência)
  ptTypoExceptions.data.ts  palavras VÁLIDAS do VERO (raras) que o algoritmo trocaria por engano: nunca são corrigidas
  typos.fixture.json        casos de teste (digitado -> esperado) que o teste em TypeScript confere, para as duas implementações concordarem

Mesmas fontes e licenças de gen-pt-accents.py (VERO pt-BR, LGPL-3.0 ou MPL-2.0; frequência OpenSubtitles 2024 via OPUS, ODC-BY).
Uso:  python3 -I scripts/gen-pt-typos.py <pasta com pt.aff, pt.dic e zx/> <pasta de saída>

O algoritmo (idêntico ao de typos.ts). Sobre a palavra digitada sem acento (f), só 4 tipos de engano, os mais prováveis, e só se
a palavra digitada NÃO for palavra (nem das frequentes, nem uma exceção):
  tr   duas letras vizinhas trocadas        (voecê → você)           tamanho mínimo 4
  ins  faltou uma letra                     (obrigdo → obrigado)     tamanho mínimo 5
  dup  letra repetida sem querer            (obrigaddo → obrigado)   tamanho mínimo 5
  key  uma letra no lugar da tecla vizinha  (obrigafo → obrigado)    tamanho mínimo 6
Candidatos são só palavras da lista-alvo. Com mais de um candidato, só vale se o melhor for MUITO mais frequente (20x) que o segundo.
"""
import collections, json, random, re, sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from ptdict import load, strip_marks  # noqa: E402

base = Path(sys.argv[1] if len(sys.argv) > 1 else '.')
out = Path(sys.argv[2] if len(sys.argv) > 2 else '.')
TARGETS = 40000
ALPHA = 'abcdefghijklmnopqrstuvwxyz'
MIN = {'tr': 4, 'ins': 5, 'dup': 5, 'key': 6}
DOMINANCE = 20

ROWS = ['qwertyuiop', 'asdfghjkl', 'zxcvbnm']
NEIGH = collections.defaultdict(set)
for r, row in enumerate(ROWS):
    for j, ch in enumerate(row):
        if j > 0: NEIGH[ch].add(row[j - 1])
        if j + 1 < len(row): NEIGH[ch].add(row[j + 1])
        if r + 1 < len(ROWS):            # a fileira de baixo é deslocada meia tecla: a tecla j encosta nas teclas j-1 e j de baixo
            for k in (j - 1, j):
                if 0 <= k < len(ROWS[r + 1]):
                    NEIGH[ch].add(ROWS[r + 1][k])
                    NEIGH[ROWS[r + 1][k]].add(ch)

valid_plain, accented, freq, rank_all = load(base)
ok = re.compile(r'^[a-zà-ÿ]+$')
vero_all = set(valid_plain) | {w for forms in accented.values() for w in forms}
vero_fold = set(valid_plain) | set(accented.keys())

# lista-alvo: frequentes e conhecidas do VERO (grafias antigas e nomes estrangeiros ficam de fora)
T, seen = [], set()
for w in freq:
    if ok.match(w) and len(w) >= 2 and w not in seen and w in vero_all:
        seen.add(w)
        T.append(w)
        if len(T) >= TARGETS:
            break
rank = {w: i for i, w in enumerate(T)}
Tf = collections.defaultdict(list)          # sem acento -> variantes acentuadas, da mais frequente
for w in T:
    Tf[strip_marks(w)].append(w)

def candidates(f):
    """Palavras da lista-alvo a um engano de f (sem acento): {sem_acento: tipo}."""
    n = len(f)
    found = {}
    def take(e, kind):
        if e in Tf and len(f) >= MIN[kind]:
            found.setdefault(e, kind)
    for i in range(n - 1):
        if f[i] != f[i + 1]:
            take(f[:i] + f[i + 1] + f[i] + f[i + 2:], 'tr')
    for i in range(1, n + 1):
        for c in ALPHA:
            take(f[:i] + c + f[i:], 'ins')
    for i in range(1, n):
        if f[i] == f[i - 1]:
            take(f[:i] + f[i + 1:], 'dup')
    for i in range(n):
        for c in NEIGH.get(f[i], ()):
            take(f[:i] + c + f[i + 1:], 'key')
    return found

def pick(found):
    """O melhor candidato (acentuado) ou None."""
    if not found:
        return None
    best = sorted((Tf[e][0] for e in found), key=lambda w: rank[w])
    top = best[0]
    if len(best) > 1 and not rank[best[1]] > DOMINANCE * (rank[top] + 50):
        return None
    forms = Tf[strip_marks(top)]
    if len(forms) > 1 and not rank[forms[1]] > DOMINANCE * (rank[forms[0]] + 50):
        return None                       # pais/país: ambígua
    return top

LAUGH = re.compile(r'^(k+|(rs)+|(ha)+h?|(he)+h?|(hi)+|(ah)+h?|(eh)+|(oh)+|(ue)+|(u+h)|(a+h+))$')

# exceções: palavras VÁLIDAS (fora da lista-alvo) que seriam trocadas
exceptions = sorted(f for f in vero_fold if len(f) >= 4 and f not in Tf and ok.match(f) and not LAUGH.match(f) and pick(candidates(f)))

def fix(typed):
    """O comportamento completo para uma palavra digitada em minúsculas sem acento (espelha typos.ts)."""
    if not re.match(r'^[a-z]+$', typed) or LAUGH.match(typed) or len(typed) < 4 or typed in Tf or typed in EX:
        return None
    return pick(candidates(typed))
EX = set(exceptions)

# ---- casos de teste ----
random.seed(2026)
pool = [w for w in T[:20000] if len(strip_marks(w)) >= 5]
cases = {}
def simulate(kind, f):
    i = random.randrange(len(f))
    if kind == 'tr': return f[:i] + f[i + 1] + f[i] + f[i + 2:] if i < len(f) - 1 else None
    if kind == 'dup': return f[:i + 1] + f[i] + f[i + 1:]
    if kind == 'ins': return f[:i] + f[i + 1:]                       # faltou uma letra
    if kind == 'key': return f[:i] + random.choice(sorted(NEIGH[f[i]]) or ['x']) + f[i + 1:] if f[i] in NEIGH else None
    if kind == 'extra': return f[:i] + random.choice(ALPHA) + f[i:]   # letra a mais qualquer
    if kind == 'sub': return f[:i] + random.choice(ALPHA) + f[i + 1:] # letra errada qualquer
while len(cases) < 900:
    w = random.choice(pool); f = strip_marks(w)
    t = simulate(random.choice(['tr', 'dup', 'ins', 'key', 'extra', 'sub']), f)
    if t and t != f:
        cases[t] = fix(t)
for w in random.sample(exceptions, min(120, len(exceptions))): cases[w] = fix(w)        # exceções: nunca mexe
for w in random.sample(sorted(Tf), 150): cases[w] = fix(w)                              # palavras válidas frequentes
for w in ['kkkk', 'rsrs', 'hahaha', 'cnpj', 'darf', 'zap', 'xyz', 'qqqq', 'whatsapp', 'esocial', 'simples', 'presumido']:
    cases[w] = fix(w)
fixture = [[k, v] for k, v in sorted(cases.items())]

# ---- saída ----
def ts(name, words, what):
    return ('// GERADO por scripts/gen-pt-typos.py: não edite à mão. %d palavras. %s\n'
            '// Fontes: dicionário VERO pt-BR (LGPL-3.0 ou MPL-2.0) e frequência de palavras de OpenSubtitles 2024 via OPUS (ODC-BY).\n'
            'export const %s = %s;\n') % (len(words), what, name, json.dumps(' '.join(words), ensure_ascii=False))
(out / 'ptWords.data.ts').write_text(ts('PT_WORDS', T, 'Palavras frequentes de conversa, em ordem de frequência (separadas por espaço).'), encoding='utf-8')
(out / 'ptTypoExceptions.data.ts').write_text(ts('PT_TYPO_EXCEPTIONS', exceptions, 'Palavras válidas sem acento que o corretor de digitação nunca troca.'), encoding='utf-8')
(out / 'typos.fixture.json').write_text(json.dumps(fixture, ensure_ascii=False, separators=(',', ':')), encoding='utf-8')
print('alvos:', len(T), '| exceções:', len(exceptions), '| casos de teste:', len(fixture), '| com correção:', sum(1 for _, v in fixture if v), file=sys.stderr)
