import { useState, type FormEvent } from "react";
import { useNavigate } from "react-router";
import { Check, Layers, Plus, TriangleAlert } from "lucide-react";
import { ListRow, Page, PageBody, PageHeader, Section } from "@/components/list";
import { Alert, Button, Field, Modal, Spinner } from "@/components/ui";
import { errorMessage, fieldErrors } from "@/lib/api";
import { useTeams } from "@/features/team/api";
import { useClassifyText, useCreateSegment, useImportExamples, useSegments, type ClassifyResult, type Segment } from "./api";
import { TeamPicker } from "./SegmentDetailPage";

/** Configuração das filas: testar uma mensagem e escolher um segmento para ajustar. */
export function QueuesConfigPage() {
  const navigate = useNavigate();
  const segments = useSegments();
  const [creating, setCreating] = useState(false);
  const examples = useImportExamples();

  return (
    <Page>
      <PageHeader title="Configurar filas" onBack={() => navigate("/filas")} />
      <PageBody>
        {segments.isPending && <Spinner className="mx-auto mt-10" />}
        {segments.data?.length === 0 && (
          <div className="mx-4 mt-6 border-b border-line pb-5 text-sm">
            <p className="font-medium">Nenhum segmento ainda</p>
            <p className="mt-1 text-muted">
              Segmentos organizam as conversas por assunto e as encaminham à equipe certa, por palavras-chave. Até você criar o primeiro,
              toda conversa fica na fila Geral, visível a todos.
            </p>
            {examples.isError && <Alert>{errorMessage(examples.error)}</Alert>}
            <div className="mt-4 flex flex-wrap gap-2">
              <Button onClick={() => setCreating(true)}>
                <Plus className="size-4" /> Criar o primeiro segmento
              </Button>
              <Button variant="secondary" loading={examples.isPending} onClick={() => examples.mutate()}>
                Importar exemplos
              </Button>
            </div>
            <p className="mt-2 text-xs text-muted">
              Os exemplos (Clientes novos, Folha de pagamento, Emissão de notas, com palavras-chave e etiquetas comuns) entram sem equipe
              vinculada; você ajusta, vincula equipes ou apaga o que não servir.
            </p>
          </div>
        )}
        {segments.data && (
          <>
            <Section title="Testar uma mensagem" note="Veja para qual fila uma mensagem de cliente iria, com as palavras atuais.">
              <li className="px-4 pt-1">
                <Tester segments={segments.data} />
              </li>
            </Section>

            <Section
              title="Segmentos"
              note="Mensagens sem correspondência clara, ou que combinam com mais de um segmento, vão para a fila Geral."
            >
              {segments.data.map((s) => (
                <ListRow
                  key={s.id}
                  icon={<Layers />}
                  title={s.name}
                  subtitle={`${s.isActive ? "" : "Desativado · "}${s.teamName ? `Equipe ${s.teamName}` : "Sem equipe"} · ${s.keywords?.length ?? 0} palavras`}
                  chevron
                  to={`/filas/configurar/${s.key}`}
                />
              ))}
              <ListRow icon={<Plus />} title="Novo segmento" tone="brand" onClick={() => setCreating(true)} />
            </Section>
          </>
        )}
      </PageBody>
      <NewSegmentModal open={creating} onClose={() => setCreating(false)} onCreated={(s) => navigate(`/filas/configurar/${s.key}`)} />
    </Page>
  );
}

function resultLine(r: ClassifyResult, name: (key?: string) => string) {
  if (r.reason === "keyword") {
    const word = r.matches.find((m) => m.segmentKey === r.segmentKey)?.phrase;
    return (
      <span className="flex items-start gap-1.5 text-fg">
        <Check className="mt-0.5 size-4 shrink-0 text-brand" />
        <span>
          Vai para <strong className="font-medium">{name(r.segmentKey)}</strong>
          {word && <span className="text-muted"> · “{word}”</span>}
        </span>
      </span>
    );
  }
  if (r.reason === "conflict") {
    const names = Array.from(new Set(r.matches.map((m) => name(m.segmentKey)))).join(" e ");
    return (
      <span className="flex items-start gap-1.5">
        <TriangleAlert className="mt-0.5 size-4 shrink-0 text-warning" />
        <span>Fila Geral: empate entre {names}. Dê prioridade maior a uma das palavras.</span>
      </span>
    );
  }
  return <span>Fila Geral: nenhuma palavra encontrada.</span>;
}

function Tester({ segments }: { segments: Segment[] }) {
  const [text, setText] = useState("");
  const classify = useClassifyText();
  const name = (key?: string) => segments.find((s) => s.key === key)?.name ?? key ?? "";

  function submit(e: FormEvent) {
    e.preventDefault();
    if (text.trim()) classify.mutate(text);
  }

  return (
    <div>
      <form onSubmit={submit} className="flex items-center gap-2">
        <input
          aria-label="Mensagem de teste"
          value={text}
          onChange={(e) => setText(e.target.value)}
          placeholder="Ex.: quero emitir uma nota"
          className="h-9 min-w-0 flex-1 rounded-lg bg-elevated px-3 text-[13.5px] outline-none placeholder:text-muted"
        />
        <button
          type="submit"
          disabled={!text.trim() || classify.isPending}
          className="h-9 shrink-0 rounded-lg px-3 text-[13.5px] font-medium text-accent hover:bg-hover disabled:opacity-50"
        >
          Testar
        </button>
      </form>
      {classify.error && <p className="mt-2 text-[12.5px] text-danger">{errorMessage(classify.error)}</p>}
      {classify.data && <p className="mt-2 text-[13px] text-muted">{resultLine(classify.data, name)}</p>}
    </div>
  );
}

function NewSegmentModal({ open, onClose, onCreated }: { open: boolean; onClose: () => void; onCreated: (s: Segment) => void }) {
  const [name, setName] = useState("");
  const [teamId, setTeamId] = useState<string | null>(null);
  const [picking, setPicking] = useState(false);
  const teams = useTeams();
  const create = useCreateSegment();
  const fields = fieldErrors(create.error);
  const teamName = teams.data?.find((t) => t.id === teamId)?.name;

  function close() {
    create.reset();
    setName("");
    setTeamId(null);
    onClose();
  }

  return (
    <>
      <Modal open={open && !picking} onClose={close} title="Novo segmento">
        <form
          onSubmit={(e) => {
            e.preventDefault();
            create.mutateAsync({ name, teamId }).then((s) => {
              close();
              onCreated(s as Segment);
            }, () => undefined);
          }}
          className="flex flex-col gap-4"
        >
          {create.isError && !fields.name && !fields.teamId && <Alert>{errorMessage(create.error)}</Alert>}
          <Field label="Nome" value={name} error={fields.name} autoFocus onChange={(e) => setName(e.target.value)} required />
          <button
            type="button"
            onClick={() => setPicking(true)}
            className="flex items-center justify-between rounded-lg border border-line px-3 py-2.5 text-left text-[13.5px] hover:bg-hover"
          >
            <span>Equipe</span>
            <span className="text-muted">{teamName ?? "Ninguém (visível a todos)"}</span>
          </button>
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
      <TeamPicker
        open={open && picking}
        current={teamId}
        teams={teams.data ?? []}
        onClose={() => setPicking(false)}
        onPick={(id) => {
          setTeamId(id);
          setPicking(false);
        }}
      />
    </>
  );
}
