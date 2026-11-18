# Corretor de acentos do campo de mensagem

Status: em desenvolvimento na branch `claude/corretor-acentos` (ainda não publicado). Código em `apps/web/src/features/conversations/`.

## O que faz

Quem escreve rápido digita "voce ja viu o orcamento?". Ao **fechar a palavra** (espaço, quebra de linha ou `. , ; : ! ? ) ]`), o campo troca por
"você já viu o orçamento?". Backspace logo em seguida desfaz aquela correção. Ao enviar, a última palavra (ainda sem espaço depois) também é corrigida.

Onde vale: mensagem do chat (e chat interno), legenda de anexo, lembrete pessoal e observação interna do cliente.
É **interno**: não há tela de configuração, nada aparece fora das caixas de texto. Está sempre ligado.

## Como decide (quatro camadas)

1. **Lista curada** (`autocorrect.ts`, ~300 palavras): sempre vale, inclusive gírias de chat ("ta" → "tá", "ne" → "né").
2. **Dicionário estendido** (`ptAccents.data.ts`, ~4.700 palavras, arquivo separado de ~30 KB comprimido, carregado em segundo plano na primeira vez):
   gerado por `scripts/gen-pt-accents.py` a partir do dicionário **VERO pt-BR** (LGPL-3.0 ou MPL-2.0) e da **frequência de palavras de conversa**
   (OpenSubtitles 2024 via OPUS, ODC-BY). Só entra a palavra acentuada que existe na ortografia atual, cuja forma sem acento **não** é de uso comum
   ("esta", "pais", "para", "pra", "aqui" ficam como estão; "numero", "historia", "negocio" entram porque a acentuada domina a conversa).
3. **Terminações seguras**: -ção/-ções, -são/-sões, -stão, -zões, -ível/-áveis (palavras que não existem sem acento).
4. **Erros de digitação** (`typos.ts`, ver abaixo): só quando a palavra digitada **não é palavra** e há uma palavra frequente a um engano de distância.

Respeita maiúsculas ("Voce" → "Você", "NAO" → "NÃO"; siglas de 2 letras como "PE" não), e-mail, link, caminho, `@menção`, `#tag` e `{variável}`.

## Erros de digitação (camada 4)

Só os quatro enganos mais comuns de quem digita rápido, e só se a palavra digitada **não existir** (nem entre as ~30 mil palavras frequentes, nem entre
as ~25 mil palavras raras válidas do VERO que o algoritmo trocaria por engano, que ficam numa lista de exceções):

| Engano | Exemplo | Tamanho mínimo |
|---|---|---|
| duas letras vizinhas trocadas | `tmepo` → `tempo`, `voecê` | 4 |
| faltou uma letra | `obrigdo` → `obrigado`, `pssoa` → `pessoa` | 5 |
| letra repetida sem querer | `obrigaddo` → `obrigado` | 5 |
| letra no lugar da tecla vizinha (teclado ABNT2) | `obrigafo` → `obrigado` | 6 |

- **Só com muita certeza**: com mais de um candidato, o melhor precisa ser 20x mais frequente que o segundo; palavras ambíguas (`pais`/`país`) não são mexidas.
- **Não mexe** em: risadas (`kkkk`, `rsrs`), siglas e termos do escritório (`cnpj`, `darf`, `esocial`, `whatsapp`...), palavras com acento, e-mail/link/menção,
  **nomes** (inicial maiúscula no meio da frase) e TUDO EM MAIÚSCULAS. Inicial maiúscula só é corrigida no começo da frase.
- Mesmo Backspace para desfazer e mesmo aprendizado (duas vezes desfeita = deixa de corrigir aquela palavra).
- Os dados (`ptWords.data.ts`, `ptTypoExceptions.data.ts`, ~180 KB comprimidos) carregam sob demanda, em segundo plano, junto com o dicionário de acentos.
- **Números** (simulação com erros artificiais em palavras frequentes, não com texto real de clientes): letras trocadas ~99,6% corrigidas certo; letra repetida
  ~99,9%; letra faltando ~83%; quase nunca troca por outra palavra (<0,5%). Palavras válidas raras do VERO: 0% trocadas (por causa da lista de exceções).
  Nomes, marcas, gírias e estrangeirismos que **não** estão no VERO nem na lista frequente é que podem ser trocados por engano: por isso o Backspace e o aprendizado.
- Regerar: `scripts/gen-pt-typos.py` (gera os dados e o `typos.fixture.json`; o teste confere que o TypeScript concorda com o Python em ~1.200 casos).

## Aprende com você (sem tela)

- Desfazer a correção de uma palavra **duas vezes** (Backspace) faz o corretor parar de corrigi-la neste navegador (nomes, siglas, termos do escritório).
  O aprendizado **expira em 90 dias**, para um engano antigo não valer para sempre. Fica no navegador (`chathm.writing.v2`).
- Na mesma digitação, a palavra desfeita não volta a ser corrigida naquele lugar.

## Decisões

- **Sem abreviações** ("vc" → "você" etc.): o cliente passaria a ver um texto que a pessoa não escreveu, e não há tela para ligar. O corretor só acentua.
- **Sem tela de configuração**: ligar/desligar, lista de ignoradas e abreviações foram retirados.

## O que continua fora

- **Erros que não são desses quatro tipos** (duas letras erradas na mesma palavra, letra errada qualquer, palavras coladas) e concordância: o navegador sublinha o que não conhece (campo em pt-BR).
- **Nomes próprios em minúsculas**: "jose" vira "josé" (acento certo, mas continua minúsculo); "franca" (adjetivo) pode virar "frança".
- **Ctrl+Z** não desfaz uma correção (a troca é feita por código); o desfazer é o Backspace.
- O que o corretor aprende fica no navegador (não segue a pessoa para outro computador).
- Campos de administração (orientações da IA, descrição de raias) não têm o corretor.

## Próximos passos (ordem sugerida)

1. ~~Erros de digitação com cuidado~~ **(feito: primeira versão, ver acima)**. Falta: sugerir (sem trocar sozinho) nos casos ambíguos e em letras erradas quaisquer; medir com texto real.
2. **Maiúscula no começo da frase** e depois de ponto ("oi. tudo bem" → "Oi. Tudo bem"), opcional.
3. **Ctrl+Z** que desfaz a correção (inserir pelo mecanismo nativo de edição do navegador em vez de trocar o valor).
4. **Vocabulário do escritório**: palavras e nomes aprendidos (e os nomes dos clientes e da equipe, que o app já conhece) compartilhados sem tela: o corretor passa a não mexer neles automaticamente, e o aprendizado vai para a conta em vez do navegador.
5. **Medir**: contar localmente, sem guardar texto, quantas correções foram aplicadas e desfeitas por palavra, para podar do dicionário as que mais são desfeitas.
6. **Celular/IME**: validar com teclados Android/iOS (composição) em aparelhos reais; hoje o corretor não age durante composição.
7. ~~Aviso na primeira vez~~ (retirado: o corretor é interno e discreto).
8. **Renovar o dicionário** quando sair versão nova do VERO ou da lista de frequência (rodar `scripts/gen-pt-accents.py`, conferir a diferença no `git diff`).

## Testes

`typos.test.ts` (algoritmo, concordância com o gerador em ~1.200 casos, capitalização, contexto), `autocorrect.test.ts` (palavras, dicionário gerado, opções), `accentController.test.ts` (motor: cursor, Backspace, composição, envio),
`writingPrefs.test.ts` (aprendizado e expiração). No navegador de teste: digitação real, cursor no meio do texto, Backspace, aprendizado, Configurações.
