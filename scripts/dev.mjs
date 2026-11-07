import './dev-env.mjs';
import concurrently from 'concurrently';
import { devDatabase } from './dev-db.mjs';

await devDatabase();

const { result } = concurrently(
  [
    { command: 'go run ./cmd/api', cwd: 'backend', name: 'api', prefixColor: 'cyan' },
    { command: 'node scripts/dev-web.mjs', name: 'web', prefixColor: 'magenta' },
  ],
  { killOthersOn: ['success', 'failure'] },
);

try {
  await result;
} catch {
  process.exitCode = 1;
}
