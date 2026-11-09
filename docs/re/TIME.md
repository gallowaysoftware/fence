# Time during barter (RE notes)

Build 26525552 / 0.46D. Evidence from a full decompile of `Assembly-CSharp.dll`.

## Findings

| Question | Answer | Evidence |
|---|---|---|
| Does the game ever set `Time.timeScale`? | **No.** Only per-tween `Tween.timeScale` for a limping exit | grep: sole hit `StoreClientMono.ApplyLeaveTimeScale(DOTweenPath)` |
| Is there a day clock? | **No.** The day ends only when the player closes the shutter | `StoreShutterButton` → `PlayerStore.EndDay()`; no hour/clock state; `PlayerStore.CanEndDay()` only checks dismissability |
| Customer patience timer? | **None** for ordinary customers | `StoreClient.clientImpatience : System.Int32` is only touched in `AppraisalServiceHelper.OnEstimate(GameItem, System.Int32)` and is action-driven (+49 per refused estimate), not time-driven |
| Is barter already "paused"? | Effectively yes: nothing in a BUY/SELL interaction expires. A customer waits at the counter indefinitely | Exits listed in BARTER.md "Walking out" are all player-action-driven |
| What still runs in real time while a customer waits | see table below | |

### Timers active during an interaction

| Timer | Time source | Effect of `timeScale = 0` |
|---|---|---|
| Customer walk-in / walk-out | `DOTweenPath` component tweens (`StoreClientMono.dOTweenPath`, `pathRight`); `OnArrived`/`OnLeft` callbacks | Freezes (assuming default DOTween update = scaled). `isIndependentUpdate` lives in the prefab, not code — **unverified** |
| Delay before main dialogue | `MetaHelper.WaitAndDo(System.Single, System.Action)` = `WaitForSeconds` (scaled), 0.25 s in `StoreUIManager.OnNextClientArrived` | Freezes |
| Dialogue flow (auto-advance, end actions, item spawning at end of main dialogue) | `DialogUIManager.ScheduleFlow` / `OnCurrentTextDisplayed` → `MetaHelper.WaitAndDo` (scaled); typewriter by Febucci TextAnimator (time mode **unverified**) | Freezes mid-conversation — a SELL customer's items may never appear |
| Call next customer after leave | `StoreClientMono.OnLeft` → `WaitAndDo(0.5f)` | Freezes |
| Tablet auto-refresh | `NegociationUIManager.Update` (`Time.deltaTime`, 0.2 s) | Stops periodic refresh; explicit `RefreshUI()` still works (queued, applied in `LateUpdate`) |
| Bargain panel | `BargainUIManager.Update` → `RefreshUI()` (no time) | Unaffected |
| Item 1-second ticks (machines: fridge, heating plate, auto-butcher, chem finisher, amenities, chemicals) | `EmporiumEntry.Update`, `oneSecondTimer += Time.deltaTime` → `GameItem.onOneSecondItemFunc` | Freezes — machines stop producing while paused |
| Bell / dismiss hold-to-click, input holds | `Time.unscaledTime` (`StoreBell`, `Interactable`, `SelectableElementHandler`) | Unaffected |
| Escape menu, some UI spawners | `WaitForSecondsRealtime` (`EscapeUIManager`, `CustomUIItemSpawner`, `PhoneButton`, `ModHelper`) | Unaffected |
| Phone / reputation notifier | `Time.deltaTime` (`PhoneUIManager`, `ReputationUIManager`) | Freezes |

## Recommendation

**Do not touch `Time.timeScale` for barter.** Nothing in an ordinary BUY/SELL interaction
times out, so the game already "holds" while the LLM thinks. Pausing buys nothing for
correctness and risks freezing dialogue end actions, walk tweens and DOTween state, which
the act path depends on (every bargain/deal method starts a dialogue).

What the mod should do instead while a decision is pending:
1. **Snapshot a signature** at request time: client reference + `identifier`, counter
   contents (`GameItem` refs + `GetNegociatedValue()`), `PlayerStore.playerCash`, bargain
   `…Offered` flags.
2. **Re-validate before executing** (P2): same client, same signature, and the
   "decision ready" predicate from BARTER.md still true (no dialogue, not moving, not trade
   locked). Mismatch → treat as `defer`, log it, re-ask if still relevant.
3. In `act` mode, optionally block human input on the counter/tablet with an overlay so the
   two players do not race; in `advise` mode, don't block anything.
4. Run the mod's own waiting with `UnityWebRequest` + `WaitForSecondsRealtime`/unscaled time,
   so it is unaffected if a later feature does pause.
5. Machines keep ticking during LLM latency (1 s ticks via `Time.deltaTime`). That is a
   small real-time advantage to slow models; acceptable for P1/P2. If it ever matters,
   pause only at a quiescent decision point (predicate true) and restore `timeScale` to 1
   **before** calling any game action — never execute an action while paused.

Remaining risk (#7 in COMPAT.md) is therefore only relevant to a future pause feature;
verify DOTween `isIndependentUpdate` on the client path prefabs and Febucci's time mode
in-game before adopting one.
