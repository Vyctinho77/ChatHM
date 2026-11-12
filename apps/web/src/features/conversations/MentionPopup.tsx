import { Avatar } from "@/components/Avatar";
import { cn } from "@/lib/cn";
import { mentionListId, mentionOptionId, type MentionPerson } from "./mentions";

/** Lista de pessoas que aparece acima do campo de mensagem enquanto se digita um @. */
export function MentionPopup({ people, index, onPick, onHover }: { people: MentionPerson[]; index: number; onPick: (p: MentionPerson) => void; onHover: (i: number) => void }) {
  return (
    <ul id={mentionListId} role="listbox" aria-label="Pessoas para mencionar" className="absolute inset-x-0 bottom-full z-30 mb-2 max-h-60 overflow-y-auto rounded-xl border border-line bg-panel py-1 shadow-xl sm:right-auto sm:w-80">
      {people.map((p, i) => (
        <li
          key={p.id}
          id={mentionOptionId(p.id)}
          role="option"
          aria-selected={i === index}
          // mousedown (e não click) mantém o foco no campo de mensagem.
          onMouseDown={(e) => {
            e.preventDefault();
            onPick(p);
          }}
          onMouseEnter={() => onHover(i)}
          className={cn("flex cursor-pointer items-center gap-2.5 px-3 py-1.5", i === index && "bg-hover")}
        >
          <Avatar name={p.name} size="sm" />
          <span className="min-w-0 flex-1">
            <span className="block truncate text-[13px] font-medium">{p.name}</span>
            {p.subtitle && <span className="block truncate text-[11px] text-muted">{p.subtitle}</span>}
          </span>
        </li>
      ))}
    </ul>
  );
}
