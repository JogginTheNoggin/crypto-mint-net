// End-to-end check of the GUI's views in a real browser: each runs its
// poo command and renders the output; forms build the command they show.
//
//   node gui/e2e/views.mjs PATH/TO/poo-gui [SCREENSHOT_DIR]
import { join } from 'node:path';
import { readFileSync, writeFileSync } from 'node:fs';
import { randomBytes } from 'node:crypto';
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
  await page.locator('.live-out', { hasText: 'rpc: listening' }).waitFor();
  const pools = section('Followed pools');
  for (let i = 0; !(await pools.locator('table.data, dl.kv').count()); i++) {
    assert.ok(i < 40, 'status never rendered');
    await pools.locator('button:has-text("Refresh")').click();
    await page.waitForTimeout(250);
  }
  await shot('node');
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

  // Peers, policies and crypto render their commands' output.
  await page.click('nav a[data-id=peers]');
  await section('Peers').locator('.run-status').first().waitFor();
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
  await imp.locator('button[type=submit]').click();
  await imp.locator('.import-plan', { hasText: 'Nothing changed yet' }).waitFor();
  assert.match(await imp.locator('.import-plan').innerText(), /already the same/);
  page.once('dialog', d => d.accept());
  await imp.locator('button.danger:has-text("Apply")').click();
  await imp.locator('.area .area .import-plan', { hasText: 'Nothing needed changing.' }).waitFor();
  // A file of every policy, renamed: each goes in under its own name, so
  // only -root is sent, and the plan says a new policy is made.
  const every = join(home, 'every-policy.json');
  writeFileSync(every, (await dl.path().then(f => readFileSync(f, 'utf8'))).replace('"name": "default"', '"name": "copy"').replace('"current": true', '"current": false'));
  await fill(imp, 'File', every);
  await imp.locator('label', { hasText: 'holds every saved policy' }).locator('input').check();
  assert.equal(await imp.locator('.cmd code').innerText(), `poo policy import -root ${home} -json ${every}`);
  await imp.locator('button[type=submit]').click();
  await imp.locator('.import-plan', { hasText: 'A new node policy.' }).waitFor();
  page.once('dialog', d => d.accept());
  await imp.locator('button.danger:has-text("Apply")').click();
  await imp.locator('.area .area .import-plan', { hasText: 'Saved.' }).waitFor();
  await section('Policies').locator('button:has-text("Refresh")').click();
  await section('Policies').locator('.result', { hasText: 'copy' }).waitFor();
  // A file picked in the browser is sent on poo's input: FILE is -.
  await imp.locator('label', { hasText: 'holds every saved policy' }).locator('input').uncheck();
  await imp.locator('input[type=file]').setInputFiles(bundle);
  await imp.locator('.cmd code', { hasText: / -json -$/ }).waitFor();
  assert.equal(await imp.locator('.cmd code').innerText(), `poo policy import -root ${home} -json -`);
  await imp.locator('button[type=submit]').click();
  await imp.locator('.import-plan', { hasText: 'already the same' }).waitFor();
  // GUI-023: Apply runs what was previewed, and only that. Picking
  // another file, or changing an option, after a preview takes Apply
  // away, and Apply sends the text that was previewed.
  const peer = '203.0.113.7:7000';
  const fileB = join(home, 'with-peer.json');
  const b = JSON.parse(readFileSync(bundle, 'utf8'));
  b.policies[0].policy.p2p = { ...(b.policies[0].policy.p2p || {}), peers: [peer] };
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
  // A's preview again, then Apply: node.json does not get B's peer.
  await imp.locator('input[type=file]').setInputFiles(bundle);
  await imp.locator('button[type=submit]').click();
  await imp.locator('.import-plan', { hasText: 'already the same' }).waitFor();
  let asked = '';
  page.once('dialog', d => { asked = d.message(); d.accept(); });
  await applyBtn.click();
  await imp.locator('.area .area .import-plan', { hasText: 'Nothing needed changing.' }).waitFor();
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
  const noDigest = async route => {
    const res = await route.fetch();
    const body = (await res.text()).replace(/(\\"digest\\": *\\")[0-9a-f]{64}/g, '$1');
    await route.fulfill({ response: res, body });
  };
  await fill(imp, 'File', bundle);
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
  const simp = section('Import node settings');
  await fill(simp, 'File', sfile);
  await fill(simp, 'Registry paths', '/old/home=/new/home');
  assert.equal(await simp.locator('.cmd code').innerText(), `poo settings import -root ${home} -map-path /old/home=/new/home -json ${sfile}`);
  await simp.locator('button[type=submit]').click();
  await simp.locator('.import-plan', { hasText: 'already the same' }).waitFor();
  let sasked = '';
  page.once('dialog', d => { sasked = d.message(); d.accept(); });
  await simp.locator('button.danger:has-text("Apply")').click();
  await simp.locator('.area .area .import-plan', { hasText: 'Nothing needed changing.' }).waitFor();
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
  // The list is drawn on the fold's toggle event, which comes after the
  // click: wait for it before reading.
  await sdef.locator('details p').waitFor();
  for (const pr of peers30) assert.ok((await sdef.locator('details').innerText()).includes(pr), `Show all lists ${pr}`);
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
