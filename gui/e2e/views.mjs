// End-to-end check of the GUI's views in a real browser: each runs its
// poo command and renders the output; forms build the command they show.
//
//   node gui/e2e/views.mjs PATH/TO/poo-gui [SCREENSHOT_DIR]
import { join } from 'node:path';
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { randomBytes } from 'node:crypto';
import { spawnSync } from 'node:child_process';
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
  // With no series poo follows the built-in reserve series; these say
  // otherwise (D170).
  await init.locator('input[name=noSeries]').check();
  await init.locator('input[name=anyCeiling]').check();
  assert.equal(await init.locator('.cmd code').innerText(), `poo init -root ${home} -example -listen 127.0.0.1:0 -p2p off -no-series -any-ceiling`);
  await init.locator('input[name=noSeries]').uncheck();
  await init.locator('input[name=anyCeiling]').uncheck();
  // Port mapping is opt-in (D182): off by default, -port-map once chosen,
  // with the privacy cost said beside it.
  await init.locator('select[name=portMap]').selectOption('auto');
  assert.match(await init.locator('.cmd code').innerText(), / -p2p off -port-map auto$/);
  assert.match(await init.innerText(), /peers learn this network's public IP address/);
  await init.locator('select[name=portMap]').selectOption('');
  assert.equal(await init.locator('.cmd code').innerText(), `poo init -root ${home} -example -listen 127.0.0.1:0 -p2p off`);
  await init.locator('button[type=submit]').click();
  await init.locator('.result', { hasText: '1 series' }).waitFor();

  // Start the node and see its pools rendered as a table. The box for
  // -reverify (D178) adds it to the line, and is left off.
  const runNode = section('Run the node');
  const rev = runNode.locator('label.reverify input');
  await rev.check();
  assert.match(await runNode.locator('.cmd code').innerText(), / -reverify$/);
  await rev.uncheck();
  assert.doesNotMatch(await runNode.locator('.cmd code').innerText(), /-reverify/);
  await page.click('button:has-text("Start node")');
  await page.locator('.live-out', { hasText: 'rpc: listening' }).waitFor();
  const pools = section('Followed pools');
  for (let i = 0; !(await pools.locator('table.series-totals').count()); i++) {
    assert.ok(i < 40, 'status never rendered');
    await pools.locator('button:has-text("Refresh")').click();
    await page.waitForTimeout(250);
  }
  await shot('node');
  // The 5 s refresh asks poo for totals per series (-counts, D177): one
  // row for the example's series with its 2 pools, no row per pool.
  assert.match(await pools.locator('.cmd code').innerText(), /^poo status -counts /);
  assert.doesNotMatch(await pools.innerText(), /"pools": \[/, 'no pool rows in the timed panel');
  // One totals row: the example's series is not built into poo, so it has
  // no name (D179), and it has 2 pools.
  assert.match(await pools.locator('table.series-totals tbody tr').first().innerText(), /^\td0b927f79f[0-9a-f]{6}…\t2\t/);
  // Snapshots (D178), as poo sums them up: a fresh node has no logs yet.
  await pools.locator('.snapshots', { hasText: /^Snapshots: \d+ of \d+ pools with a log have one.*snapshot_every/ }).waitFor();
  await pools.locator('.replay', { hasText: /^This start: \d+ pools? from (its snapshot|their snapshots)/ }).waitFor();
  // Each pool runs the full poo status only when Show is pressed
  // (GUI-GREEN-056): not on opening the view.
  const each = section('Each pool');
  assert.match(await each.locator('.cmd code').innerText(), /^poo status -root /);
  await each.locator('.not-run').waitFor();
  assert.equal(await each.locator('.panel-head button').innerText(), 'Show');
  for (let i = 0; !(await each.locator('table.data, dl.kv').count()); i++) {
    assert.ok(i < 40, 'Each pool never rendered');
    await each.locator('.panel-head button').click();
    await page.waitForTimeout(250);
  }
  assert.equal(await each.locator('.panel-head button').innerText(), 'Refresh');
  // The follow list shows its rules, and its pools folded under a count,
  // drawn when opened (GUI-GREEN-056).
  const fol = section('What the node follows');
  const folPools = fol.locator('details.follow-pools');
  // The panel first ran before the policy was written; run it again.
  await fol.locator('.panel-head button').click();
  await folPools.waitFor();
  assert.match(await folPools.locator('summary').innerText(), /^\d+ pools? followed$/);
  assert.equal(await folPools.locator('table').count(), 0, 'the pools are not drawn until opened');
  await folPools.locator('summary').click();
  await folPools.locator('table.data tbody tr').first().waitFor();
  // The entry cache line comes from poo status's bodies (D172).
  await pools.locator('.entry-cache', { hasText: /Entry cache: [0-9.]+ of [0-9]+ MiB \(entry_cache_mib\).*older ones \(outside recent_window\)/ }).waitFor();
  // A partial status reply (only max_bytes) is drawn with "?" for what
  // is missing, not thrown on, so the lines after it still draw (GUI/Red
  // R29B).
  const partial = await page.evaluate(async () => {
    const m = await import(new URL('views/node.js', location.href).href);
    try { return m.statusNotes({ bodies: { max_bytes: 64 * 1048576 } }).map(p => p.textContent); } catch (e) { return `threw: ${e.message}`; }
  });
  assert.deepEqual(partial, ['Entry cache: ? of 64 MiB (entry_cache_mib), ? bodies; ? hits, ? read from disk. ? of ? entries are in memory.'], 'a partial reply is drawn, not thrown on');
  assert.equal(await pools.locator('.file-limit-low').count(), 0, 'no file limit alert when the limit is high enough');
  // Under -reverify, a pool whose full replay differs from its snapshot
  // is an alert; all matching is a plain line (D178).
  const rv = await page.evaluate(async () => {
    const m = await import(new URL('views/node.js', location.href).href);
    const t = rp => m.statusNotes({ replay: rp }).filter(p => /Checked every record/.test(p.textContent)).map(p => [p.getAttribute('role'), p.textContent]);
    return [t({ snapshots: 0, reverified: 3, reverify_differed: 1 }), t({ snapshots: 0, reverified: 3 })];
  });
  assert.equal(rv[0][0][0], 'alert');
  assert.match(rv[0][0][1], /1 of 3 pools built a different pool/);
  // The node's release beside the GUI's (D184, GUI White S1): the same
  // binary here, so a plain line; another release is a note; an older
  // node that reports none is said so. Capped lists are counted from
  // poo's count fields (GUI White S2).
  await pools.locator('.node-release:not(.differs)', { hasText: /^The node runs v\S+/ }).waitFor();
  const relv = await page.evaluate(async () => {
    const m = await import(new URL('views/node.js', location.href).href);
    const t = (st, v) => m.statusNotes(st, v).filter(p => p.classList.contains('node-release') || p.classList.contains('replay') || p.classList.contains('reverify-not-compared')).map(p => [p.className, p.textContent]);
    return [
      t({ series: [], release: { version: '0.1.0-alpha.2', commit: 'abcdef0123456789' } }, '0.1.0-alpha.1'),
      t({ series: [] }, '0.1.0-alpha.1'),
      t({ replay: { snapshots: 0, snapshot_records: 0, records: 9, snapshots_unused: ['p0: other build', 'p1: other build'], snapshots_unused_count: 1024, reverify_not_compared: ['p2: no snapshot'], reverify_not_compared_count: 3 } }, ''),
    ];
  });
  assert.deepEqual(relv[0], [['next-step node-release differs', 'The node runs v0.1.0-alpha.2 (abcdef012345); this GUI is v0.1.0-alpha.1. They are different releases, so a view may show or send what the other does not know.']]);
  assert.deepEqual(relv[1], [['hint node-release', 'The node did not report its release (an older node).']]);
  assert.match(relv[2][0][1], /; 1024 snapshots not used: p0: other build; p1: other build; …\.$/);
  assert.deepEqual(relv[2][1], ['hint reverify-not-compared', 'Not compared: 3 pools: p2: no snapshot; ….']);
  // The port mapping line (D182): a failure is an alert with poo's error;
  // off draws nothing.
  const pmv = await page.evaluate(async () => {
    const m = await import(new URL('views/node.js', location.href).href);
    const t = pm => m.statusNotes({ port_map: pm }).filter(p => p.classList.contains('port-map')).map(p => [p.getAttribute('role'), p.textContent]);
    return [t({ mode: 'off', state: 'off' }), t({ mode: 'auto', state: 'failed', error: 'no gateway answered' }), t({ mode: 'upnp', state: 'advertised', method: 'upnp', external: '198.51.100.4:7733', advertised: '198.51.100.4:7733', lease_seconds: 3600 }), t({ mode: 'auto', state: 'unconfirmed', method: 'pcp', external: '198.51.100.4:7733', error: 'dial timed out' })];
  });
  assert.deepEqual(pmv[0], []);
  assert.deepEqual(pmv[1], [['alert', 'Port mapping (auto): failed. no gateway answered.']]);
  assert.deepEqual(pmv[2], [[null, 'Port mapping (upnp): advertised via upnp, the router reports 198.51.100.4:7733, offered to peers as 198.51.100.4:7733, lease 1 hour.']]);
  // Unconfirmed (mapped, the dial back did not arrive, removed) leaves
  // peers unable to reach the node too, so it is an alert (GUI/Red R31B).
  assert.deepEqual(pmv[3], [['alert', 'Port mapping (auto): unconfirmed via pcp, the router reports 198.51.100.4:7733. dial timed out.']]);
  assert.deepEqual(rv[1], [[null, 'Checked every record again: the full replay of all 3 pools matched their snapshots.']]);
  // Followed series: one row for the worked example's series, which is
  // not built into poo, so it has no name (GUI-GREEN-054).
  const fs = section('Followed series');
  await fs.locator('button:has-text("Refresh")').click();
  await fs.locator('table.followed-series tbody tr').first().waitFor();
  assert.match(await fs.locator('table.followed-series tbody tr').first().innerText(), /^\t[0-9a-f]{16}…\tfixed\t/);
  // Its warning column has poo's ceiling warning, as a warning only.
  await fs.locator('table.followed-series tbody tr .series-warning', { hasText: 'no witness ceiling' }).first().waitFor();
  // Follow more offers Replace (poo follow -replace) only once a series is
  // named, as poo takes it only with -series.
  const more = section('Follow more');
  const replace = more.locator('label', { hasText: 'Replace what the series follows' });
  assert.ok(!(await replace.isVisible()), 'Replace is hidden without a series');
  await fill(more, 'Series id', 'abc');
  await replace.locator('input').check();
  assert.match(await more.locator('.cmd code').innerText(), / -series abc -replace$/);
  await fill(more, 'Series id', '');
  assert.ok(!(await replace.isVisible()), 'Replace hides again with the series');
  assert.doesNotMatch(await more.locator('.cmd code').innerText(), /-replace/);
  // poo submit: an entry or layer made elsewhere, by its hex.
  await page.click('nav a[data-id=pools]');
  // poo pools prints every pool, so the view's pool list runs only when
  // Show is pressed (GUI-GREEN-057).
  const plist = page.locator('#main section.panel', { has: page.locator('h2', { hasText: 'Followed pools' }) });
  await plist.locator('.not-run').waitFor();
  await plist.locator('.panel-head button', { hasText: 'Show' }).click();
  await plist.locator('table.data tbody tr').first().waitFor();
  const submit = section('Submit an entry or layer');
  await submit.locator('select[name=kind]').selectOption('layer');
  await fill(submit, 'Pool', '1');
  await fill(submit, 'Hex', 'abcd');
  assert.equal(await submit.locator('.cmd code').innerText(), `poo submit layer -root ${home} -pool 1 abcd`);
  await fill(submit, 'From a file', '-');
  assert.ok(!(await submit.locator('label', { hasText: 'Hex' }).isVisible()), 'Hex hides when a file is named');
  assert.equal(await submit.locator('.cmd code').innerText(), `poo submit layer -root ${home} -pool 1 -f -`);
  // GUI-021: with -f -, poo reads what is pasted to its end. Stop is
  // Ctrl-C and sends nothing: poo fails on the stopped read, before it
  // decodes or hands anything to the node. "Send and end input" is
  // Ctrl-D: poo reads the paste and acts on it (here, refuses "zz").
  const submitRun = async () => {
    page.once('dialog', d => d.accept());
    await submit.locator('button[type=submit]').click();
    const ask = submit.locator('form.ask');
    await ask.locator('input').waitFor();
    assert.equal(await ask.locator('input').getAttribute('type'), 'text', 'a paste is not a secret');
    assert.ok(await ask.locator('button.end-input').isVisible(), 'the paste can be ended');
    return ask;
  };
  let paste = await submitRun();
  await paste.locator('input').fill('abcd');
  await paste.locator('button[type=submit]').click();
  await paste.locator('input').waitFor(); // poo reads on, for more
  await submit.locator('.run-bar button', { hasText: 'Stop' }).click();
  await submit.locator('.run-status.bad').waitFor({ timeout: 10000 });
  const stopped = await submit.locator('.area').innerText();
  assert.match(stopped, /stopped/, 'Stop fails the read');
  assert.doesNotMatch(stopped, /not hex|"hash"/, 'nothing pasted before Stop was decoded or sent');
  paste = await submitRun();
  await paste.locator('input').fill('zz');
  await paste.locator('button.end-input').click();
  await submit.locator('.run-status.bad').waitFor({ timeout: 10000 });
  assert.match(await submit.locator('.area').innerText(), /the layer is not hex/, 'the ended paste reached poo');
  const build = section('Build an entry');
  await build.locator('label', { hasText: 'Also print the hex' }).locator('input').check();
  assert.match(await build.locator('.cmd code').innerText(), / -bytes$/);
  // Build and Add a layer take poo grind's device, threads and for, and a
  // serial in place of a pool (GUI-GREEN-041).
  await build.locator('input[name=serial]').fill('5');
  await build.locator('input[name=device]').fill('cpu');
  await build.locator('input[name=threads]').fill('2');
  await build.locator('input[name=for]').fill('5s');
  assert.equal(await build.locator('.cmd code').innerText(), `poo build -root ${home} -serial 5 -device cpu -threads 2 -for 5s -bytes`);
  const layer = section('Add a layer');
  await layer.locator('input[name=start]').fill('7');
  await layer.locator('input[name=device]').fill('cpu');
  assert.equal(await layer.locator('.cmd code').innerText(), `poo work -root ${home} -device cpu -start 7`);
  // Building on a chosen parent, and a rival claim built to grind (D161).
  await build.locator('input[name=parent]').fill('before-claim');
  assert.equal(await build.locator('.cmd code').innerText(), `poo build -root ${home} -serial 5 -parent before-claim -device cpu -threads 2 -for 5s -bytes`);
  const rival = section('Build a rival claim');
  await rival.locator('input[name=serial]').fill('5');
  await rival.locator('input[name=parent]').fill('before-claim');
  await rival.locator('input[name=work]').fill('64');
  assert.equal(await rival.locator('.cmd code').innerText(), `poo claim -root ${home} -seed-file - -serial 5 -parent before-claim -work 64`);
  // GUI-026: a required field left empty runs nothing. Without Parent,
  // poo claim would submit the claim to the node and its peers, not build
  // it. The browser refuses the empty field; with its check off, the form
  // refuses it itself. Neither starts a job or asks for the seed.
  await rival.locator('input[name=parent]').fill('');
  assert.equal(await rival.locator('input[name=parent]').getAttribute('required'), '', 'Parent is required');
  // and says so in its label before anything is run (GUI-GREEN-050).
  const after = loc => loc.evaluate(s => getComputedStyle(s, '::after').content);
  assert.equal(await after(rival.locator('label:has(input[name=parent]) > span')), '" (required)"', 'a required field is marked');
  assert.equal(await after(rival.locator('label:has(input[name=work]) > span')), 'none', 'an optional one is not');
  const claims = () => page.evaluate(async () => (await (await fetch('/api/jobs')).json()).filter(j => j.args[0] === 'claim').length);
  await rival.locator('button[type=submit]').click();
  assert.ok(await rival.locator('input[name=parent]').evaluate(i => i.matches(':invalid')), 'the browser holds the empty Parent');
  await rival.locator('form.fields').evaluate(f => { f.noValidate = true; f.requestSubmit(); });
  assert.match(await rival.locator('.area .missing').innerText(), /^Nothing was run: fill in Parent\.$/);
  await page.waitForTimeout(300);
  assert.equal(await claims(), 0, 'no claim job ran');
  assert.equal(await rival.locator('form.ask').count(), 0, 'no seed was asked for');
  // A field required only in another form is not required where it is
  // optional: Where a payment stands takes no serial.
  assert.equal(await section('Where a payment stands').locator('input[name=serial]').getAttribute('required'), null, 'In mint serial is optional');
  // A pool's ledger out to a file and back in (D188): poo's reply says
  // what was kept and known; the export shown here has Save as file.
  const lexp = section('Export a pool\'s ledger');
  const lfile = join(home, 'pool-0-ledger.json');
  await fill(lexp, 'Pool', '0');
  await fill(lexp, 'From height', '1');
  assert.equal(await lexp.locator('.cmd code').innerText(), `poo ledger export -root ${home} -pool 0 -from 1`);
  await lexp.locator('button[type=submit]').click();
  // The file is said in a line, with Save as file; poo's output is
  // folded under Output as printed and drawn only when opened
  // (GUI-GREEN-059).
  await lexp.locator('.ledger-exported', { hasText: /^A ledger file of pool 0 (with no entries|: \d+ entr)/ }).waitFor();
  await lexp.locator('button', { hasText: 'Save as file' }).waitFor();
  assert.equal(await lexp.locator('details.raw .result table, details.raw .result dl').count(), 0, 'the ledger is not drawn before Output as printed is opened');
  await lexp.locator('details.raw summary').click();
  await lexp.locator('details.raw .result', { hasText: 'poo-ledger' }).waitFor();
  await fill(lexp, 'To a file', lfile);
  await lexp.locator('button[type=submit]').click();
  await lexp.locator('.result', { hasText: 'wrote 0 entries' }).waitFor();
  const limp = section('Import a ledger');
  await fill(limp, 'Ledger file', lfile);
  assert.equal(await limp.locator('.cmd code').innerText(), `poo ledger import -root ${home} ${lfile}`);
  page.once('dialog', d => d.accept());
  await limp.locator('button[type=submit]').click();
  await limp.locator('.ledger-imported', { hasText: /^Kept 0 entries and 0 layers; 0 entries and 0 layers were already known\.$/ }).waitFor();
  assert.equal(await limp.locator('.ledger-import-partial').count(), 0, 'a finished import has no "did not finish" alert');
  // An import that fails before sending anything prints no totals: poo's
  // error alone, no partial alert (GUI-031).
  await fill(limp, 'Ledger file', `${lfile}.missing`);
  page.once('dialog', d => d.accept());
  await limp.locator('button[type=submit]').click();
  await limp.locator('.run-status.bad').waitFor();
  assert.equal(await limp.locator('.ledger-import-partial').count(), 0, 'nothing was sent, so no partial alert');
  assert.equal(await limp.locator('.ledger-imported').count(), 0);
  // A stopped import prints poo's totals so far (sent, stopped), drawn as
  // an alert; a failure before any batch prints none, so no line.
  const notes = await page.evaluate(async () => {
    const m = await import(new URL('views/pools.js', location.href).href);
    const t = r => { const n = m.importNote(r); return n && [n.className, n.getAttribute('role'), n.textContent]; };
    return [
      t({ code: 1, out: JSON.stringify({ kept: 3, known: 1, layers_kept: 2, layers_known: 0, refused: [{ index: 4, hash: 'ab', reason: 'bad' }], switched: true, sent: 5, stopped: 'the command was stopped' }) }),
      t({ code: 1, out: '' }),
    ];
  });
  assert.deepEqual(notes[0], ['next-step ledger-import-partial', 'alert', 'The import stopped after 5 entries were sent (the command was stopped): kept 3 entries and 2 layers; 1 entries and 0 layers were already known; 1 refused (listed below); the node switched to the imported branch. Importing the same file again is safe, as entries already kept count as known.']);
  assert.equal(notes[1], null);
  // -dry-run reads the file without the node (D189).
  await fill(limp, 'Ledger file', lfile);
  await limp.locator('input[name=dryRun]').check();
  assert.equal(await limp.locator('.cmd code').innerText(), `poo ledger import -root ${home} -dry-run ${lfile}`);
  page.once('dialog', d => d.accept());
  await limp.locator('button[type=submit]').click();
  await limp.locator('.ledger-checked', { hasText: /^Checked without the node: pool 0, 0 of 0 entries read; no problems found\. Nothing was kept\.$/ }).waitFor();
  await limp.locator('input[name=dryRun]').uncheck();
  // A file picked in the browser goes to poo as its input (-).
  await limp.locator('input[type=file]').setInputFiles(lfile);
  assert.equal(await limp.locator('.cmd code').innerText(), `poo ledger import -root ${home} -`);
  await page.click('nav a[data-id=node]');

  // Wallet: create one; the passphrase prompt is a masked field.
  await page.click('nav a[data-id=wallet]');
  const create = section('Create or restore a wallet');
  // Restoring never puts the seed on the command line: poo asks for it.
  const restore = create.locator('label', { hasText: 'Restore from a seed' }).locator('input');
  await restore.check();
  assert.match(await create.locator('.cmd code').innerText(), / -restore -$/);
  await restore.uncheck();
  await create.locator('button[type=submit]').click();
  const ask = create.locator('form.ask input');
  await ask.waitFor();
  assert.equal(await ask.getAttribute('type'), 'password');
  // While it waits, the form runs no second job (GUI-012): the button is
  // off, a submit by other means is dropped, and the prompt stays.
  assert.ok(await create.locator('.actions button[type=submit]').isDisabled(), 'the form button is off while its job runs');
  for (let i = 0; i < 6; i++) await create.locator('form.fields').evaluate(f => f.requestSubmit());
  const waiting = await page.evaluate(async () => (await (await fetch('/api/jobs')).json()).filter(j => !j.done && j.args[0] === 'wallet').length);
  assert.equal(waiting, 1, 'one wallet job, not one per press');
  // poo asks for a new passphrase twice (GUI-020), each in the masked
  // field.
  for (const n of [1, 2]) {
    await create.locator(`form.ask:not([hidden]) input`).waitFor();
    await ask.fill('views-secret');
    await ask.press('Enter');
    if (n === 1) await create.locator('.result, pre', { hasText: 'new passphrase again' }).first().waitFor();
  }
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

  // Bounty market (GUI-GREEN-062): on a node with the market off, poo
  // refuses and Offers says how to turn it on; each form previews its
  // poo bounty line.
  await page.click('nav a[data-id=bounty]');
  await section('Offers').locator('.bounty-off', { hasText: /^The market is off until the node policy file \(node\.json/ }).waitFor();
  assert.equal(await section('Offers').locator('.cmd code').innerText(), `poo bounty list -root ${home}`);
  const bwork = section('Work an offer');
  await fill(bwork, 'Offer id', 'ab12');
  await fill(bwork, 'Pay to address', 'cd34');
  assert.equal(await bwork.locator('.cmd code').innerText(), `poo bounty work ab12 -root ${home} -to cd34`);
  await fill(section('Pay a claim'), 'Offer id', 'ab12');
  // No wallet file chosen in the Wallet view: poo's default, wallet.json.
  assert.equal(await section('Pay a claim').locator('.cmd code').innerText(), `poo bounty pay ab12 -root ${home}`);

  // Peers, policies and crypto render their commands' output.
  await page.click('nav a[data-id=peers]');
  await section('Peers').locator('.run-status').first().waitFor();
  // The timed panel leaves the book out (ORANGE-017); Find peers prints it.
  assert.equal(await section('Peers').locator('.cmd code').first().innerText(), `poo peers -root ${home} -counts`);
  // Claimed releases (D186, GUI White S3): "" is "not shared", and a
  // peer's release is said to be its own claim.
  const rn = await page.evaluate(async () => {
    const m = await import(new URL('views/peers.js', location.href).href);
    return [m.releaseNotes({ counts: { releases: { '': 2, '0.1.0-alpha.1': 3 } } }), m.releaseNotes({ peers: [{ addr: 'a', release: '0.1.0-alpha.1' }], counts: {} }), m.releaseNotes({ peers: [], counts: {} })].map(xs => xs.map(p => p.textContent));
  });
  assert.deepEqual(rn[0], ['Releases the connected peers claim: v0.1.0-alpha.1: 3 peers; not shared: 2 peers. Each is the peer\'s own claim.']);
  assert.match(rn[1][0], /its own claim, never checked/);
  assert.deepEqual(rn[2], []);
  // The peer book by hand, its filters, and peer export and import (D157).
  const addPeer = section('Add, remove, label or connect a peer');
  await fill(addPeer, 'Peer address', '1.0.0.9:7000');
  await addPeer.locator('input[name=label]').fill('friend box');
  assert.equal(await addPeer.locator('.cmd code').innerText(), `poo peer add -root ${home} -label 'friend box' 1.0.0.9:7000`);
  assert.ok(!(await addPeer.locator('label', { hasText: 'every unlisted address' }).isVisible()), 'Imported is for connect only');
  page.once('dialog', d => d.accept());
  await addPeer.locator('button[type=submit]').click();
  await addPeer.locator('.run-status.ok').waitFor();
  await addPeer.locator('select[name=action]').selectOption('connect');
  await fill(addPeer, 'Peer address', '');
  await addPeer.locator('label', { hasText: 'every unlisted address' }).locator('input').check();
  assert.equal(await addPeer.locator('.cmd code').innerText(), `poo peer connect -root ${home} -imported`);
  const find = section('Find peers');
  await fill(find, 'Label holds', 'friend');
  assert.equal(await find.locator('.cmd code').innerText(), `poo peers -root ${home} -label friend`);
  // The book states are boxes, as Watch's kinds are (GUI-GREEN-048).
  for (const c of ['unlisted', 'candidate']) await find.locator(`fieldset[data-flag=category] input[value=${c}]`).check();
  assert.equal(await find.locator('.cmd code').innerText(), `poo peers -root ${home} -category candidate,unlisted -label friend`);
  await find.locator('button[type=submit]').click();
  await find.locator('.result', { hasText: '1.0.0.9' }).waitFor();
  const trk = section('Tracking records');
  await trk.locator('label', { hasText: 'Only blocked' }).locator('input').check();
  assert.equal(await trk.locator('.cmd code').innerText(), `poo tracking -root ${home} -blocked`);
  const pexp = section('Export peers');
  const pfile = join(home, 'peers.json');
  await fill(pexp, 'To a file', pfile);
  await pexp.locator('button[type=submit]').click();
  await pexp.locator('.run-status.ok').waitFor();
  const pimp = section('Import peers');
  await fill(pimp, 'File', pfile);
  await pimp.locator('label', { hasText: 'address blocks too' }).locator('input').check();
  assert.equal(await pimp.locator('.cmd code').innerText(), `poo peers import -root ${home} -blocks -json ${pfile}`);
  await pimp.locator('button[type=submit]').click();
  await pimp.locator('.peers-plan', { hasText: 'already the same' }).waitFor();
  let pasked = '';
  page.once('dialog', d => { pasked = d.message(); d.accept(); });
  await pimp.locator('button.danger:has-text("Apply")').click();
  await pimp.locator('.area .area .peers-plan').waitFor();
  assert.match(pasked, /^Run poo peers import .* -apply -if-digest [0-9a-f]{64} /);
  // GUI-025: a file's blocks are read first, above the table and leading
  // it, however many addresses come with them; the notes count them.
  {
    const pf = JSON.parse(readFileSync(pfile, 'utf8'));
    const policies = pf.peers[0].policies;
    pf.peers = Array.from({ length: 600 }, (_, i) => ({ addr: `${20 + Math.floor(i / 250)}.${(i % 250) + 1}.7.9:7000`, policies }));
    pf.blocks = [{ addr: '198.51.100.20:7000', until: '2099-01-01T00:00:00Z' }];
    const big = join(home, 'big-peers.json');
    writeFileSync(big, JSON.stringify(pf));
    await fill(pimp, 'File', big);
    await pimp.locator('button[type=submit]').click();
    const plan = pimp.locator('.peers-plan', { hasText: 'Showing 500 of' });
    await plan.waitFor();
    assert.match(await plan.locator('.plan-blocks [role=alert]').innerText(), /^Read first: 1 block refuses addresses on this node: 198\.51\.100\.20:7000/, 'the block is read first');
    assert.equal(await plan.locator('tbody tr').first().getAttribute('class'), 'plan-block', 'the table starts with the block');
    {
      const notes = await plan.locator("p.hint").last().innerText();
      assert.match(notes, /^Once applied, .* takes them all out\. 1 block\. /, "poo's notes are sentences, and count the blocks");
      assert.equal(notes.split("1 block.").length, 2, "the blocks are counted once");
    }
  }
  await page.click('nav a[data-id=policies]');
  await section('Policies').locator('.result', { hasText: 'default' }).waitFor();
  // Export and import (D149): export every policy as printed, with Save as
  // file; export this one to a file; preview its import, then Apply.
  const exp = section('Export node policies');
  await exp.locator('select[name=scope]').selectOption('all');
  assert.equal(await exp.locator('.cmd code').innerText(), `poo policy export -all -root ${home}`);
  await exp.locator('button[type=submit]').click();
  await exp.locator('.run-status.ok').waitFor();
  const [dl] = await Promise.all([page.waitForEvent('download'), exp.locator('button.save-as').click()]);
  assert.equal(dl.suggestedFilename(), 'node-policies.json');
  const bundle = join(home, 'shared-policy.json');
  await exp.locator('select[name=scope]').selectOption('policy');
  await fill(exp, 'To a file', bundle);
  assert.equal(await exp.locator('.cmd code').innerText(), `poo policy export -root ${home} -out ${bundle}`);
  await exp.locator('button[type=submit]').click();
  await exp.locator('.result', { hasText: 'exported 1 node policy' }).waitFor();
  const imp = section('Import node policies');
  await fill(imp, 'File', bundle);
  assert.equal(await imp.locator('.cmd code').innerText(), `poo policy import -root ${home} -json ${bundle}`);
  // A plan that changes nothing has no digest (poo leaves it out, Source
  // R30), so there is nothing to apply and no Apply. The test blanks the
  // digest too, so this holds whichever poo prints.
  const noDigest = async route => {
    const res = await route.fetch();
    const body = (await res.text()).replace(/(\\"digest\\": *\\")[0-9a-f]{64}/g, '$1');
    await route.fulfill({ response: res, body });
  };
  await page.route('**/api/jobs/*/events*', noDigest);
  await imp.locator('button[type=submit]').click();
  await imp.locator('.nothing-to-apply', { hasText: 'Nothing to apply' }).waitFor();
  await page.unroute('**/api/jobs/*/events*', noDigest);
  assert.match(await imp.locator('.import-plan').innerText(), /already the same/);
  assert.equal(await imp.locator('button.danger:has-text("Apply")').count(), 0, 'no Apply when nothing changes');
  assert.equal(await imp.locator('.no-digest').count(), 0, 'nothing to apply is not the missing digest alert');
  // Files that each change one thing: A follows one issuer, B another
  // (and names a peer, a node setting left alone without -settings).
  mkdirSync(join(home, 'a'), { recursive: true });
  const fileA = join(home, 'a', 'shared-policy.json');
  const hA = randomBytes(32).toString('hex'), hB = randomBytes(32).toString('hex');
  const a0 = JSON.parse(readFileSync(bundle, 'utf8'));
  a0.policies[0].policy.follow = [...(a0.policies[0].policy.follow || []), { issuer: hA }];
  writeFileSync(fileA, JSON.stringify(a0, null, 2));
  // A file of every policy, renamed: each goes in under its own name, so
  // only -root is sent, and the plan says a new policy is made.
  const every = join(home, 'every-policy.json');
  writeFileSync(every, (await dl.path().then(f => readFileSync(f, 'utf8'))).replace('"name": "default"', '"name": "copy"').replace('"current": true', '"current": false'));
  await fill(imp, 'File', every);
  await imp.locator('label', { hasText: 'holds every saved policy' }).locator('input').check();
  assert.equal(await imp.locator('.cmd code').innerText(), `poo policy import -root ${home} -json ${every}`);
  await imp.locator('button[type=submit]').click();
  await imp.locator('.import-plan', { hasText: 'A new node policy.' }).waitFor();
  // The worked example's series sets no witness ceiling, so the new
  // policy's plan warns first, with poo's reason (D171). Apply stays.
  assert.match(await imp.locator('.import-plan .plan-unbounded').innerText(), /d0b927f7: the series sets no witness ceiling/);
  page.once('dialog', d => d.accept());
  await imp.locator('button.danger:has-text("Apply")').click();
  await imp.locator('.area .area .import-plan', { hasText: 'Saved.' }).waitFor();
  await section('Policies').locator('button:has-text("Refresh")').click();
  await section('Policies').locator('.result', { hasText: 'copy' }).waitFor();
  // A file picked in the browser is sent on poo's input: FILE is -.
  await imp.locator('label', { hasText: 'holds every saved policy' }).locator('input').uncheck();
  await imp.locator('input[type=file]').setInputFiles(fileA);
  await imp.locator('.cmd code', { hasText: / -json -$/ }).waitFor();
  assert.equal(await imp.locator('.cmd code').innerText(), `poo policy import -root ${home} -json -`);
  await imp.locator('button[type=submit]').click();
  await imp.locator('.import-plan', { hasText: 'Nothing changed yet' }).waitFor();
  // GUI-023: Apply runs what was previewed, and only that. Picking
  // another file, or changing an option, after a preview takes Apply
  // away, and Apply sends the text that was previewed.
  const peer = '203.0.113.7:7000';
  const fileB = join(home, 'with-peer.json');
  const b = JSON.parse(readFileSync(bundle, 'utf8'));
  b.policies[0].policy.p2p = { ...(b.policies[0].policy.p2p || {}), peers: [peer] };
  b.policies[0].policy.follow = [...(b.policies[0].policy.follow || []), { issuer: hB }];
  writeFileSync(fileB, JSON.stringify(b, null, 2));
  const applyBtn = imp.locator('button.danger:has-text("Apply")');
  assert.equal(await applyBtn.count(), 1, 'Apply offered for the previewed file');
  await imp.locator('input[type=file]').setInputFiles(fileB);
  assert.equal(await applyBtn.count(), 0, 'picking another file takes Apply away');
  await imp.locator('button[type=submit]').click();
  await imp.locator('.import-plan', { hasText: 'Nothing changed yet' }).waitFor();
  assert.match(await imp.locator('.import-plan').innerText(), /203\.0\.113\.7/, 'the plan shows B');
  await imp.locator('label', { hasText: 'Only write the policy' }).locator('input').check();
  assert.equal(await applyBtn.count(), 0, 'changing an option takes Apply away');
  await imp.locator('label', { hasText: 'Only write the policy' }).locator('input').uncheck();
  // A's preview again, then Apply: node.json gets A's issuer, not B's
  // issuer or peer.
  await imp.locator('input[type=file]').setInputFiles(fileA);
  await imp.locator('button[type=submit]').click();
  await imp.locator('.import-plan', { hasText: 'Nothing changed yet' }).waitFor();
  let asked = '';
  page.once('dialog', d => { asked = d.message(); d.accept(); });
  await applyBtn.click();
  await imp.locator('.area .area .import-plan', { hasText: 'Saved' }).waitFor();
  const afterA = readFileSync(join(home, 'policies', 'default', 'node.json'), 'utf8');
  assert.match(afterA, new RegExp(hA), "A's issuer was applied");
  assert.doesNotMatch(afterA, new RegExp(hB), "B's issuer was never applied");
  assert.match(asked, /with the previewed file shared-policy\.json\?$/, 'the dialog names the previewed file');
  assert.doesNotMatch(readFileSync(join(home, 'policies', 'default', 'node.json'), 'utf8'), /203\.0\.113\.7/, "B's peer was never applied");
  assert.match(asked, / -apply -if-digest [0-9a-f]{64} - with/, 'Apply sends the previewed file\'s digest (D151)');
  // Node settings come only with -settings (D150): B's peer is left alone
  // by default and listed, and with -settings it is a defence, read first.
  await fill(imp, 'File', fileB);
  await imp.locator('button[type=submit]').click();
  await imp.locator('.import-plan', { hasText: 'Node settings left alone' }).waitFor();
  await imp.locator('label', { hasText: 'node settings too' }).locator('input').check();
  assert.equal(await imp.locator('.cmd code').innerText(), `poo policy import -root ${home} -settings -json ${fileB}`);
  await imp.locator('button[type=submit]').click();
  await imp.locator('.import-plan .plan-defences', { hasText: 'p2p.peers' }).waitFor();
  // The file changes on disk after its preview: poo refuses it by its
  // digest, and nothing is applied.
  b.policies[0].policy.p2p.peers = ['198.51.100.9:7000'];
  writeFileSync(fileB, JSON.stringify(b, null, 2));
  page.once('dialog', d => d.accept());
  await applyBtn.click();
  await imp.locator('.area .area .run-status', { hasText: /exit|failed|1/ }).waitFor();
  await imp.locator('.area .area', { hasText: 'not the one previewed' }).waitFor();
  assert.doesNotMatch(readFileSync(join(home, 'policies', 'default', 'node.json'), 'utf8'), /198\.51\.100\.9|203\.0\.113\.7/, 'a changed file is never applied');
  // A preview that prints no digest gets no Apply, so Apply is never run
  // unpinned (GUI/Red R20B hardening). poo always prints one; the test
  // blanks it in the preview's output on its way to the page.
  await fill(imp, 'File', fileB);
  await page.route('**/api/jobs/*/events*', noDigest);
  await imp.locator('button[type=submit]').click();
  await imp.locator('.no-digest').waitFor();
  await page.unroute('**/api/jobs/*/events*', noDigest);
  await imp.locator('.import-plan').first().waitFor();
  assert.equal(await applyBtn.count(), 0, 'no Apply without a digest to pin it');
  // A long plan (ORANGE-015): a file following 1,000 issuers makes 2,000
  // lines. The first 500 are drawn, and 500 more on each press.
  const many = join(home, 'many-issuers.json');
  const hs = Array.from({ length: 1000 }, () => randomBytes(32).toString('hex'));
  const big = JSON.parse(readFileSync(bundle, 'utf8'));
  big.policies[0].policy.issuers = { ...(big.policies[0].policy.issuers || {}), accept: hs };
  big.policies[0].policy.follow = hs.map(issuer => ({ issuer }));
  writeFileSync(many, JSON.stringify(big));
  await fill(imp, 'File', many);
  await imp.locator('button[type=submit]').click();
  await imp.locator('.import-plan .plan-more', { hasText: 'Showing 500 of 2000 lines' }).waitFor();
  assert.equal(await imp.locator('.import-plan tbody tr').count(), 500);
  await imp.locator('.plan-more button', { hasText: 'Show more' }).click();
  await imp.locator('.import-plan .plan-more', { hasText: 'Showing 1000 of 2000 lines' }).waitFor();
  for (let i = 0; i < 2; i++) await imp.locator('.plan-more button').click();
  await imp.locator('.plan-more').waitFor({ state: 'detached' });
  assert.equal(await imp.locator('.import-plan tbody tr').count(), 2000);
  // Settings export and import (D155): the same form over poo settings,
  // with -map-path for registry files from another machine.
  const sexp = section('Export node settings');
  const sfile = join(home, 'node-settings.json');
  await fill(sexp, 'To a file', sfile);
  assert.equal(await sexp.locator('.cmd code').innerText(), `poo settings export -root ${home} -out ${sfile}`);
  await sexp.locator('button[type=submit]').click();
  await sexp.locator('.result', { hasText: 'exported the settings' }).waitFor();
  // One setting changed, so the plan has something to apply: a plan that
  // changes nothing has no Apply.
  const sj = JSON.parse(readFileSync(sfile, 'utf8'));
  sj.policies[0].policy.entry_cache_mib = 300;
  writeFileSync(sfile, JSON.stringify(sj, null, 2));
  const simp = section('Import node settings');
  await fill(simp, 'File', sfile);
  await fill(simp, 'Registry paths', '/old/home=/new/home');
  assert.equal(await simp.locator('.cmd code').innerText(), `poo settings import -root ${home} -map-path /old/home=/new/home -json ${sfile}`);
  await simp.locator('button[type=submit]').click();
  await simp.locator('.import-plan', { hasText: 'entry_cache_mib' }).waitFor();
  let sasked = '';
  page.once('dialog', d => { sasked = d.message(); d.accept(); });
  await simp.locator('button.danger:has-text("Apply")').click();
  await simp.locator('.area .area .import-plan', { hasText: 'Saved' }).waitFor();
  assert.match(readFileSync(join(home, 'policies', 'default', 'node.json'), 'utf8'), /"entry_cache_mib": 300/);
  assert.match(sasked, /^Run poo settings import .* -apply -if-digest [0-9a-f]{64} /);
  // A long "Read first" line (ORANGE-016): 30 new peers are 30 defences.
  // The line names 20 and how many more; Show all lists every one.
  const speers = join(home, 'many-peers.json');
  const sb = JSON.parse(readFileSync(sfile, 'utf8'));
  const peers30 = Array.from({ length: 30 }, (_, i) => `198.51.100.${i + 1}:7000`);
  sb.policies[0].policy.p2p = { ...(sb.policies[0].policy.p2p || {}), peers: peers30 };
  writeFileSync(speers, JSON.stringify(sb));
  await fill(simp, 'File', speers);
  await simp.locator('button[type=submit]').click();
  const sdef = simp.locator('.import-plan .plan-defences');
  await sdef.locator('[role=alert]', { hasText: /and \d+ more\.$/ }).waitFor();
  assert.ok(!(await sdef.locator('[role=alert]').innerText()).includes(peers30[29]), 'the line names only the first ones');
  await sdef.locator('summary', { hasText: /^Show all \d+$/ }).click();
  // The list is drawn on the details' toggle event, which comes after the
  // click returns: wait for it rather than read the details at once.
  await sdef.locator('details.plan-all p').waitFor();
  for (const pr of peers30) assert.ok((await sdef.locator('details').innerText()).includes(pr), `Show all lists ${pr}`);
  // The settable limits (D159-D161): poo lists them; Set limits saves
  // them, checked against each range, and the list shows the new value.
  const lim = section('Settable limits');
  await lim.locator('table.data', { hasText: 'p2p.max_book' }).waitFor();
  const setLim = section('Set limits');
  // A field for each limit poo listed, showing the value in effect, its
  // range and default, so no name is typed (GUI-GREEN-049).
  const maxBook = setLim.locator('input[data-limit="p2p.max_book"]');
  await maxBook.waitFor();
  assert.equal(await maxBook.getAttribute('placeholder'), '4096', 'the field shows the value in effect');
  assert.match(await setLim.locator('label:has(input[data-limit="p2p.max_book"])').innerText(), /256 to 65536, default 4096/);
  // Whether a running node takes it at once is poo's in_place (D170).
  assert.match(await setLim.locator('label:has(input[data-limit="p2p.max_book"])').innerText(), /a running node takes it (at once|on reload)/);
  await maxBook.fill('8192');
  await setLim.locator('input[data-limit="tracking.log_size"]').fill('512');
  // Refresh redraws the fields from poo's list and keeps what was typed
  // and not yet saved (ORANGE-018 note).
  await setLim.locator('input[data-limit="p2p.max_book"]').evaluate(i => i.setAttribute('data-old', ''));
  await lim.locator('button', { hasText: 'Refresh' }).click();
  await setLim.locator('input[data-limit="p2p.max_book"]:not([data-old])').waitFor();
  assert.equal(await setLim.locator('input[data-limit="p2p.max_book"]').inputValue(), '8192', 'a refresh keeps an unsaved value');
  assert.equal(await setLim.locator('.cmd code').innerText(), `poo policy limits -root ${home} -set p2p.max_book=8192 -set tracking.log_size=512`);
  page.once('dialog', d => d.accept());
  await setLim.locator('button[type=submit]').click();
  await setLim.locator('.run-status.ok').waitFor();
  // Both limits are ones a running node applies in place (D168), and the
  // line over the result says so, from poo's in_place (GUI-GREEN-053).
  assert.equal(await setLim.locator('.result .applied').innerText(), 'Saved to node.json; the running node applied it at once, with no reload.');
  await lim.locator('table.data tr', { hasText: 'p2p.max_book' }).filter({ hasText: '8192' }).first().waitFor();
  // Redrawn from the new list: empty again, showing the new value.
  await setLim.locator('input[data-limit="p2p.max_book"][placeholder="8192"]').waitFor();
  assert.equal(await setLim.locator('.cmd code').innerText(), `poo policy limits -root ${home}`, 'nothing is sent again after a save');
  // A save reloads the running node, which restarts its RPC; wait until
  // poo status answers with the saved node.json loaded before asking it
  // again, as a person would see on the Node view.
  for (let i = 0; ; i++) {
    const r = spawnSync(bin, ['poo', 'status', '-root', home], { env: { ...process.env, POO_HOME: home } });
    let st = null;
    try { st = r.status === 0 && JSON.parse(String(r.stdout)); } catch (_) { st = null; }
    if (st && !st.policy_changed) break;
    assert.ok(i < 100, 'the node came back from its reload');
    await page.waitForTimeout(100);
  }
  // A name the list lacks can still be typed, and poo checks it.
  await fill(setLim, 'Other limits', 'p2p.max_book=1');
  page.once('dialog', d => d.accept());
  await setLim.locator('button[type=submit]').click();
  await setLim.locator('.run-status.bad').waitFor();
  assert.match(await setLim.locator('.area').innerText(), /p2p\.max_book/, 'poo names the limit out of range');
  await page.click('nav a[data-id=crypto]');
  await section('Hashes, key algorithms').locator('table.data', { hasText: 'blake3' }).waitFor();
  await shot('crypto');
  // Variable supply series (D163): the definition, signing, parts and a
  // release, each a poo series command; poo works out every range.
  const vseed = 'cd'.repeat(32);
  const penv = { env: { ...process.env, POO_HOME: home } };
  const issuer = JSON.parse(String(spawnSync(bin, ['poo', 'keygen', '-seed', vseed], penv).stdout)).address;
  const sn = section('New series parameters');
  // A whole number above 2^53 is shown digit for digit, as poo printed
  // it: JSON.parse alone would show 100000000000000000 (GUI-GREEN-055).
  await sn.locator('input[name=mints]').fill('32');
  await sn.locator('input[name=poolSize]').fill('32');
  await sn.locator('input[name=supply]').fill('100000000000000001');
  await sn.locator('button[type=submit]').click();
  await sn.locator('.run-status.ok').waitFor();
  assert.equal(await sn.locator('.result dl.kv dt:text-is("supply") + dd').innerText(), '100000000000000001');
  await sn.locator('input[name=poolSize]').fill('');
  await sn.locator('input[name=supply]').fill('');
  assert.ok(!(await sn.locator('input[name=partSize]').isVisible()), 'Part size is for a variable series only');
  await sn.locator('input[name=mints]').fill('8');
  await sn.locator('input[name=issuer]').fill(issuer);
  await sn.locator('input[name=variable]').check();
  await sn.locator('input[name=partSize]').fill('4');
  await sn.locator('input[name=maxParts]').fill('5');
  assert.equal(await sn.locator('.cmd code').innerText(), `poo series new -mints 8 -issuer ${issuer} -variable -part-size 4 -max-parts 5`);
  await sn.locator('input[name=maxWitness]').fill('4096');
  assert.equal(await sn.locator('.cmd code').innerText(), `poo series new -mints 8 -max-witness 4096 -issuer ${issuer} -variable -part-size 4 -max-parts 5`);
  await sn.locator('input[name=maxWitness]').fill('');
  await sn.locator('button[type=submit]').click();
  await sn.locator('.run-status.ok').waitFor();
  await sn.locator('.result', { hasText: 'variable' }).waitFor();
  await sn.locator('button.save-as').waitFor();
  const vfile = join(home, 'variable-series.json');
  writeFileSync(vfile, spawnSync(bin, ['poo', 'series', 'new', '-mints', '8', '-issuer', issuer, '-variable', '-part-size', '4', '-max-parts', '5'], penv).stdout);
  // poo, not the GUI, refuses a part size that is not a multiple of the pool size.
  await sn.locator('input[name=partSize]').fill('3');
  await sn.locator('button[type=submit]').click();
  await sn.locator('.run-status.bad').waitFor();
  assert.match(await sn.locator('.area').innerText(), /whole multiples of the pool size/);
  // The pool size poo names is a field of the form, so the definition can
  // be fixed there (GUI-GREEN-051): with P = 3, N = 9 and M = 3 fit.
  await sn.locator('input[name=poolSize]').fill('3');
  await sn.locator('input[name=mints]').fill('9');
  await sn.locator('input[name=supply]').fill('50');
  assert.equal(await sn.locator('.cmd code').innerText(), `poo series new -mints 9 -supply 50 -pool-size 3 -issuer ${issuer} -variable -part-size 3 -max-parts 5`);
  await sn.locator('button[type=submit]').click();
  await sn.locator('.run-status.ok').waitFor();
  await sn.locator('input[name=poolSize]').fill('');
  await sn.locator('input[name=supply]').fill('');
  await sn.locator('input[name=mints]').fill('8');
  await sn.locator('input[name=partSize]').fill('4');
  const masked = async (sec, seed) => {
    const ask = sec.locator('form.ask');
    await ask.waitFor();
    assert.equal(await ask.locator('input').getAttribute('type'), 'password', 'the issuer seed is asked in a masked field');
    await ask.locator('input').fill(seed);
    await ask.locator('input').press('Enter');
  };
  const sign = section('Sign a series');
  await fill(sign, 'Series file', vfile);
  await sign.locator('input[name=alg]').fill('ed25519');
  assert.equal(await sign.locator('.cmd code').innerText(), `poo series sign ${vfile} -alg ed25519 -seed-file -`);
  await sign.locator('button[type=submit]').click();
  await masked(sign, vseed);
  await sign.locator('.run-status.ok').waitFor();
  assert.match(await sign.locator('.area').innerText(), /witness/i, 'the issuer witness is printed');
  const parts = section('Parts of a variable series');
  await fill(parts, 'Series file', vfile);
  await parts.locator('input[name=from]').fill('2');
  assert.equal(await parts.locator('.cmd code').innerText(), `poo series parts ${vfile} -from 2`);
  await parts.locator('button[type=submit]').click();
  await parts.locator('.run-status.ok').waitFor();
  const wantParts = JSON.parse(String(spawnSync(bin, ['poo', 'series', 'parts', vfile, '-from', '2'], penv).stdout));
  for (const pt of wantParts.parts) await parts.locator('.result', { hasText: pt.part_id.slice(0, 8) }).first().waitFor();
  // GUI-027: Release a part previews the serials it brings into being
  // before it asks for the seed; Sign signs only the part previewed, and
  // any change to the form takes Sign away.
  const rel = section('Release a part');
  await fill(rel, 'Series file', vfile);
  await rel.locator('input[name=part]').fill('2');
  assert.equal(await rel.locator('.cmd code').innerText(), `poo series parts ${vfile} -from 2 -to 2`, 'the form runs the preview');
  assert.match(await rel.locator('p.hint').first().innerText(), /Preview first shows which serials.*permanent.*every part before it/);
  await rel.locator('button[type=submit]').click();
  const range = rel.locator('.release-range');
  await range.waitFor();
  assert.match(await range.innerText(), /^Read first: releasing part 2 of series [0-9a-f]{12} brings into being serials 8 to 11, and with every part before it, serials 0 to 11: 12 mints\./);
  assert.equal(await rel.locator('form.ask').count(), 0, 'no seed is asked for by the preview');
  await rel.locator('input[name=part]').fill('3');
  assert.equal(await rel.locator('button', { hasText: /^Sign part/ }).count(), 0, 'a change takes Sign away');
  await rel.locator('input[name=part]').fill('2');
  // The key algorithm is kept with the preview, so Sign uses it.
  await rel.locator('input[name=alg]').fill('ed25519');
  await rel.locator('button[type=submit]').click();
  await rel.locator('.release-range').waitFor();
  let relAsked = '';
  page.once('dialog', d => { relAsked = d.message(); d.accept(); });
  await rel.locator('button', { hasText: 'Sign part 2' }).click();
  assert.match(relAsked, /^Release part 2 of series [0-9a-f]{12}: serials 8 to 11, .*serials 0 to 11: 12 mints\? This runs poo series release .* -part 2 -alg ed25519 -seed-file - and cannot be taken back\.$/);
  await masked(rel, vseed);
  await rel.locator('.release-made').waitFor();
  assert.match(await rel.locator('.area .area').first().innerText(), /previous/i, 'the release is printed');
  await rel.locator('.area .area button.save-as').first().waitFor();
  // poo checks the release just signed, sent on its input (-check -).
  await rel.locator('.release-check .result', { hasText: 'releases part 2, serials 8 to 11' }).waitFor();
  // A series with no most parts takes any part: part 50 of an 8-mint
  // series is named as serials 0 to 203 before anything is signed.
  const open = join(home, 'open-series.json');
  writeFileSync(open, spawnSync(bin, ['poo', 'series', 'new', '-mints', '8', '-issuer', issuer, '-variable', '-part-size', '4'], penv).stdout);
  await fill(rel, 'Series file', open);
  await rel.locator('input[name=part]').fill('50');
  await rel.locator('button[type=submit]').click();
  await rel.locator('.release-range', { hasText: 'serials 0 to 203: 204 mints' }).waitFor();
  const rfile = join(home, 'release-2.json');
  const sf = join(home, 'issuer-seed');
  writeFileSync(sf, vseed);
  writeFileSync(rfile, spawnSync(bin, ['poo', 'series', 'release', vfile, '-part', '2', '-seed-file', sf], penv).stdout);
  const chk = section('Check a release');
  await fill(chk, 'Series file', vfile);
  await fill(chk, 'Release file', rfile);
  assert.equal(await chk.locator('.cmd code').innerText(), `poo series release ${vfile} -check ${rfile}`);
  await chk.locator('button[type=submit]').click();
  await chk.locator('.result', { hasText: 'releases part 2, serials 8 to 11' }).waitFor();
  // The release file can be picked in the browser too, sent as -check -,
  // as Add a release and the imports take theirs (GUI-GREEN-052).
  await chk.locator('input[name=pick-check]').setInputFiles({ name: 'release-2.json', mimeType: 'application/json', buffer: readFileSync(rfile) });
  assert.equal(await chk.locator('.cmd code').innerText(), `poo series release ${vfile} -check -`);
  await chk.locator('button[type=submit]').click();
  await chk.locator('.run-status.ok').waitFor();
  await chk.locator('.result', { hasText: 'releases part 2, serials 8 to 11' }).waitFor();
  // GUI-028: a far part is asked about first, with its cost, as poo
  // hashes every part id before it and Stop cannot end that. Declined,
  // nothing runs. Run, then Stopped, the job says it is still working and
  // holds a job slot, rather than "running…", until poo ends it.
  {
    const partsJobs = () => page.evaluate(async () => (await (await fetch('/api/jobs')).json()).filter(j => j.args[0] === 'series' && j.args[1] === 'parts').length);
    const before = await partsJobs();
    await rel.locator('input[name=part]').fill('50000000000');
    let asked = '';
    page.once('dialog', d => { asked = d.message(); d.dismiss(); });
    await rel.locator('button[type=submit]').click();
    assert.match(asked, /^Part 50000000000 takes poo about 50000000000 hashes to work out.*Stop cannot end it.*job slots.*Run it\?$/);
    await page.waitForTimeout(300);
    assert.equal(await partsJobs(), before, 'a declined far part runs nothing');
    // Run in terminal on the same line asks the same (GUI/Red R28B).
    asked = '';
    page.once('dialog', d => { asked = d.message(); d.dismiss(); });
    await rel.locator('.cmd button', { hasText: 'Run in terminal' }).click();
    assert.match(asked, /^Part 50000000000 takes poo about/, 'Run in terminal asks about a far part too');
    await page.waitForTimeout(300);
    assert.ok(!page.url().includes('#/terminal'), 'declined, the terminal is not opened');
    assert.equal(await partsJobs(), before, 'and nothing runs');
    const showParts = section('Parts of a variable series');
    page.once('dialog', d => { asked = d.message(); d.dismiss(); });
    await showParts.locator('input[name=from]').fill('');
    await showParts.locator('input[name=to]').fill('200000000');
    await showParts.locator('button[type=submit]').click();
    assert.match(asked, /^Part 200000000 takes/, 'Show parts asks about a far To too');
    assert.equal(await partsJobs(), before);
    page.once('dialog', d => d.accept());
    await rel.locator('button[type=submit]').click();
    await rel.locator('.run-bar button', { hasText: 'Stop' }).click();
    // Until Source makes the part chain stop on cancel, it runs on; once
    // it does, the job ends at Stop. Either way the page says which.
    await rel.locator('.run-status.stop-pending, .run-status.bad').first().waitFor({ timeout: 10000 });
    const st = await rel.locator('.run-status').first().innerText();
    assert.match(st, /^Stop asked; poo is still working and will end on its own\. .*job slots|^exit /, 'Stop says what happened');
  }

  // Stop the node from the Node view.
  await page.click('nav a[data-id=node]');
  await page.click('button:has-text("Stop node")');
  await nodeStopped(page);

  assert.deepEqual(errors, [], 'no page errors');
  console.log('gui views e2e: ok');
} finally {
  await close();
}
