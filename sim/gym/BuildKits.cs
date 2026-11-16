using System;
using System.Collections.Generic;
using System.Linq;
using Newtonsoft.Json.Linq;

namespace Fence.Gym
{
    // Each engine's next purchase for the build planner (BuildOrder.cs), as a whole kit: what is missing today, what the
    // engine earns a day once it is all owned (`build.income` / `build.ramp` override the paper defaults, which come from
    // docs/research: CHEMISTRY-ENGINE.md, BREEDING.md, QUALITY.md §8, SECOND-ENGINE.md), and what it needs first.
    // Read-only: an adapter looks at what the shop owns and never moves anything.
    internal static class BuildKits
    {
        // Every id a kit may ever list: the planner answers the engines' gates for these ids only.
        static readonly Dictionary<string, string[]> Ids = new Dictionary<string, string[]>(StringComparer.Ordinal)
        {
            ["cellar_bays"] = new[] { "hydroponic" },
            ["crop_bays"] = new[] { "hydroponic", "WATER_TRADER", "GUTTERFLOW_TAP" },
            ["water"] = new[] { "hydroponic", "WATER_TRADER", "GUTTERFLOW_TAP", "RUSTWATER_TAP", "RETIRED_FARMER", "water_tank" },
            ["lab"] = new[] { "RETIRED_CHEMIST", "blender", "chem_finisher", "heating_plate", "evaporator" },
            ["dna"] = new[] { "PLANT_DNA", "plant_dna_manipulator", "plant_dna_disk", "hydroreed_seed" },
            ["power"] = new[] { "POWER_HIJACK", "recharger" },
            ["storage"] = new[] { "storage_bay" },
            // The heavy chip waits on Permit II: the arms engine buys it by its own rule, outside the kit.
            ["forge"] = new[] { "printer", "printer_module_metal", "printer_module_cnc", Arms.HandmadeChip, Arms.PartChip },
        };

        // What the route's own core (the wine cellar) still buys of an id a kit also lists, by the engines' old rules.
        public static int Core(string id, int engineWants)
        {
            // A second recharger or COMMERCIAL_POWER: Power.cs's own rule once the first is in.
            if (id == "recharger") return Have("recharger") > 0 ? engineWants : 0;
            if (!Production.WineOn || BuildOrder.KitOn("cellar_bays") && Listed("cellar_bays")) return 0;
            switch (id)
            {
                case "hydroponic": return Production.CoreBayWants();
                case "WATER_TRADER":
                case "RETIRED_FARMER":
                    return engineWants;
            }
            return 0;
        }

        static bool Listed(string kit) => (Directives.Current.Raw["build"]?["kits"] as JArray)?.Any(x => (string)x == kit) ?? false;

        public static IEnumerable<string> PartIds(string kit) => Ids.TryGetValue(kit, out var a) ? a : new string[0];

        // An upgrade a kit lists whose own prerequisite is in the same kit is buyable once that one is bought.
        static readonly Dictionary<string, string> After = new Dictionary<string, string>(StringComparer.Ordinal)
        {
            ["GUTTERFLOW_TAP"] = "WATER_TRADER", ["RUSTWATER_TAP"] = "WATER_TRADER", ["PLANT_DNA"] = "RETIRED_FARMER",
        };

        static IEnumerable<GameItem> Owned() => EmporiumEntry.Instance.GetAllOwnedItems(false, true).Where(i => i != null);
        static int Have(string id) => Owned().Count(i => i.identifier == id);
        static void Wild(BuildKit k, string id)
        {
            if (!PlayerStore.Instance.networkUpgrade.TryGetValue(id, out var u) || u.IsUnlocked()) return;
            k.Part(id, 1, u.GetCost(), BuyAt.Wild);
            if (!u.IsReady() && !(After.TryGetValue(id, out var pre) && k.Missing.Any(p => p.Id == pre))) k.Blocked = k.Blocked ?? id;
        }

        static void Item(BuildKit k, string id, int count, BuyAt at)
        {
            if (count <= 0) return;
            k.Part(id, count, BuildOrder.Price(id), at);
            // Bays come from the Retired Farmer: until he is unlocked (by the kit or the cellar) none can be bought.
            if (id == "hydroponic" && !NetworkUpgrade.IsRetiredFarmerUnlocked() && !k.Missing.Any(p => p.Id == "RETIRED_FARMER")) k.Blocked = k.Blocked ?? "RETIRED_FARMER";
        }

