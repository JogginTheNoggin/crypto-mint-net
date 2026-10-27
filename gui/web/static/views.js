// Views. Each panel runs one poo command and renders what it prints; each
// form builds one poo command line from its fields and runs it. Nothing
// here computes anything a command does not print, and every panel shows
// its command line, which runs the same in the terminal (docs/gui.md).
import * as api from './api.js';
import { render } from './render.js';
import * as where from './where.js';
import { hintFor } from './hints.js';

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
// an input field whenever the command reads input (masked unless the
// user shows it), and the result rendered once it ends. It resolves to
// {code, out, err}.
export function runJob(args, area, { onStart, empty } = {}) {
  area.textContent = '';
  const status = el('div', { class: 'run-status live' }, 'running…');
  const live = el('pre', { class: 'live-out' });
  const ask = el('form', { class: 'ask', hidden: true });
  const askLabel = el('label', {});
  // The answer is masked unless the user shows it: which prompts read a
  // secret is not guessed from the output (GUI-003).
  const askInput = el('input', { type: 'password', autocomplete: 'off', spellcheck: 'false' });
  const showBtn = el('button', { type: 'button', class: 'small', 'aria-pressed': 'false', title: 'Show what you type' }, 'Show');
  showBtn.onclick = () => {
    const show = askInput.type === 'password';
    askInput.type = show ? 'text' : 'password';
    showBtn.setAttribute('aria-pressed', String(show));
    askInput.focus();
  };
  const stopBtn = el('button', { type: 'button' }, 'Stop');
  ask.append(askLabel, askInput, showBtn, el('button', { type: 'submit', class: 'primary' }, 'Send'));
  area.append(el('div', { class: 'run-bar' }, status, stopBtn), ask, live);
  return new Promise(resolve => {
    api.start(args).then(j => {
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
        api.input(j.id, v).catch(e => { err += `poo-gui: ${e.message}\n`; });
      };
      api.follow(j.id, {
        chunk: c => {
          if (c.stream === 'err') err += c.data; else out += c.data;
          tail = (tail + c.data).slice(-80);
          const s = el('span', { class: c.stream === 'err' ? 'err' : '' });
          s.textContent = c.data;
          live.append(s);
        },
        state: s => {
          ask.hidden = !s.waiting;
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
          area.append(showResult(out, err, { empty }));
          resolve({ code: e.code, out, err });
        },
        error: () => { status.textContent = 'lost the connection to poo-gui'; resolve({ code: -1, out, err }); },
      });
    }).catch(e => {
      status.className = 'run-status bad';
      status.textContent = e.message;
      resolve({ code: -1, out: '', err: e.message });
    });
  });
}

