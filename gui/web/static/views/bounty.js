// The Bounty view: the bounty market (D196, GUI-GREEN-062). A node pays
// peers to grind layers on its entries; every action is a poo bounty
// command, and the tables only lay out what poo bounty list prints. The
// market is off until the bounty.post or bounty.take limit turns it on
// (Node policies, Set limits).
import * as where from '../where.js';
import * as api from '../api.js';
import { el, flag, form, mountAll, panel, rawFold, needsNode, rowsTable } from '../views.js';
import { render } from '../render.js';
import { walletFile } from './wallet.js';

const plural = (n, one, many) => `${n} ${n === 1 ? one : many}`;
const short = h => (typeof h === 'string' && h.length > 16 ? `${h.slice(0, 10)}…${h.slice(-6)}` : String(h ?? ''));

// marketNote says, from poo bounty list's post and take, what this node
// does in the market.
export function marketNote(o) {
  if (!o || typeof o.post !== 'boolean' || typeof o.take !== 'boolean') return null;
  return el('p', { class: 'hint bounty-on', role: 'status' }, `This node ${o.post ? 'posts offers' : 'does not post offers'} and ${o.take ? 'takes offers from peers' : 'does not take offers from peers'}.`);
}

// offNote is the line under a refusal of bounty list (other than no node
// answering). It never reads poo's error text (GUI White S5): st is what
// poo status printed, whose bounty {post, take} says whether the market
// is on (W-30). Both off gives the steps to turn it on, as settable
// limits (D159); a status without the field gives a fixed line that
// says the same if that is what poo said; a market that is on gives
// nothing, as poo's error says why.
export function offNote(r, st) {
  if (r.code === 0 || needsNode(r)) return null;
  const how = 'In Node policies, Set limits: bounty.post on to pay peers for layers (it also needs bounty.max_amount and bounty.budget), bounty.take on to grind for peers. Saving reloads a running node to apply it.';
  const b = st && st.bounty;
  if (b && typeof b.post === 'boolean' && typeof b.take === 'boolean') {
    if (b.post || b.take) return null;
    return el('p', { class: 'next-step bounty-off', role: 'status' }, `The market is off on this node. ${how}`);
  }
  return el('p', { class: 'next-step bounty-off', role: 'status' }, `If poo says the market is off: ${how}`);
}

// STATES says what each offer state poo prints means (D197): this node's
// offers, then a peer's.
const STATES = { open: 'open', filled: 'filled', closed: 'closed', expired: 'expired', leased: 'leased to a grinder', taken: 'taken' };
const stateText = s => (typeof s === 'string' ? STATES[s] || s : '');

