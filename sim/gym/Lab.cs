using System;
using System.Collections.Generic;
using System.Linq;
using Newtonsoft.Json.Linq;

namespace Fence.Gym
{
    // `lab: "caffeine"` (docs/re/CHEMISTRY.md §6.2, docs/research/SECOND-ENGINE.md): buy the Retired Chemist and turn
    // coffee beans into caffeine pills, one batch in a work vessel, a step each morning while the day's real seconds run
    // the reactions:
    //   blend   beans into the vessel; acid equal to the protein and lipobond an eighth of the fat (both work cold,
    //           so they finish on the shelf before anything is heated), alcohol up to 3/4 of the mush
    //   heat    base for any acid left; onto the plate (unmodded it holds ~99 °C: the coffee extracts from 50 °C while
    //           the alcohol lasts, then the alcohol and lipobond boil off; caffeine never degrades)
    //   filter  through the filter pitcher (debris, plant matter, mush, physical), back into the vessel, into the
    //           powered evaporator overnight
    //   finish  once dry, the finisher presses caffeine pills (5 ml each) until the residue runs out
    // Chem modules go to the finisher first (its impurity is the purity cap: 12 % at none, 0 at four), then the blender
    // (its loss: 35 % at none, 0 at four). Pills are legal stock (HOUSEHOLD_GOOD + MEDICAL).
    internal static class Lab
    {
        public static bool On => Directives.Current.Raw.Value<string>("lab") == "caffeine";

        static int MaxBeans => Directives.Current.Raw.Value<int?>("lab_beans") ?? 12;
        static int Reserve => Directives.Current.Raw.Value<int?>("lab_reserve") ?? 500;

        const string Bean = "coffee_bean", Pack = "common_chemical", Module = "chem_module", Pitcher = "water_pitcher", Pill = "caffeine_pill";
        static readonly string[] Machines = { "blender", "heating_plate", "evaporator", "chem_finisher", "chem_scanner" };
        static readonly string[] Glass = { "100ml_cylinder", "50ml_cylinder", "25ml_cylinder", "5ml_dropper", "1ml_dropper" };
        static readonly string[] Vessels = { Pitcher, "large_bottled_water", "water_jug", "bottled_water" };

        enum Step { Idle, Blended, Heating, Drying }

        static Step step;
        static int vessel, filterPitcher, batchBeans, batchDay;
        public static int Batches, Pills, BeansUsed, PacksOpened, ModulesFitted, Stalls;
        public static long PillValue, PillSales, PillsSold;
        static readonly Dictionary<string, int> stalls = new Dictionary<string, int>(StringComparer.Ordinal);
        static readonly HashSet<int> made = new HashSet<int>();

        public static void Reset()
        {
            step = Step.Idle;
            vessel = filterPitcher = batchBeans = batchDay = 0;
            Batches = Pills = BeansUsed = PacksOpened = ModulesFitted = Stalls = 0;
            PillValue = PillSales = PillsSold = 0;
            stalls.Clear();
            made.Clear();
        }

        static bool Up => NetworkUpgrade.GetUpgradeById("RETIRED_CHEMIST")?.IsUnlocked() ?? false;

        public static IEnumerable<string> Upgrades()
        {
            if (WantsUpgrade("RETIRED_CHEMIST")) yield return "RETIRED_CHEMIST";
        }

        // `lab_after_permit` (default true): a shop that means to buy the permit buys it first; the 1 500 spent before
        // it starves the permit official's bribes, and OPERATING_WITHOUT_LICENSE piles up to an arrest.
        public static bool WantsUpgrade(string id) => On && id == "RETIRED_CHEMIST"
            && (PlayerStore.Instance.hasBusinessPermit || Directives.Current.Raw.Value<string>("permit") != "buy" || !(Directives.Current.Raw.Value<bool?>("lab_after_permit") ?? true));

        public static int UpgradeReserve => Reserve;

        static IEnumerable<GameItem> Owned() => EmporiumEntry.Instance.GetAllOwnedItems(false, true).Where(i => i != null);
        static GameItem Parent(GameItem i) => i.parentInventory?.GetParentItem();
        static GameItem Find(string id) => Owned().Where(i => i.identifier == id).OrderBy(i => i.uniqueId).FirstOrDefault();
        static GameItem ById(int uid) => uid == 0 ? null : Owned().FirstOrDefault(i => i.uniqueId == uid);

