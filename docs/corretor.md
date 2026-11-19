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
3. **Terminações** (`ptSuffixes.data.ts`, ~15 KB comprimidos): 564 regras de terminação **mineradas do VERO** por `scripts/gen-pt-suffixes.py` (cobrem
   -ência/-ância, -ário/-ório, -ístico/-ítico/-ógico/-ético, -ção/-ções, -ável/-ível, -íssimo etc.: `pendencia` → `pendência`, `tecnologico` → `tecnológico`).
   Cada regra tem apoio (≥40 palavras) e parcela dominante (≥90% das palavras que precisam de acento naquela terminação usam o mesmo). Palavras **válidas de uso comum**
   que casam com a regra ficam como estão (lista de ~4 mil exceções: `evidencia`, `influencia`, `politica`); substantivos que o VERO aceita sem acento só por causa de um
   verbo raro (`pendencia`, `gramatica`) continuam sendo corrigidos. Até o arquivo carregar, valem as terminações escritas à mão (-ção, -são, -ível, -ável...).
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

## Verificador de palavras: sublinhado e cartão no botão direito (só na caixa de mensagem)

Palavras que **não existem** em português ganham um sublinhado vermelho ondulado, como num corretor do navegador, mas feito pelo ChatHM (igual em qualquer navegador).
**Botão direito** em cima de uma palavra sublinhada abre um cartão com até 5 sugestões e "Ignorar esta palavra"; clicar numa sugestão troca a palavra e devolve o foco à caixa.
Em qualquer outro lugar do campo, o botão direito abre o menu normal do navegador.

- **O que "existe"**: o dicionário VERO inteiro (todas as formas, com e sem acento, ~2,7 milhões) num filtro de Bloom (`ptValid.bin`, **3,3 MB**, carregado sob demanda na
  primeira vez que a pessoa digita e guardado pelo navegador por um ano), mais as palavras frequentes de conversa. Gerador: `scripts/gen-pt-bloom.py` (o teste confere
  bit a bit que TypeScript e Python concordam). O filtro tem ~1% de falso positivo: ~1% das palavras inexistentes passam por existentes (nunca o contrário).
- **Não sublinha**: a palavra em que o cursor está (ainda está sendo digitada), nomes (inicial maiúscula no meio da frase), siglas (TUDO MAIÚSCULO), palavras de 1 ou 2 letras,
  e-mail, link, `@menção`, números, risadas e termos do escritório (`cnpj`, `darf`, `esocial`, `whatsapp`...), e as palavras que a pessoa mandou ignorar.
- **Sugestões**: primeiro o que o corretor faria sozinho (acento ou erro de digitação), depois as palavras frequentes a 1 engano e, se faltar, a 2 enganos, da mais comum
  para a menos comum, mantendo a capitalização. Palavras raras que não estão entre as ~30 mil frequentes não são sugeridas (e podem não ter sugestão).
- O corretor do navegador fica desligado nesta caixa (`spellcheck=false`) para o sublinhado não aparecer duplicado.
- **Ignorar esta palavra** usa o mesmo aprendizado do Backspace (vale 90 dias neste navegador).

## Desempenho (a tela não pode travar)

Medido num navegador de teste com a CPU reduzida em 4x e 6x (um computador bem mais fraco que o normal): digitar num texto longo, passar o mouse por cima de palavras sublinhadas e
a primeira digitação do dia. Hoje: sem tarefa longa (>50 ms) na digitação nem ao passar o mouse; cada tecla pinta em ~30 ms mesmo a 6x.
O que foi ajustado:
- **Sugestões** (botão direito): antes gerava todas as combinações a 2 enganos e passava de 300 ms em palavra longa; agora é uma única varredura pela lista de palavras
  com distância de edição que desiste cedo (milissegundos, qualquer tamanho de palavra). Teste de orçamento no `spell.test.ts`.
- **Primeira vez**: os dicionários carregam quando o navegador está ocioso (em até 2,5 s), um de cada vez, e o vocabulário é montado em fatias de 4 mil palavras
  (`warmup.ts`). Palavra só com ASCII (a maioria) não passa pela normalização de acentos.
- **Sublinhado**: desenhado como imagem repetida (custo de pintura constante e o mesmo desenho em qualquer navegador) numa camada com pintura isolada
  (`contain`), em vez de `text-decoration: wavy`.
- Passar o mouse por cima do texto não dispara nenhuma renderização nem layout do corretor (medido: 0 layouts e 0 recálculos de estilo em 328 movimentos).

