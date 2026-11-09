# Participantes no atendimento

Um cliente pode precisar de mais de um setor ao mesmo tempo (folha de pagamento e nota fiscal, por exemplo). O responsável continua um só,
mas outras pessoas, de qualquer equipe ou administradores, podem ser **incluídas no mesmo atendimento**.

- **Quem inclui ou tira:** o responsável, quem já participa, e administradores/gestores. A própria pessoa também pode sair.
- **O que o participante faz:** vê a conversa (mesmo que ela seja de outra fila) e responde, sem assumir nem tirar o responsável. Aparece em **Minhas**, com "Ana + você" na lista.
- **O que não muda:** transferir, encerrar e adiar continuam do responsável (e de administradores/gestores).
- **Onde:** botão de pessoas no cabeçalho da conversa, ou "Incluir pessoa no atendimento…" em Mais opções. O cabeçalho mostra "+ Bia".
- **Ao tirar a pessoa**, a conversa some da lista dela na hora, se ela não a enxergava por outra regra (equipe ou responsável).
- **Encerrou o atendimento:** a inclusão vale só para aquele atendimento; um novo contato do cliente abre outro sem participantes.
- Tudo é auditado (`conversation.participant_add` e `conversation.participant_remove`). Migração `00028`.
