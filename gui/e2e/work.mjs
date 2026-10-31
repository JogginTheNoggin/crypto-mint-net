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
  await row.locator('button', { hasText: 'Remove' }).click();
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
