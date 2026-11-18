"""Leitura dos dados de origem dos geradores do corretor (só dados; nada de terceiros é executado): o dicionário Hunspell VERO pt-BR
expandido (todas as formas) e a lista de frequência de palavras de conversa. Ver gen-pt-accents.py para as fontes e licenças."""
import collections, re, sys, unicodedata
from pathlib import Path


def strip_marks(w):
    return ''.join(c for c in unicodedata.normalize('NFD', w) if unicodedata.category(c) != 'Mn').replace('ç', 'c')


def load(base):
    """Devolve (valid_plain, accented, freq, rank): formas válidas sem nenhum diacrítico; sem acento -> {acentuadas}; lista de frequência; posição."""
    base = Path(base)
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


    return valid_plain, accented, freq, rank
