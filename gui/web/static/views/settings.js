// The Settings view. Each view is one file here, listed in index.js (GUI-GREEN-044).
import * as where from '../where.js';
import { el } from '../views.js';

export const settingsView = {
  id: 'settings',
  title: 'Settings',
  mount(root) {
    const cur = where.get();
    const r = el('input', { value: cur.root, placeholder: '$POON_HOME or ~/.poon', spellcheck: 'false' });
    const p = el('input', { value: cur.policy, placeholder: 'the current policy', spellcheck: 'false' });
    const theme = el('select', {}, ...['system', 'light', 'dark'].map(t => el('option', { value: t }, t)));
    theme.value = where.theme();
    // Takes effect at once, without Save: it only shows or hides lines.
    const cmds = el('input', { type: 'checkbox', name: 'showCommands' });
    cmds.checked = where.showCommands();
    cmds.addEventListener('input', () => where.setShowCommands(cmds.checked));
    const f = el('form', { class: 'fields' },
      el('label', {}, el('span', {}, 'Root folder (holds the node policies)'), r),
      el('label', {}, el('span', {}, 'Node policy'), p),
      el('label', {}, el('span', {}, 'Theme'), theme),
      el('label', { class: 'check show-commands' }, el('span', {}, 'Show the command behind each action (to learn it, copy it or run it in the terminal)'), cmds),
      el('div', { class: 'actions' }, el('button', { type: 'submit', class: 'primary' }, 'Save')));
    f.onsubmit = ev => { ev.preventDefault(); where.set({ root: r.value.trim(), policy: p.value.trim() }); where.setTheme(theme.value); };
    root.append(el('h1', {}, 'Settings'),
      el('p', { class: 'hint' }, 'Which node the views talk to. Leave both empty for the usual one. (In a shell these are -root and -policy; the terminal adds nothing: what you type is what runs.)'), f);
  },
};
