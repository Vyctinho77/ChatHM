import { RichText } from "@/components/RichText";
import { cn } from "@/lib/cn";
import { splitMentions } from "@/features/conversations/mentions";
import type { Message } from "@/features/conversations/types";

/**
 * Texto de uma mensagem do chat interno. Cada @menção é um link azul: clicar leva à última mensagem da pessoa mencionada.
 * Quando é de mim que falam, o link fica em negrito.
 */
export function MentionText({ m, myId, onMention }: { m: Extract<Message, { kind: "text" }>; myId: string; onMention?: (person: { id: string; name: string }) => void }) {
  return (
    <>
      {splitMentions(m.text, m.mentions).map((s, i) => {
        const person = s.mention;
        if (!person) return <RichText key={i} text={s.text} />;
        return (
          <button
            key={i}
            type="button"
            title={`Ir para a última mensagem de ${person.name}`}
            onClick={(e) => {
              e.stopPropagation();
              onMention?.(person);
            }}
            className={cn(
              "cursor-pointer underline-offset-2 hover:underline focus-visible:underline focus-visible:outline-none",
              m.direction === "out" ? "text-sky-300" : "text-[#027eb5]",
              person.id === myId ? "font-semibold" : "font-medium",
            )}
          >
            {s.text}
          </button>
        );
      })}
    </>
  );
}
