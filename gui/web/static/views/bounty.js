// The Bounty view: the bounty market (D196, GUI-GREEN-062). A node pays
// peers to grind layers on its entries; every action is a poo bounty
// command, and the tables only lay out what poo bounty list prints. The
// market is off until node.json's bounty section turns it on, which the
// GUI does not edit.
import * as where from '../where.js';
import { el, flag, form, mountAll, panel, rawFold, needsNode } from '../views.js';
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

// offNote is a fixed line under any refusal of bounty list (other than
// no node answering): poo's own error stays above it, and the line says
// how the market is turned on if that is what poo said. It never reads
// poo's error text (GUI White S5); the GUI does not edit node.json.
export function offNote(r) {
  if (r.code === 0 || needsNode(r)) return null;
  return el('p', { class: 'next-step bounty-off', role: 'status' }, 'If poo says the market is off: it stays off until the node policy file (node.json, in the policy\'s folder) has a bounty section: "post": true to pay peers for layers, which also needs "max_amount" and "budget"; "take": true to grind for peers. Restart the node after the change. docs/running-a-node.md has an example.');
}

// unpaid counts a posted offer's accepted claims with no payment yet.
const unpaid = m => (Array.isArray(m.claims) ? m.claims.filter(c => !(Array.isArray(c.paid) && c.paid.length)).length : 0);

function table(headers, rows, cls) {
  const t = el('table', { class: `data ${cls}` });
  t.createTHead().insertRow().append(...headers.map(h => el('th', {}, h)));
  const body = t.createTBody();
  for (const cells of rows) {
    const tr = body.insertRow();
    for (const c of cells) tr.insertCell().append(c);
  }
  return el('div', { class: 'table-wrap' }, t);
}

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
    out.push(table(['offer', 'pool', 'entry', 'strength', 'per layer', 'layers', 'claims', 'unpaid', ''], mine.map(m => {
      const f = m.offer || {};
      const owed = unpaid(m);
      return [render(m.id), String(f.pool ?? ''), render(f.entry), String(f.strength ?? ''), render(f.amount), render(f.count),
        String(Array.isArray(m.claims) ? m.claims.length : 0), String(owed),
        el('span', { class: 'row-acts' },
          owed ? el('button', { type: 'button', class: 'small', onclick: () => act('pay', { id: m.id }) }, 'Pay') : '',
          el('button', { type: 'button', class: 'small', onclick: () => act('cancel', { id: m.id }) }, 'Cancel'))];
    }), 'bounty-mine'));
    const owed = mine.reduce((a, m) => a + unpaid(m), 0);
    if (owed) out.push(el('p', { class: 'next-step bounty-owed' }, `${plural(owed, 'accepted claim waits', 'accepted claims wait')} to be paid: the node kept the layer, and pays only when you press Pay (no auto-pay in phase 1).`));
  }
  if (held.length) {
    out.push(el('h3', {}, `Offers from peers (${held.length})`));
    out.push(table(['offer', 'from', 'pool', 'strength', 'per layer', 'layers', 'received', ''], held.map(h => {
      const f = h.offer || {};
      return [render(h.id), render(h.from), String(f.pool ?? ''), String(f.strength ?? ''), render(f.amount), render(f.count), String(h.received ?? ''),
        el('button', { type: 'button', class: 'small', onclick: () => act('work', { id: h.id }) }, 'Work')];
    }), 'bounty-held'));
  }
  if (!mine.length && !held.length && note) {
    out.push(el('p', { class: 'hint empty' }, o.take ? 'No offers yet. Ask peers for offers below, or post one.' : 'No offers yet. Post one below.'));
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
    const draw = (r, area) => {
      const off = offNote(r);
      if (off) { area.append(off); return; }
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
