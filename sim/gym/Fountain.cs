using System;
using System.Collections.Generic;
using System.Linq;
using Newtonsoft.Json.Linq;

namespace Fence.Gym
{
    // The water fountain (Wild's VENDING_FOUNTAIN, or the landlord's offer for +150 weekly rent; docs/re/WATER.md
    // §2.2, VendingFountainHelper.HandleNight). Each powered night it pours bottles from its Base, High Quality and
    // Pure slots and sells every one, no customer and no chance: at attractiveness 15 a 750 ml Base bottle and a
    // 450 ml HQ bottle; each +5 attractiveness (a common electronic) adds size or bottles, up to 4.75 L Base,
    // 2.75 L HQ and 2 L Pure a night at 100. A bottle is labelled as its slot's grade, so a slot only gets water
    // whose true grade is at least the slot's (a better grade sells at the slot's price; a worse one is a crime).
    internal static class Fountain
    {
        public static int AttractUpgrades, Filled;
        public static long Income;

        public static void Reset() { AttractUpgrades = Filled = 0; Income = 0; }

        // `fountain`: "landlord" takes the landlord's offer (days 43, 57, 71, 85; +150 rent a week, compounding);
        // "buy" buys Wild's (1 200 × difficulty) from `fountain_day` (default 1) once `fountain_reserve` (default
        // 500) is left over the rent and floor. Unset: never owned, never touched.
        static string Mode => Directives.Current.Raw.Value<string>("fountain");
        public static bool On => Mode == "landlord" || Mode == "buy";
        public static bool TakesLandlords => Mode == "landlord";
        public static bool Buys => Mode == "buy";
        public static int BuyDay => Directives.Current.Raw.Value<int?>("fountain_day") ?? 1;
        public static int Reserve => Directives.Current.Raw.Value<int?>("fountain_reserve") ?? 500;

        static IEnumerable<GameItem> Owned() => EmporiumEntry.Instance.GetAllOwnedItems(false, true).Where(i => i != null);
        static GameItem Parent(GameItem i) => i.parentInventory?.GetParentItem();

        public static GameItem Machine() => !On || EmporiumEntry.Instance == null || !PlayerStore.Instance.isVendingFountainUnlocked ? null
            : GraphUtils.FindAllChildrenType<GameItem>(EmporiumEntry.Instance.vendingFountainElement).FirstOrDefault(i => i.identifier == "vending_fountain");

        static GameItem Core(GameItem m) => GraphUtils.FindAllChildrenType<GameItem>(m).FirstOrDefault(i => i.identifier == "vending_fountain_core");
        static int Attract(GameItem m) => m.GetTagReadonly("VENDING_ATTRACTIVENESS").GetInt();

        // Slots 1–3 are Base, HQ and Pure (VendingFountainHelper.GetTotalCapacityPercentage); the floor purity of
        // each grade in basis points (WaterFeatureHelper's bands).
        static readonly int[] Floor = { 9800, 9900, 9990 };

        static GameSlotInventory Slot(GameItem m, int grade) => MachineHelper.GetSlot(m, grade + 1) as GameSlotInventory;

        public static void Morning(IExecutorHost ep)
        {
            var m = Machine();
            if (m == null) return;
            int cash = m.GetTagReadonly("VENDING_CASH").GetInt();
            if (cash > 0)
            {
                Chore.Act("vending_collect", m, () => VendingMachineHelper.TransferCashToPlayer(m));
                Income += cash;
                ep.Human("vending", null, null, new JObject { ["action"] = "fountain_collect", ["cash"] = cash }, "player:fountain");
            }
            // Empty or nearly empty vessels come back for the water engine to refill.
            var stock = EmporiumEntry.Instance.invElement;
            for (int g = 0; g < 3; g++)
            {
                var c = Slot(m, g)?.childItem;
                if (c != null && WaterHelper.GetCurrentCapacityML(c) < 450) Chore.Accept(stock, c, -1);
            }
            Power(m);
            // Common electronics go into the fountain before the vending machine (its chores run first).
            if (!(PlayerDay.Current?.Cuts("vending") ?? false)) Upgrade(m);
        }

