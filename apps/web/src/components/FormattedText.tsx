import { Fragment, useMemo, type ReactNode } from "react";
import { formatMessage, type FormattedPart } from "@/lib/messageFormatting";
import { Highlight } from "./Highlight";

/** Formatação sem elementos interativos, também utilizável dentro de botões de prévia. */
export function FormattedText({ text, term, renderText }: { text: string; term?: string; renderText?: (text: string) => ReactNode }) {
  const parts = useMemo(() => formatMessage(text), [text]);
  function render(list: FormattedPart[]): ReactNode {
    return list.map((part, i) => {
      if (part.type === "text") return <Fragment key={i}>{renderText ? renderText(part.text) : <Highlight text={part.text} term={term} />}</Fragment>;
      if (part.type === "bold") return <strong key={i} className="font-bold">{render(part.children)}</strong>;
      return <em key={i} className="italic">{render(part.children)}</em>;
    });
  }
  return <>{render(parts)}</>;
}
