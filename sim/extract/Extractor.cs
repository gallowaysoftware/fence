using System;
using System.Collections;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using System.Reflection;
using System.Text.Json;

using Fence.Sim;

namespace Fence.Extract
{
    sealed class BuildInfo
    {
        public string BuildId, AssemblySha256;

        public static BuildInfo Read(string gameDir, string managed)
        {
            var b = new BuildInfo { AssemblySha256 = Headless.Sha256(Path.Combine(managed, "Assembly-CSharp.dll")) };
            string acf = Path.GetFullPath(Path.Combine(gameDir, "..", "..", "appmanifest_4348910.acf"));
            if (File.Exists(acf))
            {
                var line = File.ReadLines(acf).FirstOrDefault(l => l.Trim().StartsWith("\"buildid\""));
                if (line != null) b.BuildId = line.Split('"', StringSplitOptions.RemoveEmptyEntries).Last().Trim();
            }
            return b;
        }
    }

    // Writes var/gamedata/*.json. Every table is produced by running the game's own code headless
    // (factories, initializers, generators) and reading the objects it builds.
    static class Extractor
    {
        public const int Version = 1;
        const int ItemSamples = 24;
        const int ClientSamples = 12;
        const int QueueDays = 60;

        static BuildInfo build;
        static string outDir;
        static readonly List<string> report = new List<string>();

        public static int All(Game game, string dir, BuildInfo b)
        {
            build = b;
            outDir = dir;
            System.IO.Directory.CreateDirectory(dir);
            report.AddRange(game.Errors);

            Step("items", () => Items(game));
            Step("new game", Game.StartEmporium);
            Step("reputation", Reputation);
            Step("upgrades", Upgrades);
            Step("services", Services);
            Step("difficulty", Difficulties);
            Step("market events", MarketEvents);
            Step("afterhours", Afterhours);
            Step("starting perks", StartingPerks);
            Step("constants", Constants);
            Step("client pools", ClientPools);
            Step("days", () => { PlayerStore.Instance.StartNewGame(); Days(); });
            Step("normal mix", () => { PlayerStore.Instance.StartNewGame(); NormalMix(); });
            Step("clients", () => { PlayerStore.Instance.StartNewGame(); Clients(game); });

            Write("extract_report", w =>
            {
                w.WriteStartArray("problems");
                foreach (var r in report) w.WriteStringValue(r);
                w.WriteEndArray();
            });
            Console.WriteLine($"wrote {dir} ({report.Count} problems; see extract_report.json)");
            return 0;
        }

        static void Step(string name, Action a)
        {
            Headless.Seed(StableSeed(name));
            try { a(); Console.WriteLine("ok   " + name); }
            catch (Exception e)
            {
                report.Add(name + ": " + Game.Inner(e));
                Console.WriteLine("FAIL " + name + ": " + Game.Root(e));
            }
        }

        static void Write(string table, Action<Utf8JsonWriter> body)
        {
            using var f = File.Create(Path.Combine(outDir, table + ".json"));
            using var w = new Utf8JsonWriter(f, new JsonWriterOptions { Indented = true, Encoder = System.Text.Encodings.Web.JavaScriptEncoder.UnsafeRelaxedJsonEscaping });
            w.WriteStartObject();
            w.WriteStartObject("source");
            w.WriteString("game", "Probably Stolen");
            w.WriteString("steam_buildid", build.BuildId);
            w.WriteString("assembly_csharp_sha256", build.AssemblySha256);
            w.WriteNumber("extractor_version", Version);
            w.WriteString("table", table);
            w.WriteEndObject();
            body(w);
            w.WriteEndObject();
        }

        static int StableSeed(string s)
        {
            unchecked
            {
                int h = (int)2166136261;
                foreach (char c in s) h = (h ^ c) * 16777619;
                return h & 0x7fffffff;
            }
        }

        // ---- items --------------------------------------------------------------------------