        static int Part(GameItem c, string liquid) => c.GetTagReadonly("CURRENT_PART_" + liquid.ToUpperInvariant()).GetInt();
        static int Total(GameItem c) => WaterHelper.GetTotalVolume(c);
        static double Share(GameItem c, string liquid) { int t = Total(c); return t <= 0 ? 0 : Part(c, liquid) / (double)t; }
        static int Capacity(GameItem c) => c.GetTagReadonly("LIQUID_CONTAINER_CAPACITY").GetInt();

        // A supply bottle: a loose container mostly of one reagent (a mini alcohol is 70 % alcohol, the others ~95 %).
        static IEnumerable<GameItem> Bottles(string liquid) => Owned().Where(i => Parent(i) == null && i.IsTag("LIQUID_CONTAINER_TAG") && i.identifier != "wine_bottle"
            && !Vessels.Contains(i.identifier) && !Glass.Contains(i.identifier) && Total(i) > 0 && Share(i, liquid) >= (liquid == "alcohol" ? 0.6 : 0.85)).OrderBy(i => i.uniqueId);

        static int Have(string liquid) => Bottles(liquid).Sum(b => Part(b, liquid));

        static bool Mine(GameItem i) => i.uniqueId == vessel || i.uniqueId == filterPitcher;

        // The lab's own vessels stay out of the water engine.
        public static bool Holds(GameItem i) => On && Mine(i);

        public static bool Reserved(GameItem i)
        {
            if (!On) return false;
            string id = i.identifier ?? "";
            if (Mine(i) || Machines.Contains(id) || Glass.Contains(id) || id == Pack || id == Module) return true;
            if (id == Bean) return Owned().Count(x => x.identifier == Bean && Parent(x) == null && x.uniqueId <= i.uniqueId) <= (Up ? MaxBeans : 2);
            if (id == Pitcher) return Owned().Count(x => x.identifier == Pitcher) <= 2;
            if (id == "water_filter" || id == "kotton_fabric") return Owned().Count(x => x.identifier == id && Parent(x) == null) <= 2;
            if (i.IsTag("LIQUID_CONTAINER_TAG") && Parent(i) == null && Total(i) > 0 && new[] { "alcohol", "acid", "base", "lipobond_agent" }.Any(l => Share(i, l) >= (l == "alcohol" ? 0.6 : 0.85)) && id != "wine_bottle") return true;
            return false;
        }

        static readonly string[] Inputs = { Bean, Pack, Module, Pitcher, "water_filter", "energy_credit", "blender", "heating_plate", "evaporator", "chem_finisher" };

        public static bool IsInput(string id) => On && id != null && Inputs.Contains(id);

        // The kit and the lab's goods off the counters (a buyer's "sell all" takes what is left there): into stock, or
        // the display case when stock is full (no window buyer wants a machine).
        public static void Stow()
        {
            if (!On || !Up) return;
            var e = EmporiumEntry.Instance;
            foreach (var i in new[] { e.backInvinvElement, e.frontInvinvElement }.SelectMany(inv => inv.childItems).Where(i => i != null && GeneralHelper.IsItemOwned(i) && Reserved(i) && i.identifier != Bean).ToList())
                if (Chore.Accept(e.invElement, i, -1) != 0) Chore.Accept(e.showcaseElement, i, -1);
        }

        // The blender, plate and finisher keep their chem modules in storage 0 (MachineHelper.GetModuleInv assumes a battery there).
        static GameInventory Modules(GameItem m) => Slot(m, 0) as GameInventory;

        static int Fitted(string machine) { var m = Find(machine); return m == null ? 0 : Modules(m)?.childItems.Count(x => x != null && x.identifier == Module) ?? 0; }

        public static int Wants(string id)
        {
            if (!On || id == null) return 0;
            int cash = (int)PlayerStore.Instance.playerCash, rent = PlayerDay.Current?.RentDue ?? PlayerStore.Instance.GetRentValue();
            switch (id)
            {
                case Bean: return Math.Max(0, (Up ? MaxBeans : 2) - Owned().Count(i => i.identifier == Bean && Parent(i) == null));
                case Pack: return Up && Owned().Count(i => i.identifier == Pack) == 0 && Have("alcohol") < 350000 ? 1 : 0;
                case Pitcher: return Owned().Count(i => i.identifier == Pitcher) < 2 ? 1 : 0;
                case "water_filter": return Up && !Owned().Any(i => i.identifier == "water_filter") ? 1 : 0;
                case Module:
                    int loose = Owned().Count(i => i.identifier == Module && Parent(i) == null);
                    return Up && Fitted("chem_finisher") + Fitted("blender") + loose < 8 && loose == 0 && cash > rent + 600 ? 1 : 0;
                case "blender":
                case "heating_plate":
                case "evaporator":
                case "chem_finisher":
                    return Up && Find(id) == null && cash > rent + 600 ? 1 : 0;
                case "energy_credit":
                    var ev = Find("evaporator");
                    return Up && ev != null && Charge(ev) < 12 && !Owned().Any(i => Production.Charged(i) && Parent(i) == null) ? 1 : 0;
            }
            return 0;
        }

