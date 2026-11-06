// The Pools and payments view. Each view is one file here, listed in index.js (GUI-GREEN-044).
import * as where from '../where.js';
import { el, flag, form, mountAll, panel, pickFile, rawFold } from '../views.js';
import { saveAs } from './policies.js';

// grindFields are the flags build and work share with poon grind: which
// device grinds, with how many threads, and for how long instead of a
// number of tries (GUI-GREEN-041). Device suggests what poon grind devices
// listed, once the Work view has run it.
const grindFields = () => [
  { name: 'device', label: 'Device', placeholder: 'auto, cpu, gpu:0 (default: node.json, else auto)', list: 'grind-devices' },
  { name: 'threads', label: 'Threads', type: 'number', placeholder: 'default: every core but one' },
  { name: 'for', label: 'For', placeholder: 'grind this long, up to 4m (30s), instead of tries' },
];
const grindFlags = v => [...flag('device', v.device), ...flag('threads', v.threads), ...flag('for', v.for)];

// ledgerSummary says what a ledger file poon printed holds, or returns
// null when the text is not one whole ledger file: one that does not
// parse, is not poon's ledger kind, or holds fewer entries than its own
// from and to say (GUI-GREEN-059). A format other than 1 gets a neutral
// line (White R34D N2). It only reads poon's fields; the file
// is checked in full by the node that imports it.
export function ledgerSummary(text) {
  let f;
  try { f = JSON.parse(text); } catch (_) { return null; }
  if (!f || f.kind !== 'poon-ledger') return null;
  // A later format may lay the file out differently: say so plainly and
  // still offer it to save, rather than call a good file cut (White N2).
  if (Number.isInteger(f.format) && f.format !== 1) return `A ledger file in format ${f.format}, which this poon-gui does not read; poon checks it in full on import.`;
  if (!Array.isArray(f.entries)) return null;
  const n = f.entries.length;
  if (n !== (f.to >= f.from ? f.to - f.from + 1 : 0)) return null;
  return n ? `A ledger file of pool ${f.pool}: ${n} ${n === 1 ? 'entry' : 'entries'}, heights ${f.from} to ${f.to} of ${f.height}.`
    : `A ledger file of pool ${f.pool} with no entries: the selected ledger's height is ${f.height}.`;
}

// exportResult draws a ledger export's result: one whole ledger file is
// said in a line, with Save as file and poon's output folded under Output
// as printed (GUI-GREEN-059); anything else is folded too, never offered
// to save, and gets one alert.
export function exportResult(r, area) {
  if (r.code !== 0 || !r.out.trim().startsWith('{')) return;
  const said = ledgerSummary(r.out);
  const raw = area.querySelector('.result');
  if (!said) {
    // Output poon-gui dropped in part gets ORANGE-026's alert, which
    // says the same; one alert is enough. Either way the text is not
    // drawn until asked for: it cannot be saved (GUI/Orange R34B).
    if (!r.dropped) area.prepend(el('p', { class: 'next-step ledger-partial', role: 'alert' }, 'This output is not one whole ledger file, so it is not offered to save. Fill in To a file and export again: poon then writes the file itself.'));
    if (raw) raw.parentNode.insertBefore(rawFold(raw), raw.nextSibling);
    return;
  }
  const note = el('p', { class: 'hint ledger-exported', role: 'status' }, said);
  if (raw) { raw.replaceWith(note); area.append(saveAs(r.out, 'ledger.json'), rawFold(raw)); } else area.append(note, saveAs(r.out, 'ledger.json'));
}

// importNote is the line over poon ledger import's reply: what a -dry-run
// read (D189), what a finished import kept, or, when it stopped early, the
// totals so far as an alert (GUI-031). null when there is nothing to say.
export function importNote(r) {
  let o = null;
  try { o = JSON.parse(r.out); } catch (_) { /* only an error was printed */ }
  // -dry-run: what the file holds and any problems poon found, read
  // without the node (D189).
  if (o && typeof o.decoded === 'number') {
    const k = o.problem_count ?? (o.problems || []).length;
    return (el('p', { class: k ? 'next-step ledger-checked' : 'hint ledger-checked', role: 'status' },
      `Checked without the node: pool ${o.pool}, ${o.decoded} of ${o.entries} entries read${o.series_known ? '' : '; this node does not know the series, so entry hashes were not worked out'}${k ? `; ${k} ${k === 1 ? 'problem' : 'problems'}: ${(o.problems || []).slice(0, 5).join('; ')}${k > Math.min((o.problems || []).length, 5) ? '; …' : ''}` : '; no problems found'}. Nothing was kept.`));
  }
  const totals = o && typeof o.kept === 'number' ? `kept ${o.kept} entries and ${o.layers_kept ?? 0} layers; ${o.known ?? 0} entries and ${o.layers_known ?? 0} layers were already known` : '';
  const refused = o && Array.isArray(o.refused) ? o.refused : [];
  const more = `${refused.length ? `; ${refused.length} refused (listed below)` : ''}${o && o.switched ? '; the node switched to the imported branch' : ''}`;
  // poon sends the file in batches, and the node keeps each one (and
  // may switch to the imported branch) before the next is sent. An
  // import that ends early says why and the totals so far (GUI-031):
  // what was sent is kept or known, and the rest was not sent.
  // With no totals, nothing was sent (poon prints them once a batch
  // is), so poon's error says it all.
  if (r.code !== 0 || (o && o.stopped)) {
    if (totals) return (el('p', { class: 'next-step ledger-import-partial', role: 'alert' },
      `The import stopped after ${o.sent ?? '?'} entries were sent${o.stopped ? ` (${o.stopped})` : ''}: ${totals}${more}. Importing the same file again is safe, as entries already kept count as known.`));
    return null;
  }
  if (!totals) return null;
  return (el('p', { class: refused.length ? 'next-step ledger-imported' : 'hint ledger-imported', role: 'status' },
    `${totals.charAt(0).toUpperCase()}${totals.slice(1)}${more}.`));
}

