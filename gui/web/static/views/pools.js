// The Pools and payments view. Each view is one file here, listed in index.js (GUI-GREEN-044).
import * as where from '../where.js';
import { flag, form, mountAll, panel } from '../views.js';

// grindFields are the flags build and work share with poo grind: which
// device grinds, with how many threads, and for how long instead of a
// number of tries (GUI-GREEN-041). Device suggests what poo grind devices
// listed, once the Work view has run it.
const grindFields = () => [
  { name: 'device', label: 'Device', placeholder: 'auto, cpu, gpu:0 (default: node.json, else auto)', list: 'grind-devices' },
  { name: 'threads', label: 'Threads', type: 'number', placeholder: 'default: every core but one' },
  { name: 'for', label: 'For', placeholder: 'grind this long, up to 4m (30s), instead of tries' },
];
const grindFlags = v => [...flag('device', v.device), ...flag('threads', v.threads), ...flag('for', v.for)];

export const poolsView = {
  id: 'pools',
  title: 'Pools and payments',
  mount(root, ctx) {
    const serial = { name: 'serial', label: 'Mint serial', type: 'number', required: true };
    const series = { name: 'series', label: 'Series id', placeholder: 'only if the node follows more than one' };
    // -parent (D161): build on a chosen entry instead of the selected tip.
    const parent = { name: 'parent', label: 'Parent', placeholder: 'an entry hash, root, or before-claim with a mint serial (default: the selected tip)' };
    return mountAll(root, 'Pools and payments', 'Look up mints and transactions, and build entries and layers.', [
      panel(ctx, { title: 'Followed pools', about: 'Each followed pool: tip, height, weight, waiting and stored records (poo pools).', args: () => ['pools', ...where.flags()] }),
      // poo unspent and poo tx replace the older holdings and payment
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
      // into poo's masked prompt (-seed-file -), never put on the line.
      form(ctx, { title: 'Build a rival claim', about: 'Builds a claim of a mint into an entry on the parent you choose, such as the one before the entry holding the claim now, and prints its hash and the command to grind it. poo asks for the claiming key\'s seed in a masked field.',
        fields: [{ ...serial }, { ...parent, required: true, placeholder: 'before-claim, root, or an entry hash' }, series, { name: 'work', label: 'Layer tries', type: 'number', placeholder: 'optional: then add the strongest layer found' }, { name: 'maxTries', label: 'Most entry tries', type: 'number', placeholder: 'when the series sets a minimum entry strength' }],
        args: v => ['claim', ...where.flags(), '-seed-file', '-', ...flag('serial', v.serial), ...flag('parent', v.parent), ...flag('series', v.series), ...flag('work', v.work), ...flag('max-tries', v.maxTries)], submit: 'Build claim' }),
      form(ctx, { title: 'Add a layer', about: 'Adds the strongest layer found, by default on the selected tip.',
        fields: [{ name: 'pool', label: 'Pool', type: 'number' }, { ...serial, required: false, placeholder: 'or a mint serial' }, { name: 'entry', label: 'Entry hash', placeholder: 'default: the selected tip' }, { name: 'tries', label: 'Tries', type: 'number' }, series, ...grindFields(), { name: 'start', label: 'First nonce', type: 'number', placeholder: 'default 0' }, { name: 'bytes', label: 'Also print the hex, for Submit on another node', type: 'check' }],
        args: v => ['work', ...where.flags(), ...flag('pool', v.pool), ...flag('serial', v.serial), ...flag('entry', v.entry), ...flag('tries', v.tries), ...flag('series', v.series), ...grindFlags(v), ...flag('start', v.start), ...flag('bytes', v.bytes)], submit: 'Work' }),
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
