import {
  forwardRef,
  useEffect,
  useId,
  useRef,
  type ButtonHTMLAttributes,
  type InputHTMLAttributes,
  type ReactNode,
} from "react";
import { LoaderCircle, X } from "lucide-react";
import { cn } from "@/lib/cn";

type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: "primary" | "secondary" | "ghost" | "danger";
  loading?: boolean;
};

export function Button({ variant = "primary", loading, className, children, disabled, ...rest }: ButtonProps) {
  return (
    <button
      {...rest}
      disabled={disabled || loading}
      className={cn(
        "inline-flex h-8 items-center justify-center gap-1.5 rounded-lg px-3.5 text-[13px] font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-60",
        variant === "primary" && "bg-accent text-accent-fg hover:brightness-110",
        variant === "secondary" && "border border-line bg-elevated hover:bg-hover",
        variant === "ghost" && "hover:bg-hover",
        variant === "danger" && "bg-danger text-white hover:brightness-110",
        className,
      )}
    >
      {loading && <LoaderCircle className="size-4 animate-spin" />}
      {children}
    </button>
  );
}

export function IconButton({
  label,
  className,
  children,
  active,
  ...rest
}: ButtonHTMLAttributes<HTMLButtonElement> & { label: string; active?: boolean }) {
  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      {...rest}
      className={cn(
        "inline-flex size-8 shrink-0 items-center justify-center rounded-lg text-muted transition-colors hover:bg-hover hover:text-fg disabled:opacity-50",
        active && "bg-hover text-fg",
        className,
      )}
    >
      {children}
    </button>
  );
}

type FieldProps = InputHTMLAttributes<HTMLInputElement> & { label: string; error?: string; hint?: string };

export const Field = forwardRef<HTMLInputElement, FieldProps>(function Field(
  { label, error, hint, className, id, ...rest },
  ref,
) {
  const auto = useId();
  const inputId = id ?? auto;
  return (
    <div className={cn("flex flex-col gap-1.5", className)}>
      <label htmlFor={inputId} className="text-[13px] font-medium">
        {label}
      </label>
      <input
        ref={ref}
        id={inputId}
        aria-invalid={!!error}
        aria-describedby={error || hint ? `${inputId}-desc` : undefined}
        {...rest}
        className={cn(
          "h-9 rounded-lg border bg-elevated px-3 text-[13px] outline-none transition-colors placeholder:text-muted focus:border-accent",
          error ? "border-danger" : "border-line",
        )}
      />
      {(error || hint) && (
        <p id={`${inputId}-desc`} className={cn("text-xs", error ? "text-danger" : "text-muted")}>
          {error ?? hint}
        </p>
      )}
    </div>
  );
});

export function Alert({ children, tone = "danger" }: { children: ReactNode; tone?: "danger" | "info" }) {
  return (
    <div
      role="alert"
      className={cn(
        "rounded-lg border px-3 py-2 text-[13px]",
        tone === "danger" ? "border-danger/40 bg-danger/10 text-danger" : "border-accent/40 bg-accent/10 text-fg",
      )}
    >
      {children}
    </div>
  );
}

export function Spinner({ className }: { className?: string }) {
  return <LoaderCircle className={cn("size-5 animate-spin text-muted", className)} aria-label="Carregando" />;
}

/** Modal acessível baseado em <dialog> nativo (foco e Esc gerenciados pelo navegador). */
export function Modal({
  open,
  onClose,
  title,
  children,
  className,
}: {
  open: boolean;
  onClose: () => void;
  title: string;
  children: ReactNode;
  className?: string;
}) {
  const ref = useRef<HTMLDialogElement>(null);
  useEffect(() => {
    const d = ref.current;
    if (!d) return;
    if (open && !d.open) d.showModal();
    if (!open && d.open) d.close();
  }, [open]);
  return (
    <dialog
      ref={ref}
      onClose={onClose}
      onClick={(e) => e.target === ref.current && onClose()}
      className={cn(
        "m-auto w-[calc(100%-2rem)] max-w-md rounded-2xl border border-line bg-panel p-0 text-fg shadow-2xl backdrop:bg-black/50",
        className,
      )}
    >
      {open && (
        <div className="flex flex-col gap-4 p-5">
          <div className="flex items-center justify-between">
            <h2 className="text-base font-semibold">{title}</h2>
            <IconButton label="Fechar" onClick={onClose}>
              <X className="size-5" />
            </IconButton>
          </div>
          {children}
        </div>
      )}
    </dialog>
  );
}

export function Badge({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <span
      className={cn(
        "inline-flex min-w-[18px] items-center justify-center rounded-full px-1.5 text-[10.5px] font-semibold leading-[18px] tabular-nums",
        className,
      )}
    >
      {children}
    </span>
  );
}
