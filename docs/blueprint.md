# ChatHM — Blueprint do MVP

**Objetivo:** central web de atendimento para escritórios contábeis, conectada ao WhatsApp por QR Code, que classifica mensagens recebidas e as distribui automaticamente por equipe.
**Escopo:** MVP de um escritório, com arquitetura preparada para evolução.

## Stack e arquitetura

- **Frontend:** React + TypeScript + Vite; Tailwind CSS; TanStack Query; WebSocket para atualizações em tempo real.
- **Backend:** Go (API REST com chi), módulos de autenticação, conversas, segmentação, equipes e integração WhatsApp.
- **Dados:** PostgreSQL; armazenamento privado compatível com S3 para anexos. Redis/fila apenas se o volume justificar; começar com jobs persistidos no banco.
- **Integração:** adaptador `WhatsAppProvider` para desacoplar o sistema do provedor. Primeira opção a avaliar: sessão espelhada por QR Code com whatsmeow (não oficial; sujeita a instabilidade/restrições). Preservar possibilidade de migrar para a WhatsApp Business Platform oficial, cujo fluxo de conexão é diferente.
- **Implantação:** Docker Compose no desenvolvimento; HTTPS e backups em produção. Monólito modular em Go, sem microsserviços no MVP.

## Regras de negócio do MVP

1. **Segmentos iniciais:** Clientes novos, Folha de pagamento e Emissão de notas.
2. **Classificação automática** pela mensagem do cliente, com palavras e expressões configuráveis (ex.: "quero emitir uma nota" → Emissão de notas; "preciso calcular a folha" → Folha; "quero contratar" → Clientes novos). Normalizar maiúsculas/acentos e comparar palavras completas; prever prioridades.
3. **Sem correspondência clara ou com conflito** entre segmentos: a conversa aparece na fila geral visível a todas as equipes. Não criar cópias da conversa e não enviar menu automático.
4. **Distribuição por equipe, sem triagem manual:** mensagens identificadas chegam diretamente à fila da equipe. Todos os integrantes autorizados visualizam; o primeiro que responder assume o atendimento de forma atômica, evitando respostas simultâneas.
5. **Conversa em andamento:** novas mensagens mantêm o responsável e a equipe atuais; não redistribuir automaticamente a cada frase. O responsável pode encerrar o atendimento; após encerramento, uma nova demanda pode ser reclassificada.
6. **Mídias essenciais:** enviar/receber texto, áudios, imagens (inclusive opção HD quando suportada), documentos e localização estática. Imagem como documento pode ser alternativa para preservar qualidade; verificar suporte real no adaptador.

## Interface e menu contábil (MVP)

**Layout:** inspirado no WhatsApp Web, com painéis ajustáveis no estilo grid do VS Code: menu lateral esquerdo → lista de conversas → chat. O painel de informações do contato fica fechado por padrão, abre ao clicar na foto/nome no cabeçalho e pode ser fechado sem sair da conversa. Não usar uma segunda mini-sidebar à esquerda. Temas claro, escuro e sistema, com preferência persistida por usuário.

**Menu lateral principal (enxuto):**

| Item | Função inicial | Acesso |
|---|---|---|
| Conversas | Caixa de entrada, busca, filtros Todas, Não lidas, Minhas e Gerais; mensagens e atendimento. | Atendentes e admin, respeitando equipes. |
| Filas / Segmentos | Filas de Clientes novos, Folha de pagamento, Emissão de notas e Não atribuídos; aguardando e em atendimento. | Equipes autorizadas e admin. |
| Clientes | Contatos com nome, telefone, empresa (opcional), observações e histórico. | Usuários autorizados. |
| Etiquetas | Criar/aplicar/remover tags (PENDENTE, URGENTE, PGDAS, CERTIFICADO), sem alterar o segmento. | Aplicar: atendentes; catálogo: admin. |
| Equipe | Listar membros e vínculos; admin cadastra usuários e define equipes. | Consulta conforme permissão; edição pelo admin. |
| Conexão WhatsApp | Status da sessão, número conectado, QR Code temporário, reconexão/desconexão. | Somente admin. |
| Configurações (rodapé) | Tema, perfil, troca de senha e notificações básicas. | Cada usuário; gerais pelo admin. |

