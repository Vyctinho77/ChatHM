import { useState, type FormEvent } from "react";
import { FlaskConical, Pencil, Plus, Trash2, TriangleAlert, X } from "lucide-react";
import { Alert, Button, Field, IconButton, Modal } from "@/components/ui";
import { ApiError, errorMessage, type FieldErrors } from "@/lib/api";
import { cn } from "@/lib/cn";
import { segmentClass } from "@/features/conversations/types";
import type { Team } from "@/features/team/api";
import {
  useAddKeyword,
  useClassifyText,
  useCreateSegment,
  useDeleteKeyword,
  useDeleteSegment,
  useSetSegmentTeam,
  useUpdateKeyword,
  useUpdateSegment,
  type ClassifyResult,
  type Keyword,
  type Segment,
} from "./api";

const fieldsOf = (e: unknown): FieldErrors => (e instanceof ApiError ? e.fields : {});

const selectCls = "h-8 rounded-lg border border-line bg-elevated px-2 text-[13px] outline-none focus:border-accent";

// ---------------------------------------------------------------------------
// Teste de classificação

export function ClassifyTester({ segments }: { segments: Segment[] }) {
  const [text, setText] = useState("quero emitir uma nota");
  const classify = useClassifyText();
  const name = (key?: string) => segments.find((s) => s.key === key)?.name ?? key ?? "";

  function submit(e: FormEvent) {
    e.preventDefault();
    if (text.trim()) classify.mutate(text);
  }

  return (
    <section className="rounded-xl border border-line bg-panel p-4">
      <h2 className="flex items-center gap-2 text-[14px] font-semibold">
        <FlaskConical className="size-4" /> Testar classificação
      </h2>
      <p className="mt-0.5 text-[12px] text-muted">
        Digite uma mensagem de cliente e veja para qual fila ela iria, com as palavras-chave atuais.
      </p>
      <form onSubmit={submit} className="mt-3 flex flex-col gap-2 sm:flex-row sm:items-end">
        <Field className="flex-1" label="Mensagem do cliente" value={text} onChange={(e) => setText(e.target.value)} />
        <Button type="submit" variant="secondary" loading={classify.isPending}>
          Testar
        </Button>
      </form>
      {classify.error && <div className="mt-3"><Alert>{errorMessage(classify.error)}</Alert></div>}
      {classify.data && <ClassifyOutcome r={classify.data} name={name} />}
    </section>
  );
}

