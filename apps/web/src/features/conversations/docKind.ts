/** Tipo de documento pelo que a pessoa reconhece: PDF, Word, Excel, PowerPoint, certificado digital, XML ou outro. */
export type DocKind = "pdf" | "word" | "excel" | "powerpoint" | "certificate" | "xml" | "other";

const EXT: Record<string, DocKind> = {
  pdf: "pdf",
  doc: "word", docx: "word", docm: "word", dot: "word", dotx: "word", rtf: "word", odt: "word",
  xls: "excel", xlsx: "excel", xlsm: "excel", xlsb: "excel", xlt: "excel", xltx: "excel", csv: "excel", ods: "excel",
  pfx: "certificate", p12: "certificate", cer: "certificate", crt: "certificate", der: "certificate", pem: "certificate", p7b: "certificate", p7c: "certificate", p7s: "certificate",
  xml: "xml",
  ppt: "powerpoint", pptx: "powerpoint", pptm: "powerpoint", pps: "powerpoint", ppsx: "powerpoint", odp: "powerpoint",
};

export function docKind(fileName: string | undefined, mimeType = ""): DocKind {
  fileName ??= ""; // documentos sem nome também chegam do WhatsApp
  const ext = fileName.includes(".") ? fileName.split(".").pop()!.toLowerCase() : "";
  if (EXT[ext]) return EXT[ext];
  const mime = (mimeType ?? "").toLowerCase();
  if (mime === "application/pdf") return "pdf";
  if (mime.includes("wordprocessingml") || mime === "application/msword") return "word";
  if (mime.includes("spreadsheetml") || mime === "application/vnd.ms-excel" || mime === "text/csv") return "excel";
  if (mime.includes("presentationml") || mime === "application/vnd.ms-powerpoint") return "powerpoint";
  if (mime === "application/xml" || mime === "text/xml") return "xml";
  if (/^application\/x-(pkcs12|pkcs7|x509|pem)|^application\/pkcs7/.test(mime)) return "certificate";
  return "other";
}

/** XML de nota fiscal eletrônica (NF-e, NFC-e, CT-e, NFS-e): pelo nome do arquivo, que costuma levar a chave de 44 dígitos ou "nfe". */
export const isInvoiceXml = (fileName: string | undefined) => !!fileName && /\.xml$/i.test(fileName) && /\d{44}|nf-?s?-?e|nfce|(^|[^a-z])cte([^a-z]|$)/i.test(fileName);

/** Aplicativo que abre o arquivo no computador (Office), ou null para PDF/outros. */
export const officeApp: Record<DocKind, string | null> = { pdf: null, word: "Word", excel: "Excel", powerpoint: "PowerPoint", certificate: null, xml: null, other: null };

export const docKindLabel = (kind: DocKind, ext = "") => (kind === "other" ? ext.toUpperCase() || "Arquivo" : { pdf: "PDF", word: "Word", excel: "Excel", powerpoint: "PowerPoint", certificate: "Certificado digital", xml: "XML" }[kind]);

/** Formatos que o WhatsApp aceita como foto; qualquer outro arquivo (inclusive gif/heic) vai como documento. */
export const IMAGE_TYPES = ["image/jpeg", "image/png", "image/webp"];
