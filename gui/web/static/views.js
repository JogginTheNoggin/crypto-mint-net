// Views. Each panel runs one poo command and renders what it prints; each
// form builds one poo command line from its fields and runs it. Nothing
// here computes anything a command does not print, and every panel shows
// its command line, which runs the same in the terminal (docs/gui.md).
import * as api from './api.js';
import { render, secretValue } from './render.js';
import * as where from './where.js';
import { hintFor } from './hints.js';
import { scrollback, MAX_CHARS } from './scroll.js';
// The watch and explore views live in watch.js; views lists every view so
// the order of the navigation is in one place.
import { watchView, exploreView } from './watch.js';
import { workView } from './work.js';

export function el(tag, attrs = {}, ...kids) {
  const e = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) {
    if (v === undefined || v === null || v === false) continue;
    if (k === 'class') e.className = v;
    else if (k.startsWith('on')) e.addEventListener(k.slice(2), v);
    else e.setAttribute(k, v === true ? '' : v);
  }
  e.append(...kids.filter(k => k !== null && k !== undefined && k !== false));
  return e;
}

// runJob runs args as a job and shows it in area: output as it arrives,
// an input field whenever the command reads input (masked when it is a
// passphrase or seed), and the result rendered once it ends. It resolves to
// {code, out, err}. secretOutput leaves out the Copy output button, for a
// command whose output holds a secret (keygen prints its seed).
export function runJob(args, area, { onStart, empty, refresh, secretOutput } = {}) {
  // A job still followed in this area gives up its stream before its
  // output is cleared away, as leaving the view does (GUI-012).
  if (area.unfollow) { area.unfollow(); area.unfollow = null; }
  area.textContent = '';
  const status = el('div', { class: 'run-status live' }, 'running…');
  const live = el('pre', { class: 'live-out' });
  const liveOut = scrollback(live);
  const ask = el('form', { class: 'ask', hidden: true });
  const askLabel = el('label', {});
  // A passphrase or seed is masked: the job's state says when poo reads
  // one (cli.Stdio.Secret, decision D101), never a guess from the output.
  const askInput = el('input', { type: 'password', autocomplete: 'off', spellcheck: 'false' });
  const showBtn = el('button', { type: 'button', class: 'small', 'aria-pressed': 'false', title: 'Show what you type' }, 'Show');
  showBtn.onclick = () => {
    const show = askInput.type === 'password';
    askInput.type = show ? 'text' : 'password';
    showBtn.setAttribute('aria-pressed', String(show));
    askInput.focus();
  };
  const stopBtn = el('button', { type: 'button' }, 'Stop');
  // End input is Ctrl-D: what was sent is read, then the end, as for a
  // command reading a pasted entry to its end (submit -f -). Stop is
  // Ctrl-C and sends nothing (GUI-021). A secret prompt needs no end.
  const endBtn = el('button', { type: 'button', class: 'end-input', title: 'Send what is typed, if anything, then end the input, as Ctrl-D does in a shell' }, 'Send and end input');
  ask.append(askLabel, askInput, showBtn, el('button', { type: 'submit', class: 'primary' }, 'Send'), endBtn);
  area.append(el('div', { class: 'run-bar' }, status, stopBtn), ask, live);
  return new Promise(resolve => {
    api.start(args, { refresh }).then(j => {
      // The view was left before the job started: its stream is not
      // opened; the job runs on and is in Jobs (ORANGE-010).
      if (area.gone) { resolve({ code: -1, out: '', err: '' }); return; }
      if (onStart) onStart(j);
      let out = '', err = '', tail = '';
      stopBtn.onclick = () => api.cancel(j.id).catch(() => {});
      ask.onsubmit = ev => {
        ev.preventDefault();
        const v = askInput.value;
        askInput.value = '';
        askInput.type = 'password';
        showBtn.setAttribute('aria-pressed', 'false');
        ask.hidden = true;
        return api.input(j.id, v).catch(e => { err += `poo-gui: ${e.message}\n`; });
      };
      endBtn.onclick = async () => {
        if (askInput.value !== '') await ask.onsubmit(new Event('submit'));
        ask.hidden = true;
        api.eof(j.id).catch(e => { err += `poo-gui: ${e.message}\n`; });
      };
      area.unfollow = api.follow(j.id, {
        chunk: c => {
          if (c.stream === 'err') err += c.data; else out += c.data;
          tail = (tail + c.data).slice(-80);
          liveOut.write(c.data, c.stream === 'err' ? 'err' : '');
        },
        state: s => {
          ask.hidden = !s.waiting;
          askInput.type = s.secret ? 'password' : 'text';
          showBtn.hidden = !s.secret;
          endBtn.hidden = !!s.secret;
          showBtn.setAttribute('aria-pressed', 'false');
          if (s.waiting) {
            const prompt = tail.split('\n').pop().trim() || 'input:';
            askLabel.textContent = prompt;
            askInput.focus();
          }
        },
        exit: e => {
          stopBtn.remove();
          ask.remove();
          status.className = 'run-status ' + (e.code === 0 ? 'ok' : 'bad');
          status.textContent = e.code === 0 ? 'done' : `exit ${e.code}`;
          live.remove();
          area.unfollow = null;
          // The server marks a command that prints a secret (GUI-009).
          area.append(showResult(out, err, { empty, secretOutput: secretOutput || j.secretOutput }));
          resolve({ code: e.code, out, err });
        },
        error: () => { area.unfollow = null; status.textContent = 'lost the connection to poo-gui'; resolve({ code: -1, out, err }); },
      });
    }).catch(e => {
      status.className = 'run-status bad';
      status.textContent = e.message;
      resolve({ code: -1, out: '', err: e.message });
    });
  });
}

// leave closes the event stream of a job still running in area, when its
// view is left (ORANGE-010). The page has six connections to poo-gui; a
// stream left open on every visit would use them up and stall the GUI.
// The job itself runs on, as a detached command does, and is in Jobs.
function leave(area) {
  area.gone = true;
  if (area.unfollow) { area.unfollow(); area.unfollow = null; }
}

