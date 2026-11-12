import { useMemo, useState, type FormEvent } from "react";
import { FlaskConical, Plus, Sparkles } from "lucide-react";
import { ListRow, Section } from "@/components/list";
import { Alert, Button, Field, Modal } from "@/components/ui";
import { errorMessage, fieldErrors } from "@/lib/api";
import { cn } from "@/lib/cn";
import {
  useAddKeywords,
  useClassifyText,
  useDeleteKeyword,
  useSegments,
  useSuggestions,
  useUpdateKeyword,
  type BulkResult,
  type Keyword,
  type Segment,
} from "./api";
import { parsePhrases, validWildcards } from "./keywords";

type Dialog = "add" | "suggest" | "test" | { keyword: Keyword } | null;

const textareaClass =
  "min-h-28 w-full resize-y rounded-lg border border-line bg-elevated p-2.5 text-[14px] outline-none focus:border-accent";

/**
 * Palavras-chave de um segmento: lista, adicionar várias de uma vez, sugestões e teste de mensagem.
 * Usada pelo administrador (com prioridade) e pelas equipes (cada uma nas palavras da própria fila).
 */
export function KeywordsSections({ segment: s, isAdmin }: { segment: Segment; isAdmin: boolean }) {
  const [dialog, setDialog] = useState<Dialog>(null);
  const close = () => setDialog(null);
  const keywords = s.keywords ?? [];
  const suggestions = useSuggestions(s.id);
  const nSuggestions = suggestions.data?.length ?? 0;

  return (
    <>
      <Section title="Melhorar a classificação">
        <ListRow
          icon={<Sparkles />}
          title="Sugestões de palavras"
          subtitle={
            suggestions.isPending
              ? "Procurando…"
              : nSuggestions
                ? `${nSuggestions} ${nSuggestions === 1 ? "sugestão" : "sugestões"} para este assunto`
                : "Nenhuma sugestão nova para esta fila"
          }
          chevron
          onClick={() => setDialog("suggest")}
        />
        <ListRow
          icon={<FlaskConical />}
          title="Testar uma mensagem"
          subtitle="Veja para qual fila ela iria"
          chevron
          onClick={() => setDialog("test")}
        />
      </Section>

      <Section
        title={`Palavras e expressões${keywords.length ? ` · ${keywords.length}` : ""}`}
        note={
          <>
            Acentos, maiúsculas e plural não importam, mas a palavra precisa aparecer inteira. Use <strong className="font-medium">*</strong> no fim
            para pegar variações: <strong className="font-medium">emit*</strong> vale para emitir, emita e emitiu. Se duas filas combinam na mesma
            mensagem, vence a de maior prioridade; se empatam, a conversa vai para Não atribuídos.
          </>
        }
      >
        {keywords.map((k) => (
          <ListRow
            key={k.id}
            title={k.phrase}
            value={isAdmin && k.priority > 0 ? `prioridade ${k.priority}` : undefined}
            onClick={() => setDialog({ keyword: k })}
          />
        ))}
        <ListRow icon={<Plus />} title="Adicionar palavras" tone="brand" onClick={() => setDialog("add")} />
      </Section>

      <AddKeywordsModal key={`add-${dialog === "add"}`} open={dialog === "add"} segmentId={s.id} isAdmin={isAdmin} onClose={close} />
      <SuggestionsModal key={`sug-${dialog === "suggest"}`} open={dialog === "suggest"} segmentId={s.id} onClose={close} />
      <TestModal open={dialog === "test"} onClose={close} />
      {typeof dialog === "object" && dialog && <KeywordModal segmentId={s.id} k={dialog.keyword} isAdmin={isAdmin} onClose={close} />}
    </>
  );
}

function ResultSummary({ result, onClose }: { result: BulkResult; onClose: () => void }) {
  const shared = result.added.filter((k) => k.sharedWith?.length);
  return (
    <div className="flex flex-col gap-3 text-[13.5px]">
      <p>
        {result.added.length === 0
          ? "Nenhuma palavra nova foi adicionada."
          : result.added.length === 1
            ? "1 palavra adicionada."
            : `${result.added.length} palavras adicionadas.`}
      </p>
      {shared.length > 0 && (
        <Alert tone="info">
          {shared.map((k) => `“${k.phrase}” também está em ${k.sharedWith!.join(", ")}`).join("; ")}. Mensagens com ela vão empatar e cair em Não atribuídos.
        </Alert>
      )}
      {result.skipped.length > 0 && (
        <div>
          <p className="text-muted">Não adicionadas:</p>
          <ul className="mt-1 list-disc pl-5 text-[13px]">
            {result.skipped.map((m) => (
              <li key={m}>{m}</li>
            ))}
          </ul>
        </div>
      )}
      <div className="flex justify-end">
        <Button onClick={onClose}>Fechar</Button>
      </div>
    </div>
  );
}