        static void Items(Game game)
        {
            Write("items", w =>
            {
                w.WriteNumber("samples_per_item", ItemSamples);
                w.WriteStartArray("failed");
                var failed = new List<string>();
                foreach (var id in game.ItemDirectoryOf.Keys)
                {
                    try { game.CreateItem(id); }
                    catch (Exception e) { failed.Add(id); report.Add("item " + id + ": " + Game.Root(e)); w.WriteStringValue(id); }
                }
                w.WriteEndArray();
                w.WriteStartArray("items");
                foreach (var (id, dir) in game.ItemDirectoryOf)
                {
                    if (failed.Contains(id)) continue;
                    var samples = new List<GameItem>();
                    for (int i = 0; i < ItemSamples; i++)
                    {
                        Headless.Seed(StableSeed(id) + i);
                        try { samples.Add(game.CreateItem(id)); } catch { }
                    }
                    if (samples.Count == 0) continue;
                    WriteItem(w, id, dir, samples);
                }
                w.WriteEndArray();
            });
        }

        public static void WriteItem(Utf8JsonWriter w, string id, string dir, List<GameItem> samples)
        {
            var first = samples[0];
            w.WriteStartObject();
            w.WriteString("id", id);
            if (dir != null) w.WriteString("directory", dir);
            w.WriteString("name_key", first.name);
            w.WriteStartArray("types");
            foreach (var t in samples.SelectMany(s => s.GetGameItemType()).Distinct().OrderBy(x => x, StringComparer.Ordinal)) w.WriteStringValue(t);
            w.WriteEndArray();

            w.WriteStartObject("tags");
            foreach (var tag in samples.SelectMany(s => s.state.dict.Keys).Distinct().OrderBy(x => x, StringComparer.Ordinal))
            {
                var states = samples.Select(s => s.state.dict.TryGetValue(tag, out var st) ? st : null).ToList();
                var on = states.Where(st => st != null && st.IsEnabled()).ToList();
                if (on.Count == 0) continue;
                w.WriteStartObject(tag);
                if (on.Count != samples.Count) w.WriteNumber("freq", Math.Round((double)on.Count / samples.Count, 3));
                var ints = on.Select(st => st.valueInt).Distinct().ToList();
                if (ints.Count > 1 || ints[0] != 0) Range(w, "int", on.Select(st => (double)st.valueInt));
                var strs = on.Select(st => st.valueString).Where(x => !string.IsNullOrEmpty(x)).Distinct().ToList();
                if (strs.Count == 1) w.WriteString("string", strs[0]);
                else if (strs.Count > 1) { w.WriteStartArray("strings"); foreach (var x in strs) w.WriteStringValue(x); w.WriteEndArray(); }
                if (on.Any(st => st.valueLong != 0)) Range(w, "long", on.Select(st => (double)st.valueLong));
                if (on.Any(st => st.valueDouble != 0)) Range(w, "double", on.Select(st => st.valueDouble));
                if (on.Any(st => st.valueFloat != 0)) Range(w, "float", on.Select(st => (double)st.valueFloat));
                if (on.Any(st => st.valueBool)) w.WriteBoolean("bool", true);
                w.WriteEndObject();
            }
            w.WriteEndObject();

            Range(w, "unit_value", samples.Select(s => (double)s.unitValue));
            if (samples.Any(s => s.unitBaseValue != 0)) Range(w, "unit_base_value", samples.Select(s => (double)s.unitBaseValue));
            if (samples.Any(s => s.lateUnitValue != 0)) Range(w, "late_unit_value", samples.Select(s => (double)s.lateUnitValue));
            Range(w, "base_value", samples.Select(s => Safe(() => (double)s.GetBaseValue())));
            Range(w, "negotiated_value", samples.Select(s => Safe(() => (double)s.GetNegociatedValue())));
            Range(w, "contraband_level", samples.Select(s => Safe(() => (double)ContrabandHelper.GetContrabandLevelInInt(s))));
            if (samples.Any(s => s.unitCount != 1)) Range(w, "unit_count", samples.Select(s => (double)s.unitCount));

            var byFeature = samples.SelectMany(s => s.itemFeatures.Select(f => (s, f))).GroupBy(p => p.f.identifier ?? p.f.category ?? "?").OrderBy(g => g.Key, StringComparer.Ordinal);
            w.WriteStartArray("features");
            foreach (var g in byFeature)
            {
                var fs = g.Select(p => p.f).ToList();
                var f0 = fs[0];
                w.WriteStartObject();
                w.WriteString("id", g.Key);
                if (f0.category != null && f0.category != g.Key) w.WriteString("category", f0.category);
                w.WriteNumber("freq", Math.Round((double)g.Select(p => p.s).Distinct().Count() / samples.Count, 3));
                w.WriteString("stage", f0.GetValueStage().ToString());
                w.WriteString("type", f0.featureType.ToString());
                if (f0.GetToolNeeded() != null) w.WriteString("tool", f0.GetToolNeeded());
                if (fs.Any(f => !f.isExposable)) w.WriteBoolean("not_exposable", true);
                if (fs.Any(f => f.GetPublicHidden())) w.WriteBoolean("public_hidden", true);
                Range(w, "fake_mod", fs.Select(f => (double)Safe(() => f.GetFakeValueModifier())));
                Range(w, "real_mod", fs.Select(f => (double)Safe(() => f.GetActualValueModifier())));
                Range(w, "match", fs.Select(f => f.IsFeatureMatch() ? 1.0 : 0.0));
                var conds = fs.Where(f => f.useCondition).ToList();
                if (conds.Count > 0)
                {
                    Counts(w, "fake_conditions", conds.Select(f => f.fakeCondition?.identifier + ":" + f.fakeCondition?.modValue));
                    Counts(w, "real_conditions", conds.Select(f => f.realCondition?.identifier + ":" + f.realCondition?.modValue));
                }
                w.WriteEndObject();
            }
            w.WriteEndArray();
            w.WriteEndObject();
        }

