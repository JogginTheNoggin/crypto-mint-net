// Views. Each one runs poo commands and renders what they print; none of
// them computes anything a command does not print.
import * as api from './api.js';

function el(tag, attrs = {}, ...kids) {
  const e = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) {
    if (k === 'class') e.className = v;
    else if (k.startsWith('on')) e.addEventListener(k.slice(2), v);
    else e.setAttribute(k, v);
  }
  e.append(...kids.filter(k => k !== null && k !== undefined));
  return e;
}

// jobsView lists the jobs the GUI has run, with their output.
const jobsView = {
  id: 'jobs',
  title: 'Jobs',
  mount(root, ctx) {
    const list = el('div', { class: 'jobs' });
    const detail = el('pre', { class: 'term-out job-out' });
    root.append(el('h1', {}, 'Jobs'),
      el('p', { class: 'hint' }, 'Every command the GUI has run, from views, forms and the terminal. Each is the poo command line shown.'),
      list, detail);
    let unfollow = null;
    async function refresh() {
      let js;
      try { js = await api.jobs(); } catch (e) { list.textContent = e.message; return; }
      list.textContent = '';
      for (const j of js.slice().reverse()) {
        const state = j.done ? `exit ${j.code}` : 'running';
        list.append(el('div', { class: 'job' },
          el('code', {}, j.command),
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

export const views = [jobsView];
export { el };
