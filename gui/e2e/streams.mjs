// End-to-end check that leaving a view closes its event streams
// (ORANGE-010). A browser opens at most six connections to poo-gui; a
// view that left one stream open per visit stalled the whole GUI after a
// few visits to the Node view while a node ran. It also checks that
// focus moving through key fields asks poo crypto once (ORANGE-011), and
// that a long paste reaches the command whole (ORANGE-013).
//
//   node gui/e2e/streams.mjs PATH/TO/poo-gui
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { launch, nodeStopped } from './lib.mjs';

const bin = process.argv[2];
const { env, page, errors, close } = await launch(bin);
const open = new Set();
let cryptoRuns = 0;
page.on('request', r => {
  if (r.url().includes('/events')) open.add(r);
  if (r.url().endsWith('/api/run') && (r.postData() || '').includes('"crypto"')) cryptoRuns++;
});
for (const ev of ['requestfinished', 'requestfailed']) page.on(ev, r => open.delete(r));

try {
  // Focus in and out of a key field eight times before poo answers.
  await page.click('nav a[data-id=wallet]');
  await page.locator('input[list=crypto-algs]').first().waitFor();
  await page.evaluate(() => {
    const key = document.querySelector('input[list=crypto-algs]'), nav = document.querySelector('nav a');
    for (let i = 0; i < 8; i++) { key.focus(); nav.focus(); }
  });
  await page.waitForTimeout(1000);
  assert.equal(cryptoRuns, 1, `poo crypto ran ${cryptoRuns} times for eight focuses`);

  // A pasted entry longer than one request (1 MiB) reaches the command
  // whole: sent in parts, then the input ended (ORANGE-013).
  await page.click('nav a[data-id=pools]');
  const submit = page.locator('section.panel', { has: page.locator('h2', { hasText: 'Submit an entry or layer' }) });
  await submit.locator('input[name=file]').fill('-');
  await submit.locator('input[name=pool]').fill('0');
  page.once('dialog', d => d.accept());
  await submit.locator('button[type=submit]').first().click();
  const ask = submit.locator('form.ask input').first();
  await ask.waitFor({ state: 'visible' });
  await ask.evaluate(i => { i.value = 'ab'.repeat(768 << 10); });
  await submit.locator('button.end-input').click();
  await submit.locator('.run-status.ok, .run-status.bad').waitFor();
  const said = await submit.locator('.area').innerText();
  assert.ok(!said.includes('request body too large'), `a 1.5 MiB paste did not arrive whole: ${said.slice(0, 200)}`);

  execFileSync(bin, ['poo', 'init', '-example', '-listen', '127.0.0.1:0', '-p2p', 'off'], { env });
  await page.click('nav a[data-id=node]');
  await page.click('button:has-text("Start node")');
  await page.locator('.live-out', { hasText: 'rpc: listening' }).waitFor();

  // Ten visits to the Node view, each long enough for its log stream to
  // open, with a view that runs commands in between.
  for (let i = 0; i < 10; i++) {
    await page.click('nav a[data-id=start]');
    await page.waitForTimeout(150);
    await page.click('nav a[data-id=node]');
    await page.locator('.live-out', { hasText: 'rpc: listening' }).waitFor({ timeout: 5000 })
      .catch(() => { throw new Error(`visit ${i + 1}: the node log did not show within 5 s; ${open.size} event streams open`); });
  }
  await page.waitForTimeout(500);
  assert.ok(open.size <= 3, `${open.size} event streams still open after ten visits`);

  // The GUI still answers at once.
  const ms = await page.evaluate(async () => {
    const t = performance.now();
    const ctl = new AbortController();
    setTimeout(() => ctl.abort(), 5000);
    await fetch('/api/jobs', { signal: ctl.signal });
    return performance.now() - t;
  });
  assert.ok(ms < 2000, `the jobs list took ${ms} ms`);

  await page.click('button:has-text("Stop node")');
  await nodeStopped(page);
  assert.deepEqual(errors, [], 'no page errors');
  console.log('gui streams e2e: ok');
} finally {
  await close();
}
