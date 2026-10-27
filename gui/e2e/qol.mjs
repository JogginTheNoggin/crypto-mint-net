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

try {
  // Getting started is the first view, and marks itself current.
  await page.locator('h1', { hasText: 'Getting started' }).waitFor();
  assert.equal(await page.locator('nav a[aria-current=page]').getAttribute('data-id'), 'start');
  if (shots) await page.screenshot({ path: join(shots, 'start.png') });

  // A step types its line into the terminal and runs it.
  await page.locator('.steps li', { hasText: 'Check the tool' }).getByRole('button').click();
  await page.waitForFunction(() => /\$ poo version\n\S/.test(document.querySelector('.terminal .term-out').innerText));
  assert.equal(await page.locator('nav a[aria-current=page]').getAttribute('data-id'), 'terminal');
  await page.waitForFunction(() => document.querySelector('.terminal [role=status]').textContent === 'Command finished.');

  // Tab completes a unique command, then a subcommand from that command's help.
  const line = page.locator('#term-line');
  await line.fill('wal');
  await line.press('Tab');
  await page.waitForFunction(() => document.querySelector('#term-line').value === 'wallet ');
  await line.press('b');
  await line.press('Tab');
  await page.waitForFunction(() => document.querySelector('#term-line').value === 'wallet balance ');
  // A subcommand listed inline in poo help: "poo policy list|use|...".
  await line.fill('policy ar');
  await line.press('Tab');
  await page.waitForFunction(() => document.querySelector('#term-line').value === 'policy archive ');
  // An ambiguous prefix lists the choices.
  await line.fill('s');
  await line.press('Tab');
  await page.waitForFunction(() => /series {2}sim {2}status/.test(document.querySelector('.terminal .term-out').innerText));
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
