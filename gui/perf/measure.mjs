// Measures the GUI page under load against perfgui (a GUI whose watch and
// node are simulated; see perfgui/main.go), and prints one JSON object of
// numbers (docs/proposals/gui/ORANGE-001).
//
//   go -C gui build -o /tmp/perfgui ./perf/perfgui
//   NODE_PATH=$(npm root -g) node gui/perf/measure.mjs /tmp/perfgui [scenario ...]
//
// Scenarios: load, watch, terminal, node, refresh, jobs, command, render,
// paused, work. None
// given runs them all.
import { spawn } from 'node:child_process';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const { chromium } = require('playwright');

const bin = process.argv[2];
const only = process.argv.slice(3);
const want = s => only.length === 0 || only.includes(s);

async function launch(flags = []) {
  const p = spawn(bin, flags, { stdio: ['ignore', 'pipe', 'inherit'] });
  const url = await new Promise((resolve, reject) => {
    let buf = '';
    p.stdout.on('data', d => { buf += d; const m = buf.match(/open (http\S+)/); if (m) resolve(m[1]); });
    p.on('exit', c => reject(new Error(`perfgui exited ${c}`)));
  });
  const browser = await chromium.launch();
  const page = await browser.newPage({ bypassCSP: !!process.env.CSS });
  const cdp = await page.context().newCDPSession(page);
  await cdp.send('Performance.enable');
  const requests = [];
  page.on('request', r => requests.push({ url: r.url(), method: r.method(), t: Date.now() }));
  await page.goto(url);
  await page.locator('main h1').first().waitFor();
  const metrics = async () => Object.fromEntries((await cdp.send('Performance.getMetrics')).metrics.map(m => [m.name, m.value]));
  const close = async () => { await browser.close(); p.kill('SIGINT'); };
  return { page, cdp, metrics, requests, close, home: new URL(url).origin + '/' };
}

const mb = b => Math.round(b / 1048576 * 10) / 10;
const pick = (m) => ({ heapMB: mb(m.JSHeapUsedSize), nodes: m.Nodes, layouts: m.LayoutCount, styleRecalcs: m.RecalcStyleCount, scriptS: +m.ScriptDuration.toFixed(2), taskS: +m.TaskDuration.toFixed(2) });
const diff = (a, b) => ({ heapMB: +(b.heapMB - a.heapMB).toFixed(1), nodes: b.nodes - a.nodes, layouts: b.layouts - a.layouts, styleRecalcs: b.styleRecalcs - a.styleRecalcs, scriptS: +(b.scriptS - a.scriptS).toFixed(2), taskS: +(b.taskS - a.taskS).toFixed(2) });

// frames counts animation frames and the longest gap between two, while
// fn runs, to show whether the page stayed responsive.
async function frames(page, fn) {
  await page.evaluate(() => {
    window.__f = { n: 0, max: 0, last: performance.now(), on: true, long: 0, longMs: 0 };
    const tick = t => { const f = window.__f; if (!f.on) return; f.n++; f.max = Math.max(f.max, t - f.last); f.last = t; requestAnimationFrame(tick); };
    requestAnimationFrame(tick);
    new PerformanceObserver(l => { for (const e of l.getEntries()) { window.__f.long++; window.__f.longMs += e.duration; } }).observe({ type: 'longtask' });
  });
  const t0 = Date.now();
  await fn();
  const ms = Date.now() - t0;
  const f = await page.evaluate(() => { window.__f.on = false; return window.__f; });
  return { ms, fps: Math.round(f.n / (ms / 1000)), maxFrameGapMs: Math.round(f.max), longTasks: f.long, longTaskMs: Math.round(f.longMs) };
}

const until = async (page, cond, ms) => {
  for (const end = Date.now() + ms; Date.now() < end;) {
    if (await cond()) return true;
    await page.waitForTimeout(25);
  }
  return false;
};

const out = new Proxy({}, { set(o, k, v) { o[k] = v; console.error(`## ${k} ${JSON.stringify(v)}`); return true; } });

if (want('load')) {
  const g = await launch();
  const runs = [];
  for (let i = 0; i < 5; i++) {
    await g.page.goto(g.home);
    await g.page.locator('main h1').first().waitFor();
    runs.push(await g.page.evaluate(() => {
      const n = performance.getEntriesByType('navigation')[0];
      const res = performance.getEntriesByType('resource');
      return { domContentLoadedMs: Math.round(n.domContentLoadedEventEnd), loadMs: Math.round(n.loadEventEnd), requests: res.length + 1,
        bytes: res.reduce((a, r) => a + r.transferSize, n.transferSize), firstViewMs: Math.round(performance.now()) };
    }));
  }
  out.load = { median: Object.fromEntries(Object.keys(runs[0]).map(k => [k, runs.map(r => r[k]).sort((a, b) => a - b)[2]])), runs };
  await g.close();
}

