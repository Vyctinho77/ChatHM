import { useState, type FormEvent } from "react";
import { useNavigate } from "react-router";
import { CalendarDays, Clock, MessageSquareText, Plus, UserRound, Users, X } from "lucide-react";
import { Alert, Button, Field, IconButton } from "@/components/ui";
import { errorMessage, fieldErrors } from "@/lib/api";
import { cn } from "@/lib/cn";
import { longDate } from "./dates";
import {
  kindDot,
  kindLabel,
  sortItems,
  useDeleteItem,
  useSaveItem,
  useToggleDone,
  type AgendaItem,
  type AgendaKind,
} from "./data";

export type CardView =
  | { type: "item"; id: string }
  | { type: "day"; date: string }
  | { type: "form"; id?: string; date?: string };

export type ContactOption = { name: string; conversationId: string };

const cap = (s: string) => s.charAt(0).toUpperCase() + s.slice(1);

type Props = {
  view: CardView;
  items: AgendaItem[];
  teamNames: string[];
  contacts: ContactOption[];
  onView: (v: CardView) => void;
  onClose: () => void;
};

export function AgendaCardContent(p: Props) {
  if (p.view.type === "item") {
    const item = p.items.find((i) => i.id === (p.view as { id: string }).id);
    return item ? <ItemView item={item} {...p} /> : <Empty onClose={p.onClose} />;
  }
  if (p.view.type === "day") return <DayView date={p.view.date} {...p} />;
  const editing = p.view.id ? p.items.find((i) => i.id === (p.view as { id?: string }).id) : undefined;
  return <FormView key={p.view.id ?? p.view.date ?? "new"} editing={editing} date={p.view.date} {...p} />;
}

function Header({ title, kind, onClose }: { title: string; kind?: AgendaKind; onClose: () => void }) {
  return (
    <header className="flex items-center gap-2 px-4 pb-1 pt-3">
      {kind && <span className={cn("size-2 rounded-full", kindDot(kind))} />}
      <span className="text-[12px] font-medium text-muted">{title}</span>
      <IconButton label="Fechar" className="-mr-2 ml-auto size-7" onClick={onClose}>
        <X className="size-4" />
      </IconButton>
    </header>
  );
}

function Empty({ onClose }: { onClose: () => void }) {
  return (
    <div>
      <Header title="Item" onClose={onClose} />
      <p className="px-4 pb-5 text-[13px] text-muted">Este item não existe mais.</p>
    </div>
  );
}

function Row({ icon, children }: { icon: React.ReactNode; children: React.ReactNode }) {
  return (
    <div className="flex items-start gap-2.5 text-[13px]">
      <span className="mt-0.5 text-muted [&>svg]:size-4">{icon}</span>
      <div className="min-w-0 flex-1">{children}</div>
    </div>
  );
}

