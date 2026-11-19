# Quem está online na equipe

**Online** = a pessoa tem o ChatHM aberto em pelo menos uma aba do navegador (conexão em tempo real). Não é "ativa há pouco": uma aba esquecida aberta conta como online.

- **Servidor** (`backend/internal/realtime/hub.go`): conta as conexões de cada pessoa. A primeira conexão avisa `presence {userId, online: true}` a todos; ao fechar a última,
  a pessoa só some depois de **20 segundos** de carência (um F5 ou uma reconexão rápida não pisca "offline"). `GET /api/v1/presence` devolve quem está online (estado inicial).
- **Quem vê**: qualquer pessoa logada (a lista da equipe já é visível para todos). Quem usa só token de IA (MCP) não conta: não abre o app.
- **Tela**: no card de transferência (botão direito num contato, ou o botão de transferir no chat) cada pessoa online tem uma bolinha verde no avatar, e o cabeçalho
  mostra "N online". Quem está offline fica sem bolinha. A ordem da lista não muda (nada se mexe enquanto você escolhe). A mudança chega ao vivo, sem recarregar.
- Ao reconectar depois de uma queda, a lista é pedida de novo ao servidor.
- Testes: `presence_test.go` (entrar, sair, duas abas, carência, F5), `presence.test.ts`.
