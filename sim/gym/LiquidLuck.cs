using System.Linq;
using Newtonsoft.Json.Linq;

namespace Fence.Gym
{
    // Georgio's liquid luck (docs/re/MITIGATIONS.md, Luck): +75 luck for three days a bottle, and one luck roll voids an
    // inspection's whole find (74 % at 75, docs/sim/VERIFIED.md). `liquid_luck`: "sell" (stock like any other) or "keep":
    // held, and drunk on a morning when the hot goods left outside the bays and the stash would cost at least
    // `liquid_luck_exposed` (default 150) at an inspection.
    internal static class LiquidLuck
    {
        public static int Drunk;

        static JObject Raw => Directives.Current.Raw;

        static bool Keep => Raw.Value<string>("liquid_luck") == "keep";

        public static bool Is(GameItem i) => i != null && i.IsTag("LIQUID_LUCK_TAG");

        public static bool Reserved(GameItem i) => Keep && Is(i);

        public static void Morning(IExecutorHost host)
        {
            var hd = PlayerStore.Instance.healthData;
            if (!Keep || hd == null || hd.liquidLuckDaysLeft > 0) return;
            if (Conceal.ExposedLoss() < (Raw.Value<int?>("liquid_luck_exposed") ?? 150)) return;
            var bottle = EmporiumEntry.Instance.GetAllOwnedItems(false, true).Where(i => Is(i) && i.parentInventory != null).OrderBy(i => i.uniqueId).FirstOrDefault();
            if (bottle == null) return;
            host.Human("drink", null, host.Uid(bottle), new JObject { ["exposed"] = (int)Conceal.ExposedLoss() }, "player:liquid_luck");
            bottle.onActivateSlotItemFunc?.Invoke(bottle, bottle.parentInventory, null);
            if (hd.liquidLuckDaysLeft > 0) Drunk++;
        }

        public static JObject Status() => Drunk == 0 ? null : new JObject { ["drunk"] = Drunk };
    }
}