        public static List<BuildKit> All()
        {
            var kits = new List<BuildKit>();
            var bays = Production.BayCounts();
            int owned = bays.owned;
            // The shop's bays go to the cellar first, then the crops, then the reeds: each kit asks for its own shortfall.
            int Take(int want) { int got = Math.Min(owned, want); owned -= got; return want - got; }

            // The route's own cellar bays stay the cellar's (its old rules) unless `kits` names them.
            if (Production.WineOn && Listed("cellar_bays"))
            {
                var k = new BuildKit { Name = "cellar_bays", RampDays = BuildOrder.Ramp("cellar_bays", 21), DrawPerNight = 8 * bays.cellar };
                int short_ = Take(bays.cellar);
                Item(k, "hydroponic", short_, BuyAt.Seller);
                k.IncomePerDay = BuildOrder.Income("cellar_bays", 20) * short_;
                kits.Add(k);
            }
            else Take(bays.cellar);

            if (Crops.On && BuildOrder.KitOn("crop_bays"))
            {
                var k = new BuildKit { Name = "crop_bays", RampDays = BuildOrder.Ramp("crop_bays", 4), DrawPerNight = 8 * bays.crops };
                int short_ = Take(bays.crops);
                if (Directives.Current.Raw.Value<bool?>("crops_tap") ?? true)
                {
                    Wild(k, "WATER_TRADER");
                    Wild(k, "GUTTERFLOW_TAP");
                }
                Item(k, "hydroponic", short_, BuyAt.Seller);
                // The taps water every crop bay: their income counts the whole crop once they come.
                k.IncomePerDay = BuildOrder.Income("crop_bays", 30) * (k.Missing.Any(p => p.At == BuyAt.Wild) ? bays.crops : short_);
                kits.Add(k);
            }

            if (Production.CleanBuild is var cw && cw.reeds > 0 && BuildOrder.KitOn("water"))
            {
                var k = new BuildKit { Name = "water", RampDays = BuildOrder.Ramp("water", 15), DrawPerNight = 8 * cw.reeds, IncomePerDay = BuildOrder.Income("water", 50) };
                if (!Production.WineOn) Wild(k, "RETIRED_FARMER");
                if (!cw.noTap)
                {
                    Wild(k, "WATER_TRADER");
                    Wild(k, "GUTTERFLOW_TAP");
                    if (cw.rust) Wild(k, "RUSTWATER_TAP");
                }
                Item(k, "hydroponic", Take(cw.reeds), BuyAt.Seller);
                if (!cw.noTap) Item(k, "water_tank", Math.Max(0, cw.reeds - Have("water_tank")), BuyAt.Jackson);
                if (k.Missing.Count == 0) k.IncomePerDay = 0;
                kits.Add(k);
            }

            if (Lab.On && BuildOrder.KitOn("lab"))
            {
                var k = new BuildKit { Name = "lab", RampDays = BuildOrder.Ramp("lab", 7), DrawPerNight = 6, IncomePerDay = BuildOrder.Income("lab", 45), Cells = 16 };
                Wild(k, "RETIRED_CHEMIST");
                foreach (var m in new[] { "blender", "chem_finisher", "heating_plate", "evaporator" })
                    Item(k, m, Have(m) == 0 ? 1 : 0, BuyAt.Seller);
                kits.Add(k);
            }

            if (Production.DnaOn && BuildOrder.KitOn("dna"))
            {
                var g = Production.BreedPlan;
                // Paper (BREEDING.md §2): caffeine weed ~67 a bay-night at caffeine III + potent I, pharma, less power; the
                // bloom line +0.42 wine points a berry; `wine_dna` one hardy rank (measured null).
                double perDay = g.weedBays > 0 ? 40.0 * g.weedBays : g.bloom ? 15 : 25;
                var k = new BuildKit { Name = "dna", RampDays = BuildOrder.Ramp("dna", g.weedBays > 0 ? 25 : 20), DrawPerNight = 12 * g.manipulators, IncomePerDay = BuildOrder.Income("dna", perDay), Cells = 6 };
                Wild(k, "PLANT_DNA");
                Item(k, "plant_dna_manipulator", Math.Max(0, g.manipulators - Have("plant_dna_manipulator")), BuyAt.Jackson);
                Item(k, "plant_dna_disk", Have("plant_dna_disk") == 0 ? 2 : 0, BuyAt.Jackson);
                // Purifying comes only from the hydroreed, which sellers rarely bring: one up front.
                if (g.bloom) Item(k, "hydroreed_seed", Have("hydroreed_seed") == 0 ? 1 : 0, BuyAt.Seller);
                // The line's press needs the lab's blender and finisher, its weed needs the crop bays.
                var needs = new List<string> { "cellar_bays" };
                if (g.press) needs.Add("lab");
                if (g.weedBays > 0) needs.Add("crop_bays");
                k.Needs = needs.ToArray();
                kits.Add(k);
            }

            if (Arms.Forge && BuildOrder.KitOn("forge"))
            {
                var arms = Directives.Current.Raw["arms"] as JObject;
                int printers = Math.Max(1, arms?.Value<int?>("forge_printers") ?? 1);
                var k = new BuildKit { Name = "forge", RampDays = BuildOrder.Ramp("forge", 10), DrawPerNight = 14 * printers, Cells = 12 };
                Item(k, "printer", Math.Max(0, printers - Printer.Printers().Count(Arms.Forgeable)), BuyAt.Jackson);
                Item(k, "printer_module_metal", Math.Max(0, printers - Have("printer_module_metal")), BuyAt.Jackson);
                Item(k, "printer_module_cnc", Math.Max(0, printers - Have("printer_module_cnc")), BuyAt.Jackson);
                Item(k, Arms.PartChip, Have(Arms.PartChip) == 0 ? 1 : 0, BuyAt.Jackson);
                if ((arms?.Value<int?>("keep_handmade") ?? 3) > 0) Item(k, Arms.HandmadeChip, Have(Arms.HandmadeChip) == 0 ? 1 : 0, BuyAt.Jackson);
                // A Mid order gun is ~33 a printer-night (docs/re/ARMS.md §6.1).
                k.IncomePerDay = BuildOrder.Income("forge", 33) * printers;
                kits.Add(k);
            }

            // Free power (Power.cs: POWER_HIJACK and a recharger) before the kits that draw; it earns what their cells
            // would cost (1.5 a unit). Cells and COMMERCIAL_POWER stay Power.cs's own.
            if (Power.On && BuildOrder.KitOn("power"))
            {
                var k = new BuildKit { Name = "power", RampDays = 1 };
                Wild(k, "POWER_HIJACK");
                Item(k, "recharger", Have("recharger") == 0 ? 1 : 0, BuyAt.Jackson);
                int draw = kits.Sum(x => x.DrawPerNight);
                k.IncomePerDay = BuildOrder.Income("power", 1.5) * Math.Min(draw, 24);
                kits.Add(k);
                if (k.Missing.Count > 0)
                    foreach (var x in kits.Where(x => x != k && x.DrawPerNight > 0))
                        x.Needs = x.Needs.Concat(new[] { "power" }).ToArray();
            }

            // A storage bay before any kit whose loose parts the stock grid cannot take, and on its own whenever the grid
            // runs short (the engines' machines and kit fill it, and a lot the grid cannot take is a lot the counter
            // turns away: free cells fell to 0-8 from day ~50 with the weed line, ~90 on `sprinter`).
            if (BuildOrder.StorageOn && BuildOrder.KitOn("storage"))
            {
                var k = new BuildKit { Name = "storage", RampDays = 0, Urgent = PlayerStock.FreeCells() < (Directives.Current.Raw.Value<int?>("min_free_cells") ?? 36) };
                int need = kits.Where(x => x.Missing.Count > 0).Sum(x => x.Cells) + (Directives.Current.Raw.Value<int?>("min_free_cells") ?? 36);
                if (PlayerStock.FreeCells() < need && Owned().Count(i => (i.identifier ?? "").StartsWith("storage_bay", StringComparison.Ordinal)) < ((Directives.Current.Raw["build"] as JObject)?.Value<int?>("storage_max") ?? 3)) Item(k, "storage_bay", 1, BuyAt.Jackson);
                kits.Add(k);
                if (k.Missing.Count > 0)
                    foreach (var x in kits.Where(x => x != k && x.Cells > 0 && x.Missing.Count > 0))
                        x.Needs = x.Needs.Concat(new[] { "storage" }).ToArray();
            }
            return kits;
        }
    }

