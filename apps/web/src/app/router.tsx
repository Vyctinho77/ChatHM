import { createBrowserRouter, createMemoryRouter, Navigate, type RouteObject } from "react-router";
import { DEMO } from "@/lib/env";
import { ConversationsPage } from "@/features/conversations/ConversationsPage";
import { ContactsPage } from "@/features/contacts/ContactsPage";
import { AgendaPage } from "@/features/agenda/AgendaPage";
import { QueuesConfigPage } from "@/features/queues/QueuesConfigPage";
import { QueuesPage } from "@/features/queues/QueuesPage";
import { SegmentDetailPage } from "@/features/queues/SegmentDetailPage";
import { OAuthConsentPage } from "@/features/settings/OAuthConsentPage";
import { SettingsPage } from "@/features/settings/SettingsPage";
import { McpPage } from "@/features/mcp/McpPage";
import { TagsPage } from "@/features/tags/TagsPage";
import { TeamPage } from "@/features/team/TeamPage";
import { WhatsAppPage } from "@/features/whatsapp/WhatsAppPage";
import { AdminOnly, Root } from "./Root";

const routes: RouteObject[] = [
  {
    path: "/",
    element: <Root />,
    children: [
      { index: true, element: <Navigate to="/conversas" replace /> },
      { path: "conversas/:id?", element: <ConversationsPage /> },
      { path: "filas", element: <QueuesPage /> },
      { path: "agenda", element: <AgendaPage /> },
      { path: "clientes", element: <ContactsPage /> },
      { path: "etiquetas", element: <TagsPage /> },
      { path: "equipe", element: <TeamPage /> },
      { path: "integracoes/mcp", element: <McpPage /> },
      {
        element: <AdminOnly />,
        children: [
          { path: "whatsapp", element: <WhatsAppPage /> },
          { path: "filas/configurar", element: <QueuesConfigPage /> },
          { path: "filas/configurar/:key", element: <SegmentDetailPage /> },
        ],
      },
      { path: "configuracoes", element: <SettingsPage /> },
      { path: "oauth/consent", element: <OAuthConsentPage /> },
      { path: "*", element: <Navigate to="/conversas" replace /> },
    ],
  },
];

// Na demonstração (página publicada em iframe) a navegação fica em memória.
export const router = DEMO ? createMemoryRouter(routes, { initialEntries: ["/conversas"] }) : createBrowserRouter(routes);
