using System;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using System.Net.Http;
using AssetsTools.NET;
using AssetsTools.NET.Extra;
using Newtonsoft.Json;
using Newtonsoft.Json.Linq;

namespace Fence.Sim
{
    // Engine-side data the headless game cannot load itself, read from the installed game with
    // AssetsTools.NET into var/gamedata/ (derived game data: never commit it). Each file records the
    // hash of the asset file it came from and is re-read only when that changes.
    //   loot_tables.json  TableMaster/TableGroupMaster's ScriptableObjects (scene level1)
    //   item_shapes.json  every item sprite's grid footprint, as GameItem.BuildShapeFromSprite computes it
    //   loc_en.json       the English Unity Localization string tables (addressables)
    public static class GameAssets
    {
        // Unity's class database for reading engine types from player builds (UABEA's release file).
        const string ClassDataUrl = "https://raw.githubusercontent.com/nesrak1/UABEA/master/ReleaseFiles/classdata.tpk";

        public static string LootPath(string gamedata) => Path.Combine(gamedata, "loot_tables.json");
        public static string ShapesPath(string gamedata) => Path.Combine(gamedata, "item_shapes.json");
        public static string LocPath(string gamedata) => Path.Combine(gamedata, "loc_en.json");

        // var/ is the parent of gamedata; classdata.tpk is cached in var/tools.
        public static void EnsureExtracted(string gameDir, string gamedata)
        {
            string data = Path.Combine(gameDir, "Probably Stolen_Data");
            string tpk = Path.Combine(Path.GetDirectoryName(gamedata), "tools", "classdata.tpk");
            AssetsManager am = null;
            AssetsManager Am()
            {
                if (am != null) return am;
                if (!File.Exists(tpk))
                {
                    System.IO.Directory.CreateDirectory(Path.GetDirectoryName(tpk));
                    using var http = new HttpClient();
                    File.WriteAllBytes(tpk, http.GetByteArrayAsync(ClassDataUrl).GetAwaiter().GetResult());
                }
                am = new AssetsManager();
                am.LoadClassPackage(tpk);
                am.MonoTempGenerator = new MonoCecilTempGenerator(Path.Combine(data, "Managed"));
                return am;
            }
            System.IO.Directory.CreateDirectory(gamedata);
            string level1 = Path.Combine(data, "level1");
            if (!Fresh(LootPath(gamedata), level1)) Write(LootPath(gamedata), level1, "loot_tables", ExtractLoot(Am(), level1));
            string ggm = Path.Combine(data, "globalgamemanagers");
            string res = Path.Combine(data, "resources.assets");
            if (!Fresh(ShapesPath(gamedata), res)) Write(ShapesPath(gamedata), res, "item_shapes", ExtractShapes(Am(), ggm));
            string en = Path.Combine(data, "StreamingAssets", "aa", "StandaloneWindows64", "localization-string-tables-english(en)_assets_all.bundle");
            string shared = Path.Combine(data, "StreamingAssets", "aa", "StandaloneWindows64", "localization-assets-shared_assets_all.bundle");
            if (File.Exists(en) && File.Exists(shared) && !Fresh(LocPath(gamedata), en)) Write(LocPath(gamedata), en, "loc_en", ExtractLoc(Am(), en, shared));
            am?.UnloadAll(true);
        }

        static bool Fresh(string path, string source)
        {
            if (!File.Exists(path)) return false;
            try
            {
                return (string)JObject.Parse(File.ReadAllText(path))["source"]?["sha256"] == Headless.Sha256(source);
            }
            catch (JsonException)
            {
                return false;
            }
        }

        static void Write(string path, string source, string table, JObject body)
        {
            var doc = new JObject { ["source"] = new JObject { ["game"] = "Probably Stolen", ["file"] = Path.GetFileName(source), ["sha256"] = Headless.Sha256(source), ["table"] = table } };
            foreach (var kv in body) doc[kv.Key] = kv.Value;
            File.WriteAllText(path, doc.ToString(Formatting.Indented) + "\n");
        }

        // ---- loot tables -------------------------------------------------------------------

