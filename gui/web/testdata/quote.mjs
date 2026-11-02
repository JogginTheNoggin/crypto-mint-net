// Prints, for each argument in the JSON array argv[3], how api.js quotes
// it for each grammar: [[arg, posix, windows], ...]. quote_test.go checks
// the answers against gui/shell.
import { readFileSync } from 'node:fs';

const src = readFileSync(process.argv[2], 'utf8');
const mod = await import('data:text/javascript,' + encodeURIComponent(src + '\nexport { quotePOSIX, quoteWindows };'));
const args = JSON.parse(process.argv[3]);
console.log(JSON.stringify(args.map(a => [a, mod.quotePOSIX(a), mod.quoteWindows(a)])));