// watchAt runs one watch of n events at rate and measures the page.
async function watchAt(rate, n) {
  const g = await launch(['-rate', String(rate), '-events', String(n)]);
  await g.page.click('nav a[data-id=watch]');
  // CSS adds a style sheet, to try a layout change without a rebuild.
  if (process.env.CSS) await g.page.addStyleTag({ content: process.env.CSS });
  const before = pick(await g.metrics());
  const total = () => g.page.evaluate(() => {
    const t = document.querySelector('.counts')?.textContent || '';
    return [...t.matchAll(/(\w+) (\d+)/g)].reduce((a, m) => a + Number(m[2]), 0);
  });
  let done = false;
  const f = await frames(g.page, async () => {
    await g.page.click('button:has-text("Start watching")');
    done = await until(g.page, async () => (await total()) >= n, 120000);
  });
  const after = pick(await g.metrics());
  const rows = await g.page.evaluate(() => document.querySelector('table.events tbody').children.length);
  // A click on a row while the stream is quiet: how long until it shows.
  const clickMs = await g.page.evaluate(async () => {
    const t = performance.now();
    document.querySelector('tbody tr').click();
    await new Promise(r => requestAnimationFrame(() => setTimeout(r)));
    return Math.round(performance.now() - t);
  });
  await g.close();
  return { rate: rate || 'max', events: n, allShown: done, shownPerS: Math.round(n / (f.ms / 1000)), ...f, rows, clickMs, cost: diff(before, after), after };
}

if (want('watch')) {
  out.watch = [];
  for (const [rate, n] of (process.env.RATES ? JSON.parse(process.env.RATES) : [[200, 4000], [1000, 10000], [5000, 20000], [0, 20000]])) out.watch.push(await watchAt(rate, n));
}

if (want('terminal')) {
  // poo watch typed in the terminal: every line is kept as a span.
  const n = 20000;
  const g = await launch(['-rate', '0', '-events', String(n)]);
  await g.page.click('nav a[data-id=terminal]');
  const before = pick(await g.metrics());
  const f = await frames(g.page, async () => {
    await g.page.fill('#term-line', 'watch');
    await g.page.keyboard.press('Enter');
    await until(g.page, async () => g.page.evaluate(n => (document.querySelector('.term-out')?.lastChild?.textContent || '').includes(`"seq":${n},`), n), 120000);
  });
  const after = pick(await g.metrics());
  out.terminal = { events: n, ...f, spans: await g.page.evaluate(() => document.querySelector('.term-out').childElementCount), chars: await g.page.evaluate(() => document.querySelector('.term-out').textContent.length), cost: diff(before, after), after };
  await g.close();
}

if (want('node')) {
  // A node logging -log lines a second (default 50) for 60 s on the Node
  // view. responseMs is how long the page took to answer a question:
  // a busy page answers late.
  const rate = Number(process.env.LOG_RATE || 50);
  const g = await launch(['-log', String(rate)]);
  await g.page.click('nav a[data-id=node]');
  await g.page.click('button:has-text("Start node")');
  await g.page.locator('.live-out', { hasText: 'pools followed' }).waitFor();
  const before = pick(await g.metrics());
  const samples = [];
  const t0 = Date.now();
  while (Date.now() - t0 < 60000) {
    await new Promise(r => setTimeout(r, 10000));
    const q = Date.now();
    const log = await g.page.evaluate(() => { const l = document.querySelector('.live-out'); return { spans: l?.childElementCount, chars: l?.textContent.length }; });
    samples.push({ s: Math.round((Date.now() - t0) / 1000), responseMs: Date.now() - q, ...pick(await g.metrics()), log });
  }
  out.node = { linesPerS: rate, cost: diff(before, samples.at(-1)), samples };
  await g.close();
}

if (want('refresh') || want('jobs')) {
  // Requests and jobs while views sit open with no node answering.
  const g = await launch();
  const per = {};
  for (const v of ['node', 'start', 'pools', 'peers']) {
    await g.page.click(`nav a[data-id=${v}]`).catch(() => {});
    const from = g.requests.length;
    await g.page.waitForTimeout(30000);
    const rs = g.requests.slice(from);
    per[v] = { requestsPerMin: rs.length * 2, jobsPerMin: rs.filter(r => r.method === 'POST' && r.url.endsWith('/api/jobs')).length * 2 };
  }
  const jobs = await g.page.evaluate(async () => { const r = await fetch('/api/jobs'); const t = await r.text(); return { bytes: t.length, kept: JSON.parse(t).length, refreshKept: JSON.parse(t).filter(j => j.refresh).length }; });
  await g.page.click('nav a[data-id=jobs]');
  const drawMs = await g.page.evaluate(async () => {
    await new Promise(r => setTimeout(r, 500));
    const t = performance.now();
    document.querySelector('.job-tools input[type=checkbox]').click();
    document.querySelector('.job-tools input[type=checkbox]').click();
    return +((performance.now() - t) / 2).toFixed(2);
  });
  out.refresh = per;
  out.jobs = { ...jobs, drawMs, nodes: (await g.metrics()).Nodes };
  await g.close();
}

