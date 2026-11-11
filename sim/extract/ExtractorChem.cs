using System;
using System.Collections.Generic;
using System.Linq;
using System.Reflection;
using System.Text.Json;

using Fence.Sim;

namespace Fence.Extract
{
    // chemistry.json and recipes.json: the liquid table, finisher recipes, the composition of every
    // chemical item and supply bottle, module effects and heating curves measured on the game's own
    // machines, and every recipe encoded on items (printer chips, seeds, keycards, crafting tables).
    static partial class Extractor
    {
        public static int ChemOnly(Game game, string dir, BuildInfo b)
        {
            build = b;
            outDir = dir;
            System.IO.Directory.CreateDirectory(dir);
            Step("new game", Game.StartEmporium);
            Step("chemistry", () => { PlayerStore.Instance.StartNewGame(); Chemistry(game); });
            Step("recipes", () => { PlayerStore.Instance.StartNewGame(); Recipes(game); });
            foreach (var r in report) Console.WriteLine("problem: " + r);
            return 0;
        }

        static readonly string[] ChemConstantTypes =
        {
            "ChemicalHelper", "ChemicalProductHelper", "ChemicalFeedbackHelper", "ChemFinisherRecipe", "MachineHeatingPlate",
            "MedsItemDirectory", "MachineChemFinisher",
        };

