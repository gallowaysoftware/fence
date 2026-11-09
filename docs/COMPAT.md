# Game compatibility & breakage playbook

The game hit 1.0 on 2026-10-28 and the main story ships in a later update. Builds went
100-Rev3 → Rev13 in the first ~11 days, and the developers call negotiation — the surface
we automate — unsupported for mods (`docs/PRIOR-ART.md`). Expect breakage several times a
week. Everything here exists to make "the game updated" a five-minute check, not a
debugging session.

Mod-rule timeline (from the official `APIChanges.md`): 1.0 made `<Assemblies>` and
`<Capabilities>` mandatory; Oct 30 refused process launch; Nov 8 deprecated
`reflection-external`, `dynamic-code`, `native`, `threads` (refused from **2026-12-10**)
and required developer verification for Workshop mods declaring `files`/`network` (local
`Mods/` folders exempt). The trust list's kill switch is `{allMods:false,
exceptVerified:true}` as of the 2026-11-02 issue.

## What identifies a game build
| Field | Source | Verified value |
|---|---|---|
| Steam build id | `steamapps/appmanifest_4348910.acf` → `buildid` | `26525552` |
| `Application.version` | `mod-audit.log` (`audit: game …`), or at runtime | `0.46D` |
| Build label | `"Game: VERSION 100-RevN"` in `Assembly-CSharp.dll`; what `APIChanges.md` and the community cite | `100-Rev13` |
| Unity | `globalgamemanagers` | `2021.3.45f2` |
| `Assembly-CSharp.dll` sha256 | `GAME_DIR/Probably Stolen_Data/Managed/` | see `compat/versions/` |

`make compat` records all four into `compat/versions/<buildid>.json` together with the
result of checking every entry of `compat/hooks.json` against the installed DLL, and a
dump of the public surface of the types we touch (for diffing between builds).

## After a game update
0. Read the new entries in the official `APIChanges.md`.
1. `make compat`. Green → play. Red → the report names each missing/changed member and
   the features it disables.
2. `git diff compat/versions/` against the previous build's surface dump to see what moved.
3. Fix the hooks, re-run, then play one day in `advise` mode before `act`.

Set Steam → Probably Stolen → Properties → Updates to *only update when I launch it*.
Steam cannot pin an old build for normal users; rolling back means
`download_depot 4348910 <depot> <manifest>` in the Steam console. Record the manifest id
in the snapshot so that is possible.

## Anticipated breakages
| # | Breakage | Detection | Response |
|---|---|---|---|
| 1 | Update renames/removes a member we hook | `make compat`; runtime per-hook patch failure → `hello.compat.missing` | Each hook patched in its own try/catch; dependent features disabled, rest keeps working |
| 2 | Same signature, changed behaviour (semantic drift) | Context sanity checks in the mod (non-empty options, non-negative prices, known client kind); bridge-side replay of recorded contexts | Drop to `advise` until a human confirms a day of play |
| 3 | `ModScanner` rules tighten (more hard rules, more capabilities retired) | Scanner-conformance test runs the game's own `ModScanner` against our built DLL | Keep capabilities to `network`; no threads, no reflection outside the game assembly |
| 4 | Remote trust list blocks us / kill switch | `mod-audit.log` (`blocked …`, `kill switch on`) | Local-folder mod, never published; the list is issued by the developers, nothing to do but ask them |
| 5 | Local fingerprint changes every rebuild → re-approval prompt | Expected | Accept the prompt; not a bug |
| 6 | Bridge down or LLM slow | Mod-side deadline | Mod defers to the human; never waits unbounded |
| 7 | State changes while the model thinks (human acts, dialogue fires) | `state_signature` re-check before executing | Defer. The game is never paused: `Time.timeScale = 0` would freeze the scaled dialogue delays and walk tweens every action depends on (`docs/re/TIME.md`) |
| 7b | Public bargain/trade methods skip checks the UI enforces (cash for wholesale, percent steps, perks, dialogue running) | — | The mod replicates the UI gates and only offers options a player could click (`docs/re/BARTER.md`) |
| 8 | An `act` choice leaves the save in a bad state | Game warnings in `GameLog_*.txt` | `make backup-saves` before every `act` session |
| 9 | Mod and bridge disagree on the protocol | `hello.protocol`, shared fixtures tested on both sides | Bridge 409s; mod stays vanilla |
| 10 | Updates add new client/decision types | Unknown `kind` | Bridge returns `defer`; never an error |
| 11 | LLM returns invalid JSON / illegal option | Bridge validation | One repair attempt, then `defer` |
| 12 | Game switches to IL2CPP | No `Managed/` folder | Harmony/C# approach dies; would need BepInEx+Il2CppInterop. Unlikely while it ships its own mod loader |
| 14 | Reflection on a computed type (`obj.GetType().GetField`) is charged as `reflection-external` — undeclared, so refused today | Scanner-conformance test | Reflection only from a literal `typeof`, one `RuntimeResolver` case per type (a test enforces it) |
| 15 | `ModHelper.StartCoroutine` returns null during `Init`/`OnEnable` | — | Start the loop lazily from the first hook; own `MonoBehaviour` as fallback |
| 16 | Warnings that become refusals on 2026-12-10 (coroutines on foreign MonoBehaviours, `Task`/`Thread`/timers, Newtonsoft `TypeNameHandling` ≠ None, `[ModRestricted]` helpers) | Conformance test already fails on them | — |
| 17 | Unknown build: `hello.game.assembly_sha256` empty | Bridge sees an unrecorded `assembly_mvid` | `make compat`, rebuild, reinstall |
| 18 | The conformance test sets private `ModLoader.gameAssemblyNames` | Test breaks while the mod is fine | Update the test, not the mod |
| 19 | A Unity 2022+ upgrade may block plain `http://127.0.0.1` | No `hello` | Allow insecure HTTP or move the bridge to TLS |
| 20 | Bargain roll results live in private `BargainUIManager` fields (`markupApplied`, `buyingDiscountApplied`, `blackmailApplied`, `threatenApplied`) — likeliest to change | `make compat` (offline only: `ModReflect` is a scanner guard type, so the runtime resolver cannot check it) | `accepted` is omitted when unreadable, never guessed |
| 21 | Our copy of the UI's button gates and tablet-mode logic drifts from the game's | Not detectable by compat (same signatures, new rules); table tests pin only our copy | After each update, compare `NegociationUIManager.RefreshBargainControl` / `BargainUIManager.RefreshUI` in the surface diff, and spot-check option sets against the tablet in advise mode |
| 22 | Patches outliving the mod after a kill switch or pause | — | Harmony id is the loader's own `mod.fence`, which it unpatches itself |
| 23 | Coverage gate reads `isWanted`; a stealth-wanted client silently gets no advice, a tiny tell | — | Accepted for P1 |
| 24 | `OnConfirmPanelYes` does nothing when the tablet was never built this session, so an over-budget `sell_all` may silently not happen | Act checklist | The signature doesn't change, so the human finishes it; fix by opening the tablet first if confirmed |
| 25 | `OnSellAllClick`/`OnBuyAllClick` call `TutorialUIManager.Instance.BuyDealed()` without a null check | Exception in the executor | Act turns off for the session (fail closed) |
| 26 | Bargain methods called with the panel closed (unverified) | Act checklist | The mod calls `RefreshUI()` afterwards as the button does |
| 27 | The overlay uses game-internal `CustomUIManager` windows at a hard-coded position | Missing member → `display.overlay` disabled | Overlay only; advice and acting are unaffected |
| 28 | The game is never paused: an action can land between the player's hover and click | Signature check catches state changes, not intent | Pause acting from the overlay while handling items by hand |
| 13 | Steam moves to a sandboxed (Flatpak) install with its own network namespace | `hello` never arrives | Bridge must listen where the sandbox can reach it |

## Verified builds
See `compat/versions/`. One file per Steam build id we have checked.
