// End-to-end check of Watch and Explore: live events from poo watch with a
// filter, and the views, against a real node.
//
//   node gui/e2e/watch.mjs PATH/TO/poo-gui [SCREENSHOT_DIR]
import { execFileSync } from 'node:child_process';
import { join } from 'node:path';
import assert from 'node:assert/strict';
import { launch, nodeStopped } from './lib.mjs';

const bin = process.argv[2];
const shots = process.argv[3];
const { home, env, page, errors, close } = await launch(bin);
const poo = (args, extra = {}) => execFileSync(bin, ['poo', ...args], { env: { ...env, ...extra } }).toString();
const section = title => page.locator('section.panel', { has: page.locator('h2', { hasText: title }) }).first();
const until = async (cond, what, ms = 20000) => {
  for (const end = Date.now() + ms; !(await cond());) {
    assert.ok(Date.now() < end, `timed out waiting for ${what}`);
    await page.waitForTimeout(50);
  }
};

// Count the page's event streams, open and closed, and where each began.
await page.addInitScript(() => {
  const ES = window.EventSource;
  window.__streams = [];
  window.EventSource = class extends ES {
    constructor(url, opts) { super(url, opts); window.__streams.push(this); }
  };
});
await page.reload();
const watchStreams = () => page.evaluate(() => window.__streams
  .filter(s => /\/api\/jobs\/\d+\/events/.test(s.url))
  .map(s => ({ url: s.url, open: s.readyState !== EventSource.CLOSED })));

