# Como publicar o ChatHM (roteiro para o Claude)

> Roteiro complementar: veja também [`deploy.md`](deploy.md).

Este roteiro é para **agentes** (Claude) que recebem do dono do sistema o pedido "pode publicar". O texto geral de infraestrutura está em
[`deploy.md`](deploy.md) (seção "Hostinger"); aqui está o passo a passo exato, com as ferramentas, os comandos e as armadilhas já encontradas.
Fale com o dono **em português**.

## Onde as coisas estão

| O quê | Valor |
|---|---|
| Site em produção | `https://chathm.com.br` |
| Repositório | `https://github.com/Vyctinho77/ChatHM` (público: a VPS baixa o código direto do GitHub) |
| VPS (Hostinger) | `virtualMachineId` **2134702** |
| Projetos Docker **permanentes** na VPS | `chathm` (app + Caddy) e `builder`. **Nunca mexa** neles, a não ser pelo script de troca |
| Projetos Docker **temporários** (você cria e apaga) | `chathm-release-build` e `chathm-release` |
| Banco (Supabase) | projeto `xdiwmwsmmkioexugyeaj` (somente leitura pelo MCP, de propósito) |
| Imagem | `chathm:latest` (em uso), `chathm:candidate` (a que você constrói), `chathm:previous` (a anterior, para voltar) |

Ferramentas (carregue com `ToolSearch`, ex.: `select:mcp__Hostinger_Connector__execute,mcp__Hostinger_Connector__search,mcp__Supabase__execute_sql,Monitor`):
- **Hostinger MCP**: `execute` com as operações `vps_docker_list`, `vps_docker_create`, `vps_docker_logs`, `vps_docker_delete` (e `search` para achar operações).
  Se disser que precisa de autorização, avise o dono (só ele autoriza, nas configurações de conectores) e **pare**: não há outro caminho.
- **Supabase MCP**: `execute_sql` com o `project_id` acima, **só para consultas** (`select`).
- **GitHub**: use `git` (push) e as ferramentas `mcp__github__*`; só publique o que está no GitHub.

## Regras que não se negociam

1. **Só publique quando o dono disser, nessa conversa, "pode publicar"** (ou equivalente claro). A aprovação vale **para aquele commit**, não para o que vier depois.
2. **Publique exatamente o commit testado**, já enviado ao GitHub (`git push`), pelo **hash completo** de 40 caracteres.
3. **Nunca** recrie o projeto `chathm`, nem mexa em `builder`, no Caddy ou nas variáveis de ambiente. A troca é só pelo `deploy/hostinger-release.yml`.
4. **Nunca peça, mostre nem grave segredos** (senhas, `ENCRYPTION_KEY`, `DATABASE_URL`, tokens). Se um segredo aparecer numa conversa, diga ao dono para **trocá-lo**.
5. **Não crie PR nem faça merge** sem pedido. Desenvolva e envie na branch que a sessão indicar.
6. **O banco de produção é somente leitura para você.** Limpezas de dados são feitas por ferramentas do próprio app (com prévia e confirmação digitada), nunca por SQL seu.
7. Se algo não bater, **pare e pergunte**. Não force, não contorne.
8. Use `Monitor` (laço com `sleep` curto) para esperar; **não** use `sleep` longo em primeiro plano, e pare os monitores no fim.

## Antes de começar (decida com calma)

1. **O commit contém o que já está em produção?** Descubra o estado atual: o arquivo `assets/index-*.js` da página (`curl -s https://chathm.com.br/ | grep -o 'assets/index-[^"]*\.js'`)
   e a versão do banco (abaixo). Publicar um commit que **não contém** o que está no ar **remove** o que só existe lá. Se houver dúvida, `git log` e compare.
   (Pode-se publicar um commit **antigo** da branch, por exemplo para soltar só parte do trabalho: o commit precisa estar no GitHub; o resto fica para depois.)
2. **Migrações**: os arquivos em `backend/migrations/` rodam sozinhos ao subir o app (`AUTO_MIGRATE`). O número novo precisa vir **depois** do último aplicado:
   `select max(version_id) from goose_db_version where is_applied` (via Supabase MCP). Dois arquivos com o mesmo número fazem o banco ignorar o novo.
   Migração que só **acrescenta** (tabela/coluna nova) é segura de desfazer pelo código; migração que **apaga ou muda** dados não: avise o dono antes.