        static void Power(GameItem m)
        {
            var slot = MachineHelper.GetBatterySlot(m);
            if (slot == null) return;
            int need = 2 * MachineryHelper.GetMachinePowerUsage(m);
            int have = slot.childItem == null ? 0 : PowerHelper.GetAvailableEnergyFromItem(slot.childItem, false);
            if (have >= need) return;
            var cell = Owned().Where(i => i.identifier == "energy_credit" && Parent(i) == null && PowerHelper.GetAvailableEnergyFromItem(i, false) > Math.Max(have, need))
                .OrderBy(i => PowerHelper.GetAvailableEnergyFromItem(i, false)).ThenBy(i => i.uniqueId).FirstOrDefault();
            if (cell == null) return;
            var stock = EmporiumEntry.Instance.invElement;
            if (slot.childItem != null) Chore.Accept(stock, slot.childItem, -1);
            Chore.Accept(slot, cell, -1);
        }

        static void Upgrade(GameItem m)
        {
            var core = Core(m);
            if (core == null) return;
            foreach (var e in Owned().Where(i => i.identifier == "common_electronic" && Parent(i) == null && !Production.Reserved(i) && !PlayerRole.Keep(i, null)).OrderBy(i => i.uniqueId).ToList())
            {
                if (!VendingMachineHelper.CanUpgradedAttractiveness(m)) break;
                Chore.Act("vending_upgrade", core, () => VendingMachineHelper.UpgradeAttractiveness(core, e));
                AttractUpgrades++;
            }
        }

        static int Purity(GameItem i)
        {
            try { return WaterHelper.IsEmpty(i) ? 0 : WaterHelper.GetWaterPurity(i); }
            catch (Exception) { return 0; }
        }

        // A filled vessel the fountain may pour: water, owned, loose, not a wine bottle and not held for wine.
        static bool Pourable(GameItem i) => i.IsTag("LIQUID_CONTAINER_TAG") && i.IsGameItemType("WATER") && Parent(i) == null
            && i.identifier != "water_pitcher" && i.identifier != "wine_bottle" && !WaterHelper.IsEmpty(i) && !Production.HeldForWine(i);

        // At bedtime, after the water engine's chores: each empty slot gets the fullest vessel whose true grade
        // reaches it, Pure first (it pays most), then HQ, then Base with what is left.
        public static void Evening(IExecutorHost ep)
        {
            var m = Machine();
            if (m == null) return;
            var slot = MachineHelper.GetBatterySlot(m);
            if (slot?.childItem == null || !PowerHelper.CanDrawPowerSource(slot.childItem, MachineryHelper.GetMachinePowerUsage(m))) return;
            int att = Attract(m);
            for (int g = 2; g >= 0; g--)
            {
                if (g == 2 && att < 20) continue;
                var s = Slot(m, g);
                if (s == null || s.childItem != null) continue;
                var pick = Owned().Where(i => Pourable(i) && Purity(i) >= Floor[g] && (g == 2 || Purity(i) < Floor[g + 1] || !Wanted(m, g + 1)))
                    .OrderByDescending(WaterHelper.GetCurrentCapacityML).ThenBy(i => i.uniqueId).FirstOrDefault();
                if (pick == null) continue;
                if (Chore.Accept(s, pick, -1) == 0 && Parent(pick) == m) Filled++;
            }
        }

        // A better slot still empty tonight takes the better water first.
        static bool Wanted(GameItem m, int g) => (g < 2 || Attract(m) >= 20) && Slot(m, g)?.childItem == null;

        public static JObject Status()
        {
            if (!On) return null;
            var m = Machine();
            return new JObject
            {
                ["owned"] = m != null,
                ["income_total"] = Income,
                ["attractiveness"] = m == null ? 0 : Attract(m),
                ["attract_upgrades"] = AttractUpgrades,
                ["vessels_filled"] = Filled,
            };
        }
    }
}