        static JObject ExtractLoot(AssetsManager am, string levelPath)
        {
            var level = am.LoadAssetsFile(levelPath, true);
            am.LoadClassDatabaseFromPackage(level.file.Metadata.UnityVersion);
            var tables = new JArray();
            var groups = new JArray();
            var tableIdByAsset = new Dictionary<(string file, long id), string>();
            AssetTypeValueField tableMaster = null, groupMaster = null;
            foreach (var info in level.file.GetAssetsOfType(AssetClassID.MonoBehaviour))
            {
                var bf = am.GetBaseField(level, info);
                string cls = am.GetExtAsset(level, bf["m_Script"]).baseField?["m_ClassName"].AsString;
                if (cls == "TableMaster") tableMaster = bf;
                if (cls == "TableGroupMaster") groupMaster = bf;
            }
            if (tableMaster == null) throw new InvalidOperationException("no TableMaster in level1");

            foreach (var entry in tableMaster["tableEntries"]["Array"].Children)
            {
                string id = entry["tableId"].AsString;
                var ext = am.GetExtAsset(level, entry["lootTable"]);
                if (ext.baseField == null) continue;
                tableIdByAsset[(ext.file.name, ext.info.PathId)] = id;
                tables.Add(new JObject { ["id"] = id, ["name"] = ext.baseField["m_Name"].AsString, ["items"] = LootItems(ext.baseField["table"], v => new JValue(v.AsString)) });
            }
            if (groupMaster != null)
                foreach (var entry in groupMaster["tableGroupEntries"]["Array"].Children)
                {
                    var ext = am.GetExtAsset(level, entry["tableGroup"]);
                    if (ext.baseField == null) continue;
                    var items = LootItems(ext.baseField["tableGroup"], v =>
                    {
                        var t = am.GetExtAsset(ext.file, v);
                        if (t.baseField == null) return JValue.CreateNull();
                        return tableIdByAsset.TryGetValue((t.file.name, t.info.PathId), out var tid) ? new JValue(tid) : new JValue("name:" + t.baseField["m_Name"].AsString);
                    });
                    groups.Add(new JObject { ["id"] = entry["tableGroupId"].AsString, ["items"] = items });
                }
            return new JObject { ["tables"] = tables, ["groups"] = groups };
        }

        static JArray LootItems(AssetTypeValueField list, Func<AssetTypeValueField, JToken> value)
        {
            var arr = new JArray();
            foreach (var it in list["m_ProbabilityItems"]["Array"].Children)
                arr.Add(new JObject { ["value"] = value(it["m_Value"]), ["p"] = it["m_BaseProbability"].AsFloat, ["enabled"] = it["m_Enabled"].AsBool });
            return arr;
        }

        // ---- item shapes -------------------------------------------------------------------

        // GameItem.BuildShapeFromSprite: one cell per gridSize-pixel block of the sprite's rect, set
        // where the texture's mip level sqrt(gridSize) has alpha > 0 (one texel per cell).
        public const int GridSize = 16;

        static JObject ExtractShapes(AssetsManager am, string ggmPath)
        {
            var ggm = am.LoadAssetsFile(ggmPath, true);
            am.LoadClassDatabaseFromPackage(ggm.file.Metadata.UnityVersion);
            var rm = am.GetBaseField(ggm, ggm.file.GetAssetsOfType(AssetClassID.ResourceManager).First());
            int mip = (int)Math.Sqrt(GridSize);
            var atlases = new SortedDictionary<string, SortedDictionary<string, JArray>>(StringComparer.Ordinal);
            var textures = new Dictionary<(string, long), (int w, int h, Func<int, int, bool> opaque)>();
            int unsupported = 0, sprites = 0;
            foreach (var pair in rm["m_Container"]["Array"].Children)
            {
                string path = pair["first"].AsString;
                if (!path.StartsWith("items/") && !path.StartsWith("ships/")) continue;
                var ext = am.GetExtAsset(ggm, pair["second"]);
                if (ext.info == null || ext.info.TypeId != (int)AssetClassID.Sprite) continue;
                var s = ext.baseField;
                var texRef = am.GetExtAsset(ext.file, s["m_RD"]["texture"]);
                if (texRef.baseField == null) continue;
                var key = (texRef.file.name, texRef.info.PathId);
                if (!textures.TryGetValue(key, out var tex))
                {
                    tex = Mip(texRef.baseField, texRef.file, mip);
                    textures[key] = tex;
                    if (tex.opaque == null) unsupported++;
                }
                float x = s["m_Rect"]["x"].AsFloat, y = s["m_Rect"]["y"].AsFloat, rw = s["m_Rect"]["width"].AsFloat, rh = s["m_Rect"]["height"].AsFloat;
                int w = Math.Max(1, (int)rw / GridSize), h = Math.Max(1, (int)rh / GridSize);
                var cells = new char[w * h];
                for (int i = 0; i < h; i++)
                    for (int j = 0; j < w; j++)
                    {
                        int px = (int)x / GridSize + j, py = (int)y / GridSize + (h - 1 - i);
                        cells[i * w + j] = tex.opaque == null || tex.opaque(px, py) ? '1' : '0';
                    }
                if (!atlases.TryGetValue(path, out var bySprite)) atlases[path] = bySprite = new SortedDictionary<string, JArray>(StringComparer.Ordinal);
                bySprite[s["m_Name"].AsString] = new JArray(w, new string(cells));
                sprites++;
            }
            var obj = new JObject();
            foreach (var a in atlases) obj[a.Key] = new JObject(a.Value.Select(kv => new JProperty(kv.Key, kv.Value)));
            return new JObject { ["grid_size"] = GridSize, ["sprites"] = sprites, ["unsupported_textures"] = unsupported, ["atlases"] = obj };
        }

