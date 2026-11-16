using System;
using System.Collections.Generic;
using System.Linq;
using Newtonsoft.Json.Linq;

namespace Fence.Gym
{
    // Free power (`power: {...}`, docs/research/CHEMISTRY-ENGINE.md "Free power"): POWER_HIJACK keeps the store
    // powered with no service fee (PlayerStore.UpdatePowerState), and a recharger (Dr. Jackson, 225 on hard) charges
    // every rechargeable cell in its 8 slots +3 a night (+6 with COMMERCIAL_POWER) while the store has power
    // (MachineRecharger). Off when `power` is absent.
    //
    // The interface for engines is the loose charged cell: each morning (Morning, before the engines) spent loose
    // cells go into the recharger and full ones come out, so an engine that swaps a loose cell into its machine and
    // leaves the spent one loose (every engine here does) runs on recharged cells without knowing about this file.
    // Engines that want to say how much they draw call Need(id, perNight) once a morning; the payback test uses it.
    internal static class Power
    {
        const string Recharger = "recharger", Hijack = "POWER_HIJACK", Commercial = "COMMERCIAL_POWER", Cell = "energy_credit";
        const int Slots = 8;
        // A loose cell worth handing to an engine: Production.Charged's 30 (its bays and racks want that much).
        const int Ready = 30;

        static JToken Raw => Directives.Current.Raw["power"];
        static JObject Cfg => Raw as JObject;

        public static bool On => Raw != null && Raw.Type != JTokenType.Null && !(Raw.Type == JTokenType.Boolean && !(bool)Raw);

        // `buy: "auto"` (default) buys the kit once it pays back within `payback_weeks` (4) at `unit_price` (1.5 a unit,
        // what cells cost the gym's engines); "always" buys it as soon as anything draws power.
        static bool Always => Cfg?.Value<string>("buy") == "always";
        static double PaybackWeeks => Cfg?.Value<double?>("payback_weeks") ?? 4;
        static double UnitPrice => Cfg?.Value<double?>("unit_price") ?? 1.5;
        // COMMERCIAL_POWER (525 on hard) doubles every recharger: cheaper than a second recharger and its eight cells.
        static bool CommercialOn => Cfg?.Value<bool?>("commercial") ?? true;
        static int MaxChargers => Cfg?.Value<int?>("rechargers") ?? 2;
        // Loose charged cells kept for the engines' morning swaps.
        static int LooseCells => Cfg?.Value<int?>("loose") ?? Math.Max(4, Seated() + 1);

        static readonly Dictionary<string, int> need = new Dictionary<string, int>(StringComparer.Ordinal);
        public static int CellsIn, CellsOut, Swaps;
        static int firstFreeDay;

        public static void Reset()
        {
            need.Clear();
            CellsIn = CellsOut = Swaps = 0;
            firstFreeDay = 0;
            HubFirstDay = HubMoves = HubNights = HubNeighbourNights = 0;
            HubUnits = 0;
        }

        // An engine's draw a night (evaporator 6, a hydroponic bay 8): what the kit would carry.
        public static void Need(string engine, int perNight)
        {
            if (On) need[engine] = Math.Max(0, perNight);
        }

        static IEnumerable<GameItem> Owned() => EmporiumEntry.Instance.GetAllOwnedItems(false, true).Where(i => i != null);
        static GameItem Parent(GameItem i) => i.parentInventory?.GetParentItem();

        static bool HijackUp => NetworkUpgrade.IsPowerHijackUnlocked();
        static List<GameItem> Chargers() => Owned().Where(i => i.identifier == Recharger).OrderBy(i => i.uniqueId).ToList();
        static GameItem Charger() => Chargers().FirstOrDefault();
        static int Rate => NetworkUpgrade.IsCommercialPowerUnlocked() ? 6 : 3;

        // Free: the recharger runs every night and costs nothing to run.
        public static bool Free => On && HijackUp && Charger() != null;

        static int Draw()
        {
            // Machines holding a cell draw whether or not their engine reported it.
            int seated = Owned().Where(m => m.contentWindow != null && MachineHelper.GetBatterySlot(m)?.childItem != null).Sum(MachineryHelper.GetMachinePowerUsage);
            return Math.Max(seated, need.Values.Sum());
        }

