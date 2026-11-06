import { useState, type FormEvent } from "react";
import { useNavigate, useParams } from "react-router";
import { Check, Plus } from "lucide-react";
import { ListRow, Page, PageBody, PageHeader, Section } from "@/components/list";
import { Alert, Button, Field, Modal } from "@/components/ui";
import { DetailSkeleton } from "@/components/Skeleton";
import { cn } from "@/lib/cn";
import { errorMessage, fieldErrors } from "@/lib/api";
import { useTeams, type Team } from "@/features/team/api";
import {
  useAddKeyword,
  useDeleteKeyword,
  useDeleteSegment,
  useSegments,
  useSetSegmentTeam,
  useUpdateKeyword,
  useUpdateSegment,
  type Keyword,
  type Segment,
} from "./api";

/** Escolha de equipe em uma lista simples (uma seleção por vez). */
export function TeamPicker({
  open,
  current,
  teams,
  onPick,
  onClose,
}: {
  open: boolean;
  current: string | null;
  teams: Team[];
  onPick: (teamId: string | null) => void;
  onClose: () => void;
}) {
  const options: { id: string | null; label: string; hint?: string }[] = [
    { id: null, label: "Ninguém", hint: "A fila fica visível a todos os usuários" },
    ...teams.map((t) => ({ id: t.id, label: t.name })),
  ];
  return (
    <Modal open={open} onClose={onClose} title="Equipe da fila">
      <ul className="-mx-2 flex flex-col">
        {options.map((o) => (
          <li key={o.id ?? "none"}>
            <button
              type="button"
              onClick={() => onPick(o.id)}
              className="flex w-full items-center gap-3 rounded-lg px-2 py-2.5 text-left hover:bg-hover"
            >
              <span className="min-w-0 flex-1">
                <span className="block text-[14px]">{o.label}</span>
                {o.hint && <span className="block text-[12px] text-muted">{o.hint}</span>}
              </span>
              {current === o.id && <Check className="size-4 text-brand-ink" />}
            </button>
          </li>
        ))}
      </ul>
    </Modal>
  );
}

export function SegmentDetailPage() {
  const { key } = useParams();
  const navigate = useNavigate();
  const segments = useSegments();
  const teams = useTeams();
  const segment = segments.data?.find((s) => s.key === key);
  const back = () => navigate("/filas/configurar");

  if (segments.isPending) {
    return (
      <Page>
        <PageHeader title="Segmento" onBack={back} />
        <DetailSkeleton />
      </Page>
    );
  }
  if (!segment) {
    return (
      <Page>
        <PageHeader title="Segmento" onBack={back} />
        <p className="p-8 text-center text-[13px] text-muted">Segmento não encontrado.</p>
      </Page>
    );
  }
  return <Detail segment={segment} teams={teams.data ?? []} onBack={back} />;
}

type Dialog = "rename" | "team" | "delete" | "add" | { keyword: Keyword } | null;

function Detail({ segment: s, teams, onBack }: { segment: Segment; teams: Team[]; onBack: () => void }) {
  const update = useUpdateSegment();
  const setTeam = useSetSegmentTeam();
  const [dialog, setDialog] = useState<Dialog>(null);
  const close = () => setDialog(null);
  const keywords = s.keywords ?? [];

  return (
    <Page>
      <PageHeader title={s.name} onBack={onBack} />
      <PageBody>
        {(update.error || setTeam.error) && (
          <div className="px-4 pt-4">
            <Alert>{errorMessage(update.error ?? setTeam.error)}</Alert>
          </div>
        )}

        <Section title="Fila">
          <ListRow title="Nome" value={s.name} chevron onClick={() => setDialog("rename")} />
          <ListRow
            title="Equipe"
            value={s.teamName || "Ninguém · visível a todos"}
            chevron
            onClick={() => setDialog("team")}
          />
          <ListRow
            title="Classificar mensagens"
            subtitle="Mensagens novas com as palavras abaixo vão para esta fila"
            switchValue={s.isActive}
            onClick={() => update.mutate({ id: s.id, isActive: !s.isActive })}
          />
        </Section>

        <Section
          title={`Palavras e expressões${keywords.length ? ` · ${keywords.length}` : ""}`}
          note="Acentos, maiúsculas e plural não importam, mas a palavra precisa aparecer inteira. Se duas filas combinam, vence a de maior prioridade."
        >
          {keywords.map((k) => (
            <ListRow
              key={k.id}
              title={k.phrase}
              value={k.priority > 0 ? `prioridade ${k.priority}` : undefined}
              onClick={() => setDialog({ keyword: k })}
            />
          ))}
          <ListRow icon={<Plus />} title="Adicionar palavra" tone="brand" onClick={() => setDialog("add")} />
        </Section>

        <Section>
          <ListRow title="Excluir segmento" tone="danger" onClick={() => setDialog("delete")} />
        </Section>
      </PageBody>

      <RenameModal key={String(dialog === "rename")} open={dialog === "rename"} segment={s} onClose={close} />
      <TeamPicker
        open={dialog === "team"}
        current={s.teamId}
        teams={teams}
        onClose={close}
        onPick={(teamId) => {
          setTeam.mutate({ id: s.id, teamId });
          close();
        }}
      />
      <AddKeywordModal open={dialog === "add"} segmentId={s.id} onClose={close} />
      {typeof dialog === "object" && dialog && <KeywordModal segmentId={s.id} k={dialog.keyword} onClose={close} />}
      <DeleteModal open={dialog === "delete"} segment={s} onClose={close} onDeleted={onBack} />
    </Page>
  );
}