try {
  poo(['init', '-example', '-listen', '127.0.0.1:0', '-p2p', 'off']);
  await page.click('nav a[data-id=node]');
  await page.click('button:has-text("Start node")');
  await page.locator('.live-out', { hasText: 'pools followed' }).waitFor();

  // Watch only transactions, then claim a mint from outside the GUI.
  await page.click('nav a[data-id=watch]');
  const live = section('Live events');
  await live.locator('fieldset.kinds input[value=tx]').check();
  await live.locator('fieldset.kinds input[value=selected]').check();
  assert.match(await live.locator('.cmd code').innerText(), /^poo watch -kind tx,selected$/);
  await live.locator('button:has-text("Start watching")').click();
  await live.locator('.run-status', { hasText: 'watching' }).waitFor();
  await page.waitForTimeout(300); // the stream is open before the claim
  const claim = JSON.parse(poo(['claim', '-serial', '5'], { POO_SEED: 'a1'.repeat(32) }));
  const tid = claim.tid;
  assert.ok(tid, 'claim printed its tid');
  await live.locator('tr.ev-tx').first().waitFor();
  poo(['build', '-pool', '1', '-work', '4000']); // a layer gives the entry weight, so it is selected
  await live.locator('tr.ev-selected').first().waitFor();
  // The filter left out entries and layers.
  assert.equal(await live.locator('tr.ev-entry, tr.ev-layer').count(), 0);
  await live.locator('tr.ev-selected').first().click();
  await live.locator('.event-detail', { hasText: 'selected' }).waitFor();
  if (shots) await page.screenshot({ path: join(shots, 'watch.png'), fullPage: true });

  // GUI-008: leaving the view closes the watch's event stream while the
  // job runs on; an event that arrives meanwhile shows on return, once.
  const txRows = await live.locator('tr.ev-tx').count();
  const watchJob = await page.evaluate(async () => (await (await fetch('/api/jobs')).json()).find(j => !j.done && j.args[0] === 'watch').id);
  assert.ok((await watchStreams()).some(s => s.open && s.url.includes(`/jobs/${watchJob}/`)), 'the watch has a stream');
  await page.click('nav a[data-id=jobs]');
  await until(async () => !(await watchStreams()).some(s => s.open && s.url.includes(`/jobs/${watchJob}/`)), 'the stream closed while away');
  assert.equal(await page.locator('nav a[data-id=watch].busy').count(), 1, 'the watch still runs');
  poo(['claim', '-serial', '6'], { POO_SEED: 'b2'.repeat(32) });
  await page.waitForTimeout(500);
  await page.click('nav a[data-id=watch]');
  await until(async () => (await live.locator('tr.ev-tx').count()) === txRows + 1, 'the event from while away');
  const resumed = (await watchStreams()).filter(s => s.open && s.url.includes(`/jobs/${watchJob}/`));
  assert.equal(resumed.length, 1, 'one stream again');
  assert.ok(Number(new URL(resumed[0].url, 'http://x').searchParams.get('from')) > 0, 'it resumes from where it left off');
  await page.waitForTimeout(500);
  assert.equal(await live.locator('tr.ev-tx').count(), txRows + 1, 'no event shown twice');
  // The watch keeps running while another view is shown (GREEN-015).
  await page.click('nav a[data-id=pools]');
  assert.ok(await page.locator('nav a[data-id=watch].busy').count(), 'the Watch tab shows the running watch');
  await page.click('nav a[data-id=watch]');
  await live.locator('.run-status', { hasText: 'watching' }).waitFor();
  assert.ok(await live.locator('tr.ev-selected').count() > 0, 'the table survived');
  await live.locator('button:has-text("Stop")').click();
  await live.locator('.run-status', { hasText: 'stopped' }).waitFor();
  await until(async () => (await page.locator('nav a[data-id=watch].busy').count()) === 0, 'the dot to go');

  // The ledger page is also drawn: one bar per entry, the tip marked.
  await page.click('nav a[data-id=explore]');
  const led = section('Selected ledger');
  await led.locator('input[name=pool1]').fill('1');
  await led.locator('button[type=submit]').click();
  await led.locator('svg.ledger-pic .lp-tip').waitFor();
  assert.equal(await led.locator('svg.ledger-pic .lp-entry').count(), 1);
  assert.match(await led.locator('svg.ledger-pic').getAttribute('aria-label'), /pool 1: 1 entry,/);
  if (shots) await page.screenshot({ path: join(shots, 'ledger-picture.png'), fullPage: true });
  await page.click('nav a[data-id=watch]');

  // The tid in a row links to Explore, which runs poo tx on it.
  await live.locator(`tr.ev-tx a.lookup[href*="${tid}"]`).first().click();
  const looked = section('A transaction');
  await looked.locator('.run-status.ok').waitFor();
  assert.equal(await looked.locator('.cmd code').innerText(), `poo tx ${tid}`);
  // The Pools view's lookups run poo unspent and poo tx (Source GREEN-008).
  await page.click('nav a[data-id=pools]');
  const pay = section('Where a payment stands');
  await pay.locator('label', { hasText: 'Transaction id' }).locator('input').fill(tid);
  assert.equal(await pay.locator('.cmd code').innerText(), `poo tx ${tid}`);
  const hold = section('Holdings of a mint');
  await hold.locator('label', { hasText: 'Mint serial' }).locator('input').fill('5');
  assert.equal(await hold.locator('.cmd code').innerText(), 'poo unspent -serial 5');
  await hold.locator('button[type=submit]').click();
  await hold.locator('.run-status.ok').waitFor();
  // Its address links to Explore's address lookup.
  await hold.locator('a.lookup[href^="#/explore?address="]').first().click();
  await section('An address').locator('.run-status.ok').waitFor();

  // Explore: the ledger and the transaction, through the same filters.
  await page.click('nav a[data-id=explore]');
  const ledger = section('Selected ledger');
  await ledger.locator('label', { hasText: 'Pool' }).first().locator('input').fill('1');
  await ledger.locator('button[type=submit]').click();
  await ledger.locator('.run-status.ok').waitFor();
  const tx = section('A transaction');
  await tx.locator('label', { hasText: 'Transaction id' }).locator('input').fill(tid);
  assert.equal(await tx.locator('.cmd code').innerText(), `poo tx ${tid}`);
  await tx.locator('button[type=submit]').click();
  await tx.locator('.run-status.ok').waitFor();
  await until(async () => (await tx.locator('.result').innerText()).includes('selected'), 'the transaction rendered');
  if (shots) await page.screenshot({ path: join(shots, 'explore.png'), fullPage: true });

  await page.click('nav a[data-id=node]');
  await page.click('button:has-text("Stop node")');
  await nodeStopped(page);
  assert.deepEqual(errors, [], 'no page errors');
  console.log('gui watch e2e: ok');
} finally {
  await close();
}