function ClassifyOutcome({ r, name }: { r: ClassifyResult; name: (k?: string) => string }) {
  const winner = r.matches.find((m) => m.segmentKey === r.segmentKey);
  return (
    <div className="mt-3 rounded-lg bg-hover px-3 py-2.5 text-[13px]">
      {r.reason === "keyword" && (
        <p>
          Vai para <strong>{name(r.segmentKey)}</strong>
          {winner && (
            <>
              {" "}
              pela palavra &ldquo;{winner.phrase}&rdquo;
              {winner.priority > 0 && <span className="text-muted"> (prioridade {winner.priority})</span>}
            </>
          )}
          .
        </p>
      )}
      {r.reason === "none" && <p>Fila <strong>Geral</strong>: nenhuma palavra-chave encontrada.</p>}
      {r.reason === "conflict" && (
        <p>
          Fila <strong>Geral</strong>: conflito entre{" "}
          {Array.from(new Set(r.matches.map((m) => name(m.segmentKey)))).join(" e ")}. Dê prioridade maior a uma das
          palavras para decidir.
        </p>
      )}
      {r.matches.length > 0 && (
        <ul className="mt-1.5 flex flex-wrap gap-1.5">
          {r.matches.map((m, i) => (
            <li key={i} className={cn("rounded-full px-2 py-0.5 text-[11px]", segmentClass(m.segmentKey))}>
              {name(m.segmentKey)}: &ldquo;{m.phrase}&rdquo;{m.priority > 0 ? ` · ${m.priority}` : ""}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Novo segmento

export function NewSegmentButton({ teams }: { teams: Team[] }) {
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [teamId, setTeamId] = useState("");
  const create = useCreateSegment();
  const fields = fieldsOf(create.error);

  function close() {
    create.reset();
    setName("");
    setTeamId("");
    setOpen(false);
  }

  return (
    <>
      <Button variant="secondary" onClick={() => setOpen(true)}>
        <Plus className="size-4" /> Novo segmento
      </Button>
      <Modal open={open} onClose={close} title="Novo segmento">
        <form
          onSubmit={(e) => {
            e.preventDefault();
            create.mutateAsync({ name, teamId: teamId || null }).then(close, () => undefined);
          }}
          className="flex flex-col gap-4"
        >
          {create.isError && !fields.name && !fields.teamId && <Alert>{errorMessage(create.error)}</Alert>}
          <Field label="Nome" value={name} error={fields.name} onChange={(e) => setName(e.target.value)} required />
          <label className="flex flex-col gap-1.5 text-[13px] font-medium">
            Equipe responsável
            <select className={cn(selectCls, "h-9 font-normal")} value={teamId} onChange={(e) => setTeamId(e.target.value)}>
              <option value="">Sem equipe (visível a todos)</option>
              {teams.map((t) => (
                <option key={t.id} value={t.id}>
                  {t.name}
                </option>
              ))}
            </select>
          </label>
          <div className="flex justify-end gap-2">
            <Button type="button" variant="secondary" onClick={close}>
              Cancelar
            </Button>
            <Button type="submit" loading={create.isPending}>
              Criar
            </Button>
          </div>
        </form>
      </Modal>
    </>
  );
}

// ---------------------------------------------------------------------------
// Um segmento: equipe, ativo, palavras-chave

export function SegmentConfig({ segment: s, teams }: { segment: Segment; teams: Team[] }) {
  const update = useUpdateSegment();
  const setTeam = useSetSegmentTeam();
  const remove = useDeleteSegment();
  const [renaming, setRenaming] = useState(false);
  const [deleting, setDeleting] = useState(false);

  return (
    <article className={cn("rounded-xl border border-line bg-panel p-4", !s.isActive && "opacity-80")}>
      <header className="flex flex-wrap items-center gap-3">
        <span className={cn("rounded-full px-2.5 py-0.5 text-[12px] font-medium", segmentClass(s.key))}>{s.name}</span>
        <IconButton label={`Renomear ${s.name}`} className="size-7" onClick={() => setRenaming(true)}>
          <Pencil className="size-3.5" />
        </IconButton>
        <label className="ml-auto flex items-center gap-2 text-[12.5px]">
          <input
            type="checkbox"
            className="size-4"
            checked={s.isActive}
            onChange={(e) => update.mutate({ id: s.id, isActive: e.target.checked })}
          />
          Classifica novas mensagens
        </label>
        <IconButton label={`Excluir ${s.name}`} className="size-7" onClick={() => setDeleting(true)}>
          <Trash2 className="size-3.5" />
        </IconButton>
      </header>

      <div className="mt-3 flex flex-wrap items-center gap-2">
        <label htmlFor={`team-${s.id}`} className="text-[12.5px] text-muted">
          Equipe da fila
        </label>
        <select
          id={`team-${s.id}`}
          className={selectCls}
          value={s.teamId ?? ""}
          disabled={setTeam.isPending}
          onChange={(e) => setTeam.mutate({ id: s.id, teamId: e.target.value || null })}
        >
          <option value="">Sem equipe (visível a todos)</option>
          {teams.map((t) => (
            <option key={t.id} value={t.id}>
              {t.name}
            </option>
          ))}
        </select>
        {!s.teamId && (
          <span className="flex items-center gap-1 text-[12px] text-warning">
            <TriangleAlert className="size-3.5" /> Sem equipe, todos os usuários veem esta fila.
          </span>
        )}
      </div>
      {(update.error || setTeam.error) && (
        <div className="mt-2">
          <Alert>{errorMessage(update.error ?? setTeam.error)}</Alert>
        </div>
      )}

      <Keywords segment={s} />

      <RenameModal open={renaming} segment={s} onClose={() => setRenaming(false)} />
      <Modal open={deleting} onClose={() => setDeleting(false)} title="Excluir segmento">
        <p className="text-[13px]">
          Excluir <strong>{s.name}</strong>? As conversas dele voltam para a fila Geral e passam a ser visíveis a todos os
          usuários.
        </p>
        {remove.error && <Alert>{errorMessage(remove.error)}</Alert>}
        <div className="flex justify-end gap-2">
          <Button variant="secondary" onClick={() => setDeleting(false)}>
            Cancelar
          </Button>
          <Button variant="danger" loading={remove.isPending} onClick={() => remove.mutate(s.id, { onSuccess: () => setDeleting(false) })}>
            Excluir
          </Button>
        </div>
      </Modal>
    </article>
  );
}

function RenameModal({ open, segment, onClose }: { open: boolean; segment: Segment; onClose: () => void }) {
  const [name, setName] = useState(segment.name);
  const update = useUpdateSegment();
  const fields = fieldsOf(update.error);
  return (
    <Modal open={open} onClose={onClose} title="Renomear segmento">
      <form
        onSubmit={(e) => {
          e.preventDefault();
          update.mutateAsync({ id: segment.id, name }).then(onClose, () => undefined);
        }}
        className="flex flex-col gap-4"
      >
        <Field label="Nome" value={name} error={fields.name} onChange={(e) => setName(e.target.value)} required />
        <div className="flex justify-end gap-2">
          <Button type="button" variant="secondary" onClick={onClose}>
            Cancelar
          </Button>
          <Button type="submit" loading={update.isPending}>
            Salvar
          </Button>
        </div>
      </form>
    </Modal>
  );
}

function Keywords({ segment: s }: { segment: Segment }) {
  const add = useAddKeyword();
  const [phrase, setPhrase] = useState("");
  const [priority, setPriority] = useState(0);
  const fields = fieldsOf(add.error);
  const keywords = s.keywords ?? [];

  function submit(e: FormEvent) {
    e.preventDefault();
    add.mutateAsync({ segmentId: s.id, phrase, priority }).then(() => setPhrase(""), () => undefined);
  }

  return (
    <div className="mt-4 border-t border-line pt-3">
      <h3 className="text-[12.5px] font-medium">Palavras e expressões</h3>
      <p className="mt-0.5 text-[11.5px] text-muted">
        Acentos, maiúsculas e plural não importam; a palavra precisa aparecer inteira. Clique para mudar a prioridade
        (maior vence quando duas filas combinam).
      </p>
      <ul className="mt-2 flex flex-wrap gap-1.5">
        {keywords.length === 0 && <li className="text-[12.5px] text-muted">Nenhuma palavra: este segmento não classifica nada.</li>}
        {keywords.map((k) => (
          <KeywordChip key={k.id} segmentId={s.id} k={k} />
        ))}
      </ul>
      <form onSubmit={submit} className="mt-3 flex flex-wrap items-end gap-2">
        <Field
          className="min-w-52 flex-1"
          label="Adicionar palavra ou expressão"
          value={phrase}
          error={fields.phrase}
          placeholder="ex.: nota fiscal"
          onChange={(e) => setPhrase(e.target.value)}
        />
        <Field
          className="w-24"
          label="Prioridade"
          type="number"
          min={0}
          max={100}
          value={priority}
          error={fields.priority}
          onChange={(e) => setPriority(Number(e.target.value))}
        />
        <Button type="submit" variant="secondary" loading={add.isPending} disabled={!phrase.trim()}>
          Adicionar
        </Button>
      </form>
      {add.isError && !fields.phrase && !fields.priority && <div className="mt-2"><Alert>{errorMessage(add.error)}</Alert></div>}
    </div>
  );
}

function KeywordChip({ segmentId, k }: { segmentId: string; k: Keyword }) {
  const update = useUpdateKeyword();
  const remove = useDeleteKeyword();
  const [editing, setEditing] = useState(false);
  const [value, setValue] = useState(k.priority);

  function save() {
    setEditing(false);
    if (value !== k.priority) update.mutate({ segmentId, id: k.id, priority: Math.max(0, Math.min(100, value)) });
  }

  return (
    <li className="inline-flex items-center gap-1 rounded-full border border-line py-0.5 pl-2.5 pr-1 text-[12px]">
      <button type="button" className="hover:underline" onClick={() => (setValue(k.priority), setEditing((e) => !e))} title="Editar prioridade">
        {k.phrase}
      </button>
      {editing ? (
        <input
          autoFocus
          type="number"
          min={0}
          max={100}
          aria-label={`Prioridade de ${k.phrase}`}
          value={value}
          onChange={(e) => setValue(Number(e.target.value))}
          onBlur={save}
          onKeyDown={(e) => {
            if (e.key === "Enter") save();
            if (e.key === "Escape") setEditing(false);
          }}
          className="h-5 w-12 rounded border border-line bg-elevated px-1 text-center text-[11px] outline-none"
        />
      ) : (
        k.priority > 0 && <span className="rounded-full bg-accent/15 px-1.5 text-[10.5px] font-semibold text-accent">{k.priority}</span>
      )}
      <button
        type="button"
        aria-label={`Remover ${k.phrase}`}
        className="rounded-full p-0.5 text-muted hover:bg-hover hover:text-fg"
        disabled={remove.isPending}
        onClick={() => remove.mutate({ segmentId, id: k.id })}
      >
        <X className="size-3" />
      </button>
    </li>
  );
}
