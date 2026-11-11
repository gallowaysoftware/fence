using System;
using System.Collections.Generic;
using System.Linq;
using Newtonsoft.Json.Linq;

namespace Fence.Gym
{
    // The vending machine (Wild's VENDING_MACHINE, 1 200, or the `vending` perk; VendingMachineHelper): each
    // night it sells some of what it holds at the full value plus the retail markup and +10 %, with no
    // customer. Chance per item and night = attractiveness × type factor × 200 / (200 + price) %; only the
    // first copy of an identifier is always eligible (each further copy at half the odds of the one before);
    // at least 1–3 items of value ≤ 50 sell anyway. Attractiveness starts at 15 (+5 per common electronic,
    // max 100), the grid at 8×2 (+1 row per metal ingot, max 12). The counter only sells from the stock
    // grid and the case, so stock goes in at bedtime and what did not sell comes back each morning.
    internal static class Vending
    {
        public static int Stocked, AttractUpgrades, CapacityUpgrades;
        public static long Income;

        public static void Reset() { Stocked = AttractUpgrades = CapacityUpgrades = 0; Income = 0; full = false; }

        // `vending` (gym): "buy" buys Wild's machine with cash once the shop can spare it (`vending_day`, default
        // 1; `vending_reserve`, default 500, kept over the rent and floor); "off" leaves a machine empty;
        // unset stocks one if owned (the perk) and never buys one.
        static string Mode => Directives.Current.Raw.Value<string>("vending");
        public static bool Buys => Mode == "buy";
        public static int BuyDay => Directives.Current.Raw.Value<int?>("vending_day") ?? 1;
        public static int Reserve => Directives.Current.Raw.Value<int?>("vending_reserve") ?? 500;
        // `vending_upgrades` (default true): common electronics into attractiveness, ingots into rows.
        static bool Upgrades => Directives.Current.Raw.Value<bool?>("vending_upgrades") ?? true;

        static IEnumerable<GameItem> Owned() => EmporiumEntry.Instance.GetAllOwnedItems(false, true).Where(i => i != null);
        static GameItem Parent(GameItem i) => i.parentInventory?.GetParentItem();

        public static GameItem Machine() => EmporiumEntry.Instance == null || Mode == "off" || !PlayerStore.Instance.isVendingMachineUnlocked ? null
            : GraphUtils.FindAllChildrenType<GameItem>(EmporiumEntry.Instance.vendingMachineElement).FirstOrDefault(i => i.identifier == "vending_machine");

        static GameItem Core(GameItem m) => GraphUtils.FindAllChildrenType<GameItem>(m).FirstOrDefault(i => i.identifier == "vending_machine_core");
        static int Attract(GameItem m) => m.GetTagReadonly("VENDING_ATTRACTIVENESS").GetInt();

        public static void Morning(IExecutorHost ep)
        {
            var m = Machine();
            if (m == null) return;
            int cash = m.GetTagReadonly("VENDING_CASH").GetInt();
            if (cash > 0)
            {
                Chore.Act("vending_collect", m, () => VendingMachineHelper.TransferCashToPlayer(m));
                Income += cash;
                ep.Human("vending", null, null, new JObject { ["action"] = "collect", ["cash"] = cash }, "player:vending");
            }
            var stock = EmporiumEntry.Instance.invElement;
            foreach (var i in VendingMachineHelper.GetVendingInventory(m).childItems.Where(x => x != null).ToList())
                Chore.Accept(stock, i, -1);
            Power(m);
            if (Upgrades && !(PlayerDay.Current?.Cuts("vending") ?? false)) Upgrade(m);
        }

        static void Power(GameItem m)
        {
            var slot = MachineHelper.GetBatterySlot(m);
            if (slot == null) return;
            int need = 2 * MachineryHelper.GetMachinePowerUsage(m);
            int have = slot.childItem == null ? 0 : PowerHelper.GetAvailableEnergyFromItem(slot.childItem, false);
            if (have >= need) return;
            // The weakest cell that still runs it a few nights: the bays and furnace want the full ones.
            var cell = Owned().Where(i => i.identifier == "energy_credit" && Parent(i) == null && PowerHelper.GetAvailableEnergyFromItem(i, false) > Math.Max(have, need))
                .OrderBy(i => PowerHelper.GetAvailableEnergyFromItem(i, false)).ThenBy(i => i.uniqueId).FirstOrDefault();
            if (cell == null) return;
            var stock = EmporiumEntry.Instance.invElement;
            if (slot.childItem != null) Chore.Accept(stock, slot.childItem, -1);
            Chore.Accept(slot, cell, -1);
        }