        static double Safe(Func<double> f)
        {
            try { return f(); } catch { return double.NaN; }
        }

        static void Range(Utf8JsonWriter w, string name, IEnumerable<double> values)
        {
            var v = values.Where(x => !double.IsNaN(x)).ToList();
            if (v.Count == 0) return;
            double min = v.Min(), max = v.Max();
            if (min == max) { w.WriteNumber(name, min); return; }
            w.WriteStartObject(name);
            w.WriteNumber("min", min);
            w.WriteNumber("max", max);
            w.WriteNumber("mean", Math.Round(v.Average(), 3));
            w.WriteEndObject();
        }

        static void Counts(Utf8JsonWriter w, string name, IEnumerable<string> values)
        {
            w.WriteStartObject(name);
            foreach (var g in values.GroupBy(x => x ?? "null").OrderBy(g => g.Key, StringComparer.Ordinal)) w.WriteNumber(g.Key, g.Count());
            w.WriteEndObject();
        }

        // ---- run tables ---------------------------------------------------------------------

        static void Reputation()
        {
            Write("reputation", w =>
            {
                w.WritePropertyName("factions");
                var d = new Dump(w, 4) { Skip = f => typeof(Delegate).IsAssignableFrom(f.FieldType) && Dump.FieldName(f) != "onUnlock" };
                d.Value(PlayerStore.Instance.storeReputations);
            });
        }

        static void Upgrades()
        {
            Write("upgrades", w =>
            {
                w.WritePropertyName("upgrades");
                var ups = PlayerStore.Instance.networkUpgrade;
                w.WriteStartArray();
                foreach (var kv in ups.OrderBy(k => k.Key, StringComparer.Ordinal))
                {
                    w.WriteStartObject();
                    w.WriteString("id", kv.Key);
                    TryWrite(w, "cost", () => kv.Value.GetCost());
                    TryWrite(w, "has_prerequisite_at_start", () => kv.Value.IsHavePrerequisite());
                    w.WritePropertyName("fields");
                    new Dump(w, 3).Object(kv.Value);
                    w.WriteEndObject();
                }
                w.WriteEndArray();
                w.WritePropertyName("unlock_actions");
                new Dump(w, 2).Value(NetworkUpgradeList.UnlockActions.Keys.OrderBy(k => k, StringComparer.Ordinal).ToList());
            });
        }

        static void TryWrite(Utf8JsonWriter w, string name, Func<object> f)
        {
            object v;
            try { v = f(); } catch (Exception e) { w.WriteString(name, "error: " + Game.Root(e)); return; }
            w.WritePropertyName(name);
            new Dump(w, 4).Value(v);
        }

        static void Services()
        {
            Write("services", w =>
            {
                w.WritePropertyName("services");
                new Dump(w, 3).Value(PlayerStore.Instance.storeService);
            });
        }

