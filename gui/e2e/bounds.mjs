// End-to-end check of the Watch view's bounds (GUI-007): events held while
// paused, the error box and an unfinished line stay bounded however much a
// stream sends. The stream is synthetic, served in place of poon watch's,
// so the check needs no busy node.
//
//   node gui/e2e/bounds.mjs PATH/TO/poon-gui
import assert from 'node:assert/strict';
import { launch } from './lib.mjs';

const bin = process.argv[2];
const { page, errors, close } = await launch(bin);
const N = 1500;

try {
  // The watch job and its event stream are served here.
  await page.route('**/api/jobs', async route => {
    if (route.request().method() !== 'POST') return route.continue();
    route.fulfill({ status: 201, contentType: 'application/json', body: JSON.stringify({ id: 'fake', args: ['watch'], command: 'poon watch' }) });
  });
  let release;
  const released = new Promise(r => { release = r; });
  let served = false;
  await page.route('**/api/jobs/fake/events*', async route => {
    if (served) return route.fulfill({ status: 204 }); // ends the EventSource
    served = true;
    await released;
    const sse = (ev, data) => `event: ${ev}\ndata: ${JSON.stringify(data)}\n\n`;
    let body = sse('state', { waiting: false });
    let seq = 0;
    for (let i = 0; i < N; i++) {
      body += sse('chunk', { seq: seq++, stream: 'out', data: JSON.stringify({ seq: i, kind: 'tx', pool: 1, tid: 't' + i }) + '\n' });
    }
    body += sse('chunk', { seq: seq++, stream: 'err', data: 'e'.repeat(200 << 10) });
    route.fulfill({ status: 200, contentType: 'text/event-stream', body });
  });

  await page.click('nav a[data-id=watch]');
  const live = page.locator('section.panel', { has: page.locator('h2', { hasText: 'Live events' }) }).first();
  await live.locator('button:has-text("Start watching")').click();
  await live.locator('.run-status', { hasText: 'watching' }).waitFor();
  await live.locator('button:has-text("Pause")').click();
  release();
  await live.locator('button', { hasText: `Resume (${N - 1000} dropped)` }).waitFor();
  const errLen = await live.locator('pre.err').evaluate(e => e.textContent.length);
  assert.ok(errLen <= (64 << 10) + 100, `the error box keeps a bounded tail (${errLen})`);
  assert.equal(await live.locator('tr.ev').count(), 0, 'nothing shown while paused');
  await live.locator('button', { hasText: 'Resume' }).click();
  await live.locator('.run-status', { hasText: `${N - 1000} events dropped while paused` }).waitFor();
  assert.equal(await live.locator('tr.ev').count(), 1000, 'the table holds the newest 1000');
  assert.equal(await live.locator('tr.ev').first().locator('td').first().innerText(), String(N - 1), 'newest first');

  assert.deepEqual(errors, [], 'no page errors');
  console.log('gui bounds e2e: ok');
} finally {
  await close();
}