3. **Testes no commit exato**:
   - Backend: `cd backend && gofmt -l internal && go vet ./... && TEST_DATABASE_URL='postgres://postgres:test@localhost:5432/chathm_t?sslmode=disable' go test -race ./...`
     (o Postgres de teste local: `pg_ctlcluster 16 main start`; se o banco `chathm_t` não existir, `createdb chathm_t` e senha `test` para o usuário `postgres`).
   - Web: `cd apps/web && pnpm exec tsc --noEmit -p . && pnpm exec eslint src && pnpm exec vitest run`.
   - Script do listener (se mexeu em `scripts/`): `node --test scripts/chathm-listen.test.mjs`.
4. Mudou algo visível? Teste no navegador (Playwright já vem instalado; use um servidor `vite` local com a API simulada) e confira o resultado **antes** de pedir a publicação.

## O roteiro (passo a passo)

### Passo 0: conferir o estado (antes de construir)
- `curl -s -o /dev/null -w "%{http_code}" https://chathm.com.br/readyz` → **200**.
- Supabase: `select max(version_id) from goose_db_version where is_applied` → anote (esperado: a versão da última publicação).
- `git fetch` e confirme que o `HEAD` local **é** o topo da branch no GitHub (`git rev-parse HEAD origin/<branch>`), ou que o commit a publicar está no GitHub.
- `vps_docker_list` (`{"virtualMachineId": 2134702}`) → só `builder` e `chathm` (se sobrou `chathm-release*` de uma tentativa anterior, entenda por quê e apague).

### Passo 1: construir a candidata (não toca na produção)
`vps_docker_create` com `project_name: "chathm-release-build"` e `content` = o YAML de `deploy/hostinger-build.yml` (leia o arquivo do repositório; **não** copie de memória),
trocando o texto depois do `#` pelo **hash completo**:

```yaml
name: chathm-release-build

services:
  build:
    image: docker:27-cli
    restart: "no"
    volumes: ["/var/run/docker.sock:/var/run/docker.sock"]
    command:
      - sh
      - -c
      - "df -h / | tail -1 && docker build --progress=plain -t chathm:candidate 'https://github.com/Vyctinho77/ChatHM.git#<HASH_COMPLETO>' && echo BUILD_OK || echo BUILD_FAILED"
```

Espere o fim com `vps_docker_logs` (`projectName: "chathm-release-build"`): o **fim dos logs** tem `BUILD_OK` (ou `BUILD_FAILED`, e aí leia o erro, corrija e recomece).
Demora ~20 s com o cache quente e ~2 min a frio. Atenção:
- `Project deployed successfully` no começo **só quer dizer que o contêiner subiu**, não que a imagem terminou.
- A linha `fatal: Not a valid object name <hash>^{commit}` **é normal**: o Docker tenta o commit local, não acha e em seguida busca no GitHub (`From https://github.com/... -> FETCH_HEAD`). Se depois dela vier `DONE`, está certo.
- O log é grande (um dump inteiro). Leia só quando for conferir o resultado.

### Passo 2: trocar a imagem em produção
`vps_docker_create` com `project_name: "chathm-release"` e `content` = o YAML de `deploy/hostinger-release.yml` **inteiro, como está no arquivo** (os `$$` são a forma de escrever `$` dentro de um compose; mantenha).
O script: valida o compose do `chathm`, guarda a imagem atual como `chathm:previous`, promove `chathm:candidate` a `chathm:latest`, recria **só o contêiner do app**
(o Caddy não reinicia), espera ficar `healthy` (confere a cada 2 s, até 4 min) e, se não ficar, **volta sozinho** para a anterior.

Sucesso = `DEPLOY_OK` no fim dos logs de `chathm-release` (`health=healthy`, ~4 s depois de subir; a troca inteira leva ~15 s). Falha = `UNHEALTHY_ROLLING_BACK` e `ROLLED_BACK`
(a produção continua na versão anterior; investigue os logs do app antes de tentar de novo) ou `ABORT_*` (nada foi trocado).
Há alguns segundos de indisponibilidade enquanto o app reinicia.