export const poolsView = {
  id: 'pools',
  title: 'Pools and payments',
  mount(root, ctx) {
    const serial = { name: 'serial', label: 'Mint serial', type: 'number', required: true };
    const series = { name: 'series', label: 'Series id', placeholder: 'only if the node follows more than one' };
    // -parent (D161): build on a chosen entry instead of the selected tip.
    const parent = { name: 'parent', label: 'Parent', placeholder: 'an entry hash, root, or before-claim with a mint serial (default: the selected tip)' };
    // poon ledger export and import (D188): a pool's entries and layers as
    // one file, to carry history to another node. Import checks every
    // entry as if it came from a peer; what it keeps and refuses is poon's
    // reply, drawn as it printed it.
    const ledgerExport = form(ctx, { title: 'Export a pool\'s ledger', about: 'Writes a pool\'s entries and layers, from one height to another, as a ledger file another node can import.',
      fields: [{ name: 'pool', label: 'Pool', type: 'number', required: true }, series, { name: 'from', label: 'From height', type: 'number', placeholder: 'default 1' }, { name: 'to', label: 'To height', type: 'number', placeholder: 'default: the tip' }, { name: 'out', label: 'To a file', placeholder: 'a path on this machine (default: show it here, with Save as file)' }],
      args: v => ['ledger', 'export', ...where.flags(), ...flag('pool', v.pool), ...flag('series', v.series), ...flag('from', v.from), ...flag('to', v.to), ...flag('out', v.out)], submit: 'Export',
      // The file is for Save as file and another node, not for reading:
      // drawn, a ledger of 512 post-quantum entries is 5 MB of base64 and
      // 6,500 page elements (GUI-GREEN-059). The page says what it holds
      // and keeps poon's output under Output as printed, drawn when opened.
      // A text that is not one whole ledger file is never offered to save.
      onResult: exportResult });
    let picker = null;
    const ledgerImport = form(ctx, { title: 'Import a ledger', about: 'Gives the node the entries and layers of a ledger file. Each is checked as if a peer had sent it: valid ones are kept, the rest are listed with poon\'s reason, and the node switches to a heavier branch only as it would for a peer.',
      fields: [{ name: 'file', label: 'Ledger file', placeholder: 'a path on this machine, or pick one below', required: true },
        // -dry-run (D189): poon reads and checks the file without the node.
        { name: 'dryRun', label: 'Only check the file: read it and say what it holds, without the node (nothing is kept)', type: 'check' }],
      args: v => ['ledger', 'import', ...where.flags(), ...flag('dry-run', v.dryRun), ...(v.file ? [v.file] : [])], submit: 'Import', danger: true,
      onStart: j => picker.feed(j),
      onResult: (r, area) => { const n = importNote(r); if (n) area.prepend(n); } });
    picker = pickFile(ledgerImport);
    return mountAll(root, 'Pools and payments', 'Look up mints and transactions, and build entries and layers.', [
      // poon pools prints a row for every pool: on the reserve series 380 KB
      // and 500 drawn rows each time this view opened, before any form
      // could be used. It runs when Show is pressed (GUI-GREEN-057), as
      // the Node view's Each pool does (GUI-GREEN-056).
      panel(ctx, { title: 'Followed pools', about: 'Each followed pool: tip, height, weight, waiting and stored records (poon pools). It runs when you press Show.', args: () => ['pools', ...where.flags()], start: false }),
      // poon unspent and poon tx replace the older holdings and payment
      // (Source GREEN-008): the same answers, with the pool, the height and
      // the decoded transaction, and no serial needed for a transaction.
      form(ctx, { title: 'Holdings of a mint', about: 'Unspent portions of a mint.', fields: [serial, series],
        args: v => ['unspent', ...where.flags(), ...flag('serial', v.serial), ...flag('series', v.series)], submit: 'Look up' }),
      form(ctx, { title: 'Where a payment stands', about: 'A transaction, the weight behind it, and known rivals.',
        fields: [{ name: 'tid', label: 'Transaction id', placeholder: 'hex' }, { ...serial, required: false, label: 'In mint serial', placeholder: 'optional: look only in its pool' }, series],
        args: v => ['tx', ...where.flags(), ...flag('in-serial', v.serial), ...flag('series', v.series), ...(v.tid ? [v.tid] : [])], submit: 'Look up' }),
      form(ctx, { title: 'Build an entry', about: 'Builds an entry from the pending transactions of a pool.',
        fields: [{ name: 'pool', label: 'Pool', type: 'number' }, { ...serial, required: false, placeholder: 'or a mint serial' }, { name: 'work', label: 'Layer tries', type: 'number', placeholder: 'optional' }, parent, series, ...grindFields(), { name: 'maxTries', label: 'Most entry tries', type: 'number', placeholder: 'when the series sets a minimum entry strength' }, { name: 'bytes', label: 'Also print the hex, for Submit on another node', type: 'check' }],
        args: v => ['build', ...where.flags(), ...flag('pool', v.pool), ...flag('serial', v.serial), ...flag('parent', v.parent), ...flag('work', v.work), ...flag('series', v.series), ...grindFlags(v), ...flag('max-tries', v.maxTries), ...flag('bytes', v.bytes)], submit: 'Build' }),
      // A rival claim (D161): the mint's claim built into an entry on a
      // chosen parent instead of submitted, so it can be grinded to
      // compete with the entry holding the claim now. The seed is typed
      // into poon's masked prompt (-seed-file -), never put on the line.
      form(ctx, { title: 'Build a rival claim', about: 'Builds a claim of a mint into an entry on the parent you choose, such as the one before the entry holding the claim now, and prints its hash and the command to grind it. poon asks for the claiming key\'s seed in a masked field.',
        fields: [{ ...serial }, { ...parent, required: true, placeholder: 'before-claim, root, or an entry hash' }, series, { name: 'work', label: 'Layer tries', type: 'number', placeholder: 'optional: then add the strongest layer found' }, { name: 'alg', label: 'Key algorithm', placeholder: 'ed25519', list: 'crypto-algs', help: 'The claiming key\'s algorithm (-alg).' }, { name: 'maxTries', label: 'Most entry tries', type: 'number', placeholder: 'when the series sets a minimum entry strength' }],
        args: v => ['claim', ...where.flags(), '-seed-file', '-', ...flag('serial', v.serial), ...flag('parent', v.parent), ...flag('series', v.series), ...flag('work', v.work), ...flag('max-tries', v.maxTries), ...flag('alg', v.alg)], submit: 'Build claim' }),
      form(ctx, { title: 'Add a layer', about: 'Adds the strongest layer found, by default on the selected tip.',
        fields: [{ name: 'pool', label: 'Pool', type: 'number' }, { ...serial, required: false, placeholder: 'or a mint serial' }, { name: 'entry', label: 'Entry hash', placeholder: 'default: the selected tip' }, { name: 'tries', label: 'Tries', type: 'number', placeholder: 'default 65536, or more for a series with a high layer floor; with For: as many as fit' }, series, ...grindFields(), { name: 'start', label: 'First nonce', type: 'number', placeholder: 'default 0' }, { name: 'bytes', label: 'Also print the hex, for Submit on another node', type: 'check' }],
        args: v => ['work', ...where.flags(), ...flag('pool', v.pool), ...flag('serial', v.serial), ...flag('entry', v.entry), ...flag('tries', v.tries), ...flag('series', v.series), ...grindFlags(v), ...flag('start', v.start), ...flag('bytes', v.bytes)], submit: 'Work' }),
      // poon submit (Source D142) hands the node an entry or layer made
      // elsewhere, as the hex Build or Add a layer prints with -bytes,
      // typed in or read by poon from a file (-f, BLUE-015).
      form(ctx, { title: 'Submit an entry or layer', about: 'Gives the node an entry or a layer made elsewhere: the hex of an entry or layer reply.',
        fields: [{ name: 'kind', label: 'Kind', type: 'select', options: ['entry', 'layer'] }, { name: 'pool', label: 'Pool', type: 'number' }, { ...serial, required: false, placeholder: 'or a mint serial' }, series, { name: 'file', label: 'From a file', placeholder: 'a path on this machine, or - to paste it when asked, then Send and end input' },
          { name: 'hex', label: 'Hex', placeholder: 'the entry or layer, hex', only: { field: 'file', values: [''] } }],
        args: v => ['submit', v.kind || 'entry', ...where.flags(), ...flag('series', v.series), ...flag('pool', v.pool), ...flag('serial', v.serial), ...flag('f', v.file), ...(!v.file && v.hex ? [v.hex] : [])], submit: 'Submit', danger: true }),
      ledgerExport, ledgerImport,
    ]);
  },
};