function ItemView({ item, onView, onClose }: Props & { item: AgendaItem }) {
  const navigate = useNavigate();
  const toggle = useToggleDone();
  const remove = useDeleteItem();
  const [confirming, setConfirming] = useState(false);

  return (
    <div>
      <Header title={kindLabel[item.kind]} kind={item.kind} onClose={onClose} />
      <div className="flex flex-col gap-3 px-4 pb-4">
        {(toggle.error || remove.error) && <Alert>{errorMessage(toggle.error ?? remove.error)}</Alert>}
        <h2 className={cn("text-[16px] font-semibold leading-snug", item.done && "text-muted line-through")}>{item.title}</h2>
        <Row icon={<CalendarDays />}>
          {cap(longDate(item.date))}
          {item.time && (
            <span className="ml-1 inline-flex items-center gap-1 text-muted">
              · <Clock className="size-3.5" /> {item.time}
            </span>
          )}
          {item.kind === "deadline" && <span className="text-muted"> · dia inteiro</span>}
        </Row>
        <Row icon={<Users />}>Equipe {item.team}</Row>
        {item.contact && (
          <Row icon={<UserRound />}>
            <span>{item.contact.name}</span>
            <button
              type="button"
              onClick={() => navigate(`/conversas/${item.contact!.conversationId}`)}
              className="ml-2 inline-flex items-center gap-1 text-accent hover:underline"
            >
              <MessageSquareText className="size-3.5" /> Abrir conversa
            </button>
          </Row>
        )}
        {item.note && <p className="whitespace-pre-wrap rounded-lg bg-hover px-3 py-2 text-[13px] leading-snug">{item.note}</p>}

        {confirming ? (
          <div className="flex items-center gap-2 pt-1 text-[13px]">
            <span className="mr-auto">Excluir este item?</span>
            <Button variant="secondary" onClick={() => setConfirming(false)}>
              Não
            </Button>
            <Button variant="danger" loading={remove.isPending} onClick={() => remove.mutate(item.id, { onSuccess: onClose })}>
              Excluir
            </Button>
          </div>
        ) : (
          <div className="flex items-center gap-1 pt-1">
            <Button variant="secondary" loading={toggle.isPending} onClick={() => toggle.mutate(item.id)}>
              {item.done ? "Reabrir" : "Concluir"}
            </Button>
            <Button variant="ghost" onClick={() => onView({ type: "form", id: item.id })}>
              Editar
            </Button>
            <Button variant="ghost" className="ml-auto text-danger" onClick={() => setConfirming(true)}>
              Excluir
            </Button>
          </div>
        )}
      </div>
    </div>
  );
}

function DayView({ date, items, teamNames, onView, onClose }: Props & { date: string }) {
  const list = sortItems(items.filter((i) => i.date === date));
  return (
    <div>
      <Header title={cap(longDate(date))} onClose={onClose} />
      <ul className="pb-1">
        {list.length === 0 && <li className="px-4 pb-3 pt-1 text-[13px] text-muted">Nada agendado neste dia.</li>}
        {list.map((i) => (
          <li key={i.id}>
            <button
              type="button"
              onClick={() => onView({ type: "item", id: i.id })}
              className="flex w-full items-center gap-2.5 px-4 py-2 text-left hover:bg-hover"
            >
              <span className={cn("size-2 shrink-0 rounded-full", kindDot(i.kind))} />
              <span className="w-11 shrink-0 text-[12px] tabular-nums text-muted">{i.time ?? "dia todo"}</span>
              <span className={cn("min-w-0 flex-1 truncate text-[13.5px]", i.done && "text-muted line-through")}>{i.title}</span>
            </button>
          </li>
        ))}
      </ul>
      <div className="border-t border-line px-2 py-1.5">
        <button
          type="button"
          disabled={teamNames.length === 0}
          onClick={() => onView({ type: "form", date })}
          className="flex w-full items-center gap-2 rounded-lg px-2 py-2 text-left text-[13.5px] font-medium text-brand hover:bg-hover"
        >
          <Plus className="size-4" /> Novo neste dia
        </button>
      </div>
    </div>
  );
}

const fieldCls =
  "w-full rounded-lg border border-line bg-elevated px-3 text-[13px] outline-none transition-colors placeholder:text-muted focus:border-accent";

