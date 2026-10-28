// End-to-end check of the GUI's views in a real browser: each runs its
// poo command and renders the output; forms build the command they show.
//
//   node gui/e2e/views.mjs PATH/TO/poo-gui [SCREENSHOT_DIR]
import { join } from 'node:path';
import assert from 'node:assert/strict';
import { launch, nodeStopped } from './lib.mjs';

const bin = process.argv[2];
const shots = process.argv[3];
const { home, page, errors, close } = await launch(bin);
const shot = async name => { if (shots) await page.screenshot({ path: join(shots, `${name}.png`), fullPage: true }); };
const section = title => page.locator('section.panel', { has: page.locator('h2', { hasText: title }) }).first();
const fill = async (sec, label, value) => sec.locator('label', { hasText: label }).first().locator('input, select').fill(value);

try {
  // Settings: the views' -root flag, shown in the header.
  await page.click('nav a[data-id=settings]');
  await page.locator('label', { hasText: 'Root' }).locator('input').fill(home);
  await page.click('button:has-text("Save")');
  await page.locator('#where', { hasText: `-root ${home}` }).waitFor();

  // Node: write a policy with the form; its preview is the command run.
  await page.click('nav a[data-id=node]');
  const init = section('Write a node policy');
  await init.locator('label', { hasText: 'worked example' }).locator('input').check();
  await fill(init, 'RPC address', '127.0.0.1:0');
  await fill(init, 'P2P address', 'off');
  assert.equal(await init.locator('.cmd code').innerText(), `poo init -root ${home} -example -listen 127.0.0.1:0 -p2p off`);
  await init.locator('button[type=submit]').click();
  await init.locator('.result', { hasText: '1 series' }).waitFor();

  // Start the node and see its pools rendered as a table.
  await page.click('button:has-text("Start node")');
  await page.locator('.live-out', { hasText: 'pools followed' }).waitFor();
  const pools = section('Followed pools');
  for (let i = 0; !(await pools.locator('table.data, dl.kv').count()); i++) {
    assert.ok(i < 40, 'status never rendered');
    await pools.locator('button:has-text("Refresh")').click();
    await page.waitForTimeout(250);
  }
  await shot('node');

  // Wallet: create one; the passphrase prompt is a masked field.
  await page.click('nav a[data-id=wallet]');
  const create = section('Create or restore a wallet');
  await create.locator('button[type=submit]').click();
  const ask = create.locator('form.ask input');
  await ask.waitFor();
  assert.equal(await ask.getAttribute('type'), 'password');
  await ask.fill('views-secret');
  await ask.press('Enter');
  await create.locator('.run-status.ok').waitFor();
  const addrs = section('Addresses');
  await addrs.locator('button[type=submit]').click();
  // Answer the passphrase prompt if the command asks for one.
  await addrs.locator('form.ask:not([hidden]) input, .run-status.ok, .run-status.bad').first().waitFor();
  if (await addrs.locator('form.ask:not([hidden]) input').count()) {
    await addrs.locator('form.ask input').fill('views-secret');
    await addrs.locator('form.ask input').press('Enter');
  }
  await addrs.locator('.run-status.ok').waitFor();
  await addrs.locator('.result table.data, .result dl.kv').first().waitFor();
  await shot('wallet');

  // Peers, policies and crypto render their commands' output.
  await page.click('nav a[data-id=peers]');
  await section('Peers').locator('.run-status').first().waitFor();
  await page.click('nav a[data-id=policies]');
  await section('Policies').locator('.result', { hasText: 'default' }).waitFor();
  await page.click('nav a[data-id=crypto]');
  await section('Hashes, key algorithms').locator('table.data', { hasText: 'blake3' }).waitFor();
  await shot('crypto');

  // Stop the node from the Node view.
  await page.click('nav a[data-id=node]');
  await page.click('button:has-text("Stop node")');
  await nodeStopped(page);

  assert.deepEqual(errors, [], 'no page errors');
  console.log('gui views e2e: ok');
} finally {
  await close();
}
