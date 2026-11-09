# fence

LLMs learning to play [Probably Stolen](https://store.steampowered.com/app/4348910/), the
cyberpunk pawnshop sim by Questing Goose Studio / tinyBuild.

- **A mod** on the game's official mod loader (`network` capability only) that records
  everything a player can see and do, shows advice in a turn-based overlay (one customer is
  a turn, one day a meta-turn), and can optionally act on the counter.
- **A bridge** (Go) the mod talks to over localhost: per-customer advice, a morning
  strategist that plans the day and scores it at night, memory of prices and outcomes, and
  any OpenAI-compatible or Anthropic model behind it.
- **A gym** that runs the game's own logic headless from your installed copy, so policies can
  play thousands of days per minute under the real rules.
- **Research notes** on how the game works (`docs/re/`), what experienced players know
  (`docs/knowledge/`), and what experiments show (`docs/research/`).

Start with [`docs/PLAN.md`](docs/PLAN.md), [`docs/PROTOCOL.md`](docs/PROTOCOL.md) and
[`docs/COMPAT.md`](docs/COMPAT.md).

## Not affiliated, nothing included

This is an unofficial fan project, not affiliated with or endorsed by Questing Goose Studio or
tinyBuild. It contains no game code, assets or data: everything game-derived (data tables, a
patched headless copy of the game's assemblies) is produced locally from your own installed
copy into `var/`, which is never committed. You need to own the game. The mod follows the
developers' [modding code of conduct](https://github.com/dragonpan2/probably-stolen-modding),
is never published to the Workshop, and documents its behaviour openly.

## License

MIT — see [LICENSE](LICENSE). "Probably Stolen" is a trademark of its owners.