// showResult renders a command's output: JSON as tables, anything else as
// the text the command printed. Errors are shown as printed, with a next
// step under them when one is known (hints.js). empty is what to say when
// the command printed nothing.
export function showResult(out, err, { empty } = {}) {
  const box = el('div', { class: 'result' });
  if (out.trim()) {
    let v, ok = false;
    try { v = JSON.parse(out); ok = true; } catch (_) { /* text output */ }
    box.append(ok ? render(v) : el('pre', { class: 'text-out' }, out));
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
function cmdLine(args, ctx) {
  const code = el('code', {}, api.quote(args));
  const row = el('div', { class: 'cmd' }, el('span', { class: 'cmd-label' }, 'Runs'), code,
    el('button', { type: 'button', class: 'small', title: 'Type this command into the terminal and run it', onclick: () => ctx.openInTerminal(api.quote(row.args).replace(/^poo /, '')) }, 'Run in terminal'));
  row.args = args;
  row.set = a => { row.args = a; code.textContent = api.quote(a); };
  return row;
}

// panel runs args() when shown (and every refresh ms) and renders it.
function panel(ctx, { title, about, args, refresh, empty }) {
  const area = el('div', { class: 'area' });
  const line = cmdLine(args(), ctx);
  const btn = el('button', { type: 'button' }, 'Refresh');
  const sec = el('section', { class: 'panel' },
    el('div', { class: 'panel-head' }, el('h2', {}, title), btn), about ? el('p', { class: 'hint' }, about) : null, line, area);
  let busy = false;
  const go = async () => {
    if (busy) return;
    busy = true;
    const a = args();
    line.set(a);
    await runJob(a, area, { empty });
    busy = false;
  };
  btn.onclick = go;
  go();
  let t = refresh ? setInterval(go, refresh) : null;
  return { el: sec, stop: () => t && clearInterval(t) };
}

// form builds a command line from fields and runs it on submit.
//   fields: [{name, label, type: text|number|password|select|check|list, flag, options, placeholder, required, help}]
//   args(values) returns the argv; the preview updates as fields change.
function form(ctx, { title, about, fields, args, submit = 'Run', danger }) {
  const area = el('div', { class: 'area' });
  const inputs = {};
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
    inputs[fd.name] = input;
    f.append(el('label', { class: fd.type === 'check' ? 'check' : '' }, el('span', {}, fd.label), input, fd.help ? el('small', {}, fd.help) : null));
  }
  const values = () => {
    const v = {};
    for (const fd of fields) {
      const i = inputs[fd.name];
      v[fd.name] = fd.type === 'check' ? i.checked : fd.type === 'list' ? i.value.split(/[\s,]+/).filter(Boolean) : i.value.trim();
    }
    return v;
  };
  const line = cmdLine(args(values()), ctx);
  f.addEventListener('input', () => line.set(args(values())));
  f.append(el('div', { class: 'actions' }, el('button', { type: 'submit', class: danger ? 'danger' : 'primary' }, submit)));
  f.addEventListener('submit', ev => {
    ev.preventDefault();
    if (danger && !confirm(`Run ${api.quote(args(values()))}?`)) return;
    runJob(args(values()), area);
  });
  const sec = el('section', { class: 'panel' }, el('div', { class: 'panel-head' }, el('h2', {}, title)),
    about ? el('p', { class: 'hint' }, about) : null, f, line, area);
  return { el: sec, stop: () => {} };
}

// flag adds -name value when value is set.
const flag = (name, value) => (value === '' || value === undefined || value === false) ? [] : value === true ? [`-${name}`] : [`-${name}`, value];
const flags = (name, list) => list.flatMap(v => [`-${name}`, v]);

function mountAll(root, title, intro, parts) {
  root.append(el('h1', {}, title), intro ? el('p', { class: 'hint' }, intro) : null);
  for (const p of parts) root.append(p.el);
  return () => parts.forEach(p => p.stop());
}

// node finds the node job this GUI is running, if any.
async function nodeJob() {
  const js = await api.jobs();
  return js.reverse().find(j => !j.done && j.args[0] === 'node');
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
    let following = null;
    const showRunning = async () => {
      const j = await nodeJob().catch(() => null);
      startBtn.disabled = !!j;
      if (j && following !== j.id) {
        following = j.id;
        runArea.textContent = '';
        const log = el('pre', { class: 'live-out' });
        const stop = el('button', { type: 'button', class: 'danger' }, 'Stop node');
        stop.onclick = () => api.cancel(j.id);
        runArea.append(el('div', { class: 'run-bar' }, el('span', { class: 'run-status live' }, `running: ${j.command}`), stop), log);
        api.follow(j.id, {
          chunk: c => { const s = el('span', { class: c.stream === 'err' ? 'err' : '' }); s.textContent = c.data; log.append(s); log.scrollTop = log.scrollHeight; },
          exit: e => { following = null; startBtn.disabled = false; runArea.prepend(el('div', { class: 'run-status ' + (e.code ? 'bad' : 'ok') }, `node stopped (exit ${e.code})`)); stop.remove(); },
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
    const status = panel(ctx, { title: 'Followed pools', about: 'What the running node follows, and where each pool stands.', args: () => ['status', ...where.flags()], refresh: 5000 });
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
    const t = setInterval(showRunning, 3000);
    const stop = mountAll(root, 'Node', 'Run the full node and see what it follows.', [{ el: runSec, stop: () => clearInterval(t) }, status, init]);
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
      panel(ctx, { title: 'Followed pools', args: () => ['status', ...where.flags()] }),
      form(ctx, { title: 'Holdings of a mint', about: 'Unspent portions of a mint.', fields: [serial, series],
        args: v => ['holdings', ...where.flags(), ...flag('serial', v.serial), ...flag('series', v.series)], submit: 'Look up' }),
      form(ctx, { title: 'Where a payment stands', about: 'A transaction, the weight behind it, and known rivals.',
        fields: [serial, { name: 'tid', label: 'Transaction id', placeholder: 'hex' }, series],
        args: v => ['payment', ...where.flags(), ...flag('serial', v.serial), ...flag('tid', v.tid), ...flag('series', v.series)], submit: 'Look up' }),
      form(ctx, { title: 'Build an entry', about: 'Builds an entry from the pending transactions of a pool.',
        fields: [{ name: 'pool', label: 'Pool', type: 'number' }, { name: 'work', label: 'Layer tries', type: 'number', placeholder: 'optional' }, series],
        args: v => ['build', ...where.flags(), ...flag('pool', v.pool), ...flag('work', v.work), ...flag('series', v.series)], submit: 'Build' }),
      form(ctx, { title: 'Add a layer', about: 'Adds the strongest layer found, by default on the selected tip.',
        fields: [{ name: 'pool', label: 'Pool', type: 'number' }, { name: 'entry', label: 'Entry hash', placeholder: 'default: the selected tip' }, { name: 'tries', label: 'Tries', type: 'number' }, series],
        args: v => ['work', ...where.flags(), ...flag('pool', v.pool), ...flag('entry', v.entry), ...flag('tries', v.tries), ...flag('series', v.series)], submit: 'Work' }),
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
        { name: 'seconds', label: 'Seconds', type: 'number', placeholder: 'timeout/block; -1 blocks until unblocked' },
        { name: 'range', label: 'Whole range (block, unblock)', type: 'check' },
        { name: 'what', label: 'Clear', type: 'select', options: ['', 'flags', 'measures', 'all'] },
        { name: 'template', label: 'Tracking template', placeholder: 'template name' },
      ], args: v => ['peer', v.action, ...where.flags(), ...flag('seconds', v.seconds), ...flag('range', v.range), ...flag('what', v.what), ...flag('template', v.template), ...(v.addr ? [v.addr] : [])],
      submit: 'Apply', danger: true }),
      panel(ctx, { title: 'Tracking', about: "Every tracked address's status and the blocked ranges.", args: () => ['tracking', ...where.flags()] }),
      form(ctx, { title: "One peer's record", fields: [{ name: 'addr', label: 'Peer address', placeholder: 'host:port' }],
        args: v => ['tracking', ...where.flags(), ...(v.addr ? [v.addr] : [])], submit: 'Look up' }),
      panel(ctx, { title: 'Chases', about: 'Heavier stated weights waiting or being checked.', args: () => ['chases', ...where.flags()] }),
    ]);
  },
};

