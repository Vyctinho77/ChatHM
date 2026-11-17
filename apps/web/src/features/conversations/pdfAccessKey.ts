/**
 * Chave de acesso de nota fiscal eletrônica (NF-e, NFC-e, CT-e, MDF-e): 44 dígitos que a DANFE imprime em grupos de 4
 * (com espaços) e que quebram ao copiar na mão. Aqui ela é encontrada no texto do PDF e conferida pelo dígito verificador
 * (módulo 11), pelo código da UF e pelo modelo do documento, para não confundir com outros números longos.
 */

const MODELS = new Set(["55", "65", "57", "58", "59", "62", "63", "67"]);

/** Confere UF (11–53), modelo e dígito verificador (módulo 11, pesos 2 a 9 da direita para a esquerda). */
export function isValidAccessKey(key: string): boolean {
  if (!/^\d{44}$/.test(key)) return false;
  const uf = Number(key.slice(0, 2));
  if (uf < 11 || uf > 53) return false;
  if (!MODELS.has(key.slice(20, 22))) return false;
  let sum = 0;
  let weight = 2;
  for (let i = 42; i >= 0; i--) {
    sum += Number(key[i]) * weight;
    weight = weight === 9 ? 2 : weight + 1;
  }
  const r = sum % 11;
  const dv = r < 2 ? 0 : 11 - r;
  return dv === Number(key[43]);
}

/** Chaves de acesso válidas encontradas no texto (sem repetir, na ordem em que aparecem). */
export function findAccessKeys(text: string): string[] {
  const found: string[] = [];
  // Sequências de dígitos que podem ter espaços ou pontos entre eles (os grupos da DANFE).
  for (const run of text.matchAll(/\d[\d\s.]{40,80}\d/g)) {
    const digits = run[0].replace(/\D/g, "");
    if (digits.length < 44) continue;
    // Se a sequência tem exatamente 44 dígitos, é ela; se for maior (número colado), procura a janela válida.
    for (let i = 0; i + 44 <= digits.length; i++) {
      const cand = digits.slice(i, i + 44);
      if (isValidAccessKey(cand)) {
        if (!found.includes(cand)) found.push(cand);
        i += 43;
      }
      if (digits.length === 44) break;
    }
  }
  return found;
}

/** Formata em grupos de 4, como na DANFE. */
export function formatAccessKey(key: string): string {
  return key.replace(/(\d{4})(?=\d)/g, "$1 ");
}
