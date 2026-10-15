// Screenshots the Vite mock preview at the state matching each design board.
// usage: node app-shots.mjs <outDir> [WxH] [board ...]   (needs `pnpm dev` on :5173)
import { chromium } from 'playwright-core';
import fs from 'node:fs';
import path from 'node:path';
const DUMP = fs.readFileSync(new URL('./textdump.js', import.meta.url), 'utf8');

const here = path.dirname(new URL(import.meta.url).pathname);
const CHROME = `${process.env.HOME}/.cache/ms-playwright/chromium-1243/chrome-linux64/chrome`;
const BASE = process.env.APP || (process.env.SKEL ? 'http://localhost:5173/?slow' : 'http://localhost:5173/');
const [out = path.join(here, 'shots/app'), size = '1200x760', ...only] = process.argv.slice(2);
const [W, H] = size.split('x').map(Number);
fs.mkdirSync(out, { recursive: true });

// session(fn) runs fn(session) inside the page, on the app's own module instance.
const S = (body) => async (page) => page.evaluate(`import('/src/lib/session.svelte.ts').then(({ session }) => { ${body} })`);
const click = (sel) => async (page) => page.click(sel);
const seq = (...fs) => async (page) => { for (const f of fs) { await f(page); await page.waitForTimeout(300); } };
const type = (sel, text) => async (page) => { if (sel) await page.fill(sel, text); else await page.keyboard.type(text); };

export const MAP = {
  Main: ['/home'], LibA: ['/library'], NewB: ['/library/new'], NewC: ['/library/import'],
  BrA: ['/browse'], ProjA: ['/browse/iris-shaders'],
  AccB: ['/skins'], AccC: ['/accounts'], AccD: ['/accounts', S("session.open({ kind: 'add-account' })")],
  DlB: ['/downloads'], DlC: ['/instance/fo/mods'],
  DlgBlocked: ['/downloads', S("session.open({ kind: 'blocked', jobId: 'a00000000001' })")],
  DlgCrash: ['/instance/fo/overview', S("session.open({ kind: 'crash', instanceId: 'fo', runId: 'r3' })")],
  DlgDelete: ['/library', S("session.notify({ tone: 'trash', subject: 'Snapshot 25w14a', text: 'to the trash', undo: () => {} })")],
  DlgHost: ['/instance/fo/overview', S("session.open({ kind: 'host', instanceId: 'fo' })")],
  DlgLaunch: ['/instance/fo/overview', S("session.play('fo')")],
  ISetA: ['/instance/fo/settings'], ISetB: ['/instance/fo/settings/commands'], ISetC: ['/instance/fo/settings/sandbox'],
  LogsA: ['/instance/fo/logs', type('input[type=search]', 'mixin')], LogsB: ['/instance/fo/logs/files'], LogsC: ['/instance/fo/logs/history'],
  ModsA: ['/instance/fo/mods'], PacksA: ['/instance/fo/resourcepacks'], PacksB: ['/instance/fo/shaders'],
  PacksC: ['/instance/fo/datapacks'], WorldsA: ['/instance/fo/worlds'], ServersA: ['/instance/fo/servers'],
  WorldsMenu: ['/instance/fo/worlds', click('button[aria-label="More for Northwatch"]')],
  DlgCopyWorld: ['/instance/fo/worlds', seq(click('button[aria-label="More for Northwatch"]'), click('text=Copy to instance…'))],
  ServersMenu: ['/instance/fo/servers', click('button[aria-label="More for Friends SMP"]')],
  DlgServer: ['/instance/fo/servers', click('text=Add server')],
  ShotsA: ['/instance/fo/screenshots'], ShotsB: ['/instance/fo/screenshots', click('.shots a, .shot, [href*="screenshots/"]')],
  OvA: ['/instance/fo/overview'],
  PalA: ['/home', seq(S("session.open({ kind: 'palette', mode: 'commands' })"), type(null, 'upd'))],
  PalB: ['/library', seq(S("session.open({ kind: 'palette', mode: 'instances' })"), type(null, '1.21'))],
  PalC: ['/home', S("session.open({ kind: 'shortcuts' })")],
  RevA: ['/instance/fo/mods/updates', click('text=Changelog >> nth=0')], RevB: ['/instance/cab/overview/update'],
  RevC: ['/browse', S("session.open({ kind: 'install', ref: 'distant-horizons', source: 'modrinth', instanceId: 'fo', content: 'mods' })")],
  SetA: ['/settings/general'], SetB: ['/settings/appearance'], SetC: ['/settings/java'], SetD: ['/settings/search/memory'],
  SetupA: ['/setup/1'], SetupB: ['/setup/2'], SetupC: ['/setup/3'],
  SyncB: ['/syncing'], VerA: ['/instance/fo/settings'], VerB: ['/instance/fo/settings/minecraft'], VerC: ['/instance/fo/settings/loader'],
};

const boards = Object.entries(MAP).filter(([b]) => !only.length || only.includes(b));
const browser = await chromium.launch({ executablePath: CHROME });
const ctx = await browser.newContext({ deviceScaleFactor: 1, viewport: { width: W, height: H } });
for (const [b, [route, act]] of boards) {
  const page = await ctx.newPage();
  const errs = [];
  page.on('pageerror', (e) => errs.push(e.message));
  await page.goto(BASE + '#' + route);
  if (process.env.SKEL) await page.waitForTimeout(500);
  else { await page.waitForLoadState('networkidle'); await page.waitForTimeout(400); }
  if (act) { try { await act(page); } catch (e) { errs.push('act: ' + e.message.split('\n')[0]); } await page.waitForTimeout(600); }
  // Preview-only state that the design boards don't show: the update banner and the mock download queue.
  const keepDl = ['DlB', 'DlC', 'DlgBlocked'].includes(b);
  await page.evaluate(`Promise.all([import('/src/lib/session.svelte.ts'), import('/src/lib/tasks.svelte.ts')]).then(([{ session }, { tasks }]) => {
    session.updateLater = ${b !== 'DlgDelete'};
    if (!${keepDl}) tasks.active = [];
  })`);
  await page.waitForTimeout(200);
  await page.evaluate(() => document.fonts.ready);
  await page.screenshot({ path: path.join(out, b + '.png') });
  fs.writeFileSync(path.join(out, b + '.txt'), (await page.evaluate(DUMP)).join('\n'));
  if (errs.length) console.log(b, 'ERR', errs.slice(0, 3).join(' | '));
  await page.close();
}
await browser.close();
console.log('done', boards.length);
