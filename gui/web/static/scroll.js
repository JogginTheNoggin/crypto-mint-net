// Scrollback for the panes that show a command's output as it arrives:
// the terminal, a running node's log and a running command (ORANGE-003).
// Like a terminal's scrollback it keeps the newest max characters, so a
// command that prints for hours cannot fill the tab, and it scrolls to
// the end once per frame rather than once per write. Text is shown
// exactly as printed; only the oldest is let go.
export const MAX_CHARS = 1 << 20;
// Output goes into blocks of whole lines, about BLOCK characters each.
// A browser lays out a block of text as a whole, so writing to the last
// small block costs the same however much is above it; one long run of
// text would be laid out again, all of it, on every write.
const BLOCK = 4 << 10;

export function scrollback(pre, { max = MAX_CHARS, follow = false } = {}) {
  let blocks = [], size = 0, frame = 0;
  const end = () => { frame = 0; pre.scrollTop = pre.scrollHeight; };
  return {
    write(text, cls = '') {
      let b = blocks[blocks.length - 1];
      if (!b || (b.n >= BLOCK && b.nl)) {
        b = { el: document.createElement('div'), n: 0, nl: false, span: null, cls: null };
        b.el.className = 'blk';
        pre.append(b.el);
        blocks.push(b);
      }
      if (b.span && b.cls === cls) b.span.firstChild.appendData(text);
      else {
        b.span = document.createElement('span');
        if (cls) b.span.className = cls;
        b.span.textContent = text;
        b.cls = cls;
        b.el.append(b.span);
      }
      b.n += text.length;
      b.nl = text.endsWith('\n');
      size += text.length;
      while (size > max && blocks.length > 1) {
        const old = blocks.shift();
        size -= old.n;
        old.el.remove();
      }
      if (follow && !frame) frame = requestAnimationFrame(end);
    },
    clear() { pre.textContent = ''; blocks = []; size = 0; },
  };
}
