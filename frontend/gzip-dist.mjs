// Runs after `vite build` (package.json "build"). The built bundle is embedded in the Go
// binary, which must stay under 20 MB, so its text files are stored gzipped (name.gz in
// place of name); Go's core/gzfs serves them under their own names, inflated once. Fonts
// and pictures are compressed already and stay as they are.
import { readFileSync, readdirSync, rmSync, statSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { gzipSync } from 'node:zlib';

const TEXT = /\.(js|css|svg|json)$/;
const walk = (dir) => {
  for (const name of readdirSync(dir)) {
    const file = join(dir, name);
    if (statSync(file).isDirectory()) walk(file);
    else if (TEXT.test(name)) {
      writeFileSync(file + '.gz', gzipSync(readFileSync(file), { level: 9 }));
      rmSync(file);
    }
  }
};
walk(new URL('dist/', import.meta.url).pathname);
