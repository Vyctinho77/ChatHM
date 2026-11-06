import { Link } from "react-router";
import { Check, Circle } from "lucide-react";
import { useMe } from "@/features/auth/api";
import { useSegments } from "@/features/queues/api";
import { useTeams, useUsers } from "@/features/team/api";
import { useConnection } from "@/features/whatsapp/api";

/** Lista curta para o gestor deixar o escritório pronto. Só administradores, enquanto faltar algo. */
export function GettingStarted() {
  const { data: me } = useMe();
  const isAdmin = me?.user.role === "admin";
  const teams = useTeams();
  const segments = useSegments();
  const users = useUsers(isAdmin);
  const connection = useConnection();
  if (!isAdmin) return null;

  const steps = [
    { done: connection.data?.state === "connected", title: "WhatsApp conectado", to: "/whatsapp" },
    { done: (teams.data?.length ?? 0) > 0, title: "Criar equipes", to: "/equipe?aba=equipes" },
    { done: (segments.data?.length ?? 0) > 0, title: "Definir segmentos", to: "/filas/configurar" },
    { done: (users.data?.length ?? 0) > 1, title: "Enviar o token à equipe", to: "/equipe" },
  ];
  if (steps.every((s) => s.done)) return null;

  return (
    <ul className="mt-6 flex flex-col gap-1 text-left text-sm">
      {steps.map((s) => (
        <li key={s.title}>
          <Link to={s.to} className="flex items-center gap-2.5 rounded-lg px-3 py-1.5 hover:bg-hover">
            {s.done ? <Check className="size-4 text-brand" /> : <Circle className="size-4 text-muted" />}
            <span className={s.done ? "text-muted line-through" : "text-fg"}>{s.title}</span>
          </Link>
        </li>
      ))}
    </ul>
  );
}
