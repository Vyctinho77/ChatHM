# Publicar na internet (um servidor, HTTPS automático)

Uma imagem só (`Dockerfile` na raiz): a API em Go serve também o frontend compilado, na mesma origem — sem CORS e com
uma CSP restrita (scripts só do próprio site; o script inline do `index.html` entra por hash). O `deploy/docker-compose.yml`
sobe **Postgres + app + Caddy** (certificado HTTPS automático via Let's Encrypt).

## Requisitos
- Um servidor Linux pequeno (1 vCPU / 1–2 GB servem para um escritório) com Docker e Compose, portas **80 e 443** abertas.
- Um domínio com registro **A** apontando para o IP do servidor (ex.: `atendimento.seuescritorio.com.br`). HTTPS é obrigatório:
  em produção a API recusa iniciar sem cookie seguro e sem MFA de administrador.

## Passo a passo
```bash
git clone https://github.com/Vyctinho77/ChatHM.git && cd ChatHM
cp deploy/.env.example deploy/.env      # preencha DOMAIN, POSTGRES_PASSWORD, ENCRYPTION_KEY e SETUP_TOKEN
docker compose -f deploy/docker-compose.yml --env-file deploy/.env up -d --build
```
Abra `https://SEU_DOMINIO`: aparece o QR Code (peça o `SETUP_TOKEN` se configurado). Escaneie com o **WhatsApp do escritório**,
informe seus dados e configure o autenticador. Depois, em **Equipe → Acesso da equipe**, copie o token e envie à equipe.

## Segurança — leia antes de usar com o número real
- **Faça a instalação logo após subir o servidor** e mantenha o `SETUP_TOKEN`: até existir um administrador, quem abrir a página
  primeiro conecta o próprio WhatsApp.
- **Guarde o `ENCRYPTION_KEY` fora do servidor** (gerenciador de senhas). Sem ela, a sessão do WhatsApp e os autenticadores não abrem mais.
- **Backups**: volumes `pgdata` (banco, inclui a sessão do WhatsApp cifrada) e `attachments` (anexos cifrados). Exemplo de dump:
  `docker compose -f deploy/docker-compose.yml exec db pg_dump -U chathm chathm > backup.sql`.
- A integração por QR Code **não é oficial**: o WhatsApp pode restringir o número. Teste primeiro com um número secundário.
- Mantenha o sistema atualizado (`git pull` + `up -d --build`); as migrações rodam sozinhas ao subir.
- Antivírus de anexos é opcional (`CLAMAV_ADDR`); sem ele, os arquivos só passam pela validação de tipo e tamanho.

## Outros destinos
O mesmo `Dockerfile` roda em Fly.io, Railway ou Render: crie um Postgres, um volume persistente em `/data` (anexos) e informe as
variáveis do `deploy/docker-compose.yml` (`DATABASE_URL`, `ENCRYPTION_KEY`, `ALLOWED_ORIGINS=https://seu-dominio`, `COOKIE_SECURE=true`,
`REQUIRE_ADMIN_MFA=true`, `TRUST_PROXY_HEADERS=true`, `SETUP_TOKEN`). Plano gratuito sem disco persistente perde os anexos a cada reinício.

## Com o banco no Supabase
O Supabase entrega o **Postgres** (o app conecta direto, como usuário `postgres`); ele **não hospeda** o servidor Go, que precisa rodar
em um servidor/PaaS (um container). Use `deploy/docker-compose.supabase.yml` e a **Session pooler** (IPv4, porta 5432, usuário `postgres.<ref>`)
em `DATABASE_URL`.
- **Só uma instância por banco.** O banco guarda a sessão do WhatsApp: se o app rodar no servidor e também na sua máquina apontando para o
  mesmo banco, os dois disputam a sessão ("aberta em outro local") e a conexão cai. Pare o local antes de subir o servidor.
- **Ative o RLS** nas tabelas do app (sem políticas): o app não usa a API REST do Supabase, e assim a chave `anon` não lê nada.
  O usuário `postgres` do app ignora o RLS, então nada muda para ele:
  `ALTER TABLE public.<tabela> ENABLE ROW LEVEL SECURITY;` para cada tabela listada em Advisors → Security.
- As migrações rodam sozinhas ao subir (`AUTO_MIGRATE`); um banco que aplicou a agenda com a versão antiga (por data) é ajustado automaticamente.

## Com a Cloudflare (Tunnel)
`deploy/docker-compose.tunnel.yml` troca o Caddy por um **Cloudflare Tunnel**: a Cloudflare fornece o HTTPS e o domínio, e o servidor não
precisa abrir portas (serve até para um computador em casa, desde que fique ligado). Passos: crie o túnel (Zero Trust → Networks → Tunnels),
copie o token para `TUNNEL_TOKEN` e, em *Public hostname*, aponte seu domínio para `http://app:8080`. O servidor Go precisa estar sempre ligado
para manter o WhatsApp conectado — por isso não roda em Workers. **Cloudflare Containers** é possível, mas as instâncias podem dormir e reiniciar
(o disco é temporário), o que derruba a sessão do WhatsApp; use só se aceitar isso, com anexos no R2 (`STORAGE_DRIVER=s3`).

## Vender para outros escritórios (uma instância por cliente)
Cada cliente tem o **próprio servidor, banco e número de WhatsApp**, isolados. Para reaproveitar uma instância (ou preparar uma nova) use
**Configurações → Entregar instância** (só administrador, com o código do autenticador e digitando ENTREGAR):
1. desvincula o WhatsApp atual e remove a sessão;
2. apaga **todos** os dados (usuários, equipes, conversas, contatos, anexos, registros) — não dá para desfazer;
3. devolve a instalação ao início e mostra **uma única vez** um **código de instalação** (`XXXXX-XXXXX`, uso único).
Envie ao cliente o endereço e o código. Ele abre o endereço, digita o código, escaneia o QR com o WhatsApp do escritório dele e vira o
administrador. Enquanto o código não for usado, **só ele** libera a instalação (o `SETUP_TOKEN` do servidor deixa de valer). Para uma instância
nova, suba outro servidor com o mesmo `docker-compose` (outro domínio e outro banco/projeto Supabase) e use o `SETUP_TOKEN` na primeira instalação.

## Hostinger (VPS com Docker Manager)

O Docker Manager não constrói imagens a partir de uma URL do GitHub, então a atualização tem dois passos. O app usa a imagem local
`chathm:latest`; o banco (Supabase) e as variáveis (`DOMAIN`, `DATABASE_URL`, `ENCRYPTION_KEY`, `SETUP_TOKEN`) ficam no projeto `chathm`
(`deploy/docker-compose.hostinger.yml`, valores só no campo de ambiente, nunca no repositório) e **não mudam** numa atualização.

**Antes de publicar**
1. Confira **qual branch está em produção** e se o que você vai publicar **contém** esse branch (`git merge-base --is-ancestor <produção> <novo>`).
   Publicar um branch que não o contém **remove** da produção o que só existe lá.
2. As migrações rodam sozinhas ao subir (`AUTO_MIGRATE`). Os números precisam vir **depois** da última já aplicada no banco
   (`select max(version_id) from goose_db_version where is_applied`): dois arquivos com o mesmo número fazem o banco ignorar o novo.
3. Rode a suíte completa (backend com `-race` e web) no commit exato que será publicado.

**Passo 1, construir a candidata** (`deploy/hostinger-build.yml`, com o commit no lugar de `COLE-AQUI-O-COMMIT`). Não toca na produção.
Espere `BUILD_OK` nos logs (cerca de 2 min).

**Passo 2, trocar** (`deploy/hostinger-release.yml`). Valida a configuração, guarda a versão atual como `chathm:previous`, recria só o
container do app, espera ficar saudável e, se não ficar, volta sozinho para a anterior. Sucesso = `DEPLOY_OK`; falha = `ROLLED_BACK`.
Há alguns segundos de indisponibilidade enquanto o app reinicia.

**Depois**: confira `https://<domínio>/readyz` e o arquivo `assets/index-*.js` da página, e apague os dois projetos auxiliares (não guardam estado).
