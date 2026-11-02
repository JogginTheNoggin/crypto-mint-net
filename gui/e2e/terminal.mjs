// End-to-end check of the GUI in a real browser: the terminal runs the
// same commands as poo, prompts for a passphrase in a masked field, runs a
// node, and the Jobs view lists and stops it.
//
//   node gui/e2e/terminal.mjs PATH/TO/poo-gui [SCREENSHOT_DIR]
//
// Needs the playwright package and a Chromium it can launch.
import { execFileSync } from 'node:child_process';
import { join } from 'node:path';
import assert from 'node:assert/strict';
import { launch } from './lib.mjs';

const bin = process.argv[2];
const shots = process.argv[3];
const { home, env, url, browser, page, errors, close } = await launch(bin);

try {
  assert.ok(!page.url().includes('?t='), 'the secret stays in the address bar');
  // GUI-001: the sign-in address works once; a second browser that opens
  // it (from history, a process list or a log) is not signed in.
  {
    const other = await browser.newContext();
    const r = await (await other.newPage()).goto(url);
    assert.equal(r.status(), 403, 'a used sign-in address is refused');
    await other.close();
  }
  await page.click('nav a[data-id=terminal]');
  const line = page.locator('#term-line');
  const out = page.locator('.terminal .term-out');
  const runs = () => page.locator('.terminal').getAttribute('data-runs').then(Number);
  let expect = 0;
  // type types a line; a new command line (not input to a running one)
  // is awaited with idle().
  const type = async text => {
    if (await page.locator('.term-in .prompt').innerText() === '$ poo') expect = (await runs()) + 1;
    await line.fill(text);
    await line.press('Enter');
  };
  // until polls cond from here rather than with page.waitForFunction,
  // which some Playwright versions run through eval, and the page's CSP
  // (rightly) forbids eval.
  const until = async (cond, what, ms = 20000) => {
    for (const end = Date.now() + ms; !(await cond());) {
      assert.ok(Date.now() < end, `timed out waiting for ${what}`);
      await page.waitForTimeout(50);
    }
  };
  const idle = () => until(async () => (await runs()) >= expect, 'the command to end');

  // Same bytes as poo itself.
  const seed = 'ab'.repeat(32);
  const want = execFileSync(bin, ['poo', 'keygen', '-seed', seed, '-alg', 'ed25519'], { env }).toString();
  await type(`poo keygen -seed ${seed} -alg ed25519`);
  await idle();
  const text = await out.innerText();
  assert.ok(text.includes(want.trim().split('\n')[1].trim()), 'keygen output matches poo');

  // GUI-006: a line holding a seed is not saved to the browser's storage,
  // nor is one that starts with a space; up-arrow still recalls it this
  // session, and Jobs hides the seed.
  await type(' version');
  await idle();
  const saved = await page.evaluate(() => localStorage.getItem('poo-gui-history') || '');
  assert.ok(!saved.includes(seed), 'the seed is not saved in history');
  assert.ok(!saved.includes(' version'), 'a line starting with a space is not saved');
  await line.press('ArrowUp');
  await line.press('ArrowUp');
  assert.equal(await line.inputValue(), `poo keygen -seed ${seed} -alg ed25519`, 'up-arrow recalls the line this session');
  await line.fill('');
  await page.click('nav a[data-id=jobs]');
  const keyRow = page.locator('.job', { hasText: /^poo keygen/ }).first();
  await keyRow.waitFor();
  assert.ok((await keyRow.innerText()).includes('<hidden>') && !(await keyRow.innerText()).includes(seed), 'Jobs hides the seed');
  assert.equal(await keyRow.getByRole('button', { name: 'Run again in terminal' }).count(), 0, 'a hidden line is not offered to run again');
  // GUI-009: keygen printed a seed; its output is not kept to show again.
  assert.ok((await keyRow.innerText()).includes('output not kept'), 'Jobs says the output was not kept');
  assert.equal(await keyRow.getByRole('button', { name: 'Output' }).count(), 0, 'no Output button for it');
  const replay = await page.evaluate(async () => {
    const id = (await (await fetch('/api/jobs')).json()).find(j => j.args[0] === 'keygen').id;
    const r = await fetch(`/api/jobs/${id}/events?from=0`);
    return r.text();
  });
  assert.ok(!replay.includes(seed) && replay.includes('"withheld":true'), 'the event stream does not replay the seed');
  // The Make a key form shows a seed hidden until Show.
  await page.click('nav a[data-id=crypto]');
  const mk = page.locator('section.panel', { has: page.locator('h2', { hasText: 'Make a key' }) }).first();
  await mk.locator('button[type=submit]').click();
  const hidden = mk.locator('.result .secret');
  await hidden.waitFor();
  assert.equal(await hidden.locator('.secret-text').innerText(), '••••••••', 'the seed is hidden');
  await hidden.getByRole('button', { name: 'Show' }).click();
  assert.match(await hidden.locator('.secret-text').innerText(), /^[0-9a-f]{64}$/, 'Show reveals it');
  await page.click('nav a[data-id=terminal]');

  // An error is shown with its exit code, as a shell would.
  await type('no-such-command');
  await idle();
  assert.match(await out.innerText(), /poo: unknown command "no-such-command"[\s\S]*\[exit 1\]/);

  // GUI-002: "clear" is a poo command line like any other, as in a
  // shell after "poo"; the Clear button clears the screen.
  await type('clear');
  await idle();
  assert.match(await out.innerText(), /poo: unknown command "clear"/);
  await page.click('[data-act=clear]');
  assert.equal(await out.innerText(), '');
  // GUI-002: shell syntax the terminal does not run is refused, not
  // given a meaning a shell would not give it.
  const before = errors.length;
  await type('status | cat');
  await idle();
  assert.match(await out.innerText(), /no shell syntax/);
  // The browser logs the refused request; that one is expected.
  errors.splice(before, errors.length - before, ...errors.slice(before).filter(e => !/status of 400/.test(e)));

  // A passphrase prompt gets a masked field, and the passphrase is not
  // echoed. A new passphrase is asked twice (GUI-020): two that differ
  // change nothing, two that match make the wallet.
  const masked = async () => until(async () => (await line.getAttribute('type')) === 'password', 'a masked field');
  await type(`wallet create -wallet ${join(home, 'w.json')}`);
  await masked();
  await line.fill('hunter2-secret');
  await line.press('Enter');
  await out.filter({ hasText: 'new passphrase again' }).waitFor();
  await masked();
  await line.fill('hunter2-other');
  await line.press('Enter');
  await idle();
  assert.match(await out.innerText(), /the two new passphrases differ/);
  await type(`wallet create -wallet ${join(home, 'w.json')}`);
  for (let n = 0; n < 2; n++) {
    await masked();
    await line.fill('hunter2-secret');
    await line.press('Enter');
    if (n === 0) await until(async () => (await out.innerText()).split('new passphrase again').length > 2, 'the second ask');
  }
  await idle();
  assert.equal(await line.getAttribute('type'), 'text');
  const after = await out.innerText();
  assert.ok(!after.includes('hunter2-secret') && !after.includes('hunter2-other'), 'passphrase not echoed');
  assert.match(after, /wrote .*w\.json/);

  // Run a node in the terminal, send it to the background, and query it.
  await type('init -example -listen 127.0.0.1:0 -p2p off');
  await idle();
  await type('node');
  await out.filter({ hasText: 'rpc: listening' }).waitFor();
  if (shots) await page.screenshot({ path: join(shots, 'terminal.png') });
  await page.click('[data-act=detach]');
  await idle();
  for (let i = 0; ; i++) {
    await type('status');
    await idle();
    if (/"pools"|"series"|\[\s*\{/.test((await out.innerText()).split('$ poo status').pop())) break;
    assert.ok(i < 20, 'status never answered');
    await page.waitForTimeout(250);
  }

  // GUI-021: Ctrl-C is not an end of input, as in a shell. submit -f -
  // reads to its end; Ctrl-C fails that read, and nothing is decoded or
  // sent. Ctrl-D ends the input, and poo reads what was typed.
  const lastRun = async mark => (await out.innerText()).split(mark).pop();
  await type('submit layer -pool 1 -f -');
  await until(async () => (await page.locator('.term-in .prompt').innerText()) === 'input', 'the input prompt');
  await type('abcd');
  await line.press('Control+c');
  await idle();
  {
    const said = await lastRun('$ poo submit layer');
    assert.match(said, /stopped/, 'Ctrl-C fails the read');
    assert.doesNotMatch(said, /not hex|"hash"/, 'nothing typed before Ctrl-C was decoded or sent');
  }
  await type('submit layer -pool 1 -f -');
  await until(async () => (await page.locator('.term-in .prompt').innerText()) === 'input', 'the input prompt');
  await type('zz');
  await line.press('Control+d');
  await idle();
  assert.match(await lastRun('$ poo submit layer'), /\^D\n[\s\S]*the layer is not hex/, 'Ctrl-D ended the input and poo read it');

  // GUI-018: wallet follow, run in the terminal against the running node,
  // swaps the series' rule for the wallet's pools in one request (D139).
  // The node applies it, and the follow list has that one rule, not the
  // old one, not none, and not two.
  // answer gives the wallet's passphrase if the command asks for it.
  const answer = async () => {
    const asked = async () => (await line.getAttribute('type')) === 'password';
    await until(async () => (await asked()) || (await runs()) >= expect, 'a prompt or the command to end');
    if (await asked()) {
      await line.fill('hunter2-secret');
      await line.press('Enter');
    }
    await idle();
  };
  const since = async mark => (await out.innerText()).split(mark).pop();
  await type(`wallet watch -pool 0 -wallet ${join(home, 'w.json')}`);
  await answer();
  await type(`wallet follow -wallet ${join(home, 'w.json')}`);
  await answer();
  assert.match(await since('$ poo wallet follow'), /series [0-9a-f]+ follows pools \[0\]; the node applied it/, 'the running node applied the follow');
  await type('follow');
  await idle();
  const listed = JSON.parse((await since('$ poo follow')).replace(/^[^{]*/, '').replace(/[^}]*$/, ''));
  assert.equal(listed.follow.length, 1, 'one follow rule for the series');
  assert.deepEqual(listed.follow[0].pools, [0], "the rule is the wallet's pools");
  assert.deepEqual(listed.pools.map(p => p.pool), [0], 'the node follows only that pool now');

  // The Jobs view lists the running node and stops it.
  await page.click('nav a[data-id=jobs]');
  const nodeRow = page.locator('.job', { hasText: /^poo node/ }).filter({ hasText: 'running' });
  await nodeRow.first().waitFor();
  if (shots) await page.screenshot({ path: join(shots, 'jobs.png') });
  await nodeRow.first().getByRole('button', { name: 'Stop' }).click();
  await page.locator('.job', { hasText: /^poo node/ }).filter({ hasText: 'exit 0' }).first().waitFor({ timeout: 20000 });

  assert.deepEqual(errors, [], 'no page errors');
  console.log('gui e2e: ok');
} finally {
  await close();
}