        static void Difficulties()
        {
            Write("difficulty", w =>
            {
                w.WritePropertyName("difficulties");
                new Dump(w, 4).Value(Difficulty.GetAll());
                w.WritePropertyName("effects");
                new Dump(w, 3).Value(Difficulty.Effects);
                TryWrite(w, "current", () => Difficulty.Current.id);
            });
        }

        static void MarketEvents()
        {
            Write("market_events", w =>
            {
                foreach (var f in typeof(StoreEventManager).GetFields(BindingFlags.Static | BindingFlags.Public | BindingFlags.NonPublic)
                    .Where(f => typeof(IEnumerable).IsAssignableFrom(f.FieldType) && f.FieldType != typeof(string)).OrderBy(f => f.Name, StringComparer.Ordinal))
                {
                    w.WritePropertyName(f.Name);
                    var list = f.GetValue(null) as IEnumerable;
                    w.WriteStartArray();
                    if (list != null)
                        foreach (var bp in list)
                        {
                            w.WriteStartObject();
                            w.WritePropertyName("blueprint");
                            new Dump(w, 2).Value(bp);
                            // Instantiate each blueprint's event once to read its market modifiers.
                            var create = bp?.GetType().GetFields(BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic)
                                .Select(x => x.GetValue(bp)).OfType<Delegate>().FirstOrDefault(d => d.Method.ReturnType == typeof(StoreEvent) && d.Method.GetParameters().Length == 0);
                            if (create != null) TryWrite(w, "event", () => create.DynamicInvoke());
                            w.WriteEndObject();
                        }
                    w.WriteEndArray();
                }
                var factories = typeof(GameItem).Assembly.GetTypes().Where(t => t.Name.StartsWith("EventList"))
                    .SelectMany(t => t.GetMethods(BindingFlags.Static | BindingFlags.Public))
                    .Where(m => m.ReturnType == typeof(StoreEvent) && m.GetParameters().All(p => p.HasDefaultValue))
                    .OrderBy(m => m.DeclaringType.Name + "." + m.Name, StringComparer.Ordinal);
                w.WriteStartArray("event_factories");
                foreach (var m in factories)
                {
                    w.WriteStartObject();
                    w.WriteString("factory", m.DeclaringType.Name + "." + m.Name);
                    TryWrite(w, "event", () => m.Invoke(null, m.GetParameters().Select(p => p.DefaultValue).ToArray()));
                    w.WriteEndObject();
                }
                w.WriteEndArray();
            });
        }

        static void Afterhours()
        {
            Write("afterhours", w =>
            {
                foreach (var t in new[] { typeof(AfterhourEvents) })
                    foreach (var f in t.GetFields(BindingFlags.Static | BindingFlags.Public | BindingFlags.NonPublic).OrderBy(f => f.Name, StringComparer.Ordinal))
                    {
                        if (f.IsLiteral) continue;
                        w.WritePropertyName(f.Name);
                        new Dump(w, 4).Value(f.GetValue(null));
                    }
            });
        }

        static void StartingPerks()
        {
            Write("starting_perks", w =>
            {
                foreach (var f in typeof(StartingPerkList).GetFields(BindingFlags.Static | BindingFlags.Public | BindingFlags.NonPublic).OrderBy(f => f.Name, StringComparer.Ordinal))
                {
                    if (f.IsLiteral) continue;
                    w.WritePropertyName(f.Name);
                    new Dump(w, 3).Value(f.GetValue(null));
                }
                var factories = typeof(StartingPerkList).GetMethods(BindingFlags.Static | BindingFlags.Public)
                    .Where(m => m.ReturnType == typeof(StartingPerk) && m.GetParameters().Length == 0).OrderBy(m => m.Name, StringComparer.Ordinal);
                w.WriteStartArray("perks");
                foreach (var m in factories)
                {
                    w.WriteStartObject();
                    w.WriteString("factory", m.Name);
                    TryWrite(w, "perk", () => m.Invoke(null, null));
                    w.WriteEndObject();
                }
                w.WriteEndArray();
            });
        }

