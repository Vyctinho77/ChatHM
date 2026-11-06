import './dev-env.mjs';
import concurrently from 'concurrently';
import { devDatabase } from './dev-db.mjs';

await devDatabase();

const { result } = concurrently(
  [
    { command: 'go run ./cmd/api', cwd: 'backend', name: 'api', prefixColor: 'cyan' },
    { command: 'npm --prefix apps/web run dev', name: 'web', prefixColor: 'magenta' },
  ],
  { killOthersOn: ['success', 'failure'] },
);

try {
  await result;
} catch {
  process.exitCode = 1;
}
