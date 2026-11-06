import { useState, type FormEvent } from "react";
import { Pencil, Plus, Trash2 } from "lucide-react";
import { Alert, Button, Field, IconButton, Modal } from "@/components/ui";
import { ListSkeleton } from "@/components/Skeleton";
import { ApiError, errorMessage, type FieldErrors } from "@/lib/api";
import { useMe } from "@/features/auth/api";
import type { TagInfo } from "@/features/conversations/types";
import { useCreateTag, useDeleteTag, useRenameTag, useTagCatalog } from "./api";

const fieldsOf = (e: unknown): FieldErrors => (e instanceof ApiError ? e.fields : {});

export function TagsPage() {
  const { data: me } = useMe();
  const isAdmin = me?.user.role === "admin";
  const tags = useTagCatalog();
  const create = useCreateTag();
  const [name, setName] = useState("");
  const [editing, setEditing] = useState<TagInfo | null>(null);
  const [deleting, setDeleting] = useState<TagInfo | null>(null);
  const fields = fieldsOf(create.error);

  function submit(e: FormEvent) {
    e.preventDefault();
    create.mutateAsync(name).then(() => setName(""), () => undefined);
  }

  return (
    <div className="h-full overflow-y-auto bg-app">
      <div className="mx-auto max-w-2xl px-4 py-6 sm:px-8">
        <h1 className="text-xl font-semibold">Etiquetas</h1>
        <p className="mt-1 text-[13px] text-muted">
          Marcadores simples para organizar conversas (como PENDENTE ou URGENTE). Aplique pelo painel do contato, dentro
          da conversa. Etiquetas não mudam a fila da conversa.
        </p>

        {isAdmin && (
          <form onSubmit={submit} className="mt-5 flex items-end gap-2">
            <Field
              className="flex-1"
              label="Nova etiqueta"
              value={name}
              error={fields.name}
              placeholder="ex.: RETORNO"
              onChange={(e) => setName(e.target.value)}
            />
            <Button type="submit" loading={create.isPending} disabled={!name.trim()}>
              <Plus className="size-4" /> Criar
            </Button>
          </form>
        )}
        {create.isError && !fields.name && <div className="mt-2"><Alert>{errorMessage(create.error)}</Alert></div>}

        <div className="mt-5">
          {tags.isPending && <ListSkeleton avatar={false} rows={4} label="Carregando etiquetas" />}
          {tags.isError && <Alert>{errorMessage(tags.error)}</Alert>}
          <ul className="divide-y divide-line border-y border-line">
            {tags.data?.length === 0 && <li className="px-4 py-6 text-center text-[13px] text-muted">Nenhuma etiqueta cadastrada.</li>}
            {tags.data?.map((t) => (
              <li key={t.id} className="flex items-center gap-3 px-4 py-2.5">
                <span className="rounded-full border border-line px-2.5 py-0.5 text-[11px] font-semibold uppercase tracking-wide">
                  {t.name}
                </span>
                {typeof t.inUse === "number" && (
                  <span className="text-[12px] text-muted">
                    {t.inUse === 0 ? "sem uso" : `${t.inUse} ${t.inUse === 1 ? "conversa" : "conversas"}`}
                  </span>
                )}
                {isAdmin && (
                  <span className="ml-auto flex">
                    <IconButton label={`Renomear ${t.name}`} onClick={() => setEditing(t)}>
                      <Pencil className="size-4" />
                    </IconButton>
                    <IconButton label={`Excluir ${t.name}`} onClick={() => setDeleting(t)}>
                      <Trash2 className="size-4" />
                    </IconButton>
                  </span>
                )}
              </li>
            ))}
          </ul>
          {!isAdmin && (
            <p className="mt-3 text-[12px] text-muted">Somente administradores criam, renomeiam ou excluem etiquetas.</p>
          )}
        </div>
      </div>

      {editing && <RenameModal tag={editing} onClose={() => setEditing(null)} />}
      {deleting && <DeleteModal tag={deleting} onClose={() => setDeleting(null)} />}
    </div>
  );
}

function RenameModal({ tag, onClose }: { tag: TagInfo; onClose: () => void }) {
  const rename = useRenameTag();
  const [name, setName] = useState(tag.name);
  const fields = fieldsOf(rename.error);
  return (
    <Modal open onClose={onClose} title="Renomear etiqueta">
      <form
        onSubmit={(e) => {
          e.preventDefault();
          rename.mutateAsync({ id: tag.id, name }).then(onClose, () => undefined);
        }}
        className="flex flex-col gap-4"
      >
        <Field label="Nome" value={name} error={fields.name} onChange={(e) => setName(e.target.value)} required />
        <div className="flex justify-end gap-2">
          <Button type="button" variant="secondary" onClick={onClose}>
            Cancelar
          </Button>
          <Button type="submit" loading={rename.isPending}>
            Salvar
          </Button>
        </div>
      </form>
    </Modal>
  );
}

function DeleteModal({ tag, onClose }: { tag: TagInfo; onClose: () => void }) {
  const remove = useDeleteTag();
  return (
    <Modal open onClose={onClose} title="Excluir etiqueta">
      <p className="text-[13px]">
        Excluir <strong>{tag.name}</strong>? Ela será removida de todas as conversas
        {tag.inUse ? ` (${tag.inUse} no momento)` : ""}.
      </p>
      {remove.error && <Alert>{errorMessage(remove.error)}</Alert>}
      <div className="flex justify-end gap-2">
        <Button variant="secondary" onClick={onClose}>
          Cancelar
        </Button>
        <Button variant="danger" loading={remove.isPending} onClick={() => remove.mutate(tag.id, { onSuccess: onClose })}>
          Excluir
        </Button>
      </div>
    </Modal>
  );
}