        static int Charge(GameItem m)
        {
            var b = MachineHelper.GetBatterySlot(m)?.childItem;
            return b == null ? 0 : PowerHelper.GetAvailableEnergyFromItem(b, false);
        }

        static GraphNodeStorage Slot(GameItem m, int i) => (GraphNodeStorage)MachineHelper.GetBodyStorage(m, i);

        static void Stall(string why)
        {
            Stalls++;
            stalls[why] = (stalls.TryGetValue(why, out int n) ? n : 0) + 1;
        }

        public static void Morning(IExecutorHost ep)
        {
            if (!On || !Up) return;
            try { Run(ep); }
            catch (Exception e) { ep.ProductionError(e); }
        }

        static void Run(IExecutorHost ep)
        {
            var stock = EmporiumEntry.Instance.invElement;
            // The kit lands on the back counter, where a buyer's "sell all" would take it.
            foreach (var i in Owned().Where(i => Parent(i) == null && !ReferenceEquals(i.parentInventory, stock) && Reserved(i) && i.identifier != Bean).ToList())
                Chore.Accept(stock, i, -1);
            FitModules();
            var blender = Find("blender");
            var plate = Find("heating_plate");
            var evap = Find("evaporator");
            var finisher = Find("chem_finisher");
            if (blender == null || plate == null || evap == null || finisher == null) { Stall("machines"); return; }
            if (ById(vessel) == null) step = Step.Idle;
            switch (step)
            {
                case Step.Blended: Heat(plate); return;
                case Step.Heating: Filter(ep, plate, evap); return;
                case Step.Drying:
                    if (!Finish(ep, evap, finisher)) return;
                    break;
            }
            Blend(ep, blender);
        }

        static void FitModules()
        {
            foreach (var machine in new[] { "chem_finisher", "blender" })
            {
                var m = Find(machine);
                if (m == null) continue;
                foreach (var mod in Owned().Where(i => i.identifier == Module && Parent(i) == null).ToList())
                {
                    if (Fitted(machine) >= 4) break;
                    if (Chore.Accept(Modules(m), mod, -1) == 0) ModulesFitted++;
                }
            }
        }

        // The filter pitcher, and the work vessel: the largest other empty pitcher, jug or water bottle (the kit's two
        // bottles, once the bays have drunk them, hold a batch of about five beans).
        static bool PickVessels()
        {
            var cands = Owned().Where(i => Vessels.Contains(i.identifier) && (Parent(i) == null || Mine(i)) && !Production.HeldForWine(i)).OrderBy(i => i.uniqueId).ToList();
            if (ById(filterPitcher) == null) filterPitcher = cands.FirstOrDefault(i => i.identifier == Pitcher && i.uniqueId != vessel)?.uniqueId ?? 0;
            if (ById(vessel) == null) vessel = cands.Where(i => i.uniqueId != filterPitcher && Total(i) < 20000).OrderByDescending(Capacity).ThenBy(i => i.uniqueId).FirstOrDefault()?.uniqueId ?? 0;
            return vessel != 0 && filterPitcher != 0;
        }