function FormView({
  editing,
  date,
  teamNames,
  contacts,
  onView,
  onClose,
}: Props & { editing?: AgendaItem; date?: string }) {
  const save = useSaveItem();
  const [kind, setKind] = useState<AgendaKind>(editing?.kind ?? "appointment");
  const [title, setTitle] = useState(editing?.title ?? "");
  const [day, setDay] = useState(editing?.date ?? date ?? "");
  const [time, setTime] = useState(editing?.time ?? "");
  const [team, setTeam] = useState(editing?.team ?? teamNames[0] ?? "");
  const [contact, setContact] = useState(editing?.contact?.conversationId ?? "");
  const [note, setNote] = useState(editing?.note ?? "");
  const [errors, setErrors] = useState<Record<string, string>>({});

  function submit(e: FormEvent) {
    e.preventDefault();
    const err: Record<string, string> = {};
    if (!title.trim()) err.title = "Informe um título.";
    else if (title.trim().length > 120) err.title = "Use até 120 caracteres.";
    if (!/^\d{4}-\d{2}-\d{2}$/.test(day)) err.date = "Escolha a data.";
    if (!team) err.team = "Escolha a equipe.";
    setErrors(err);
    if (Object.keys(err).length) return;

    const c = contacts.find((x) => x.conversationId === contact);
    const item: AgendaItem = {
      id: editing?.id ?? crypto.randomUUID(),
      kind,
      title: title.trim(),
      date: day,
      time: kind === "appointment" && time ? time : undefined,
      team,
      contact: kind === "appointment" || contact ? c : undefined,
      note: note.trim(),
      done: editing?.done ?? false,
    };
    save.mutate(item, { onSuccess: () => onView({ type: "item", id: item.id }), onError: (err) => setErrors(fieldErrors(err)) });
  }

  return (
    <form onSubmit={submit}>
      <Header title={editing ? "Editar item" : "Novo item"} onClose={onClose} />
      <div className="flex flex-col gap-3 px-4 pb-4">
        {save.error && <Alert>{errorMessage(save.error)}</Alert>}
        <div role="radiogroup" aria-label="Tipo" className="flex rounded-lg bg-elevated p-0.5 text-[13px]">
          {(["appointment", "deadline"] as const).map((k) => (
            <button
              key={k}
              type="button"
              role="radio"
              aria-checked={kind === k}
              onClick={() => setKind(k)}
              className={cn(
                "flex-1 rounded-md py-1.5 font-medium text-muted transition-colors",
                kind === k && "bg-hover text-fg shadow-sm",
              )}
            >
              {kindLabel[k]}
            </button>
          ))}
        </div>
        <Field label="Título" value={title} error={errors.title} autoFocus onChange={(e) => setTitle(e.target.value)} />
        <div className="flex gap-3">
          <Field className="flex-1" label="Data" type="date" value={day} error={errors.date} onChange={(e) => setDay(e.target.value)} />
          {kind === "appointment" && (
            <Field className="w-28" label="Hora" type="time" value={time} error={errors.time} onChange={(e) => setTime(e.target.value)} />
          )}
        </div>
        <label className="flex flex-col gap-1.5 text-[13px] font-medium">
          Equipe
          <select className={cn(fieldCls, "h-9 font-normal")} value={team} onChange={(e) => setTeam(e.target.value)}>
            {teamNames.map((t) => (
              <option key={t}>{t}</option>
            ))}
          </select>
          {errors.team && <span className="text-xs font-normal text-danger">{errors.team}</span>}
        </label>
        <label className="flex flex-col gap-1.5 text-[13px] font-medium">
          Cliente <span className="-mt-1 text-[11.5px] font-normal text-muted">opcional</span>
          <select className={cn(fieldCls, "h-9 font-normal")} value={contact} onChange={(e) => setContact(e.target.value)}>
            <option value="">Sem cliente</option>
            {contacts.map((c) => (
              <option key={c.conversationId} value={c.conversationId}>
                {c.name}
              </option>
            ))}
          </select>
        </label>
        <label className="flex flex-col gap-1.5 text-[13px] font-medium">
          Observação
          <textarea
            rows={3}
            value={note}
            onChange={(e) => setNote(e.target.value)}
            className={cn(fieldCls, "resize-none py-2 font-normal")}
          />
        </label>
        <div className="flex justify-end gap-2 pt-1">
          <Button type="button" variant="secondary" onClick={() => (editing ? onView({ type: "item", id: editing.id }) : onClose())}>
            Cancelar
          </Button>
          <Button type="submit" loading={save.isPending} disabled={teamNames.length === 0}>
            Salvar
          </Button>
        </div>
      </div>
    </form>
  );
}
