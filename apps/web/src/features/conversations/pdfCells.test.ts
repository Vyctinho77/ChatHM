import { describe, expect, it } from "vitest";
import { cellAt, extractLines, type Lines } from "./pdfCells";

const OPS = { save: 10, restore: 11, transform: 12, constructPath: 91, endPath: 28 };
const flip = (x: number, y: number): [number, number] => [x, 100 - y]; // PDF (y para cima) → tela (y para baixo)
// Caminho de retângulo no formato do pdf.js: mover, 3 linhas, fechar.
const rect = (x: number, y: number, w: number, h: number) => [0, x, y, 1, x + w, y, 1, x + w, y + h, 1, x, y + h, 4];
const stroke = (data: number[]) => [20, [data], null];

describe("extractLines", () => {
  it("um retângulo vira duas linhas horizontais e duas verticais, em px da tela", () => {
    const l = extractLines([OPS.constructPath], [stroke(rect(10, 20, 40, 30))], OPS, flip);
    expect(l.h.map((x) => [x.a, x.b, x.at]).sort()).toEqual([[10, 50, 50], [10, 50, 80]]);
    expect(l.v.map((x) => [x.a, x.b, x.at]).sort()).toEqual([[50, 80, 10], [50, 80, 50]]);
  });

  it("aplica a transformação do PDF (cm) e a desfaz no restore", () => {
    const l = extractLines(
      [OPS.save, OPS.transform, OPS.constructPath, OPS.restore, OPS.constructPath],
      [null, [2, 0, 0, 2, 0, 0], stroke(rect(5, 5, 10, 10)), null, stroke(rect(5, 5, 10, 10))],
      OPS,
      flip,
    );
    const widths = l.h.map((x) => x.b - x.a).sort((a, b) => a - b);
    expect(widths).toEqual([10, 10, 20, 20]); // duas na escala 2 (20) e duas sem escala (10)
  });

  it("ignora caminho só de recorte (endPath), linhas inclinadas e traços curtíssimos", () => {
    const l = extractLines(
      [OPS.constructPath, OPS.constructPath, OPS.constructPath],
      [[OPS.endPath, [null], null], stroke([0, 0, 0, 1, 30, 30]), stroke([0, 0, 0, 1, 2, 0])],
      OPS,
      flip,
    );
    expect(l).toEqual({ h: [], v: [] });
  });
});

describe("cellAt", () => {
  // Grade 2x2: quadrados de 50, de (0,0) a (100,100).
  const grid: Lines = {
    h: [0, 50, 100].map((at) => ({ a: 0, b: 100, at })),
    v: [0, 50, 100].map((at) => ({ a: 0, b: 100, at })),
  };
  const page = { width: 1000, height: 1000 };

  it("acha o retângulo que envolve o ponto", () => {
    expect(cellAt(75, 25, grid, page)).toMatchObject({ left: 50, top: 0, width: 50, height: 50 });
  });

  it("dois pontos da mesma célula têm o mesmo id; células vizinhas, ids diferentes", () => {
    expect(cellAt(60, 10, grid, page)!.id).toBe(cellAt(95, 45, grid, page)!.id);
    expect(cellAt(60, 10, grid, page)!.id).not.toBe(cellAt(10, 10, grid, page)!.id);
  });

  it("fora de qualquer quadro (faltando um lado) não é célula", () => {
    expect(cellAt(150, 25, grid, page)).toBeUndefined();
    expect(cellAt(25, -20, grid, page)).toBeUndefined();
  });

  it("retângulo grande demais (a moldura da página) não conta como campo", () => {
    expect(cellAt(75, 25, grid, { width: 60, height: 60 })).toBeUndefined();
  });
});
