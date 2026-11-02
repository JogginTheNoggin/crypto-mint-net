// End-to-end check of the quality-of-life prototypes: tab completion read
// from poo's help, the Getting started guide, the accessibility hooks, next
// steps under errors, empty states, the Jobs filters, the narrow menu and high contrast.
//
//   node gui/e2e/qol.mjs PATH/TO/poo-gui [SCREENSHOT_DIR]
//
// Needs the playwright package and a Chromium it can launch.
import { execFileSync } from 'node:child_process';
import { join } from 'node:path';
import assert from 'node:assert/strict';
import { launch } from './lib.mjs';

const bin = process.argv[2];
const shots = process.argv[3];
const { page, errors, close, env } = await launch(bin);

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
  // The guide reaches grinding too, and each node step links its view (GREEN-036).
  const grindStep = page.locator('.steps li', { hasText: 'Help secure the ledger' });
  assert.equal(await grindStep.locator('code').innerText(), 'poo grind start tip');
  assert.equal(await grindStep.locator('a.step-view').getAttribute('href'), '#/work');
  await page.locator('.steps li', { hasText: 'Check its health' }).locator('a.step-view').click();
  await page.locator('h1', { hasText: 'Node' }).waitFor();
  await page.click('nav a[data-id=start]');

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

  // Node: poo doctor as a checklist. With no node policy the policy check
  // fails with its fix as printed, and the raw output is kept.
  await page.click('nav a[data-id=node]');
  const health = page.locator('section.panel', { has: page.locator('h2', { hasText: 'Health check' }) });
  await health.locator('ul.doctor li.doctor-fail', { has: page.locator('strong', { hasText: /^policy$/ }) }).waitFor();
  assert.match(await health.locator('li.doctor-fail .doctor-fix').innerText(), /^fix: poo init/);
  // Its fix is a command (fix_args), so it can be run from here.
  assert.equal(await health.locator('li.doctor-fail .doctor-fix button', { hasText: 'Run in terminal' }).count(), 1);
  assert.equal(await health.locator('.cmd code').innerText(), 'poo doctor -o json');
  assert.equal(await health.locator('details.raw').count(), 1);
  if (shots) await page.screenshot({ path: join(shots, 'doctor.png') });

  // Peers: the action form shows only the flags poo peer takes for the
  // chosen action, and never sends a hidden one.
  await page.click('nav a[data-id=peers]');
  const act = page.locator('section.panel', { has: page.locator('h2', { hasText: 'Act on a peer' }) });
  const visible = async () => (await act.locator('form.fields > label:not([hidden]) > span').allInnerTexts());
  await act.locator('select[name=action]').selectOption('block');
  assert.deepEqual(await visible(), ['Action', 'Peer address', 'Seconds', 'Whole range']);
  await act.locator('input[name=range]').check();
  await act.locator('input[name=addr]').fill('203.0.113.5:8646');
  assert.equal(await act.locator('.cmd code').innerText(), 'poo peer block -range 203.0.113.5:8646');
  await act.locator('select[name=action]').selectOption('unblock');
  assert.deepEqual(await visible(), ['Action', 'Peer address']);
  // Hidden means not drawn, not only marked (GREEN-032).
  assert.ok(!(await act.locator('input[name=seconds]').isVisible()), 'a hidden flag is not drawn');
  assert.equal(await act.locator('.cmd code').innerText(), 'poo peer unblock 203.0.113.5:8646');
  await act.locator('select[name=action]').selectOption('clear');
  assert.deepEqual(await visible(), ['Action', 'Peer address', 'Clear']);

  // Series and keys: key and hash fields suggest what poo crypto printed.
  await page.click('nav a[data-id=crypto]');
  await page.locator('#crypto-hashes option[value=blake3]').waitFor({ state: 'attached' });
  assert.ok(await page.locator('#crypto-algs option[value=ed25519]').count(), 'key algorithms suggested');

  // Wallet: one wallet file for every form, shown in each preview.
  await page.click('nav a[data-id=wallet]');
  await page.fill('.wallet-pick input', 'w2.json');
  const bal = page.locator('section.panel', { has: page.locator('h2', { hasText: 'Balance' }) });
  assert.equal(await bal.locator('.cmd code').innerText(), 'poo wallet balance -wallet w2.json');
  assert.equal(await page.locator('section.panel label', { hasText: 'Wallet file' }).count(), 1, 'one wallet field');
  // Every poo wallet command has a form (GREEN-039), so a shared payment
  // Pay starts can be co-signed and submitted here too.
  const wsec = t => page.locator('section.panel', { has: page.locator('h2', { hasText: t }) }).first();
  await wsec('Record a shared policy').locator('input[name=name]').fill('joint');
  await wsec('Record a shared policy').locator('input[name=k]').fill('2');
  await wsec('Record a shared policy').locator('input[name=key]').fill('self ed25519:ab');
  assert.equal(await wsec('Record a shared policy').locator('.cmd code').innerText(), 'poo wallet policy -wallet w2.json -name joint -k 2 -key self -key ed25519:ab');
  await wsec('Co-sign a payment').locator('input[name=file]').fill('pay.json');
  assert.equal(await wsec('Co-sign a payment').locator('.cmd code').innerText(), 'poo wallet cosign -wallet w2.json pay.json');
  await wsec('Submit a co-signed payment').locator('input[name=file]').fill('pay.json');
  assert.equal(await wsec('Submit a co-signed payment').locator('.cmd code').innerText(), 'poo wallet submit -wallet w2.json pay.json');
  for (const [t, c] of [['Send dropped legs again', 'resend'], ['Follow only the watched mints', 'follow'], ['Change the passphrase', 'passwd'], ['Show the seed', 'seed']]) {
    assert.equal(await wsec(t).locator('.cmd code').innerText(), `poo wallet ${c} -wallet w2.json`);
  }
  const used = new Set(await page.locator('section.panel .cmd code').evaluateAll(cs => cs.map(c => c.textContent.split(' ')[2])));
  const help = execFileSync(bin, ['poo', 'wallet', 'help'], { env }).toString();
  for (const sub of [...help.matchAll(/^  ([a-z]+) /gm)].map(m => m[1])) assert.ok(used.has(sub), `the Wallet view runs poo wallet ${sub}`);
  await page.fill('.wallet-pick input', '');

  // A result has a Copy output button for poo's exact output.
  await page.click('nav a[data-id=crypto]');
  await page.locator('section.panel button.copy-out').first().waitFor();

  // Getting started uses the -root from Settings, like the views.
  await page.click('nav a[data-id=settings]');
  await page.locator('label', { hasText: 'Root' }).locator('input').fill('/tmp/qol-root');
  await page.click('button:has-text("Save")');
  await page.click('nav a[data-id=start]');
  const step = page.locator('.steps li', { hasText: 'Write a node policy' });
  assert.equal(await step.locator('code').innerText(), 'poo init -example -root /tmp/qol-root');
  assert.equal(await page.locator('.steps li', { hasText: 'Check the tool' }).locator('code').innerText(), 'poo version');
  await page.click('nav a[data-id=settings]');
  await page.locator('label', { hasText: 'Root' }).locator('input').fill('');
  await page.click('button:has-text("Save")');

  assert.deepEqual(errors, [], 'no page errors');
  console.log('gui qol e2e: ok');
} finally {
  await close();
}