// showResult renders a command's output: JSON as tables, anything else as
// the text the command printed. Errors are shown as printed, with a next
// step under them when one is known (hints.js). empty is what to say when
// the command printed nothing.
export function showResult(out, err, { empty, secretOutput } = {}) {
  const box = el('div', { class: 'result' });
  if (out.trim()) {
    let v, ok = false;
    try { v = JSON.parse(out); ok = true; } catch (_) { /* text output */ }
    // A command that prints a secret shows plain text hidden; render
    // hides a seed field in JSON (GUI-009).
    if (ok) box.append(render(v));
    else if (secretOutput) box.append(secretValue(out.trim()));
    else {
      // A long text result shows its last MAX_CHARS characters, as the
      // live pane and the terminal keep (ORANGE-008); Copy output still
      // copies all of it.
      const cut = out.length > MAX_CHARS ? out.length - MAX_CHARS : 0;
      if (cut) box.append(el('p', { class: 'hint' }, `The first ${cut} characters are not shown; Copy output copies all of it.`));
      box.append(el('pre', { class: 'text-out' }, cut ? out.slice(cut) : out));
    }
    // The output exactly as poo printed it, for a script or a bug report.
    const copy = el('button', { type: 'button', class: 'small copy-out' }, 'Copy output');
    copy.onclick = () => {
      if (!navigator.clipboard) return;
      navigator.clipboard.writeText(out).then(() => { copy.textContent = 'Copied'; setTimeout(() => { copy.textContent = 'Copy output'; }, 1200); },
        () => { copy.textContent = 'Copy refused'; });
    };
    if (!secretOutput) box.append(copy);
  }
  if (err.trim()) {
    box.append(el('pre', { class: 'text-out err' }, err));
    const h = hintFor(err);
    if (h) box.append(el('p', { class: 'next-step' }, h.text, ' ', el('a', { href: `#/${h.view}` }, h.label)));
  }
  if (!out.trim() && !err.trim()) box.append(el('p', { class: 'hint empty' }, empty || 'No output.'));
  return box;
}

// cmdLine shows a command line with a button that runs it in the terminal.
export function cmdLine(args, ctx) {
  const code = el('code', {}, api.quote(args));
  const row = el('div', { class: 'cmd' }, el('span', { class: 'cmd-label' }, 'Runs'), code,
    el('button', { type: 'button', class: 'small', title: 'Type this command into the terminal and run it', onclick: () => ctx.openInTerminal(api.quote(row.args).replace(/^poo /, '')) }, 'Run in terminal'));
  row.args = args;
  row.set = a => { row.args = a; code.textContent = api.quote(a); };
  return row;
}

// panel runs args() when shown (and every refresh ms) and renders it.
// onResult, as for form, is called with the finished job and the area.
export function panel(ctx, { title, about, args, refresh, empty, onResult }) {
  const area = el('div', { class: 'area' });
  const line = cmdLine(args(), ctx);
  const btn = el('button', { type: 'button' }, 'Refresh');
  const sec = el('section', { class: 'panel' },
    el('div', { class: 'panel-head' }, el('h2', {}, title), btn), about ? el('p', { class: 'hint' }, about) : null, line, area);
  let busy = false;
  // waiting are reload calls the next run answers: one that starts after
  // them, so what it shows was read after whatever they followed.
  let waiting = [];
  // auto is a run the panel starts itself, on opening or on its timer: a
  // refresh job, dropped before the user's own when the list is full.
  const go = async (auto) => {
    if (busy) return;
    busy = true;
    const answers = waiting;
    waiting = [];
    const a = args();
    line.set(a);
    const r = await runJob(a, area, { empty, refresh: auto === true });
    noNode(sec, area, r);
    if (onResult) onResult(r, area);
    busy = false;
    answers.forEach(done => done(r));
    if (waiting.length) go(false);
  };
  // reload runs the panel again, after the run in progress if there is
  // one, and resolves with that new run's {code, out, err} once it is
  // drawn.
  const reload = () => new Promise(done => { waiting.push(done); go(false); });
  btn.onclick = () => go(false);
  go(true);
  // A hidden tab does not refresh: nobody sees the result, and each run
  // is a job and a stream. Showing the tab again refreshes at once
  // (ORANGE-004).
  const visible = () => { if (!document.hidden) go(true); };
  let t = refresh ? setInterval(visible, refresh) : null;
  if (t) document.addEventListener('visibilitychange', visible);
  return { el: sec, reload, stop: () => { leave(area); if (t) { clearInterval(t); document.removeEventListener('visibilitychange', visible); } } };
}

// gate lets one of several actions run at a time: while it is held,
// every button added to it is off. Forms and buttons that change the
// same thing share one, so no change starts from what another is about
// to make stale (GUI-014).
export function gate() {
  const buttons = new Set();
  let held = false;
  return {
    get held() { return held; },
    add(b) { buttons.add(b); b.disabled = held; return b; },
    take() {
      if (held) return false;
      held = true;
      for (const b of buttons) b.disabled = true;
      return true;
    },
    release() {
      held = false;
      for (const b of buttons) if (b.isConnected) b.disabled = false; else buttons.delete(b);
    },
  };
}

// NO_NODE is the error every command that needs a running node prints
// when none answers.
const NO_NODE = /is the node running\?/;

// noNode collapses a panel whose command failed only because no node is
// answering, and shows the view's one banner while any panel is in that
// state (GREEN-011). It decides nothing: the panel's own result, error
// text and next step stay, one click away, exactly as poo printed them.
function noNode(sec, area, r) {
  const down = r.code !== 0 && NO_NODE.test(r.err) && !r.out.trim();
  if (down) sec.dataset.noNode = '1'; else delete sec.dataset.noNode;
  const res = area.querySelector(':scope > .result');
  if (down && res) {
    const d = el('details', { class: 'no-node' }, el('summary', {}, 'Needs a running node (see the note above). Show the error'));
    res.replaceWith(d);
    d.append(res);
  }
  const view = sec.parentElement;
  const banner = view && view.querySelector(':scope > .node-banner');
  if (banner) banner.hidden = !view.querySelector(':scope > section.panel[data-no-node]');
}

