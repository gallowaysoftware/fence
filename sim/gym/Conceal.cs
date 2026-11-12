using System;
using System.Collections.Generic;
using System.Linq;
using Newtonsoft.Json.Linq;

namespace Fence.Gym
{
    // Hot stock out of an inspector's reach (docs/re/CONCEALMENT.md): smuggler's bays (never searched,
    // burgled or repossessed) and the poster stash (2x2, searched only while the poster is pulled up).
    // Each morning, before any inspector can arrive, flaggable goods go into the largest bay with room,
    // then the stash; goods that have cooled to heat 0 come back out to be sold.
    internal static class Conceal
    {
        public static int Stashed, Released, NotFitted;

        public static void Reset() => Stashed = Released = NotFitted = 0;

        static IEnumerable<GameItem> Owned() => EmporiumEntry.Instance.GetAllOwnedItems(false, true).Where(i => i != null);

        public static List<GameItem> Bays() => Owned().Where(i => ContrabandHelper.IsSmugglerBay(i) && i.parentInventory?.GetParentItem() == null).ToList();

        static bool Hot(GameItem i) => StolenHelper.IsStolenItem(i) || ContrabandHelper.GetContrabandLevelInInt(i) > 0;

        static bool InBay(GameItem i)
        {
            for (var p = i.parentInventory?.GetParentItem(); p != null; p = p.parentInventory?.GetParentItem())
                if (ContrabandHelper.IsSmugglerBay(p)) return true;
            return false;
        }

        static bool Into(GameItem bay, GameItem item)
        {
            foreach (var inv in GraphUtils.FindChildrenType<GameInventory>(bay))
                if (Chore.Accept(inv, item, -1) == 0 && InBay(item)) return true;
            return false;
        }

        public static int FreeCells()
        {
            int free = 0;
            foreach (var b in Bays())
                foreach (var inv in GraphUtils.FindChildrenType<GameGridInventory>(b))
                    free += inv.inventoryShape.width * inv.inventoryShape.height - inv.childItems.Where(x => x != null).Sum(PlayerRole.Cells);
            return free;
        }

        // The plan's `conceal` object (docs/re/CONCEALMENT.md §6): policy always | on_warning | never (on_warning
        // acts every morning too: the morning pass runs before any inspector of the day), min_level,
        // max_exposed_heat, use_poster.
        static JObject Plan => Directives.Current.Raw["conceal"] as JObject ?? new JObject();

        static bool Flaggable(GameItem i)
        {
            if (Lab.Shelf(i)) return false;
            int level = ContrabandHelper.GetContrabandLevelInInt(i);
            if (level > 0) return level >= (Plan.Value<int?>("min_level") ?? 1);
            return StolenHelper.IsStolenItem(i) && PlayerRole.Heat(i) > (Plan.Value<int?>("max_exposed_heat") ?? 0);
        }

        static GameGridInventory Stash => EmporiumEntry.Instance.hiddenElement;

        static bool InStash(GameItem i) => ReferenceEquals(i.parentInventory, Stash);

        public static void Morning(IExecutorHost ep)
        {
            if ((Plan.Value<string>("policy") ?? "always") == "never") return;
            var bays = Bays();
            bool poster = (Plan.Value<bool?>("use_poster") ?? true) && Stash != null;
            if (bays.Count == 0 && !poster) return;
            var stock = EmporiumEntry.Instance.invElement;
            bool sellContraband = Directives.Current.Raw["sell"]?.Value<bool?>("sell_contraband") ?? false;
            // The poster stash is searched only while the poster is pulled up: keep it down.
            if (poster && StorePoster.Instance != null && StorePoster.Instance.isPulled) StorePoster.Instance.isPulled = false;
            foreach (var i in (Stash?.childItems ?? new List<GameItem>()).Where(x => x != null && !Flaggable(x) && (ContrabandHelper.GetContrabandLevelInInt(x) == 0 || sellContraband)).ToList())
                if (Chore.Accept(stock, i, -1) == 0) Released++;
            // Cooled goods out first (they make room), then the hottest in.
            foreach (var i in Owned().Where(InBay).Where(i => !StolenHelper.IsStolenItem(i) && (ContrabandHelper.GetContrabandLevelInInt(i) == 0 || sellContraband || Lab.Shelf(i))).ToList())
                if (Chore.Accept(stock, i, -1) == 0)
                {
                    Released++;
                    ep.Human("conceal", null, ep.Uid(i), new JObject { ["action"] = "unstash" }, "player:bay");
                }
            foreach (var i in Owned().Where(i => Flaggable(i) && !InBay(i) && !InStash(i) && !ContrabandHelper.IsSmugglerBay(i) && i.parentInventory?.GetParentItem() == null)
                         .OrderByDescending(i => ContrabandHelper.GetContrabandLevelInInt(i)).ThenByDescending(PlayerRole.Heat).ThenByDescending(PlayerRole.Est).ToList())
            {
                string into = null;
                foreach (var b in bays.OrderByDescending(b => GraphUtils.FindChildrenType<GameGridInventory>(b).Sum(g => g.inventoryShape.width * g.inventoryShape.height)))
                    if (Into(b, i)) { into = "bay:" + b.uniqueId; break; }
                if (into == null && poster && Chore.Accept(Stash, i, -1) == 0 && InStash(i)) into = "poster";
                if (into == null) { NotFitted++; continue; }
                Stashed++;
                ep.Human("conceal", null, ep.Uid(i), new JObject { ["action"] = "stash", ["into"] = into, ["heat"] = PlayerRole.Heat(i), ["contraband_level"] = ContrabandHelper.GetContrabandLevelInInt(i) }, "player:conceal");
            }
        }

        // What an inspection would take of the goods not concealed (contraband always; stolen at heat %).
        public static double ExposedLoss() => Owned().Where(i => Hot(i) && !InBay(i) && !InStash(i))
            .Sum(i => PlayerRole.Est(i) * (ContrabandHelper.GetContrabandLevelInInt(i) > 0 ? 1.0 : PlayerRole.Heat(i) / 100.0));

        public static JObject Status()
        {
            var bays = Bays();
            var owned = Owned().ToList();
            if (bays.Count == 0 && !owned.Any(Hot)) return null;
            return new JObject
            {
                ["poster_stash"] = new JArray((Stash?.childItems ?? new List<GameItem>()).Where(x => x != null).Select(x => x.uniqueId)),
                ["poster_pulled"] = StorePoster.Instance?.isPulled ?? false,
                ["not_fitted_total"] = NotFitted,
                ["bays"] = new JArray(bays.Select(b => b.identifier)),
                ["free_cells"] = FreeCells(),
                ["stashed_now"] = owned.Count(i => InBay(i) || InStash(i)),
                ["hot_outside"] = owned.Count(i => Hot(i) && !InBay(i) && !InStash(i)),
                ["stashed_total"] = Stashed,
                ["released_total"] = Released,
            };
        }
    }
}