        static int Capacity => Rate * Slots * Math.Max(1, Chargers().Count);

        // Hijack + recharger against what the same charge costs as bought cells.
        static bool Pays(int cost)
        {
            int d = Math.Min(Draw(), Capacity);
            if (d <= 0) return false;
            return Always || cost <= PaybackWeeks * 7 * d * UnitPrice;
        }

        static int KitCost => (HijackUp ? 0 : 450) + (Charger() != null ? 0 : 225);

        public static IEnumerable<string> Upgrades()
        {
            if (!On) yield break;
            if (WantsUpgrade(Hijack)) yield return Hijack;
            if (WantsUpgrade(Commercial)) yield return Commercial;
        }

        // The lab's machines come before its power (a kit without them powers nothing it was bought for).
        static bool Waits => Lab.On && !Lab.Running;

        public static bool WantsUpgrade(string id)
        {
            if (!On || Waits) return false;
            if (id == Hijack) return !HijackUp && Pays(KitCost);
            // COMMERCIAL_POWER doubles the recharger: worth it once the draw passes what one recharger carries.
            if (id == Commercial) return CommercialOn && Free && !NetworkUpgrade.IsCommercialPowerUnlocked() && Draw() > Capacity && Pays(525);
            return false;
        }

        public static int Wants(string id)
        {
            if (!On || id == null) return 0;
            int cash = (int)PlayerStore.Instance.playerCash, rent = PlayerDay.Current?.RentDue ?? PlayerStore.Instance.GetRentValue();
            switch (id)
            {
                // After the hijack (a recharger on the paid service costs its fee every night).
                // Another once the draw outgrows them (after COMMERCIAL_POWER when that is on).
                case Recharger:
                    int n = Chargers().Count;
                    bool more = n == 0 || (n < MaxChargers && Draw() > Capacity && (!CommercialOn || NetworkUpgrade.IsCommercialPowerUnlocked()));
                    return HijackUp && more && !Waits && cash > rent + 400 ? 1 : 0;
                // Enough to carry the draw at the charge rate, plus the loose ones and one per machine.
                case Cell:
                    int pool = Math.Min(Slots * Math.Max(1, Chargers().Count), (Draw() + Rate - 1) / Rate) + LooseCells + Seated();
                    return Free ? Math.Max(0, pool - Owned().Count(Rechargeable)) : 0;
            }
            return 0;
        }

        public static bool IsInput(string id) => On && (id == Recharger || (id == Cell && Free));

        public static bool JacksonBuys(string id) => On && (id == Recharger || (id == Cell && Free));

        public static bool Reserved(GameItem i) => (On ? i.identifier == Recharger || (Free && Rechargeable(i)) : CellarKit && i.identifier == Recharger) || (HubOn && Rechargeable(i) && Hub() != null);

        static bool Rechargeable(GameItem i) => i.IsTag("power_source_item") && !PowerHelper.IsDisposablePowerSource(i) && !CurrentConverterHelper.IsCurrentConverter(i)
            && i.identifier != "energy_credit_hyp" && (i.identifier ?? "").StartsWith(Cell, StringComparison.Ordinal);

        static int Seated() => Owned().Count(m => m.contentWindow != null && MachineHelper.GetBatterySlot(m)?.childItem != null);

        static int Energy(GameItem i) => PowerHelper.GetAvailableEnergyFromItem(i, false);
        static bool Full(GameItem i) => Energy(i) >= i.GetTagReadonly("power_source_item_max_energy").GetInt();

        // Each morning before the engines: the recharger into the stock grid (a kit lands on the back counter), spent
        // loose cells in, full ones out.
        public static void Morning(IExecutorHost ep)
        {
            if (!On) return;
            try { Rotate(ep); }
            catch (Exception e) { ep.ProductionError(e); }
        }

        static void Rotate(IExecutorHost ep)
        {
            var chargers = Chargers();
            if (chargers.Count == 0) return;
            var e = EmporiumEntry.Instance;
            foreach (var charger in chargers)
                if (!ReferenceEquals(charger.parentInventory, e.invElement) && Chore.Accept(e.invElement, charger, -1) != 0 && !ReferenceEquals(charger.parentInventory, e.showcaseElement))
                    Chore.Accept(e.showcaseElement, charger, -1);
            if (firstFreeDay == 0 && HijackUp) firstFreeDay = ep.Day;
            foreach (var slots in chargers.Select(c => GraphUtils.FindChildrenType<GameInventory>(c).FirstOrDefault()).Where(x => x != null))
                Rotate(slots);
        }