// form builds a command line from fields and runs it on submit.
//   fields: [{name, label, type: text|number|password|select|check|list, flag, options, placeholder, required, help}]
//   args(values) returns the argv; the preview updates as fields change.
// onResult, if given, is called with the finished job's {code, out, err}
// and the result area, so a view can act on the data rather than on the
// rendered page.
export function form(ctx, { title, about, fields, args, submit = 'Run', danger, onResult, secretOutput, gate: g }) {
  const area = el('div', { class: 'area' });
  const inputs = {};
  const labels = {};
  const f = el('form', { class: 'fields', autocomplete: 'off' });
  for (const fd of fields) {
    let input;
    if (fd.type === 'select') {
      input = el('select', { name: fd.name }, ...fd.options.map(o => el('option', { value: o.value ?? o }, o.label ?? o)));
    } else if (fd.type === 'check') {
      input = el('input', { type: 'checkbox', name: fd.name });
    } else {
      input = el('input', { name: fd.name, type: fd.type === 'number' ? 'text' : (fd.type || 'text'), inputmode: fd.type === 'number' ? 'numeric' : undefined, placeholder: fd.placeholder || '', spellcheck: 'false' });
    }
    if (fd.value !== undefined) input.value = fd.value;
    if (fd.list) input.setAttribute('list', fd.list);
    inputs[fd.name] = input;
    labels[fd.name] = el('label', { class: fd.type === 'check' ? 'check' : '' }, el('span', {}, fd.label), input, fd.help ? el('small', {}, fd.help) : null);
    f.append(labels[fd.name]);
  }
  // A field with only: {field, values} is shown, and sent, only while that
  // other field (an action select) holds one of values: the same pairs poo
  // checks with flagsFor, so a form never offers a flag poo would refuse.
  // only: {field, not} is the other way round: hidden while the field holds
  // one of not (a grind target of tip or pending takes no -until-from).
  const shown = fd => !fd.only || (fd.only.not ? !fd.only.not.includes(inputs[fd.only.field].value.trim())
    : fd.only.values.includes(inputs[fd.only.field].value));
  const showOnly = () => { for (const fd of fields) labels[fd.name].hidden = !shown(fd); };
  const values = () => {
    const v = {};
    for (const fd of fields) {
      const i = inputs[fd.name];
      const on = shown(fd);
      v[fd.name] = fd.type === 'check' ? on && i.checked : fd.type === 'list' ? (on ? i.value.split(/[\s,]+/).filter(Boolean) : []) : on ? i.value.trim() : '';
    }
    return v;
  };
  showOnly();
  f.addEventListener('input', showOnly);
  const line = cmdLine(args(values()), ctx);
  f.addEventListener('input', () => line.set(args(values())));
  const submitBtn = el('button', { type: 'submit', class: danger ? 'danger' : 'primary' }, submit);
  f.append(el('div', { class: 'actions' }, submitBtn));
  // The button is off while the form's job runs, as a panel is busy: a
  // second press would hide the first job's prompt and leave it waiting
  // for input no one can give (GUI-012). Stop ends it.
  // With a gate, the form also waits for it, and holds it until its job
  // and onResult (which may return a promise) are done.
  let busy = false;
  if (g) g.add(submitBtn);
  f.addEventListener('submit', ev => {
    ev.preventDefault();
    if (busy || (g && g.held)) return;
    if (danger && !confirm(`Run ${api.quote(args(values()))}?`)) return;
    if (g) g.take();
    busy = submitBtn.disabled = true;
    runJob(args(values()), area, { secretOutput }).then(async r => {
      busy = false;
      try { if (onResult) await onResult(r, area); } finally {
        if (g) g.release(); else submitBtn.disabled = false;
      }
    });
  });
  const sec = el('section', { class: 'panel' }, el('div', { class: 'panel-head' }, el('h2', {}, title)),
    about ? el('p', { class: 'hint' }, about) : null, f, line, area);
  // run fills fields from values and submits, as if the user had typed
  // them and pressed the button (a link into a view uses it).
  const run = vals => {
    for (const [k, v] of Object.entries(vals)) if (inputs[k]) inputs[k].value = v;
    showOnly();
    line.set(args(values()));
    f.requestSubmit();
    sec.scrollIntoView({ block: 'start' });
  };
  // hint sets a field's placeholder, as a view that knows the current
  // value shows it (GREEN-033); an empty field still sends nothing.
  const hint = (name, text) => { if (inputs[name]) inputs[name].placeholder = text; };
  return { el: sec, stop: () => leave(area), run, hint };
}

// flag adds -name value when value is set.
export const flag = (name, value) => (value === '' || value === undefined || value === false) ? [] : value === true ? [`-${name}`] : [`-${name}`, value];
export const flags = (name, list) => list.flatMap(v => [`-${name}`, v]);

// A long view gets a row of its panel titles at the top. Each one scrolls
// to its panel and focuses it; the hash is the router, so it stays as is.
function jumpList(els) {
  const heads = els.map(e => [e, e.querySelector('h2')]).filter(([, h]) => h);
  if (heads.length < 5) return null;
  return el('nav', { class: 'jump', 'aria-label': 'Panels on this view' }, 'Jump to: ',
    ...heads.map(([sec, h]) => el('button', { type: 'button', onclick: () => {
      sec.scrollIntoView({ block: 'start' });
      h.tabIndex = -1;
      h.focus({ preventScroll: true });
    } }, h.textContent)));
}

export function mountAll(root, title, intro, parts) {
  // The banner shows when a panel below needs a node and none answers.
  // On the Node view itself it points at the Health check and the start
  // button below rather than linking to the view it is on (GREEN-020).
  const banner = el('div', { class: 'node-banner', role: 'status', hidden: true },
    'No node is answering for this node policy, so the panels that need one are waiting. ',
    ...(title === 'Node' ? ['The Health check below says what is missing, and Start node runs one.']
      : [el('a', { href: '#/node' }, 'Start one on the Node view'), '.']));
  root.append(el('h1', {}, title), intro ? el('p', { class: 'hint' }, intro) : null, banner);
  const jump = jumpList(parts.map(p => p.el));
  if (jump) root.append(jump);
  for (const p of parts) root.append(p.el);
  return () => parts.forEach(p => p.stop());
}

// node finds the node job this GUI is running, if any.
async function nodeJob() {
  const js = await api.jobs();
  return js.reverse().find(j => !j.done && j.args[0] === 'node');
}

// doctorReport lays out poo doctor -o json: one row per check, as printed.
// A check whose fix is a poo command line (fix_args, D110) gets a button
// that types it into the terminal exactly as poo printed it: poo already
// names the checked policy's -dir when Settings chose -root or -policy
// (GREEN-036), so the view adds nothing. Any other fix is shown as the
// text poo printed.
const RUNNABLE_FIX = /^poo [a-z][\w./:=-]*( [\w./:=-]+)*$/;
export function doctorReport(rep, ctx) {
  const list = el('ul', { class: 'doctor' });
  for (const c of rep.checks || []) {
    let run = null;
    if (Array.isArray(c.fix_args) && c.fix_args.length) run = c.fix_args;
    else if (c.fix && RUNNABLE_FIX.test(c.fix)) run = c.fix.replace(/^poo /, '').split(' ');
    const fix = c.fix ? (run
      ? el('div', { class: 'doctor-fix' }, 'fix: ', el('code', {}, api.quote(run)), ' ',
        el('button', { type: 'button', class: 'small', onclick: () => ctx.openInTerminal(api.quote(run).replace(/^poo /, '')) }, 'Run in terminal'))
      : el('div', { class: 'doctor-fix' }, `fix: ${c.fix}`)) : null;
    list.append(el('li', { class: `doctor-${c.result}` },
      el('span', { class: `pill pill-${c.result}` }, c.result),
      el('strong', {}, c.name), ' ', el('span', {}, c.detail || ''), fix));
  }
  return el('div', { class: 'result' }, rep.dir ? el('p', { class: 'hint' }, `Data directory: ${rep.dir}`) : null, list);
}

