#!/usr/bin/env python3
"""
Gera apps/web/src/features/conversations/ptValid.bin: um filtro de Bloom com TODAS as formas válidas do dicionário VERO pt-BR (palavras com e sem
acento, minúsculas). Serve para o corretor saber se uma palavra digitada "existe" (e sublinhar as que não existem). Mesmas fontes e licenças de
gen-pt-accents.py (VERO pt-BR, LGPL-3.0 ou MPL-2.0).
Uso:  python3 -I scripts/gen-pt-bloom.py <pasta com pt.aff e pt.dic> <pasta de saída>
Saída: ptValid.bin e spell.fixture.json (casos que o teste em TypeScript confere: as duas implementações têm de concordar bit a bit).

Formato: [k: 1 byte][m: uint32 little-endian, em bits][m bits]. Sonda i (0..k-1) = (h1 + i*h2) mod 2^32 mod m, onde, sobre os bytes UTF-8 da palavra,
  h1 = FNV-1a de 32 bits (basis 2166136261, primo 16777619)
  h2 = (djb2 com xor: h = 5381; h = ((h << 5) + h) ^ byte, 32 bits) | 1
Taxa de falso positivo ~1%: uma palavra inexistente passa por existente em ~1% das vezes (nunca o contrário).
"""
import json, math, random, re, sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from ptdict import load  # noqa: E402

base = Path(sys.argv[1] if len(sys.argv) > 1 else '.')
out = Path(sys.argv[2] if len(sys.argv) > 2 else '.')
BITS_PER_WORD = 9.6
K = 7
M32 = 0xFFFFFFFF

def hashes(word):
    data = word.encode('utf-8')
    h1 = 2166136261
    h2 = 5381
    for b in data:
        h1 = ((h1 ^ b) * 16777619) & M32
        h2 = (((h2 << 5) + h2) & M32) ^ b
    return h1, h2 | 1

def probes(word, m):
    h1, h2 = hashes(word)
    return [((h1 + i * h2) & M32) % m for i in range(K)]

valid_plain, accented, _freq, _rank = load(base)
ok = re.compile(r'^[a-zà-ÿ]{2,25}$')
words = sorted({w for w in valid_plain if ok.match(w)} | {w for forms in accented.values() for w in forms if ok.match(w)})
n = len(words)
m = int(math.ceil(n * BITS_PER_WORD / 8)) * 8
bits = bytearray(m // 8)
for w in words:
    for p in probes(w, m):
        bits[p >> 3] |= 1 << (p & 7)

def has(w):
    return all(bits[p >> 3] & (1 << (p & 7)) for p in probes(w, m))

(out / 'ptValid.bin').write_bytes(bytes([K]) + m.to_bytes(4, 'little') + bytes(bits))

random.seed(99)
members = random.sample(words, 400)
def mutate(w):
    i = random.randrange(len(w))
    return w[:i] + random.choice('abcdefghijklmnopqrstuvwxyz') + w[i + 1:]
others = [mutate(w) for w in random.sample(words, 400)] + [''.join(random.choice('abcdefghijklmnopqrstuvwxyz') for _ in range(random.randint(4, 12))) for _ in range(200)]
cases = [[w, has(w)] for w in members + others]
(out / 'spell.fixture.json').write_text(json.dumps(cases, ensure_ascii=False, separators=(',', ':')), encoding='utf-8')
fp = sum(1 for w, v in cases[400:] if v and w not in set(words))
print('palavras:', n, '| bits:', m, '| bytes:', m // 8 + 5, '| k:', K, '| falsos positivos nos 600 casos inexistentes:', fp, file=sys.stderr)
