// Flag parity (GUI-GREEN-042): every flag poo CMD -h lists is one some
// form of the GUI can send, or is in LEFT_OUT with the reason it is not.
// GREEN-040 checks that every command is reachable; this checks a level
// deeper, so a flag Source adds cannot go missing from the GUI unnoticed.
//
// For each form on each view it fills every field (each choice of a
// select in turn, since a select can show other fields), reads the
// command line the form previews, and keeps the flags in it. It never
// presses a button, so nothing runs.
//
//   node gui/e2e/parity.mjs PATH/TO/poo-gui
import { spawnSync } from 'node:child_process';
import assert from 'node:assert/strict';
import { launch } from './lib.mjs';

const bin = process.argv[2];
const { page, env, errors, close } = await launch(bin);

// Flags every view sets in one place, or that the GUI has no use for.
const EVERYWHERE = {
  root: 'Settings sets it for every view', policy: 'Settings sets it for every view',
  dir: 'the GUI names a policy by -root and -policy instead',
  wallet: 'the Wallet view sets it once, above its forms', o: 'the GUI reads poo\'s JSON, the default',
  wide: 'widens poo\'s text tables; the GUI draws its own from the JSON',
  'passphrase-file': 'poo asks for the passphrase in a masked field instead (D106)',
  'new-passphrase-file': 'poo asks for the new passphrase in a masked field instead (D106)',
  'seed-file': 'a seed is typed into poo\'s masked prompt, never named as a file (D98)',
};
// Flags of one command the GUI leaves to the terminal, each with why.
// Grey can move any of them into a form; the check accepts either.
const LEFT_OUT = {
  node: { 'log-level': 'the live log is for reading; the terminal sets a level', 'log-format': 'the live log is for reading; the terminal sets a format' },
  sim: Object.fromEntries(['build-every', 'builders', 'degree', 'give-up', 'head-start', 'k', 'max-latency', 'min-latency', 'pay-every', 'share', 'split', 'threshold', 'tries', 'work-every'].map(f => [f, 'a simulator tuning flag; the form says poo sim load -h has them'])),
  'series new': Object.fromEntries(['entry-hash-credit', 'max-witness', 'min-entry-strength', 'min-layer-strength', 'pool-size', 'supply'].map(f => [f, 'the form says poo series new -h has the rest'])),
  demo: { quiet: 'Getting started runs demo -quiet; the GUI has no demo form' },
  keygen: { seed: 'for test vectors only, since it is on the command line (D98); the form has poo ask for the seed in a masked field (-seed-file -)' },
  watch: { once: 'the Watch view reconnects by itself, as poo watch without -once does' },
  // poo lists the shared filter flags on every view command, but refuses
  // -kind outside poo watch ("-kind selects events and applies to poo
  // watch only"), so no form offers it there.
  ...Object.fromEntries(['pools', 'pending', 'unspent', 'ledger', 'tx', 'address'].map(c => [c, { kind: 'poo refuses -kind outside poo watch' }])),
  tx: { kind: 'poo refuses -kind outside poo watch', ...Object.fromEntries(['address', 'address-policy', 'pool', 'serial', 'tid'].map(f => [f, 'one transaction by its id: In mint serial (-in-serial) is the narrowing that helps'])) },
  address: { kind: 'poo refuses -kind outside poo watch', address: 'the address is the command\'s own argument', 'address-policy': 'the address is the command\'s own argument' },
  'grind schedule': { 'if-version': 'add and set are applied as edits under the node\'s lock and keep changes made meanwhile (D125); Remove sends it' },
  'settings import': { apply: 'Apply, under the preview, runs the previewed line with -apply', 'if-digest': 'Apply sends the digest the preview printed (D151, D155); a preview has none to send' },
  'policy import': { apply: 'Apply, under the preview, runs the previewed line with -apply', 'if-digest': 'Apply sends the digest the preview printed (D151, GUI-023); a preview has none to send' },
  grind: { lease: 'a lease ends a job its client stops renewing; the GUI\'s Start leaves the job to the node, and Run is followed and stops with the page' },
};
// Commands whose subcommands share one set of flags, of which the GUI's
// action select shows each action only the ones poo takes for it (only:,
// GREEN-028): the family's forms together must cover the family's flags.
const FAMILIES = ['peer', 'sim', 'policy'];

