import { useMemo, useState } from "react";
import { ChevronLeft, ChevronRight, Plus } from "lucide-react";
import { Page, PageHeader } from "@/components/list";
import { Alert, Button, IconButton, Spinner } from "@/components/ui";
import { errorMessage } from "@/lib/api";
import { cn } from "@/lib/cn";
import { useMediaQuery } from "@/lib/theme";
import { useConversations } from "@/features/conversations/store";
import { AgendaCardContent, type CardView, type ContactOption } from "./AgendaCard";
import { addMonths, monthGrid, monthLabel, sameMonth, toISO, weekdayShort } from "./dates";
import { kindDot, sortItems, useAgendaItems, type AgendaItem } from "./data";
import { FloatingCard } from "./FloatingCard";

const MAX_CHIPS = 3;
const cap = (s: string) => s.charAt(0).toUpperCase() + s.slice(1);

type Open = { view: CardView; anchor: DOMRect } | null;

/** Calendário mensal; clicar num item abre um card flutuante. */
export function AgendaPage() {
  const wide = useMediaQuery("(min-width: 768px)");
  const [today] = useState(() => new Date());
  const [month, setMonth] = useState(() => new Date(today.getFullYear(), today.getMonth(), 1));
  const [open, setOpen] = useState<Open>(null);
  const { items, teamNames, isPending, error } = useAgendaItems();
  const { data: conversations = [] } = useConversations();

  const contacts = useMemo<ContactOption[]>(() => {
    const seen = new Set<string>();
    return conversations
      .filter((c) => !seen.has(c.contact.id) && seen.add(c.contact.id))
      .map((c) => ({ name: c.contact.name, conversationId: c.id }))
      .sort((a, b) => a.name.localeCompare(b.name));
  }, [conversations]);

  const byDate = useMemo(() => {
    const m = new Map<string, AgendaItem[]>();
    for (const i of items) m.set(i.date, [...(m.get(i.date) ?? []), i]);
    for (const [k, v] of m) m.set(k, sortItems(v));
    return m;
  }, [items]);

  const days = monthGrid(month);
  const rows = days.length / 7;
  const todayISO = toISO(today);
  const show = (view: CardView, el: Element) => setOpen({ view, anchor: el.getBoundingClientRect() });
  const goToday = () => setMonth(new Date(today.getFullYear(), today.getMonth(), 1));

  return (
    <Page>
      <div className="flex h-full flex-col">
        <PageHeader
          title="Agenda"
          actions={
            <Button variant="secondary" disabled={isPending || !!error || teamNames.length === 0} onClick={(e) => show({ type: "form", date: todayISO }, e.currentTarget)}>
              <Plus className="size-4" /> Novo
            </Button>
          }
        />

        {error && <div className="px-3 py-2"><Alert>{errorMessage(error)}</Alert></div>}
        {!isPending && !error && teamNames.length === 0 && (
          <p className="px-5 pb-1 pt-3 text-xs leading-relaxed text-muted">
            Para criar compromissos, cadastre ou participe de uma equipe.
          </p>
        )}
        <div className="flex items-center gap-1 px-3 py-2">
          <IconButton label="Mês anterior" onClick={() => setMonth((m) => addMonths(m, -1))}>
            <ChevronLeft className="size-5" />
          </IconButton>
          <IconButton label="Próximo mês" onClick={() => setMonth((m) => addMonths(m, 1))}>
            <ChevronRight className="size-5" />
          </IconButton>
          <h2 className="ml-1 text-[16px] font-semibold">{cap(monthLabel(month))}</h2>
          <button
            type="button"
            onClick={goToday}
            className="ml-2 rounded-lg px-2.5 py-1 text-[13px] font-medium text-accent hover:bg-hover"
          >
            Hoje
          </button>
          <div className="ml-auto hidden items-center gap-4 text-[12px] text-muted sm:flex">
            <span className="flex items-center gap-1.5">
              <span className={cn("size-2 rounded-full", kindDot("appointment"))} /> Compromisso
            </span>
            <span className="flex items-center gap-1.5">
              <span className={cn("size-2 rounded-full", kindDot("deadline"))} /> Prazo
            </span>
          </div>
        </div>

        {isPending ? (
          <Spinner className="mx-auto mt-10" />
        ) : (
          <div className="flex min-h-0 flex-1 flex-col border-t border-line" role="grid" aria-label={cap(monthLabel(month))}>
            <div role="row" className="grid grid-cols-7 border-b border-line">
              {weekdayShort.map((w) => (
                <div key={w} role="columnheader" className="px-2 py-1.5 text-[11.5px] font-medium uppercase tracking-wide text-muted">
                  {w}
                </div>
              ))}
            </div>
            <div className="grid min-h-0 flex-1 grid-cols-7" style={{ gridTemplateRows: `repeat(${rows}, minmax(0, 1fr))` }}>
              {days.map((d) => {
                const iso = toISO(d);
                const list = byDate.get(iso) ?? [];
                const inMonth = sameMonth(d, month);
                const isToday = iso === todayISO;
                return (
                  <div
                    key={iso}
                    role="gridcell"
                    // Célula vazia: no computador cria um item nesse dia; no celular abre a lista do dia.
                    onClick={(e) => show(wide ? { type: "form", date: iso } : { type: "day", date: iso }, e.currentTarget)}
                    className="flex min-h-0 cursor-pointer flex-col gap-0.5 overflow-hidden border-b border-r border-line p-1 transition-colors hover:bg-hover/60 [&:nth-child(7n)]:border-r-0"
                  >
                    <button
                      type="button"
                      aria-label={`Dia ${d.getDate()}`}
                      onClick={(e) => {
                        e.stopPropagation();
                        show({ type: "day", date: iso }, e.currentTarget);
                      }}
                      className={cn(
                        "flex size-6 shrink-0 items-center justify-center self-start rounded-full text-[12px] tabular-nums",
                        isToday ? "bg-accent font-semibold text-accent-fg" : inMonth ? "text-fg hover:bg-hover" : "text-muted/60 hover:bg-hover",
                      )}
                    >
                      {d.getDate()}
                    </button>

                    {wide ? (
                      <>
                        {list.slice(0, MAX_CHIPS).map((i) => (
                          <button
                            key={i.id}
                            type="button"
                            onClick={(e) => {
                              e.stopPropagation();
                              show({ type: "item", id: i.id }, e.currentTarget);
                            }}
                            className={cn(
                              "flex min-w-0 items-center gap-1.5 rounded px-1.5 py-0.5 text-left text-[11.5px] leading-tight hover:bg-hover",
                              !inMonth && "opacity-60",
                              i.done && "text-muted line-through",
                            )}
                          >
                            <span className={cn("size-1.5 shrink-0 rounded-full", kindDot(i.kind))} />
                            <span className="truncate">
                              {i.time && <span className="mr-1 tabular-nums text-muted">{i.time}</span>}
                              {i.title}
                            </span>
                          </button>
                        ))}
                        {list.length > MAX_CHIPS && (
                          <button
                            type="button"
                            onClick={(e) => {
                              e.stopPropagation();
                              show({ type: "day", date: iso }, e.currentTarget);
                            }}
                            className="rounded px-1.5 py-0.5 text-left text-[11.5px] font-medium text-muted hover:bg-hover hover:text-fg"
                          >
                            +{list.length - MAX_CHIPS} mais
                          </button>
                        )}
                      </>
                    ) : (
                      list.length > 0 && (
                        <span className="mt-auto flex flex-wrap justify-center gap-0.5 pb-0.5" aria-hidden>
                          {list.slice(0, 4).map((i) => (
                            <span key={i.id} className={cn("size-1.5 rounded-full", kindDot(i.kind))} />
                          ))}
                        </span>
                      )
                    )}
                  </div>
                );
              })}
            </div>
          </div>
        )}
      </div>

      {open && (
        <FloatingCard anchor={open.anchor} label="Agenda" onClose={() => setOpen(null)}>
          <AgendaCardContent
            view={open.view}
            items={items}
            teamNames={teamNames}
            contacts={contacts}
            onView={(view) => setOpen((o) => (o ? { ...o, view } : o))}
            onClose={() => setOpen(null)}
          />
        </FloatingCard>
      )}
    </Page>
  );
}
