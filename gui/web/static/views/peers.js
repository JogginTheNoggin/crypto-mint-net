// The Peers view. Each view is one file here, listed in index.js (GUI-GREEN-044).
import * as where from '../where.js';
import { el, flag, form, mountAll, panel } from '../views.js';
import { importForm, planTable, saveAs } from './policies.js';

// The peer book's states, as poo peers -category takes them.
const CATEGORIES = 'candidate, priority, backup, archived, unlisted';

export const peersView = {
  id: 'peers',
  title: 'Peers',
  mount(root, ctx) {
    return mountAll(root, 'Peers', 'Who the node is connected to, how they behave, and what it is checking.', [
      panel(ctx, { title: 'Peers', about: 'Connected peers, their categories and points, and the peer book.', args: () => ['peers', ...where.flags()], refresh: 10000 }),
      // The book filtered as poo peers filters it (D157).
      form(ctx, { title: 'Find peers', about: 'The peer book, only the addresses that match every field filled in.', fields: [
        { name: 'category', label: 'Categories', placeholder: CATEGORIES },
        { name: 'policy', label: 'Mint policy', placeholder: 'a series id, or a part of one' },
        { name: 'label', label: 'Label holds' },
        { name: 'addr', label: 'Address holds' }],
      args: v => ['peers', ...where.flags(), ...flag('category', v.category), ...flag('mint-policy', v.policy), ...flag('label', v.label), ...flag('addr', v.addr)], submit: 'Find' }),
      // Adding a peer by hand, taking one out, labelling it and connecting
      // to it now (D157). Which action takes which flag is poo peer's own
      // rule; Connect with no address and Imported tries every address an
      // imported peer file gave.
      form(ctx, { title: 'Add, remove, label or connect a peer', fields: [
        { name: 'action', label: 'Action', type: 'select', options: [{ value: 'add', label: 'add' }, { value: 'remove', label: 'remove' }, { value: 'label', label: 'label' }, { value: 'connect', label: 'connect now' }] },
        { name: 'addr', label: 'Peer address', placeholder: 'host:port' },
        { name: 'label', label: 'Label', placeholder: 'your own note on the address (label with none clears it)', only: { field: 'action', values: ['add', 'label'] } },
        { name: 'keep', label: 'Also list it in node.json (p2p.peers), so it is always dialled, even with the node stopped', type: 'check', only: { field: 'action', values: ['add'] } },
        { name: 'imported', label: 'With no address: every unlisted address an imported peer file gave', type: 'check', only: { field: 'action', values: ['connect'] } }],
      args: v => ['peer', v.action, ...where.flags(), ...flag('label', v.label), ...flag('keep', v.keep), ...flag('imported', v.imported), ...(v.addr ? [v.addr] : [])],
      submit: 'Apply', danger: true }),
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
      form(ctx, { title: 'Tracking records', about: 'One peer\'s record, or only the blocked or timed-out addresses.', fields: [
        { name: 'addr', label: 'Peer address', placeholder: 'host:port (default: all)' },
        { name: 'blocked', label: 'Only blocked', type: 'check' },
        { name: 'timedOut', label: 'Only timed out', type: 'check' }],
      args: v => ['tracking', ...where.flags(), ...flag('blocked', v.blocked), ...flag('timed-out', v.timedOut), ...(v.addr ? [v.addr] : [])], submit: 'Look up' }),
      panel(ctx, { title: 'Chases', about: 'Heavier stated weights waiting or being checked.', args: () => ['chases', ...where.flags()] }),
      // The peer book shared as one file (D157): others import it as
      // unlisted addresses, after a preview, pinned to the file previewed.
      form(ctx, { title: 'Export peers', about: 'Writes the peer book, or some categories of it, and the blocks, to one file others can import. Without a file, it is printed here, with Save as file.', fields: [
        { name: 'category', label: 'Categories', placeholder: `${CATEGORIES}, blocked, timed-out (default: all)` },
        { name: 'out', label: 'To a file', placeholder: 'a path on this machine (default: print it)' }],
      args: v => ['peers', 'export', ...where.flags(), ...flag('category', v.category), ...flag('out', v.out)], submit: 'Export',
      onResult: (r, area) => { if (r.code === 0 && r.out.trim().startsWith('{')) area.append(saveAs(r.out, 'peers.json')); } }),
      importForm(ctx, PEERS_IMPORT),
    ]);
  },
};

const PEERS_IMPORT = {
  title: 'Import peers', about: 'Shows what a peer file would add here: new addresses (unlisted), more mint policies, labels and, if asked, blocks. It never removes, unblocks or shortens anything. Nothing changes until you press Apply.',
  fields: [
    { name: 'blocks', label: 'Take the file\'s address blocks too, adding or lengthening only (default: list them and leave them)', type: 'check' },
    { name: 'ranges', label: 'With blocks, take its network range blocks too', type: 'check' }],
  args: v => ['peers', 'import', ...where.flags(), ...flag('blocks', v.blocks), ...flag('ranges', v.ranges), '-json', ...(v.file ? [v.file] : [])],
  digest: o => o.digest,
  show: showPeerPlan,
};

// showPeerPlan draws poo peers import -json: one table of what the import
// adds, and a line for what it leaves alone or refuses.
function showPeerPlan(r, area, applied) {
  if (r.code !== 0) return;
  let o;
  try { o = JSON.parse(r.out); } catch (_) { return; }
  const raw = area.querySelector('.result');
  if (!raw || !o || typeof o !== 'object') return;
  const p = o.plan || {};
  const lines = [];
  for (const [kind, xs] of [['added', p.added], ['more', p.more], ['label', p.labels], ['label kept', p.label_conflicts], ['block', p.blocks], ['skipped', p.skipped], ['refused', p.refused]]) {
    for (const c of xs || []) lines.push([kind, c]);
  }
  const text = n => typeof n === 'string' ? n : [n.addr, n.detail].filter(Boolean).join(': ');
  const notes = [
    ...(p.notes || o.notes || []).map(text),
    `${p.same || 0} already the same.`,
    o.applied ? 'Applied.' : applied ? 'Nothing needed changing.' : 'Nothing changed yet: press Apply to make this change.',
  ].join(' ');
  const det = el('details', { class: 'raw' }, el('summary', {}, 'Output as printed'));
  raw.replaceWith(el('div', { class: 'import-plan peers-plan' },
    el('h3', {}, `Peer book${o.running ? ' (node running)' : ''}`),
    lines.length ? planTable(lines, ['address', 'detail'], c => [c.addr, c.detail || '']) : el('p', {}, 'No address changes.'),
    el('p', { class: 'hint' }, notes)));
  det.append(raw);
  area.append(det);
}
