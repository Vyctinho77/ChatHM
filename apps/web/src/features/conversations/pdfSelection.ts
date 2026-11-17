/**
 * Seleção de texto sem "pulos".
 *
 * A camada de texto do PDF é feita de pedaços de texto soltos (posição absoluta) sobre a página. Quando o mouse passa por um espaço
 * sem texto durante o arrasto, o navegador não sabe onde está o "ponto de chegada" e joga a seleção para o fim (ou o começo) da
 * página: vários campos se selecionam sozinhos. O visualizador do pdf.js resolve com um elemento invisível (`.endOfContent`) que
 * cobre a página durante o arrasto e fica colado ao último pedaço de texto sob o mouse: no espaço vazio, a seleção "gruda" ali
 * em vez de saltar. Este módulo faz o mesmo para o nosso visualizador, em todas as páginas ao mesmo tempo.
 */

const LAYER = ".pdf-text-layer";

/** Volta o marcador para o fim da camada e tira o estado "selecionando". */
function reset(layer: HTMLElement) {
  const end = layer.querySelector<HTMLElement>(".endOfContent");
  if (end) {
    end.style.width = "";
    end.style.height = "";
    end.style.userSelect = "";
    layer.append(end);
  }
  layer.classList.remove("selecting");
}

/** Pedaço de texto (elemento) onde a seleção termina/começa, e se é o começo que se move. */
function anchorOf(range: Range, prev: Range | undefined): { el: Element | null; modifyStart: boolean } {
  const modifyStart = !!prev && (range.compareBoundaryPoints(Range.END_TO_END, prev) === 0 || range.compareBoundaryPoints(Range.START_TO_END, prev) === 0);
  let node: Node | null = modifyStart ? range.startContainer : range.endContainer;
  if (node.nodeType === Node.TEXT_NODE) node = node.parentNode;
  if (!modifyStart && range.endOffset === 0 && node) {
    // O fim está no começo de um elemento: o último texto de fato selecionado é o anterior.
    do {
      while (node && !node.previousSibling) node = node.parentNode;
      node = node?.previousSibling ?? null;
    } while (node && !node.childNodes.length);
  }
  return { el: node instanceof Element ? node : null, modifyStart };
}

/** Liga o comportamento a todas as camadas de texto dentro de `root`. Devolve a função que desliga. */
export function installTextLayerSelection(root: HTMLElement): () => void {
  let prev: Range | undefined;
  let down = false;
  const layers = () => Array.from(root.querySelectorAll<HTMLElement>(LAYER));

  const onDown = (e: PointerEvent) => {
    if (e.button !== 0 || !(e.target instanceof Element) || !e.target.closest(LAYER)) return;
    down = true;
    for (const l of layers()) l.classList.add("selecting");
  };
  const onUp = () => {
    down = false;
    prev = undefined;
    for (const l of layers()) reset(l);
  };
  const onChange = () => {
    if (!down) return;
    const sel = document.getSelection();
    if (!sel || sel.rangeCount === 0) return;
    const range = sel.getRangeAt(0);
    const { el, modifyStart } = anchorOf(range, prev);
    const layer = el?.parentElement?.closest<HTMLElement>(LAYER);
    const end = layer?.querySelector<HTMLElement>(".endOfContent");
    if (el && layer && end && el.parentElement) {
      end.style.userSelect = "text";
      el.parentElement.insertBefore(end, modifyStart ? el : el.nextSibling);
    }
    prev = range.cloneRange();
  };

  root.addEventListener("pointerdown", onDown);
  window.addEventListener("pointerup", onUp);
  window.addEventListener("pointercancel", onUp);
  window.addEventListener("blur", onUp);
  document.addEventListener("selectionchange", onChange);
  return () => {
    root.removeEventListener("pointerdown", onDown);
    window.removeEventListener("pointerup", onUp);
    window.removeEventListener("pointercancel", onUp);
    window.removeEventListener("blur", onUp);
    document.removeEventListener("selectionchange", onChange);
    for (const l of layers()) reset(l);
  };
}
