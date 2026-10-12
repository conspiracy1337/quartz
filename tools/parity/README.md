# Design parity check (dev only)

Renders every `design/project/*.dc.html` board and the matching state of the Vite mock
preview, then compares them.

- `support.js` stands in for the design tool's runtime (helmet, `{{expr}}`, `sc-if`,
  `sc-for`, `dc-import`, DCLogic state). It is injected through Playwright, so `design/`
  is never touched.
- `design-shots.mjs <out> [WxH] [board…]` screenshots the boards and dumps their text.
- `app-shots.mjs <out> [WxH] [board…]` does the same for the app; `MAP` lists each
  board's route and the overlay, query or dialog it shows. Restart `pnpm dev` first,
  because hot reloads give the page module instances the script can't reach.
- `pair.py <design> <app> <out>` stacks each pair and ranks them by pixel difference.
- `textdiff.py <design> <app> [board…]` lists text that is only on one side, plus text
  that moved or changed style.
- `SKEL=1` compares skeleton states (`?loading=true` against `?slow`).

The scripts need `playwright-core` and the cached Chromium in `~/.cache/ms-playwright`.
