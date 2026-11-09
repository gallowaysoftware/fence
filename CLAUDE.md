# CLAUDE.md — fence

An LLM plays **Probably Stolen** (Questing Goose / tinyBuild, Steam app `4348910`, Unity
2021.3.45f2 Mono) through the game's official mod loader. Read `docs/PLAN.md` for phases,
`docs/PROTOCOL.md` for the mod↔bridge contract, `docs/COMPAT.md` before touching anything
that depends on game internals.

## Shape
- `mod/` — C# mod loaded by the game (`manifest.xml` + DLLs in `Mods/fence/`). Thin: it
  observes game state at decision points, asks the bridge, and executes the answer. No
  intelligence lives here.
- `cmd/bridge`, `internal/` — Go sidecar on `127.0.0.1`. Prompting, memory, schema
  validation, LLM clients (OpenAI-compatible + Anthropic), decision log.
- `protocol/` — JSON Schemas + fixtures shared by both sides. A fixture change must pass
  both the Go and the C# contract tests.
- `compat/` — the hook manifest (every game member the mod depends on) and one recorded
  snapshot per game build we have verified against.

## Invariants
- **The mod never blocks the game on the bridge without a deadline.** Every `decide` has
  a deadline; on timeout, error, or invalid answer the mod returns control to the human
  (`defer`). Events are fire-and-forget.
- **The mod may only choose among options it enumerated.** The bridge validates the
  choice against the request's `options` and bounds; it never invents an action.
- **The model plays with a player's information.** `decide.context` holds only
  player-visible values; hidden ones go in `decide.hidden`, which is logged for evaluation
  and training but never reaches a prompt. The model perceives like an expert player:
  every on-screen detail (visual tells described in words) and all game rules, never truth flags.
- **Capabilities: `network` only.** `threads`, `dynamic-code`, `native` and
  `reflection-external` are retiring/refused by the game's `ModScanner`; never use them.
  Network goes through `UnityWebRequest` in coroutines. The mod is never published to
  the Workshop (network mods there need developer verification from 2026-12-10).
- **Every hook is patched individually and may fail individually.** A missing member
  disables the features that need it and is reported in `hello.compat`; it never takes
  the mod down.
- **Never commit game binaries, decompiled game source, or data extracted from the game.**
  Reference the installed game via `GAME_DIR`. Member signatures in `compat/` are fine.
  The simulator's rules are our own code; its data tables are extracted from the local
  install into `var/gamedata/` (gitignored) at build time. Personal research use only.

## Game paths (Linux, Proton)
- Install: `~/.local/share/Steam/steamapps/common/Probably Stolen` (`GAME_DIR`)
- Data/saves/logs: `~/.local/share/Steam/steamapps/compatdata/4348910/pfx/drive_c/users/steamuser/AppData/LocalLow/Questing Goose Studio/Probably Stolen`
  (`GameLog_*.txt`, `mod-audit.log`, `save_*.es3`, `mod-trust-list.json`)
- Steam build: `steamapps/appmanifest_4348910.acf` → `buildid`

## Commands
- `make test` — Go + C# tests. `make mod` / `make mod-install` — build and copy into `GAME_DIR/Mods/fence`.
- `make compat` — check the installed game against `compat/hooks.json`; writes a snapshot.
- `make bridge` — run the bridge.

## Conventions
Go 1.26, `gofmt -s`, table-driven tests. C# matches the game's style; target the game's
own reference assemblies. Comment sparingly. Never push without being asked.
