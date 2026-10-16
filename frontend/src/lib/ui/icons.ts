// Every icon used in the designs, as the exact SVG markup they use. Filled icons are
// drawn with fill, the rest with a stroke whose width the caller sets per use, as the
// designs do. These strings are constants, never user input.

export interface IconDef { body: string; fill?: true }

const s = (body: string): IconDef => ({ body });
const f = (body: string): IconDef => ({ body, fill: true });

export const icons = {
  search: s('<circle cx="11" cy="11" r="7"/><path d="m20 20-3.5-3.5"/>'),
  'chevron-down': s('<path d="m6 9 6 6 6-6"/>'),
  'chevron-up': s('<path d="m6 15 6-6 6 6"/>'),
  'chevron-right': s('<path d="m9 6 6 6-6 6"/>'),
  'chevron-left': s('<path d="m15 6-6 6 6 6"/>'),
  'up-down': s('<path d="m7 15 5 5 5-5M7 9l5-5 5 5"/>'),
  x: s('<path d="M6 6l12 12M18 6 6 18"/>'),
  plus: s('<path d="M12 5v14M5 12h14"/>'),
  check: s('<path d="m5 12 5 5 9-10"/>'),
  'arrow-left': s('<path d="M19 12H5M11 6l-6 6 6 6"/>'),
  'arrow-right': s('<path d="M5 12h14M13 6l6 6-6 6"/>'),
  'arrow-up': s('<path d="M12 19V5M6 11l6-6 6 6"/>'),
  more: f('<circle cx="5" cy="12" r="1.6"/><circle cx="12" cy="12" r="1.6"/><circle cx="19" cy="12" r="1.6"/>'),
  play: f('<path d="M7 4v16l13-8z"/>'),
  'play-wide': f('<path d="M6 3.5v17l14-8.5z"/>'),
  stop: f('<rect x="5" y="5" width="14" height="14" rx="2"/>'),
  pause: f('<rect x="6" y="5" width="4" height="14" rx="1"/><rect x="14" y="5" width="4" height="14" rx="1"/>'),
  microsoft: f('<rect x="3" y="3" width="8.5" height="8.5"/><rect x="12.5" y="3" width="8.5" height="8.5"/><rect x="3" y="12.5" width="8.5" height="8.5"/><rect x="12.5" y="12.5" width="8.5" height="8.5"/>'),
  cube: s('<path d="M12 2.5 21 7.5v9L12 21.5 3 16.5v-9z"/><path d="M12 12 21 7.5M12 12v9.5M12 12 3 7.5"/>'),
  home: s('<path d="M4 10.5 12 4l8 6.5V20h-5v-6H9v6H4z"/>'),
  house: s('<path d="M4 10.5 12 4l8 6.5V20H4z"/>'),
  library: s('<path d="m12 3 9 5-9 5-9-5z"/><path d="m3 13 9 5 9-5"/>'),
  compass: s('<circle cx="12" cy="12" r="9"/><path d="m15.5 8.5-2 5-5 2 2-5z"/>'),
  shirt: s('<path d="M8 3h8l4 4-3 3v11H7V10L4 7z"/>'),
  sync: s('<path d="M4 12a8 8 0 0 1 13.7-5.7L20 8.5M20 4v4.5h-4.5M20 12a8 8 0 0 1-13.7 5.7L4 15.5M4 20v-4.5h4.5"/>'),
  download: s('<path d="M12 4v11M7 10l5 5 5-5M5 20h14"/>'),
  upload: s('<path d="M12 15V4M7 9l5-5 5 5M5 20h14"/>'),
  settings: s('<circle cx="12" cy="12" r="3"/><path d="M12 2.5v3M12 18.5v3M2.5 12h3M18.5 12h3M5.3 5.3l2.1 2.1M16.6 16.6l2.1 2.1M5.3 18.7l2.1-2.1M16.6 7.4l2.1-2.1"/>'),
  'settings-simple': s('<circle cx="12" cy="12" r="3"/><path d="M12 2.5v3M12 18.5v3M2.5 12h3M18.5 12h3"/>'),
  folder: s('<path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/>'),
  server: s('<rect x="4" y="4" width="16" height="7" rx="1.5"/><rect x="4" y="13" width="16" height="7" rx="1.5"/><path d="M8 7.5h.01M8 16.5h.01"/>'),
  bolt: s('<path d="M13 2 4 14h7l-1 8 9-12h-7z"/>'),
  shield: s('<path d="M12 3 4 6v6c0 5 3.5 8 8 9 4.5-1 8-4 8-9V6z"/>'),
  'shield-check': s('<path d="M12 3 4 6v6c0 5 3.5 8 8 9 4.5-1 8-4 8-9V6z"/><path d="m9 12 2 2 4-4"/>'),
  lock: s('<rect x="5" y="10" width="14" height="10" rx="2"/><path d="M8 10V7a4 4 0 0 1 8 0v3"/>'),
  warn: s('<path d="M12 3 2 20h20zM12 10v4M12 17h.01"/>'),
  info: s('<circle cx="12" cy="12" r="9"/><path d="M12 11v5M12 8h.01"/>'),
  issue: s('<circle cx="12" cy="12" r="9"/><path d="M12 8v4M12 16h.01"/>'),
  refresh: s('<path d="M20 11a8 8 0 1 0-2.3 5.7M20 4v7h-7"/>'),
  external: s('<path d="M14 4h6v6M20 4l-9 9M18 14v5H5V6h5"/>'),
  list: s('<path d="M4 6h16M4 12h16M4 18h16"/>'),
  file: s('<path d="M6 3h9l4 4v14H6z"/>'),
  link: s('<path d="M10 14a4 4 0 0 0 5.7 0l3-3a4 4 0 0 0-5.7-5.7l-1 1M14 10a4 4 0 0 0-5.7 0l-3 3a4 4 0 0 0 5.7 5.7l1-1"/>'),
  trash: s('<path d="M4 7h16M9 7V4h6v3M6 7l1 13h10l1-13"/>'),
  camera: s('<path d="M4 8h3l2-3h6l2 3h3v11H4zM12 17a3.5 3.5 0 1 0 0-7 3.5 3.5 0 0 0 0 7"/>'),
  'rotate-left': s('<path d="M4 12a8 8 0 1 0 2.3-5.7M4 4v5h5"/>'),
  'rotate-right': s('<path d="M20 12a8 8 0 1 1-2.3-5.7M20 4v5h-5"/>'),
  spinner: s('<path d="M12 3a9 9 0 1 0 9 9"/>'),
  globe: s('<path d="M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18M3 12h18M12 3c3 3 3 15 0 18M12 3c-3 3-3 15 0 18"/>'),
  chat: s('<path d="M4 5h16v11H9l-5 4z"/>'),
  gpu: s('<path d="M4 7h16v10H4zM8 17v3M16 17v3M8 11h8"/>'),
  github: s('<path d="M9 18c-4 1.5-4-2-6-2.5M15 21v-3.5a3 3 0 0 0-1-2.5c3-.3 6-1.5 6-6.5a5 5 0 0 0-1.4-3.5 4.6 4.6 0 0 0-.1-3.5s-1.1-.3-3.6 1.4a12.4 12.4 0 0 0-6.5 0C5.9 1.7 4.8 2 4.8 2a4.6 4.6 0 0 0-.1 3.5A5 5 0 0 0 3.3 9c0 5 3 6.2 6 6.5a3 3 0 0 0-1 2.5V21"/>'),
  java: s('<path d="M8 3c0 2 2 2 2 4s-2 2-2 4M12 3c0 2 2 2 2 4s-2 2-2 4M5 14h13v3a4 4 0 0 1-4 4H9a4 4 0 0 1-4-4zM18 15h1a2 2 0 0 1 0 4h-1"/>'),
  fabric: s('<path d="M5 19 19 5M8 5h11v11"/>'),
  phone: s('<rect x="6" y="2.5" width="12" height="19" rx="2.5"/><path d="M11 18h2"/>'),
  'wifi-off': s('<path d="M2 8.5a15 15 0 0 1 20 0M5 12a10 10 0 0 1 14 0M8.5 15.5a5 5 0 0 1 7 0M12 19h.01M3 3l18 18"/>'),
} satisfies Record<string, IconDef>;

export type IconName = keyof typeof icons;
