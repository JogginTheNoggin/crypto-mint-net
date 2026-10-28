// End-to-end check of the quality-of-life prototypes: tab completion read
// from poo's help, the Getting started guide, the accessibility hooks, next
// steps under errors, empty states, the Jobs filters, the narrow menu and high contrast.
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

  // An error keeps its exact text and gains a next step that links a view.
  await page.click('nav a[data-id=pools]');
  const status = page.locator('section.panel', { has: page.locator('h2', { hasText: 'Followed pools' }) });
  // With no node, the view shows one banner and each panel that needs a
  // node folds its error away, unchanged, behind "Show the error" (GREEN-011).
  const banner = page.locator('.node-banner:not([hidden])');
  await banner.waitFor();
  await status.locator('details.no-node > summary').click();
  await status.locator('.next-step').waitFor();
  assert.match(await status.locator('pre.err').innerText(), /^poo: rpc: is the node running\?/);
  await banner.locator('a').click();
  await until(async () => (await page.locator('nav a[aria-current=page]').getAttribute('data-id')) === 'node', 'the Node view');
  if (shots) await page.screenshot({ path: join(shots, 'next-step.png') });

  // An empty listing says what to do instead of "No output."
  await page.click('nav a[data-id=policies]');
  await page.locator('.hint.empty', { hasText: 'No saved node policies yet' }).waitFor();

  // Jobs: repeats of a command group into one row, and the filters narrow
  // the list. Three runs in a row from the terminal are adjacent whatever
  // the views' refresh timers did, so the check does not depend on timing.
  await page.click('nav a[data-id=terminal]');
  const term = page.locator('#term-line');
  for (let i = 0; i < 3; i++) {
    const before = Number(await page.locator('.terminal').getAttribute('data-runs'));
    await term.fill('version');
    await term.press('Enter');
    await until(async () => Number(await page.locator('.terminal').getAttribute('data-runs')) > before, 'version to finish');
  }
  await page.click('nav a[data-id=jobs]');
  const rows = page.locator('.jobs .job');
  await rows.first().waitFor();
  // The newest row is the three runs, shown once with their count.
  const newest = rows.first();
  await until(async () => (await newest.locator('code').innerText()) === 'poo version' &&
    (await newest.locator('.count').count()) === 1, 'poo version grouped into one row');
  assert.equal(await newest.locator('.count').innerText(), '×3');
  await page.fill('.job-tools input[type=search]', 'policy');
  await until(async () => (await rows.count()) > 0 && (await rows.allInnerTexts()).every(t => t.includes('policy')), 'the text filter');
  await page.fill('.job-tools input[type=search]', '');
  await page.locator('.job-tools label', { hasText: 'Failed only' }).locator('input').check();
  await until(async () => (await rows.allInnerTexts()).every(t => /exit [1-9]/.test(t)), 'the failed filter');
  if (shots) await page.screenshot({ path: join(shots, 'jobs-filter.png') });

  // A narrow window gets a view menu instead of scrolling tabs.
  await page.setViewportSize({ width: 390, height: 800 });
  assert.ok(await page.locator('#nav-menu').isVisible(), 'menu shown when narrow');
  assert.ok(!(await page.locator('nav#nav').isVisible()), 'tabs hidden when narrow');
  await page.selectOption('#nav-menu', 'wallet');
  await page.locator('h1', { hasText: 'Wallet' }).waitFor();
  if (shots) await page.screenshot({ path: join(shots, 'narrow.png') });
  await page.setViewportSize({ width: 1280, height: 800 });
  assert.ok(!(await page.locator('#nav-menu').isVisible()), 'menu hidden when wide');

  // A system that asks for more contrast gets firmer colours, in both themes.
  const muted = () => page.evaluate(() => getComputedStyle(document.documentElement).getPropertyValue('--muted').trim());
  await page.emulateMedia({ contrast: 'more', colorScheme: 'light' });
  assert.equal(await muted(), '#44443f');
  await page.emulateMedia({ contrast: 'more', colorScheme: 'dark' });
  assert.equal(await muted(), '#c8c8c0');
  await page.emulateMedia({ contrast: 'no-preference', colorScheme: 'light' });

  assert.deepEqual(errors, [], 'no page errors');
  console.log('gui qol e2e: ok');
} finally {
  await close();
}
