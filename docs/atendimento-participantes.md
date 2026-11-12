# Participantes no atendimento

Um cliente pode precisar de mais de um setor ao mesmo tempo (folha de pagamento e nota fiscal, por exemplo). O responsável continua um só,
mas outras pessoas, de qualquer equipe ou administradores, podem ser **incluídas no mesmo atendimento**.

- **Quem inclui ou tira:** o responsável, quem já participa, e administradores/gestores. A própria pessoa também pode sair.
- **O que o participante faz:** vê a conversa (mesmo que ela seja de outra fila) e responde, sem assumir nem tirar o responsável. Aparece em **Minhas**, com "Ana + você" na lista.
- **O que não muda:** transferir continua do responsável (e de administradores/gestores); adiar para todos também, mas qualquer pessoa pode adiar **só para si** (ver `adiamento.md`). **Encerrar para todos** também; mas quem participa pode **encerrar a própria parte** (ver o fim desta página).
- **Onde:** botão de pessoas no cabeçalho da conversa, ou "Incluir pessoa no atendimento…" em Mais opções. O cabeçalho mostra "+ Bia".
- **Ao tirar a pessoa**, a conversa some da lista dela na hora, se ela não a enxergava por outra regra (equipe ou responsável).
- **Encerrou o atendimento:** a inclusão vale só para aquele atendimento; um novo contato do cliente abre outro sem participantes.
- Tudo é auditado (`conversation.participant_add` e `conversation.participant_remove`). Migração `00028`.

## Encerrar: a própria parte ou para todos

Encerrar não precisa mais acabar o atendimento de todo mundo. Quem está no atendimento (responsável ou participante) escolhe:

| Ação | Quem usa | O que acontece |
|---|---|---|
| **Encerrar minha parte** | responsável e participantes | A pessoa sai; a conversa continua aberta para quem ficou |
| **Encerrar para todos** | responsável, gestor, administrador | O atendimento termina para todos; o cliente que escrever depois volta para a fila |

- **Participante** que encerra a parte: sai do atendimento (o mesmo que "tirar a pessoa") e a conversa continua com o responsável. Numa
  conversa de fila, ainda sem responsável, ela só sai: nunca a encerra.
- **Responsável** que encerra a parte, havendo participantes: escolhe quem continua como responsável (a mais antiga vem marcada) e,
  se tem lembrete na conversa, se ele vai junto (como numa transferência). A pessoa que sai fica no histórico como "transferida".
- **Única pessoa no atendimento:** "Encerrar atendimento" é um botão só, sem pergunta (as duas ações são iguais). A última pessoa a sair
  também encerra o atendimento.
- **Administrador ou gestor de fora:** não tem "parte" para encerrar. Só "Encerrar para todos", com uma confirmação que diz quem está
  no atendimento. As pessoas afetadas recebem um aviso na tela ("Admin Um encerrou o atendimento de … para todos"), e quem recebe o
  atendimento de quem saiu também ("Beatriz passou o atendimento de … para você").
- **Cliente escreve depois que alguém saiu:** a mensagem cai na mesma conversa, com quem ficou.

API: `POST /api/v1/conversations/{id}/close` com `{ "scope": "all" | "me", "passTo"?, "shareReminder"? }`; sem corpo, encerra para todos
(como antes). `scope: "me"` por quem não está no atendimento responde `403 not_involved`; `passTo` precisa ser uma participante ativa
(`422`); sem `passTo`, passa à participante mais antiga. Auditoria: `conversation.close` (com `scope` e as pessoas afetadas) e
`conversation.leave` (`role`: `participant` ou `assignee`, e `handoverTo`). Aviso em tempo real: `conversation.notice`
(`closed_for_all` ou `handed_over`), só para as pessoas envolvidas.
