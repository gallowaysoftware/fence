#!/usr/bin/env bash
# Build the mod in Release and install it into "$GAME_DIR/Mods/fence".
# Only manifest.xml and the DLLs it lists are copied: any other DLL in the folder makes the
# loader refuse the mod, so stale ones are removed.
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GAME_DIR="${GAME_DIR:-$HOME/.local/share/Steam/steamapps/common/Probably Stolen}"
dest="$GAME_DIR/Mods/fence"
bin="$here/src/Fence.Mod/bin/Release/netstandard2.1"

if [ ! -f "$GAME_DIR/Probably Stolen_Data/Managed/Assembly-CSharp.dll" ]; then
  echo "install: no game at $GAME_DIR (set GAME_DIR)" >&2
  exit 2
fi

dotnet build "$here/src/Fence.Mod/Fence.Mod.csproj" -c Release -p:GameDir="$GAME_DIR" -nologo -v quiet

mapfile -t dlls < <(sed -n 's:.*<Assembly>\(.*\)</Assembly>.*:\1:p' "$here/src/Fence.Mod/manifest.xml")
mkdir -p "$dest"
find "$dest" -maxdepth 2 -name '*.dll' -delete
cp "$here/src/Fence.Mod/manifest.xml" "$dest/"
for dll in "${dlls[@]}"; do
  cp "$bin/$dll" "$dest/$dll"
done

echo "installed into $dest:"
(cd "$dest" && sha256sum manifest.xml "${dlls[@]}")
