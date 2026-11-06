// End-to-end check of the Ledger view (R41): looking through what a real
// node holds by clicking, from the pools to an entry, a transaction and an
// address, with no command to build; and the command lines hidden until
// Settings shows them.
//
//   node gui/e2e/ledger.mjs PATH/TO/poon-gui [SCREENSHOT_DIR]
import { execFileSync } from 'node:child_process';
import { join } from 'node:path';
import assert from 'node:assert/strict';
import { launch, nodeStopped } from './lib.mjs';

const bin = process.argv[2];
const shots = process.argv[3];
// A fresh browser as a user has it: command lines hidden.
const { env, page, errors, close } = await launch(bin, [], { showCommands: false });
const poon = (args, extra = {}) => execFileSync(bin, ['poon', ...args], { env: { ...env, ...extra } }).toString();
const body = page.locator('.ledger-body');
const shot = async name => { if (shots) await page.screenshot({ path: join(shots, `ledger-${name}.png`), fullPage: true }); };

try {
  poon(['init', '-example', '-listen', '127.0.0.1:0', '-p2p', 'off']);

  // With no node running, the view says so and links to the Node view.
  await page.click('nav a[data-id=ledger]');
  await body.locator('.ledger-no-node a[href="#/node"]').waitFor();

  await page.click('nav a[data-id=node]');
  await page.click('button:has-text("Start node")');
  await page.locator('.live-out', { hasText: 'rpc: listening' }).waitFor();
  // Three entries in pool 1: a claim in each, each given weight.
  const tids = [];
  for (const [serial, seed] of [[5, 'a1'], [6, 'b2'], [7, 'c3']]) {
    tids.push(JSON.parse(poon(['claim', '-serial', String(serial)], { POON_SEED: seed.repeat(32) })).tid);
    poon(['build', '-pool', '1', '-work', '4000']);
  }

  // Command lines are hidden by default, on every view.
  await page.click('nav a[data-id=ledger]');
  const pools = body.locator('table.ledger-pools');
  await pools.waitFor();
  assert.equal(await page.locator('.ledger-cmds .cmd').count(), 1);
  assert.equal(await page.locator('.ledger-cmds .cmd').isVisible(), false, 'the command line is hidden by default');
  // The example follows one series: no series column. Pool 1 has height 3.
  assert.deepEqual(await pools.locator('thead th').allInnerTexts(), ['Pool', 'Mints', 'Height', 'Weight', 'Pending', 'Other branches']);
  const row1 = pools.locator('tbody tr', { hasText: 'Pool 1' });
  assert.match(await row1.innerText(), /^Pool 1\t4–7\t3\t/);
  await shot('pools');

  // Clicking the row (not only its link) opens the pool's ledger.
  await row1.locator('td').nth(2).click();
  const entries = body.locator('table.ledger-entries');
  await entries.waitFor();
  assert.match(page.url(), /#\/ledger\?pool=1&series=[0-9a-f]{64}$/);
  assert.equal(await page.locator('.crumbs').innerText(), 'Pools › Pool 1');
  assert.deepEqual((await entries.locator('tbody tr td:first-child').allInnerTexts()), ['3', '2', '1'], 'newest first');
  assert.equal(await body.locator('svg.ledger-pic a.lp-go').count(), 3, 'each bar opens its entry');
  assert.match(await body.locator('.ledger-summary').innerText(), /^Height 3 · weight \d+ · showing heights 1 to 3$/);
  // The commands it ran, as poon prints them.
  const lines = await page.locator('.ledger-cmds .cmd code').allTextContents();
  assert.ok(lines.some(l => /^poon ledger -pool 1 -series [0-9a-f]{64} -limit 25$/.test(l)), lines.join('\n'));
  assert.ok(lines.some(l => /^poon pending -pool 1 -series [0-9a-f]{64}$/.test(l)), lines.join('\n'));
  await shot('pool');

  // Pages of one entry: Older, then Newer and Newest.
  const series = new URL(page.url()).hash.match(/series=([0-9a-f]{64})/)[1];
  await page.goto(page.url().replace(/#.*/, `#/ledger?pool=1&series=${series}&limit=1`));
  await entries.locator('tbody tr').first().waitFor();
  assert.deepEqual(await entries.locator('tbody tr td:first-child').allInnerTexts(), ['3']);
  assert.equal(await body.locator('.ledger-pages a', { hasText: 'Newer' }).count(), 0, 'no Newer at the tip');
  await body.locator('.ledger-pages a', { hasText: 'Older' }).click();
  await body.locator('.ledger-summary', { hasText: 'showing height 2' }).waitFor();
  await body.locator('.ledger-pages a', { hasText: 'Older' }).click();
  await body.locator('.ledger-summary', { hasText: 'showing height 1' }).waitFor();
  assert.equal(await body.locator('.ledger-pages a', { hasText: 'Older' }).count(), 0, 'no Older at height 1');
  await body.locator('.ledger-pages a', { hasText: 'Newer' }).click();
  await body.locator('.ledger-summary', { hasText: 'showing height 2' }).waitFor();
  await body.locator('.ledger-pages a', { hasText: 'Newest' }).click();
  await body.locator('.ledger-summary', { hasText: 'showing height 3' }).waitFor();

  // An entry: its facts and its transaction; Older and Newer step by one.
  await body.locator('svg.ledger-pic a.lp-go').first().click();
  await body.locator('h2', { hasText: 'Entry at height 3' }).waitFor();
  assert.equal(await page.locator('.crumbs').innerText(), 'Pools › Pool 1 › Height 3');
  assert.match(await body.locator('dl.facts').innerText(), /Own weight\s+\d+/);
  await body.locator('.ledger-pages a', { hasText: 'Older: height 2' }).click();
  await body.locator('h2', { hasText: 'Entry at height 2' }).waitFor();
  const txRow = body.locator('table.ledger-txs tbody tr');
  assert.equal(await txRow.count(), 1);
  assert.match(await txRow.innerText(), /Mint claim\t6\t100\t1 output/);
  await shot('entry');

  // Its transaction: where it stands, linked back to the entry, and what it pays.
  await txRow.locator('td').nth(1).click();
  await body.locator('h2', { hasText: 'Mint claim' }).waitFor();
  assert.match(page.url(), new RegExp(`tid=${tids[1]}`));
  assert.equal(await page.locator('.crumbs').innerText(), `Pools › Pool 1 › Height 2 › Transaction ${tids[1].slice(0, 10)}…${tids[1].slice(-6)}`);
  assert.match(await body.locator('dl.facts').innerText(), /Where it stands\s+In the selected ledger, at height 2 of pool 1\./);
  assert.match(await body.locator('dl.facts').innerText(), /Weight behind it\s+\d+ \(a rival must beat this/);
  await body.locator('details.ledger-raw').waitFor({ state: 'attached' });
  const out = body.locator('table.ledger-outputs tbody tr');
  assert.equal(await out.count(), 1);
  await shot('tx');

  // The address it paid: balance, holdings and history.
  await out.locator('a.go').click();
  await body.locator('.ledger-balance', { hasText: '100' }).waitFor();
  assert.equal(await body.locator('table.ledger-holdings tbody tr').count(), 1);
  const hist = body.locator('table.ledger-history tbody tr');
  assert.equal(await hist.count(), 1);
  assert.match(await hist.innerText(), /Pool 1, height 2\t\+100/);
  const addr = new URL(page.url()).hash.match(/address=([0-9a-f]+)/)[1];
  await shot('address');
  // Back returns to the transaction.
  await page.goBack();
  await body.locator('h2', { hasText: 'Mint claim' }).waitFor();

  // A mint opens who holds it, in its pool (GUI-GREEN-067).
  await body.locator('dl.facts a.go', { hasText: /^6$/ }).click();
  await body.locator('h2', { hasText: 'Mint 6' }).waitFor();
  assert.equal(await page.locator('.crumbs').innerText(), 'Pools › Pool 1 › Mint 6');
  const holders = body.locator('table.ledger-mint tbody tr');
  assert.equal(await holders.count(), 1);
  assert.match(await holders.innerText(), new RegExp(`^${addr.slice(0, 10)}…${addr.slice(-6)}\t100\t`));
  await shot('mint');

  // Find: a transaction id, then an address, then neither.
  const find = async text => {
    await page.locator('.ledger-search input').fill(text);
    await page.locator('.ledger-search button').click();
  };
  await find(tids[0]);
  await body.locator('dl.facts', { hasText: 'height 1 of pool 1' }).waitFor();
  await find(addr);
  await body.locator('.ledger-found-note').waitFor();
  await body.locator('.ledger-balance', { hasText: '100' }).waitFor();
  // A mint number opens the mint; one no followed pool holds says so.
  await find('2');
  await body.locator('h2', { hasText: 'Mint 2' }).waitFor();
  assert.match(await body.innerText(), /not claimed yet/);
  await find('99');
  await body.locator('dl.facts', { hasText: 'No pool this node follows holds this mint.' }).waitFor();
  await find('not-a-hash');
  await body.locator('.ledger-not-found').waitFor();

  // A look-up link elsewhere opens the Ledger view: the Watch view's
  // tables and the rendered results link here now.
  await page.click('nav a[data-id=pools]');
  const hold = page.locator('section.panel', { has: page.locator('h2', { hasText: 'Holdings of a mint' }) });
  await hold.locator('label', { hasText: 'Mint serial' }).locator('input').fill('5');
  await hold.locator('button[type=submit]').click();
  await hold.locator('a.lookup[href^="#/ledger?address="]').first().click();
  await body.locator('.ledger-balance').waitFor();

  // An entry built with no layer waits for one: one press fills Add a
  // layer with its pool and hash (GUI-GREEN-067).
  poon(['claim', '-serial', '4'], { POON_SEED: 'd4'.repeat(32) });
  await page.click('nav a[data-id=pools]');
  const build = page.locator('section.panel', { has: page.locator('h2', { hasText: 'Build an entry' }) });
  await build.locator('input[name=pool]').fill('1');
  await build.locator('button[type=submit]').click();
  await build.locator('.build-next button').click();
  const layer = page.locator('section.panel', { has: page.locator('h2', { hasText: 'Add a layer' }) });
  assert.equal(await layer.locator('input[name=pool]').inputValue(), '1');
  const filled = await layer.locator('input[name=entry]').inputValue();
  assert.match(filled, /^[0-9a-f]{64}$/);
  assert.ok((await build.locator('.area').textContent()).includes(filled), 'the hash poon printed for the built entry');

  // Settings shows the command lines, at once and on every view.
  await page.click('nav a[data-id=settings]');
  await page.locator('label.show-commands input').check();
  await page.click('nav a[data-id=ledger]');
  await pools.waitFor();
  assert.equal(await page.locator('.ledger-cmds .cmd').isVisible(), true, 'shown once Settings turns them on');
  await page.reload();
  await pools.waitFor();
  assert.equal(await page.locator('.ledger-cmds .cmd').isVisible(), true, 'and kept');
  await page.click('nav a[data-id=settings]');
  await page.locator('label.show-commands input').uncheck();

  await page.click('nav a[data-id=node]');
  await page.click('button:has-text("Stop node")');
  await nodeStopped(page);
  assert.deepEqual(errors, [], 'no page errors');
  console.log('ledger view ok');
} finally {
  await close();
}