const nodeView = {
  id: 'node',
  title: 'Node',
  mount(root, ctx) {
    const runArea = el('div', { class: 'area' });
    const startBtn = el('button', { type: 'button', class: 'primary' }, 'Start node');
    const runLine = cmdLine(['node', ...where.flags()], ctx);
    const runSec = el('section', { class: 'panel' },
      el('div', { class: 'panel-head' }, el('h2', {}, 'Run the node'), startBtn),
      el('p', { class: 'hint' }, 'Runs poo node inside the GUI until you stop it or close the GUI. A node already running elsewhere (for example as a service) is used as it is: the views below talk to whichever node serves this node policy.'),
      runLine, runArea);
    let following = null, unfollowLog = null, gone = false;
    const showRunning = async () => {
      if (document.hidden && following) return;
      const j = await nodeJob().catch(() => null);
      if (gone) return;
      startBtn.disabled = !!j;
      if (j && following !== j.id) {
        following = j.id;
        runArea.textContent = '';
        const log = el('pre', { class: 'live-out' });
        const logOut = scrollback(log, { follow: true });
        const stop = el('button', { type: 'button', class: 'danger' }, 'Stop node');
        stop.onclick = () => api.cancel(j.id);
        runArea.append(el('div', { class: 'run-bar' }, el('span', { class: 'run-status live' }, `running: ${j.command}`), stop), log);
        if (unfollowLog) unfollowLog();
        unfollowLog = api.follow(j.id, {
          chunk: c => logOut.write(c.data, c.stream === 'err' ? 'err' : ''),
          exit: e => { following = null; unfollowLog = null; startBtn.disabled = false; runArea.prepend(el('div', { class: 'run-status ' + (e.code ? 'bad' : 'ok') }, `node stopped (exit ${e.code})`)); stop.remove(); },
        });
      }
    };
    startBtn.onclick = async () => {
      startBtn.disabled = true;
      const a = ['node', ...where.flags()];
      runLine.set(a);
      try { await api.start(a); } catch (e) { runArea.textContent = e.message; }
      setTimeout(showRunning, 200);
    };
    showRunning();
    // poo status says when node.json changed since the node loaded it and
    // the change waits for a reload (Source GREEN-037). The panel says so
    // above what it shows, with the Reload form's own button (GREEN-038).
    const status = panel(ctx, { title: 'Followed pools', about: 'What the running node follows, and where each pool stands.', args: () => ['status', ...where.flags()], refresh: 5000,
      onResult: (r, area) => {
        let st;
        try { st = r.code === 0 && JSON.parse(r.out); } catch (_) { return; }
        if (!st || !st.policy_changed) return;
        area.prepend(el('p', { class: 'next-step policy-changed', role: 'status' },
          'The node policy file has changed since the node loaded it, and the change waits for a reload. ',
          el('button', { type: 'button', class: 'small', onclick: () => reload.run({}) }, 'Reload now')));
      } });
    const init = form(ctx, {
      title: 'Write a node policy',
      about: 'Creates the node policy (settings, series to follow) and makes it current.',
      fields: [
        { name: 'example', label: 'Follow the worked example series', type: 'check' },
        { name: 'series', label: 'Series files', type: 'list', placeholder: 'path/to/series.json …', help: 'Series parameter files (poo series new) to know and follow.' },
        { name: 'listen', label: 'RPC address', placeholder: 'default; "off" to disable' },
        { name: 'p2p', label: 'P2P address', placeholder: 'default; "off" to accept none' },
        { name: 'peer', label: 'Peers', type: 'list', placeholder: 'host:port …' },
        { name: 'registry', label: 'Address registries', type: 'list', placeholder: 'URL or file …' },
        { name: 'advertise', label: 'Advertise as', placeholder: 'host:port' },
        { name: 'force', label: 'Replace the settings of an existing policy (its data is kept)', type: 'check' },
      ],
      args: v => ['init', ...where.flags(), ...flag('example', v.example), ...flags('series', v.series), ...flag('listen', v.listen),
        ...flag('p2p', v.p2p), ...flags('peer', v.peer), ...flags('registry', v.registry), ...flag('advertise', v.advertise), ...flag('force', v.force)],
      submit: 'Write policy',
    });
    const followFields = [
      { name: 'issuer', label: 'Issuer id', placeholder: 'every series of an issuer' },
      { name: 'series', label: 'Series id' },
      { name: 'pool', label: 'Pools', type: 'list', placeholder: 'K …' },
      { name: 'mint', label: 'Mints', type: 'list', placeholder: 'serial …' },
    ];
    const followArgs = (cmd, v) => [cmd, ...where.flags(), ...flag('issuer', v.issuer), ...flag('series', v.series), ...flags('pool', v.pool), ...flags('mint', v.mint), ...flag('replace', v.replace), ...flag('no-reload', v.noReload)];
    const follows = panel(ctx, { title: 'What the node follows', about: 'The follow list of the node policy.', args: () => ['follow', ...where.flags()] });
    // Replace (GREEN-038, Source) is poo follow -replace: with a series, the
    // pools and mints given become all it follows of that series, in one
    // change. It shows only once a series is named, as poo requires.
    const follow = form(ctx, { title: 'Follow more', about: 'Adds to the node policy; a running node applies it at once.',
      fields: [...followFields, { name: 'replace', label: 'Replace what the series follows with these', type: 'check', only: { field: 'series', not: [''] } }, { name: 'noReload', label: 'Only write the policy (no reload)', type: 'check' }], args: v => followArgs('follow', v), submit: 'Follow' });
    const unfollow = form(ctx, { title: 'Follow less', fields: [...followFields, { name: 'noReload', label: 'Only write the policy (no reload)', type: 'check' }],
      args: v => followArgs('unfollow', v), submit: 'Unfollow', danger: true });
    const reload = form(ctx, { title: 'Reload the node policy', about: 'Makes the running node apply its node policy file as it now stands.', fields: [],
      args: () => ['reload', ...where.flags()], submit: 'Reload', onResult: r => { if (r.code === 0) status.reload(); } });
    const t = setInterval(showRunning, 3000);
    // poo doctor checks the policy, files, series, RPC and P2P in one go.
    // Its report is drawn as a checklist; the raw output stays one click away.
    const health = panel(ctx, { title: 'Health check', about: 'What poo doctor finds for this node policy, with the fix it suggests.',
      args: () => ['doctor', '-o', 'json', ...where.flags()],
      onResult: (r, area) => {
        let rep;
        try { rep = JSON.parse(r.out); } catch (_) { return; }
        const raw = area.querySelector('.result');
        const det = el('details', { class: 'raw' }, el('summary', {}, 'Output as printed'));
        if (raw) { raw.replaceWith(doctorReport(rep, ctx)); det.append(raw); area.append(det); }
      } });
    const stop = mountAll(root, 'Node', 'Run the full node and see what it follows.', [health, { el: runSec, stop: () => { clearInterval(t); gone = true; if (unfollowLog) unfollowLog(); } }, status, follows, follow, unfollow, reload, init]);
    return stop;
  },
};

