import { describe, expect, it } from "vitest";
import { copyPopupPlace, lastLineBox } from "./pdfSelectionCopy";

const line = (left: number, right: number, top: number, bottom = top + 20) => ({ left, right, top, bottom });

describe("botão Copiar junto da seleção do PDF", () => {
  it("fica em cima do fim da seleção", () => {
    expect(copyPopupPlace(line(100, 400, 300), 1200)).toEqual({ x: 400, y: 292, below: false });
  });

  it("não sai da tela pelos lados", () => {
    expect(copyPopupPlace(line(0, 10, 300), 1200).x).toBe(64);
    expect(copyPopupPlace(line(1100, 1195, 300), 1200).x).toBe(1136);
  });

  it("sem espaço em cima (seleção no topo), vai para baixo da linha", () => {
    expect(copyPopupPlace(line(100, 400, 70, 90), 1200, 60)).toEqual({ x: 400, y: 98, below: true });
  });

  it("usa a última linha com tamanho de verdade", () => {
    const rects = [line(10, 300, 100), line(10, 200, 130), line(200, 200, 160)]; // o último é vazio
    expect(lastLineBox(rects)).toEqual(line(10, 200, 130));
    expect(lastLineBox([line(5, 5, 5, 5)])).toBeNull();
    expect(lastLineBox([])).toBeNull();
  });
});