        // Types whose constants and static tables carry economic rules.
        static readonly string[] ConstantTypes =
        {
            "PlayerStore", "StoreClientManager", "StoreClient", "StolenHelper", "ContrabandHelper", "SecData", "StoreReputation",
            "StoreReputationPerk", "BargainUIManager", "NegociationHelper", "StoreService", "HealthData", "ShowcaseHelper",
            "AppraisalHelper", "PriceHelper", "StoreStation", "StoreEventManager", "NetworkUpgrade", "NetworkUpgradeList",
            "Difficulty", "DifficultyRules", "ItemFeatureList", "ItemConditionList", "WaterHelper", "PowerHelper",
            "FoodItemHelper", "SoldItem", "StoreClientList", "StoreClientData", "CommissaryData", "ExchangeManager",
            "AfterhourLocation", "ExpeditionData", "ProcurementData", "GunOrderData", "AppraisalServiceHelper", "UniqueItemHelper",
            "StartingPerk", "RunStats",
        };

        static void Constants()
        {
            Write("constants", w =>
            {
                foreach (var name in ConstantTypes.OrderBy(x => x, StringComparer.Ordinal))
                {
                    var t = typeof(GameItem).Assembly.GetType(name);
                    if (t == null) { report.Add("constants: no type " + name); continue; }
                    w.WriteStartObject(name);
                    foreach (var f in t.GetFields(BindingFlags.Static | BindingFlags.Public | BindingFlags.NonPublic).OrderBy(f => f.Name, StringComparer.Ordinal))
                    {
                        var ft = f.FieldType;
                        bool simple = ft.IsPrimitive || ft == typeof(string) || ft.IsEnum || (ft.IsArray && (ft.GetElementType().IsPrimitive || ft.GetElementType() == typeof(string)));
                        bool table = !simple && f.IsInitOnly && typeof(IEnumerable).IsAssignableFrom(ft) && ft.IsGenericType &&
                                     ft.GetGenericArguments().All(a => a.IsPrimitive || a == typeof(string));
                        if (!simple && !table) continue;
                        if (f.Name.Contains("k__BackingField") || f.Name == "instance" || f.Name == "padlock") continue;
                        object v;
                        try { v = f.GetValue(null); } catch { continue; }
                        w.WritePropertyName(f.Name);
                        new Dump(w, 4).Value(v);
                    }
                    w.WriteEndObject();
                }
            });
        }

        // ---- clients ------------------------------------------------------------------------

        static void ClientPools()
        {
            Write("client_pools", w =>
            {
                w.WriteStartArray("pools");
                foreach (var t in typeof(GameItem).Assembly.GetTypes().OrderBy(t => t.FullName, StringComparer.Ordinal))
                    foreach (var f in t.GetFields(BindingFlags.Static | BindingFlags.Public | BindingFlags.NonPublic).OrderBy(f => f.Name, StringComparer.Ordinal))
                    {
                        if (f.FieldType != typeof(RandomClientPool)) continue;
                        var pool = (RandomClientPool)f.GetValue(null);
                        w.WriteStartObject();
                        w.WriteString("field", t.Name + "." + f.Name);
                        w.WriteString("faction", pool.factionId);
                        if (pool.getChance != null) w.WriteString("chance", Dump.DelegateName(pool.getChance));
                        w.WriteStartArray("entries");
                        foreach (var e in pool.entries)
                        {
                            w.WriteStartObject();
                            w.WriteString("factory", Dump.DelegateName(e.create));
                            w.WriteStartArray("tier_weights");
                            foreach (var x in e.tierWeights) w.WriteNumberValue(x);
                            w.WriteEndArray();
                            w.WriteEndObject();
                        }
                        w.WriteEndArray();
                        w.WriteEndObject();
                    }
                w.WriteEndArray();
                w.WriteStartArray("faction_pools_order");
                foreach (var p in StoreClientList.factionPools) w.WriteStringValue(p.factionId);
                w.WriteEndArray();
            });
        }

        static bool ClientSkip(FieldInfo f)
        {
            var t = f.FieldType;
            string n = Dump.FieldName(f);
            return t == typeof(Dialogue) || n == "possibleSprites" || n == "spriteName" || n == "portrait" ||
                   typeof(UnityEngine.Object).IsAssignableFrom(t);
        }

        // Clients are sampled twice: as on day 1 of a new game, and on day 30 with every story
        // introduction flag set (stolen goods, aug, …), which is when most draws are unlocked.
        static readonly (string name, int day, bool introduced)[] ClientPhases = { ("day1", 1, false), ("day30_introduced", 30, true) };