        static void Rotate(GameInventory slots)
        {
            var e = EmporiumEntry.Instance;
            // Out: every full cell, and the fullest while fewer than `loose` (4) loose cells hold a few nights' draw (a cell
            // fills at +3 a night, so waiting for full would idle the machines).
            foreach (var c in slots.childItems.Where(x => x != null && Full(x)).ToList())
                if (Chore.Accept(e.invElement, c, -1) == 0) CellsOut++;
            while (Owned().Count(x => Rechargeable(x) && Parent(x) == null && Energy(x) >= Ready) < LooseCells
                && slots.childItems.Where(x => x != null && Energy(x) >= Ready).OrderByDescending(Energy).ThenBy(x => x.uniqueId).FirstOrDefault() is GameItem best
                && Chore.Accept(e.invElement, best, -1) == 0)
                CellsOut++;
            // In: loose cells below that.
            foreach (var c in Owned().Where(x => Rechargeable(x) && Parent(x) == null && Energy(x) < Ready).OrderBy(Energy).ThenBy(x => x.uniqueId).ToList())
            {
                if (slots.childItems.Count(x => x != null) >= Slots) break;
                if (Chore.Accept(slots, c, -1) == 0) CellsIn++;
            }
        }

        // A machine's battery below `min`: swapped for the fullest loose cell, the spent one left loose for the
        // recharger. For engines without their own cell logic; true when the machine now holds `min`.
        public static bool TopUp(GameItem machine, int min)
        {
            var slot = MachineHelper.GetBatterySlot(machine);
            if (slot == null) return false;
            int have = slot.childItem == null ? 0 : Energy(slot.childItem);
            if (have >= min) return true;
            var cell = Owned().Where(x => Rechargeable(x) && Parent(x) == null).OrderByDescending(Energy).ThenBy(x => x.uniqueId).FirstOrDefault();
            if (cell == null || Energy(cell) <= have) return false;
            if (slot.childItem != null) Chore.Accept(EmporiumEntry.Instance.invElement, slot.childItem, -1);
            if (Chore.Accept(slot, cell, -1) != 0) return false;
            Swaps++;
            return Energy(cell) >= min;
        }

        // ---- the water cellar's kit (`clean_water.power`, docs/research/QUALITY.md §8) ----
        // Its bays' cells, recharged: the recharger once two bays grow, POWER_HIJACK after it, COMMERCIAL_POWER with four
        // bays (`commercial_power`), a cell in each bay and four charging. The cellar calls these at its own points in
        // the morning; with `power` set this file's own rules run instead.

        static bool CellarKit => !On && Production.CellarPowerKit;

        public static bool HasRecharger => LooseCharger() != null;

        public static bool CellarWantsRecharger => CellarKit && Production.CellarPowerWanted;

        public static IEnumerable<string> CellarUpgrades()
        {
            if (!CellarWantsRecharger || LooseCharger() == null) yield break;
            yield return Hijack;
            if (Production.CellarBayCount >= 4 && Production.CellarCommercialPower) yield return Commercial;
        }

        static GameItem LooseCharger() => Owned().FirstOrDefault(i => i.identifier == Recharger && Parent(i) == null);

        public static int CellarCellWants() => !CellarKit || LooseCharger() == null ? -1 : Math.Max(0, Production.CellarBayCount + 4 - Owned().Count(i => i.identifier == Cell));

