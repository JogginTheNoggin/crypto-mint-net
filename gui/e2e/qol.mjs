// End-to-end check of the quality-of-life prototypes: tab completion read
// from poo's help, the Getting started guide and the accessibility hooks.
//
//   node gui/e2e/qol.mjs PATH/TO/poo-gui [SCREENSHOT_DIR]
//
// Needs the playwright package and a Chromium it can launch.
import { join } from 'node:path';
import assert from 'node:assert/strict';
import { launch } from './lib.mjs';

const bin = process.argv[2];
const shots = process.argv[3];
const { page, errors, close } = await launch(bin);

// until polls cond from the test; page.waitForFunction can need eval,
// which the GUI's CSP forbids.
const until = async (cond, what, ms = 20000) => {
  for (const end = Date.now() + ms; !(await cond());) {
    assert.ok(Date.now() < end, `timed out waiting for ${what}`);
    await page.waitForTimeout(50);
  }
};
const termText = () => page.locator('.terminal .term-out').innerText();

try {
  // Getting started is the first view, and marks itself current.
  await page.locator('h1', { hasText: 'Getting started' }).waitFor();
  assert.equal(await page.locator('nav a[aria-current=page]').getAttribute('data-id'), 'start');
  if (shots) await page.screenshot({ path: join(shots, 'start.png') });

  // A step types its line into the terminal and runs it.
  await page.locator('.steps li', { hasText: 'Check the tool' }).getByRole('button').click();
  await until(async () => /\$ poo version\n\S/.test(await termText()), 'version output');
  assert.equal(await page.locator('nav a[aria-current=page]').getAttribute('data-id'), 'terminal');
  await until(async () => (await page.locator('.terminal [role=status]').textContent()) === 'Command finished.', 'the exit announcement');

  // Tab completes a unique command, then a subcommand from that command's help.
  const line = page.locator('#term-line');
  await line.fill('wal');
  await line.press('Tab');
  await until(async () => (await line.inputValue()) === 'wallet ', 'completion to "wallet "');
  await line.press('b');
  await line.press('Tab');
  await until(async () => (await line.inputValue()) === 'wallet balance ', 'completion to "wallet balance "');
  // A subcommand listed inline in poo help: "poo policy list|use|...".
  await line.fill('policy ar');
  await line.press('Tab');
  await until(async () => (await line.inputValue()) === 'policy archive ', 'completion to "policy archive "');
  // An ambiguous prefix lists the choices.
  await line.fill('s');
  await line.press('Tab');
  await until(async () => /series {2}sim {2}status/.test(await termText()), 'the list of choices');
  if (shots) await page.screenshot({ path: join(shots, 'complete.png') });

  // The skip link moves focus to the content without changing the view.
  await page.focus('.skip');
  await page.keyboard.press('Enter');
  assert.equal(await page.evaluate(() => document.activeElement.id), 'main');
  assert.ok(!page.url().endsWith('#main'));

  assert.deepEqual(errors, [], 'no page errors');
  console.log('gui qol e2e: ok');
} finally {
  await close();
}
