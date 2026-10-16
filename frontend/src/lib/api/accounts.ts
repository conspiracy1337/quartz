// Accounts, Microsoft sign-in, the skin library and capes (Go's accounts.Manager).
import { bridged } from '../bridge';
import { revs } from '../revs.svelte';
import * as Go from '../wailsjs/go/api/Accounts';
import { mockAccounts as mock } from './mock/accounts';
import type { Account, Alert, Skin } from './types';

export const accountsApi = {
  list: () => revs.load('accounts', bridged(Go.List, mock.list)),
  /** Whether this build can sign in with Microsoft (it needs an approved client ID). */
  microsoftReady: bridged(Go.MicrosoftReady, mock.microsoftReady),
  addOffline: bridged(Go.AddOffline, mock.addOffline),
  setDefault: bridged(Go.SetDefault, mock.setDefault),
  remove: bridged(Go.Remove, mock.remove),
  /** Both resolve with the flow number; progress arrives as accounts:signin events. */
  signInBrowser: bridged(Go.SignInBrowser, () => 0),
  signInCode: bridged(Go.SignInCode, () => 0),
  cancelSignIn: bridged(Go.CancelSignIn, () => undefined),

  skins: () => revs.load('accounts', bridged(Go.Skins, mock.skins)),
  capes: (accountId: string) => revs.load('accounts', bridged(() => Go.Capes(accountId), () => mock.capes(accountId))),
  wear: bridged(Go.Wear, mock.wear),
  resetSkin: bridged(Go.ResetSkin, mock.resetSkin),
  /** Opens a native file dialog; resolves with an empty hash when it was cancelled. */
  importSkinFile: bridged(Go.ImportSkinFile, () => mock.importSkin('file')),
  importSkinURL: bridged(Go.ImportSkinURL, (url: string) => mock.importSkin(url)),
  importSkinPlayer: bridged(Go.ImportSkinPlayer, (name: string) => mock.importSkin(name)),
  renameSkin: bridged(Go.RenameSkin, mock.renameSkin),
  setSkinSlim: bridged(Go.SetSkinSlim, mock.setSkinSlim),
  removeSkin: bridged(Go.RemoveSkin, mock.removeSkin),
  restoreSkin: bridged(Go.RestoreSkin, (s: Skin) => mock.restoreSkin(s)),
};

/** Home's "needs to sign in again" alerts, one per account whose refresh failed. */
export const signInAlerts = (list: Account[]): Alert[] =>
  list
    .filter((a) => a.status === 'signin')
    .map((a) => ({ tone: 'warn', subject: a.name, text: 'needs to sign in again', action: 'Sign in', target: { kind: 'sign-in', accountId: a.id } }));