        static void Chemistry(Game game)
        {
            Write("chemistry", w =>
            {
                w.WritePropertyName("liquids");
                new Dump(w, 2).Value(Liquid.Liquids);

                w.WritePropertyName("finisher_recipes");
                new Dump(w, 2).Value(ChemFinisherRecipe.chemFinisherRecipes);

                w.WriteStartArray("purity_grades");
                foreach (var m in typeof(ItemConditionList).GetMethods(BindingFlags.Public | BindingFlags.Static)
                             .Where(m => m.GetParameters().Length == 0 && m.ReturnType == typeof(ItemCondition)))
                {
                    ItemCondition c;
                    try { c = (ItemCondition)m.Invoke(null, null); } catch { continue; }
                    if (c?.category != "CATEGORY_CHEMICAL_PURITY") continue;
                    w.WriteStartObject();
                    w.WriteString("id", c.identifier);
                    w.WriteNumber("rank", ChemicalProductHelper.GetGradeNumber(c.identifier));
                    w.WriteNumber("mod_value_pct", c.modValue);
                    w.WriteEndObject();
                }
                w.WriteEndArray();

                w.WriteStartObject("constants");
                foreach (var name in ChemConstantTypes)
                {
                    var t = typeof(GameItem).Assembly.GetType(name);
                    if (t == null) { report.Add("chemistry: no type " + name); continue; }
                    w.WriteStartObject(name);
                    foreach (var f in t.GetFields(BindingFlags.Static | BindingFlags.Public | BindingFlags.NonPublic).OrderBy(f => f.Name, StringComparer.Ordinal))
                    {
                        var ft = f.FieldType;
                        if (!(ft.IsPrimitive || ft == typeof(string)) || f.Name.Contains("k__BackingField")) continue;
                        object v;
                        try { v = f.GetValue(null); } catch { continue; }
                        w.WritePropertyName(f.Name);
                        new Dump(w, 1).Value(v);
                    }
                    w.WriteEndObject();
                }
                w.WriteEndObject();

                // The reaction handlers in the order HandleTemperatureReaction sums them (declaration order).
                w.WriteStartArray("reaction_handlers");
                foreach (var m in typeof(ChemicalHelper).GetMethods(BindingFlags.Public | BindingFlags.Static)
                             .Where(m => m.Name.StartsWith("Handle") && m.ReturnType == typeof(int) && m.GetParameters().Length == 1))
                    w.WriteStringValue(m.Name);
                w.WriteEndArray();

                w.WriteStartArray("chem_items");
                foreach (var id in game.ItemDirectoryOf.Keys)
                {
                    GameItem it;
                    try { it = game.CreateItem(id); } catch { continue; }
                    bool chem = it.IsTag("CHEM_ITEM_TAG") || it.IsTag("CHEMICAL_PRODUCT_TAG");
                    bool filled = it.IsTag("LIQUID_CONTAINER_TAG") && !WaterHelper.IsEmpty(it);
                    if (!chem && !filled && !it.IsTag("MODULE_TYPE_CHEM")) continue;
                    WriteChemItem(w, id, it, game);
                }
                w.WriteEndArray();

                w.WriteStartArray("supply_bottles");
                foreach (var m in typeof(PreBuildChemHelper).GetMethods(BindingFlags.Public | BindingFlags.Static)
                             .Where(m => m.ReturnType == typeof(GameItem) && m.GetParameters().Length == 0))
                {
                    var samples = new List<GameItem>();
                    for (int i = 0; i < ItemSamples; i++)
                    {
                        Headless.Seed(StableSeed(m.Name) + i);
                        try { samples.Add((GameItem)m.Invoke(null, null)); } catch (Exception e) { report.Add("supply " + m.Name + ": " + Game.Inner(e)); break; }
                    }
                    if (samples.Count == 0) continue;
                    w.WriteStartObject();
                    w.WriteString("builder", "PreBuildChemHelper." + m.Name);
                    w.WriteString("container", samples[0].identifier);
                    w.WriteNumber("capacity_ul", WaterHelper.GetTotalCapacity(samples[0]));
                    Composition(w, samples);
                    w.WriteEndObject();
                }
                w.WriteEndArray();

                w.WriteStartObject("unpacks");
                foreach (var id in game.ItemDirectoryOf.Keys)
                {
                    GameItem it;
                    try { it = game.CreateItem(id); } catch { continue; }
                    if (!it.GetGameItemType().Contains("CHEMICAL_SUPPLIES") || it.onActivateSlotItemFunc == null) continue;
                    w.WriteStartArray(id);
                    foreach (var got in Unpack(it)) w.WriteStringValue(got);
                    w.WriteEndArray();
                }
                w.WriteEndObject();

                w.WriteStartArray("module_effects");
                for (int n = 0; n <= 5; n++)
                {
                    w.WriteStartObject();
                    w.WriteNumber("chem_modules", n);
                    TryNumber(w, "blender_loss_pct", () => MachineBlender.GetLoss(WithChemModules("blender", n)));
                    TryNumber(w, "finisher_impurity_pct", () => MachineChemFinisher.GetLoss(WithChemModules("chem_finisher", n)));
                    TryNumber(w, "heating_plate_fixed_target_c", () =>
                    {
                        var p = WithChemModules("heating_plate", n);
                        p.onOneSecondItemFunc(p, null, null);
                        return MachineBlender.GetUpgradeCount(p) >= 2 ? -1 : p.GetTagReadonly("CURRENT_TARGET_TEMP").GetInt();
                    });
                    w.WriteEndObject();
                }
                w.WriteEndArray();

                w.WriteStartArray("heating_curves");
                foreach (int modules in new[] { 0, 1, 2, 3, 4 })
                    foreach (int mode in new[] { 1, 2 })
                        foreach (var (container, ml) in new[] { ("med_bottle_red", 250), ("water_pitcher", 1000), ("water_pitcher", 3000) })
                            HeatingCurve(w, modules, mode, container, ml);
                w.WriteEndArray();

                w.WriteStartArray("cooling_curves");
                foreach (var (container, ml) in new[] { ("med_bottle_red", 250), ("water_pitcher", 1000), ("water_pitcher", 3000) })
                    CoolingCurve(w, container, ml);
                w.WriteEndArray();
            });
        }

        static void TryNumber(Utf8JsonWriter w, string name, Func<double> f)
        {
            try { w.WriteNumber(name, f()); } catch (Exception e) { w.WriteString(name, "error: " + Game.Root(e)); }
        }