        static void Clients(Game game)
        {
            var dict = StoreClientListDict.storeClientDict;
            var ps = PlayerStore.Instance;
            var introFlags = typeof(PlayerStore).GetFields(BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic)
                .Where(f => f.FieldType == typeof(bool) && f.Name.StartsWith("is") && f.Name.EndsWith("Introduced")).ToList();
            Write("clients", w =>
            {
                w.WriteNumber("samples_per_client", ClientSamples);
                w.WriteStartObject("phases");
                foreach (var (name, day, intro) in ClientPhases)
                {
                    w.WriteStartObject(name);
                    w.WriteNumber("day", day);
                    if (intro) { w.WriteStartArray("flags_set"); foreach (var f in introFlags) w.WriteStringValue(f.Name); w.WriteEndArray(); }
                    w.WriteEndObject();
                }
                w.WriteEndObject();
                var results = new SortedDictionary<string, List<(string phase, List<StoreClient> s, List<List<GameItem>> stock, string err)>>(StringComparer.Ordinal);
                foreach (var (name, day, intro) in ClientPhases)
                {
                    StoreStation.Instance.dayCounter = day;
                    foreach (var f in introFlags) f.SetValue(ps, intro); // a new game starts with none set
                    foreach (var kv in dict)
                    {
                        var samples = new List<StoreClient>();
                        var stock = new List<List<GameItem>>();
                        string err = null;
                        for (int i = 0; i < ClientSamples; i++)
                        {
                            Headless.Seed(StableSeed(kv.Key) + 1000 * day + i);
                            try
                            {
                                var c = kv.Value();
                                if (c == null) continue;
                                samples.Add(c);
                                stock.Add(RunMainDialogue(c));
                            }
                            catch (Exception e) { err ??= Game.Root(e); }
                        }
                        if (!results.TryGetValue(kv.Key, out var list)) results[kv.Key] = list = new();
                        list.Add((name, samples, stock, err));
                    }
                }
                w.WriteStartArray("clients");
                foreach (var (id, phases) in results)
                {
                    w.WriteStartObject();
                    w.WriteString("id", id);
                    w.WriteString("factory", Dump.DelegateName(dict[id]));
                    var first = phases.SelectMany(p => p.s).FirstOrDefault();
                    if (first != null)
                    {
                        w.WriteString("faction", first.clientFaction);
                        TryWrite(w, "cooldown", () => first.cooldown);
                    }
                    foreach (var (phase, samples, stock, err) in phases)
                    {
                        w.WriteStartObject(phase);
                        if (err != null) w.WriteString("error", err);
                        if (samples.Count > 0) WriteClientSamples(w, samples, stock);
                        w.WriteEndObject();
                    }
                    if (first != null)
                    {
                        w.WritePropertyName("first_sample");
                        new Dump(w, 3) { Skip = ClientSkip }.Object(first);
                    }
                    w.WriteEndObject();
                }
                w.WriteEndArray();
            });
        }