        static void Blend(IExecutorHost ep, GameItem blender)
        {
            if (!PickVessels()) { Stall("vessels"); return; }
            var v = ById(vessel);
            var beans = Owned().Where(i => i.identifier == Bean && Parent(i) == null && !Crops.Reserved(i) && !StolenHelper.IsStolenItem(i)).OrderBy(i => i.uniqueId).ToList();
            if (beans.Count == 0) { Stall("beans"); return; }
            OpenPackIfShort(ep);
            int alcohol = Have("alcohol");
            if (alcohol < 20000 || !Bottles("acid").Any() || !Bottles("lipobond_agent").Any()) { Stall("reagents"); return; }
            // ~45 ml of mush a bean (less the blender's loss); alcohol at 3/4 of it gives the full yield, and a
            // third of that still most of it, so a short supply only shrinks the batch.
            int perBean = 34000;
            int n = Math.Min(Math.Min(beans.Count, MaxBeans), Math.Max(1, alcohol * 2 / perBean));
            n = Math.Min(n, Math.Max(1, (Capacity(v) - Total(v)) / 150000));
            var input = Slot(blender, 1);
            var cslot = Slot(blender, 2);
            if (Chore.Accept(cslot, v, -1) != 0) { Stall("blender"); return; }
            int loaded = 0;
            foreach (var b in beans.Take(n))
                if (Chore.Accept(input, b, -1) == 0) loaded++;
            var press = ep.MachineButton(blender, LocHelper.GetLocalizedMechanic("mech_blender_btn_start"));
            press?.Invoke();
            int mush = Part(v, "coffee_mush");
            if (press == null || mush == 0) { Stall("blend"); return; }
            Pour("acid", v, Part(v, "protein") - Part(v, "acid") + Part(v, "base"));
            Pour("lipobond_agent", v, Part(v, "fat") / 8 + 500);
            Pour("alcohol", v, Math.Min(Have("alcohol"), mush * 3 / 4));
            batchBeans = loaded;
            batchDay = ep.Day;
            BeansUsed += loaded;
            step = Step.Blended;
            ep.Human("production", null, ep.Uid(v), new JObject { ["action"] = "lab_blend", ["beans"] = loaded, ["mush_ul"] = mush, ["alcohol_ul"] = Part(v, "alcohol") }, "player:lab");
        }

        static void OpenPackIfShort(IExecutorHost ep)
        {
            if (Have("alcohol") >= 150000 && Bottles("acid").Any() && Bottles("base").Any() && Bottles("lipobond_agent").Any()) return;
            var pack = Owned().Where(i => i.identifier == Pack && !StolenHelper.IsStolenItem(i)).OrderBy(i => i.uniqueId).FirstOrDefault();
            if (pack?.onActivateSlotItemFunc == null) return;
            var stock = EmporiumEntry.Instance.invElement;
            if (!ReferenceEquals(pack.parentInventory, stock)) Chore.Accept(stock, pack, -1);
            Chore.Act("unpack", pack, () => pack.onActivateSlotItemFunc(pack, pack.parentInventory as GameInventory, null));
            PacksOpened++;
        }

        // A measured pour of `ul` of a reagent: the supply bottle's mix through the graduated glassware, largest first.
        static void Pour(string liquid, GameItem into, int ul)
        {
            if (ul <= 0) return;
            var glass = Glass.Select(Find).Where(g => g != null).ToList();
            foreach (var src in Bottles(liquid).ToList())
            {
                double share = Share(src, liquid);
                int mix = (int)(ul / share);
                foreach (var g in glass)
                {
                    int cap = Capacity(g);
                    while (mix >= cap && Total(src) > 0)
                    {
                        if (Total(g) > 0) Chore.Act("pour", g, () => WaterHelper.TransferLiquid(g, into));
                        int before = Part(into, liquid);
                        Chore.Act("pour", src, () => WaterHelper.TransferLiquid(src, g));
                        Chore.Act("pour", g, () => WaterHelper.TransferLiquid(g, into));
                        int moved = Part(into, liquid) - before;
                        if (moved <= 0) break;
                        ul -= moved;
                        mix = (int)(ul / share);
                    }
                }
                if (ul < 1000) return;
            }
        }

        static void Heat(GameItem plate)
        {
            var v = ById(vessel);
            Pour("base", v, Part(v, "acid") - Part(v, "base"));
            if (Chore.Accept(Slot(plate, 1), v, -1) != 0) { Stall("plate"); return; }
            Chore.Act("plate_on", plate, () => plate.ModifyTag("HEATING_PLATE_STATE", s => s.Enable().SetInt(2)));
            step = Step.Heating;
        }

