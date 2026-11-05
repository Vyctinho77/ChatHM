# Agenda (protótipo)

Status: **protótipo visual, somente na versão de demonstração** (dados em memória). O app normal não inclui a
página nem o item de menu; entram quando houver backend.

## Decisões de produto

- Itens: **compromisso com cliente** (data, hora opcional, cliente opcional, observação) e **prazo/vencimento**
  (dia inteiro).
- Visibilidade **por equipe, como as filas**: cada item pertence a uma equipe; só integrantes e admin enxergam.
- Calendário mensal; clicar num item abre um **card flutuante** ao lado dele (no celular, uma folha embaixo) com
  detalhes e ações: concluir/reabrir, editar, excluir e "Abrir conversa" quando há cliente. Clicar num dia vazio cria
  um item naquele dia; "+N mais" abre a lista do dia.

## Código

`apps/web/src/features/agenda`: `dates.ts` (datas locais, com testes), `data.ts` (dados fictícios e hooks em
memória), `FloatingCard.tsx` (posicionamento e fechamento), `AgendaCard.tsx` (detalhe, lista do dia, formulário),
`AgendaPage.tsx` (calendário). O menu e a rota são condicionais a `VITE_DEMO`, então o bundle normal não os contém.

## Backend proposto (ainda não implementado)

```
agenda_items(id, team_id → teams, kind appointment|deadline, title, date, time NULL,
             contact_id → contacts NULL, note, done_at NULL, created_by → users, created_at, updated_at)
```

- Leitura/escrita aplicam a mesma regra de visibilidade por equipe das conversas (admin vê tudo; fora da equipe → 404).
- Criar/editar exige ser integrante da equipe do item (ou admin); excluir idem. Auditoria de criar/editar/excluir.
- Item com cliente referencia o contato; "Abrir conversa" abre a conversa aberta do contato quando o usuário a vê.
- Fora do escopo inicial: recorrência (ex.: DAS todo dia 20), notificações/lembretes e sincronização com calendários externos.

## Perguntas em aberto

- Prazos fiscais recorrentes devem ser gerados automaticamente (modelo por mês)?
- Lembrete antes do horário (notificação no navegador)?