// when shows a time poo printed (RFC 3339, to the nanosecond) in the
// reader's own time zone and words, with poo's text in the tooltip and
// the datetime attribute (GUI-GREEN-063). Text that is not a time is
// shown as printed.
export function when(s) {
  if (typeof s !== 'string' || !s) return '';
  const d = new Date(s);
  if (Number.isNaN(d.getTime())) return s;
  return el('time', { datetime: s, title: s }, d.toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' }));
}
// open says an offer can still take layers: poo says open, or (an older
// node) says nothing. Cancel and Work are offered only then; poo still
// decides (GUI-GREEN-063).
const isOpen = st => st === undefined || st === 'open';

// unpaid counts a posted offer's accepted claims with no payment yet.
const unpaid = m => (Array.isArray(m.claims) ? m.claims.filter(c => !(Array.isArray(c.paid) && c.paid.length)).length : 0);


// marketView draws poo bounty list: this node's offers with their claims,
// and the offers peers sent. act(kind, values) fills a form below.
export function marketView(o, act = () => {}) {
  const out = [];
  const note = marketNote(o);
  if (note) out.push(note);
  const mine = Array.isArray(o && o.mine) ? o.mine : [];
  const held = Array.isArray(o && o.held) ? o.held : [];
  if (mine.length) {
    out.push(el('h3', {}, `Your offers (${mine.length})`));
    // Long lists draw 500 rows at a time with Show more (GUI/Orange R37B).
    out.push(rowsTable(mine, m => {
      const f = m.offer || {};
      const owed = unpaid(m);
      return [render(m.id), String(f.pool ?? ''), render(f.entry), String(f.strength ?? ''), render(f.amount), render(f.count),
        String(Array.isArray(m.claims) ? m.claims.length : 0), String(owed), stateText(m.state), when(m.closes),
        el('span', { class: 'row-acts' },
          owed ? el('button', { type: 'button', class: 'small', onclick: () => act('pay', { id: m.id }) }, 'Pay') : '',
          isOpen(m.state) ? el('button', { type: 'button', class: 'small', onclick: () => act('cancel', { id: m.id }) }, 'Cancel') : '')];
    }, { headers: ['offer', 'pool', 'entry', 'strength', 'per layer', 'layers', 'claims', 'unpaid', 'state', 'closes', ''], cls: 'bounty-mine', more: 'bounty-more', noun: 'offers' }));
    const owed = mine.reduce((a, m) => a + unpaid(m), 0);
    if (owed) out.push(el('p', { class: 'next-step bounty-owed' }, `${plural(owed, 'accepted claim waits', 'accepted claims wait')} to be paid: the node kept the layer, and pays only when you press Pay (no auto-pay in phase 1).`));
  }
  if (held.length) {
    out.push(el('h3', {}, `Offers from peers (${held.length})`));
    out.push(rowsTable(held, h => {
      const f = h.offer || {};
      // Work is offered on an open offer (or one poo gives no state for);
      // poo still decides if a lease is granted.
      const workable = isOpen(h.state);
      return [render(h.id), render(h.from), String(f.pool ?? ''), String(f.strength ?? ''), render(f.amount), render(f.count), when(h.received), stateText(h.state), when(h.closes),
        workable ? el('button', { type: 'button', class: 'small', onclick: () => act('work', { id: h.id }) }, 'Work') : ''];
    }, { headers: ['offer', 'from', 'pool', 'strength', 'per layer', 'layers', 'received', 'state', 'closes', ''], cls: 'bounty-held', more: 'bounty-more', noun: 'offers' }));
  }
  if (!mine.length && !held.length && note) {
    // Only the steps this node takes are offered (GUI-GREEN-064).
    const next = o.post && o.take ? ' Ask peers for offers below, or post one.' : o.take ? ' Ask peers for offers below.' : o.post ? ' Post one below.' : '';
    out.push(el('p', { class: 'hint empty' }, `No offers yet.${next}`));
  }
  return out;
}

// workNote is the line over poo bounty work's reply.
export function workNote(o) {
  if (!o || typeof o.found !== 'boolean') return null;
  if (!o.found) return el('p', { class: 'next-step bounty-work', role: 'status' }, `No layer found in ${o.tried ?? 0} tries before the lease or the time ran out; work the offer again for a new lease.`);
  return el('p', { class: o.outcome === 'accepted' ? 'hint bounty-work' : 'next-step bounty-work', role: 'status' },
    `Found a layer of strength ${o.strength} in ${o.tried} tries; the payer answered: ${o.outcome}.${o.outcome === 'accepted' ? ' The payer pays when it presses Pay; the payment goes to the address you gave.' : ''}`);
}

// payNote is the line over poo bounty pay's reply.
export function payNote(o) {
  if (!o || typeof o.amount !== 'number' || typeof o.pay_to !== 'string') return null;
  return el('p', { class: 'hint bounty-paid', role: 'status' }, `Paid ${o.amount} to ${short(o.pay_to)} for the layer with nonce ${o.nonce}; the grinder ${o.told ? 'was told' : 'was not told (it is not connected now); Payments sent in the Wallet view shows the payment'}.`);
}

const parse = r => { try { return r.code === 0 ? JSON.parse(r.out) : null; } catch (_) { return null; } };

export const bountyView = {
  id: 'bounty',
  title: 'Bounty market',
  mount(root, ctx) {
    const sf = { name: 'series', label: 'Series id', placeholder: 'only if the node follows more than one' };
    const forms = {};
    const act = (kind, vals) => {
      const f = forms[kind];
      if (!f) return;
      for (const [k, v] of Object.entries(vals)) if (f.inputs[k]) f.inputs[k].value = v;
      f.form.dispatchEvent(new Event('input'));
      f.el.scrollIntoView({ block: 'start' });
      (f.inputs.to && !f.inputs.to.value ? f.inputs.to : f.form.querySelector('button[type=submit]')).focus();
    };
    let market = null;
    const draw = async (r, area) => {
      if (r.code !== 0) {
        if (needsNode(r)) return;
        // Whether the market is on comes from poo status (W-30), never
        // from the error's words (GUI White S5).
        let st = null;
        try { const s = await api.run(['status', '-counts', ...where.flags()]); st = s.code === 0 ? JSON.parse(s.out) : null; } catch (_) { /* no status */ }
        const off = offNote(r, st);
        if (off) area.append(off);
        return;
      }
      const o = parse(r);
      if (!o || !('mine' in o || 'held' in o)) return;
      const raw = area.querySelector('.result');
      const box = el('div', { class: 'bounty-market' }, ...marketView(o, act));
      if (raw) { raw.replaceWith(box); area.append(rawFold(raw)); } else area.append(box);
    };
    const again = () => { if (market) market.reload(); };
    market = panel(ctx, { title: 'Offers', about: 'The offers this node posted, with the layers peers claimed on them, and the offers peers sent it. Pay, Cancel and Work fill the forms below.', args: () => ['bounty', 'list', ...where.flags()], onResult: draw });
    forms.ask = form(ctx, { title: 'Ask peers for offers', about: 'Asks every connected peer that runs the market for its open offers, and waits a moment for answers. Offers below the node\'s min_price are not kept.', fields: [sf],
      args: v => ['bounty', 'list', ...where.flags(), '-ask', ...flag('series', v.series)], submit: 'Ask', onResult: draw });
    forms.post = form(ctx, { title: 'Post an offer', about: 'Offers to pay peers for layers on an entry this node holds. The offer goes to every connected peer that runs the market; it must fit the node\'s max_amount, and all open offers and unpaid layers must fit its budget.', fields: [
      { name: 'pool', label: 'Pool', type: 'number', placeholder: 'or a mint serial below' }, { name: 'serial', label: 'Or a mint serial', type: 'number' },
      { name: 'entry', label: 'Entry', required: true, placeholder: 'the entry hash (Build an entry prints it)' },
      { name: 'strength', label: 'Least strength (bits)', type: 'number', required: true, placeholder: 'at least the series\' layer floor' },
      { name: 'amount', label: 'Units per layer', type: 'number', required: true }, { name: 'count', label: 'Layers', type: 'number', placeholder: 'default 1' },
      { name: 'open', label: 'Open for (seconds)', type: 'number', placeholder: 'default 3600' }, sf],
      args: v => ['bounty', 'post', ...where.flags(), ...flag('pool', v.pool), ...flag('serial', v.serial), ...flag('entry', v.entry), ...flag('strength', v.strength), ...flag('amount', v.amount), ...flag('count', v.count), ...flag('open', v.open), ...flag('series', v.series)],
      submit: 'Post', danger: true, onResult: again });
    forms.work = form(ctx, { title: 'Work an offer', about: 'Takes a lease on a peer\'s offer, grinds until it finds a layer, the lease ends or the time is up, and claims the layer to be paid to your address. The address is fixed when the lease is taken.', fields: [
      { name: 'id', label: 'Offer id', required: true }, { name: 'to', label: 'Pay to address', required: true, placeholder: 'a fresh one: Wallet, New address' },
      { name: 'maxSeconds', label: 'Grind at most (seconds)', type: 'number', placeholder: 'default and most: 240' }, sf],
      args: v => ['bounty', 'work', ...(v.id ? [v.id] : []), ...where.flags(), ...flag('to', v.to), ...flag('max-seconds', v.maxSeconds), ...flag('series', v.series)],
      submit: 'Work', onResult: (r, area) => { const n = workNote(parse(r)); if (n) area.prepend(n); again(); } });
    forms.pay = form(ctx, { title: 'Pay a claim', about: 'Pays an accepted claim from the wallet the Wallet view names, and tells the grinder the payment. The command asks for the passphrase when it needs it.', fields: [
      { name: 'id', label: 'Offer id', required: true }, { name: 'claim', label: 'Claim nonce', type: 'number', placeholder: 'default: the offer\'s only unpaid claim' }, sf],
      args: v => ['bounty', 'pay', ...(v.id ? [v.id] : []), ...where.flags(), ...flag('wallet', walletFile()), ...flag('claim', v.claim), ...flag('series', v.series)],
      submit: 'Pay', danger: true, onResult: (r, area) => { const n = payNote(parse(r)); if (n) area.prepend(n); again(); } });
    forms.cancel = form(ctx, { title: 'Cancel an offer', about: 'Closes an offer early. Claims already accepted stay owed.', fields: [{ name: 'id', label: 'Offer id', required: true }, sf],
      args: v => ['bounty', 'cancel', ...(v.id ? [v.id] : []), ...where.flags(), ...flag('series', v.series)], submit: 'Cancel offer', danger: true, onResult: again });
    return mountAll(root, 'Bounty market', 'Pay peers to grind layers on your entries, or grind for theirs (D196). Phase 1: fixed prices, direct peers only, and you press Pay yourself.',
      [market, forms.ask, forms.post, forms.work, forms.pay, forms.cancel]);
  },
};
