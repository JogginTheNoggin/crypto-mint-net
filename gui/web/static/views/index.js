// The views, in the order the navigation shows them (GUI-GREEN-044).
//
// Each view is one file in this directory that exports an object
// { id, title, mount(root, ctx) }. mount draws the view into root, using
// the panel and form helpers in ../views.js, and returns a function that
// stops what it started (or nothing). Every action is a poon command line
// those helpers show and run; a view computes nothing poon does not print.
//
// To add a view: write views/NAME.js, add it below, and add a row to
// docs/FEATURES.md. The navigation, the narrow-window menu and the e2e
// checks that walk every view (gui/e2e/parity.mjs) all read this list.
import { nodeView } from './node.js';
import { watchView, exploreView } from './watch.js';
import { workView } from './work.js';
import { bountyView } from './bounty.js';
import { poolsView } from './pools.js';
import { walletView } from './wallet.js';
import { peersView } from './peers.js';
import { policiesView } from './policies.js';
import { cryptoView } from './crypto.js';
import { jobsView } from './jobs.js';
import { settingsView } from './settings.js';

export const views = [nodeView, watchView, exploreView, workView, bountyView, poolsView, walletView, peersView, policiesView, cryptoView, jobsView, settingsView];