function AddKeywordsModal({ open, segmentId, isAdmin, onClose }: { open: boolean; segmentId: string; isAdmin: boolean; onClose: () => void }) {
  const add = useAddKeywords();
  const [text, setText] = useState("");
  const [priority, setPriority] = useState(50);
  const phrases = useMemo(() => parsePhrases(text), [text]);
  const badWildcard = phrases.find((p) => !validWildcards(p));
  const fields = fieldErrors(add.error);

  function submit(e: FormEvent) {
    e.preventDefault();
    add.mutate({ segmentId, phrases, priority: isAdmin ? priority : undefined });
  }

  return (
    <Modal open={open} onClose={onClose} title="Adicionar palavras">
      {add.data ? (
        <ResultSummary result={add.data} onClose={onClose} />
      ) : (
        <form onSubmit={submit} className="flex flex-col gap-4">
          <label className="flex flex-col gap-1.5 text-[13px]">
            <span className="font-medium">Palavras ou expressões</span>
            <textarea
              value={text}
              autoFocus
              onChange={(e) => setText(e.target.value)}
              placeholder={"nota fiscal\nnfe, danfe\nemit* nota"}
              className={textareaClass}
            />
            <span className="text-muted">
              Uma por linha, ou separadas por vírgula. Use * no fim para variações (emit*).
              {phrases.length > 0 && ` · ${phrases.length} ${phrases.length === 1 ? "palavra" : "palavras"}`}
            </span>
          </label>
          {badWildcard && <Alert>“{badWildcard}”: use * só no fim de uma palavra e com ao menos 3 letras antes (exemplo: emit*).</Alert>}
          {isAdmin && (
            <Field
              label="Prioridade"
              type="number"
              min={0}
              max={100}
              value={priority}
              error={fields.priority}
              hint="De 0 a 100. A maior vence quando duas filas combinam. As sugestões já trazem a sua."
              onChange={(e) => setPriority(Number(e.target.value))}
            />
          )}
          {add.isError && <Alert>{errorMessage(add.error)}</Alert>}
          <div className="flex justify-end gap-2">
            <Button type="button" variant="secondary" onClick={onClose}>
              Cancelar
            </Button>
            <Button type="submit" loading={add.isPending} disabled={phrases.length === 0 || !!badWildcard}>
              Adicionar
            </Button>
          </div>
        </form>
      )}
    </Modal>
  );
}

function SuggestionsModal({ open, segmentId, onClose }: { open: boolean; segmentId: string; onClose: () => void }) {
  const suggestions = useSuggestions(segmentId, open);
  const add = useAddKeywords();
  // Começam marcadas as expressões específicas (prioridade 60); as palavras soltas e genéricas o usuário escolhe.
  const [picked, setPicked] = useState<Record<string, boolean> | null>(null);
  const list = suggestions.data ?? [];
  const selection = picked ?? Object.fromEntries(list.map((s) => [s.phrase, s.priority >= 60]));
  const chosen = list.filter((s) => selection[s.phrase]);
  const packs = [...new Set(list.map((s) => s.pack))];

  return (
    <Modal open={open} onClose={onClose} title="Sugestões de palavras">
      {add.data ? (
        <ResultSummary result={add.data} onClose={onClose} />
      ) : (
        <div className="flex flex-col gap-4">
          <p className="text-[13px] text-muted">
            Palavras comuns do assunto desta fila. Marque as que fazem sentido para o escritório; as genéricas (como “nota” ou “folha”) pegam
            mais mensagens, mas também podem pegar conversas de outro assunto.
          </p>
          {suggestions.isPending && <p className="text-[13px] text-muted">Procurando…</p>}
          {suggestions.isError && <Alert>{errorMessage(suggestions.error)}</Alert>}
          {!suggestions.isPending && list.length === 0 && (
            <p className="text-[13px] text-muted">Não há sugestões novas: esta fila já tem as palavras mais comuns do assunto.</p>
          )}
          {packs.map((pack) => (
            <fieldset key={pack} className="flex flex-col gap-1">
              <legend className="mb-1 text-[12px] font-medium uppercase tracking-wide text-muted">{pack}</legend>
              {list
                .filter((s) => s.pack === pack)
                .map((s) => (
                  <label key={s.phrase} className="flex cursor-pointer items-center gap-2.5 rounded-lg px-1 py-1 text-[14px] hover:bg-hover">
                    <input
                      type="checkbox"
                      checked={!!selection[s.phrase]}
                      onChange={(e) => setPicked({ ...selection, [s.phrase]: e.target.checked })}
                      className="size-4 accent-[var(--c-accent)]"
                    />
                    <span className="flex-1">{s.phrase}</span>
                    {s.priority < 60 && <span className="text-[11.5px] text-muted">genérica</span>}
                  </label>
                ))}
            </fieldset>
          ))}
          {add.isError && <Alert>{errorMessage(add.error)}</Alert>}
          <div className="flex items-center justify-end gap-2">
            <Button type="button" variant="secondary" onClick={onClose}>
              Cancelar
            </Button>
            <Button
              loading={add.isPending}
              disabled={chosen.length === 0}
              onClick={() => add.mutate({ segmentId, phrases: chosen.map((s) => s.phrase) })}
            >
              Adicionar {chosen.length > 0 ? chosen.length : ""}
            </Button>
          </div>
        </div>
      )}
    </Modal>
  );
}

