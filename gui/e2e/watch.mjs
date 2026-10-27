// End-to-end check of Watch and Explore: live events from poo watch with a
// filter, and the views, against a real node.
//
//   node gui/e2e/watch.mjs PATH/TO/poo-gui [SCREENSHOT_DIR]
import { execFileSync } from 'node:child_process';
import { join } from 'node:path';
import assert from 'node:assert/strict';
import { launch } from './lib.mjs';

const bin = process.argv[2];
const shots = process.argv[3];
const { home, env, page, errors, close } = await launch(bin);
const poo = (...args) => execFileSync(bin, ['poo', ...args], { env }).toString();
const section = title => page.locator('section.panel', { has: page.locator('h2', { hasText: title }) }).first();
const until = async (cond, what, ms = 20000) => {
  for (const end = Date.now() + ms; !(await cond());) {
    assert.ok(Date.now() < end, `timed out waiting for ${what}`);
    await page.waitForTimeout(50);
  }
};

try {
  poo('init', '-example', '-listen', '127.0.0.1:0', '-p2p', 'off');
  await page.click('nav a[data-id=node]');
  await page.click('button:has-text("Start node")');
  await page.locator('.live-out', { hasText: 'pools followed' }).waitFor();

  // Watch only transactions, then claim a mint from outside the GUI.
  await page.click('nav a[data-id=watch]');
  const live = section('Live events');
  await live.locator('label', { hasText: 'Kinds' }).locator('input').fill('tx, selected');
  assert.match(await live.locator('.cmd code').innerText(), /^poo watch -kind tx,selected$/);
  await live.locator('button:has-text("Start watching")').click();
  await live.locator('.run-status', { hasText: 'watching' }).waitFor();
  await page.waitForTimeout(300); // the stream is open before the claim
  const claim = JSON.parse(poo('claim', '-seed', 'a1'.repeat(32), '-serial', '5'));
  const tid = claim.tid;
  assert.ok(tid, 'claim printed its tid');
  await live.locator('tr.ev-tx').first().waitFor();
  poo('build', '-pool', '1', '-work', '4000'); // a layer gives the entry weight, so it is selected
  await live.locator('tr.ev-selected').first().waitFor();
  // The filter left out entries and layers.
  assert.equal(await live.locator('tr.ev-entry, tr.ev-layer').count(), 0);
  await live.locator('tr.ev-selected').first().click();
  await live.locator('.event-detail', { hasText: 'selected' }).waitFor();
  if (shots) await page.screenshot({ path: join(shots, 'watch.png'), fullPage: true });
  await live.locator('button:has-text("Stop")').click();
  await live.locator('.run-status', { hasText: 'stopped' }).waitFor();

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
  await page.locator('.run-status', { hasText: 'node stopped (exit 0)' }).waitFor({ timeout: 20000 });
  assert.deepEqual(errors, [], 'no page errors');
  console.log('gui watch e2e: ok');
} finally {
  await close();
}
