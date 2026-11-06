import './dev-env.mjs';
import { spawn, spawnSync } from 'node:child_process';
import { createConnection } from 'node:net';
import { fileURLToPath } from 'node:url';
import { existsSync } from 'node:fs';
import { assertLocalDatabaseUrl } from './dev-config.mjs';

const root = fileURLToPath(new URL('../', import.meta.url));
const database = assertLocalDatabaseUrl(process.env.DATABASE_URL);

function listening() {
  return new Promise((resolve) => {
    const socket = createConnection({ host: database.hostname.replace(/[\[\]]/g, ''), port: Number(database.port || 5432) });
    socket.setTimeout(500);
    socket.once('connect', () => { socket.destroy(); resolve(true); });
    socket.once('error', () => { socket.destroy(); resolve(false); });
    socket.once('timeout', () => { socket.destroy(); resolve(false); });
  });
}

function run(command, args, env = process.env) {
  return new Promise((resolve, reject) => {
    const child = spawn(command, args, { cwd: root, env, stdio: 'inherit', windowsHide: true });
    child.once('error', reject);
    child.once('exit', (code) => code === 0 ? resolve() : reject(new Error(`[dev] Banco local: comando terminou com código ${code}.`)));
  });
}

export async function devDatabase(action = 'start') {
  const nativeCluster = existsSync(new URL('../.local/pgdata-17/PG_VERSION', import.meta.url));
  if (action === 'start' && await listening() && !nativeCluster) return;
  const docker = spawnSync('docker', ['--version'], { windowsHide: true, stdio: 'ignore' });
  if (docker.status === 0) {
    if (database.port !== '5433' || database.pathname !== '/chathm_dev') throw new Error('[dev] O compose de desenvolvimento usa a porta 5433 e o banco chathm_dev.');
    await run('docker', ['compose', '-f', 'docker-compose.dev.yml', ...(action === 'stop' ? ['stop', 'postgres'] : ['up', '-d', '--wait', 'postgres'])]);
  } else if (process.platform === 'win32') {
    await run('powershell.exe', ['-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', 'scripts/dev-db.ps1', '-Action', action], {
      ...process.env,
      DEV_PG_PORT: database.port || '5432',
      DEV_PG_USER: decodeURIComponent(database.username),
      DEV_PG_PASSWORD: decodeURIComponent(database.password),
      DEV_PG_DATABASE: database.pathname.slice(1),
    });
  } else {
    throw new Error('[dev] Instale Docker e execute npm run db:up, ou prepare um PostgreSQL local conforme .env.development.local.');
  }
}

if (process.argv[1] && fileURLToPath(import.meta.url) === process.argv[1]) {
  await devDatabase(process.argv[2] || 'start');
}
