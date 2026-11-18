import { useRef, useState } from "react";
import { Building2, Phone, Plus, UserRound, X } from "lucide-react";
import { ViewableAvatar } from "@/components/AvatarViewer";
import { Menu, MenuItem } from "@/components/Menu";
import { EditableName } from "@/components/EditableName";
import { Field, IconButton } from "@/components/ui";
import { fieldErrors } from "@/lib/api";
import { formatTaxId } from "@/lib/taxId";
import { cn } from "@/lib/cn";
import { capabilities, useConversationActions, useTagCatalog } from "./store";
import { AutomationOptOut } from "./AutomationOptOut";
import { finishWithPrefs } from "./accentController";
import { useAccentTextarea } from "./useAccentTextarea";
import { PersonName } from "@/features/team/PersonName";
import { segmentClass, segmentLabel, type Conversation } from "./types";

const statusLabel = { new: "Aguardando", in_progress: "Em atendimento", closed: "Encerrado" } as const;

/** Use `key={conversation.id}` para reiniciar o estado local ao trocar de conversa. */
export function ContactPanel({ conversation: c, onClose }: { conversation: Conversation; onClose: () => void }) {
  const actions = useConversationActions();
  const [note, setNote] = useState(c.contact.note);
  const noteRef = useRef<HTMLTextAreaElement>(null);
  const noteAccent = useAccentTextarea({ ref: noteRef, onValue: (v) => setNote(v) });

  const { data: catalog = [] } = useTagCatalog();
  const available = catalog.filter((t) => !c.tags.includes(t.name));

  return (
    <aside aria-label="Dados do contato" className="flex h-full min-w-0 flex-col bg-panel">
      <header className="flex h-14 shrink-0 items-center gap-1.5 border-b border-line px-2.5">
        <IconButton label="Fechar dados do contato" onClick={onClose}>
          <X className="size-5" />
        </IconButton>
        <h2 className="text-[13.5px] font-semibold">Dados do contato</h2>
      </header>

      <div className="min-h-0 flex-1 overflow-y-auto">
        <div className="flex flex-col items-center gap-2 border-b border-line px-6 py-6 text-center">
          <ViewableAvatar name={c.contact.name} src={c.contact.avatarUrl} group={c.contact.kind === "group"} size="lg" />
          {!c.contact.saved && c.contact.kind !== "group" && <p className="mt-1 text-xs text-muted">Nome no WhatsApp</p>}
          <p className="mt-1.5 flex max-w-full justify-center text-[15px] font-semibold">
            <EditableName value={c.contact.name} onSave={(name) => actions.setName(c.id, name)} />
          </p>
          <p className="text-[12.5px] text-muted">{c.contact.phone}</p>
        </div>

        <dl className="flex flex-col gap-3.5 border-b border-line px-5 py-4 text-[13px]">
          <Row icon={<Phone />} label="Telefone" value={c.contact.phone} />
          {c.contact.company && <Row icon={<Building2 />} label="Empresa" value={c.contact.company} />}
          <div>
            <dt className="text-xs text-muted">Segmento atual</dt>
            <dd className="mt-1">
              <span className={cn("rounded-full px-2 py-0.5 text-[11px] font-medium", segmentClass(c.segment))}>
                {segmentLabel(c)}
              </span>
            </dd>
          </div>
          <Row
            icon={<UserRound />}
            label="Atendimento"
            value={<>{c.snoozedUntil ? "Adiado" : statusLabel[c.status]}{c.assignee && <> · <PersonName id={c.assignee.id}>{c.assignee.name}</PersonName></>}</>}
          />
        </dl>

        {c.contact.kind !== "group" && <FinancialRefs conversation={c} />}

        <AutomationOptOut contactId={c.contact.id} />

        <section className="border-b border-line px-5 py-5">
          <div className="flex items-center justify-between">
            <h3 className="text-xs text-muted">Etiquetas</h3>
            {capabilities.tags && (
            <Menu
              trigger={({ toggle }) => (
                <IconButton label="Adicionar etiqueta" onClick={toggle} disabled={available.length === 0} className="size-7">
                  <Plus className="size-4" />
                </IconButton>
              )}
            >
              {(close) =>
                available.map((t) => (
                  <MenuItem key={t.id} onClick={() => (actions.setTags(c.id, [...c.tags, t.name]), close())}>
                    {t.name}
                  </MenuItem>
                ))
              }
            </Menu>
            )}
          </div>
          <div className="mt-2 flex flex-wrap gap-1.5">
            {c.tags.length === 0 && (
              <p className="text-[12.5px] text-muted">
                {capabilities.tags ? "Nenhuma etiqueta." : "Etiquetas chegam com as filas automáticas."}
              </p>
            )}
            {c.tags.map((t) => (
              <span
                key={t}
                className="inline-flex items-center gap-1 rounded-full border border-line py-0.5 pl-2.5 pr-1 text-[11px] font-semibold uppercase"
              >
                {t}
                {capabilities.tags && <button
                  type="button"
                  aria-label={`Remover etiqueta ${t}`}
                  className="rounded-full p-0.5 text-muted hover:bg-hover hover:text-fg"
                  onClick={() => actions.setTags(c.id, c.tags.filter((x) => x !== t))}
                >
                  <X className="size-3" />
                </button>}
              </span>
            ))}
          </div>
        </section>

        <section className="px-5 py-5">
          <label htmlFor="contact-note" className="text-xs text-muted">
            Observação interna
          </label>
          <textarea
            id="contact-note"
            ref={noteRef}
            value={note}
            onChange={noteAccent.onChange}
            onKeyDown={(e) => void noteAccent.onKeyDown(e)}
            {...noteAccent.attrs}
            onBlur={() => {
              const final = finishWithPrefs(note);
              if (final !== note) setNote(final);
              if (final !== c.contact.note) actions.setNote(c.id, final);
            }}
            rows={4}
            placeholder="Visível apenas para a equipe"
            className="mt-2 w-full resize-none rounded-lg border border-line bg-elevated p-2.5 text-[13px] outline-none placeholder:text-muted focus:border-accent"
          />
        </section>
      </div>
    </aside>
  );
}

