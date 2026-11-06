import { Link } from "react-router";
import { Check, Circle } from "lucide-react";
import { useMe } from "@/features/auth/api";
import { useSegments } from "@/features/queues/api";
import { useTeams, useUsers } from "@/features/team/api";
import { useConnection } from "@/features/whatsapp/api";

/** Passos para o gestor deixar o escritório pronto. Só aparece para administradores, enquanto faltar algo. */
export function GettingStarted() {
  const { data: me } = useMe();
  const isAdmin = me?.user.role === "admin";
  const teams = useTeams();
  const segments = useSegments();
  const users = useUsers(isAdmin);
  const connection = useConnection();
  if (!isAdmin) return null;

  const steps = [
    { done: (teams.data?.length ?? 0) > 0, title: "Crie as equipes", hint: "Ex.: Folha, Fiscal, Atendimento. Elas definem quem vê cada fila.", to: "/equipe?aba=equipes", cta: "Criar equipes" },
    { done: (segments.data?.length ?? 0) > 0, title: "Defina os segmentos", hint: "Assuntos e palavras-chave que encaminham a conversa à equipe certa.", to: "/filas/configurar", cta: "Configurar filas" },
    { done: (users.data?.length ?? 0) > 1, title: "Convide as pessoas", hint: "Você gera um código e cada pessoa configura o próprio autenticador.", to: "/equipe", cta: "Convidar" },
    { done: connection.data?.state === "connected", title: "Conecte o WhatsApp", hint: "Leia o QR Code com o celular do escritório.", to: "/whatsapp", cta: "Conectar" },
  ];
  if (steps.every((s) => s.done)) return null;

  return (
    <div className="mt-6 w-full max-w-md rounded-xl border border-line bg-panel p-4 text-left">
      <p className="text-sm font-semibold">Primeiros passos</p>
      <ol className="mt-3 flex flex-col gap-3">
        {steps.map((s) => (
          <li key={s.title} className="flex items-start gap-3">
            {s.done ? (
              <Check className="mt-0.5 size-4 shrink-0 text-brand" aria-label="Concluído" />
            ) : (
              <Circle className="mt-0.5 size-4 shrink-0 text-muted" aria-label="Pendente" />
            )}
            <div className="min-w-0 flex-1">
              <p className={s.done ? "text-sm text-muted line-through" : "text-sm font-medium"}>{s.title}</p>
              {!s.done && <p className="text-xs text-muted">{s.hint}</p>}
            </div>
            {!s.done && (
              <Link to={s.to} className="shrink-0 text-xs font-medium text-accent hover:underline">
                {s.cta}
              </Link>
            )}
          </li>
        ))}
      </ol>
    </div>
  );
}