## Medição (sem tela)

Para melhorar a lista com uso real, o corretor **conta**, por camada e por par "palavra digitada → palavra corrigida", quantas vezes a correção foi aplicada e quantas
a pessoa desfez com Backspace. Camadas: `curated`, `extended`, `suffix`, `typo_tr` (letras trocadas), `typo_ins` (faltou letra), `typo_dup` (letra repetida), `typo_key` (tecla vizinha).

**O que NÃO é guardado:** texto da mensagem, frase em volta, conversa, cliente, quem digitou. Só palavras soltas (letras, até 40) e contadores. O navegador acumula e
envia em lotes (de 5 em 5 minutos e quando a aba some) para o próprio servidor (`POST /api/v1/writing/stats`); o servidor recusa o que não tem esse formato
(campo a mais, frase, número, camada desconhecida) e a tabela `writing_stats` (migração 45) não tem coluna para guardar outra coisa.

**Como ler** (administrador; não há tela): `GET /api/v1/writing/stats?min=5&limit=100` devolve
- `layers`: por camada, aplicadas, desfeitas, taxa de desfeitas e quantas palavras;
- `suspicious`: as palavras mais desfeitas com pelo menos `min` aplicações (candidatas a sair da lista, ou a entrar nas exceções/termos do escritório);
- `popular`: as correções mais usadas que ninguém desfez (o que a lista acerta).

Ou direto no banco:
```sql
select layer, from_word, to_word, applied, undone, round(undone::numeric / applied, 2) as taxa
  from writing_stats where applied >= 5 order by undone::float / applied desc limit 50;
```
**O que fazer com o resultado:** palavra com taxa alta de desfeitas = a lista erra (ou é um termo do escritório): tirar da lista curada/exceções ou acrescentar em `DOMAIN` (`typos.ts`);
camada inteira com taxa alta = apertar o critério daquela camada. Depois regerar os dados (`scripts/gen-pt-*.py`) e conferir os testes.

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
- O **sublinhado e o cartão** só existem na caixa de mensagem; legenda de anexo, lembrete e observação do cliente seguem com o corretor do navegador (e o mesmo corretor de acentos).
- O dicionário de palavras pesa 3,3 MB na primeira vez (depois fica no navegador); sem ele carregado, nada é sublinhado.

## Próximos passos (ordem sugerida)

1. ~~Erros de digitação com cuidado~~ **(feito: primeira versão, ver acima)**. Falta: sugerir (sem trocar sozinho) nos casos ambíguos e em letras erradas quaisquer; medir com texto real.
2. **Maiúscula no começo da frase** e depois de ponto ("oi. tudo bem" → "Oi. Tudo bem"), opcional.
3. **Ctrl+Z** que desfaz a correção (inserir pelo mecanismo nativo de edição do navegador em vez de trocar o valor).
4. **Sublinhado nos outros campos** (legenda, lembrete, observação) e **vocabulário do escritório**: palavras e nomes aprendidos (e os nomes dos clientes e da equipe, que o app já conhece) compartilhados sem tela: o corretor passa a não mexer neles automaticamente, e o aprendizado vai para a conta em vez do navegador.
5. ~~Medir~~ **(feito, ver "Medição")**. Falta: usar os números (rodar uma semana com a equipe, ler o relatório e podar a lista) e, se quiser, um aviso ao administrador quando uma camada passar de uma taxa de desfeitas.
6. **Celular/IME**: validar com teclados Android/iOS (composição) em aparelhos reais; hoje o corretor não age durante composição.
7. ~~Aviso na primeira vez~~ (retirado: o corretor é interno e discreto).
8. **Renovar o dicionário** quando sair versão nova do VERO ou da lista de frequência (rodar `scripts/gen-pt-accents.py`, conferir a diferença no `git diff`).

## Testes

`spell.test.ts` (filtro de Bloom, palavras sublinhadas, sugestões), `typos.test.ts` (algoritmo, concordância com o gerador em ~1.200 casos, capitalização, contexto), `autocorrect.test.ts` (palavras, dicionário gerado, opções), `accentController.test.ts` (motor: cursor, Backspace, composição, envio),
`writingPrefs.test.ts` (aprendizado e expiração), `autocorrectStats.test.ts` (contagem, lotes, falhas, privacidade) e, no servidor, `writing_test.go` (soma, relatório, validação, privacidade). No navegador de teste: digitação real, cursor no meio do texto, Backspace, aprendizado, Configurações.