        // The texture's mip level as an alpha test, reading the stored mip chain (the game's GetPixel(x, y, mip)).
        static (int, int, Func<int, int, bool>) Mip(AssetTypeValueField t, AssetsFileInstance file, int level)
        {
            int w = t["m_Width"].AsInt, h = t["m_Height"].AsInt, fmt = t["m_TextureFormat"].AsInt, mips = t["m_MipCount"].AsInt;
            int bpp = fmt switch { 1 => 1, 3 => 3, 4 => 4, 5 => 4, _ => 0 };
            if (bpp == 0 || mips <= level) return (w, h, null);
            byte[] bytes = t["image data"].AsByteArray;
            if (bytes == null || bytes.Length == 0)
            {
                var sd = t["m_StreamData"];
                string p = sd["path"].AsString;
                if (p.StartsWith("archive:")) return (w, h, null);
                using var fs = File.OpenRead(Path.Combine(Path.GetDirectoryName(file.path), p));
                fs.Seek((long)sd["offset"].AsULong, SeekOrigin.Begin);
                bytes = new byte[sd["size"].AsUInt];
                fs.ReadExactly(bytes);
            }
            long off = 0;
            for (int l = 0; l < level; l++) off += (long)Math.Max(1, w >> l) * Math.Max(1, h >> l) * bpp;
            int mw = Math.Max(1, w >> level), mh = Math.Max(1, h >> level);
            int alpha = fmt switch { 1 => 0, 4 => 3, 5 => 0, _ => -1 };
            return (w, h, (x, y) =>
            {
                if (x < 0 || y < 0 || x >= mw || y >= mh) return false;
                if (alpha < 0) return true;
                return bytes[off + ((long)y * mw + x) * bpp + alpha] > 0;
            });
        }

        // ---- localization ------------------------------------------------------------------

        static JObject ExtractLoc(AssetsManager am, string tablesBundle, string sharedBundle)
        {
            var keysByCollection = new Dictionary<string, Dictionary<long, string>>(StringComparer.Ordinal);
            foreach (var f in Files(am, sharedBundle))
                foreach (var info in f.file.GetAssetsOfType(AssetClassID.MonoBehaviour))
                {
                    var bf = am.GetBaseField(f, info);
                    var entries = bf["m_Entries"];
                    if (entries.IsDummy) continue;
                    var keys = new Dictionary<long, string>();
                    foreach (var e in entries["Array"].Children) keys[e["m_Id"].AsLong] = e["m_Key"].AsString;
                    keysByCollection[bf["m_TableCollectionName"].AsString] = keys;
                }
            var tables = new SortedDictionary<string, SortedDictionary<string, string>>(StringComparer.Ordinal);
            foreach (var f in Files(am, tablesBundle))
                foreach (var info in f.file.GetAssetsOfType(AssetClassID.MonoBehaviour))
                {
                    var bf = am.GetBaseField(f, info);
                    var data = bf["m_TableData"];
                    if (data.IsDummy) continue;
                    string name = bf["m_Name"].AsString;
                    string collection = name.EndsWith("_en") ? name.Substring(0, name.Length - 3) : name;
                    if (!keysByCollection.TryGetValue(collection, out var keys)) continue;
                    var table = new SortedDictionary<string, string>(StringComparer.Ordinal);
                    foreach (var e in data["Array"].Children)
                        if (keys.TryGetValue(e["m_Id"].AsLong, out var key)) table[key] = e["m_Localized"].AsString;
                    tables[collection] = table;
                }
            return new JObject { ["tables"] = JObject.FromObject(tables) };
        }

        static IEnumerable<AssetsFileInstance> Files(AssetsManager am, string bundlePath)
        {
            var b = am.LoadBundleFile(bundlePath);
            for (int i = 0; i < b.file.BlockAndDirInfo.DirectoryInfos.Count; i++)
            {
                var f = am.LoadAssetsFileFromBundle(b, i, false);
                if (f == null) continue;
                am.LoadClassDatabaseFromPackage(f.file.Metadata.UnityVersion);
                yield return f;
            }
        }
    }
}