        static bool Spare(GameItem i) => Parent(i) == null && !StolenHelper.IsStolenItem(i) && !ContrabandHelper.IsContraband(i) && !Labels.Upgraded(i)
            && !Production.Reserved(i) && !Production.IsInput(i.identifier) && !PlayerRole.Keep(i, null);

        static void Upgrade(GameItem m)
        {
            var core = Core(m);
            if (core == null) return;
            foreach (var e in Owned().Where(i => i.identifier == "common_electronic" && Spare(i)).OrderBy(i => i.uniqueId).ToList())
            {
                if (!VendingMachineHelper.CanUpgradedAttractiveness(m)) break;
                Chore.Act("vending_upgrade", core, () => VendingMachineHelper.UpgradeAttractiveness(core, e));
                AttractUpgrades++;
            }
            // A row costs an ingot (50–150 by purity): only while last night's grid was full.
            if (full && VendingMachineHelper.CanUpgradedCapacity(m))
            {
                var ingot = Owned().Where(i => i.identifier == "metal_ingot" && Spare(i)).OrderBy(PlayerRole.Est).ThenBy(i => i.uniqueId).FirstOrDefault();
                if (ingot != null)
                {
                    Chore.Act("vending_upgrade", core, () => VendingMachineHelper.UpgradeCapacity(core, ingot));
                    CapacityUpgrades++;
                }
            }
        }

        static bool full;

        static bool Eligible(GameItem i)
        {
            if (!ReferenceEquals(i.parentInventory, EmporiumEntry.Instance.invElement) || !GeneralHelper.IsItemOwned(i)) return false;
            if (PlayerRole.HidesFake(i)) return false;
            if (i.IsTag("VENDING_BLACKLISTED_TAG") || i.IsTag("CONTAINER_TAG") || i.IsGameItemType("MACHINE") || i.IsGameItemType("MODULE")) return false;
            if (i.IsTag("LIQUID_CONTAINER_TAG") || i.identifier == "wine_bottle" || i.identifier == "energy_credit" || i.identifier == "common_electronic") return false;
            return VendingMachineHelper.GetVendingPrice(i) > 0 && Spare(i);
        }

        // At bedtime: the spare stock with the best expected sale a night per cell, one of each kind first.
        public static void Evening(IExecutorHost ep)
        {
            var m = Machine();
            if (m == null) return;
            var slot = MachineHelper.GetBatterySlot(m);
            if (slot?.childItem == null || !PowerHelper.CanDrawPowerSource(slot.childItem, MachineryHelper.GetMachinePowerUsage(m))) return;
            var grid = VendingMachineHelper.GetVendingInventory(m);
            int att = Attract(m);
            double Ev(GameItem i)
            {
                int p = VendingMachineHelper.GetVendingPrice(i);
                return VendingMachineHelper.GetSaleChance(att, i, p) / 100.0 * p / PlayerRole.Cells(i);
            }
            var pool = EmporiumEntry.Instance.invElement.childItems.Where(i => i != null && Eligible(i)).ToList();
            var firsts = pool.GroupBy(i => i.identifier).Select(g => g.OrderByDescending(Ev).ThenBy(i => i.uniqueId).First()).ToList();
            var order = firsts.OrderByDescending(Ev).ThenBy(i => i.uniqueId)
                .Concat(pool.Except(firsts).OrderByDescending(i => Ev(i) * 0.5).ThenBy(i => i.uniqueId)).ToList();
            int put = 0;
            full = false;
            foreach (var i in order)
            {
                if (Chore.Accept(grid, i, -1) == 0 && ReferenceEquals(i.parentInventory, grid)) put++;
                else full = true;
            }
            Stocked += put;
            if (put > 0) ep.Human("vending", null, null, new JObject { ["action"] = "stock", ["items"] = put, ["attractiveness"] = att }, "player:vending");
        }

        public static JObject Status()
        {
            var m = Machine();
            if (m == null && Income == 0) return null;
            return new JObject
            {
                ["income_total"] = Income,
                ["stocked_total"] = Stocked,
                ["attractiveness"] = m == null ? 0 : Attract(m),
                ["rows"] = m == null ? 0 : m.GetTagReadonly("VENDING_CAPACITY").GetInt(),
                ["attract_upgrades"] = AttractUpgrades,
                ["capacity_upgrades"] = CapacityUpgrades,
            };
        }
    }
}