**Painel contextual do contato (opcional):** foto, nome, telefone, segmento atual, etiquetas e observação interna. Não duplicar filas no menu e no chat, nem ocupar permanentemente espaço com detalhes de contato.

**Critérios de aceite de interface:** menu único à esquerda; conversa ocupa a área restante com o painel direito fechado; abrir/fechar detalhes não perde histórico nem rascunho; filtros respeitam visibilidade por equipe; tema sistema acompanha o dispositivo; layout utilizável em telas menores.

## Fase 1 — Fundação e acesso seguro

**Construir:** monorepo `apps/web` e `backend`; PostgreSQL e migrações; login por e-mail e senha; criação de usuários por administrador; perfis admin e atendente; equipes e membros. Tabelas: `users`, `teams`, `team_members`, `sessions`, `audit_logs`.

**Segurança:** Argon2id; cookies HttpOnly, Secure, SameSite; expiração/revogação; proteção contra brute force e CSRF; TLS em produção; autorização no backend. Admin com MFA antes da operação real.

**Aceite:** login/logout; senha nunca em texto puro; telas e APIs respeitam permissões; admin cadastra atendente e vincula a equipes; testes de autenticação e autorização passam.

## Fase 2 — Conectar WhatsApp e receber conversas

**Construir:** interface `WhatsAppProvider`; painel de conexão por QR temporário; estados desconectado, aguardando QR, conectado, erro; persistência protegida da sessão; recepção de eventos e gravação em `contacts`, `conversations`, `messages`, `whatsapp_connections`. Dedupe por ID externo.

**Segurança:** só admin gerencia QR e sessão; material de sessão com AES-256-GCM, chave fora do banco; logs sem segredos. Validar reconexão e compatibilidade do provedor.

**Aceite:** admin conecta pelo QR; status em tempo real; texto recebido aparece uma única vez; reinício não perde sessão válida; falhas e desconexões visíveis.

## Fase 3 — Segmentação e filas automáticas

**Construir:** `segments`, `segment_keywords`, `conversation_assignments`; classificador determinístico com normalização, expressões e prioridades; filas por segmento + geral; vínculo segmento → equipe; atribuição ao primeiro que responder com transação/bloqueio no PostgreSQL; WebSocket.

**Aceite:** "quero emitir uma nota" → Emissão de notas; "preciso de férias de funcionário" → Folha; "quero orçamento" → Clientes novos; "bom dia" e conflitos → todas as equipes; sem duplicação; dois atendentes simultâneos não assumem o mesmo atendimento; conversa ativa não muda de fila por palavras isoladas.

## Fase 4 — Chat completo para atendimento

**Construir:** layout responsivo, navegação, painel contextual retrátil, temas, histórico, busca, estados novo/em atendimento/encerrado, responsável, envio de texto. Áudio, imagens, documentos e localização estática nos dois sentidos; gravação de áudio no navegador; anexos em armazenamento privado; feedback de envio/erro e recuperação de falhas.

**Segurança:** URLs de arquivo de curta duração após autorização; validação de tamanho/tipo real, limites, antimalware; criptografia em trânsito e em repouso (inclusive backups); acesso restrito à equipe ou ao responsável.

**Aceite:** recebe e responde no mesmo painel; áudios tocam e são enviados; imagem, documento e localização trafegam nos dois sentidos; HD funciona ou mostra alternativa; anexos protegidos; erros não apagam mensagens nem duplicam envios.

## Fase 5 — Validação, proteção de dados e piloto

**Construir:** E2E com múltiplos atendentes, reconexão, duplicatas e anexos grandes; logs estruturados sem conteúdo sensível, métricas e alertas; backups com teste de restauração; retenção/exclusão, revisão de permissões e procedimento de incidentes (LGPD); documentação de instalação e operação.

**Aceite:** fluxo QR → mensagem → classificação → fila → resposta → encerramento no piloto; nenhuma equipe acessa conversas de outra; restauração comprovada; duplicatas não geram atendimentos duplicados; limitações documentadas.

## Ordem prática

login + equipes → QR + receber texto → classificação + filas → respostas + mídias → testes e piloto. Sempre: migrações versionadas, testes automatizados e autorização no Go.

## Fora do MVP

IA, chatbot/menu automático, integração com ERP contábil, CRM, campanhas, dashboards avançados, localização em tempo real e múltiplos escritórios (SaaS).
