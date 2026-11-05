# Frontend (`apps/web`)

React 19 + TypeScript + Vite, Tailwind CSS v4, TanStack Query, React Router, painéis redimensionáveis
(`react-resizable-panels`) e ícones `lucide-react`.

## Estrutura

```
src/
├── app/               # rotas (router.tsx) e Root: decide login → MFA → aplicação
├── components/        # UI compartilhada: Avatar, Button, Field, Modal, Menu, Logo
├── lib/               # api (cliente + CSRF), theme, format, cn
└── features/
    ├── auth/          # login, verificação/ativação de MFA, hook useMe
    ├── shell/         # TopBar, Sidebar (menu único), tema, layout em painéis
    ├── conversations/ # lista, chat, mensagens, composer, painel do contato
    ├── team/          # Equipe: pessoas e equipes (API real da Fase 1)
    ├── settings/      # Configurações: tema, perfil, MFA, troca de senha
    └── placeholder/   # telas das próximas fases (Filas, Clientes, Etiquetas, WhatsApp)
```

## Layout

- Menu lateral único à esquerda → lista de conversas → chat, separados por divisórias arrastáveis
  (tamanhos salvos no navegador).
- Painel do contato fechado por padrão; abre ao clicar no nome/foto no cabeçalho do chat.
  Em telas ≥1280px vira uma coluna; em telas menores sobrepõe o chat sem desmontá-lo.
- Rascunhos ficam por conversa (sessionStorage): sobrevivem à troca de conversa e ao abrir/fechar o painel.
- Abaixo de 1024px o menu vira gaveta (☰); abaixo de 768px lista e chat se alternam (botão voltar).
- Tema claro / escuro / sistema: tokens CSS em `index.css` (`bg-panel`, `text-muted`, `bg-out`…),
  aplicado antes do primeiro paint e salvo no usuário (`PATCH /me/preferences`).

## Dados

| Área | Fonte |
|---|---|
| Login, MFA, sessão, tema, Equipe, Configurações | API real (Fase 1) |
| Conversas, mensagens, etiquetas, observações | **Dados fictícios** em `features/conversations/mock.ts` |

Os hooks de `features/conversations/store.ts` (`useConversations`, `useMessages`, `useSendMessage`…) já têm
a forma que a API terá; nas Fases 2–3 só a implementação deles muda (REST + WebSocket), não os componentes.

Já funcionam na interface com os dados fictícios: envio otimista (pendente → enviada, falha marcada),
assumir a conversa ao responder, bloqueio do composer quando outra pessoa atende, encerrar atendimento,
anexar documento/imagem (local), gravar áudio pelo navegador (MediaRecorder), busca na conversa,
filtros Todas / Não lidas / Minhas / Gerais e filtro por segmento.

## Sem suporte por decisão

Ligação e chamada de vídeo não aparecem no cabeçalho do chat: o blueprint não as inclui e o adaptador
WhatsApp não as oferece.

## Build de demonstração

`pnpm --filter @chathm/web build:demo` gera `apps/web/dist-demo/`, uma versão estática com a API simulada
no navegador (`src/lib/demo-api.ts`) e navegação em memória, para mostrar a interface sem backend.
Contas: `admin@hm.com.br`, `heliton@hm.com.br`, `paula@hm.com.br`, todas com a senha `demo`.
O build normal não inclui esse código (a condição `import.meta.env.VITE_DEMO` é eliminada pelo bundler).
