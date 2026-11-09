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
  (tamanhos salvos no navegador). O perfil do usuário (com "Sair") fica no rodapé do menu lateral, abaixo de
  Configurações; o cabeçalho tem apenas o logo e o seletor de tema.
- Painel do contato fechado por padrão; abre ao clicar no nome/foto no cabeçalho do chat.
  Em telas ≥1280px vira uma coluna; em telas menores sobrepõe o chat sem desmontá-lo.
- Rascunhos ficam por conversa (sessionStorage): sobrevivem à troca de conversa e ao abrir/fechar o painel.
- Abaixo de 1024px o menu vira gaveta (☰); abaixo de 768px lista e chat se alternam (botão voltar).
- Tema claro / escuro / sistema: tokens CSS em `index.css` (`bg-panel`, `text-muted`, `bg-out`…),
  aplicado antes do primeiro paint e salvo no usuário (`PATCH /me/preferences`).

## Dados

| Área | Fonte |
|---|---|
| Login, MFA, sessão, tema, Equipe, Configurações | API real |
| Conversas, mensagens, observação do contato, Conexão WhatsApp | API real + WebSocket (`features/realtime`) |
| Filas / Segmentos, Etiquetas, responder (assumir), encerrar, etiquetar | API real |
| Anexos e áudio | desativados até a Fase 4 (`capabilities.media` em `store.api.ts`) |

`features/conversations/store.ts` escolhe a implementação no build: `store.api.ts` no app e `store.mock.ts`
(dados fictícios) somente no build de demonstração — o bundle de produção não contém os dados fictícios.

## Telas da Fase 3

- **Filas / Segmentos** (`features/queues`), no padrão de listas do WhatsApp (componentes em `components/list.tsx`):
  - `/filas`: uma linha por fila (ícone, nome, "Equipe X · N em atendimento", contador verde de aguardando, seta).
    Tocar abre a lista de conversas já filtrada (`/conversas?fila=folha`). Atendentes só veem as filas da
    própria equipe + Não atribuídos. A engrenagem (só admin) leva à configuração.
  - `/filas/configurar` (admin): testar uma mensagem e lista de segmentos (+ Novo segmento).
  - `/filas/configurar/:key` (admin): Nome, Equipe, interruptor "Classificar mensagens", palavras e
    expressões (tocar edita a prioridade ou remove) e Excluir segmento; edições em janelas simples.
  - Princípios: a linha é a unidade, sem caixas por item; opções raras atrás da engrenagem e em telas próprias;
    verde como único destaque; textos de ajuda de uma linha. Não existe guia de design oficial do WhatsApp,
    então o padrão segue as listas de configurações dele e a documentação de listas da Apple e do Material.
- **Etiquetas** (`features/tags`): catálogo; só o admin cria, renomeia e exclui.
- **Chat**: responder assume a conversa; se outra pessoa assumir antes, o texto volta ao campo e o aviso explica.
  Conversa assumida por outra pessoa bloqueia o campo e some da lista de quem perdeu o acesso (evento
  `conversation.removed`). Falha de envio mantém a bolha com "Tentar de novo".
- Balões com imagem: borda fina, imagem de ponta a ponta, legenda e autor abaixo, hora sobre a imagem sem legenda.

## Agenda (protótipo, só na demonstração)

Calendário mensal com card flutuante por item; compromissos e prazos; visibilidade por equipe. Detalhes e plano de
backend em [`agenda.md`](agenda.md).

## Tipografia

Inter (variável, empacotada com o app via `@fontsource-variable/inter`, sem chamadas externas), corpo em 13,5px.
Escala usada: 10,5–11px (metadados, horários), 12–12,5px (prévias, rótulos), 13–13,5px (texto, nomes),
15–16px (títulos de painel), 20px (títulos de página).

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

## Conversas: arquivos, digitando e quem atende

- **Documentos** (`conversations/docKind.ts`, `DocBadge.tsx`): PDF, Word, Excel e PowerPoint têm ícone próprio (cor e letra de cada
  aplicativo, desenho próprio, sem usar as marcas registradas). Reconhecidos pela extensão (maiúsculas não importam) e, na falta
  dela, pelo tipo MIME. PDF abre no visualizador do app; Word/Excel/PowerPoint baixam o arquivo com o nome certo, e o navegador
  oferece abrir no Office instalado no computador (o app não abre `.docx`/`.xlsx` por dentro, para não enviar o arquivo a terceiros).
- **Arrastar arquivos** (`useFileDrop.tsx`, `DropOverlay.tsx`): arrastar do Explorer/Finder para qualquer ponto do chat mostra
  “Solte para enviar” e abre a mesma prévia de anexos de “Anexar” (até 10 arquivos de 25 MB; jpeg/png/webp vão como foto, o
  resto como documento). Se não dá para enviar (outra pessoa atende, encerrado, adiado), o aviso diz o motivo. Soltar fora do
  chat ou em pasta nunca abre o arquivo no navegador; pasta e arquivo vazio mostram aviso.
- **Digitando**: além do cabeçalho e da lista, uma bolha com três pontos aparece no fim das mensagens (e a conversa rola até ela
  se a pessoa já está no fim).
- **Quem atende**: o primeiro nome do responsável aparece embaixo da hora na lista de conversas e, dentro da conversa, em um selo ao lado do nome do cliente (“Você” para
  o próprio usuário). A etiqueta da fila geral só diz “Não atribuídos” enquanto ninguém atende; com responsável, diz “Fila geral”.
