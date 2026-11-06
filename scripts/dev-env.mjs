import { randomBytes } from 'node:crypto';
import { existsSync, readFileSync, writeFileSync } from 'node:fs';
import { parseEnv } from 'node:util';
import { fileURLToPath } from 'node:url';
import { assertLocalDatabaseUrl } from './dev-config.mjs';

const envPath = new URL('../.env.development.local', import.meta.url);

if (!existsSync(envPath)) {
  const example = readFileSync(new URL('../.env.development.example', import.meta.url), 'utf8');
  const key = randomBytes(32).toString('base64');
  writeFileSync(envPath, `${example.trimEnd()}\nENCRYPTION_KEY=${key}\n`, { flag: 'wx' });
  console.log('[dev] .env.development.local criado com banco local e chave própria.');
}

const local = parseEnv(readFileSync(envPath, 'utf8'));
assertLocalDatabaseUrl(local.DATABASE_URL);
assertLocalDatabaseUrl(local.TEST_DATABASE_URL);
if (!local.ENCRYPTION_KEY) throw new Error('[dev] Defina ENCRYPTION_KEY em .env.development.local.');
if (!/^(127\.0\.0\.1|localhost|\[::1\]):\d+$/.test(local.HTTP_ADDR ?? '')) {
  throw new Error('[dev] HTTP_ADDR deve escutar somente em localhost.');
}
// Deliberately override inherited production variables for the dev processes.
Object.assign(process.env, local, {
  APP_ENV: 'development',
  STORAGE_DRIVER: 'fs',
  STORAGE_DIR: fileURLToPath(new URL('../.local/attachments', import.meta.url)),
  WEB_DIR: '',
  SETUP_TOKEN: local.SETUP_TOKEN ?? '',
  CLAMAV_ADDR: local.CLAMAV_ADDR ?? '',
  VITE_API_PROXY: `http://${local.HTTP_ADDR}`,
});
console.log(`[dev] PostgreSQL local · WhatsApp ${local.WHATSAPP_PROVIDER === 'fake' ? 'simulado' : 'de teste'}`);