        // Before the bays: the recharger's fullest cells out for any bay that will need one. After: every loose cell short
        // of full back into it, a full one making room for an empty one.
        public static void CellarTend(bool before)
        {
            var r = LooseCharger();
            if (!CellarKit || r == null) return;
            var stock = EmporiumEntry.Instance.invElement;
            if (before)
            {
                int low = Production.CellarLowBays;
                int loose = Owned().Count(i => Production.Charged(i) && Parent(i) == null);
                foreach (var cell in GraphUtils.FindAllChildrenType<GameItem>(r).Where(i => i.identifier == Cell && PowerHelper.GetAvailableEnergyFromItem(i, false) >= 40)
                             .OrderByDescending(i => PowerHelper.GetAvailableEnergyFromItem(i, false)).ThenBy(i => i.uniqueId).Take(Math.Max(0, low - loose)).ToList())
                    Chore.Accept(stock, cell, -1);
                return;
            }
            var grid = GraphUtils.FindChildrenType<GameGridInventory>(r).FirstOrDefault();
            if (grid == null) return;
            var spare = Owned().Where(i => i.identifier == Cell && Parent(i) == null && PowerHelper.GetAvailableEnergyFromItem(i, false) < 100)
                .OrderBy(i => PowerHelper.GetAvailableEnergyFromItem(i, false)).ThenBy(i => i.uniqueId).ToList();
            foreach (var full in GraphUtils.FindAllChildrenType<GameItem>(r).Where(i => PowerHelper.GetAvailableEnergyFromItem(i, false) >= 100).Take(spare.Count).ToList())
                Chore.Accept(stock, full, -1);
            foreach (var cell in spare)
                if (Chore.Accept(grid, cell, -1) != 0) break;
        }

        // ---- the eel hub (`eel_hub`, docs/research/FORMULA-EXPLOITS.md §6; lab `fish.eelhub`) ----
        // An electric-eel tank charges its own battery and then, at the same full output, every cell and every battery
        // machine edge-adjacent to it in the stock grid, blackouts included (AquacultureHelper.GeneratePower, the night's
        // early pass). Each evening the machines running on a cell (highest draw first) and the loose cells short of full
        // (emptiest first) are moved next to the hub tank, and spent cells are kept for it. The fish engine keeps
        // `eel_hub: {"eels": n}` electric eels (default 2) in its big tank and leaves converters out of the tanks the hub
        // carries.

        static JToken HubRaw => Directives.Current.Raw["eel_hub"];
        public static bool HubOn => HubRaw != null && HubRaw.Type != JTokenType.Null && !(HubRaw.Type == JTokenType.Boolean && !(bool)HubRaw);
        public static int HubEels => (HubRaw as JObject)?.Value<int?>("eels") ?? 2;
        public static int HubFirstDay, HubMoves, HubNights, HubNeighbourNights;
        public static long HubUnits;

        // The power the tank's fish make tonight (the tooltip's power output: 1/2/3 per electric eel by stage, none starving).
        static int Output(GameItem ras) => MachineRAS.GetTankGrid(ras) is GameGridInventory g ? AquacultureHelper.GetTankPowerOutput(g) : 0;

        public static GameItem Hub()
        {
            if (!HubOn || EmporiumEntry.Instance == null) return null;
            var stock = EmporiumEntry.Instance.invElement;
            return Owned().Where(i => i.IsTag("RAS_TAG") && ReferenceEquals(i.parentInventory, stock) && Output(i) > 0).OrderByDescending(Output).ThenBy(i => i.uniqueId).FirstOrDefault();
        }

        public static int HubOutput() => Hub() is GameItem h ? Output(h) : 0;

        // A machine the hub runs by itself once beside it: its draw is within the hub's output.
        public static bool HubCarries(GameItem machine) => HubOn && Hub() is GameItem h && h != machine && Output(h) >= MachineryHelper.GetMachinePowerUsage(machine);

        static bool OnCell(GameItem m) => MachineHelper.GetBatterySlot(m)?.childItem is GameItem c && Rechargeable(c);

        public static void Evening(IExecutorHost ep)
        {
            if (!HubOn) return;
            try { Arrange(ep); }
            catch (Exception e) { ep.ProductionError(e); }
        }

