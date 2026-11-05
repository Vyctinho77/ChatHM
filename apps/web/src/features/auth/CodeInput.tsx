import { Field } from "@/components/ui";

/** Campo para código TOTP de 6 dígitos (aceita colar com espaços). */
export function CodeInput({ value, onChange, error }: { value: string; onChange: (v: string) => void; error?: string }) {
  return (
    <Field
      label="Código de 6 dígitos"
      inputMode="numeric"
      autoComplete="one-time-code"
      autoFocus
      maxLength={7}
      placeholder="000000"
      className="[&_input]:text-center [&_input]:font-mono [&_input]:text-lg [&_input]:tracking-[0.4em]"
      value={value}
      error={error}
      onChange={(e) => onChange(e.target.value.replace(/\D/g, "").slice(0, 6))}
    />
  );
}
