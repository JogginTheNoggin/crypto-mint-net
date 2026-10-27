// Shared set-up for the end-to-end checks: start poo-gui in a fresh
// POO_HOME and open it in Chromium.
import { spawn } from 'node:child_process';
import { mkdtempSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const { chromium } = require('playwright');

export async function launch(bin) {
  const home = mkdtempSync(join(tmpdir(), 'poo-gui-e2e-'));
  const env = { ...process.env, POO_HOME: home };
  delete env.POO_PASSPHRASE;
  const gui = spawn(bin, ['-no-open'], { env, stdio: ['ignore', 'pipe', 'inherit'] });
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
