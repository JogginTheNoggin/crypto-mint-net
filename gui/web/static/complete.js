// Tab completion for the terminal. The words come from poo's own help
// output, read through the same commands a user could type, so the list
// can never name a command poo does not have.
import * as api from './api.js';

const cache = new Map(); // help argv joined by spaces -> Promise<words>

// helpWords reads "poo help" (or "poo CMD help") once and returns the
// command words it lists.
function helpWords(prefix) {
  const key = prefix.join(' ');
  if (!cache.has(key)) {
    cache.set(key, api.run([...prefix, 'help']).then(r => {
      if (r.code !== 0) return [];
      return prefix.length === 0 ? parseTop(r.out) : parseSub(r.out);
    }).catch(() => { cache.delete(key); return []; }));
  }
  return cache.get(key);
}

// parseTop reads the "  poo NAME ..." lines of poo help. A line such as
// "poo policy list|use|delete" also gives policy's subcommands.
export function parseTop(text) {
  const top = new Set();
  const subs = {};
  for (const m of text.matchAll(/^ {2}poo ([a-z][a-z-]*)(?: ([a-z]+(?:\|[a-z]+)+))?/gm)) {
    top.add(m[1]);
    if (m[2]) subs[m[1]] = m[2].split('|');
  }
  return { top: [...top], subs };
}

// parseSub reads the "  NAME ..." lines of a command's own help.
export function parseSub(text) {
  const words = new Set();
  for (const m of text.matchAll(/^ {2}([a-z][a-z-]*)(?= |$)/gm)) words.add(m[1]);
  return { top: [...words], subs: {} };
}

function commonPrefix(ws) {
  if (!ws.length) return '';
  let p = ws[0];
  for (const w of ws) while (!w.startsWith(p)) p = p.slice(0, -1);
  return p;
}

// complete returns {line, options} for a typed line: line is the line
// with the word under completion extended, options the candidates when
// there is more than one.
export async function complete(line) {
  const body = line.replace(/^\s*poo\s+/, '');
  const lead = line.slice(0, line.length - body.length);
  const words = body.split(/\s+/);
  const word = words.pop();
  if (word.startsWith('-') || words.length > 1) return { line, options: [] };
  let cands;
  const top = await helpWords([]);
  if (words.length === 0) {
    cands = top.top;
  } else if (top.subs[words[0]]) {
    cands = top.subs[words[0]];
  } else if (top.top.includes(words[0])) {
    cands = (await helpWords([words[0]])).top;
  } else {
    cands = [];
  }
  const hits = cands.filter(c => c.startsWith(word)).sort();
  if (hits.length === 0) return { line, options: [] };
  const before = lead + (words.length ? words.join(' ') + ' ' : '');
  if (hits.length === 1) return { line: before + hits[0] + ' ', options: [] };
  return { line: before + commonPrefix(hits), options: hits };
}
