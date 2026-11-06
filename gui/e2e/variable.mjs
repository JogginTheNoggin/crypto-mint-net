// End-to-end check of variable supply series on a running node (D166):
// the Variable series panel, a claim on a part not released yet shown as
// waiting, Add a release (a picked file, sent as -), and the panel after.
// Usage: node gui/e2e/variable.mjs path/to/poon-gui
import { join } from 'node:path';
import { writeFileSync } from 'node:fs';
import { spawnSync } from 'node:child_process';
import assert from 'node:assert/strict';
import { launch, nodeStopped } from './lib.mjs';

const bin = process.argv[2];
if (!bin) { console.error('usage: node gui/e2e/variable.mjs POON_GUI_BINARY'); process.exit(2); }

const { home, env, page, errors, close } = await launch(bin);
const section = title => page.locator('section.panel', { has: page.locator('h2', { hasText: title }) }).first();
const poon = (...args) => {
  const r = spawnSync(bin, ['poon', ...args], { env });
  assert.equal(r.status, 0, `poon ${args.join(' ')}: ${r.stderr}`);
  return String(r.stdout);
};

try {
  // An issuer, a variable series of 8 mints in parts of 4, signed, and
  // its part 2 released: all made by poon, as an issuer would.
  const seed = join(home, 'issuer-seed');
  writeFileSync(seed, '5e'.repeat(32));
  const issuer = JSON.parse(poon('keygen', '-seed-file', seed)).address;
  const params = join(home, 'series.json');
  writeFileSync(params, poon('series', 'new', '-mints', '8', '-pool-size', '4', '-variable', '-part-size', '4', '-issuer', issuer));
  const signed = join(home, 'signed.json');
  writeFileSync(signed, poon('series', 'sign', params, '-seed-file', seed));
  const release = poon('series', 'release', params, '-part', '2', '-seed-file', seed);
  poon('init', '-series', signed, '-listen', '127.0.0.1:0', '-p2p', 'off');

  await page.click('nav a[data-id=node]');
  await page.click('button:has-text("Start node")');
  await page.locator('.live-out', { hasText: 'rpc: listening' }).waitFor();
  const vs = section('Variable series');
  assert.equal(await vs.locator('.cmd code').innerText(), `poon series followed`);
  // Before any release: part 1 only, part 2 next, 8 mints.
  for (let i = 0; !(await vs.locator('table.variable-series').count()); i++) {
    assert.ok(i < 40, 'the Variable series panel never drew its table');
    await vs.locator('button:has-text("Refresh")').click();
    await page.waitForTimeout(250);
  }
  // While the node reloads the panel may show poon's error instead.
  const row = async () => (await vs.locator('table.variable-series tbody tr').count()) ? vs.locator('table.variable-series tbody tr').first().innerText() : '';
  assert.match(await row(), /\t1\t2\t8\t/, 'one part released, part 2 next, 8 mints');

  // A claim on serial 9 (part 2) waits for a release: poon says so, and
  // the GUI shows it as waiting, not as a failure.
  await page.click('nav a[data-id=pools]');
  const rival = section('Build a rival claim');
  await rival.locator('input[name=serial]').fill('9');
  await rival.locator('input[name=parent]').fill('root');
  await rival.locator('button[type=submit]').click();
  const ask = rival.locator('form.ask');
  await ask.waitFor();
  await ask.locator('input').fill('a1'.repeat(32));
  await ask.locator('input').press('Enter');
  await rival.locator('.run-status.waiting', { hasText: 'waiting for a release (exit 3)' }).waitFor();

  // Add a release: the file picked in the browser goes to poon as -.
  await page.click('nav a[data-id=node]');
  const add = section('Add a release');
  await add.locator('input[name=pick]').setInputFiles({ name: 'release-2.json', mimeType: 'application/json', buffer: Buffer.from(release) });
  assert.equal(await add.locator('.cmd code').innerText(), `poon series add-release -`);
  page.once('dialog', d => d.accept());
  await add.locator('button[type=submit]').click();
  await add.locator('.run-status.ok').waitFor();
  // The line over the result says what the node did with it, from poon's
  // in_place or reloading (GUI-GREEN-053).
  assert.match(await add.locator('.result .applied').innerText(), /^Saved to node\.json; the running node (applied it at once, with no reload|is reloading to apply it)\.$/);
  // The node reloads to follow part 2; the panel then says so.
  for (let i = 0; !/\t2\t3\t12\t/.test(await row()); i++) {
    assert.ok(i < 60, `the panel never showed part 2 released: ${await row()}`);
    await vs.locator('button:has-text("Refresh")').click();
    await page.waitForTimeout(250);
  }
  assert.match(await row(), /part 2$/, 'the newest release is part 2');

  await page.click('button:has-text("Stop node")');
  await nodeStopped(page);
  assert.deepEqual(errors, [], 'no page errors');
  console.log('gui variable e2e: ok');
} finally {
  await close();
}
