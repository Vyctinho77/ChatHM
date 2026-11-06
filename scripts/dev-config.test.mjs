import test from 'node:test';
import assert from 'node:assert/strict';
import { assertLocalDatabaseUrl } from './dev-config.mjs';
import { spawnSync } from 'node:child_process';

test('dev accepts only local PostgreSQL development/test databases', () => {
  for (const host of ['127.0.0.1', 'localhost', '[::1]']) {
    assert.doesNotThrow(() => assertLocalDatabaseUrl(`postgres://dev:local@${host}:5433/chathm_dev`));
  }
  for (const value of [
    'postgres://user:secret@db.production.supabase.co:5432/postgres',
    'postgres://user:secret@localhost:5433/postgres',
    'postgres://user:secret@localhost.example.com:5433/chathm_dev',
    'https://localhost/chathm_dev', '',
  ]) {
    assert.throws(() => assertLocalDatabaseUrl(value), { message: /\[dev\]/ });
  }
});

test('dev replaces inherited production credentials before starting any service', () => {
  const result = spawnSync(process.execPath, ['--input-type=module', '-e', `
    await import('./scripts/dev-env.mjs');
    console.log(JSON.stringify({host:new URL(process.env.DATABASE_URL).hostname,provider:process.env.WHATSAPP_PROVIDER,env:process.env.APP_ENV,storage:process.env.STORAGE_DRIVER,keyIsProduction:process.env.ENCRYPTION_KEY==='production-placeholder'}));
  `], {
    cwd: new URL('../', import.meta.url),
    env: {...process.env, DATABASE_URL:'postgres://user:placeholder@production.invalid/postgres', WHATSAPP_PROVIDER:'whatsmeow', APP_ENV:'production', STORAGE_DRIVER:'s3', ENCRYPTION_KEY:'production-placeholder'},
    encoding: 'utf8',
  });
  assert.equal(result.status, 0, result.stderr);
  const actual = JSON.parse(result.stdout.trim().split('\n').at(-1));
  assert.equal(actual.host, '127.0.0.1');
  assert.equal(actual.provider, 'fake');
  assert.equal(actual.env, 'development');
  assert.equal(actual.storage, 'fs');
  assert.equal(actual.keyIsProduction, false);
});
