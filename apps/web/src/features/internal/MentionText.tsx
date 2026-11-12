import { RichText } from "@/components/RichText";
import { cn } from "@/lib/cn";
import { splitMentions } from "@/features/conversations/mentions";
import type { Message } from "@/features/conversations/types";

/** Texto de uma mensagem do chat interno com as @menções em destaque (mais forte quando é de mim que falam). */
export function MentionText({ m, myId }: { m: Extract<Message, { kind: "text" }>; myId: string }) {
  return (
    <>
      {splitMentions(m.text, m.mentions).map((s, i) =>
        s.mention ? (
          <span
            key={i}
            className={cn(
              "rounded px-0.5 font-semibold",
              s.mention.id === myId ? "bg-amber-300/70 text-amber-950" : m.direction === "out" ? "bg-white/20 text-white" : "bg-[#5f6e24]/15 text-[#46521a]",
            )}
          >
            {s.text}
          </span>
        ) : (
          <RichText key={i} text={s.text} />
        ),
      )}
    </>
  );
}
