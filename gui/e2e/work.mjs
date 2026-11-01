// End-to-end check of the Work view against a real node: the devices and
// jobs poo grind lists, a job run and followed live with its plan and
// rate, Stop ending it as an interrupt does (exit 0), and a reload taking
// a running job up again, and SIGHUP reloading the node it hosts.
//
//   node gui/e2e/work.mjs PATH/TO/poo-gui [SCREENSHOT_DIR]
import { execFileSync } from 'node:child_process';
import { join } from 'node:path';
import assert from 'node:assert/strict';
import { launch, nodeStopped } from './lib.mjs';

const bin = process.argv[2];
const shots = process.argv[3];
const { env, pid, page, errors, close } = await launch(bin);
const poo = (args, extra = {}) => execFileSync(bin, ['poo', ...args], { env: { ...env, ...extra } }).toString();
const section = title => page.locator('section.panel', { has: page.locator('h2', { hasText: title }) }).first();

try {
  poo(['init', '-example', '-listen', '127.0.0.1:0', '-p2p', 'off']);
  await page.click('nav a[data-id=node]');
  await page.click('button:has-text("Start node")');
  await page.locator('.live-out', { hasText: 'rpc: listening' }).waitFor();

  await page.click('nav a[data-id=work]');
  // Devices: the table poo grind devices prints, with the CPU in it.
  await section('Devices').locator('.result', { hasText: 'cpu' }).waitFor();

  // A pending transaction, ground into an entry by a job on "pending".
  poo(['claim', '-serial', '5'], { POO_SEED: 'a1'.repeat(32) });
  const run = section('Run a job');
  const target = run.locator('input[name=target]');
  const forIn = run.locator('label', { hasText: /^For/ }).locator('input');
  await target.fill('pending');
  await forIn.fill('3s');
  assert.match(await run.locator('.cmd code').innerText(), /^poo grind run -for 3s pending$/);
  await run.locator('button:has-text("Run and follow")').click();
  await run.locator('tbody tr', { hasText: 'entry' }).first().waitFor({ timeout: 20000 });
  await run.locator('.run-status', { hasText: '(exit 0)' }).waitFor({ timeout: 30000 });

  // Layers on the tip: the plan poo picked, a rate, then its end.
  await target.fill('tip');
  await forIn.fill('4s');
  assert.match(await run.locator('.cmd code').innerText(), /^poo grind run -for 4s tip$/);
  await run.locator('button:has-text("Run and follow")').click();
  await run.locator('.grind-plan', { hasText: 'device' }).waitFor({ timeout: 20000 });
  await run.locator('.rate-chart').waitFor({ timeout: 20000 });
  // The job's one start shows its spec beside the plans (D112).
  await run.locator('.grind-plan h3', { hasText: /^Job$/ }).waitFor();
  assert.match(await run.locator('.counts').innerText(), /H\/s/);
  // The run keeps going while another view is shown.
  await page.click('nav a[data-id=pools]');
  assert.ok(await page.locator('nav a[data-id=work].busy').count(), 'the Work tab shows the running job');
  await page.click('nav a[data-id=work]');
  await run.locator('.run-status', { hasText: '(exit 0)' }).waitFor({ timeout: 30000 });
  if (shots) await page.screenshot({ path: join(shots, 'work.png'), fullPage: true });

  // Without -for, Stop interrupts it: poo stops the node's job and exits 0.
  await forIn.fill('');
  await run.locator('button:has-text("Run and follow")').click();
  await run.locator('.grind-plan', { hasText: 'device' }).waitFor({ timeout: 20000 });
  // A reload takes the running job up again with its Stop (GUI-010): Run
  // stays off, so a second grind cannot start beside it.
  await page.reload();
  await page.click('nav a[data-id=work]');
  await run.locator('button:has-text("Stop"):enabled').waitFor({ timeout: 20000 });
  await run.locator('.grind-plan', { hasText: 'device' }).waitFor({ timeout: 20000 });
  assert.ok(await run.locator('button:has-text("Run and follow")').isDisabled(), 'Run is off while the job runs');
  assert.ok(await page.locator('nav a[data-id=work].busy').count(), 'the Work tab shows the adopted job');
  await run.locator('button:has-text("Stop")').click();
  await run.locator('.run-status', { hasText: '(exit 0)' }).waitFor({ timeout: 30000 });

  // The node lists both jobs as ended.
  const jobs = section('Jobs');
  await jobs.locator('button:has-text("Refresh")').click();
  await jobs.locator('table.grind-jobs tr', { hasText: 'stopped' }).first().waitFor();

  // A job left to the node shows as a row with Stop, which runs
  // poo grind stop ID; the rates read as H/s, as in the live counter.
  const bg = section('Start a job in the background');
  await bg.locator('button[type=submit]').click();
  await bg.locator('.run-status.ok').waitFor();
  await jobs.locator('button:has-text("Refresh")').click();
  const going = jobs.locator('table.grind-jobs tr', { has: page.locator('button', { hasText: 'Stop' }) });
  await going.first().waitFor();
  assert.match(await jobs.locator('table.grind-jobs').innerText(), /H\/s/);
  await going.first().locator('button', { hasText: 'Stop' }).click();
  for (const end = Date.now() + 20000; await going.count();) {
    assert.ok(Date.now() < end, 'the stopped job lost its Stop button');
    await page.waitForTimeout(100);
  }
  // Device fields offer the ids poo grind devices printed, and auto.
  assert.equal(await run.locator('input[name=device]').getAttribute('list'), 'grind-devices');
  const ids = await page.locator('#grind-devices option').evaluateAll(os => os.map(o => o.value));
  assert.ok(ids.includes('auto') && ids.includes('cpu'), `device choices ${ids}`);
  if (shots) await page.screenshot({ path: join(shots, 'work-jobs.png'), fullPage: true });

  // The schedule (GREEN-026): a job added through poo grind schedule add
  // shows as a row, the running node starts it at once, and Remove drops
  // it again with its job.
  const sched = section('Schedule');
  // A long view lists its panels at the top (GREEN-030); a title scrolls
  // to its panel and leaves the hash, the router, alone.
  const hash = await page.evaluate(() => location.hash);
  await page.locator('nav.jump button', { hasText: /^Schedule$/ }).click();
  await page.waitForFunction(() => {
    const h = [...document.querySelectorAll('section.panel h2')].find(h => h.textContent === 'Schedule');
    return h && Math.abs(h.closest('section').getBoundingClientRect().top) < 40 && document.activeElement === h;
  });
  assert.equal(await page.evaluate(() => location.hash), hash, 'the jump list leaves the hash alone');
  const add = section('Schedule a job');
  assert.ok(!(await add.locator('label', { hasText: /^For/ }).count()), 'a scheduled job has no For');
  // Until from reaches is only for an entry or tx: target (GREEN-032).
  const until = add.locator('input[name=untilFrom]');
  assert.ok(!(await until.isVisible()), 'tip takes no until-from');
  await add.locator('input[name=target]').fill('tx:' + '00'.repeat(32));
  assert.ok(await until.isVisible(), 'a tx: target shows until-from');
  await until.fill('5');
  assert.match(await add.locator('.cmd code').innerText(), /-until-from 5/);
  await add.locator('input[name=target]').fill('tip');
  assert.ok(!(await until.isVisible()) && !/until-from/.test(await add.locator('.cmd code').innerText()), 'hidden until-from is not sent');
  assert.ok(!(await section('Run a job').locator('input[name=untilFrom]').isVisible()), 'the run form hides until-from for tip');
  await add.locator('input[name=hours]').fill('0-23');
  assert.match(await add.locator('.cmd code').innerText(), /^poo grind schedule add tip -hours 0-23$/);
  await add.locator('button[type=submit]').click();
  await add.locator('.run-status.ok').waitFor();
  const row = sched.locator('table.grind-schedule tbody tr', { hasText: '0-23' });
  await row.waitFor({ timeout: 10000 });
  await jobs.locator('button:has-text("Refresh")').click();
  await going.first().waitFor({ timeout: 10000 });
  const defs = section('Schedule defaults');
  await defs.locator('input[name=threads]').fill('1');
  await defs.locator('button[type=submit]').click();
  await sched.locator('.counts', { hasText: 'device auto, 1 thread.' }).waitFor({ timeout: 10000 });
  // Each default field shows what is set now (GREEN-033).
  await defs.locator('input[name=threads][placeholder^="now 1 (up to "]').waitFor();
  // The ranges come from poo's limits in the same reply (GUI-015).
  assert.match(await defs.locator('input[name=rate]').getAttribute('placeholder'), /\(up to 50\)$/);
  assert.equal(await defs.locator('input[name=device]').getAttribute('placeholder'), 'now auto');
  // The layer floor and rate (D122) are schedule defaults too, and a
  // floor of 0 turns them off again.
  await defs.locator('input[name=floor]').fill('200');
  await defs.locator('input[name=rate]').fill('5');
  assert.match(await defs.locator('.cmd code').innerText(), /-layer-floor 200 -layer-rate 5$/);
  await defs.locator('button[type=submit]').click();
  await sched.locator('.counts', { hasText: 'strength 200 or more, at most 5 a second' }).waitFor({ timeout: 10000 });
  // A rate past what peers take is poo's to refuse (GUI-015, D127): the
  // field says why, the form shows poo's error with the range, and the
  // schedule keeps the rate it had.
  assert.match(await defs.locator('label', { hasText: 'Layer rate' }).locator('small').innerText(), /time out and then block/);
  await defs.locator('input[name=rate]').fill('100000');
  await defs.locator('button[type=submit]').click();
  await defs.locator('.run-status.bad').waitFor({ timeout: 10000 });
  assert.match(await defs.locator('.area').innerText(), /layer_rate must be 0 \(the default, \d+\) to \d+/);
  await sched.locator('.counts', { hasText: 'strength 200 or more, at most 5 a second' }).waitFor({ timeout: 10000 });
  await defs.locator('input[name=floor]').fill('0');
  await defs.locator('input[name=rate]').fill('');
  await defs.locator('button[type=submit]').click();
  // The rate stays set with no floor, and the line says so (GREEN-035).
  await sched.locator('.counts', { hasText: /^Defaults: device auto, 1 thread, no layer floor \(layer rate 5 kept for when one is set\)\.$/ }).waitFor({ timeout: 10000 });
  await defs.locator('input[name=rate]').fill('0');
  await defs.locator('button[type=submit]').click();
  await sched.locator('.counts', { hasText: /^Defaults: device auto, 1 thread\.$/ }).waitFor({ timeout: 10000 });
  // Two quick Removes (GUI-013): the first turns every Remove off until
  // the table is drawn again, so the second cannot go by a shifted number.
  for (const t of ['pending', 'tip']) {
    await add.locator('input[name=target]').fill(t);
    await add.locator('input[name=hours]').fill(t === 'pending' ? '1-2' : '3-4');
    await add.locator('button[type=submit]').click();
    await add.locator('.run-status.ok').waitFor();
    await sched.locator('table.grind-schedule tbody tr', { hasText: t === 'pending' ? '1-2' : '3-4' }).waitFor({ timeout: 10000 });
  }
  const removes = sched.locator('table.grind-schedule tbody button', { hasText: 'Remove' });
  await sched.locator('table.grind-schedule tbody tr').nth(2).waitFor();
  // The click and the checks run in one go in the page, so the Remove
  // cannot finish first: while it runs every Remove is off, and so are Add
  // and Set (GUI-014), since a change submitted now would start from the
  // schedule the Remove is replacing. A submit by other means is dropped.
  await add.locator('input[name=hours]').fill('5-6');
  const during = await page.evaluate(() => {
    const sec = t => [...document.querySelectorAll('section.panel')].find(s => s.querySelector('h2')?.textContent === t);
    const rm = [...sec('Schedule').querySelectorAll('table.grind-schedule tbody button')];
    rm[0].click();
    const addSec = sec('Schedule a job');
    const out = { remove: rm[1].disabled, add: addSec.querySelector('.actions button').disabled, set: sec('Schedule defaults').querySelector('.actions button').disabled };
    const before = addSec.querySelector('.area').firstChild;
    addSec.querySelector('form.fields').requestSubmit();
    out.addRan = addSec.querySelector('.area').firstChild !== before;
    return out;
  });
  assert.deepEqual(during, { remove: true, add: true, set: true, addRan: false }, 'one schedule change at a time');
  await sched.locator('table.grind-schedule tbody tr', { hasText: '0-23' }).waitFor({ state: 'detached', timeout: 10000 });
  await sched.locator('table.grind-schedule tbody tr', { hasText: '1-2' }).locator('button', { hasText: 'Remove' }).click();
  await sched.locator('table.grind-schedule tbody tr', { hasText: '1-2' }).waitFor({ state: 'detached', timeout: 10000 });
  assert.equal(await sched.locator('table.grind-schedule tbody tr').count(), 1, 'one job left, and the dropped Add added none');
  // Once the table is drawn again, Add works, and its job is in the table
  // with the others' changes kept.
  assert.ok(!(await add.locator('.actions button[type=submit]').isDisabled()), 'Add is on again');
  await add.locator('button[type=submit]').click();
  await sched.locator('table.grind-schedule tbody tr', { hasText: '5-6' }).waitFor({ timeout: 10000 });
  assert.equal(await sched.locator('table.grind-schedule tbody tr').count(), 2, 'the added job and the one left');
  await sched.locator('table.grind-schedule tbody tr', { hasText: '5-6' }).locator('button', { hasText: 'Remove' }).click();
  await sched.locator('table.grind-schedule tbody tr', { hasText: '5-6' }).waitFor({ state: 'detached', timeout: 10000 });
  await sched.locator('table.grind-schedule tbody tr', { hasText: '3-4' }).waitFor();
  // A stale row (GREEN-031): the schedule changes outside this page, so
  // the tip row's number 1 now names a pending job. Remove sends -target
  // tip, poo refuses, and the table is read again with the pending job
  // kept.
  poo(['grind', 'schedule', 'add', 'pending', '-hours', '5-6']);
  poo(['grind', 'schedule', 'remove', '1']);
  await sched.locator('table.grind-schedule tbody tr', { hasText: '3-4' }).locator('button', { hasText: 'Remove' }).click();
  await sched.locator('table.grind-schedule tbody tr', { hasText: '5-6' }).waitFor({ timeout: 10000 });
  assert.equal(await sched.locator('table.grind-schedule tbody tr').count(), 1, 'the stale Remove removed nothing');
  await sched.locator('table.grind-schedule tbody tr', { hasText: '5-6' }).locator('button', { hasText: 'Remove' }).click();
  await sched.locator('.empty', { hasText: 'No scheduled jobs' }).waitFor({ timeout: 10000 });
  for (const end = Date.now() + 20000; ;) {
    await jobs.locator('button:has-text("Refresh")').click();
    await page.waitForTimeout(300);
    if (!(await going.count())) break;
    assert.ok(Date.now() < end, 'the removed schedule\'s job stopped');
  }
  if (shots) await page.screenshot({ path: join(shots, 'work-schedule.png'), fullPage: true });

  // Without a terminal SIGHUP is a reload (GUI-011): the node the GUI
  // hosts reloads its node policy, and poo-gui keeps running.
  await page.click('nav a[data-id=node]');
  process.kill(pid, 'SIGHUP');
  await page.locator('.live-out', { hasText: 'reloading its node policy' }).waitFor({ timeout: 20000 });
  await page.locator('.run-status.live', { hasText: 'running: poo node' }).waitFor();
  await page.click('nav a[data-id=work]');
  await section('Devices').locator('.result', { hasText: 'cpu' }).waitFor();
  await page.click('nav a[data-id=node]');
  await page.click('button:has-text("Stop node")');
  await nodeStopped(page);
  assert.deepEqual(errors, [], 'no page errors');
  console.log('gui work e2e: ok');
} finally {
  await close();
}
