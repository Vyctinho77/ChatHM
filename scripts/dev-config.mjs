export function assertLocalDatabaseUrl(value, name = 'DATABASE_URL') {
  let url;
  try { url = new URL(value); } catch { throw new Error(`[dev] ${name} precisa de uma URI PostgreSQL local.`); }
  if (!['postgres:', 'postgresql:'].includes(url.protocol)
      || !['127.0.0.1', 'localhost', '[::1]'].includes(url.hostname)
      || !/_(dev|test)$/.test(url.pathname.slice(1))) {
    throw new Error(`[dev] ${name} deve apontar para localhost e um banco terminado em _dev ou _test. Bancos remotos não são usados pelo modo dev.`);
  }
  return url;
}
