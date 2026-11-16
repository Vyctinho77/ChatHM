/** CPF (000.000.000-00) ou CNPJ (00.000.000/0000-00) com a pontuação normal; outro valor volta como veio. */
export function formatTaxId(digits: string | undefined): string {
  const d = digits ?? "";
  if (d.length === 11) return `${d.slice(0, 3)}.${d.slice(3, 6)}.${d.slice(6, 9)}-${d.slice(9)}`;
  if (d.length === 14) return `${d.slice(0, 2)}.${d.slice(2, 5)}.${d.slice(5, 8)}/${d.slice(8, 12)}-${d.slice(12)}`;
  return d;
}