    internal static partial class Production
    {
        // Bays the cellar, the crops and the reeds want, and the bays owned.
        public static (int owned, int cellar, int crops) BayCounts() =>
            (Bays().Count, WineOn ? Production.BaysTarget() : 0, Crops.On ? Crops.ExtraBays : 0);

        // The cellar's bays by its old rule (Production.WantsEngine "hydroponic"), the reeds' and crops' left to the plan.
        public static int CoreBayWants()
        {
            var bays = Bays();
            return bays.Count >= 1 && bays.All(b => MachineHydroponic.GetInputSlot(b).childItem != null)
                && bays.Count < Production.BaysTarget() + DnaExtraBays() && PlayerStore.Instance.playerCash > 1500 ? 1 : 0;
        }

        // The gene lines' shape (Breeding.cs): crop bays growing a line's crop, a bloom line, the press, manipulators.
        public static (int weedBays, bool bloom, bool press, int manipulators) BreedPlan => !BreedOn ? (0, false, false, 1)
            : (Lines().Where(l => l.Crop != "bloomberry_seed").Sum(l => (Directives.Current.Raw["crops"] as JObject)?.Value<int?>(l.Crop) ?? 0), BloomLine != null, PressOn, Manipulators);

        public static (int reeds, bool noTap, bool rust) CleanBuild => (Reeds, NoTap, RustTap);
    }
}
