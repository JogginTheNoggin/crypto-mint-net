// The Jobs view. Each view is one file here, listed in index.js (GUI-GREEN-044).
import * as api from '../api.js';
import { el } from '../views.js';
import { scrollback } from '../scroll.js';

// jobsView lists the jobs the GUI has run, with their output.
export const jobsView = {
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