/** Cadastro financeiro do cliente: CNPJ/CPF e id no Asaas. Os agentes de cobrança acham o contato por eles, sem casar por telefone. */
function FinancialRefs({ conversation: c }: { conversation: Conversation }) {
  const actions = useConversationActions();
  const [taxId, setTaxId] = useState(formatTaxId(c.contact.taxId));
  const [asaasId, setAsaasId] = useState(c.contact.asaasId ?? "");
  const [errors, setErrors] = useState<{ taxId?: string; asaasId?: string }>({});
  const [saved, setSaved] = useState(false);

  async function save(refs: { taxId?: string; asaasId?: string }) {
    setSaved(false);
    try {
      await actions.setReferences(c.id, refs);
      setErrors((e) => ({ ...e, ...Object.fromEntries(Object.keys(refs).map((k) => [k, undefined])) }));
      setSaved(true);
    } catch (err) {
      const f = fieldErrors(err);
      setErrors((e) => ({ ...e, taxId: refs.taxId !== undefined ? (f.taxId ?? "Não foi possível salvar.") : e.taxId, asaasId: refs.asaasId !== undefined ? (f.asaasId ?? "Não foi possível salvar.") : e.asaasId }));
    }
  }

  return (
    <section aria-label="Cadastro financeiro" className="flex flex-col gap-3 border-b border-line px-5 py-4">
      <h3 className="text-xs text-muted">Cadastro financeiro</h3>
      <Field
        label="CNPJ ou CPF"
        inputMode="numeric"
        autoComplete="off"
        placeholder="00.000.000/0000-00"
        value={taxId}
        error={errors.taxId}
        onChange={(e) => (setTaxId(e.target.value), setSaved(false))}
        onBlur={() => taxId.replace(/\D/g, "") !== (c.contact.taxId ?? "") && void save({ taxId })}
      />
      <Field
        label="ID do cliente no Asaas"
        autoComplete="off"
        placeholder="cus_000005401844"
        value={asaasId}
        error={errors.asaasId}
        hint={saved ? "Salvo." : "Liga as cobranças do Asaas a este contato."}
        onChange={(e) => (setAsaasId(e.target.value), setSaved(false))}
        onBlur={() => asaasId.trim() !== (c.contact.asaasId ?? "") && void save({ asaasId })}
      />
    </section>
  );
}

function Row({ icon, label, value }: { icon: React.ReactNode; label: string; value: React.ReactNode }) {
  return (
    <div className="flex gap-3">
      <span className="mt-0.5 text-muted [&>svg]:size-4">{icon}</span>
      <div>
        <dt className="text-xs text-muted">{label}</dt>
        <dd>{value}</dd>
      </div>
    </div>
  );
}
