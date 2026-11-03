// Prints, for each case in the JSON array argv[3] (each an args list),
// what api.js copyLine gives with the windows grammar, or null.
// copyline_test.go checks the answers (GUI-024).
import { readFileSync } from 'node:fs';

const src = readFileSync(process.argv[2], 'utf8').replace("let grammar = 'posix';", "let grammar = 'windows';");
const mod = await import('data:text/javascript,' + encodeURIComponent(src));
console.log(JSON.stringify(JSON.parse(process.argv[3]).map(args => mod.copyLine(args))));
