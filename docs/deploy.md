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
