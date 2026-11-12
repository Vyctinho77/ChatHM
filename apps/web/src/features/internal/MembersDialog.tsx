import { Avatar } from "@/components/Avatar";
import { Modal, Spinner } from "@/components/ui";
import { usePeople } from "@/features/team/api";
import { RoleBadge } from "./RoleBadge";
import { roleOrder } from "./roles";

/** Quem participa do chat interno: todo o escritório, com perfil e equipes. */
export function MembersDialog({ myId, onClose }: { myId: string; onClose: () => void }) {
  const { data: people, isPending } = usePeople();
  const sorted = [...(people ?? [])].sort((a, b) => roleOrder[a.role] - roleOrder[b.role] || a.name.localeCompare(b.name, "pt-BR"));
  return (
    <Modal open onClose={onClose} title="Pessoas do escritório">
      <p className="text-sm text-muted">Todo mundo que usa o ChatHM participa do chat interno.</p>
      {isPending && <Spinner className="mx-auto my-3" />}
      <ul className="-mx-1 flex max-h-[55vh] flex-col overflow-y-auto">
        {sorted.map((p) => (
          <li key={p.id} className="flex items-center gap-3 rounded-lg px-2 py-2">
            <Avatar name={p.name} size="sm" />
            <span className="min-w-0 flex-1">
              <span className="block truncate text-sm font-medium">
                {p.name}
                {p.id === myId && <span className="font-normal text-muted"> (você)</span>}
              </span>
              {p.teams.length > 0 && <span className="block truncate text-xs text-muted">{p.teams.join(", ")}</span>}
            </span>
            <RoleBadge role={p.role} />
          </li>
        ))}
      </ul>
    </Modal>
  );
}