        static GameItem WithChemModules(string machineId, int n)
        {
            var m = DirectoryMaster.Item(machineId);
            GeneralHelper.SetItemOwned(m, true);
            var grid = (GraphNodeStorage)MachineHelper.GetBodyStorage(m, 0);
            for (int i = 0; i < n; i++)
            {
                var mod = DirectoryMaster.Item("chem_module");
                GeneralHelper.SetItemOwned(mod, true);
                if (GraphUtils.TryAcceptAll(grid, mod, -1) != 0) throw new InvalidOperationException($"{machineId}: module {i + 1} did not fit");
            }
            return m;
        }

        static void Composition(Utf8JsonWriter w, List<GameItem> samples)
        {
            w.WriteStartObject("composition_ul");
            foreach (var l in Liquid.Liquids)
            {
                var v = samples.Select(s => (double)s.GetTagReadonly(l.currentPart).GetInt()).ToList();
                if (v.All(x => x == 0)) continue;
                Range(w, l.identifier, v);
            }
            w.WriteEndObject();
        }

        static void WriteChemItem(Utf8JsonWriter w, string id, GameItem first, Game game)
        {
            var samples = new List<GameItem> { first };
            for (int i = 1; i < ItemSamples; i++)
            {
                Headless.Seed(StableSeed(id) + i);
                try { samples.Add(game.CreateItem(id)); } catch { }
            }
            w.WriteStartObject();
            w.WriteString("id", id);
            w.WriteStartArray("types");
            foreach (var t in first.GetGameItemType().OrderBy(x => x, StringComparer.Ordinal)) w.WriteStringValue(t);
            w.WriteEndArray();
            w.WriteNumber("unit_value", first.unitValue);
            Range(w, "contraband_level", samples.Select(s => Safe(() => ContrabandHelper.GetContrabandLevelInInt(s))));
            if (first.IsTag("BLENDABLE_TAG")) w.WriteBoolean("blendable", true);
            if (first.IsTag("SOLUBLE_TAG")) w.WriteBoolean("soluble", true);
            if (first.IsTag("MODULE_TYPE_CHEM")) w.WriteBoolean("chem_module", true);
            if (first.IsTag("LIQUID_CONTAINER_TAG")) w.WriteNumber("capacity_ul", WaterHelper.GetTotalCapacity(first));
            if (first.IsTag("CHEMICAL_PRODUCT_TAG"))
            {
                w.WriteString("active_ingredient", first.GetTagReadonly("ACTIVATE_INGREDIENT_TAG").GetString());
                Range(w, "purity_pct", samples.Select(s => Math.Round(ChemicalProductHelper.GetPurity(s), 2)));
                Counts(w, "grades", samples.Select(ChemicalProductHelper.GetCurrentChemicalConditionId));
            }
            if (first.IsTag("CHEMICAL_INTENT_TAG"))
            {
                w.WriteString("intent", ChemicalFeedbackHelper.GetChemicalIntent(first).ToString());
                Range(w, "euphoria_level", samples.Select(s => (double)ChemicalFeedbackHelper.GetEuphoriaLevel(s)));
                Range(w, "toxic", samples.Select(s => ChemicalFeedbackHelper.IsToxic(s) ? 1.0 : 0.0));
                Range(w, "lethal", samples.Select(s => ChemicalFeedbackHelper.IsLethal(s) ? 1.0 : 0.0));
                Range(w, "feedback_level", samples.Select(s => Safe(() => ChemicalFeedbackHelper.GetItemFeedbackLevel(s))));
            }
            Composition(w, samples);
            w.WriteEndObject();
        }

