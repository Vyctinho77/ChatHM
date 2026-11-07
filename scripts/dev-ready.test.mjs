import test from 'node:test';
import assert from 'node:assert/strict';
import { createServer } from 'node:http';
import { waitForApi } from './dev-ready.mjs';

test('frontend waits through unavailable and non-ready responses until API readiness', async t => {
  let calls = 0;
  const server = createServer((req, res) => {
    assert.equal(req.url, '/readyz');
    calls++;
    res.writeHead(calls === 1 ? 503 : 200, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify({ status: calls < 3 ? 'unavailable' : 'ready' }));
  });
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  t.after(() => { server.closeAllConnections(); server.close(); });
  await waitForApi(`http://127.0.0.1:${server.address().port}`, { intervalMs: 5, timeoutMs: 2000 });
  assert.equal(calls, 3);
});

test('readiness timeout does not start the frontend when the API is unavailable', async t => {
  const server = createServer((_req, res) => { res.writeHead(503); res.end(); });
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  t.after(() => { server.closeAllConnections(); server.close(); });
  await assert.rejects(waitForApi(`http://127.0.0.1:${server.address().port}`, { intervalMs: 5, timeoutMs: 70 }), /API não ficou pronta/);
});

test('readiness wait can be cancelled', async () => {
  const controller = new AbortController();
  controller.abort();
  await assert.rejects(waitForApi('http://127.0.0.1:1', { signal: controller.signal }), { name: 'AbortError' });
});