### Passo 3: verificar (não declare sucesso só pelo `DEPLOY_OK`)
- `readyz` → 200.
- O `assets/index-*.js` da página **mudou** (se a mudança tocou o front-end; mudança só de backend mantém o mesmo arquivo).
- Rotas protegidas respondem **401** sem login (confirma que o app novo subiu e a segurança está de pé): `GET /api/v1/ai/webhook`, `GET /mcp/replies`, e as rotas novas desta publicação.
  (Um `POST` sem corpo em rota `GET` dá 405: não é erro.)
- Banco: `select max(version_id) from goose_db_version where is_applied` → a versão nova, se houve migração; confira também a tabela/coluna nova (`information_schema`).
- Arquivos novos servidos (ex.: dados do corretor em `/assets/...`): `curl -sI` → 200.
- Se tocou em configuração viva (raias da IA etc.), **releia o estado do banco**: o dono pode ter mudado a configuração pela tela entre uma publicação e outra
  (já aconteceu: as raias da IA foram ligadas por ele). Não assuma que ficou como você deixou; **relate o que encontrou**, sem desfazer.

### Passo 4: limpar
`vps_docker_delete` para **`chathm-release-build`** e depois para **`chathm-release`** (`{"virtualMachineId": 2134702, "projectName": "..."}`).
A segunda exclusão responde `state: "delayed"` (fica na fila): rode `vps_docker_list` de novo até só restarem `builder` e `chathm`
(o `chathm-release` pode aparecer como `exited` por cerca de 1 minuto). Os projetos temporários não guardam estado.

### Passo 5: relatar (português, curto, verificável)
Diga: o commit publicado; `BUILD_OK`/`DEPLOY_OK`; as verificações do passo 3 com o que **você realmente conferiu** (e o que **não** conferiu: por exemplo "não abri o app logado em produção");
se houve migração; o estado de coisas que o dono mexe pela tela; e o que ficou pendente com ele. Não afirme o que não verificou.

## Voltar atrás (rollback)
- **Automático**: o script de troca volta sozinho se o app não ficar saudável (`ROLLED_BACK`).
- **Manual** (o app subiu mas o comportamento está errado): crie de novo o projeto `chathm-release` com o `deploy/hostinger-release.yml`, trocando `chathm:candidate` por **`chathm:previous`**
  na linha `docker tag ... chathm:latest` (o comentário no arquivo explica). Vale **uma vez**: a cada publicação `chathm:previous` é sobrescrita. Depois apague o projeto.
- **O banco não volta junto.** As migrações não são desfeitas. Voltar o código é seguro quando a migração só acrescentou coisas; senão, converse com o dono antes.
- Para voltar a um commit **antigo** (e não ao imediatamente anterior), faça o fluxo normal (passos 1 a 4) com o hash desse commit.

## Armadilhas já vistas
- **Branch de produção ≠ branch de trabalho.** A produção é "o que foi publicado por último", não um nome de branch. Confira o `assets/index-*.js` e a versão do banco.
- **Dois "mundos" de código.** Quando uma linha de trabalho foi separada em outra branch (ex.: um recurso em desenvolvimento), junte com `git revert` do commit que retirou + `git merge`, rode **tudo** de novo e só então publique.
- **O dono usa o sistema entre uma publicação e outra** (liga raias, usa a medição). Dados novos nas tabelas são esperados; não os apague.
- **Arquivos grandes no repositório** (o dicionário `ptValid.bin` tem 3,3 MB) entram na imagem pelo `vite build` e são servidos de `/assets/` (cache de 1 ano): confira que respondem 200 depois de publicar.
- **A VPS tem 1 núcleo.** Não rode duas construções ao mesmo tempo. O cache do Docker ocupa espaço (`docker builder prune` limpa; a próxima construção volta a ~2 min).
- **Esperas**: consulte o estado a cada 15 a 40 s com `Monitor` em vez de dormir um tempo fixo; leia os logs só quando o monitor avisar.

## Atribuição dos commits
Siga as instruções da sessão (cada sessão dá as linhas `Co-Authored-By`/`Claude-Session` que entram no fim da mensagem do commit). Não coloque nome de modelo em arquivos do repositório.
