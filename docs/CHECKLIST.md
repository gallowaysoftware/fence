# In-game checklist (overlay, advise, act)

Setup: `make backup-saves`; bridge running (`make bridge-drift` for advise,
`make bridge-drift-act` for act); launch the game. The mod changed, so the loader may ask to
approve it again unless Developer mode is on. Bridge log: `var/log/<session>.jsonl`; mod
lines: `[fence]` in the newest `GameLog_*.txt` / `Player.log`.

## 1. Load and overlay (advise mode)
- [ ] F8: no `could not patch` lines, no "no overlay window" warning; `mod-scan fence` shows only `network`; bridge logs `session started … compat_ok=true` and nothing under `disabled_features`.
- [ ] The overlay appears on the store screen after loading a save (not in the main menu), reappears after a reload / new day, and doesn't cover the counter or tablet. It drags; text wraps; the Today list scrolls; Hide collapses it to a bar and back; Ask again re-asks (one log line).
- [ ] Header: mode, bridge status, model. Stop the bridge → "bridge down" in header and summary.

## 2. Turns (advise mode)
- [ ] Each customer: name, faction, intent, budget match the tablet. Status goes waiting → thinking… → advice; every option shows with its note; the advised one is marked ▶.
- [ ] Story / wanted / security / barter / appraisal customers show "not covered: …" and get no advice.
- [ ] Dragging items doesn't spam (one ask ~0.75 s after you stop); changing the counter after advice shows stale.
- [ ] Odds and feature text match the tablet and the bargain-button tooltips.
- [ ] Sell-and-buy customers: the advised trade button is the one the tablet shows. Buying customers: budget and the over-budget label are right.
- [ ] Nothing ever happens on its own in advise mode.

## 3. Outcomes and the day (advise mode)
- [ ] Today lists each customer with the right outcome. Bargain outcomes (markup success, refuse-and-stay, walk-out) report correctly; wholesale and multi-item sales arrive as one trade; dismiss / bell / all-done / walk-out / shutter each send `left` with the right cause. All carry `actor: human`.
- [ ] The day summary matches the bridge; day 0 shows "no data yet"; lessons appear after the first night with decisions.
- [ ] Restart the bridge mid-customer: the next ask re-hellos and retries once.
- [ ] Reading bargain flags doesn't reset a rep multiplier; note how often items get a negative local uid.

## 4. Acting (only after 1–3 pass) — `make backup-saves`, `make bridge-drift-act`
- [ ] With `act_enabled` off: header shows "advise (act off in settings)"; nothing acts.
- [ ] Mods → fence → gear → turn on `act_enabled` (restart if not picked up live — note which).
- [ ] "fence will:" then "fence did:" for: `sell_all` in budget; `sell_all` over budget (capped? confirm panel closed? — also once with the tablet never opened this session); `buy_all`; `accept_wholesale`; a bargain step then a sale; `blackmail`; `threaten` (aug); `dismiss`.
- [ ] After a bargain the panel shows the step used and the rep line updated, as if clicked.
- [ ] Human wins: drag an item while it's thinking → "dropped (state changed)", nothing runs.
- [ ] Dialogue at answer time: waits up to 3 s, then acts or drops. At most 4 actions per customer; nothing asked within ~1.5 s of an action.
- [ ] Pause acting stops the next action ("fence: act paused"); Resume works.
- [ ] Outcomes from executed moves carry `actor: mod` under the executed decision id.
- [ ] A buying customer with nothing staged gets stage advice ("offer: …"); the mod never moves items itself, even in act mode.
- [ ] Exit (P2): one full day of standard trades with you only staging items.

## 5. Recording (the bridge runs as the `fence-bridge` user service)
- [ ] Load a save: the overlay's first line says `recording`; the bridge log has a `day_snapshot` with `phase: load`.
- [ ] Sleep and wake: a `services_set`, then `phase: sleep` and `phase: morning` snapshots (morning has the night log).
- [ ] Change the window, open the shutter: `showcase`/`unshowcase` actions, then a `phase: open` snapshot matching the clientele panel.
- [ ] Every customer — story, landlord, wanted, inspector included — produces exactly one `customer_observed`.
- [ ] Your own drags, sales and bargains appear as `human_action` with the advice in force; nothing is recorded for the game's own moves.
- [ ] Night: upgrade bought, going out, scavenging, commissary, requests each recorded.
- [ ] Stop the bridge (`systemctl --user stop fence-bridge`) for >10 s: red `NOT RECORDING`; start it again: the backlog flushes and the line returns to `recording`.
- [ ] `make data-status` afterwards shows snapshots, customers and human actions for the session, no gaps.
