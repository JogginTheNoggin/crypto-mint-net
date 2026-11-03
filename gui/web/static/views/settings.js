// The Settings view. Each view is one file here, listed in index.js (GUI-GREEN-044).
import * as where from '../where.js';
import { el } from '../views.js';

export const settingsView = {
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
