/** Versão média de uma foto (a que cabe no balão). Só os anexos do servidor têm; o resto (blob, demonstração) segue como está. */
export function previewUrl(url?: string): string | undefined {
  return url && url.startsWith("/api/v1/attachments/") && !url.includes("?") ? `${url}?size=preview` : url;
}
