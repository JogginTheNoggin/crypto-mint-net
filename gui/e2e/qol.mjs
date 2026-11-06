// End-to-end check of the quality-of-life prototypes: tab completion read
// from poon's help, the Getting started guide, the accessibility hooks, next
// steps under errors, empty states, the Jobs filters, the narrow menu and high contrast.
//
//   node gui/e2e/qol.mjs PATH/TO/poon-gui [SCREENSHOT_DIR]
//
// Needs the playwright package and a Chromium it can launch.
import { execFileSync } from 'node:child_process';
import { mkdtempSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
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
  // The header shows the release, the one poon version prints: poon and
  // poon-gui share one version (GUI White G4, GUI-GREEN-058).
  const poonVersion = String(execFileSync(bin, ['poon', 'version'], { env })).trim();
  await page.locator('#version:not([hidden])').waitFor();
  assert.equal(await page.locator('#version').innerText(), `v${poonVersion}`);
  // Getting started is the first view, and marks itself current.
  await page.locator('h1', { hasText: 'Getting started' }).waitFor();
  assert.equal(await page.locator('nav a[aria-current=page]').getAttribute('data-id'), 'start');
  if (shots) await page.screenshot({ path: join(shots, 'start.png') });
  // The guide reaches grinding too, and each node step links its view (GREEN-036).
  const grindStep = page.locator('.steps li', { hasText: 'Help secure the ledger' });
  assert.equal(await grindStep.locator('code').innerText(), 'poon grind start tip');
  assert.equal(await grindStep.locator('a.step-view').getAttribute('href'), '#/work');
  await page.locator('.steps li', { hasText: 'Check its health' }).locator('a.step-view').click();
  await page.locator('h1', { hasText: 'Node' }).waitFor();
  await page.click('nav a[data-id=start]');

  // A step types its line into the terminal and runs it.
  await page.locator('.steps li', { hasText: 'Check the tool' }).getByRole('button').click();
  await until(async () => /\$ poon version\n\S/.test(await termText()), 'version output');
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
  // A subcommand listed inline in poon help: "poon policy list|use|...".
  await line.fill('policy ar');
  await line.press('Tab');
  await until(async () => (await line.inputValue()) === 'policy archive ', 'completion to "policy archive "');
  // An ambiguous prefix lists the choices.
  await line.fill('s');
  await line.press('Tab');
  await until(async () => /series {2}settings {2}sign {2}sim {2}status/.test(await termText()), 'the list of choices');
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
  // The pool list runs when Show is pressed (GUI-GREEN-057).
  await status.locator('.panel-head button', { hasText: 'Show' }).click();
  await banner.waitFor();
  await status.locator('details.no-node > summary').click();
  await status.locator('.next-step').waitFor();
  assert.match(await status.locator('pre.err').innerText(), /^poon: rpc: is the node running\?/);
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
  await until(async () => (await newest.locator('code').innerText()) === 'poon version' &&
    (await newest.locator('.count').count()) === 1, 'poon version grouped into one row');
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
  // The Jump to list is a row across the view, never the view list's
  // 170 px sidebar with its background and border (GUI-GREEN-060).
  const jumpBox = () => page.evaluate(() => { const n = document.querySelector('nav.jump'), cs = getComputedStyle(n); return { wide: n.getBoundingClientRect().width > 0.9 * document.querySelector('#main').getBoundingClientRect().width, bg: cs.backgroundColor, border: cs.borderRightWidth + ' ' + cs.borderBottomWidth }; });
  assert.deepEqual(await jumpBox(), { wide: true, bg: 'rgba(0, 0, 0, 0)', border: '0px 0px' }, 'the jump list when narrow');
  await page.setViewportSize({ width: 1280, height: 800 });
  assert.ok(!(await page.locator('#nav-menu').isVisible()), 'menu hidden when wide');
  assert.deepEqual(await jumpBox(), { wide: true, bg: 'rgba(0, 0, 0, 0)', border: '0px 0px' }, 'the jump list when wide');
  assert.equal(await page.locator('nav#nav').evaluate(n => Math.round(n.getBoundingClientRect().width)), 170, 'the view list keeps its width');

  // A system that asks for more contrast gets firmer colours, in both themes.
  const muted = () => page.evaluate(() => getComputedStyle(document.documentElement).getPropertyValue('--muted').trim());
  await page.emulateMedia({ contrast: 'more', colorScheme: 'light' });
  assert.equal(await muted(), '#44443f');
  await page.emulateMedia({ contrast: 'more', colorScheme: 'dark' });
  assert.equal(await muted(), '#c8c8c0');
  await page.emulateMedia({ contrast: 'no-preference', colorScheme: 'light' });

  // Node: poon doctor as a checklist. With no node policy the policy check
  // fails with its fix as printed, and the raw output is kept.
  await page.click('nav a[data-id=node]');
  const health = page.locator('section.panel', { has: page.locator('h2', { hasText: 'Health check' }) });
  await health.locator('ul.doctor li.doctor-fail', { has: page.locator('strong', { hasText: /^policy$/ }) }).waitFor();
  assert.match(await health.locator('li.doctor-fail .doctor-fix').innerText(), /^fix: poon init/);
  // Its fix is a command (fix_args), so it can be run from here.
  assert.equal(await health.locator('li.doctor-fail .doctor-fix button', { hasText: 'Run in terminal' }).count(), 1);
  assert.equal(await health.locator('.cmd code').innerText(), 'poon doctor -o json');
  assert.equal(await health.locator('details.raw').count(), 1);
  // "Output as printed" is drawn only when opened (ORANGE-020); Copy
  // output is there before, with poon's exact output.
  const raw = health.locator('details.raw');
  assert.equal(await raw.locator('.result .undrawn').count(), 1, 'the folded output is not drawn yet');
  assert.equal(await raw.locator('.result dl, .result table').count(), 0);
  await raw.locator('summary').click();
  await raw.locator('.result dl, .result table').first().waitFor();
  assert.equal(await raw.locator('.result .undrawn').count(), 0, 'opening it draws it');
  await raw.locator('summary').click();
  await raw.locator('summary').click();
  assert.equal(await raw.locator('.result > dl, .result > table, .result > .table-wrap').count(), 1, 'opened again, it is drawn once');
  if (shots) await page.screenshot({ path: join(shots, 'doctor.png') });

  // Peers: the action form shows only the flags poon peer takes for the
  // chosen action, and never sends a hidden one.
  await page.click('nav a[data-id=peers]');
  const act = page.locator('section.panel', { has: page.locator('h2', { hasText: 'Act on a peer' }) });
  const visible = async () => (await act.locator('form.fields > label:not([hidden]) > span').allInnerTexts());
  await act.locator('select[name=action]').selectOption('block');
  assert.deepEqual(await visible(), ['Action', 'Peer address', 'Seconds', 'Whole range']);
  await act.locator('input[name=range]').check();
  await act.locator('input[name=addr]').fill('203.0.113.5:7733');
  assert.equal(await act.locator('.cmd code').innerText(), 'poon peer block -range 203.0.113.5:7733');
  await act.locator('select[name=action]').selectOption('unblock');
  assert.deepEqual(await visible(), ['Action', 'Peer address']);
  // Hidden means not drawn, not only marked (GREEN-032).
  assert.ok(!(await act.locator('input[name=seconds]').isVisible()), 'a hidden flag is not drawn');
  assert.equal(await act.locator('.cmd code').innerText(), 'poon peer unblock 203.0.113.5:7733');
  await act.locator('select[name=action]').selectOption('clear');
  assert.deepEqual(await visible(), ['Action', 'Peer address', 'Clear']);

  // Series and keys: key and hash fields suggest what poon crypto printed.
  await page.click('nav a[data-id=crypto]');
  await page.locator('#crypto-hashes option[value=blake3]').waitFor({ state: 'attached' });
  assert.ok(await page.locator('#crypto-algs option[value=ed25519]').count(), 'key algorithms suggested');

  // Wallet: one wallet file for every form, shown in each preview.
  await page.click('nav a[data-id=wallet]');
  await page.fill('.wallet-pick input', 'w2.json');
  const bal = page.locator('section.panel', { has: page.locator('h2', { hasText: 'Balance' }) });
  assert.equal(await bal.locator('.cmd code').innerText(), 'poon wallet balance -wallet w2.json');
  assert.equal(await page.locator('section.panel label', { hasText: 'Wallet file' }).count(), 1, 'one wallet field');
  // One series for the whole view too, on every wallet command (GUI-GREEN-041).
  await page.fill('.wallet-pick input[name=wallet-series]', 'ab12');
  assert.equal(await bal.locator('.cmd code').innerText(), 'poon wallet balance -wallet w2.json -series ab12');
  const recv = page.locator('section.panel', { has: page.locator('h2', { hasText: 'What has arrived' }) });
  await recv.locator('input[name=tid]').fill('aa bb');
  await recv.locator('input[name=tidSerial]').fill('3 5');
  assert.equal(await recv.locator('.cmd code').innerText(), 'poon wallet receive -wallet w2.json -series ab12 -serial 3 -serial 5 -tid aa -tid bb');
  await page.fill('.wallet-pick input[name=wallet-series]', '');
  // Every poon wallet command has a form (GREEN-039), so a shared payment
  // Pay starts can be co-signed and submitted here too.
  const wsec = t => page.locator('section.panel', { has: page.locator('h2', { hasText: t }) }).first();
  await wsec('Record a shared policy').locator('input[name=name]').fill('joint');
  await wsec('Record a shared policy').locator('input[name=k]').fill('2');
  await wsec('Record a shared policy').locator('input[name=key]').fill('self ed25519:ab');
  assert.equal(await wsec('Record a shared policy').locator('.cmd code').innerText(), 'poon wallet policy -wallet w2.json -name joint -k 2 -key self -key ed25519:ab');
  await wsec('Co-sign a payment').locator('input[name=file]').fill('pay.json');
  assert.equal(await wsec('Co-sign a payment').locator('.cmd code').innerText(), 'poon wallet cosign -wallet w2.json pay.json');
  await wsec('Submit a co-signed payment').locator('input[name=file]').fill('pay.json');
  assert.equal(await wsec('Submit a co-signed payment').locator('.cmd code').innerText(), 'poon wallet submit -wallet w2.json pay.json');
  // Cold keys (D193): sign a co-signing file with a key kept elsewhere.
  // An empty key file has poon ask for the key in a masked field.
  const sg = wsec('Sign with a key kept elsewhere');
  await sg.locator('input[name=file]').fill('pay.json');
  assert.equal(await sg.locator('.cmd code').innerText(), 'poon sign pay.json -key-file -');
  // Key file shows only once "The key is in a file" is ticked (GUI-032).
  assert.equal(await sg.locator('input[name=keyFile]').isVisible(), false, 'no Key file field until the key is said to be in a file');
  await sg.locator('label', { hasText: 'The key is in a file' }).locator('input').check();
  assert.equal(await sg.locator('.cmd code').innerText(), 'poon sign pay.json -key-file -');
  // A key pasted where the path goes never reaches the command line, and
  // Sign refuses it, runs nothing and empties the field (GUI-032).
  const pasted = 'ab'.repeat(32);
  await sg.locator('input[name=keyFile]').fill(pasted);
  assert.equal(await sg.locator('.cmd code').innerText(), "poon sign pay.json -key-file '…'");
  const jobsBefore = (await page.evaluate(async () => (await import('/api.js')).jobs())).length;
  await sg.locator('button[type=submit]').click();
  await sg.locator('.refused[role=alert]', { hasText: /^Nothing was run: Key file holds what looks like a key, not a path/ }).waitFor();
  assert.equal(await sg.locator('input[name=keyFile]').inputValue(), '');
  assert.equal((await page.evaluate(async () => (await import('/api.js')).jobs())).length, jobsBefore, 'a key-shaped Key file starts no job');
  assert.ok(!(await page.content()).includes(pasted), 'the pasted key is nowhere on the page');
  // Written with the separators a key is often printed with, it is a key
  // too: colons as openssl prints it, groups with spaces, a dot at the end
  // (GUI/Red R36B).
  const lk = await page.evaluate(async () => { const { looksLikeKey } = await import('/views/wallet.js'); return ['ab'.repeat(16), 'AB'.repeat(32), 'ab'.repeat(15), '/keys/' + 'ab'.repeat(32), 'cold.key', 'C:\\k\\' + 'ab'.repeat(32), '',
    Array(32).fill('ab').join(':'), Array(5).fill('abcdef01').join(' '), 'ab'.repeat(32) + '.', 'ab-'.repeat(16), 'my.key.' + 'ab'.repeat(16)].map(looksLikeKey); });
  assert.deepEqual(lk, [true, true, false, false, false, false, false, true, true, true, true, false]);
  // After a refusal the form still follows its next run: unticked as the
  // refusal says, Sign shows poon's masked prompt, and Stop ends the job
  // (GUI-033). Before, the refusal left the result area marked gone, so
  // the job was never followed: no prompt, no Stop, a job slot held.
  const tmp = mkdtempSync(join(tmpdir(), 'poon-gui-qol-'));
  const payFile = join(tmp, 'pay.json');
  writeFileSync(payFile, '{"hash":"sha256","legs":[{"serial":1}]}');
  await sg.locator('input[name=file]').fill(payFile);
  await sg.locator('label', { hasText: 'The key is in a file' }).locator('input').uncheck();
  await sg.locator('button[type=submit]').click();
  await sg.locator('form.ask').waitFor({ state: 'visible', timeout: 10000 });
  await sg.locator('button', { hasText: /^Stop$/ }).click();
  await sg.locator('form.ask').waitFor({ state: 'hidden', timeout: 10000 });
  const signJob = async () => (await page.evaluate(async () => (await import('/api.js')).jobs())).filter(j => j.args.includes('sign') && j.args.includes('-key-file') && !j.done);
  for (let i = 0; i < 50 && (await signJob()).length; i++) await page.waitForTimeout(100);
  assert.deepEqual(await signJob(), [], 'Stop ended the sign job');
  // The same after the missing-field refusal (GUI-026).
  await sg.locator('input[name=file]').fill('');
  // The browser would refuse the empty field first; GUI-026's own check
  // is what is tested here.
  await sg.locator('input[name=file]').evaluate(i => i.removeAttribute('required'));
  await sg.locator('button[type=submit]').click();
  await sg.locator('.missing[role=alert]').waitFor();
  await sg.locator('input[name=file]').fill(payFile);
  await sg.locator('button[type=submit]').click();
  await sg.locator('form.ask').waitFor({ state: 'visible', timeout: 10000 });
  await sg.locator('button', { hasText: /^Stop$/ }).click();
  await sg.locator('form.ask').waitFor({ state: 'hidden', timeout: 10000 });
  await sg.locator('input[name=file]').fill('pay.json');
  await sg.locator('label', { hasText: 'The key is in a file' }).locator('input').check();
  await sg.locator('input[name=keyFile]').fill('cold.key');
  await sg.locator('input[name=alg]').fill('ml-dsa-65');
  await sg.locator('label', { hasText: '(-raw)' }).locator('input').check();
  await sg.locator('input[name=out]').fill('signed.json');
  await sg.locator('label', { hasText: 'Show only' }).locator('input').check();
  assert.equal(await sg.locator('.cmd code').innerText(), 'poon sign pay.json -key-file cold.key -alg ml-dsa-65 -raw -out signed.json -show');
  const notes = await page.evaluate(async () => {
    const { signNote } = await import('/views/wallet.js');
    const t = o => { const n = signNote({ code: 0, out: JSON.stringify(o) }); return n && n.textContent; };
    const leg = { serial: 1, tid: 'aa', inputs: 2, outputs: [] };
    return [t({ alg: 'ed25519', public_key: 'pk', address: 'ad', legs: [leg, leg], added: 2, missing: [[0, 1], [0, 0]], wrote: 'pay.json' }),
      t({ alg: 'ed25519', public_key: 'pk', address: 'ad', legs: [leg], added: 1, missing: [[0]] }),
      t({ alg: 'ed25519', public_key: 'pk', address: 'ad', legs: [leg], added: 0, missing: [[1, 1]], shown: true }),
      signNote({ code: 1, out: '' })];
  });
  assert.deepEqual(notes, [
    'This key (address ad) added 2 signatures to 2 transactions, written to pay.json; 1 signature is still needed.',
    'This key (address ad) added 1 signature to 1 transaction; no more signatures are needed: Submit a co-signed payment sends it.',
    'Shown only, nothing was signed: the payment has 1 transaction, and this key\'s address is ad; 2 signatures are still needed.',
    null]);
  const wpay = wsec('Pay');
  await wpay.locator('input[name=change]').fill('cc11');
  assert.match(await wpay.locator('.cmd code').innerText(), / -change cc11$/);
  await wpay.locator('input[name=change]').fill('');
  // Pay's one confirm warns when no co-signing file is filled in, and
  // only then (GUI-GREEN-061). poon refuses such a payment before it
  // records anything (Source #291), so no line of the GUI's follows.
  await wpay.locator('input[name=to]').fill('ab12');
  await wpay.locator('input[name=amount]').fill('5');
  const asked = [];
  const onDialog = d => { asked.push(d.message()); d.dismiss(); };
  page.on('dialog', onDialog);
  await wpay.locator('form.fields button[type=submit]').click();
  await wpay.locator('input[name=out]').fill('pay.json');
  await wpay.locator('form.fields button[type=submit]').click();
  await until(async () => asked.length === 2, 'two Pay confirms');
  page.off('dialog', onDialog);
  assert.match(asked[0], /^Run poon wallet pay -wallet w2\.json -to ab12 -amount 5\?\n\nNo co-signing file is filled in\. If this wallet is watch-only, or pays from a shared policy, fill one in first/);
  assert.equal(asked[1], 'Run poon wallet pay -wallet w2.json -to ab12 -amount 5 -out pay.json?');
  assert.equal(await wpay.locator('.run-status').count(), 0, 'a dismissed confirm runs nothing');
  for (const n of ['to', 'amount', 'out']) await wpay.locator(`input[name=${n}]`).fill('');
  // The seven cold-key steps are folded; each step's button goes to its
  // form, and the first links to Make a key.
  const cold = page.locator('section.cold-key');
  assert.equal(await cold.locator('details.cold').evaluate(d => d.open), false, 'the steps start folded');
  await cold.locator('summary').click();
  assert.equal(await cold.locator('.cold-steps li').count(), 7);
  assert.equal(await cold.locator('.cold-steps a').first().getAttribute('href'), '#/crypto');
  await cold.locator('.cold-steps button', { hasText: 'Sign with a key kept elsewhere' }).click();
  assert.equal(await page.evaluate(() => document.activeElement.tagName + ':' + document.activeElement.textContent), 'H2:Sign with a key kept elsewhere');
  const wcr = wsec('Create or restore a wallet');
  await wcr.locator('label', { hasText: 'Watch-only' }).locator('input').check();
  assert.equal(await wcr.locator('.cmd code').innerText(), 'poon wallet create -wallet w2.json -watch-only');
  await wcr.locator('label', { hasText: 'Watch-only' }).locator('input').uncheck();
  // A watch-only wallet claims to a policy, into a file the key signs
  // (GREEN-060); Claim file shows only with To a wallet policy.
  const wcl = wsec('Claim a mint');
  await wcl.locator('input[name=serial]').fill('7');
  assert.ok(!(await wcl.locator('input[name=out]').isVisible()), 'no claim file until a policy is named');
  await wcl.locator('input[name=toPolicy]').fill('cold');
  await wcl.locator('input[name=out]').fill('claim.json');
  assert.equal(await wcl.locator('.cmd code').innerText(), 'poon wallet claim -wallet w2.json -serial 7 -to-policy cold -out claim.json');
  await wcl.locator('input[name=toPolicy]').fill('');
  assert.equal(await wcl.locator('.cmd code').innerText(), 'poon wallet claim -wallet w2.json -serial 7');
  await wcl.locator('input[name=serial]').fill('');
  for (const [t, c] of [['Send dropped legs again', 'resend'], ['Follow only the watched mints', 'follow'], ['Change the passphrase', 'passwd'], ['Show the seed', 'seed']]) {
    assert.equal(await wsec(t).locator('.cmd code').innerText(), `poon wallet ${c} -wallet w2.json`);
  }
  const used = new Set(await page.locator('section.panel .cmd code').evaluateAll(cs => cs.map(c => c.textContent.split(' ')[2])));
  const help = execFileSync(bin, ['poon', 'wallet', 'help'], { env }).toString();
  for (const sub of [...help.matchAll(/^  ([a-z]+) /gm)].map(m => m[1])) assert.ok(used.has(sub), `the Wallet view runs poon wallet ${sub}`);

  // Every command poon help lists is run by some view or Getting started
  // step, or is here with the reason it stays in the terminal (GREEN-040).
  const TERMINAL_ONLY = {
    help: 'the terminal\'s own help, and every form shows its line',
    completion: 'sets up a shell, not this GUI',
    holdings: 'older form of poon unspent, kept for scripts; Explore runs unspent',
    payment: 'older form of poon tx, kept for scripts; Explore runs tx',
    claim: 'claims with a key file, where the key is: a watch-only wallet claims this way (D193; the Wallet view\'s cold-key steps say so); the Wallet view claims from the wallet',
    pay: 'pays with a raw key file; the Wallet view pays from the wallet',
    settings: 'new in Source Round 20 (D155); GUI/Grey adds its export and import form next, and the terminal runs it until then',
  };
  const ran = new Set();
  // Every view in the navigation, so a new one is walked too (GUI-GREEN-044).
  for (const v of await page.locator('nav a[data-id]').evaluateAll(as => as.map(a => a.dataset.id).filter(id => id !== 'terminal'))) {
    await page.click(`nav a[data-id=${v}]`);
    await page.locator(`nav a[data-id=${v}][aria-current=page]`).waitFor();
    await page.locator('#main h1').first().waitFor();
    for (const t of await page.locator('#main .cmd code, #main .step-cmd code').allInnerTexts()) ran.add(t.split(' ')[1]);
  }
  const top = execFileSync(bin, ['poon', 'help'], { env }).toString();
  for (const cmd of new Set([...top.matchAll(/^  poon ([a-z]+)/gm)].map(m => m[1]))) {
    assert.ok(ran.has(cmd) || TERMINAL_ONLY[cmd], `a view runs poon ${cmd}, or TERMINAL_ONLY says why not`);
  }
  await page.click('nav a[data-id=wallet]');
  await page.fill('.wallet-pick input', 'w2.json');
  await page.fill('.wallet-pick input', '');

  // Each command line has Copy, for a shell; never with a secret on it
  // (GUI-GREEN-046).
  await page.context().grantPermissions(['clipboard-read', 'clipboard-write']);
  await page.click('nav a[data-id=crypto]');
  const key = page.locator('section.panel', { has: page.locator('h2', { hasText: 'Make a key' }) });
  await key.locator('input[name=alg]').fill('ed25519');
  await key.locator('button.copy-cmd').click();
  await key.locator('button.copy-cmd', { hasText: 'Copied' }).waitFor();
  assert.equal(await page.evaluate(() => navigator.clipboard.readText()), await key.locator('.cmd code').innerText());
  // A seed is never on a form's line: poon asks for it (D98, GUI-GREEN-047).
  await key.locator('input[name=own]').check();
  assert.equal(await key.locator('.cmd code').innerText(), 'poon keygen -alg ed25519 -seed-file -');
  assert.ok(!(await key.locator('button.copy-cmd').isDisabled()), 'a line that reads its seed from input is copied');
  // A key made by another tool gives only its public half (D193).
  await key.locator('input[name=own]').uncheck();
  await key.locator('input[name=raw]').check();
  assert.equal(await key.locator('.cmd code').innerText(), 'poon keygen -alg ed25519 -seed-file - -raw');
  await key.locator('input[name=raw]').uncheck();
  // A new key saved to a file shows only its public half (GREEN-060).
  await key.locator('input[name=own]').uncheck();
  await key.locator('input[name=out]').fill('cold.key');
  assert.equal(await key.locator('.cmd code').innerText(), 'poon keygen -alg ed25519 -out cold.key');
  await key.locator('input[name=out]').fill('');
  await key.locator('input[name=own]').check();
  const secret = await page.evaluate(async () => {
    const { secretLine } = await import('/api.js');
    return ['poon keygen -seed ' + 'ab'.repeat(32), 'poon keygen -seed=ab', 'poon wallet create -restore -', 'poon wallet create -restore=-', 'poon keygen -seed-file -', 'poon sim -seed 3', 'poon keygen -seed'].map(secretLine);
  });
  assert.deepEqual(secret, [true, true, false, false, false, false, true], 'secretLine: a value of - is read in a masked field, not on the line');
  // A key where a key file's path goes is a secret line too (GUI-034):
  // plain, after =, with colons, quoted in groups, before a file name; a
  // path or - is not.
  const keyLines = await page.evaluate(async () => {
    const { secretLine } = await import('/api.js');
    const k = 'ab'.repeat(32);
    return [`poon sign x.json -key-file ${k}`, `poon sign x.json --key-file=${k}`, `poon sign x.json -key-file ${Array(32).fill('ab').join(':')}`, `poon sign -key-file '${Array(8).fill('abcdef01').join(' ')}' x.json`,
      `poon keygen -seed-file ${k}.`, 'poon sign x.json -key-file cold.key', 'poon sign x.json -key-file -', `poon sign x.json -key-file /keys/${k}`].map(secretLine);
  });
  assert.deepEqual(keyLines, [true, true, true, true, true, false, false, false], 'secretLine: a key typed as a key file');
  // A long line is read in time in proportion to its length: the value
  // after a key file flag stops at a path's / \\ or ., or 512 characters
  // (GUI/Red R38B: 20,000 words took 9 s).
  const long = await page.evaluate(async () => {
    const { secretLine } = await import('/api.js');
    const t0 = performance.now();
    const r = [secretLine('poon sign x.json -key-file ' + Array(20000).fill('zz').join(' ')), secretLine(Array(5000).fill('-key-file zz').join(' ')),
      secretLine('poon sign -key-file ' + Array(40).fill('ab').join(' ') + ' x.json'), secretLine('poon sign -key-file ' + Array(200).fill('zz').join(' ') + ' ' + 'ab'.repeat(32)),
      // One hex word longer than the cap is still tested (GUI/Red R39B).
      secretLine('poon sign -key-file ' + 'ab'.repeat(300))];
    return [...r, performance.now() - t0 < 1000];
  });
  assert.deepEqual(long, [false, false, true, false, true, true], 'secretLine on a long line is quick');
  // A list longer than 500 says it was cut, as a table does (ORANGE-018).
  const cut = await page.evaluate(async () => {
    const { render } = await import('/render.js');
    const d = document.createElement('div');
    d.append(render({ serials: Array.from({ length: 10000 }, (_, i) => i), short: [1, 2, 3] }));
    return { items: d.querySelectorAll('li').length, hints: [...d.querySelectorAll('.hint')].map(h => h.textContent) };
  });
  assert.deepEqual(cut, { items: 503, hints: ['Showing 500 of 10000 items; the terminal shows all.'] }, 'a long list is cut at 500 and says so');
  // The Variable series table draws 500 rows, then 500 more a press
  // (ORANGE-019): a node can follow 4,096 series.
  const vt = await page.evaluate(async () => {
    const { variableTable } = await import('/views/node.js');
    const vs = Array.from({ length: 1200 }, (_, i) => ({ id: String(i) }));
    const t = variableTable(vs, x => [x.id, '1', '2', '8', '800', 'none: part 1 only']);
    const rows = () => t.querySelectorAll('tbody tr').length;
    const note = () => t.querySelector('.variable-more')?.firstChild.textContent ?? null;
    const seen = [[rows(), note()]];
    t.querySelector('.variable-more button').click(); seen.push([rows(), note()]);
    t.querySelector('.variable-more button').click(); seen.push([rows(), note()]);
    const small = variableTable(vs.slice(0, 3), x => [x.id, '1', '2', '8', '800', '']);
    seen.push([small.querySelectorAll('tbody tr').length, small.querySelector('.variable-more')]);
    return seen;
  });
  assert.deepEqual(vt, [[500, 'Showing 500 of 1200 variable series. '], [1000, 'Showing 1000 of 1200 variable series. '], [1200, null], [3, null]], 'the Variable series table draws 500 rows a press');
  // A long doctor detail shows its first 1,000 characters, and all of it
  // while Show all is open (ORANGE-025); a short one is drawn as before.
  const dd = await page.evaluate(async () => {
    const { doctorReport } = await import('/views/node.js');
    const long = 'series x: no witness ceiling; '.repeat(1000);
    const rep = doctorReport({ checks: [{ name: 'ceiling', result: 'warn', detail: long }, { name: 'rpc', result: 'ok', detail: 'the node answers' }] }, {});
    document.body.append(rep);
    const li = rep.querySelectorAll('li');
    const text = () => li[0].querySelector('.doctor-detail-cut').firstChild.data.length;
    const more = li[0].querySelector('details.doctor-more');
    const seen = [text(), more.querySelector('summary').textContent, li[1].querySelector('.doctor-more')];
    more.open = true; await new Promise(r => setTimeout(r, 50)); seen.push(text());
    more.open = false; await new Promise(r => setTimeout(r, 50)); seen.push(text());
    rep.remove();
    return [...seen, long.length];
  });
  assert.deepEqual(dd, [1001, 'Show all 30000 characters', null, 30000, 1001, 30000], 'a long doctor detail is cut at 1,000 characters with Show all');
  // A result whose output poon-gui dropped in part says so and loses its
  // Save as file, which would write a file with a gap (ORANGE-026).
  const inc = await page.evaluate(async () => {
    const { incomplete, el } = await import('/views.js');
    const { saveAs } = await import('/views/policies.js');
    const area = el('div', {}, el('div', { class: 'result' }, '{}'), saveAs('{}', 'x.json'));
    incomplete(area);
    return [area.querySelectorAll('.save-as').length, area.firstChild.className, /dropped/.test(area.firstChild.textContent)];
  });
  assert.deepEqual(inc, [0, 'next-step output-dropped', true], 'a result with dropped output has no Save as file and says why');
  // parseJSON keeps a whole number above 2^53 exact, wherever it is, and
  // leaves digits inside strings and every smaller number alone
  // (GUI-GREEN-055).
  const pj = await page.evaluate(async () => {
    const { parseJSON } = await import('/render.js');
    const v = parseJSON('{"total": 9007199254740993, "n": -18446744073709551615, "s": "12345678901234567890 \\" 99999999999999999", "ok": 9007199254740991, "f": 1.5}');
    return [typeof v.total, String(v.total), String(v.n), v.s, v.ok, v.f];
  });
  assert.deepEqual(pj, ['bigint', '9007199254740993', '-18446744073709551615', '12345678901234567890 " 99999999999999999', 9007199254740991, 1.5]);
  // A big number in a list, on its own line or as the whole text is found
  // too; hex with long digit runs alone stays plain JSON.parse (ORANGE-022).
  const pj2 = await page.evaluate(async () => {
    const { parseJSON } = await import('/render.js');
    return [parseJSON('[1, 9007199254740993]')[1], parseJSON('{"a":\n  [\n    18446744073709551615\n  ]}').a[0], parseJSON(' 9007199254740993'), parseJSON('{"tip": "0ac5991086712345678901234567890fe0", "h": 12}')].map(x => typeof x === 'object' ? JSON.stringify(x) : `${typeof x}:${x}`);
  });
  assert.deepEqual(pj2, ['bigint:9007199254740993', 'bigint:18446744073709551615', 'bigint:9007199254740993', '{"tip":"0ac5991086712345678901234567890fe0","h":12}']);
  // A string that looks like the old fixed mark stays a string, in both
  // ways of reading: only numbers ever become BigInts (GUI/Red R30B).
  const pj3 = await page.evaluate(async () => {
    const { parseJSON, parseMarked } = await import('/render.js');
    const text = '{"s": "\\u0000big:12", "t": "\\u0000big:abc", "n": 9007199254740993}';
    return [parseJSON, parseMarked].map(f => { try { const v = f(text); return [typeof v.s, v.s.length, typeof v.t, typeof v.n, String(v.n)]; } catch (e) { return `threw: ${e.message}`; } });
  });
  assert.deepEqual(pj3, [['string', 7, 'string', 'bigint', '9007199254740993'], ['string', 7, 'string', 'bigint', '9007199254740993']], 'a string is never read as a number');
  // A ledger export is offered to save only as one whole ledger file:
  // cut at either end, short of its own from..to, or another kind of
  // file, it is not (GUI-GREEN-059).
  const ls = await page.evaluate(async () => {
    const { ledgerSummary } = await import('/views/pools.js');
    const entry = h => `{"height":${h},"hash":"${'a'.repeat(64)}","item":"AAAA"}`;
    const file = (from, to, n, kind = 'poon-ledger') => `{\n  "kind": "${kind}",\n  "format": 1,\n  "pool": 3,\n  "from": ${from},\n  "to": ${to},\n  "height": 9,\n  "entries":[${Array.from({ length: n }, (_, i) => '\n    ' + entry(from + i)).join(',')}\n  ]\n}\n`;
    const whole = file(2, 4, 3);
    return [ledgerSummary(whole), ledgerSummary(file(1, 1, 1)), ledgerSummary(file(10, 9, 0)), ledgerSummary(whole.slice(0, -40)), ledgerSummary(whole.slice(40)), ledgerSummary(file(2, 4, 2)), ledgerSummary(file(2, 4, 3, 'poon settings')), ledgerSummary('wrote 3 entries to x'), ledgerSummary('{"kind":"poon-ledger","format":2,"pages":[]}')];
  });
  assert.deepEqual(ls, ['A ledger file of pool 3: 3 entries, heights 2 to 4 of 9.', 'A ledger file of pool 3: 1 entry, heights 1 to 1 of 9.', 'A ledger file of pool 3 with no entries: the selected ledger\'s height is 9.', null, null, null, null, null,
    'A ledger file in format 2, which this poon-gui does not read; poon checks it in full on import.'], 'a later format gets a neutral line, not the cut-file alert (White N2)');
  // A ledger export that is not one whole file is folded under Output as
  // printed, not drawn, and gets one alert: its own, or ORANGE-026's when
  // poon-gui dropped part of the output (GUI/Orange R34B notes).
  const ex = await page.evaluate(async () => {
    const { exportResult } = await import('/views/pools.js');
    const { incomplete, el } = await import('/views.js');
    const cut = '{"kind": "poon-ledger", "format": 1, "entries": [';
    return [false, true].map(dropped => {
      const area = el('div', {}, el('div', { class: 'result' }, cut));
      const r = { code: 0, out: cut, err: '', dropped };
      exportResult(r, area);
      if (dropped) incomplete(area);
      return [area.querySelectorAll('[role=alert]').length, area.querySelectorAll('.ledger-partial').length, area.querySelectorAll('.output-dropped').length,
        !!area.querySelector('details.raw:not([open]) > .result'), area.querySelectorAll('.result:not(details.raw .result)').length, area.querySelectorAll('.save-as').length];
    });
  });
  assert.deepEqual(ex, [[1, 1, 0, true, 0, 0], [1, 0, 1, true, 0, 0]], 'a cut ledger export is folded and has one alert');
  // The Bounty view's tables lay out poon bounty list: an offer with an
  // unpaid claim gets Pay, an open offer of this node's Cancel, an open
  // peer's offer Work, and each fills its form (GUI-GREEN-062, 063).
  const bm = await page.evaluate(async () => {
    const { marketView, offNote, payNote, workNote, when } = await import('/views/bounty.js');
    const id1 = 'a'.repeat(64), id2 = 'b'.repeat(64), id3 = 'c'.repeat(64);
    const offer = { pool: 1, entry: 'e'.repeat(64), strength: 8, amount: 10, count: 2 };
    const acts = [];
    const box = document.createElement('div');
    box.append(...marketView({ post: true, take: true, mine: [
      { id: id1, offer, claims: [{ nonce: 7, pay_to: 'f'.repeat(64), paid: ['d'.repeat(64)] }, { nonce: 8, pay_to: 'f'.repeat(64) }] },
      { id: id2, offer, claims: [], state: 'expired', closes: '2026-11-06T15:00:00Z' }],
    held: [{ id: id3, from: '9'.repeat(64), offer, received: '2026-11-06T13:39:37Z', state: 'open' }, { id: 'd'.repeat(64), from: '9'.repeat(64), offer, received: '2026-11-06T13:39:37Z', state: 'filled' }] }, (k, v) => acts.push(`${k}:${v.id[0]}`)));
    for (const b of box.querySelectorAll('button')) b.click();
    return { text: [...box.querySelectorAll('h3, .bounty-on, .bounty-owed')].map(e => e.textContent), unpaid: [...box.querySelectorAll('.bounty-mine tbody tr')].map(r => r.children[7].textContent), acts,
      states: [...box.querySelectorAll('.bounty-mine tbody tr, .bounty-held tbody tr')].map(r => [...r.children].filter((_, i, a) => i === a.length - 3).map(c => c.textContent)[0]),
      off: (() => { const bad = { code: 1, out: '', err: 'poon: refused' }; const t = n => n && n.textContent.split('.')[0];
        return [t(offNote(bad, { bounty: { post: false, take: false } })), offNote(bad, { bounty: { post: true, take: false } }), t(offNote(bad, null)), t(offNote(bad, {})),
          offNote({ code: 1, out: '', err: 'poon: rpc: is the node running?' }, null), offNote({ code: 0, out: '{}', err: '' }, null)]; })(),
      closes: (() => { const t = box.querySelector('.bounty-mine tbody tr:nth-child(2) time'); return t && [t.getAttribute('datetime'), t.title, t.textContent !== t.title && t.textContent.length > 0]; })(),
      when: [when('not a time'), when(''), when(undefined)],
      pay: payNote({ offer: id1, nonce: 8, pay_to: 'f'.repeat(64), amount: 10, told: true }).textContent,
      work: [workNote({ found: true, strength: 19, tried: 1048576, outcome: 'accepted' }).textContent, workNote({ found: false, tried: 5 }).textContent] };
  });
  assert.deepEqual(bm.text, ['This node posts offers and takes offers from peers.', 'Your offers (2)', '1 accepted claim waits to be paid: the node kept the layer, and pays only when you press Pay (no auto-pay in phase 1).', 'Offers from peers (2)']);
  assert.deepEqual(bm.unpaid, ['1', '0']);
  // Each offer's state from poon (D197); Work only on an open peer offer.
  assert.deepEqual(bm.states, ['', 'expired', 'open', 'filled']);
  // A long list of peers' offers draws 500 rows, then the rest on Show
  // more (GUI/Orange R37B: 4,096 held offers were 70,000 elements).
  const bigHeld = await page.evaluate(async () => {
    const { marketView } = await import('/views/bounty.js');
    const offer = { pool: 1, strength: 8, amount: 10, count: 1 };
    const box = document.createElement('div');
    box.append(...marketView({ post: false, take: true, mine: [], held: Array.from({ length: 600 }, (_, i) => ({ id: i.toString(16).padStart(64, '0'), from: 'f'.repeat(64), offer, state: 'open' })) }));
    const rows = () => box.querySelectorAll('.bounty-held tbody tr').length;
    const first = [rows(), box.querySelector('.bounty-more').textContent];
    box.querySelector('.bounty-more button').click();
    return [...first, rows(), box.querySelectorAll('.bounty-more').length];
  });
  assert.deepEqual(bigHeld, [500, 'Showing 500 of 600 offers. Show more', 600, 0]);
  // poon's notes on the market's settings (D202) show over the tables.
  const bnotes = await page.evaluate(async () => {
    const { marketView } = await import('/views/bounty.js');
    const box = document.createElement('div');
    box.append(...marketView({ post: true, take: false, mine: [], held: [], notes: ['bounty.post is on but bounty.budget is not set'] }));
    return [...box.querySelectorAll('.bounty-note')].map(n => n.textContent);
  });
  assert.deepEqual(bnotes, ['bounty.post is on but bounty.budget is not set']);
  assert.deepEqual(bm.acts, ['pay:a', 'cancel:a', 'work:c'], 'no Cancel on an expired offer');
  // With no offers, only the steps this node takes are offered, and the
  // off line names the switch as Set limits shows it (GUI-GREEN-064).
  const empty = await page.evaluate(async () => {
    const { marketView, offNote } = await import('/views/bounty.js');
    const t = (post, take) => marketView({ post, take, mine: [], held: [] }).map(e => e.textContent).filter(s => s.startsWith('No offers'))[0];
    return [t(true, true), t(false, true), t(true, false), /bounty\.post on to pay[\s\S]*bounty\.take on to grind/.test(offNote({ code: 1, out: '', err: 'poon: off' }, { bounty: { post: false, take: false } }).textContent)];
  });
  assert.deepEqual(empty, ['No offers yet. Ask peers for offers below, or post one.', 'No offers yet. Ask peers for offers below.', 'No offers yet. Post one below.', true]);
  // A time poon printed is shown in the reader's own words, poon's text in
  // the tooltip (GUI-GREEN-063).
  assert.deepEqual(bm.closes, ['2026-11-06T15:00:00Z', '2026-11-06T15:00:00Z', true]);
  assert.deepEqual(bm.when, ['not a time', '', '']);
  assert.deepEqual(bm.off, ['The market is off on this node', null, 'If poon says the market is off: In Node policies, Set limits: bounty', 'If poon says the market is off: In Node policies, Set limits: bounty', null, null], 'off comes from status bounty (W-30); without it, a fixed line; error text is never read (S5)');
  assert.equal(bm.pay, 'Paid 10 to ffffffffff…ffffff for the layer with nonce 8; the grinder was told.');
  assert.deepEqual(bm.work, ['Found a layer of strength 19 in 1048576 tries; the payer answered: accepted. The payer pays when it presses Pay; the payment goes to the address you gave.', 'No layer found in 5 tries before the lease or the time ran out; work the offer again for a new lease.']);
  // The Followed series table names the reserve series by poon's builtin
  // field only (D170), and draws a variable series' mints from what is
  // released (GUI-GREEN-054).
  const ft = await page.evaluate(async () => {
    const { seriesTable } = await import('/views/node.js');
    const t = seriesTable([
      { id: 'fe52c1ac4178cc691c871524e53c4d1d2614a06b9dc68da13a19376b6591f139', builtin: 'default', pools: 1024, params: { mints: 32768, supply: 100000000000000, min_entry_strength: 20, min_layer_strength: 24 } },
      { id: 'fe52c1ac4178cc691c871524e53c4d1d2614a06b9dc68da13a19376b6591f13a', pools: 2, params: { mints: 64, supply: 100 }, variable: { mints: 12 }, warning: 'the series sets no witness ceiling: one witness can take seconds and hundreds of MB to check' },
    ]);
    return [...t.querySelectorAll('tbody tr')].map(r => [...r.children].map(c => c.textContent));
  });
  assert.deepEqual(ft.map(r => r.slice(0, 7)), [
    ['the built-in default series', 'fe52c1ac4178cc69…', 'fixed', '1024', '32768', '100000000000000', '20 / 24'],
    ['', 'fe52c1ac4178cc69…', 'variable', '2', '12', '100', '0 / 0'],
  ], 'the reserve is named by poon\'s builtin, not by its id');
  // The warning column shows poon's warning as given (D171): none for the
  // reserve, which sets a 128 KiB ceiling.
  assert.equal(ft[0][7], '');
  assert.match(ft[1][7], /no witness ceiling/);
  await key.locator('button', { hasText: 'Make key' }).click();
  const ask = key.locator('form.ask');
  await ask.waitFor();
  assert.equal(await ask.locator('input').getAttribute('type'), 'password', 'the seed is asked in a masked field');
  await ask.locator('input').fill('ab'.repeat(32));
  await ask.locator('input').press('Enter');
  await key.locator('.run-status.ok').waitFor();
  const want = execFileSync(bin, ['poon', 'keygen', '-seed', 'ab'.repeat(32)], { env }).toString();
  assert.equal((await key.locator('.result dd code.hex').first().getAttribute('title')).split('\n')[0], JSON.parse(want).address, 'the key is made from the seed typed in');
  await key.locator('input[name=own]').uncheck();
  // GUI-024: on Windows a copied line is pasted into PowerShell or cmd.exe,
  // so Copy is off for a word either shell would run or change. The server
  // here answers posix; /api/info is answered "windows", as on Windows.
  {
    const windows = async route => {
      const r = await route.fetch();
      await route.fulfill({ response: r, json: { ...(await r.json()), grammar: 'windows' } });
    };
    await page.route('**/api/info', windows);
    const origin = new URL(page.url()).origin;
    await page.goto(`${origin}/#/explore?address=x$(calc)`);
    await page.reload(); // a new page load, as a link from outside is
    const addr = page.locator('section.panel', { has: page.locator('h2', { hasText: 'An address' }) });
    const copy = addr.locator('button.copy-cmd');
    await addr.locator('.cmd code', { hasText: / x\$\(calc\)$/ }).waitFor(); // windows quoting: bare
    assert.ok(await copy.isDisabled(), 'a link with $(...) is not copied for a Windows shell');
    assert.match(await copy.getAttribute('title'), /Windows shell/);
    for (const bad of ['a;calc', 'x"&calc&"y', 'a|b', '%PATH%']) {
      await addr.locator('input[name=addr]').fill(bad);
      assert.ok(await copy.isDisabled(), `${bad} is not copied for a Windows shell`);
    }
    await addr.locator('input[name=addr]').fill('abc123');
    assert.ok(!(await copy.isDisabled()), 'a plain line is copied');
    await copy.click();
    await addr.locator('button.copy-cmd', { hasText: 'Copied' }).waitFor();
    assert.match(await page.evaluate(() => navigator.clipboard.readText()), /^poon address .*abc123$/);
    await addr.locator('input[name=addr]').fill('my addr');
    await copy.click();
    await addr.locator('button.copy-cmd', { hasText: 'Copied' }).waitFor();
    assert.match(await page.evaluate(() => navigator.clipboard.readText()), /^poon address .*"my addr"$/, 'a space is double-quoted');
    await page.unroute('**/api/info', windows);
    await page.goto(origin + '/#/start');
    await page.reload();
    await page.locator('#main h1').first().waitFor();
  }

  // A result has a Copy output button for poon's exact output.
  await page.click('nav a[data-id=crypto]');
  await page.locator('section.panel button.copy-out').first().waitFor();

  // Getting started uses the -root from Settings, like the views.
  await page.click('nav a[data-id=settings]');
  await page.locator('label', { hasText: 'Root' }).locator('input').fill('/tmp/qol-root');
  await page.click('button:has-text("Save")');
  await page.click('nav a[data-id=start]');
  const step = page.locator('.steps li', { hasText: 'Write a node policy' });
  // The guide writes the default node policy, which follows the built-in
  // reserve series (D170), as poon init does with no series (GUI-GREEN-054).
  assert.equal(await step.locator('code').innerText(), 'poon init -root /tmp/qol-root');
  assert.equal(await page.locator('.steps li', { hasText: 'Check the tool' }).locator('code').innerText(), 'poon version');
  await page.click('nav a[data-id=settings]');
  await page.locator('label', { hasText: 'Root' }).locator('input').fill('');
  await page.click('button:has-text("Save")');

  assert.deepEqual(errors, [], 'no page errors');
  console.log('gui qol e2e: ok');
} finally {
  await close();
}
