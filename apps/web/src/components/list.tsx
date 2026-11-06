// Padrão de lista no estilo das configurações do WhatsApp: a linha é a unidade da
// interface (ícone, título, uma linha de apoio, detalhe à direita), sem caixas em volta
// de cada item, separadores finos que começam depois do ícone e telas que se aprofundam
// com uma seta (›). O único destaque de cor é o verde, nos títulos de seção e nos avisos.
import { Link } from "react-router";
import type { ReactNode } from "react";
import { ArrowLeft, ChevronRight } from "lucide-react";
import { cn } from "@/lib/cn";
import { IconButton } from "./ui";

/** Tela inteira com rolagem; o cabeçalho (PageHeader) fica fixo no topo. */
export function Page({ children }: { children: ReactNode }) {
  return <div className="h-full overflow-y-auto bg-panel">{children}</div>;
}

export function PageHeader({ title, onBack, actions }: { title: string; onBack?: () => void; actions?: ReactNode }) {
  return (
    <header className="sticky top-0 z-10 border-b border-line bg-panel/95 backdrop-blur">
      <div className="flex h-14 items-center gap-1 px-2 sm:px-3">
        {onBack && (
          <IconButton label="Voltar" onClick={onBack}>
            <ArrowLeft className="size-5" />
          </IconButton>
        )}
        <h1 className={cn("truncate text-[17px] font-semibold", onBack ? "pl-1" : "pl-2")}>{title}</h1>
        {actions && <div className="ml-auto flex items-center">{actions}</div>}
      </div>
    </header>
  );
}

export function PageBody({ children }: { children: ReactNode }) {
  return <div className="mx-auto max-w-2xl pb-12">{children}</div>;
}

/** Grupo de linhas com título curto em verde (opcional) e uma nota discreta no fim. */
export function Section({ title, note, children }: { title?: string; note?: ReactNode; children: ReactNode }) {
  return (
    <section>
      {title && <h2 className="px-4 pb-1 pt-6 text-[12.5px] font-medium text-brand-ink">{title}</h2>}
      <ul>{children}</ul>
      {note && <p className="px-4 pt-2 text-[12px] leading-snug text-muted">{note}</p>}
    </section>
  );
}

type RowProps = {
  /** Ícone dentro de um círculo neutro (como a foto numa lista de conversas). */
  icon?: ReactNode;
  title: ReactNode;
  /** Uma linha de apoio sob o título. */
  subtitle?: ReactNode;
  /** Detalhe à direita, em texto discreto (ex.: o valor atual de um ajuste). */
  value?: ReactNode;
  /** Elemento à direita (ex.: contador verde). */
  trailing?: ReactNode;
  chevron?: boolean;
  to?: string;
  onClick?: () => void;
  /** Linha de liga/desliga: o estado aparece como interruptor e a linha toda é clicável. */
  switchValue?: boolean;
  tone?: "default" | "danger" | "brand";
  disabled?: boolean;
};

export function ListRow({
  icon,
  title,
  subtitle,
  value,
  trailing,
  chevron,
  to,
  onClick,
  switchValue,
  tone = "default",
  disabled,
}: RowProps) {
  const body = (
    <>
      {icon !== undefined && (
        <span className="flex size-10 shrink-0 items-center justify-center rounded-full bg-hover text-muted [&>svg]:size-5">
          {icon}
        </span>
      )}
      <span className="flex min-w-0 flex-1 items-center gap-3 border-b border-line py-3 [li:last-child_&]:border-transparent">
        <span className="min-w-0 flex-1">
          <span
            className={cn(
              "block truncate text-[14px] leading-snug",
              tone === "danger" && "text-danger",
              tone === "brand" && "font-medium text-brand-ink",
            )}
          >
            {title}
          </span>
          {subtitle && <span className="mt-0.5 block truncate text-[12.5px] text-muted">{subtitle}</span>}
        </span>
        {value && <span className="max-w-[45%] truncate text-[13px] text-muted">{value}</span>}
        {trailing}
        {switchValue !== undefined && <SwitchGraphic checked={switchValue} />}
        {chevron && <ChevronRight className="size-4 shrink-0 text-muted/70" />}
      </span>
    </>
  );
  const cls =
    "flex w-full items-center gap-3 px-4 text-left transition-colors hover:bg-hover focus-visible:bg-hover disabled:cursor-not-allowed disabled:opacity-60";

  if (to) {
    return (
      <li>
        <Link to={to} className={cls}>
          {body}
        </Link>
      </li>
    );
  }
  if (onClick) {
    return (
      <li>
        <button
          type="button"
          onClick={onClick}
          disabled={disabled}
          className={cls}
          role={switchValue !== undefined ? "switch" : undefined}
          aria-checked={switchValue}
        >
          {body}
        </button>
      </li>
    );
  }
  return <li className={cls}>{body}</li>;
}

function SwitchGraphic({ checked }: { checked: boolean }) {
  return (
    <span
      aria-hidden
      className={cn(
        "relative h-5 w-9 shrink-0 rounded-full transition-colors",
        checked ? "bg-brand" : "bg-muted/40",
      )}
    >
      <span
        className={cn(
          "absolute left-0.5 top-0.5 size-4 rounded-full bg-white shadow transition-transform",
          checked && "translate-x-4",
        )}
      />
    </span>
  );
}

/** Contador verde, igual ao de mensagens não lidas. */
export function CountBadge({ n }: { n: number }) {
  if (n <= 0) return null;
  return (
    <span className="inline-flex min-w-[20px] items-center justify-center rounded-full bg-unread px-1.5 text-[11.5px] font-semibold leading-5 text-accent-fg tabular-nums">
      {n}
    </span>
  );
}