const poolsView = {
  id: 'pools',
  title: 'Pools and payments',
  mount(root, ctx) {
    const serial = { name: 'serial', label: 'Mint serial', type: 'number', required: true };
    const series = { name: 'series', label: 'Series id', placeholder: 'only if the node follows more than one' };
    return mountAll(root, 'Pools and payments', 'Look up mints and transactions, and build entries and layers.', [
      panel(ctx, { title: 'Followed pools', about: 'Each followed pool: tip, height, weight, waiting and stored records (poo pools).', args: () => ['pools', ...where.flags()] }),
      // poo unspent and poo tx replace the older holdings and payment
      // (Source GREEN-008): the same answers, with the pool, the height and
      // the decoded transaction, and no serial needed for a transaction.
      form(ctx, { title: 'Holdings of a mint', about: 'Unspent portions of a mint.', fields: [serial, series],
        args: v => ['unspent', ...where.flags(), ...flag('serial', v.serial), ...flag('series', v.series)], submit: 'Look up' }),
      form(ctx, { title: 'Where a payment stands', about: 'A transaction, the weight behind it, and known rivals.',
        fields: [{ name: 'tid', label: 'Transaction id', placeholder: 'hex' }, { ...serial, label: 'In mint serial', placeholder: 'optional: look only in its pool' }, series],
        args: v => ['tx', ...where.flags(), ...flag('in-serial', v.serial), ...flag('series', v.series), ...(v.tid ? [v.tid] : [])], submit: 'Look up' }),
      form(ctx, { title: 'Build an entry', about: 'Builds an entry from the pending transactions of a pool.',
        fields: [{ name: 'pool', label: 'Pool', type: 'number' }, { name: 'work', label: 'Layer tries', type: 'number', placeholder: 'optional' }, series, { name: 'bytes', label: 'Also print the hex, for Submit on another node', type: 'check' }],
        args: v => ['build', ...where.flags(), ...flag('pool', v.pool), ...flag('work', v.work), ...flag('series', v.series), ...flag('bytes', v.bytes)], submit: 'Build' }),
      form(ctx, { title: 'Add a layer', about: 'Adds the strongest layer found, by default on the selected tip.',
        fields: [{ name: 'pool', label: 'Pool', type: 'number' }, { name: 'entry', label: 'Entry hash', placeholder: 'default: the selected tip' }, { name: 'tries', label: 'Tries', type: 'number' }, series, { name: 'bytes', label: 'Also print the hex, for Submit on another node', type: 'check' }],
        args: v => ['work', ...where.flags(), ...flag('pool', v.pool), ...flag('entry', v.entry), ...flag('tries', v.tries), ...flag('series', v.series), ...flag('bytes', v.bytes)], submit: 'Work' }),
      // poo submit (Source D142) hands the node an entry or layer made
      // elsewhere, as the hex Build or Add a layer prints with -bytes,
      // typed in or read by poo from a file (-f, BLUE-015).
      form(ctx, { title: 'Submit an entry or layer', about: 'Gives the node an entry or a layer made elsewhere: the hex of an entry or layer reply.',
        fields: [{ name: 'kind', label: 'Kind', type: 'select', options: ['entry', 'layer'] }, { name: 'pool', label: 'Pool', type: 'number' }, { ...serial, required: false, placeholder: 'or a mint serial' }, series, { name: 'file', label: 'From a file', placeholder: 'a path on this machine, or - to paste it when asked, then Send and end input' },
          { name: 'hex', label: 'Hex', placeholder: 'the entry or layer, hex', only: { field: 'file', values: [''] } }],
        args: v => ['submit', v.kind || 'entry', ...where.flags(), ...flag('series', v.series), ...flag('pool', v.pool), ...flag('serial', v.serial), ...flag('f', v.file), ...(!v.file && v.hex ? [v.hex] : [])], submit: 'Submit', danger: true }),
    ]);
  },
};

const peersView = {
  id: 'peers',
  title: 'Peers',
  mount(root, ctx) {
    return mountAll(root, 'Peers', 'Who the node is connected to, how they behave, and what it is checking.', [
      panel(ctx, { title: 'Peers', about: 'Connected peers, their categories and points, and the peer book.', args: () => ['peers', ...where.flags()], refresh: 10000 }),
      form(ctx, { title: 'Act on a peer', fields: [
        { name: 'action', label: 'Action', type: 'select', options: ['timeout', 'block', 'unblock', 'clear', 'template', 'delete'] },
        { name: 'addr', label: 'Peer address', placeholder: 'host:port' },
        // Which action takes which flag is poo peer's own rule (pkg/cli/node.go).
        { name: 'seconds', label: 'Seconds', type: 'number', placeholder: '-1 blocks until unblocked', only: { field: 'action', values: ['timeout', 'block'] } },
        { name: 'range', label: 'Whole range', type: 'check', only: { field: 'action', values: ['block'] } },
        { name: 'what', label: 'Clear', type: 'select', options: ['', 'flags', 'measures', 'all'], only: { field: 'action', values: ['clear'] } },
        { name: 'template', label: 'Tracking template', placeholder: 'template name', only: { field: 'action', values: ['template'] } },
      ], args: v => ['peer', v.action, ...where.flags(), ...flag('seconds', v.seconds), ...flag('range', v.range), ...flag('what', v.what), ...flag('template', v.template), ...(v.addr ? [v.addr] : [])],
      submit: 'Apply', danger: true }),
      panel(ctx, { title: 'Tracking', about: "Every tracked address's status and the blocked ranges.", args: () => ['tracking', ...where.flags()] }),
      form(ctx, { title: "One peer's record", fields: [{ name: 'addr', label: 'Peer address', placeholder: 'host:port' }],
        args: v => ['tracking', ...where.flags(), ...(v.addr ? [v.addr] : [])], submit: 'Look up' }),
      panel(ctx, { title: 'Chases', about: 'Heavier stated weights waiting or being checked.', args: () => ['chases', ...where.flags()] }),
    ]);
  },
};

