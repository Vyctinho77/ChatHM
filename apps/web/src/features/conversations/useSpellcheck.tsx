import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState, type MouseEvent, type RefObject } from "react";
import { findUnknownWords, suggest, type Unknown } from "./spell";
import { warmUpSpelling } from "./warmup";
import { SpellCard } from "./SpellCheck";
import { ignoredWords, ignoreWord } from "./writingPrefs";

/**
 * Sublinhado (vermelho, ondulado) das palavras que não existem e cartão de sugestões no botão direito, para uma caixa de texto.
 * A caixa continua sendo um <textarea>; o sublinhado é desenhado numa camada transparente por trás dela, com o mesmo texto e as mesmas
 * medidas, e o botão direito só abre o cartão em cima de uma palavra sublinhada (em qualquer outro lugar, o menu normal do navegador).
 */
export function useSpellcheck(o: {
  ref: RefObject<HTMLTextAreaElement | null>;
  text: string;
  caret: number;
  /** Troca o trecho [start, end) por `to` e põe o cursor depois dele. */
  onReplace: (start: number, end: number, to: string) => void;
}) {
  const { ref, text, caret, onReplace } = o;
  const [ready, setReady] = useState(false);
  const [tick, setTick] = useState(0); // palavras ignoradas mudaram
  const [card, setCard] = useState<{ x: number; y: number; hit: Unknown; options: string[] } | null>(null);
  const backdrop = useRef<HTMLDivElement>(null);

  // Os dados carregam uma vez, em segundo plano, quando aparece o primeiro texto.
  const hasText = text.length > 0;
  useEffect(() => {
    if (!hasText || ready) return;
    let alive = true;
    void warmUpSpelling().then(() => alive && setReady(true));
    return () => {
      alive = false;
    };
  }, [hasText, ready]);

  const unknown = useMemo(() => (ready ? findUnknownWords(text, caret, ignoredWords()) : []), [ready, text, caret, tick]); // eslint-disable-line react-hooks/exhaustive-deps

  const syncScroll = useCallback(() => {
    if (backdrop.current && ref.current) backdrop.current.scrollTop = ref.current.scrollTop;
  }, [ref]);
  useLayoutEffect(syncScroll, [syncScroll, text]);

  function onContextMenu(e: MouseEvent<HTMLTextAreaElement>) {
    if (!backdrop.current || unknown.length === 0) return;
    const spans = backdrop.current.querySelectorAll<HTMLElement>("[data-spell]");
    for (const span of spans) {
      for (const r of span.getClientRects()) {
        if (e.clientX >= r.left && e.clientX <= r.right && e.clientY >= r.top && e.clientY <= r.bottom) {
          const hit = unknown[Number(span.dataset.spell)];
          if (!hit) return;
          e.preventDefault();
          setCard({ x: e.clientX, y: e.clientY, hit, options: suggest(hit.word) });
          return;
        }
      }
    }
  }

  const close = useCallback(() => setCard(null), []);
  const pick = (to: string) => {
    if (!card) return;
    onReplace(card.hit.start, card.hit.end, to);
    setCard(null);
  };
  const ignore = () => {
    if (!card) return;
    ignoreWord(card.hit.word);
    setTick((t) => t + 1);
    setCard(null);
    ref.current?.focus();
  };

  return {
    unknown,
    backdrop,
    onScroll: syncScroll,
    onContextMenu,
    card: card ? <SpellCard x={card.x} y={card.y} word={card.hit.word} options={card.options} onPick={pick} onIgnore={ignore} onClose={close} /> : null,
  };
}