const walletView = {
  id: 'wallet',
  title: 'Wallet',
  mount(root, ctx) {
    const w = { name: 'wallet', label: 'Wallet file', placeholder: 'default: wallet.json in the root' };
    const wf = v => [...where.flags(), ...flag('wallet', v.wallet)];
    const series = { name: 'series', label: 'Series id', placeholder: 'only if the node follows more than one' };
    const pass = 'The command asks for the passphrase when it needs it.';
    return mountAll(root, 'Wallet', 'Keys from one seed, payments and requests, all through poo wallet. Passphrases go straight to the command and are never stored.', [
      form(ctx, { title: 'Balance', about: 'Spendable units per watched mint. ' + pass, fields: [w, series], args: v => ['wallet', 'balance', ...wf(v), ...flag('series', v.series)], submit: 'Show balance' }),
      form(ctx, { title: 'Addresses', about: 'Every address, used or not, and shared ones. ' + pass, fields: [w], args: v => ['wallet', 'addresses', ...wf(v)], submit: 'Show addresses' }),
      form(ctx, { title: 'New address', fields: [w, { name: 'label', label: 'Label' }], args: v => ['wallet', 'address', ...wf(v), ...flag('label', v.label)], submit: 'Give out an address' }),
      form(ctx, { title: 'Pay', about: 'Pays an address, or a payment request file.', fields: [w,
        { name: 'to', label: 'To address' }, { name: 'amount', label: 'Amount', type: 'number' },
        { name: 'request', label: 'Or a request file' }, { name: 'mint', label: 'Only from mints', type: 'list', placeholder: 'serials …' },
        { name: 'memo', label: 'Memo' }, { name: 'out', label: 'Co-signing file', placeholder: 'for a shared policy' }, series],
      args: v => ['wallet', 'pay', ...wf(v), ...flag('to', v.to), ...flag('amount', v.amount), ...flag('request', v.request), ...flags('mint', v.mint), ...flag('memo', v.memo), ...flag('out', v.out), ...flag('series', v.series)],
      submit: 'Pay', danger: true }),
      form(ctx, { title: 'Request a payment', about: 'Asks to be paid, to a fresh address.', fields: [w, { name: 'amount', label: 'Amount', type: 'number' }, { name: 'memo', label: 'What for' }, { name: 'out', label: 'Request file', placeholder: 'default: print it' }],
        args: v => ['wallet', 'request', ...wf(v), ...flag('amount', v.amount), ...flag('memo', v.memo), ...flag('out', v.out)], submit: 'Make request' }),
      form(ctx, { title: 'What has arrived', fields: [w, { name: 'address', label: 'Requested address' }, { name: 'request', label: 'Or the request file' }, { name: 'threshold', label: 'Threshold', placeholder: 'from(e) for settled' }],
        args: v => ['wallet', 'receive', ...wf(v), ...flag('address', v.address), ...flag('request', v.request), ...flag('threshold', v.threshold)], submit: 'Check' }),
      form(ctx, { title: 'Payments sent', about: 'Where every payment sent stands.', fields: [w, { name: 'threshold', label: 'Threshold', placeholder: 'from(e) for settled' }],
        args: v => ['wallet', 'status', ...wf(v), ...flag('threshold', v.threshold)], submit: 'Show' }),
      form(ctx, { title: 'Watch mints', about: 'Adds mints or pools to look for portions in.', fields: [w, { name: 'serial', label: 'Mint serials', type: 'list' }, { name: 'pool', label: 'Pools', type: 'list' }],
        args: v => ['wallet', 'watch', ...wf(v), ...flags('serial', v.serial), ...flags('pool', v.pool)], submit: 'Watch' }),
      form(ctx, { title: 'Claim a mint', fields: [w, { name: 'serial', label: 'Mint serial', type: 'number' }, series],
        args: v => ['wallet', 'claim', ...wf(v), ...flag('serial', v.serial), ...flag('series', v.series)], submit: 'Claim', danger: true }),
      form(ctx, { title: 'Create or restore a wallet', fields: [w, { name: 'alg', label: 'Key algorithm', placeholder: 'default ed25519 (see Crypto)' }, { name: 'restore', label: 'Restore from seed', placeholder: 'hex, to restore', help: 'Part of the command line, as in a shell: it shows in the preview and in Jobs.' }, { name: 'gap', label: 'Gap', type: 'number' }],
        args: v => ['wallet', 'create', ...wf(v), ...flag('alg', v.alg), ...flag('restore', v.restore), ...flag('gap', v.gap)], submit: 'Create' }),
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
        { name: 'fresh', label: 'Unarchive fresh (drop stored data)', type: 'check' }],
      args: v => ['policy', v.sub, ...rootFlag(), ...flag('fresh', v.fresh), ...(v.name ? [v.name] : [])], submit: 'Apply', danger: true }),
    ]);
  },
};

