# Corretor de acentos do campo de mensagem

Status: em desenvolvimento na branch `claude/corretor-acentos` (ainda não publicado). Código em `apps/web/src/features/conversations/`.

## O que faz

Quem escreve rápido digita "voce ja viu o orcamento?". Ao **fechar a palavra** (espaço, quebra de linha ou `. , ; : ! ? ) ]`), o campo troca por
"você já viu o orçamento?". Backspace logo em seguida desfaz aquela correção. Ao enviar, a última palavra (ainda sem espaço depois) também é corrigida.

Onde vale: mensagem do chat (e chat interno), legenda de anexo, lembrete pessoal e observação interna do cliente.
Configurações → **Escrita**: ligar/desligar os acentos, ligar as abreviações de chat e gerir a lista de palavras ignoradas. Mudar vale na hora.

## Como decide (três camadas)

1. **Lista curada** (`autocorrect.ts`, ~300 palavras): sempre vale, inclusive gírias de chat ("ta" → "tá", "ne" → "né").
2. **Dicionário estendido** (`ptAccents.data.ts`, ~4.700 palavras, arquivo separado de ~30 KB comprimido, carregado em segundo plano na primeira vez):
   gerado por `scripts/gen-pt-accents.py` a partir do dicionário **VERO pt-BR** (LGPL-3.0 ou MPL-2.0) e da **frequência de palavras de conversa**
   (OpenSubtitles 2024 via OPUS, ODC-BY). Só entra a palavra acentuada que existe na ortografia atual, cuja forma sem acento **não** é de uso comum
   ("esta", "pais", "para", "pra", "aqui" ficam como estão; "numero", "historia", "negocio" entram porque a acentuada domina a conversa).
3. **Terminações seguras**: -ção/-ções, -são/-sões, -stão, -zões, -ível/-áveis (palavras que não existem sem acento).

Respeita maiúsculas ("Voce" → "Você", "NAO" → "NÃO"; siglas de 2 letras como "PE" não), e-mail, link, caminho, `@menção`, `#tag` e `{variável}`.

## Aprende com você

- Desfazer a correção de uma palavra **duas vezes** a coloca na lista "Palavras que não corrijo" (Configurações → Escrita). Dá para adicionar e remover à mão.
- Na mesma digitação, a palavra desfeita não volta a ser corrigida naquele lugar.

## Abreviações (opcional, desligado por padrão)

"vc/vcs", "tb/tbm", "obg/obgd/obgda", "blz", "msg/msgs", "hj", "ctz", "cmg", "ctg", "mt/mto/mta", "td/tds", "qdo", "qto", "dps", "pfv", "vlw". Sempre com 2+ letras:
"n" e "q" sozinhos são ambíguos demais. Fica desligado porque o cliente passa a ver o texto expandido.

## O que continua fora

- **Erros de digitação** ("naoo", "vocee", letras trocadas) e concordância: o corretor só acentua e expande. O navegador sublinha o que não conhece (campo em pt-BR).
- **Nomes próprios em minúsculas**: "jose" vira "josé" (acento certo, mas continua minúsculo); "franca" (adjetivo) pode virar "frança".
- **Ctrl+Z** não desfaz uma correção (a troca é feita por código); o desfazer é o Backspace.
- Preferências ficam no navegador (não seguem a pessoa para outro computador).
- Campos de administração (orientações da IA, descrição de raias) não têm o corretor.

## Próximos passos (ordem sugerida)

1. **Erros de digitação com cuidado**: corrigir sozinho só quando há uma única palavra a 1 edição de distância e ela é muito mais frequente; senão, sugerir.
   Precisa de um filtro de palavras válidas (~1–2 MB, em Web Worker) além do mapa de acentos. Mesmo Backspace para desfazer.
2. **Maiúscula no começo da frase** e depois de ponto ("oi. tudo bem" → "Oi. Tudo bem"), opcional.
3. **Ctrl+Z** que desfaz a correção (inserir pelo mecanismo nativo de edição do navegador em vez de trocar o valor).
4. **Vocabulário do escritório**: palavras e nomes compartilhados pela equipe (administrador mantém a lista), e preferências na conta em vez do navegador.
5. **Medir**: contar localmente, sem guardar texto, quantas correções foram aplicadas e desfeitas por palavra, para podar do dicionário as que mais são desfeitas.
6. **Celular/IME**: validar com teclados Android/iOS (composição) em aparelhos reais; hoje o corretor não age durante composição.
7. **Aviso na primeira vez**: "Corrigi 'voce' para 'você'. Backspace desfaz", para ninguém estranhar.
8. **Renovar o dicionário** quando sair versão nova do VERO ou da lista de frequência (rodar `scripts/gen-pt-accents.py`, conferir a diferença no `git diff`).

## Testes

`autocorrect.test.ts` (palavras, dicionário gerado, opções), `accentController.test.ts` (motor: cursor, Backspace, composição, envio),
`writingPrefs.test.ts` (preferências e aprendizado). No navegador de teste: digitação real, cursor no meio do texto, Backspace, aprendizado, Configurações.
