// End-to-end check of poon-gui -view-only: the header says so, reads run,
// and a command that changes something is refused by poon's own view-only
// mode, with the same message poon -view-only prints in a shell.
//
//   node gui/e2e/viewonly.mjs PATH/TO/poon-gui
//
// Needs the playwright package and a Chromium it can launch.
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { launch } from './lib.mjs';

const bin = process.argv[2];

// Without the flag there is no badge.
{
  const { page, close } = await launch(bin);
  try {
    await page.locator('nav a[data-id=terminal]').waitFor();
    assert.equal(await page.locator('#view-only').isVisible(), false, 'no badge without -view-only');
  } finally {
    await close();
  }
}

const { env, page, errors, close } = await launch(bin, ['-view-only']);
try {
  await page.locator('#view-only').waitFor();
  await page.click('nav a[data-id=terminal]');
  const line = page.locator('#term-line');
  const out = page.locator('.terminal .term-out');
  const until = async (cond, what) => {
    const end = Date.now() + 20000;
    while (!(await cond())) {
      if (Date.now() > end) throw new Error(`timed out waiting for ${what}`);
      await page.waitForTimeout(50);
    }
  };
  await line.fill('version');
  await line.press('Enter');
  await until(async () => (await out.innerText()).includes('poon '), 'version output');

  // The refusal is the core's: compare with poon -view-only from a shell.
  let want = '';
  try {
    execFileSync(bin, ['poon', '-view-only', 'init'], { env, stdio: ['ignore', 'ignore', 'pipe'] });
  } catch (e) {
    want = e.stderr.toString().trim();
  }
  assert.ok(want.includes('view-only'), `poon -view-only init refuses: ${want}`);
  await line.fill('init');
  await line.press('Enter');
  await until(async () => (await out.innerText()).includes(want), 'the refusal');
  await until(async () => (await out.innerText()).includes('[exit 1]'), 'exit 1');

  assert.deepEqual(errors, [], 'no page errors');
  console.log('gui view-only e2e: ok');
} finally {
  await close();
}