        static void WriteClientSamples(Utf8JsonWriter w, List<StoreClient> samples, List<List<GameItem>> stock)
        {
            Counts(w, "intent", samples.Select(s => s.clientIntent.ToString()));
            if (samples.Any(s => s.useClientBudget)) Range(w, "budget", samples.Where(s => s.useClientBudget).Select(s => (double)s.GetBudget()));
            Counts(w, "buying_tags", samples.SelectMany(s => s.clientBuyingTagList ?? new List<string>()));
            if (samples.Any(s => s.clientBuyingIdList?.Count > 0)) Counts(w, "buying_ids", samples.SelectMany(s => s.clientBuyingIdList));
            if (samples.Any(s => s.clientBlackTagList?.Count > 0)) Counts(w, "black_tags", samples.SelectMany(s => s.clientBlackTagList));
            if (samples.Any(s => s.clientBlackIdList?.Count > 0)) Counts(w, "black_ids", samples.SelectMany(s => s.clientBlackIdList));
            Range(w, "no_contraband", samples.Select(s => s.noAcceptingContraband ? 1.0 : 0.0));
            Range(w, "wholesale", samples.Select(s => s.isOfferWholesale ? 1.0 : 0.0));

            int n = stock.Count(s => s.Count > 0);
            if (n > 0)
            {
                w.WriteNumber("stock_samples", n);
                Range(w, "stock_count", stock.Select(s => (double)s.Count));
                Range(w, "stock_value", stock.Select(s => (double)s.Sum(i => Safe(() => i.GetNegociatedValue()))));
                Counts(w, "stock_items", stock.SelectMany(s => s.Select(ItemKey)));
                var stolen = stock.SelectMany(s => s).Where(StolenHelper.IsStolenItem).ToList();
                Range(w, "stock_stolen_share", stock.SelectMany(s => s).Select(i => StolenHelper.IsStolenItem(i) ? 1.0 : 0.0));
                if (stolen.Count > 0) Range(w, "stock_heat", stolen.Select(i => (double)i.GetTagReadonly("STOLEN_VALUE_INT").GetInt()));
                var feats = stock.SelectMany(s => s).SelectMany(i => i.itemFeatures).ToList();
                if (feats.Count > 0)
                {
                    // Conditions and fakes are applied when stock spawns, not by the item factories.
                    w.WriteStartObject("stock_features");
                    foreach (var g in feats.GroupBy(f => f.identifier ?? f.category ?? "?").OrderBy(g => g.Key, StringComparer.Ordinal))
                    {
                        w.WriteStartObject(g.Key);
                        w.WriteNumber("count", g.Count());
                        int fake = g.Count(f => !f.IsFeatureMatch());
                        if (fake > 0) w.WriteNumber("fake", fake);
                        if (g.Any(f => f.GetToolNeeded() != null)) w.WriteString("tool", g.First(f => f.GetToolNeeded() != null).GetToolNeeded());
                        Range(w, "shown_mod", g.Select(f => (double)Safe(() => f.GetCurrentValueMod())));
                        Range(w, "real_mod", g.Select(f => (double)Safe(() => f.GetActualValueModifier())));
                        w.WriteEndObject();
                    }
                    w.WriteEndObject();
                }
            }
        }

        static string ItemKey(GameItem i) => i.identifier ?? "name:" + i.name;

        // A selling client's stock appears in the end action of their opening dialogue. Play the main
        // dialogue chain's actions without the UI and collect what lands on the counters.
        static List<GameItem> RunMainDialogue(StoreClient c)
        {
            var e = EmporiumEntry.Instance;
            ClearCounters();
            var ps = PlayerStore.Instance;
            c.spriteName ??= ""; // sprite names come from the stubbed SpriteDict
            c.possibleSprites ??= new List<string>();
            var inst = StoreClientInstance.CreateClientInstance(c);
            ps.currentClientInstance = inst;
            try
            {
                int guard = 0;
                for (var d = c.mainDialogue; d != null && guard++ < 64; d = d.nextDialogue)
                {
                    try { d.startAction?.Invoke(); } catch { }
                    try { d.endAction?.Invoke(); } catch { }
                }
                return e.frontInvinvElement.childItems.Concat(e.backInvinvElement.childItems).Where(i => !GeneralHelper.IsItemOwned(i)).ToList();
            }
            finally
            {
                ps.currentClientInstance = null;
                ClearCounters();
            }
        }

        static void ClearCounters()
        {
            var e = EmporiumEntry.Instance;
            foreach (var inv in new GameGridInventory[] { e.frontInvinvElement, e.backInvinvElement })
                foreach (var i in inv.childItems.ToList()) i.Destroy();
        }

        // ---- days ---------------------------------------------------------------------------

        static readonly int[] MixAttractiveness = { 100, 250, 450, 700 };
        static readonly int[] MixReputation = { -50, 0, 30, 60 };
        const int MixDraws = 2000;