const cryptoView = {
  id: 'crypto',
  title: 'Series and keys',
  mount(root, ctx) {
    return mountAll(root, 'Series and keys', 'Offline tools: the choices a series can make, keys, and series parameters.', [
      panel(ctx, { title: 'Hashes, key algorithms and policy types', args: () => ['crypto'] }),
      form(ctx, { title: 'Make a key', about: 'A key and its 1-of-1 address. Leave the seed empty for a random one; the output includes the seed.',
        fields: [{ name: 'alg', label: 'Key algorithm', placeholder: 'ed25519' }, { name: 'hash', label: 'Hash', placeholder: 'sha256' }, { name: 'seed', label: 'Seed', placeholder: 'hex, optional', help: 'Part of the command line, as in a shell.' }],
        args: v => ['keygen', ...flag('alg', v.alg), ...flag('hash', v.hash), ...flag('seed', v.seed)], submit: 'Make key' }),
      form(ctx, { title: 'New series parameters', about: 'Prints new series parameters, checked. More options are in the terminal (poo series new -h).',
        fields: [{ name: 'hash', label: 'Hash', placeholder: 'sha256' }, { name: 'algs', label: 'Key algorithms', placeholder: 'comma list' }, { name: 'types', label: 'Policy types', placeholder: 'comma list' }, { name: 'mints', label: 'Mints', type: 'number' }],
        args: v => ['series', 'new', ...flag('hash', v.hash), ...flag('algs', v.algs), ...flag('types', v.types), ...flag('mints', v.mints)], submit: 'Make series' }),
      form(ctx, { title: 'Series file info', fields: [{ name: 'file', label: 'Series file', placeholder: 'series.json' }],
        args: v => ['series', 'info', ...(v.file ? [v.file] : [])], submit: 'Show' }),
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
    async function refresh() {
      try { js = await api.jobs(); } catch (e) { list.textContent = e.message; return; }
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
          el('button', { type: 'button', onclick: () => showJob(j) }, 'Output'),
          j.done ? null : el('button', { type: 'button', onclick: () => api.cancel(j.id).then(refresh) }, 'Stop'),
          el('button', { type: 'button', onclick: () => ctx.openInTerminal(j.command.replace(/^poo /, '')) }, 'Run again in terminal')));
      }
    }
    function showJob(j) {
      if (unfollow) unfollow();
      detail.textContent = '';
      unfollow = api.follow(j.id, {
        chunk: c => {
          const s = el('span', { class: c.stream === 'err' ? 'err' : '' });
          s.textContent = c.data;
          detail.append(s);
        },
        exit: refresh,
      });
    }
    refresh();
    const t = setInterval(refresh, 3000);
    return () => { clearInterval(t); if (unfollow) unfollow(); };
  },
};

export const views = [nodeView, poolsView, walletView, peersView, policiesView, cryptoView, jobsView, settingsView];
