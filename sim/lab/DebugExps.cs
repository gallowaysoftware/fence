using System;
using System.Linq;

namespace Fence.Lab
{
    // Harness self-checks: how the headless game exposes machine panels.
    public static class DebugExps
    {
        public static void Chem(int seed)
        {
            W.New(seed);
            var jug = W.Own("water_jug");
            WaterHelper.FillWithRustWater(jug);
            var tab = ProductionExps.Owned("water_tablet");
            ChemicalHelper.DissolveInto(tab, jug);
            int Cl() => jug.GetTagReadonly(Liquid.GetCurrentPartFromId("chlorine")).GetInt();
            int Ch() => jug.GetTagReadonly(Liquid.GetCurrentPartFromId("chemical_contaminant")).GetInt();
            Console.WriteLine($"func null? {jug.onOneSecondItemFunc == null}; in GetAllItems {W.E.GetAllItems().Contains(jug)}; chlorine {Cl()} chem {Ch()}");
            W.QuietDay();
            Console.WriteLine($"after QuietDay: chlorine {Cl()} chem {Ch()} day {W.Day}");
            var pit = W.Own("water_pitcher");
            WaterHelper.FillWithRustWater(pit);
            ChemicalHelper.DissolveInto(ProductionExps.Owned("water_tablet"), pit);
            int c0 = pit.GetTagReadonly(Liquid.GetCurrentPartFromId("chlorine")).GetInt();
            W.QuietDay();
            Console.WriteLine($"pitcher func null? {pit.onOneSecondItemFunc == null}; chlorine {c0} -> {pit.GetTagReadonly(Liquid.GetCurrentPartFromId("chlorine")).GetInt()}");
            ChemicalHelper.RunNightlyChemicalSimulation();
            Console.WriteLine($"after RunNightlyChemicalSimulation: chlorine {Cl()} chem {Ch()}");
        }

        public static void Machines(int seed)
        {
            W.New(seed);
            foreach (var id in new[] { "wine_bottle", "hydroponic", "wine_rack", "furnace", "smuggler_bay" })
            {
                var m = W.Own(id);
                var cw = m.contentWindow;
                string s = $"{id}: window={(cw == null ? "null" : cw.GetType().Name)} child={cw?.childElement?.GetType().Name ?? "null"}";
                for (int i = 0; i < 6; i++) { var st = MachineHelper.GetBodyStorage(m, i); s += $" [{i}]={st?.GetType().Name ?? "null"}"; }
                s += " children=" + string.Join(",", GraphUtils.FindChildrenType<GameInventory>(m).Select(x => x.GetType().Name));
                Console.WriteLine(s);
            }
            for (int k = 0; k < 4; k++)
            {
                var bay = W.Own("hydroponic");
                var sd = W.Make("bloomberry_seed");
                GeneralHelper.SetItemOwned(sd, true);
                GraphUtils.TryAcceptAll(MachineHydroponic.GetInputSlot(bay), sd, -1);
                Console.WriteLine($"bay quality tag {bay.GetTagReadonly("TOTAL_PERCENTAGE_QUALITY_BONUS_INT").GetInt()} ratio {bay.GetTagReadonly("MACHINE_QUALITY_RATIO_INT").GetInt()} adj {ModuleHelper.GetAjustedQuality(bay)} features [{string.Join(",", bay.itemFeatures.Select(f => f.identifier))}] base harvest {HydroponicHelper.GetBaseHarvestQuantity(sd)} genetics HQ x{PlantGeneticsHelper.GetStatMultiplier(sd, "HARVEST_QUANTITY")} growth x{PlantGeneticsHelper.GetStatMultiplier(sd, "GROWTH_SPEED")} health {HydroponicHelper.GetStartingHealthPercent(sd)}");
            }
        }
    }
}
