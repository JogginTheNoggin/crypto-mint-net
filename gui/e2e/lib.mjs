// Shared set-up for the end-to-end checks: start poo-gui in a fresh
// POO_HOME and open it in Chromium.
import { spawn } from 'node:child_process';
import { mkdtempSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const { chromium } = require('playwright');

// flags are extra poo-gui flags, such as -view-only.
export async function launch(bin, flags = []) {
  const home = mkdtempSync(join(tmpdir(), 'poo-gui-e2e-'));
  const env = { ...process.env, POO_HOME: home };
  delete env.POO_PASSPHRASE;
  const gui = spawn(bin, ['-no-open', ...flags], { env, stdio: ['ignore', 'pipe', 'inherit'] });
  const url = await new Promise((resolve, reject) => {
    let buf = '';
    gui.stdout.on('data', d => {
      buf += d;
      const m = buf.match(/open (http:\/\/127\.0\.0\.1:\d+\/\?t=[0-9a-f]+)/);
      if (m) resolve(m[1]);
    });
    gui.on('exit', c => reject(new Error(`poo-gui exited ${c}`)));
    setTimeout(() => reject(new Error('no address printed')), 20000);
  });
  const browser = await chromium.launch();
  const page = await browser.newPage();
  const errors = [];
  page.on('pageerror', e => errors.push(e.message));
  page.on('console', m => { if (m.type() === 'error') errors.push(m.text()); });
  await page.goto(url);
  const close = async () => { await browser.close(); gui.kill('SIGINT'); };
  return { home, env, url, browser, page, errors, close };
}

// nodeStopped waits for the Node view to report that the node it started
// has stopped, and requires exit 0. One known exception is let through:
// when an RPC request is still in its handler, the headless node's
// 5-second shutdown times out and poo node exits 1 with "context deadline
// exceeded" although it stopped (GREEN-015 item 3, with Source/Grey).
// Anything else fails, with the node's output in the message.
export async function nodeStopped(page) {
  const area = page.locator('section.panel', { has: page.locator('h2', { hasText: 'Run the node' }) }).locator('.area');
  const status = area.locator('.run-status', { hasText: 'node stopped' });
  try {
    await status.waitFor({ timeout: 45000 });
  } catch (e) {
    console.error('node view at timeout:\n' + await area.innerText().catch(() => '?'));
    throw e;
  }
  const text = await status.innerText();
  if (text === 'node stopped (exit 0)') return;
  const out = await area.innerText();
  if (text === 'node stopped (exit 1)' && out.includes('context deadline exceeded')) {
    console.error('note: poo node exited 1 on a shutdown timeout (known headless issue, GREEN-015 item 3)');
    return;
  }
  throw new Error(`${text}; node output:\n${out}`);
}
