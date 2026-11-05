import { randomBytes } from 'node:crypto';
import { existsSync, readFileSync, writeFileSync } from 'node:fs';
import { loadEnvFile } from 'node:process';

const envPath = new URL('../.env', import.meta.url);

if (!existsSync(envPath)) {
  const example = readFileSync(new URL('../.env.example', import.meta.url), 'utf8');
  const key = randomBytes(32).toString('base64');
  writeFileSync(envPath, `${example.trimEnd()}\nENCRYPTION_KEY=${key}\n`, { flag: 'wx' });
  console.log('[dev] .env criado a partir de .env.example com chave de criptografia aleatória.');
}

loadEnvFile(envPath);
