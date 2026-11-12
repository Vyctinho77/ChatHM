import { usePeople } from "@/features/team/api";
import type { Message } from "@/features/conversations/types";
import { RoleBadge } from "./RoleBadge";
import { authorColor } from "./roles";

/** Quem escreveu, acima da primeira mensagem de uma sequência: nome (com cor própria), perfil e equipe. */
export function AuthorLine({ m }: { m: Message }) {
  const { data: people } = usePeople();
  const teams = people?.find((p) => p.id === m.authorId)?.teams ?? [];
  const inactive = people !== undefined && !people.some((p) => p.id === m.authorId); // a lista traz só pessoas ativas
  return (
    <p className="flex flex-wrap items-center gap-x-1.5 gap-y-0.5 pb-0.5 text-[12.5px] leading-tight">
      <span className="font-semibold" style={{ color: authorColor(m.authorId ?? m.authorName ?? "") }}>
        {m.authorName}
        {inactive && <span className="font-normal opacity-70"> (desativado)</span>}
      </span>
      {m.authorRole && <RoleBadge role={m.authorRole} onBubbleSurface />}
      {teams.length > 0 && <span className="font-normal text-black/50">{teams.join(", ")}</span>}
    </p>
  );
}