if (want('command')) {
  // One short real command (poo crypto) through each path the page uses.
  const g = await launch();
  out.command = await g.page.evaluate(async () => {
    const H = { 'Content-Type': 'application/json', 'X-Poo-GUI': '1' };
    const time = async f => { const ts = []; for (let i = 0; i < 30; i++) { const t = performance.now(); await f(); ts.push(performance.now() - t); } ts.sort((a, b) => a - b); return { medianMs: +ts[15].toFixed(2), p90Ms: +ts[27].toFixed(2) }; };
    const run = () => fetch('/api/run', { method: 'POST', headers: H, body: JSON.stringify({ args: ['crypto'] }) }).then(r => r.json());
    const job = async () => {
      const j = await (await fetch('/api/jobs', { method: 'POST', headers: H, body: JSON.stringify({ args: ['crypto'] }) })).json();
      await new Promise(res => { const es = new EventSource(`/api/jobs/${j.id}/events?from=0`); es.addEventListener('exit', () => { es.close(); res(); }); });
    };
    const list = () => fetch('/api/jobs').then(r => r.json());
    return { run: await time(run), jobAndStream: await time(job), listJobs: await time(list) };
  });
  await g.close();
}

if (want('render')) {
  // A finished command's result drawn by runJob, as a panel or form shows
  // it: a JSON list rendered as a table, or a long text result.
  out.render = [];
  for (const [cmd, n] of [['bigjson', 500], ['bigjson', 5000], ['big', 50000]]) {
    const g = await launch(['-events', String(n)]);
    const before = pick(await g.metrics());
    const r = await g.page.evaluate(async cmd => {
      const { runJob } = await import('./views.js');
      const area = document.createElement('div');
      document.querySelector('main').append(area);
      const t0 = performance.now();
      const res = await runJob([cmd], area);
      const ran = performance.now() - t0;
      await new Promise(r => requestAnimationFrame(() => setTimeout(r)));
      return { code: res.code, outBytes: res.out.length, ranMs: Math.round(ran), shownMs: Math.round(performance.now() - t0), nodes: area.querySelectorAll('*').length };
    }, cmd);
    out.render.push({ cmd, n, ...r, cost: diff(before, pick(await g.metrics())) });
    await g.close();
  }
}

if (want('paused')) {
  // Watch paused before the stream starts, at full rate: every event is
  // held, and the oldest let go past MAX_EVENTS.
  const n = 20000;
  const g = await launch(['-rate', '0', '-events', String(n)]);
  await g.page.click('nav a[data-id=watch]');
  const before = pick(await g.metrics());
  const f0 = {};
  const f = await frames(g.page, async () => {
    await g.page.click('button:has-text("Start watching")');
    await g.page.click('button:has-text("Pause")');
    // Done when the dropped count has stopped moving for a second.
    let last = '', since = Date.now();
    await until(g.page, async () => {
      const t = await g.page.locator('button', { hasText: 'Resume' }).innerText();
      if (t !== last) { last = t; since = Date.now(); }
      return t.includes('dropped') && Date.now() - since > 1000;
    }, 120000);
    f0.dropped = last;
  });
  const resume = await frames(g.page, async () => {
    await g.page.click('button:has-text("Resume")');
    await g.page.waitForTimeout(200);
  });
  out.paused = { events: n, ...f, ...f0, msIncludesOneSecondSettle: true, cost: diff(before, pick(await g.metrics())), resumeMaxFrameGapMs: resume.maxFrameGapMs };
  await g.close();
}

if (want('work')) {
  // The Work view's live run of poo grind run, at rising rates of
  // progress lines (a found line before every 4th).
  out.work = [];
  for (const [rate, n] of (process.env.WORK_RATES ? JSON.parse(process.env.WORK_RATES) : [[2, 120], [200, 4000], [2000, 20000], [0, 20000]])) {
    const g = await launch(['-rate', String(rate), '-events', String(n)]);
    await g.page.click('nav a[data-id=work]');
    const before = pick(await g.metrics());
    const doneAt = () => g.page.evaluate(n => (document.querySelector('.chart-wrap svg')?.getAttribute('aria-label') || '').length > 0 &&
      !!document.querySelector('.counts') && document.querySelector('.counts').textContent.includes(`${(n - 1) * 1000} tries`), n);
    let done = false;
    const f = await frames(g.page, async () => {
      await g.page.click('button:has-text("Run and follow")');
      done = await until(g.page, doneAt, 180000);
    });
    out.work.push({ rate: rate || 'max', events: n, allShown: done, ...f, cost: diff(before, pick(await g.metrics())), foundRows: await g.page.evaluate(() => [...document.querySelectorAll('section.panel')].find(p => p.querySelector('h2')?.textContent === 'Run a job')?.querySelector('tbody')?.children.length) });
    await g.close();
  }
}

console.log(JSON.stringify(out, null, 2));