function RenameModal({ open, segment, onClose }: { open: boolean; segment: Segment; onClose: () => void }) {
  const [name, setName] = useState(segment.name);
  const update = useUpdateSegment();
  const fields = fieldErrors(update.error);
  return (
    <Modal open={open} onClose={onClose} title="Nome do segmento">
      <form
        onSubmit={(e) => {
          e.preventDefault();
          update.mutateAsync({ id: segment.id, name }).then(onClose, () => undefined);
        }}
        className="flex flex-col gap-4"
      >
        <Field label="Nome" value={name} error={fields.name} autoFocus onChange={(e) => setName(e.target.value)} required />
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

function PriorityField({ value, onChange, error }: { value: number; onChange: (n: number) => void; error?: string }) {
  return (
    <Field
      label="Prioridade"
      type="number"
      min={0}
      max={100}
      value={value}
      error={error}
      hint="De 0 a 100. A maior vence quando duas filas combinam."
      onChange={(e) => onChange(Number(e.target.value))}
    />
  );
}

function AddKeywordModal({ open, segmentId, onClose }: { open: boolean; segmentId: string; onClose: () => void }) {
  const add = useAddKeyword();
  const [phrase, setPhrase] = useState("");
  const [priority, setPriority] = useState(0);
  const fields = fieldErrors(add.error);

  function close() {
    add.reset();
    setPhrase("");
    setPriority(0);
    onClose();
  }
  function submit(e: FormEvent) {
    e.preventDefault();
    add.mutateAsync({ segmentId, phrase, priority }).then(close, () => undefined);
  }

  return (
    <Modal open={open} onClose={close} title="Adicionar palavra">
      <form onSubmit={submit} className="flex flex-col gap-4">
        <Field
          label="Palavra ou expressão"
          placeholder="ex.: nota fiscal"
          value={phrase}
          error={fields.phrase}
          autoFocus
          onChange={(e) => setPhrase(e.target.value)}
        />
        <PriorityField value={priority} onChange={setPriority} error={fields.priority} />
        {add.isError && !fields.phrase && !fields.priority && <Alert>{errorMessage(add.error)}</Alert>}
        <div className="flex justify-end gap-2">
          <Button type="button" variant="secondary" onClick={close}>
            Cancelar
          </Button>
          <Button type="submit" loading={add.isPending} disabled={!phrase.trim()}>
            Adicionar
          </Button>
        </div>
      </form>
    </Modal>
  );
}

function KeywordModal({ segmentId, k, onClose }: { segmentId: string; k: Keyword; onClose: () => void }) {
  const update = useUpdateKeyword();
  const remove = useDeleteKeyword();
  const [priority, setPriority] = useState(k.priority);
  const fields = fieldErrors(update.error);
  return (
    <Modal open onClose={onClose} title={k.phrase}>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          update.mutateAsync({ segmentId, id: k.id, priority }).then(onClose, () => undefined);
        }}
        className="flex flex-col gap-4"
      >
        <PriorityField value={priority} onChange={setPriority} error={fields.priority} />
        {remove.error && <Alert>{errorMessage(remove.error)}</Alert>}
        <div className="flex items-center gap-2">
          <button
            type="button"
            disabled={remove.isPending}
            onClick={() => remove.mutate({ segmentId, id: k.id }, { onSuccess: onClose })}
            className={cn("rounded-lg px-2 py-1.5 text-[13.5px] text-danger hover:bg-hover disabled:opacity-60")}
          >
            Remover
          </button>
          <span className="ml-auto flex gap-2">
            <Button type="button" variant="secondary" onClick={onClose}>
              Cancelar
            </Button>
            <Button type="submit" loading={update.isPending}>
              Salvar
            </Button>
          </span>
        </div>
      </form>
    </Modal>
  );
}

function DeleteModal({
  open,
  segment,
  onClose,
  onDeleted,
}: {
  open: boolean;
  segment: Segment;
  onClose: () => void;
  onDeleted: () => void;
}) {
  const remove = useDeleteSegment();
  return (
    <Modal open={open} onClose={onClose} title="Excluir segmento">
      <p className="text-[13.5px]">
        Excluir <strong className="font-medium">{segment.name}</strong>? As conversas dele voltam para a fila Não atribuídos e
        passam a ser visíveis a todos os usuários.
      </p>
      {remove.error && <Alert>{errorMessage(remove.error)}</Alert>}
      <div className="flex justify-end gap-2">
        <Button variant="secondary" onClick={onClose}>
          Cancelar
        </Button>
        <Button variant="danger" loading={remove.isPending} onClick={() => remove.mutate(segment.id, { onSuccess: onDeleted })}>
          Excluir
        </Button>
      </div>
    </Modal>
  );
}