        // StoreClientManager.PickClient, the normal-customer draw, sampled over a grid of store
        // attractiveness (cutoffs 400/650 pick the pool tier) and one reputation applied to every
        // faction (it sets the faction chances). Day 30 with every introduction flag set, every
        // client off cooldown.
        static void NormalMix()
        {
            var ps = PlayerStore.Instance;
            StoreStation.Instance.dayCounter = 30;
            foreach (var f in typeof(PlayerStore).GetFields(BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic)
                .Where(f => f.FieldType == typeof(bool) && f.Name.StartsWith("is") && f.Name.EndsWith("Introduced")))
                f.SetValue(ps, true);
            int baseAtt = ps.GetCurrentStoreAttractiveness() - ps.projectorAttractivenessBonus;
            Write("normal_mix", w =>
            {
                w.WriteNumber("draws_per_cell", MixDraws);
                w.WriteNumber("day", 30);
                w.WriteStartArray("cells");
                foreach (int att in MixAttractiveness)
                    foreach (int rep in MixReputation)
                    {
                        ps.projectorAttractivenessBonus = att - baseAtt;
                        foreach (var r in ps.storeReputations) r.SetReputation(rep);
                        var ids = new List<string>();
                        var factions = new List<string>();
                        Headless.Seed(StableSeed("mix") + att * 1000 + rep);
                        for (int i = 0; i < MixDraws; i++)
                        {
                            ps.storeClientManager.clientTracker.Clear(); // the mix without cooldowns; cooldowns are a rule
                            var c = ps.storeClientManager.PickClient();
                            if (c == null) continue;
                            ids.Add(c.identifier);
                            factions.Add(c.clientFaction);
                        }
                        w.WriteStartObject();
                        w.WriteNumber("attractiveness", ps.GetCurrentStoreAttractiveness());
                        w.WriteNumber("reputation_all", rep);
                        TryWrite(w, "normal_customer_count", () => ps.storeClientManager.GetNormalCustomerCount());
                        Counts(w, "factions", factions);
                        Counts(w, "clients", ids);
                        w.WriteEndObject();
                    }
                w.WriteEndArray();
            });
            ps.projectorAttractivenessBonus = 0;
        }

        // How fast the unmodified game code advances idle days headless.
        public static int Bench(int days)
        {
            Game.StartEmporium();
            var ps = PlayerStore.Instance;
            var sw = System.Diagnostics.Stopwatch.StartNew();
            int queued = 0;
            for (int d = 0; d < days; d++)
            {
                ps.OnShutterOpened();
                queued += ps.storeClientManager.clientStack.Count;
                ps.EndDay();
                ps.EndNight();
                ps.BeginDay();
            }
            Console.WriteLine($"{days} days in {sw.ElapsedMilliseconds} ms ({days * 60000.0 / Math.Max(1, sw.ElapsedMilliseconds):0} days/min), {queued} clients generated, day counter {StoreStation.Instance.dayCounter}");
            return 0;
        }

        // The real generator, day after day, for a player who opens, serves no one, and sleeps.
        static void Days()
        {
            var ps = PlayerStore.Instance;
            Write("days", w =>
            {
                w.WriteString("policy", "open shutter, dismiss everyone, close, sleep");
                w.WriteStartArray("days");
                for (int day = 0; day < QueueDays; day++)
                {
                    w.WriteStartObject();
                    w.WriteNumber("day", StoreStation.Instance.dayCounter);
                    w.WriteNumber("cash", ps.playerCash);
                    TryWrite(w, "attractiveness", () => ps.GetCurrentStoreAttractiveness());
                    TryWrite(w, "normal_customer_count", () => ps.storeClientManager.GetNormalCustomerCount());
                    string err = null;
                    try { ps.OnShutterOpened(); } catch (Exception e) { err = "open: " + Game.Root(e); if (day == 0 && Environment.GetEnvironmentVariable("FENCE_TRACE") != null) Console.WriteLine(Game.Inner(e)); }
                    w.WriteStartArray("queue");
                    foreach (var c in ps.storeClientManager.clientStack)
                    {
                        w.WriteStartObject();
                        w.WriteString("id", c.identifier);
                        w.WriteString("faction", c.clientFaction);
                        w.WriteString("intent", c.clientIntent.ToString());
                        if (c.useClientBudget) w.WriteNumber("budget", c.GetBudget());
                        w.WriteEndObject();
                    }
                    w.WriteEndArray();
                    try { ps.EndDay(); } catch (Exception e) { err ??= "close: " + Game.Root(e); }
                    try { ps.EndNight(); } catch (Exception e) { err ??= "night: " + Game.Root(e); }
                    try { ps.BeginDay(); } catch (Exception e) { err ??= "morning: " + Game.Root(e); }
                    if (err != null) w.WriteString("error", err);
                    w.WriteEndObject();
                    var over = typeof(PlayerStore).GetField("__gameOver").GetValue(null) as string;
                    if (over != null) { w.WriteStartObject(); w.WriteString("game_over", over); w.WriteEndObject(); break; }
                }
                w.WriteEndArray();
            });
        }
    }
}
