// Datas da agenda. Trabalha com "YYYY-MM-DD" no fuso local (sem horário), para que um
// prazo em 13/11 caia sempre no dia 13, em qualquer fuso do navegador.

const pad = (n: number) => String(n).padStart(2, "0");

export const toISO = (d: Date) => `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;

export function fromISO(s: string): Date {
  const [y, m, d] = s.split("-").map(Number);
  return new Date(y!, m! - 1, d!);
}

export const addDays = (d: Date, n: number) => new Date(d.getFullYear(), d.getMonth(), d.getDate() + n);

export const sameMonth = (a: Date, month: Date) => a.getFullYear() === month.getFullYear() && a.getMonth() === month.getMonth();

/**
 * Dias exibidos no calendário de um mês: semanas completas começando no domingo
 * (4 a 6 semanas, só as necessárias).
 */
export function monthGrid(month: Date): Date[] {
  const first = new Date(month.getFullYear(), month.getMonth(), 1);
  const daysInMonth = new Date(month.getFullYear(), month.getMonth() + 1, 0).getDate();
  const weeks = Math.ceil((first.getDay() + daysInMonth) / 7);
  const start = addDays(first, -first.getDay());
  return Array.from({ length: weeks * 7 }, (_, i) => addDays(start, i));
}

export const addMonths = (d: Date, n: number) => new Date(d.getFullYear(), d.getMonth() + n, 1);

export const weekdayShort = ["dom", "seg", "ter", "qua", "qui", "sex", "sáb"] as const;

const monthFmt = new Intl.DateTimeFormat("pt-BR", { month: "long", year: "numeric" });
const longFmt = new Intl.DateTimeFormat("pt-BR", { weekday: "long", day: "numeric", month: "long" });

export const monthLabel = (d: Date) => monthFmt.format(d);
export const longDate = (iso: string) => longFmt.format(fromISO(iso));