        static void Arrange(IExecutorHost ep)
        {
            var hub = Hub();
            if (hub == null) return;
            if (HubFirstDay == 0) HubFirstDay = ep.Day;
            var stock = EmporiumEntry.Instance.invElement;
            var machines = Owned().Where(m => m != hub && Parent(m) == null && m.contentWindow != null && OnCell(m) && MachineryHelper.GetMachinePowerUsage(m) > 0)
                .OrderByDescending(MachineryHelper.GetMachinePowerUsage).ThenBy(m => m.uniqueId).ToList();
            var cells = Owned().Where(c => Rechargeable(c) && Parent(c) == null && !Full(c)).OrderBy(Energy).ThenBy(c => c.uniqueId).ToList();
            foreach (var it in machines.Concat(cells))
            {
                if (hub.FindAdjacentType<GameItem>().Contains(it)) continue;
                if (PlaceBeside(stock, hub, it)) HubMoves++;
            }
            int n = hub.FindAdjacentType<GameItem>().Count(i => i != hub && ((Rechargeable(i) && !Full(i)) || (i.contentWindow != null && OnCell(i))));
            if (Environment.GetEnvironmentVariable("FENCE_HUB_DEBUG") != null)
                Console.Error.WriteLine($"hub d{ep.Day} out={Output(hub)} at=({hub.modifiedShape.minX},{hub.modifiedShape.minY})-({hub.modifiedShape.maxX},{hub.modifiedShape.maxY}) grid={stock.inventoryShape.width}x{stock.inventoryShape.height} machines={machines.Count} cells={cells.Count} near={string.Join(",", hub.FindAdjacentType<GameItem>().Where(i => i != hub).Select(i => i.identifier + ":" + (Rechargeable(i) ? Energy(i) : -1)))} counted={n}");
            HubNights++;
            HubNeighbourNights += n;
            HubUnits += (long)n * Output(hub);
        }

        // The item dropped on a free spot of the grid that touches the hub tank's edge, in any orientation (a drag and drop).
        static bool PlaceBeside(GameGridInventory grid, GameItem hub, GameItem it)
        {
            GridShape h = hub.modifiedShape, own = it.modifiedShape;
            if (h == null || own == null) return false;
            int reach = Math.Max(own.globalWidth, own.globalHeight);
            for (int o = 4; o >= 1; o--)
                for (int y = h.minY - reach; y <= h.maxY + 1; y++)
                    for (int x = h.minX - reach; x <= h.maxX + 1; x++)
                    {
                        var b = new GridShapeBuilder(own);
                        b.SetTransform(x, y, false, o);
                        if (!Touches(b, h)) continue;
                        var slot = grid.TryInventorySlot(it, it.unitCount, b);
                        if (slot == null || slot.numTransfer <= 0) continue;
                        if (Chore.Act("place_beside", it, () => slot.TryAcceptOnce(it.unitCount)) > 0 && ReferenceEquals(it.parentInventory, grid)) return true;
                    }
            return false;
        }

        // GameGridInventory.IsAdjacent's test: the hub's shape moved one cell along an axis overlaps the candidate's.
        static bool Touches(GridShape cand, GridShape hub)
        {
            if (cand.Intersects(hub)) return false;
            foreach (var (dx, dy) in new[] { (1, 0), (-1, 0), (0, 1), (0, -1) })
            {
                var moved = new GridShapeBuilder(hub);
                moved.minX += dx;
                moved.minY += dy;
                if (cand.Intersects(moved)) return true;
            }
            return false;
        }

        static JObject HubStatus()
        {
            var hub = Hub();
            return new JObject
            {
                ["first_day"] = HubFirstDay, ["output"] = hub == null ? 0 : Output(hub), ["moves"] = HubMoves, ["nights"] = HubNights,
                ["neighbour_nights"] = HubNeighbourNights, ["units"] = HubUnits,
                ["neighbours"] = hub == null ? 0 : hub.FindAdjacentType<GameItem>().Count(i => i != hub),
            };
        }

        public static JObject Status()
        {
            if (!On) return HubOn ? new JObject { ["hub"] = HubStatus() } : null;
            var chargers = Chargers();
            int charging = chargers.Sum(c => GraphUtils.FindChildrenType<GameInventory>(c).FirstOrDefault()?.childItems.Count(x => x != null) ?? 0);
            return new JObject
            {
                ["hijack"] = HijackUp, ["commercial"] = NetworkUpgrade.IsCommercialPowerUnlocked(), ["rechargers"] = chargers.Count, ["free_from_day"] = firstFreeDay,
                ["draw"] = Draw(), ["need"] = JObject.FromObject(need), ["cells"] = Owned().Count(Rechargeable),
                ["capacity"] = Free ? Capacity : 0, ["charging"] = charging, ["cells_in"] = CellsIn, ["cells_out"] = CellsOut, ["swaps"] = Swaps,
                ["hub"] = HubOn ? HubStatus() : null,
            };
        }
    }
}
