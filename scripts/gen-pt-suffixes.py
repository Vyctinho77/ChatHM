#!/usr/bin/env python3
"""
Gera apps/web/src/features/conversations/ptSuffixes.data.ts: regras de TERMINAÇÃO para pôr o acento em palavras que a lista de acentos não
cobre (pendencia → pendência, tecnologico → tecnológico). Mesmas fontes e licenças de gen-pt-accents.py (VERO pt-BR, LGPL-3.0 ou MPL-2.0).
Uso:  python3 -I scripts/gen-pt-suffixes.py <pasta com pt.aff e pt.dic> > apps/web/src/features/conversations/ptSuffixes.data.ts

Como as regras são escolhidas (dados, não palpite): para cada palavra acentuada do VERO, a terminação "a partir da primeira letra acentuada"
(sem acento → com acento, até 7 letras) é um candidato. Fica a regra quando
  - tem apoio (>= MIN_SUPPORT palavras acentuadas do VERO), e
  - é a regra dominante: entre as palavras que, SEM acento, não são palavras válidas e terminam igual, >= MIN_SHARE precisam desse acento.
Para nunca errar uma palavra VÁLIDA, as palavras válidas sem acento que casam com alguma regra (ex.: "evidencia", verbo) vão para a lista de EXCEÇÕES.
"""
import collections, json, sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from ptdict import load, strip_marks  # noqa: E402

base = Path(sys.argv[1] if len(sys.argv) > 1 else '.')
MIN_SUPPORT = 40
MIN_SHARE = 0.9
MAX_SUFFIX = 7
MIN_STEM = 3          # a palavra precisa de pelo menos 3 letras antes da terminação
MAX_EXCEPTIONS = 4000
MAX_RULE_EXCEPTIONS = 120   # por regra

valid_plain, accented, freq, rank = load(base)

# S (sem acento, NÃO é palavra válida) -> {terminação acentuada (a partir da 1ª letra acentuada)}
need = collections.defaultdict(set)
for S, forms in accented.items():
    if S in valid_plain or len(S) < MIN_STEM + 2:
        continue
    for W in forms:
        if len(W) != len(S):
            continue
        d = next((i for i in range(len(S)) if S[i] != W[i]), None)
        if d is None or len(S) - d > MAX_SUFFIX or d < MIN_STEM:
            continue
        need[S].add(W[d:])

# por terminação sem acento s: quantas S a exigem com cada acentuação a
by_suffix = collections.defaultdict(collections.Counter)    # s -> Counter(a)
for S, ends in need.items():
    if len(ends) != 1:
        continue                    # a mesma S aceita acentos diferentes: ambígua
    a = next(iter(ends))
    s = strip_marks(a)
    by_suffix[s][a] += 1
# todas as S inválidas (que precisam de acento) terminadas em s, para medir a parcela dominante
ends_in = collections.Counter()
for S in need:
    for k in range(1, MAX_SUFFIX + 1):
        ends_in[S[-k:]] += 1

rules = {}
for s, cnt in by_suffix.items():
    a, n = cnt.most_common(1)[0]
    if n >= MIN_SUPPORT and n / ends_in[s] >= MIN_SHARE:
        rules[s] = (a, n, n / ends_in[s])
# prefere a terminação mais curta que já cobre: uma regra mais longa que só repete o efeito de outra, sai
def covered(s):
    return any(s != t and s.endswith(t) and strip_marks(rules[t][0]) == t and rules[s][0].endswith(rules[t][0]) for t in rules)
rules = {s: v for s, v in rules.items() if not covered(s)}

def apply(w):
    for s in sorted(rules, key=len, reverse=True):
        if w.endswith(s) and len(w) - len(s) >= MIN_STEM:
            return w[:-len(s)] + rules[s][0]
    return None

# exceções por regra: uma regra que obriga a listar muitas palavras válidas não vale a pena (a terminação é ambígua)
exc_by_rule = collections.Counter()
for w in valid_plain:
    if len(w) >= MIN_STEM + 2:
        for s_ in rules:
            if w.endswith(s_) and len(w) - len(s_) >= MIN_STEM:
                exc_by_rule[s_] += 1
rules = {s_: v for s_, v in rules.items() if exc_by_rule[s_] <= MAX_RULE_EXCEPTIONS}
# Uma palavra válida sem acento só vira exceção se for de uso comum (aparece na lista de frequência: "esta", "para") ou se a regra
# inventasse uma palavra que o VERO não conhece. Os substantivos que o VERO aceita sem acento por causa de um verbo raro ("pendencia",
# "gramatica") continuam sendo corrigidos: quem digita assim quer o substantivo.
acc_all = {w for forms in accented.values() for w in forms}
def exception(w):
    r = apply(w)
    return r is not None and (w in rank or r not in acc_all)
exceptions = sorted(w for w in valid_plain if len(w) >= MIN_STEM + 2 and exception(w))
print('regras:', len(rules), '| exceções:', len(exceptions), file=sys.stderr)
for s, (a, n, share) in sorted(rules.items(), key=lambda kv: -kv[1][1])[:25]:
    print('  %s -> %s  apoio=%d parcela=%.2f' % (s, a, n, share), file=sys.stderr)
if len(exceptions) > MAX_EXCEPTIONS:
    print('AVISO: exceções acima de %d; aperte MIN_SHARE' % MAX_EXCEPTIONS, file=sys.stderr)

print('// GERADO por scripts/gen-pt-suffixes.py: não edite à mão. %d regras, %d exceções.' % (len(rules), len(exceptions)))
print('// Fonte: dicionário VERO pt-BR (LGPL-3.0 ou MPL-2.0, https://pt-br.libreoffice.org/projetos/projeto-vero-verificador-ortografico/).')
print('// Formato das regras: "semacento:comacento" separados por espaço (terminação). Exceções: palavras válidas sem acento que casam com alguma regra.')
print('export const PT_SUFFIX_RULES = %s;' % json.dumps(' '.join('%s:%s' % (s, rules[s][0]) for s in sorted(rules, key=lambda x: (-len(x), x))), ensure_ascii=False))
print('export const PT_SUFFIX_EXCEPTIONS = %s;' % json.dumps(' '.join(exceptions), ensure_ascii=False))
