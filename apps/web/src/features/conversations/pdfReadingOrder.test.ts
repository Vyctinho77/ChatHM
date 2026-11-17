import { describe, expect, it } from "vitest";
import { readingOrder, type Box } from "./pdfReadingOrder";

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

  it("não mistura as linhas de uma tabela: segue célula a célula, de cima para baixo", () => {
    const boxes = [
      box(0, 0, 80, 6), box(300, 0, 80, 6), // rótulos da linha 1
      box(0, 60, 80, 6), box(300, 60, 80, 6), // rótulos da linha 2
      box(0, 12, 100, 12), box(300, 12, 50, 12), // valores da linha 1
      box(0, 72, 100, 12), box(300, 72, 50, 12), // valores da linha 2
    ];
    const out = readingOrder(boxes);
    // Célula a célula, linha por linha: (rótulo+valor) esquerda, (rótulo+valor) direita; depois a linha 2.
    expect(out).toEqual([0, 4, 1, 5, 2, 6, 3, 7]);
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
});
