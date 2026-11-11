// Dados fictícios usados até a integração com o WhatsApp (Fase 2).
// Nomes, telefones e empresas são inventados.
import type { Conversation, Message } from "./types";

export const ME = "__me__"; // substituído pelo id do usuário logado

type DistributiveOmit<T, K extends PropertyKey> = T extends unknown ? Omit<T, K> : never;
type Seed = Omit<Conversation, "lastMessage"> & { messages: DistributiveOmit<Message, "id" | "conversationId">[] };

export function buildMock(myId: string, myName: string) {
  const today = new Date();
  const at = (daysAgo: number, h: number, m: number) =>
    new Date(today.getFullYear(), today.getMonth(), today.getDate() - daysAgo, h, m).toISOString();

  // Parecida com a do WhatsApp: 64 amostras, rajadas de fala com pausas e piso em 0,2.
  const wave = (seed: number, n = 64) =>
    Array.from({ length: n }, (_, i) => {
      const word = Math.max(0, Math.sin(seed * 3.1 + i * 0.37)) ** 0.6; // palavras
      const syllable = 0.35 + 0.65 * Math.abs(Math.sin(seed * 5.7 + i * 1.9)); // sílabas
      const noise = 0.85 + 0.15 * Math.sin(seed * 13.3 + i * 7.1);
      return 0.2 + 0.8 * Math.min(1, word * syllable * noise);
    });

  let seq = 0;
  const id = () => `m${++seq}`;

  function svgImage(a: string, b: string, label: string) {
    const svg = `<svg xmlns='http://www.w3.org/2000/svg' width='640' height='420'><defs><linearGradient id='g' x1='0' y1='0' x2='1' y2='1'><stop offset='0' stop-color='${a}'/><stop offset='1' stop-color='${b}'/></linearGradient></defs><rect width='640' height='420' fill='url(#g)'/><text x='50%' y='50%' fill='white' font-family='sans-serif' font-size='28' text-anchor='middle' opacity='.85'>${label}</text></svg>`;
    return `data:image/svg+xml;utf8,${encodeURIComponent(svg)}`;
  }


  const seeds: Seed[] = [
    {
      id: "c1",
      contact: { id: "p1", name: "Rafael Moreira", phone: "+55 92 99811-2040", company: "Moreira Transportes", note: "Prefere contato pela manhã." },
      segment: "notas",
      status: "in_progress",
      assignee: { id: ME, name: "Você" },
      unreadCount: 0,
      tags: [],
      messages: [
        { direction: "in", sentAt: at(2, 13, 52), kind: "audio", durationSec: 8, waveform: wave(1) },
        { direction: "out", sentAt: at(2, 15, 46), status: "read", authorName: ME, kind: "text", text: "já tenho o documento" },
        { direction: "out", sentAt: at(2, 15, 47), status: "read", authorName: ME, kind: "document", fileName: "PROCESSO_2026_0458-NFSe.pdf", mimeType: "application/pdf", sizeBytes: 2_100_000 },
        { direction: "out", sentAt: at(2, 16, 0), status: "read", authorName: ME, kind: "location", lat: -3.1019, lng: -60.025, name: "Certificadora Exemplo", address: "Av. Exemplo, 100 — Centro, Manaus/AM" },
        { direction: "out", sentAt: at(2, 16, 54), status: "read", authorName: ME, kind: "audio", durationSec: 22, waveform: wave(2) },
        { direction: "in", sentAt: at(0, 8, 47), kind: "image", url: svgImage("#0f766e", "#1e3a8a", "Foto do documento"), width: 640, height: 420, caption: "Segue a foto do cartão CNPJ" },
        { direction: "out", sentAt: at(0, 8, 48), status: "read", authorName: ME, kind: "document", fileName: "PGDAS-RECIBO-10.2026.pdf", mimeType: "application/pdf", sizeBytes: 5_200 },
        { direction: "out", sentAt: at(0, 8, 48), status: "read", authorName: ME, kind: "image", url: svgImage("#166534", "#0f766e", "Pendências do Simples"), width: 640, height: 420, caption: "Tinha essas pendências de Simples e Parcelamento" },
        { direction: "out", sentAt: at(0, 8, 49), status: "delivered", authorName: ME, kind: "image", url: svgImage("#1e3a8a", "#6d28d9", "Comprovante"), width: 640, height: 420 },
        { direction: "in", sentAt: at(0, 8, 49), kind: "audio", durationSec: 5, waveform: wave(3) },
        { direction: "out", sentAt: at(0, 8, 51), status: "read", authorName: ME, kind: "text", text: "se quiser nós podemos emitir a nota pra você" },
        { direction: "out", sentAt: at(0, 8, 51), status: "delivered", authorName: ME, kind: "text", text: "Bom dia!" },
        { direction: "in", sentAt: at(0, 8, 53), kind: "audio", durationSec: 15, waveform: wave(4) },
      ],
    },
    {
      id: "c2",
      contact: { id: "p2", name: "Camila Torres", phone: "+55 11 98450-3321", note: "" },
      segment: "geral",
      status: "new",
      unreadCount: 1,
      tags: [],
      messages: [{ direction: "in", sentAt: at(0, 9, 0), kind: "audio", durationSec: 12, waveform: wave(5) }],
    },
    {
      id: "cg1",
      contact: { id: "pg1", name: "Condomínio Bella Vista", phone: "", kind: "group", note: "" },
      segment: "geral",
      status: "new",
      unreadCount: 2,
      tags: [],
      messages: [
        { direction: "in", sentAt: at(0, 8, 41), kind: "text", text: "Bom dia! Alguém consegue a certidão negativa do condomínio?", authorName: "Marta Lima", group: true },
        { direction: "in", sentAt: at(0, 8, 43), kind: "text", text: "Eu envio ainda hoje", authorName: "João Ribeiro", group: true },
        { direction: "in", sentAt: at(0, 8, 44), kind: "text", text: "Obrigada!", authorName: "Marta Lima", group: true },
      ],
    },
    {
      id: "c3",
      contact: { id: "p3", name: "Drogaria Bom Preço", phone: "+55 48 99102-7788", company: "Drogaria Bom Preço LTDA", note: "" },
      segment: "folha",
      status: "in_progress",
      assignee: { id: "u-outro", name: "Marina" },
      unreadCount: 0,
      tags: [],
      messages: [
        { direction: "in", sentAt: at(0, 8, 50), kind: "text", text: "Bom dia, preciso calcular a folha deste mês, teria como gerar um adiantamento?" },
        { direction: "out", sentAt: at(0, 8, 55), status: "read", authorName: "Marina", kind: "text", text: "Bom dia, teria como gerar um comprovante do adiantamento para conferência?" },
      ],
    },
    {
      id: "c4",
      contact: { id: "p4", name: "Luciana Prado Arquitetura", phone: "+55 21 99777-1200", company: "LP Arquitetura ME", note: "Certificado A1 vence em dezembro." },
      segment: "notas",
      status: "in_progress",
      assignee: { id: ME, name: "Você" },
      unreadCount: 0,
      tags: ["ACOMPANHAMENTO", "CERTIFICADO"],
      messages: [{ direction: "in", sentAt: at(0, 8, 46), kind: "document", fileName: "PGDAS-EXTRATO-10.2026.pdf", mimeType: "application/pdf", sizeBytes: 182_000 }],
    },
    {
      id: "c5",
      contact: { id: "p5", name: "Mel Confeitaria", phone: "+55 31 98888-4545", note: "" },
      segment: "clientes_novos",
      status: "new",
      unreadCount: 3,
      tags: ["URGENTE"],
      messages: [
        { direction: "in", sentAt: at(0, 8, 40), kind: "text", text: "Olá! Quero contratar a contabilidade para minha confeitaria" },
        { direction: "in", sentAt: at(0, 8, 41), kind: "text", text: "Vocês fazem orçamento?" },
        { direction: "in", sentAt: at(0, 8, 42), kind: "audio", durationSec: 16, waveform: wave(6) },
      ],
    },
    {
      id: "c6",
      contact: { id: "p6", name: "Bruno Kamei Motopeças", phone: "+55 41 99654-0098", company: "Kamei Motopeças", note: "" },
      segment: "notas",
      status: "closed",
      assignee: { id: ME, name: "Você" },
      unreadCount: 0,
      tags: ["PGDAS"],
      messages: [{ direction: "out", sentAt: at(0, 8, 40), status: "read", authorName: ME, kind: "document", fileName: "PGDAS-RECIBO-10.2026.pdf", mimeType: "application/pdf", sizeBytes: 5_100 }],
    },
    {
      id: "c7",
      contact: { id: "p7", name: "+55 48 9210-1900", phone: "+55 48 9210-1900", note: "" },
      segment: "geral",
      status: "new",
      unreadCount: 0,
      tags: [],
      messages: [{ direction: "in", sentAt: at(0, 8, 39), kind: "text", text: "bom dia" }],
    },
    {
      id: "c8",
      contact: { id: "p8", name: "Silvia Andrade Clínica", phone: "+55 85 99123-6677", company: "Clínica Andrade", note: "" },
      segment: "folha",
      status: "new",
      unreadCount: 2,
      tags: ["PENDENTE"],
      messages: [
        { direction: "in", sentAt: at(1, 17, 20), kind: "text", text: "Preciso de férias de funcionário para o mês que vem" },
        { direction: "in", sentAt: at(1, 17, 21), kind: "text", text: "Qual o prazo?" },
      ],
    },
    {
      id: "c9",
      contact: { id: "p9", name: "Otávio Nunes", phone: "+55 62 98111-0202", note: "" },
      segment: "clientes_novos",
      status: "in_progress",
      assignee: { id: ME, name: "Você" },
      unreadCount: 0,
      tags: [],
      messages: [
        { direction: "in", sentAt: at(3, 10, 5), kind: "text", text: "Quero abrir um MEI, vocês ajudam?" },
        { direction: "out", sentAt: at(3, 10, 30), status: "read", authorName: ME, kind: "text", text: "Ajudamos sim! Vou te enviar a lista de documentos." },
      ],
    },
  ];

  const fix = <T extends { id: string; name: string } | undefined>(a: T) =>
    a && a.id === ME ? { id: myId, name: myName } : a;
  const messages: Record<string, Message[]> = {};
  const conversations: Conversation[] = seeds.map(({ messages: ms, ...c }) => {
    const full = ms.map((m) => {
      const t = new Date(m.sentAt).getTime();
      return {
        ...m,
        id: id(),
        conversationId: c.id,
        authorName: m.authorName === ME ? myName : m.authorName,
        // Recibos fictícios: entregue em segundos, lida alguns minutos depois.
        deliveredAt:
          m.status === "delivered" || m.status === "read" ? new Date(t + 15_000).toISOString() : undefined,
        readAt: m.status === "read" ? new Date(t + 60_000 * (1 + (t / 60_000) % 4)).toISOString() : undefined,
      };
    }) as Message[];
    messages[c.id] = full;
    return { ...c, assignee: fix(c.assignee), lastMessage: full[full.length - 1]! };
  });
  return { conversations, messages };
}