        static List<string> Unpack(GameItem pack)
        {
            if (PlayerStore.Instance.networkUpgrade.TryGetValue("RETIRED_CHEMIST", out var chemist)) chemist.state = 2;
            var inv = new GameGridInventory(12, 12);
            GeneralHelper.SetItemOwned(pack, true);
            if (GraphUtils.TryAcceptAll(inv, pack, -1) != 0) return new List<string> { "error: pack did not fit" };
            try { pack.onActivateSlotItemFunc(pack, inv, null); }
            catch (Exception e) { return new List<string> { "error: " + Game.Root(e) }; }
            return inv.childItems.Where(i => i != pack).Select(i =>
            {
                var parts = Liquid.Liquids.Where(l => i.GetTagReadonly(l.currentPart).GetInt() > 0)
                    .Select(l => $"{l.identifier}:{i.GetTagReadonly(l.currentPart).GetInt()}");
                return i.identifier + (i.IsTag("LIQUID_CONTAINER_TAG") ? " {" + string.Join(",", parts) + "}" : "");
            }).ToList();
        }

        static readonly int[] Marks = { 40, 45, 50, 55, 60, 70, 80, 99, 100, 110, 120, 125, 130, 155 };

        // Seconds for a container of plain water on a plate to reach each mark: the plate's tick, then the
        // container's own (cooling + reactions), as the game interleaves them each second.
        static void HeatingCurve(Utf8JsonWriter w, int modules, int mode, string containerId, int ml)
        {
            w.WriteStartObject();
            w.WriteNumber("chem_modules", modules);
            w.WriteString("mode", mode == 1 ? "low" : "high");
            w.WriteString("container", containerId);
            w.WriteNumber("water_ml", ml);
            try
            {
                var plate = WithChemModules("heating_plate", modules);
                plate.onOneSecondItemFunc(plate, null, null);
                if (modules >= 2) plate.ModifyTag("CURRENT_TARGET_TEMP", s => s.Enable().SetInt(160));
                plate.ModifyTag("HEATING_PLATE_STATE", s => s.Enable().SetInt(mode));
                var c = DirectoryMaster.Item(containerId);
                GeneralHelper.SetItemOwned(c, true);
                WaterHelper.AddPureWater(c, ml * 1000);
                ChemicalHelper.InitContainerTempFunction(c);
                c.ModifyTag("CONTAINER_TEMP_TAG", s => s.Enable().SetDouble(26.0));
                var slot = (GraphNodeStorage)MachineHelper.GetBodyStorage(plate, 1);
                if (GraphUtils.TryAcceptAll(slot, c, -1) != 0) throw new InvalidOperationException("container did not fit");
                w.WriteNumber("target_c", plate.GetTagReadonly("CURRENT_TARGET_TEMP").GetInt());
                w.WriteStartObject("seconds_to");
                int mark = 0;
                double max = 0;
                for (int sec = 1; sec <= 1800 && mark < Marks.Length; sec++)
                {
                    plate.onOneSecondItemFunc(plate, null, null);
                    c.onOneSecondItemFunc?.Invoke(c, null, null);
                    double t = c.GetTagReadonly("CONTAINER_TEMP_TAG").GetDouble();
                    max = Math.Max(max, t);
                    while (mark < Marks.Length && t >= Marks[mark]) w.WriteNumber(Marks[mark++].ToString(), sec);
                }
                w.WriteEndObject();
                w.WriteNumber("max_c_in_1800s", Math.Round(max, 2));
                w.WriteNumber("water_ml_left", WaterHelper.GetTotalVolume(c) / 1000.0);
            }
            catch (Exception e) { w.WriteString("error", Game.Root(e)); }
            w.WriteEndObject();
        }

