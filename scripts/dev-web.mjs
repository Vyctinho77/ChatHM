import './dev-env.mjs';
import { spawn } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { waitForApi } from './dev-ready.mjs';

try {
  console.log('[dev] Aguardando a API e o banco ficarem prontos…');
  await waitForApi(process.env.VITE_API_PROXY);
  const vite = fileURLToPath(new URL('../apps/web/node_modules/vite/bin/vite.js', import.meta.url));
  const web = fileURLToPath(new URL('../apps/web', import.meta.url));
  const child = spawn(process.execPath, [vite, web], { env: process.env, stdio: 'inherit', windowsHide: true });
  const interrupt = () => child.kill('SIGINT');
  const terminate = () => child.kill('SIGTERM');
  process.on('SIGINT', interrupt);
  process.on('SIGTERM', terminate);
  try {
    process.exitCode = await new Promise((resolve, reject) => {
      child.once('error', reject);
      child.once('exit', code => resolve(code ?? 1));
    });
  } finally {
    process.off('SIGINT', interrupt);
    process.off('SIGTERM', terminate);
  }
} catch (error) {
  console.error(error.message);
  process.exitCode = 1;
}
