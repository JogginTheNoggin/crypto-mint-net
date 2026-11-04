// The parts every view is built from (the views themselves are in views/,
// one file each, GUI-GREEN-044). Each panel runs one poo command and
// renders what it prints; each form builds one poo command line from its
// fields and runs it. Nothing here computes anything a command does not
// print, and every panel shows its command line, which runs the same in
// the terminal (docs/gui.md).
import * as api from './api.js';
import { render, secretValue } from './render.js';
import * as where from './where.js';
import { hintFor } from './hints.js';
import { scrollback, MAX_CHARS } from './scroll.js';

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
// WAITING is poo's word for a transaction on a part of a variable series
// the node has no release for yet (D166).
const WAITING = /waiting for a release/;

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
        return api.input(j.id, v).then(() => true, e => { err += `poo-gui: ${e.message}\n`; return false; });
      };
      // The input is ended only once what is typed has arrived whole: a
      // send that failed must not leave the command reading half of it.
      endBtn.onclick = async () => {
        if (askInput.value !== '' && !(await ask.onsubmit(new Event('submit')))) return;
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
          // poo refuses a mint of a part not released yet as waiting for a
          // release, not as invalid (D166): said as poo says it.
          const waiting = e.code !== 0 && WAITING.test(err);
          status.className = 'run-status ' + (e.code === 0 ? 'ok' : waiting ? 'waiting' : 'bad');
          status.textContent = e.code === 0 ? 'done' : waiting ? `waiting for a release (exit ${e.code})` : `exit ${e.code}`;
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
    if (!secretOutput) box.append(copyButton('Copy output', () => out, 'small copy-out'));
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
// copyButton copies text() to the clipboard and says whether it worked.
export function copyButton(label, text, cls = 'small') {
  const b = el('button', { type: 'button', class: cls }, label);
  b.onclick = () => {
    if (!navigator.clipboard) { b.textContent = 'Copy refused'; return; }
    navigator.clipboard.writeText(text()).then(() => { b.textContent = 'Copied'; setTimeout(() => { b.textContent = label; }, 1200); },
      () => { b.textContent = 'Copy refused'; });
  };
  return b;
}

// cmdLine shows the command a panel or form runs, quoted as the
// platform's shell takes it, with Run in terminal and Copy: the line
// pasted into a shell runs the same poo command (GUI-GREEN-046). Copy is
// off while the line holds a secret flag, as the terminal keeps such a
// line out of its history (GUI-006).
export function cmdLine(args, ctx) {
  const code = el('code', {}, api.quote(args));
  const copy = copyButton('Copy', () => api.copyLine(row.args), 'small copy-cmd');
  copy.title = 'Copy this command line, to run it in a shell';
  const row = el('div', { class: 'cmd' }, el('span', { class: 'cmd-label' }, 'Runs'), code,
    el('button', { type: 'button', class: 'small', title: 'Type this command into the terminal and run it', onclick: () => ctx.openInTerminal(api.quote(row.args).replace(/^poo /, '')) }, 'Run in terminal'), ' ', copy);
  // Copy is off for a line with a secret, and for one no quoting makes
  // safe to paste into a Windows shell (GUI-024).
  const secret = () => {
    const held = api.secretLine(api.quote(row.args));
    const unsafe = api.copyLine(row.args) === null;
    copy.disabled = held || unsafe;
    copy.title = held ? 'The line holds a secret, so it is not copied'
      : unsafe ? 'The line has characters a Windows shell (PowerShell or cmd) would run or change, so it is not copied: use Run in terminal, or type it'
      : 'Copy this command line, to run it in a shell';
  };
  row.args = args;
  row.set = a => { row.args = a; code.textContent = api.quote(a); secret(); };
  secret();
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
//   fields: [{name, label, type: text|number|password|select|check|list|choices, flag, options, placeholder, required, help}]
// A choices field is a box to tick for each of options, sent as one comma
// list, for a flag that takes some of a fixed set (-category, -kind). A
// field with flag set marks its select or boxes with data-flag, so the
// parity check holds their choices to the ones poo -h lists (GUI-GREEN-048).
//   args(values) returns the argv; the preview updates as fields change.
// onResult, if given, is called with the finished job's {code, out, err}
// and the result area, so a view can act on the data rather than on the
// rendered page.
export function form(ctx, { title, about, fields, args, submit = 'Run', danger, onResult, onStart, secretOutput, gate: g }) {
  const area = el('div', { class: 'area' });
  const inputs = {};
  const labels = {};
  const f = el('form', { class: 'fields', autocomplete: 'off' });
  for (const fd of fields) {
    let input;
    if (fd.type === 'select') {
      input = el('select', { name: fd.name, 'data-flag': fd.flag }, ...fd.options.map(o => el('option', { value: o.value ?? o }, o.label ?? o)));
    } else if (fd.type === 'choices') {
      input = choices(fd.name, fd.options, fd.label, fd.flag);
    } else if (fd.type === 'check') {
      input = el('input', { type: 'checkbox', name: fd.name });
    } else {
      input = el('input', { name: fd.name, type: fd.type === 'number' ? 'text' : (fd.type || 'text'), inputmode: fd.type === 'number' ? 'numeric' : undefined, placeholder: fd.placeholder || '', spellcheck: 'false' });
    }
    if (fd.value !== undefined) input.value = fd.value;
    if (fd.list) input.setAttribute('list', fd.list);
    inputs[fd.name] = input;
    labels[fd.name] = fd.type === 'choices' ? input
      : el('label', { class: fd.type === 'check' ? 'check' : '' }, el('span', {}, fd.label), input, fd.help ? el('small', {}, fd.help) : null);
    f.append(labels[fd.name]);
  }
  // A field with only: {field, values} is shown, and sent, only while that
  // other field (an action select) holds one of values: the same pairs poo
  // checks with flagsFor, so a form never offers a flag poo would refuse.
  // only: {field, not} is the other way round: hidden while the field holds
  // one of not (a grind target of tip or pending takes no -until-from).
  // A check box's value is whether it is ticked: values: [true].
  const shown = fd => !fd.only || (fd.only.not ? !fd.only.not.includes(inputs[fd.only.field].value.trim())
    : fd.only.values.includes(inputs[fd.only.field].type === 'checkbox' ? inputs[fd.only.field].checked : inputs[fd.only.field].value));
  // A required field is required only while it is shown: one hidden by
  // only is not sent, so it cannot be missing (GUI-026).
  const showOnly = () => {
    for (const fd of fields) {
      labels[fd.name].hidden = !shown(fd);
      if (fd.required && 'required' in inputs[fd.name]) inputs[fd.name].required = shown(fd);
    }
  };
  const values = () => {
    const v = {};
    for (const fd of fields) {
      const i = inputs[fd.name];
      const on = shown(fd);
      v[fd.name] = fd.type === 'check' ? on && i.checked : fd.type === 'choices' ? (on ? i.value() : '') : fd.type === 'list' ? (on ? i.value.split(/[\s,]+/).filter(Boolean) : []) : on ? i.value.trim() : '';
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
    // A required field left empty runs nothing: dropped from the line, it
    // can make the command do something else, as "Build a rival claim"
    // without Parent submits the claim to the node and its peers
    // (GUI-026). The browser refuses it first; this holds if it does not.
    const v = values();
    const missing = fields.filter(fd => fd.required && shown(fd) && (v[fd.name] === '' || v[fd.name] === false || (Array.isArray(v[fd.name]) && !v[fd.name].length)));
    if (missing.length) {
      leave(area);
      area.replaceChildren(el('p', { class: 'run-status bad missing', role: 'alert' }, `Nothing was run: fill in ${missing.map(fd => fd.label).join(', ')}.`));
      inputs[missing[0].name].focus?.();
      return;
    }
    if (danger && !confirm(`Run ${api.quote(args(values()))}?`)) return;
    if (g) g.take();
    busy = submitBtn.disabled = true;
    runJob(args(values()), area, { secretOutput, onStart }).then(async r => {
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
    for (const [k, v] of Object.entries(vals)) if (inputs[k] && typeof inputs[k].value !== 'function') inputs[k].value = v;
    showOnly();
    line.set(args(values()));
    f.requestSubmit();
    sec.scrollIntoView({ block: 'start' });
  };
  // hint sets a field's placeholder, as a view that knows the current
  // value shows it (GREEN-033); an empty field still sends nothing.
  const hint = (name, text) => { if (inputs[name]) inputs[name].placeholder = text; };
  return { el: sec, form: f, inputs, stop: () => leave(area), run, hint };
}

// choices is a set of boxes, one for each option, in a fieldset whose
// value() is the ticked ones as a comma list: none ticked sends nothing,
// so poo's default (every one) holds. Watch's kinds use it too.
export function choices(name, options, legend, flagName) {
  const boxes = options.map(o => el('input', { type: 'checkbox', name, value: o }));
  const set = el('fieldset', { class: 'kinds', 'data-flag': flagName }, el('legend', {}, legend),
    ...options.map((o, i) => el('label', { class: 'check' }, boxes[i], ` ${o}`)));
  set.value = () => boxes.filter(b => b.checked).map(b => b.value).join(',');
  return set;
}

// pickFile adds "Or pick a file" under a form's file field. A file picked
// in the browser goes to poo on its standard input: the field becomes -,
// and feed(job) sends the file's text, then end of input, as poo reads it
// (ORANGE-014). Typing a path again drops the picked file. Nothing is
// stored by the GUI. get() is the file picked now, or null; onPick runs
// when one is picked. Every form that takes a file this way uses it
// (GUI-GREEN-052).
export function pickFile(f, { field = 'file', onPick } = {}) {
  const fileIn = f.inputs[field];
  let picked = null;
  const pick = el('input', { type: 'file', accept: '.json,application/json', name: field === 'file' ? 'pick' : `pick-${field}` });
  pick.onchange = () => {
    const file = pick.files && pick.files[0];
    if (!file) return;
    picked = file;
    if (onPick) onPick(file);
    fileIn.value = '-';
    fileIn.dispatchEvent(new Event('input', { bubbles: true }));
  };
  fileIn.addEventListener('input', () => { if (fileIn.value.trim() !== '-') { picked = null; pick.value = ''; } });
  fileIn.closest('label').after(el('label', {}, el('span', {}, 'Or pick a file'), pick, el('small', {}, 'Sent to poo as its input (- above); nothing is stored by the GUI')));
  return {
    get: () => picked,
    feed: (j, file = picked) => { if (file && fileIn.value.trim() === '-') api.inputFile(j.id, file).catch(() => {}); },
  };
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
