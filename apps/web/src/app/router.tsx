import { createBrowserRouter, createMemoryRouter, Navigate, type RouteObject } from "react-router";
import { DEMO } from "@/lib/env";
import { Contact, Layers, Smartphone, Tag } from "lucide-react";
import { ConversationsPage } from "@/features/conversations/ConversationsPage";
import { ComingSoon } from "@/features/placeholder/ComingSoon";
import { SettingsPage } from "@/features/settings/SettingsPage";
import { TeamPage } from "@/features/team/TeamPage";
import { AdminOnly, Root } from "./Root";

const routes: RouteObject[] = [
  {
    path: "/",
    element: <Root />,
    children: [
      { index: true, element: <Navigate to="/conversas" replace /> },
      { path: "conversas/:id?", element: <ConversationsPage /> },
      {
        path: "filas",
        element: (
          <ComingSoon
            icon={Layers}
            title="Filas / Segmentos"
            description="Acompanhe as filas de Clientes novos, Folha de pagamento, Emissão de notas e Geral, com o que está aguardando e em atendimento."
            phase="Fase 3 — segmentação e filas automáticas"
          />
        ),
      },
      {
        path: "clientes",
        element: (
          <ComingSoon
            icon={Contact}
            title="Clientes"
            description="Contatos com nome, telefone, empresa, observações e histórico de atendimentos."
            phase="Fase 2 — contatos recebidos pelo WhatsApp"
          />
        ),
      },
      {
        path: "etiquetas",
        element: (
          <ComingSoon
            icon={Tag}
            title="Etiquetas"
            description="Catálogo de etiquetas (PENDENTE, URGENTE, PGDAS, CERTIFICADO…). Atendentes aplicam; administradores gerenciam."
            phase="Fase 3"
          />
        ),
      },
      { path: "equipe", element: <TeamPage /> },
      {
        element: <AdminOnly />,
        children: [
          {
            path: "whatsapp",
            element: (
              <ComingSoon
                icon={Smartphone}
                title="Conexão WhatsApp"
                description="Status da sessão, número conectado, QR Code temporário e reconexão. Somente administradores."
                phase="Fase 2 — conectar WhatsApp"
              />
            ),
          },
        ],
      },
      { path: "configuracoes", element: <SettingsPage /> },
      { path: "*", element: <Navigate to="/conversas" replace /> },
    ],
  },
];

// Na demonstração (página publicada em iframe) a navegação fica em memória.
export const router = DEMO ? createMemoryRouter(routes, { initialEntries: ["/conversas"] }) : createBrowserRouter(routes);
