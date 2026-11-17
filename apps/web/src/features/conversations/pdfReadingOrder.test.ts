import { describe, expect, it } from "vitest";
import { readingOrder, type Box } from "./pdfReadingOrder";
import type { Cell } from "./pdfCells";

const box = (left: number, top: number, width = 50, height = 10): Box => ({ left, top, width, height });

describe("readingOrder", () => {
  it("lê cada célula inteira (rótulo + valor) antes da célula ao lado, mesmo com o PDF desenhando rótulos e valores separados", () => {
    // Ordem de desenho como numa DANFE: todos os rótulos primeiro, depois os valores.
    const boxes = [
      box(0, 0, 80, 6), // 0: rótulo esquerdo
      box(300, 0, 80, 6), // 1: rótulo direito
      box(0, 12, 120, 12), // 2: valor esquerdo
      box(300, 12, 60, 12), // 3: valor direito
    ];
    expect(readingOrder(boxes)).toEqual([0, 2, 1, 3]);
  });

  it("lê uma tabela coluna a coluna (cada célula inteira, de cima para baixo), sem misturar as colunas", () => {
    const boxes = [
      box(0, 0, 80, 6), box(300, 0, 80, 6), // rótulos da linha 1
      box(0, 60, 80, 6), box(300, 60, 80, 6), // rótulos da linha 2
      box(0, 12, 100, 12), box(300, 12, 50, 12), // valores da linha 1
      box(0, 72, 100, 12), box(300, 72, 50, 12), // valores da linha 2
    ];
    const out = readingOrder(boxes);
    // Coluna da esquerda inteira (rótulo1, valor1, rótulo2, valor2) e depois a da direita.
    expect(out).toEqual([0, 4, 2, 6, 1, 5, 3, 7]);
  });

  it("célula de DANFE: rótulo no alto à esquerda e valor embaixo à direita ficam juntos, longe do rótulo da célula vizinha", () => {
    // Seis colunas de 90 de largura: rótulos pequenos (h 6) no topo, valores (h 9) alinhados à direita e mais abaixo.
    const labels = Array.from({ length: 6 }, (_, i) => box(24 + i * 90, 0, 70, 6));
    const values = Array.from({ length: 6 }, (_, i) => box(70 + i * 90, 20, 35, 9));
    const boxes = [...labels, ...values]; // o PDF desenha todos os rótulos e depois todos os valores
    const out = readingOrder(boxes);
    // rótulo i seguido do seu valor i (índice i + 6), coluna por coluna
    expect(out).toEqual([0, 6, 1, 7, 2, 8, 3, 9, 4, 10, 5, 11]);
  });

  it("junta palavras vizinhas da mesma linha e mantém a ordem da esquerda para a direita", () => {
    const boxes = [box(120, 0, 40), box(0, 0, 50), box(60, 0, 50)];
    expect(readingOrder(boxes)).toEqual([1, 2, 0]);
  });

  it("lê um parágrafo (linhas alinhadas à esquerda) de cima para baixo", () => {
    const boxes = [box(0, 24, 200), box(0, 0, 200), box(0, 12, 200)];
    expect(readingOrder(boxes)).toEqual([1, 2, 0]);
  });

  it("textos distantes na vertical não viram o mesmo bloco", () => {
    const boxes = [box(0, 0), box(0, 200)];
    expect(readingOrder(boxes)).toEqual([0, 1]);
  });

  it("deixa elementos sem tamanho no fim, sem perder nenhum", () => {
    const boxes = [box(0, 0, 0, 0), box(10, 0), box(0, 0, 0, 0)];
    const out = readingOrder(boxes);
    expect(out).toEqual([1, 0, 2]);
    expect([...out].sort()).toEqual([0, 1, 2]);
  });

  it("com células: o rótulo e o valor de cada campo ficam juntos, mesmo longe um do outro e sem sobreposição", () => {
    const cellA: Cell = { id: "A", left: 0, top: 0, width: 90, height: 45 };
    const cellB: Cell = { id: "B", left: 90, top: 0, width: 90, height: 45 };
    const boxes = [
      box(4, 4, 30, 5), // 0: rótulo curto de A (no alto, à esquerda)
      box(94, 4, 40, 5), // 1: rótulo de B
      box(60, 30, 25, 9), // 2: valor de A (embaixo, à direita)
      box(150, 30, 25, 9), // 3: valor de B
    ];
    expect(readingOrder(boxes, [cellA, cellB, cellA, cellB])).toEqual([0, 2, 1, 3]);
  });

  it("com células: as células seguem linha por linha, da esquerda para a direita", () => {
    const c = (id: string, left: number, top: number): Cell => ({ id, left, top, width: 90, height: 45 });
    const cells = [c("d", 90, 50), c("a", 0, 0), c("c", 0, 50), c("b", 90, 0)];
    const boxes = [box(94, 54), box(4, 4), box(4, 54), box(94, 4)]; // d, a, c, b
    expect(readingOrder(boxes, cells)).toEqual([1, 3, 2, 0]); // a, b, c, d
  });

  it("textos fora de qualquer célula continuam pela ordem de leitura", () => {
    const cell: Cell = { id: "A", left: 0, top: 50, width: 90, height: 45 };
    const boxes = [box(4, 54), box(0, 0)]; // 0 numa célula, 1 solto no alto
    expect(readingOrder(boxes, [cell, undefined])).toEqual([1, 0]);
  });
});
