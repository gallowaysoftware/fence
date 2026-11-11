using System;
using System.Collections.Generic;
using System.IO;
using System.Reflection;
using System.Runtime.CompilerServices;
using System.Text.RegularExpressions;
using Newtonsoft.Json.Linq;

namespace Fence.Sim
{
    // Item footprints for the patched GameItem.BuildShapeFromSprite (GameAssets' item_shapes.json).
    // Without the file every item is 1x1, which is how the headless game ran before it existed.
    public static class Shapes
    {
        static Dictionary<string, Dictionary<string, (int w, byte[] cells)>> atlases;
        public static int Misses { get; private set; }

        public static int Load(string path)
        {
            atlases = null;
            Misses = 0;
            if (!File.Exists(path)) return 0;
            var doc = JObject.Parse(File.ReadAllText(path));
            atlases = new Dictionary<string, Dictionary<string, (int, byte[])>>(StringComparer.Ordinal);
            int n = 0;
            foreach (var a in (JObject)doc["atlases"])
            {
                var sprites = new Dictionary<string, (int, byte[])>(StringComparer.Ordinal);
                foreach (var s in (JObject)a.Value)
                {
                    string cells = (string)s.Value[1];
                    var data = new byte[cells.Length];
                    for (int i = 0; i < cells.Length; i++) data[i] = (byte)(cells[i] == '1' ? 1 : 0);
                    sprites[s.Key] = ((int)s.Value[0], data);
                    n++;
                }
                atlases[a.Key] = sprites;
            }
            return n;
        }

        // RenderHandler.LoadFromAtlas falls back to the unknown sprite for a name it cannot find.
        // Typed object: the patch calls it from inside Assembly-CSharp and casts there.
        [MethodImpl(MethodImplOptions.NoInlining)]
        public static object Build(string atlas, string sprite)
        {
            if (atlases == null) return new GridShapeBuilder(1, 1);
            if (!TryGet(atlas, sprite, out var shape) && !TryGet("Items/itematlas", "unknown", out shape))
            {
                Misses++;
                return new GridShapeBuilder(1, 1);
            }
            return new GridShapeBuilder().SetData((byte[])shape.cells.Clone(), shape.w);
        }

        static bool TryGet(string atlas, string sprite, out (int w, byte[] cells) shape)
        {
            shape = default;
            return atlas != null && sprite != null && atlases.TryGetValue(atlas.ToLowerInvariant(), out var s) && s.TryGetValue(sprite, out shape);
        }
    }

    // The English string tables for the patched LocHelper.Get / HasEntry (GameAssets' loc_en.json).
    // Without the file, Get returns the key and HasEntry is false.
    public static class Loc
    {
        static Dictionary<string, Dictionary<string, string>> tables;

        public static int Load(string path)
        {
            tables = null;
            if (path == null || !File.Exists(path)) return 0;
            var doc = JObject.Parse(File.ReadAllText(path));
            tables = new Dictionary<string, Dictionary<string, string>>(StringComparer.Ordinal);
            int n = 0;
            foreach (var t in (JObject)doc["tables"])
            {
                var d = new Dictionary<string, string>(StringComparer.Ordinal);
                foreach (var e in (JObject)t.Value) d[e.Key] = (string)e.Value;
                tables[t.Key] = d;
                n += d.Count;
            }
            return n;
        }

        public static bool HasEntry(string table, string key) =>
            tables != null && key != null && table != null && tables.TryGetValue(table, out var t) && t.ContainsKey(key);

        static readonly Regex Placeholder = new Regex(@"\{(\d+)(?::[^{}]*)?\}");

        // Unity's smart strings reduced to their positional placeholders; formatter options are dropped.
        public static string Get(string table, string key, object[] args)
        {
            if (!HasEntry(table, key)) return key;
            string text = tables[table][key];
            if (args == null || args.Length == 0) return text;
            return Placeholder.Replace(text, m =>
            {
                int i = int.Parse(m.Groups[1].Value);
                return i < args.Length ? Convert.ToString(args[i], System.Globalization.CultureInfo.InvariantCulture) : m.Value;
            });
        }
    }

    // Rebuilds the scene's TableEntry/TableGroupEntry lists from loot_tables.json and runs the game's
    // own TableMaster.Start / TableGroupMaster.Start (LootRegistry.LoadVanillaTables / LoadVanillaGroups).
    // Without them every ItemSpawner.SpawnFromTable roll makes a blank item (no identifier).
    public static class Loot
    {
        [MethodImpl(MethodImplOptions.NoInlining)]
        public static int Install(string path)
        {
            if (!File.Exists(path)) return 0;
            var doc = JObject.Parse(File.ReadAllText(path));
            var byId = new Dictionary<string, LootTable>(StringComparer.Ordinal);
            var entries = new List<TableEntry>();
            foreach (var t in doc["tables"])
            {
                var lt = (LootTable)RuntimeHelpers.GetUninitializedObject(typeof(LootTable));
                Game.MarkAlive(lt);
                lt.table = List<string>(t["items"], v => (string)v);
                byId[(string)t["id"]] = lt;
                entries.Add(new TableEntry { tableId = (string)t["id"], lootTable = lt });
            }
            var groups = new List<TableGroupEntry>();
            foreach (var g in doc["groups"])
            {
                var tg = (TableGroup)RuntimeHelpers.GetUninitializedObject(typeof(TableGroup));
                Game.MarkAlive(tg);
                tg.tableGroup = List<LootTable>(g["items"], v => v.Type == JTokenType.String && byId.TryGetValue((string)v, out var lt) ? lt : null);
                groups.Add(new TableGroupEntry { tableGroupId = (string)g["id"], tableGroup = tg });
            }
            const BindingFlags I = BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic;
            // LootRegistry.ResolveTableId asks TableMaster.Instance, which must compare non-null.
            Game.MarkAlive(TableMaster.Instance);
            Game.MarkAlive(TableGroupMaster.Instance);
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