        static void Filter(IExecutorHost ep, GameItem plate, GameItem evap)
        {
            var v = ById(vessel);
            if (ById(filterPitcher) == null) PickVessels();
            var f = ById(filterPitcher);
            if (f == null) { Stall("vessels"); return; }
            var stock = EmporiumEntry.Instance.invElement;
            Chore.Act("plate_off", plate, () => plate.ModifyTag("HEATING_PLATE_STATE", s => s.Enable().SetInt(0)));
            // The water filter (size 22) takes debris, mush, plant matter and physical; kotton (150) only the first three.
            var seated = WaterHelper.GetFilterItem(f);
            if (seated == null || DurabilityHelper.GetRealDurability(seated) < 60)
            {
                var fresh = Owned().Where(i => (i.identifier == "water_filter" || i.identifier == "kotton_fabric") && Parent(i) == null && DurabilityHelper.GetRealDurability(i) >= 60)
                    .OrderBy(i => i.identifier == "water_filter" ? 0 : 1).ThenBy(i => i.uniqueId).FirstOrDefault();
                if (fresh != null)
                {
                    if (seated != null) Chore.Accept(stock, seated, -1);
                    foreach (var inv in GraphUtils.FindChildrenType<GameInventory>(f))
                        if (Chore.Accept(inv, fresh, -1) == 0) break;
                }
            }
            if (WaterHelper.GetFilterItem(f) == null) Stall("no_filter");
            if (Total(f) > 0) { Stall("filter_pitcher_full"); return; }
            Chore.Act("pour", v, () => WaterHelper.TransferLiquid(v, f));
            Chore.Act("pour", f, () => WaterHelper.TransferLiquid(f, v));
            if (Charge(evap) < 6)
            {
                var cell = Owned().Where(i => Production.Charged(i) && Parent(i) == null).OrderByDescending(i => PowerHelper.GetAvailableEnergyFromItem(i, false)).FirstOrDefault();
                var bat = MachineHelper.GetBatterySlot(evap);
                if (cell != null)
                {
                    if (bat.childItem != null) Chore.Accept(stock, bat.childItem, -1);
                    Chore.Accept(bat, cell, -1);
                }
            }
            if (Chore.Accept(Slot(evap, 2), v, -1) != 0) { Stall("evaporator"); return; }
            step = Step.Drying;
        }

        // True once the batch is pressed and the vessel is free again.
        static bool Finish(IExecutorHost ep, GameItem evap, GameItem finisher)
        {
            var v = ById(vessel);
            var stock = EmporiumEntry.Instance.invElement;
            if (WaterHelper.IsWet(v))
            {
                if (Charge(evap) < 6) Stall("evaporator_power");
                return false;
            }
            if (Chore.Accept(Slot(finisher, 1), v, -1) != 0) { Stall("finisher"); return false; }
            Chore.Act("configure", finisher, () => finisher.ModifyTag("SELECTED_CHEM_ID_TAG", s => s.Enable().SetString(Pill)));
            var output = (GameInventory)Slot(finisher, 2);
            string label = LocHelper.GetLocalizedMechanic("mech_chem_finisher_btn_output");
            int pressed = 0;
            for (int k = 0; k < 400 && MachineChemFinisher.CanOutput(finisher, (GameSlotInventory)Slot(finisher, 1)); k++)
            {
                var press = ep.MachineButton(finisher, label);
                if (press == null) break;
                press();
                pressed++;
                foreach (var p in output.childItems.Where(x => x != null).ToList())
                {
                    if (!made.Add(p.uniqueId)) continue;
                    Chore.Act("own", p, () => GeneralHelper.SetItemOwned(p, true));
                    Pills++;
                    PillValue += PlayerRole.Est(p);
                    if (Chore.Accept(stock, p, -1) != 0) Chore.Accept(EmporiumEntry.Instance.backInvinvElement, p, -1);
                }
            }
            Batches++;
            step = Step.Idle;
            ep.Human("production", null, ep.Uid(v), new JObject { ["action"] = "lab_finish", ["beans"] = batchBeans, ["pills"] = pressed, ["days"] = ep.Day - batchDay }, "player:lab");
            return true;
        }

        public static void Trade(bool sold, GameItem item, int amount)
        {
            if (!On || !sold || item == null || item.identifier != Pill) return;
            PillsSold++;
            PillSales += amount;
        }

        public static JObject Status() => !On ? null : new JObject
        {
            ["unlocked"] = Up, ["step"] = step.ToString(), ["batches"] = Batches, ["beans_used"] = BeansUsed, ["pills"] = Pills, ["pill_value"] = PillValue,
            ["pills_sold"] = PillsSold, ["pill_sales"] = PillSales, ["packs_opened"] = PacksOpened, ["modules_fitted"] = ModulesFitted,
            ["finisher_modules"] = Fitted("chem_finisher"), ["blender_modules"] = Fitted("blender"), ["stalls"] = JObject.FromObject(stalls),
        };
    }
}