const flagsOf = line => new Set(line.split(' ').filter(t => /^-[a-z]/.test(t)).map(t => t.slice(1)));
// commandOf is the command's words, before its first flag; x and 1 are
// what this check typed into fields, as a target can come before flags.
const commandOf = line => {
  const w = [];
  for (const t of line.split(' ').slice(1)) { if (t.startsWith('-')) break; if (t !== 'x' && t !== '1') w.push(t); }
  return w;
};

const seen = new Map(); // command words -> flags the GUI can send
const panelCmds = new Set(); // the first two words of each panel's line
const add = line => {
  const c = commandOf(line).join(' ');
  if (!seen.has(c)) seen.set(c, new Set());
  for (const f of flagsOf(line)) seen.get(c).add(f);
};

try {
  // Every view the navigation lists (gui/web/static/views/index.js), so a
  // new view is checked without being named here (GUI-GREEN-044).
  const views = await page.locator('nav a[data-id]').evaluateAll(as => as.map(a => a.dataset.id).filter(id => id !== 'terminal'));
  assert.ok(views.includes('wallet') && views.length > 8, 'the views were read from the navigation');
  for (const v of views) {
    await page.click(`nav a[data-id=${v}]`);
    await page.locator(`nav a[data-id=${v}][aria-current=page]`).waitFor();
    // In the page, so a field that shows once another is filled (only:)
    // is filled on the next pass, as a user would see it appear.
    const lines = await page.evaluate(() => {
      const out = [];
      // The Wallet view's file and series, set once above its forms.
      for (const i of document.querySelectorAll('#main .wallet-pick input')) { i.value = 'x'; i.dispatchEvent(new Event('input', { bubbles: true })); }
      for (const sec of document.querySelectorAll('#main section.panel')) {
        const form = sec.querySelector('form.fields');
        // A panel runs one fixed line; it counts for the subcommand check.
        if (!form) { for (const c of sec.querySelectorAll('.cmd code')) out.push('panel ' + c.textContent); continue; }
        const sel = form.querySelector('select');
        for (const choice of sel ? [...sel.options].map(o => o.value) : [null]) {
          if (sel) { sel.value = choice; form.dispatchEvent(new Event('input', { bubbles: true })); }
          for (let pass = 0; pass < 3; pass++) {
            // Any other select takes its last choice, which is not empty.
            for (const o of form.querySelectorAll('label:not([hidden]) select')) if (o !== sel) o.value = o.options[o.options.length - 1].value;
            for (const i of form.querySelectorAll('label:not([hidden]) input:not([type=file])')) {
              if (i.type === 'checkbox') i.checked = true;
              else i.value = i.inputMode === 'numeric' ? '1' : 'x';
            }
            form.dispatchEvent(new Event('input', { bubbles: true }));
          }
          for (const c of sec.querySelectorAll('.cmd code')) out.push(c.textContent);
        }
      }
      return out;
    });
    // A panel's line has no fields to fill, so only the subcommand check
    // counts it; the flag check is about what forms can send.
    lines.filter(l => !l.startsWith('panel ')).forEach(add);
    lines.filter(l => l.startsWith('panel ')).forEach(l => panelCmds.add(commandOf(l.slice(6)).slice(0, 2).join(' ')));
  }

  // A field that states poo's default in its placeholder ("default 16")
  // must state the one poo -h prints for its flag, so the GUI's hint
  // cannot drift from poo (GUI-GREEN-043). Each such field is filled
  // alone, so the preview says which flag it is.
  const stated = [];
  for (const v of views) {
    await page.click(`nav a[data-id=${v}]`);
    await page.locator(`nav a[data-id=${v}][aria-current=page]`).waitFor();
    stated.push(...await page.evaluate(() => {
      const out = [];
      for (const sec of document.querySelectorAll('#main section.panel')) {
        const form = sec.querySelector('form.fields');
        if (!form) continue;
        const sel = form.querySelector('select');
        for (const choice of sel ? [...sel.options].map(o => o.value) : [null]) {
          if (sel) sel.value = choice;
          for (const i of form.querySelectorAll('input')) { if (i.type === 'checkbox') i.checked = false; else i.value = ''; }
          form.dispatchEvent(new Event('input', { bubbles: true }));
          for (const i of form.querySelectorAll('label:not([hidden]) input')) {
            // Only a stated value: a number, list or name, not prose.
            const m = /\bdefault:? ([0-9][\w.,:-]*|ed25519|sha256|auto)/.exec(i.placeholder || '');
            if (!m) continue;
            i.value = '7777';
            form.dispatchEvent(new Event('input', { bubbles: true }));
            out.push({ line: sec.querySelector('.cmd code').textContent, said: m[1].replace(/,$/, ''), placeholder: i.placeholder });
            i.value = '';
            form.dispatchEvent(new Event('input', { bubbles: true }));
          }
        }
      }
      return out;
    }));
  }
  const drift = [];
  for (const s of stated) {
    const t = s.line.split(' ');
    const k = t.indexOf('7777');
    if (k < 1 || !t[k - 1].startsWith('-')) continue;
    const fl = t[k - 1].slice(1);
    const cmd = commandOf(s.line.replace(' 7777', ''));
    const r = spawnSync(bin, ['poo', ...cmd, '-h'], { env });
    const m = new RegExp(`^ {2}-${fl}(?: \\w+)?\n\\s+(.*)$`, 'm').exec(`${r.stdout}${r.stderr}`);
    // Go's flag help leaves a zero default out.
    const poo = (m && /\(default "?([^")]*)"?\)$/.exec(m[1]) || [, '0'])[1];
    if (poo !== s.said) drift.push(`poo ${cmd.join(' ')} -${fl}: the GUI says default ${s.said}, poo says ${poo}`);
  }
  assert.ok(stated.length > 5, 'fields that state a default were found');
  assert.deepEqual([...new Set(drift)], [], 'a stated default is poo\'s');

  // Every subcommand poo lists is run by some view, or LEFT_OUT_SUBS says
  // why not. An action select is a copy of poo's list of subcommands, and
  // a new action would carry flags the family already has, so the flag
  // check alone would miss it (GUI-GREEN-045). The lists come from poo
  // help ("poo peer timeout|block|…", "poo policy export …") and from the
  // help of each command that has its own (poo grind help, poo wallet help).
  const LEFT_OUT_SUBS = {
    'completion bash': 'sets up a shell, not the GUI', 'completion zsh': 'sets up a shell, not the GUI', 'completion fish': 'sets up a shell, not the GUI',
  };
  const helpOf = args => { const r = spawnSync(bin, ['poo', ...args], { env }); return `${r.stdout}${r.stderr}`; };
  const subs = new Set();
  for (const m of helpOf(['help']).matchAll(/^ {2}poo ([a-z]+) ([a-z][a-z|-]*)(?= |$)/gm)) {
    for (const sub of m[2].split('|')) subs.add(`${m[1]} ${sub}`);
  }
  for (const cmd of ['grind', 'wallet']) {
    for (const m of helpOf([cmd, 'help']).matchAll(/^ {2}([a-z]+)(?= )/gm)) subs.add(`${cmd} ${m[1]}`);
  }
  assert.ok(subs.has('peer block') && subs.has('grind schedule') && subs.has('wallet pay'), 'the subcommands were read from poo help');
  const ranSubs = new Set([...seen.keys()].map(c => c.split(' ').slice(0, 2).join(' ')).concat([...panelCmds]));
  const missingSubs = [...subs].filter(c => !ranSubs.has(c) && !LEFT_OUT_SUBS[c]).map(c => `poo ${c}`);
  assert.deepEqual(missingSubs, [], 'every subcommand poo lists is run by a view, or LEFT_OUT_SUBS says why not');

  const gaps = [];
  for (const [cmd, sent] of seen) {
    if (!cmd) continue;
    // poo prints a command's flags to standard error for -h.
    const r = spawnSync(bin, ['poo', ...cmd.split(' '), '-h'], { env });
    const help = `${r.stdout}${r.stderr}`;
    const listed = [...help.matchAll(/^ {2}-([a-z][a-z0-9-]*)/gm)].map(m => m[1]);
    const words = cmd.split(' ');
    const left = Object.assign({}, ...words.map((_, n) => LEFT_OUT[words.slice(0, n + 1).join(' ')] || {}));
    if (FAMILIES.includes(words[0])) {
      for (const [c, fl] of seen) if (c.split(' ')[0] === words[0]) fl.forEach(f => sent.add(f));
    }
    for (const fl of listed) if (!sent.has(fl) && !EVERYWHERE[fl] && !left[fl]) gaps.push(`poo ${cmd} -${fl}`);
  }
  assert.deepEqual(gaps, [], 'every flag poo lists is sent by a form, or LEFT_OUT says why not');
  assert.deepEqual(errors, [], 'no page errors');
  console.log(`gui parity e2e: ok (${seen.size} commands)`);
} finally {
  await close();
}
