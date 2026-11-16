using System.Linq;
using Newtonsoft.Json.Linq;

namespace Fence.Gym
{
    // `fridge: true` (gym; docs/re/RECIPES.md §8): Dr. Jackson's fridge (100) chills what spends a powered night in it,
    // +20 % on drinks, rations, premium water and nudka, kept through the next day (2 power a night). Bought from him
    // when offered; each evening the most valuable chillable stock goes into the chill zone, each morning it comes
    // back out to be sold.
    internal static class Fridge
    {
        public static int Chilled;

        static bool On => Directives.Current.Raw.Value<bool?>("fridge") ?? false;

        static System.Collections.Generic.IEnumerable<GameItem> Owned() => EmporiumEntry.Instance.GetAllOwnedItems(false, true).Where(i => i != null);

        static GameItem Machine() => Owned().FirstOrDefault(i => i.IsTag("FRIDGE_TAG"));

        public static bool Wants(string id) => On && id == "fridge" && Machine() == null;

        public static bool Reserved(GameItem i) => On && i.IsTag("FRIDGE_TAG");

        static bool Chillable(GameItem i) => MachineFridge.IsChillable(i) || MachineFridge.IsSpirit(i);

        public static void Morning(IExecutorHost host)
        {
            var m = On ? Machine() : null;
            var zone = m == null ? null : MachineFridge.GetChillZone(m);
            if (zone == null) return;
            var stock = EmporiumEntry.Instance.invElement;
            foreach (var i in zone.childItems.Where(x => x != null).ToList()) Chore.Accept(stock, i, -1);
            var slot = MachineHelper.GetBatterySlot(m);
            int have = slot?.childItem == null ? 0 : PowerHelper.GetAvailableEnergyFromItem(slot.childItem, false);
            if (slot == null || have >= 2 * MachineryHelper.GetMachinePowerUsage(m)) return;
            var cell = Owned().Where(i => i.identifier == "energy_credit" && i.parentInventory?.GetParentItem() == null && PowerHelper.GetAvailableEnergyFromItem(i, false) > have)
                .OrderBy(i => PowerHelper.GetAvailableEnergyFromItem(i, false)).ThenBy(i => i.uniqueId).FirstOrDefault();
            if (cell == null) return;
            if (slot.childItem != null) Chore.Accept(stock, slot.childItem, -1);
            Chore.Accept(slot, cell, -1);
        }

        public static void Evening(IExecutorHost host)
        {
            var m = On ? Machine() : null;
            var slot = m == null ? null : MachineHelper.GetBatterySlot(m);
            if (slot?.childItem == null || !PowerHelper.CanDrawPowerSource(slot.childItem, MachineryHelper.GetMachinePowerUsage(m))) return;
            var zone = MachineFridge.GetChillZone(m);
            foreach (var i in EmporiumEntry.Instance.invElement.childItems.Where(i => i != null && Chillable(i) && !Production.Reserved(i) && !StolenHelper.IsStolenItem(i))
                         .OrderByDescending(PlayerRole.Est).ThenBy(i => i.uniqueId).ToList())
                if (Chore.Accept(zone, i, -1) == 0) Chilled++;
        }

        public static JObject Status() => Chilled == 0 ? null : new JObject { ["chilled"] = Chilled };
    }
}
