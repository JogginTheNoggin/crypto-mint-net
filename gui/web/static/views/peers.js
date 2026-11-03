// The Peers view. Each view is one file here, listed in index.js (GUI-GREEN-044).
import * as where from '../where.js';
import { flag, form, mountAll, panel } from '../views.js';

export const peersView = {
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
