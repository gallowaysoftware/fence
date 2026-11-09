using System;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using System.Net.Http;
using System.Reflection;
using System.Runtime.CompilerServices;
using AssetsTools.NET;
using AssetsTools.NET.Extra;
using Newtonsoft.Json;
using Newtonsoft.Json.Linq;

namespace Fence.Gym
{
    // The game's loot tables are ScriptableObjects wired into TableMaster/TableGroupMaster in the
    // scene (level1), not code: headless they are empty and every ItemSpawner.SpawnFromTable roll
    // makes a blank item (no identifier), which scavenger stock and the dumping ground rely on. They
    // are read from the installed game's assets into var/gamedata/loot_tables.json (derived game
    // data: never commit it) and fed through the game's own LootRegistry.LoadVanilla* at new game.
    internal static class Loot
    {
        // Unity's class database for reading engine types from player builds (UABEA's release file).
        const string ClassDataUrl = "https://raw.githubusercontent.com/nesrak1/UABEA/master/ReleaseFiles/classdata.tpk";

        public static string Path => Boot.Var("gamedata", "loot_tables.json");

        public static void EnsureExtracted(string gameDir)
        {
            string data = System.IO.Path.Combine(gameDir, "Probably Stolen_Data");
            string sha = Sim.Headless.Sha256(System.IO.Path.Combine(data, "level1"));
            if (File.Exists(Path))
            {
                try
                {
                    if ((string)JObject.Parse(File.ReadAllText(Path))["source"]?["level1_sha256"] == sha) return;
                }
                catch (JsonException) { }
            }
            string tpk = Boot.Var("tools", "classdata.tpk");
            if (!File.Exists(tpk))
            {
                System.IO.Directory.CreateDirectory(System.IO.Path.GetDirectoryName(tpk));
                using var http = new HttpClient();
                File.WriteAllBytes(tpk, http.GetByteArrayAsync(ClassDataUrl).GetAwaiter().GetResult());
            }
            var am = new AssetsManager();
            am.LoadClassPackage(tpk);
            am.MonoTempGenerator = new MonoCecilTempGenerator(System.IO.Path.Combine(data, "Managed"));
            var level = am.LoadAssetsFile(System.IO.Path.Combine(data, "level1"), true);
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
                tables.Add(new JObject { ["id"] = id, ["name"] = ext.baseField["m_Name"].AsString, ["items"] = Items(ext.baseField["table"], v => new JValue(v.AsString)) });
            }
            if (groupMaster != null)
                foreach (var entry in groupMaster["tableGroupEntries"]["Array"].Children)
                {
                    var ext = am.GetExtAsset(level, entry["tableGroup"]);
                    if (ext.baseField == null) continue;
                    var items = Items(ext.baseField["tableGroup"], v =>
                    {
                        var t = am.GetExtAsset(ext.file, v);
                        if (t.baseField == null) return JValue.CreateNull();
                        return tableIdByAsset.TryGetValue((t.file.name, t.info.PathId), out var tid) ? new JValue(tid) : new JValue("name:" + t.baseField["m_Name"].AsString);
                    });
                    groups.Add(new JObject { ["id"] = entry["tableGroupId"].AsString, ["items"] = items });
                }

            var doc = new JObject
            {
                ["source"] = new JObject { ["game"] = "Probably Stolen", ["level1_sha256"] = sha, ["table"] = "loot_tables" },
                ["tables"] = tables,
                ["groups"] = groups,
            };
            System.IO.Directory.CreateDirectory(System.IO.Path.GetDirectoryName(Path));
            File.WriteAllText(Path, doc.ToString(Formatting.Indented) + "\n");
        }

        static JArray Items(AssetTypeValueField list, Func<AssetTypeValueField, JToken> value)
        {
            var arr = new JArray();
            foreach (var it in list["m_ProbabilityItems"]["Array"].Children)
                arr.Add(new JObject { ["value"] = value(it["m_Value"]), ["p"] = it["m_BaseProbability"].AsFloat, ["enabled"] = it["m_Enabled"].AsBool });
            return arr;
        }

        // Rebuilds the scene's TableEntry/TableGroupEntry lists from the JSON and runs the game's own
        // TableMaster.Start / TableGroupMaster.Start (LootRegistry.LoadVanillaTables / LoadVanillaGroups).
        [MethodImpl(MethodImplOptions.NoInlining)]
        public static int Install()
        {
            if (!File.Exists(Path)) return 0;
            var doc = JObject.Parse(File.ReadAllText(Path));
            var byId = new Dictionary<string, LootTable>(StringComparer.Ordinal);
            var entries = new List<TableEntry>();
            foreach (var t in doc["tables"])
            {
                var lt = (LootTable)RuntimeHelpers.GetUninitializedObject(typeof(LootTable));
                Sim.Game.MarkAlive(lt);
                lt.table = List<string>(t["items"], v => (string)v);
                byId[(string)t["id"]] = lt;
                entries.Add(new TableEntry { tableId = (string)t["id"], lootTable = lt });
            }
            var groups = new List<TableGroupEntry>();
            foreach (var g in doc["groups"])
            {
                var tg = (TableGroup)RuntimeHelpers.GetUninitializedObject(typeof(TableGroup));
                Sim.Game.MarkAlive(tg);
                tg.tableGroup = List<LootTable>(g["items"], v => v.Type == JTokenType.String && byId.TryGetValue((string)v, out var lt) ? lt : null);
                groups.Add(new TableGroupEntry { tableGroupId = (string)g["id"], tableGroup = tg });
            }
            const BindingFlags I = BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic;
            typeof(TableMaster).GetField("tableEntries", I).SetValue(TableMaster.Instance, entries);
            typeof(TableMaster).GetMethod("Start", I).Invoke(TableMaster.Instance, null);
            var gm = TableGroupMaster.Instance;
            typeof(TableGroupMaster).GetField("tableGroupEntries", I).SetValue(gm, groups);
            typeof(TableGroupMaster).GetMethod("Start", I).Invoke(gm, null);
            return entries.Count + groups.Count;
        }

        static RNGNeeds.ProbabilityList<T> List<T>(JToken items, Func<JToken, T> value)
        {
            const BindingFlags I = BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic;
            var list = (RNGNeeds.ProbabilityList<T>)Activator.CreateInstance(typeof(RNGNeeds.ProbabilityList<T>), true);
            var listField = typeof(RNGNeeds.ProbabilityList<T>).GetField("m_ProbabilityItems", I);
            var raw = new List<RNGNeeds.ProbabilityItem<T>>();
            foreach (var it in items)
            {
                var pi = (RNGNeeds.ProbabilityItem<T>)RuntimeHelpers.GetUninitializedObject(typeof(RNGNeeds.ProbabilityItem<T>));
                typeof(RNGNeeds.ProbabilityItem<T>).GetField("m_Value", I).SetValue(pi, value(it["value"]));
                typeof(RNGNeeds.ProbabilityItem<T>).GetField("m_BaseProbability", I).SetValue(pi, (float)it["p"]);
                typeof(RNGNeeds.ProbabilityItem<T>).GetField("m_Enabled", I).SetValue(pi, (bool)it["enabled"]);
                raw.Add(pi);
            }
            listField.SetValue(list, raw);
            return list;
        }
    }
}