function TestModal({ open, onClose }: { open: boolean; onClose: () => void }) {
  const [text, setText] = useState("");
  const test = useClassifyText();
  const segments = useSegments();
  const nameOf = (key?: string) => segments.data?.find((s) => s.key === key)?.name ?? key ?? "";
  const r = test.data;
  const conflict = r?.reason === "conflict" ? [...new Set(r.matches.map((m) => nameOf(m.segmentKey)))] : [];

  return (
    <Modal open={open} onClose={onClose} title="Testar uma mensagem">
      <form
        onSubmit={(e) => {
          e.preventDefault();
          if (text.trim()) test.mutate(text);
        }}
        className="flex flex-col gap-4"
      >
        <label className="flex flex-col gap-1.5 text-[13px]">
          <span className="font-medium">Mensagem do cliente</span>
          <textarea
            value={text}
            autoFocus
            onChange={(e) => {
              setText(e.target.value);
              test.reset();
            }}
            placeholder="ex.: Bom dia, preciso emitir uma nota fiscal"
            className={textareaClass}
          />
        </label>
        {test.isError && <Alert>{errorMessage(test.error)}</Alert>}
        {r && (
          <div className={cn("rounded-lg border border-line p-3 text-[13.5px]")} role="status">
            {r.reason === "keyword" && (
              <p>
                Iria para a fila <strong className="font-medium">{nameOf(r.segmentKey)}</strong>.
              </p>
            )}
            {r.reason === "conflict" && (
              <p>
                Empate entre <strong className="font-medium">{conflict.join(" e ")}</strong>: iria para Não atribuídos. Para resolver, ajuste as
                prioridades ou as palavras.
              </p>
            )}
            {r.reason === "none" && <p>Nenhuma palavra combinou: iria para Não atribuídos. Adicione uma palavra desta mensagem à fila certa.</p>}
            {r.matches.length > 0 && (
              <ul className="mt-2 flex flex-col gap-0.5 text-[12.5px] text-muted">
                {r.matches.map((m, i) => (
                  <li key={i}>
                    “{m.phrase}” · {nameOf(m.segmentKey)}
                    {m.priority > 0 ? ` · prioridade ${m.priority}` : ""}
                  </li>
                ))}
              </ul>
            )}
          </div>
        )}
        <div className="flex justify-end gap-2">
          <Button type="button" variant="secondary" onClick={onClose}>
            Fechar
          </Button>
          <Button type="submit" loading={test.isPending} disabled={!text.trim()}>
            Testar
          </Button>
        </div>
      </form>
    </Modal>
  );
}

function KeywordModal({ segmentId, k, isAdmin, onClose }: { segmentId: string; k: Keyword; isAdmin: boolean; onClose: () => void }) {
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
        {isAdmin ? (
          <Field
            label="Prioridade"
            type="number"
            min={0}
            max={100}
            value={priority}
            error={fields.priority}
            hint="De 0 a 100. A maior vence quando duas filas combinam."
            onChange={(e) => setPriority(Number(e.target.value))}
          />
        ) : (
          <p className="text-[13px] text-muted">A prioridade é definida pelo administrador do escritório.</p>
        )}
        {remove.error && <Alert>{errorMessage(remove.error)}</Alert>}
        <div className="flex items-center gap-2">
          <button
            type="button"
            disabled={remove.isPending}
            onClick={() => remove.mutate({ segmentId, id: k.id }, { onSuccess: onClose })}
            className="rounded-lg px-2 py-1.5 text-[13.5px] text-danger hover:bg-hover disabled:opacity-60"
          >
            Remover
          </button>
          <span className="ml-auto flex gap-2">
            <Button type="button" variant="secondary" onClick={onClose}>
              {isAdmin ? "Cancelar" : "Fechar"}
            </Button>
            {isAdmin && (
              <Button type="submit" loading={update.isPending}>
                Salvar
              </Button>
            )}
          </span>
        </div>
      </form>
    </Modal>
  );
}