// The Wallet view's wallet file, kept per browser like the Settings flags.
const WALLET_KEY = 'poo-gui-wallet';
const walletFile = () => { try { return localStorage.getItem(WALLET_KEY) || ''; } catch (_) { return ''; } };
const setWalletFile = v => { try { localStorage.setItem(WALLET_KEY, v); } catch (_) { /* storage off */ } };

const walletView = {
  id: 'wallet',
  title: 'Wallet',
  mount(root, ctx) {
    // The wallet file is chosen once for the whole view, like -root and
    // -policy in Settings, instead of in every form.
    const pick = el('input', { name: 'wallet-file', value: walletFile(), placeholder: 'wallet.json in the root', spellcheck: 'false' });
    const wf = () => [...where.flags(), ...flag('wallet', pick.value.trim())];
    const picker = el('section', { class: 'panel wallet-pick' },
      el('label', {}, el('span', {}, 'Wallet file (-wallet)'), pick),
      el('small', { class: 'hint' }, 'Every form below uses this file. Empty means wallet.json in the root, as in a shell.'));
    pick.addEventListener('input', () => {
      setWalletFile(pick.value.trim());
      // Each form's preview shows the new -wallet at once.
      for (const f of root.querySelectorAll('form.fields')) f.dispatchEvent(new Event('input'));
    });
    const series = { name: 'series', label: 'Series id', placeholder: 'only if the node follows more than one' };
    const pass = 'The command asks for the passphrase when it needs it.';
    return mountAll(root, 'Wallet', 'Keys from one seed, payments and requests, all through poo wallet. Passphrases go straight to the command and are never stored.', [
      { el: picker, stop: () => {} },
      form(ctx, { title: 'Balance', about: 'Spendable units per watched mint. ' + pass, fields: [series], args: v => ['wallet', 'balance', ...wf(v), ...flag('series', v.series)], submit: 'Show balance' }),
      form(ctx, { title: 'Addresses', about: 'Every address, used or not, and shared ones. ' + pass, fields: [], args: v => ['wallet', 'addresses', ...wf(v)], submit: 'Show addresses' }),
      form(ctx, { title: 'New address', fields: [{ name: 'label', label: 'Label' }], args: v => ['wallet', 'address', ...wf(v), ...flag('label', v.label)], submit: 'Give out an address' }),
      form(ctx, { title: 'Pay', about: 'Pays an address, or a payment request file.', fields: [
        { name: 'to', label: 'To address' }, { name: 'amount', label: 'Amount', type: 'number' },
        { name: 'request', label: 'Or a request file' }, { name: 'mint', label: 'Only from mints', type: 'list', placeholder: 'serials …' },
        { name: 'memo', label: 'Memo' }, { name: 'out', label: 'Co-signing file', placeholder: 'for a shared policy' }, series],
      args: v => ['wallet', 'pay', ...wf(v), ...flag('to', v.to), ...flag('amount', v.amount), ...flag('request', v.request), ...flags('mint', v.mint), ...flag('memo', v.memo), ...flag('out', v.out), ...flag('series', v.series)],
      submit: 'Pay', danger: true }),
      // A shared k-of-n payment from start to end (GREEN-039): Pay with a
      // co-signing file writes what co-signers sign; each runs cosign on
      // it, and submit sends it once enough have signed.
      form(ctx, { title: 'Record a shared policy', about: 'A k-of-n address this wallet signs for with others. self is a fresh key of this wallet; give each co-signer\'s public key. ' + pass, fields: [
        { name: 'name', label: 'Name' }, { name: 'k', label: 'Signatures needed', type: 'number', placeholder: 'default 1' },
        { name: 'key', label: 'Keys', type: 'list', placeholder: 'self, ALG:HEX or HEX …' }, { name: 'label', label: 'Label' }],
        args: v => ['wallet', 'policy', ...wf(v), ...flag('name', v.name), ...flag('k', v.k), ...flags('key', v.key), ...flag('label', v.label)], submit: 'Record' }),
      form(ctx, { title: 'Co-sign a payment', about: 'Adds this wallet\'s signatures to a co-signing file another signer wrote with Pay. ' + pass, fields: [{ name: 'file', label: 'Co-signing file' }],
        args: v => ['wallet', 'cosign', ...wf(v), ...(v.file ? [v.file] : [])], submit: 'Co-sign' }),
      form(ctx, { title: 'Submit a co-signed payment', about: 'Sends a payment whose co-signing file has every signature it needs.', fields: [{ name: 'file', label: 'Co-signing file' }],
        args: v => ['wallet', 'submit', ...wf(v), ...(v.file ? [v.file] : [])], submit: 'Submit', danger: true }),
      form(ctx, { title: 'Request a payment', about: 'Asks to be paid, to a fresh address.', fields: [{ name: 'amount', label: 'Amount', type: 'number' }, { name: 'memo', label: 'What for' }, { name: 'out', label: 'Request file', placeholder: 'default: print it' }],
        args: v => ['wallet', 'request', ...wf(v), ...flag('amount', v.amount), ...flag('memo', v.memo), ...flag('out', v.out)], submit: 'Make request' }),
      form(ctx, { title: 'What has arrived', fields: [{ name: 'address', label: 'Requested address' }, { name: 'request', label: 'Or the request file' }, { name: 'threshold', label: 'Threshold', placeholder: 'from(e) for settled' }],
        args: v => ['wallet', 'receive', ...wf(v), ...flag('address', v.address), ...flag('request', v.request), ...flag('threshold', v.threshold)], submit: 'Check' }),
      form(ctx, { title: 'Payments sent', about: 'Where every payment sent stands.', fields: [{ name: 'threshold', label: 'Threshold', placeholder: 'from(e) for settled' }],
        args: v => ['wallet', 'status', ...wf(v), ...flag('threshold', v.threshold)], submit: 'Show' }),
      form(ctx, { title: 'Send dropped legs again', about: 'Sends again the parts of payments the node dropped (Payments sent shows them). ' + pass, fields: [],
        args: v => ['wallet', 'resend', ...wf(v)], submit: 'Resend' }),
      form(ctx, { title: 'Watch mints', about: 'Adds mints or pools to look for portions in.', fields: [{ name: 'serial', label: 'Mint serials', type: 'list' }, { name: 'pool', label: 'Pools', type: 'list' }],
        args: v => ['wallet', 'watch', ...wf(v), ...flags('serial', v.serial), ...flags('pool', v.pool)], submit: 'Watch' }),
      form(ctx, { title: 'Claim a mint', fields: [{ name: 'serial', label: 'Mint serial', type: 'number' }, series],
        args: v => ['wallet', 'claim', ...wf(v), ...flag('serial', v.serial), ...flag('series', v.series)], submit: 'Claim', danger: true }),
      form(ctx, { title: 'Follow only the watched mints', about: 'Light mode: the node policy follows only the pools of the mints this wallet watches, in place of what it follows now.', fields: [],
        args: v => ['wallet', 'follow', ...wf(v)], submit: 'Follow only these', danger: true }),
      form(ctx, { title: 'Change the passphrase', about: 'Seals the seed under a new passphrase. poo asks for the old one, then the new one, each in a masked field.', fields: [],
        args: v => ['wallet', 'passwd', ...wf(v)], submit: 'Change' }),
      form(ctx, { title: 'Show the seed', about: 'Prints the seed, to back it up. Anyone with it can spend from this wallet. The output is hidden until you show it, and Jobs hides it.', fields: [],
        args: v => ['wallet', 'seed', ...wf(v)], submit: 'Show seed', danger: true, secretOutput: true }),
      // Restoring runs -restore -, so poo asks for the seed through its
      // secret prompt (D106) and the seed is typed into a masked field,
      // never onto the command line.
      form(ctx, { title: 'Create or restore a wallet', fields: [{ name: 'alg', label: 'Key algorithm', placeholder: 'default ed25519', list: 'crypto-algs' }, { name: 'restore', label: 'Restore from a seed', type: 'check', help: 'poo asks for the seed in a masked field after you press Create.' }, { name: 'gap', label: 'Gap', type: 'number' }],
        args: v => ['wallet', 'create', ...wf(v), ...flag('alg', v.alg), ...(v.restore ? ['-restore', '-'] : []), ...flag('gap', v.gap)], submit: 'Create' }),
    ]);
  },
};

