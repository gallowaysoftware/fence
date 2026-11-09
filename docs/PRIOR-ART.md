# Prior art (surveyed 2026-11-09)

## Official
[dragonpan2/probably-stolen-modding](https://github.com/dragonpan2/probably-stolen-modding)
is the developer-linked modding repo: `ModdingGuide.md`, `APIChanges.md` (per-update
changelog — **read it after every game update**), the signed `trust-list.json`,
`create-mod.bat`, and a `LearnerMod` example (netstandard2.1).

What it means for us:
- **Negotiation is explicitly unsupported.** No `ModHook` covers it; Harmony on it "can
  break without notice". Our barter integration lives entirely on that surface, so it is
  the part `make compat` exists for.
- Background work must go through `ModHelper.StartCoroutine` / `Delay` /
  `RunInBackground` / `RunOnMainThread`; awaiting `UnityWebRequest` under `network` is
  fine; `Task.Run`, `Task.Delay`, `ContinueWith` are not. Private members via
  `ModReflect`, never `reflection-external`.
- Dev loop: an empty `dev.txt` beside the exe enables the F8 console (`mod-scan <id>`,
  `mod-trust`, `mod-trust-list`) and F9 dev menu; a local mod's *Developer mode* toggle
  skips re-approval on rebuild. `ModLog` for logging, `ModHelper.GetConfig` for settings.
- Decompiling is tolerated ("look, don't copy").

## Community code mods worth reading (for API knowledge — don't copy)
| Repo | Why |
|---|---|
| [Mooooooon/ProbablyAI](https://github.com/Mooooooon/ProbablyAI) | Closest to us: LLM assistant on the official loader, OpenAI-compatible tool calling. Reads customer budget/intent/demands; rebuilds the price stack from player-visible values only (`GameCustomerPricing.cs`). Does not trade. |
| [DrNaki/ProbablyStolen-BestOffer](https://github.com/DrNaki/ProbablyStolen-BestOffer) | Best map of the trade path (`GameTradeAdapter.cs`): `PlayerStore.Instance.currentClientInstance`, `GetBudget()`, `item.GetNegociatedValue()`, `CanSellThisItem`/`SellItem`, `NegociationUIManager.Instance`. |
| Codesoulme/shenkongdangpuMOD `free-chat-bargain-barter` | LLM free-chat bargaining; declares `dynamic-code` + `reflection-external`, so it stops loading 2026-12-10. |
| KCN-judu/ProbablyStolen-DemandHighlight, 0BlueYan0/probably-stolen-demand-panel, ProbablyAssembled `MarketQuoteHooks.cs` | Smaller readers of demand/market state. |

No full "AI plays Probably Stolen" project exists. MelonLoader is broken on 1.0 (Harmony
2.4.2 conflict); demo-era mods were IL2CPP and do not port.

## Haggling mechanics (community claims — unverified, see `docs/re/BARTER.md`)
One bargain per side, one Blackmail, one Threaten per customer; steps 10/25/50/75%;
markup/discount accept ≈ 100 − 2×pct, else stay ≈ 100 − 1.2×pct or walk out (−3 rep);
± faction rep/6; Blackmail on criminals only, ≈ 60 + rep/4; Threaten from day 12.
Sources: [wikily.gg](https://wikily.gg/probably-stolen/haggling),
[gamewikistation](https://gamewikistation.com/games/probably-stolen/systems/bargaining/).