        static void CoolingCurve(Utf8JsonWriter w, string containerId, int ml)
        {
            w.WriteStartObject();
            w.WriteString("container", containerId);
            w.WriteNumber("water_ml", ml);
            try
            {
                var c = DirectoryMaster.Item(containerId);
                WaterHelper.AddPureWater(c, ml * 1000);
                ChemicalHelper.InitContainerTempFunction(c);
                c.ModifyTag("CONTAINER_TEMP_TAG", s => s.Enable().SetDouble(99.0));
                w.WriteStartObject("temp_c_after_seconds");
                for (int sec = 1; sec <= 600; sec++)
                {
                    c.onOneSecondItemFunc(c, null, null);
                    if (sec is 10 or 30 or 60 or 120 or 180 or 300 or 360 or 600)
                        w.WriteNumber(sec.ToString(), Math.Round(c.GetTagReadonly("CONTAINER_TEMP_TAG").GetDouble(), 2));
                }
                w.WriteEndObject();
            }
            catch (Exception e) { w.WriteString("error", Game.Root(e)); }
            w.WriteEndObject();
        }

        // ---- recipes ------------------------------------------------------------------------

        static void Recipes(Game game)
        {
            Write("recipes", w =>
            {
                var built = new List<(string id, GameItem it)>();
                foreach (var id in game.ItemDirectoryOf.Keys)
                {
                    try { built.Add((id, game.CreateItem(id))); } catch { }
                }

                // Progress machines (3D printer, desequencer, DNA manipulator…) read their recipe from the source item.
                w.WriteStartArray("progress_recipes");
                foreach (var (id, it) in built.Where(b => b.it.IsTag("PROGRESS_ITEM_TAG")))
                {
                    w.WriteStartObject();
                    w.WriteString("source", id);
                    w.WriteNumber("source_value", it.unitValue);
                    w.WriteNumber("work", it.GetTagReadonly("PROGRESS_ITEM_TARGET_TAG").GetInt());
                    string target = it.GetTagReadonly("PROGRESS_ITEM_TARGET_ITEM_ID_TAG").GetString();
                    if (!string.IsNullOrEmpty(target))
                    {
                        w.WriteString("output", target);
                        w.WriteNumber("output_count", it.GetTagReadonly("PROGRESS_ITEM_TARGET_ITEM_COUNT_TAG").GetInt());
                        TryNumber(w, "output_value", () => DirectoryMaster.Item(target).unitValue);
                    }
                    var machines = new List<string>();
                    string single = it.GetTagReadonly("PROGRESS_ITEM_REQUIRED_MACHINE_TAG").GetString();
                    if (!string.IsNullOrEmpty(single)) machines.Add(single);
                    int mc = it.GetTagReadonly("PROGRESS_ITEM_REQUIRED_MACHINE_TAG_COUNT").GetInt();
                    for (int i = 0; i < mc; i++) machines.Add(it.GetTagReadonly("PROGRESS_ITEM_REQUIRED_MACHINE_TAG_" + i).GetString());
                    if (machines.Count > 0) { w.WriteStartArray("machine_tags"); foreach (var m in machines) w.WriteStringValue(m); w.WriteEndArray(); }
                    w.WriteStartArray("consumes");
                    int rc = it.IsTag("PROGRESS_ITEM_USE_ADDITIONAL_ITEMS") ? it.GetTagReadonly("REQUIRED_ITEM_TYPE_COUNT_TAG").GetInt() : 0;
                    for (int i = 0; i < rc; i++)
                    {
                        w.WriteStartObject();
                        string rid = it.GetTagReadonly("REQUIRED_ITEM_ID_" + i).GetString();
                        w.WriteString("id", rid);
                        w.WriteNumber("count", it.GetTagReadonly("REQUIRED_ITEM_COUNT_" + i).GetInt());
                        TryNumber(w, "value", () => DirectoryMaster.Item(rid).unitValue);
                        w.WriteEndObject();
                    }
                    w.WriteEndArray();
                    w.WriteEndObject();
                }
                w.WriteEndArray();

                w.WriteStartArray("machines");
                foreach (var (id, it) in built.Where(b => b.it.GetGameItemType().Contains("MACHINE")))
                {
                    w.WriteStartObject();
                    w.WriteString("id", id);
                    w.WriteNumber("unit_value", it.unitValue);
                    TryNumber(w, "power_per_cycle", () => MachineryHelper.GetMachinePowerUsage(it));
                    foreach (var tag in new[] { "BASE_PROCESSING_SPEED", "CURRENT_PROCESSING_SPEED_TAG", "MACHINE_PERFORMANCE_RATIO_INT", "MACHINE_QUALITY_RATIO_INT" })
                        if (it.IsTag(tag)) w.WriteNumber(tag, it.GetTagReadonly(tag).GetInt());
                    w.WriteBoolean("nightly", it.onCycleEndSlotItemFunc != null || it.onCycleEndEarlySlotItemFunc != null || it.onCycleEndLateSlotItemFunc != null);
                    w.WriteBoolean("per_second", it.onOneSecondItemFunc != null);
                    if (it.IsTag("STANDARD_MACHINE_TAG") || it.IsTag("TURBO_BOOST_ELIGIBLE_TAG")) w.WriteBoolean("turbo_boostable", true);
                    w.WriteEndObject();
                }
                w.WriteEndArray();

                w.WriteStartArray("finisher_recipes");
                foreach (var r in ChemFinisherRecipe.chemFinisherRecipes)
                {
                    w.WriteStartObject();
                    w.WriteString("output", r.identifier);
                    w.WriteString("active_ingredient", r.activeIngredientId);
                    w.WriteNumber("draw_ul", r.requiredVolume);
                    w.WriteBoolean("requires_dry", r.isRequireDry);
                    TryNumber(w, "output_value", () => DirectoryMaster.Item(r.identifier).unitValue);
                    w.WriteEndObject();
                }
                w.WriteEndArray();

                // Crafting/vending tables: recipes live on the item's recipeManager.
                w.WriteStartArray("crafting_recipes");
                foreach (var (id, it) in built.Where(b => b.it.recipeManager != null))
                {
                    foreach (var r in it.recipeManager.availableRecipes)
                    {
                        w.WriteStartObject();
                        w.WriteString("station", id);
                        if (r.customName != null) w.WriteString("name", r.customName);
                        Filters(w, "tools", r.requiredTools);
                        Filters(w, "materials", r.requiredMaterials);
                        w.WriteStartArray("outputs");
                        try
                        {
                            foreach (var o in r.CraftWithoutMaterials())
                            {
                                w.WriteStartObject();
                                w.WriteString("id", o.identifier);
                                w.WriteNumber("count", o.unitCount);
                                w.WriteNumber("value", o.unitValue);
                                w.WriteEndObject();
                            }
                        }
                        catch (Exception e) { w.WriteStringValue("error: " + Game.Root(e)); }
                        w.WriteEndArray();
                        if (r.remainingCraftAmount >= 0) w.WriteNumber("stock", r.remainingCraftAmount);
                        w.WriteEndObject();
                    }
                }
                w.WriteEndArray();
            });
        }

        static void Filters(Utf8JsonWriter w, string name, List<CraftingFilter> filters)
        {
            w.WriteStartArray(name);
            foreach (var f in filters)
            {
                w.WriteStartObject();
                w.WriteNumber("amount", f.amount);
                if (f is CraftingIdentifierFilter idf) { w.WriteStartArray("ids"); foreach (var x in idf.identifierFilter.OrderBy(x => x, StringComparer.Ordinal)) w.WriteStringValue(x); w.WriteEndArray(); }
                else if (f is CraftingTagsFilter tf) { w.WriteStartArray("tags"); foreach (var x in tf.tagFilter.OrderBy(x => x, StringComparer.Ordinal)) w.WriteStringValue(x); w.WriteEndArray(); }
                else w.WriteString("kind", f.GetType().Name);
                w.WriteEndObject();
            }
            w.WriteEndArray();
        }
    }
}
