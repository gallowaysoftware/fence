#!/usr/bin/env bash
# Snapshot the game's saves before any session where the mod acts on its own.
set -euo pipefail
DATA="${GAME_DATA:-$HOME/.local/share/Steam/steamapps/compatdata/4348910/pfx/drive_c/users/steamuser/AppData/LocalLow/Questing Goose Studio/Probably Stolen}"
DEST="${BACKUP_DIR:-$HOME/.local/share/fence/save-backups}/$(date +%Y%m%d-%H%M%S)"
shopt -s nullglob
saves=("$DATA"/*.es3)
(( ${#saves[@]} )) || { echo "no saves in $DATA" >&2; exit 1; }
mkdir -p "$DEST"
cp -p "${saves[@]}" "$DEST/"
echo "backed up ${#saves[@]} save(s) to $DEST"
