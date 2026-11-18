#!/usr/bin/env python3
"""
Gera apps/web/src/features/conversations/ptAccents.data.ts: o mapa "sem acento -> com acento" que o corretor de acentos usa além da lista curada.

Fontes (só LEITURA de dados; nada de terceiros é executado):
  - dicionário VERO pt-BR (LibreOffice), licença LGPL-3.0 ou MPL-2.0: pacote npm "dictionary-pt-br" (index.aff, index.dic). Serve para
    saber quais palavras acentuadas existem na ortografia atual e se a forma SEM acento também é palavra ("esta" x "está": não corrige).
  - frequência de palavras do português falado (OpenSubtitles 2024 via OPUS, licença ODC-BY): pacote npm "@zxcvbn-ts/language-pt-br"
    (dist/commonWords.json.mjs, lista comprimida em ordem de frequência). Serve para ficar só com as palavras que aparecem em conversa.

Como regenerar (numa pasta de trabalho fora do repositório):
  npm i dictionary-pt-br && npm pack @zxcvbn-ts/language-pt-br && tar -xzf zxcvbn-ts-language-pt-br-*.tgz
  cp node_modules/dictionary-pt-br/index.aff pt.aff; cp node_modules/dictionary-pt-br/index.dic pt.dic
  python3 -I scripts/gen-pt-accents.py <pasta> > apps/web/src/features/conversations/ptAccents.data.ts

Regras do mapa: a palavra acentuada existe no VERO; só uma acentuada é possível (ou uma domina claramente); a forma sem acento não é mais
frequente que a acentuada (ex.: "pra"). Duas camadas:
  1) a forma sem acento NÃO é palavra válida (voce, orcamento, informacao);
  2) a forma sem acento é palavra válida mas rara (verbo obscuro: "numero", "historia", "musica") e a acentuada é MUITO mais frequente
     (posição na lista até 12.000 e pelo menos 6 vezes à frente): "numero" vira "número", mas "esta", "pais" e "para" ficam como estão.
Grafias antigas (idéia, vôo) e nomes estrangeiros ficam de fora porque o VERO não os conhece. EXCLUDE tira o que dá mais problema que ajuda
(siglas e risadas); palavras de duas letras só entram se estiverem em TWO_LETTER.
"""
import collections, json, re, sys, unicodedata
from pathlib import Path

base = Path(sys.argv[1] if len(sys.argv) > 1 else '.')

def strip_marks(w):
    return ''.join(c for c in unicodedata.normalize('NFD', w) if unicodedata.category(c) != 'Mn').replace('ç', 'c')

# ---- 1. expande o Hunspell (.aff/.dic) ----
aff = (base / 'pt.aff').read_text(encoding='utf-8').split('\n')
rules = {'PFX': collections.defaultdict(list), 'SFX': collections.defaultdict(list)}
cross = {}
i = 0
while i < len(aff):
    p = aff[i].split()
    if len(p) >= 4 and p[0] in ('PFX', 'SFX') and p[3].isdigit():
        kind, flag, cr, n = p[0], p[1], p[2], int(p[3])
        cross[(kind, flag)] = cr == 'Y'
        for j in range(1, n + 1):
            q = aff[i + j].split()
            strip = '' if q[2] == '0' else q[2]
            add = '' if q[3] == '0' else q[3].split('/')[0]
            cond = q[4] if len(q) > 4 else '.'
            rules[kind][flag].append((strip, add, re.compile(('^' if kind == 'PFX' else '') + cond + ('' if kind == 'PFX' else '$'))))
        i += n + 1
    else:
        i += 1

valid_plain = set()
accented = collections.defaultdict(set)
ok = re.compile(r'^[a-zà-ÿ]+$')

def add(w):
    w = unicodedata.normalize('NFC', w.lower())
    if not ok.match(w) or len(w) < 2:
        return
    s = strip_marks(w)
    (valid_plain.add(w) if s == w else accented[s].add(w))

for ln in (base / 'pt.dic').read_text(encoding='utf-8').split('\n')[1:]:
    if not ln.strip() or ln.startswith('#'):
        continue
    word, _, flags = ln.partition('/')
    word, flags = word.strip(), flags.strip()
    add(word)
    sf = []
    for f in flags:
        for strip, ad, rx in rules['SFX'].get(f, []):
            if rx.search(word) and word.endswith(strip):
                nw = word[:len(word) - len(strip)] + ad if strip else word + ad
                add(nw)
                if cross.get(('SFX', f)):
                    sf.append(nw)
    for f in flags:
        for strip, ad, rx in rules['PFX'].get(f, []):
            if rx.search(word) and (not strip or word.startswith(strip)):
                add(ad + word[len(strip):])
                if cross.get(('PFX', f)):
                    for sw in sf:
                        if sw.startswith(strip):
                            add(ad + sw[len(strip):])

# ---- 2. lista de frequência (formato do @zxcvbn-ts: cada palavra começa com uma letra = quantos caracteres repete da anterior) ----
t = (base / 'zx/package/dist/commonWords.json.mjs').read_text(encoding='utf-8')
packed = re.search(r'decompress\("([^"]*)"', t).group(1)
freq, prev = [], ''
for c, rest in re.findall(r'([A-Z])([^A-Z]+)', packed):
    prev = prev[:ord(c) - 65] + rest
    freq.append(prev)
rank = {}
for k, w in enumerate(freq):
    rank.setdefault(w, k)

# ---- 3. o mapa ----
EXCLUDE = {'ha', 'ai', 'sos', 'sa', 'le', 've', 'va', 'poe', 'cu', 'bla'}
TWO_LETTER = {'ja', 'so', 'la', 'ta', 'ne', 'pe', 'fe', 'ca'}
out = {}
for w in freq:
    if not ok.match(w) or w == strip_marks(w):
        continue
    s = strip_marks(w)
    forms = accented.get(s)
    if not forms or w not in forms or s in EXCLUDE or (len(s) <= 2 and s not in TWO_LETTER):
        continue                         # o VERO não conhece (grafia antiga, nome estrangeiro), ou é sigla/risada/curta demais
    if s in valid_plain:                 # camada 2: a forma sem acento é palavra válida, mas a acentuada domina
        if not (rank[w] <= 12000 and rank.get(s, 10**9) >= 6 * max(rank[w], 50)):
            continue
    if len(forms) > 1:
        c = sorted((f for f in forms if f in rank), key=lambda f: rank[f])
        if not (c and c[0] == w and (len(c) == 1 or rank[c[1]] > 20 * (rank[c[0]] + 50))):
            continue                     # ambígua (vovô/vovó, mantém/mantêm)
    if s in rank and rank[s] < rank[w]:
        continue                         # a forma sem acento é mais usada em conversa (pra, aqui)
    out[s] = w

print('// GERADO por scripts/gen-pt-accents.py: não edite à mão. %d palavras.' % len(out))
print('// Fontes: dicionário VERO pt-BR (LGPL-3.0 ou MPL-2.0, https://pt-br.libreoffice.org/projetos/projeto-vero-verificador-ortografico/) e')
print('// frequência de palavras de OpenSubtitles 2024 via OPUS (ODC-BY, https://opus.nlpl.eu/), pacote @zxcvbn-ts/language-pt-br.')
print('// Formato: "semacento:comacento" separados por espaço.')
print('export const PT_ACCENTS = %s;' % json.dumps(' '.join('%s:%s' % kv for kv in sorted(out.items())), ensure_ascii=False))
