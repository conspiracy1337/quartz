// Renders design/project/*.dc.html with our runtime stand-in and screenshots them.
// usage: node design-shots.mjs <outDir> [WxH] [board ...]
import { chromium } from 'playwright-core';
import fs from 'node:fs';
import path from 'node:path';
const DUMP = fs.readFileSync(new URL('./textdump.js', import.meta.url), 'utf8');

const here = path.dirname(new URL(import.meta.url).pathname); // tools/parity
const DESIGN = '/storage/CodingWorkspaces/mc-launcher/design/project';
const FONTS = '/storage/CodingWorkspaces/mc-launcher/src/frontend/src/assets/fonts';
const CHROME = `${process.env.HOME}/.cache/ms-playwright/chromium-1243/chrome-linux64/chrome`;
const [out = path.join(here, 'shots/design'), size = '1200x760', ...only] = process.argv.slice(2);
const [W, H] = size.split('x').map(Number);
fs.mkdirSync(out, { recursive: true });

const canvas = JSON.parse(fs.readFileSync(path.join(DESIGN, 'canvas.json'), 'utf8'));
const boards = Object.entries(canvas.boards).filter(([f]) => !only.length || only.includes(f.replace('.dc.html', '')));

const fontCSS = fs.readdirSync(FONTS).map((f) => {
  const m = /geist-(sans|mono)-latin-(\d+)-normal/.exec(f);
  return `@font-face{font-family:'${m[1] === 'mono' ? 'Geist Mono' : 'Geist'}';font-weight:${m[2]};src:url(http://design.local/__fonts/${f}) format('woff2')}`;
}).join('\n');

const browser = await chromium.launch({ executablePath: CHROME });
const ctx = await browser.newContext({ deviceScaleFactor: 1 });
await ctx.route('**/*', async (route) => {
  const u = new URL(route.request().url());
  if (u.hostname === 'fonts.googleapis.com') return route.fulfill({ contentType: 'text/css', body: fontCSS });
  if (u.hostname !== 'design.local') return route.abort();
  if (u.pathname.startsWith('/__fonts/')) return route.fulfill({ path: path.join(FONTS, path.basename(u.pathname)) });
  const name = path.basename(u.pathname);
  if (name === 'support.js') return route.fulfill({ path: path.join(here, 'support.js'), contentType: 'text/javascript' });
  const file = path.join(DESIGN, name);
  if (!fs.existsSync(file)) return route.fulfill({ status: 404, body: '' });
  return route.fulfill({ path: file, contentType: name.endsWith('.css') ? 'text/css' : 'text/html' });
});

for (const [file, b] of boards) {
  const page = await ctx.newPage();
  const errs = [];
  page.on('pageerror', (e) => errs.push(e.message));
  page.on('console', (m) => m.type() === 'error' && errs.push(m.text()));
  const fill = b.expand === 'fill';
  await page.setViewportSize(fill ? { width: W, height: H } : { width: b.w, height: b.h });
  await page.goto(`http://design.local/${file}${process.env.SKEL ? '?loading=true' : ''}`);
  await page.waitForFunction(() => window.__dcReady, null, { timeout: 10000 }).catch(() => errs.push('not ready'));
  await page.evaluate(() => document.fonts.ready);
  await page.waitForTimeout(300);
  await page.screenshot({ path: path.join(out, file.replace('.dc.html', '.png')) });
  fs.writeFileSync(path.join(out, file.replace('.dc.html', '.txt')), (await page.evaluate(DUMP)).join('\n'));
  if (errs.length) console.log(file, 'ERR', errs.slice(0, 3).join(' | '));
  await page.close();
}
await browser.close();
console.log('done', boards.length);
