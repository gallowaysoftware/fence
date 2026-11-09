# Always-on bridge

The bridge runs as a systemd **user** service so every session is recorded whether or not
anything else is running.

```sh
make service-install    # build bin/fence-bridge-svc, install and start the unit
make service-status
make service-logs       # follow the journal
make service-uninstall  # stop, disable, remove the unit (keeps var/ and the env file)
```

- Settings live in `~/.config/fence/bridge.env` (created once from
  `fence-bridge.env.example`); `systemctl --user restart fence-bridge` after editing.
- `observe` (the default) records everything — events, day snapshots, every customer, the
  human's moves, outcomes — into `var/log/*.jsonl` and `var/memory.db`, with no model.
  `advise` and `act` additionally need `FENCE_POLICY=llm` and a reachable model; if the
  model is down the bridge defers and keeps recording.
- The unit runs `bin/fence-bridge-svc`, a copy separate from `bin/bridge`, so a manual
  `make bridge` build never replaces the running service. Only one bridge can hold
  `127.0.0.1:7341`: stop any manual bridge before installing.
- To run when logged out (Steam is usually a login session anyway):
  `loginctl enable-linger $USER`.
- `make data-status` reports what was recorded and any gaps; `make data-export` writes
  per-run trajectories to `var/export/`.