const policiesView = {
  id: 'policies',
  title: 'Node policies',
  mount(root, ctx) {
    const rootFlag = () => where.get().root ? ['-root', where.get().root] : [];
    const name = { name: 'name', label: 'Policy name' };
    return mountAll(root, 'Node policies', 'Saved and archived node policies; * marks the current one.', [
      panel(ctx, { title: 'Policies', args: () => ['policy', 'list', ...rootFlag()], empty: 'No saved node policies yet. Write one on the Node view.' }),
      form(ctx, { title: 'Use, archive, unarchive or delete', fields: [
        { name: 'sub', label: 'Action', type: 'select', options: ['use', 'archive', 'unarchive', 'delete'] }, name,
        { name: 'fresh', label: 'Fresh (drop stored data)', type: 'check', only: { field: 'sub', values: ['unarchive'] } }],
      args: v => ['policy', v.sub, ...rootFlag(), ...flag('fresh', v.fresh), ...(v.name ? [v.name] : [])], submit: 'Apply', danger: true }),
    ]);
  },
};

// cryptoChoices offers the hashes and key algorithms poo crypto printed as
// suggestions in the fields that name one (a datalist suggests, poo still
// decides). The lists stay in the page for the Wallet view's key field too.
function cryptoChoices(out) {
  let c;
  try { c = JSON.parse(out); } catch (_) { return; }
  for (const [id, list] of [['crypto-hashes', c.hashes], ['crypto-algs', c.key_algs]]) {
    let dl = document.getElementById(id);
    if (!dl) { dl = el('datalist', { id }); document.body.append(dl); }
    dl.textContent = '';
    for (const x of list || []) dl.append(el('option', { value: x.name }));
  }
}

// A key field opened before Series and keys asks poo crypto once, the
// first time it is focused, so its suggestions are poo's own. One ask is
// in flight at a time (ORANGE-011): focus moving in and out of key fields
// before poo answers would otherwise run poo crypto for each focus, each
// a job in the Jobs list.
let askingCrypto = null;
document.addEventListener('focusin', e => {
  const l = e.target.getAttribute && e.target.getAttribute('list');
  if ((l === 'crypto-algs' || l === 'crypto-hashes') && !document.getElementById(l) && !askingCrypto) {
    askingCrypto = api.run(['crypto']).then(r => { if (r.code === 0) cryptoChoices(r.out); }).catch(() => {})
      .finally(() => { askingCrypto = null; });
  }
});

const cryptoView = {
  id: 'crypto',
  title: 'Series and keys',
  mount(root, ctx) {
    return mountAll(root, 'Series and keys', 'Offline tools: the choices a series can make, keys, and series parameters.', [
      panel(ctx, { title: 'Hashes, key algorithms and policy types', args: () => ['crypto'], onResult: r => { if (r.code === 0) cryptoChoices(r.out); } }),
      form(ctx, { title: 'Make a key', about: 'A key and its 1-of-1 address. Leave the seed empty for a random one; the output includes the seed.',
        fields: [{ name: 'alg', label: 'Key algorithm', placeholder: 'ed25519', list: 'crypto-algs' }, { name: 'hash', label: 'Hash', placeholder: 'sha256', list: 'crypto-hashes' }, { name: 'seed', label: 'Seed', placeholder: 'hex, optional', help: 'Part of the command line, as in a shell: it shows in the preview. Jobs hides it, and it is never saved.' }],
        args: v => ['keygen', ...flag('alg', v.alg), ...flag('hash', v.hash), ...flag('seed', v.seed)], submit: 'Make key', secretOutput: true }),
      form(ctx, { title: 'New series parameters', about: 'Prints new series parameters, checked. More options are in the terminal (poo series new -h).',
        fields: [{ name: 'hash', label: 'Hash', placeholder: 'sha256', list: 'crypto-hashes' }, { name: 'algs', label: 'Key algorithms', placeholder: 'comma list: ' + 'see the list above' }, { name: 'types', label: 'Policy types', placeholder: 'comma list: see the list above' }, { name: 'mints', label: 'Mints', type: 'number' }],
        args: v => ['series', 'new', ...flag('hash', v.hash), ...flag('algs', v.algs), ...flag('types', v.types), ...flag('mints', v.mints)], submit: 'Make series' }),
      form(ctx, { title: 'Series file info', fields: [{ name: 'file', label: 'Series file', placeholder: 'series.json' }],
        args: v => ['series', 'info', ...(v.file ? [v.file] : [])], submit: 'Show' }),
      // The last two commands poo help lists that no view ran (GREEN-040).
      form(ctx, { title: 'The worked example\'s series', about: 'The series poo init -example follows, as JSON: a starting point for a series file.', fields: [],
        args: () => ['series', 'example'], submit: 'Show' }),
      form(ctx, { title: 'Simulate the network', about: 'The deterministic network simulator, on the worked example\'s series, in this process: it touches no files and no node. The same seed gives the same result. More flags are in the terminal (poo sim load -h).',
        fields: [{ name: 'scenario', label: 'Scenario', type: 'select', options: ['load', 'partition', 'attack', 'sizing'] },
          { name: 'seed', label: 'Seed', type: 'number', placeholder: 'default 1' }, { name: 'runs', label: 'Runs', type: 'number', placeholder: 'default 1, seeds from Seed up' },
          { name: 'nodes', label: 'Nodes', type: 'number', placeholder: 'default 16' }, { name: 'duration', label: 'Duration (ms)', type: 'number', placeholder: 'default 60000' },
          { name: 'sizes', label: 'Pool sizes', placeholder: 'default 1,2,4,8', only: { field: 'scenario', values: ['sizing'] } }],
        args: v => ['sim', v.scenario || 'load', ...flag('seed', v.seed), ...flag('runs', v.runs), ...flag('nodes', v.nodes), ...flag('duration', v.duration), ...flag('sizes', v.sizes)], submit: 'Simulate' }),
    ]);
  },
};

