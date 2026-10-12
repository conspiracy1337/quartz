import type { LoaderOption, McVersion, VersionFamily } from '../types';
import { date } from './time';

const MON: Record<string, number> = { Jan: 1, Feb: 2, Mar: 3, Apr: 4, May: 5, Jun: 6, Jul: 7, Aug: 8, Sep: 9, Oct: 10, Nov: 11, Dec: 12 };
/** Parses "Sep 16, 2026". */
const d = (s: string) => {
  const [m, day, y] = s.replace(',', '').split(' ');
  return date(Number(y), MON[m!]!, Number(day));
};
const V = (id: string, java: number, released: string, snapshot = false): McVersion => ({ id, java, released: d(released), ...(snapshot ? { snapshot } : {}) });
const F = (id: string, note: string, releases: McVersion[], snapshots: McVersion[] = []): VersionFamily => ({ id, note, releases, snapshots });

export const families: VersionFamily[] = [
  F('26.x', 'Newest', [V('26.2', 21, 'Sep 16, 2026'), V('26.1.2', 21, 'Jun 9, 2026'), V('26.1', 21, 'Mar 24, 2026')], [V('26w38a', 21, 'Sep 23, 2026', true)]),
  F('1.21', '2024 to 2025', [V('1.21.5', 21, 'Mar 25, 2025'), V('1.21.4', 21, 'Dec 3, 2024'), V('1.21.3', 21, 'Oct 23, 2024'), V('1.21.2', 21, 'Oct 22, 2024'), V('1.21.1', 21, 'Aug 8, 2024'), V('1.21', 21, 'Jun 13, 2024')], [V('25w14a', 21, 'Apr 2, 2025', true)]),
  F('1.20', '2023 to 2024', [V('1.20.6', 21, 'Apr 29, 2024'), V('1.20.5', 21, 'Apr 23, 2024'), V('1.20.4', 17, 'Dec 7, 2023'), V('1.20.3', 17, 'Dec 5, 2023'), V('1.20.2', 17, 'Sep 21, 2023'), V('1.20.1', 17, 'Jun 12, 2023'), V('1.20', 17, 'Jun 7, 2023')]),
  F('1.19', '2022 to 2023', [V('1.19.4', 17, 'Mar 14, 2023'), V('1.19.3', 17, 'Dec 7, 2022'), V('1.19.2', 17, 'Aug 5, 2022'), V('1.19.1', 17, 'Jul 27, 2022'), V('1.19', 17, 'Jun 7, 2022')]),
  F('1.18', '2021 to 2022', [V('1.18.2', 17, 'Feb 28, 2022'), V('1.18.1', 17, 'Dec 10, 2021'), V('1.18', 17, 'Nov 30, 2021')]),
  F('1.17', '2021', [V('1.17.1', 16, 'Jul 6, 2021'), V('1.17', 16, 'Jun 8, 2021')]),
  F('1.16', '2020 to 2021', [V('1.16.5', 8, 'Jan 15, 2021'), V('1.16.4', 8, 'Nov 2, 2020'), V('1.16.3', 8, 'Sep 10, 2020'), V('1.16.2', 8, 'Aug 11, 2020'), V('1.16.1', 8, 'Jun 24, 2020'), V('1.16', 8, 'Jun 23, 2020')]),
  F('1.15', '2019 to 2020', [V('1.15.2', 8, 'Jan 21, 2020'), V('1.15.1', 8, 'Dec 17, 2019'), V('1.15', 8, 'Dec 10, 2019')]),
  F('1.14', '2019', [V('1.14.4', 8, 'Jul 19, 2019'), V('1.14.3', 8, 'Jun 24, 2019'), V('1.14.2', 8, 'May 27, 2019'), V('1.14.1', 8, 'May 13, 2019'), V('1.14', 8, 'Apr 23, 2019')]),
  F('1.13', '2018', [V('1.13.2', 8, 'Oct 22, 2018'), V('1.13.1', 8, 'Aug 22, 2018'), V('1.13', 8, 'Jul 18, 2018')]),
  F('1.12', '2017', [V('1.12.2', 8, 'Sep 18, 2017'), V('1.12.1', 8, 'Aug 3, 2017'), V('1.12', 8, 'Jun 7, 2017')]),
  F('1.11', '2016', [V('1.11.2', 8, 'Dec 21, 2016'), V('1.11.1', 8, 'Dec 20, 2016'), V('1.11', 8, 'Nov 14, 2016')]),
  F('1.10', '2016', [V('1.10.2', 8, 'Jun 23, 2016'), V('1.10.1', 8, 'Jun 22, 2016'), V('1.10', 8, 'Jun 8, 2016')]),
  F('1.9', '2016', [V('1.9.4', 8, 'May 10, 2016'), V('1.9.2', 8, 'Mar 30, 2016'), V('1.9', 8, 'Feb 29, 2016')]),
  F('1.8', '2014 to 2015', [V('1.8.9', 8, 'Dec 9, 2015'), V('1.8.8', 8, 'Jul 28, 2015'), V('1.8', 8, 'Sep 2, 2014')]),
  F('1.7', '2013 to 2014', [V('1.7.10', 8, 'Jun 26, 2014'), V('1.7.2', 8, 'Oct 25, 2013')]),
  F('1.6', '2013', [V('1.6.4', 8, 'Sep 19, 2013'), V('1.6.2', 8, 'Jul 8, 2013'), V('1.6.1', 8, 'Jul 1, 2013')]),
];

const lv = (id: string, released: string) => ({ id, name: id, released: d(released) });
export const loaders: LoaderOption[] = [
  { kind: 'none', name: 'None', note: 'Vanilla, no mods', stable: [], beta: [] },
  { kind: 'fabric', name: 'Fabric', note: 'Light, updates fast', stable: [lv('0.16.10', 'Jan 8, 2025'), lv('0.16.9', 'Nov 19, 2024'), lv('0.16.8', 'Oct 24, 2024'), lv('0.16.7', 'Oct 12, 2024'), lv('0.16.5', 'Sep 4, 2024')], beta: [lv('0.17.0-beta.2', 'Feb 3, 2025')] },
  { kind: 'quilt', name: 'Quilt', note: 'Runs most Fabric mods', stable: [lv('0.27.1', 'Jan 20, 2025'), lv('0.27.0', 'Dec 10, 2024')], beta: [lv('0.28.0-beta.1', 'Feb 1, 2025')] },
  { kind: 'neoforge', name: 'NeoForge', note: 'Modern Forge', stable: [lv('21.4.121', 'Feb 12, 2025'), lv('21.4.111', 'Jan 30, 2025'), lv('21.4.75', 'Jan 4, 2025')], beta: [lv('21.4.130-beta', 'Feb 20, 2025')] },
  { kind: 'forge', name: 'Forge', note: 'Classic big mods', stable: [lv('54.1.0', 'Feb 10, 2025'), lv('54.0.26', 'Jan 22, 2025')], beta: [] },
];