const settingsView = {
  id: 'settings',
  title: 'Settings',
  mount(root) {
    const cur = where.get();
    const r = el('input', { value: cur.root, placeholder: '$POO_HOME or ~/.poo', spellcheck: 'false' });
    const p = el('input', { value: cur.policy, placeholder: 'the current policy', spellcheck: 'false' });
    const theme = el('select', {}, ...['system', 'light', 'dark'].map(t => el('option', { value: t }, t)));
    theme.value = where.theme();
    const f = el('form', { class: 'fields' },
      el('label', {}, el('span', {}, 'Root (-root)'), r),
      el('label', {}, el('span', {}, 'Node policy (-policy)'), p),
      el('label', {}, el('span', {}, 'Theme'), theme),
      el('div', { class: 'actions' }, el('button', { type: 'submit', class: 'primary' }, 'Save')));
    f.onsubmit = ev => { ev.preventDefault(); where.set({ root: r.value.trim(), policy: p.value.trim() }); where.setTheme(theme.value); };
    root.append(el('h1', {}, 'Settings'),
      el('p', { class: 'hint' }, 'Views add these flags to the commands they run, so they pick a node policy the way -root and -policy do in a shell. The terminal adds nothing: what you type is what runs.'), f);
  },
};

// jobsView lists the jobs the GUI has run, with their output.
const jobsView = {
  id: 'jobs',
  title: 'Jobs',
  mount(root, ctx) {
    const list = el('div', { class: 'jobs' });
    const detail = el('pre', { class: 'term-out job-out' });
    // The filters only choose which rows to show; every job is still kept.
    const filter = el('input', { type: 'search', placeholder: 'Filter by command', 'aria-label': 'Filter jobs by command', spellcheck: 'false' });
    const failed = el('input', { type: 'checkbox' });
    const repeats = el('input', { type: 'checkbox', checked: true });
    const tools = el('div', { class: 'job-tools' }, filter,
      el('label', {}, failed, ' Failed only'),
      el('label', { title: 'Views refresh their panels, so the same command runs many times. Show it once, with a count.' }, repeats, ' Group repeats'));
    for (const i of [filter, failed, repeats]) i.addEventListener('input', () => draw());
    root.append(el('h1', {}, 'Jobs'),
      el('p', { class: 'hint' }, 'Every command the GUI has run, from views, forms and the terminal. Each is the poo command line shown.'),
      tools, list, detail);
    let unfollow = null;
    let js = [];
    // The list is drawn again only when it changed (ORANGE-004).
    let last = '';
    async function refresh() {
      if (document.hidden) return;
      try { js = await api.jobs(); } catch (e) { list.textContent = e.message; last = ''; return; }
      const now = JSON.stringify(js);
      if (now === last) return;
      last = now;
      draw();
    }
    function draw() {
      const q = filter.value.trim().toLowerCase();
      // Newest first. With grouping, a run of the same finished command
      // line and exit code shows once, as its newest job, with a count.
      const rows = [];
      for (const j of js.slice().reverse()) {
        if (q && !j.command.toLowerCase().includes(q)) continue;
        if (failed.checked && !(j.done && j.code !== 0)) continue;
        const prev = rows[rows.length - 1];
        if (repeats.checked && prev && j.done && prev.j.done && prev.j.command === j.command && prev.j.code === j.code) { prev.n++; continue; }
        rows.push({ j, n: 1 });
      }
      list.textContent = '';
      if (!rows.length) list.append(el('p', { class: 'hint empty' }, js.length ? 'No jobs match the filter.' : 'No commands have run yet. Every view, form and terminal line shows up here.'));
      for (const { j, n } of rows) {
        const state = j.done ? `exit ${j.code}` : 'running';
        list.append(el('div', { class: 'job' },
          el('code', {}, j.command),
          el('time', { datetime: j.started, title: j.started }, new Date(j.started).toLocaleTimeString()),
          n > 1 ? el('span', { class: 'count', title: `${n} runs in a row; the newest is shown` }, `×${n}`) : null,
          el('span', { class: j.done ? (j.code === 0 ? 'ok' : 'bad') : 'live' }, state),
          // A finished command that printed a secret keeps no output to
          // show again (GUI-009).
          j.done && j.secretOutput
            ? el('span', { class: 'hint', title: 'Its output held a secret, such as a seed, so poo-gui gave it out once and did not keep it' }, 'output not kept (it held a secret)')
            : el('button', { type: 'button', onclick: () => showJob(j) }, 'Output'),
          j.done ? null : el('button', { type: 'button', onclick: () => api.cancel(j.id).then(refresh) }, 'Stop'),
          // A line with a hidden secret cannot be run again as shown.
          j.redacted ? null : el('button', { type: 'button', onclick: () => ctx.openInTerminal(j.command.replace(/^poo /, '')) }, 'Run again in terminal')));
      }
    }
    function showJob(j) {
      if (unfollow) unfollow();
      const shown = scrollback(detail);
      shown.clear();
      unfollow = api.follow(j.id, {
        chunk: c => shown.write(c.data, c.stream === 'err' ? 'err' : ''),
        dropped: d => shown.write(d.withheld ? '[output not kept: it held a secret]\n' : '[earlier output dropped]\n', 'note'),
        exit: refresh,
      });
    }
    refresh();
    const t = setInterval(refresh, 3000);
    return () => { clearInterval(t); if (unfollow) unfollow(); };
  },
};

export const views = [nodeView, watchView, exploreView, workView, poolsView, walletView, peersView, policiesView, cryptoView, jobsView, settingsView];
