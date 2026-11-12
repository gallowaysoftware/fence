using System;
using System.Collections.Generic;
using System.Linq;
using Newtonsoft.Json.Linq;

namespace Fence.Gym
{
    // `breadth` (gym, off unless set): stock what the shop's buyers ask for. The player remembers each buyer's
    // visible interests (the tablet's tags) and budget; the `top` types by budget over the last `window` days
    // (default 6, 14) are kept at `k` sellable units each (default 2). (`queue` is gone: the queue's buyers are
    // hidden until they reach the counter.) A seller's goods of a short type are
    // staged on their own (not the whole lot) and bought at up to `max_pct` of their estimate (default 95)
    // while the staged price is at most `lot_max` (default 600); the whole lot still goes wholesale when the
    // usual limits allow it.
    internal static class Breadth
    {
        static readonly List<(int day, string[] tags, int budget)> seen = new List<(int, string[], int)>();

        public static void Reset() => seen.Clear();

        static JObject Cfg => Directives.Current.Raw["breadth"] as JObject;
        public static bool On => Cfg != null;
        static int Top => Cfg?.Value<int?>("top") ?? 6;
        static int K => Cfg?.Value<int?>("k") ?? 2;
        static int Window => Cfg?.Value<int?>("window") ?? 14;
        public static double MaxPct => (Cfg?.Value<int?>("max_pct") ?? 95) / 100.0;
        public static int LotMax => Cfg?.Value<int?>("lot_max") ?? 600;

        // Types never stocked on purpose: the catch-alls, and goods the executor does not buy anyway.
        static readonly HashSet<string> Skip = new HashSet<string>(StringComparer.Ordinal) { "ALL_TYPE", "ALL_TYPE_EXCEPT_CONTRABAND", "CONTRABAND", "RESTRICTED_1", "RESTRICTED_2", "RESTRICTED_3", "ACCESS_CARD" };

        static string[] Visible(StoreClient c) => (c.clientBuyingTagList ?? new List<string>()).Where(t => TypeHelper.ALL_TYPES_LIST.Contains(t) && !Skip.Contains(t)).Distinct().ToArray();

        public static void Saw(StoreClient c, int day)
        {
            if (!On || c == null || !c.IsClientBuying()) return;
            var tags = Visible(c);
            if (tags.Length > 0) seen.Add((day, tags, c.useClientBudget ? c.GetBudget() : 300));
        }

        static bool Sellable(GameItem i) => !Production.Reserved(i) && !Directives.Current.HoldUids.Contains(i.uniqueId) && !(PlayerDay.Current?.Reserved(i) ?? false)
            && !StolenHelper.IsStolenItem(i) && ContrabandHelper.GetContrabandLevelInInt(i) == 0 && !PlayerRole.IsSign(i);

        public static int Have(string tag)
        {
            var e = EmporiumEntry.Instance;
            return e.invElement.childItems.Concat(e.showcaseElement.childItems)
                .Count(i => i != null && GeneralHelper.IsItemOwned(i) && i.IsTag(tag) && Sellable(i));
        }

        // Each short type with the units still wanted.
        public static Dictionary<string, int> Short(int day)
        {
            var need = new Dictionary<string, int>(StringComparer.Ordinal);
            if (!On) return need;
            seen.RemoveAll(x => x.day < day - Window);
            var score = new Dictionary<string, double>(StringComparer.Ordinal);
            foreach (var (_, tags, budget) in seen)
                foreach (var t in tags)
                    score[t] = (score.TryGetValue(t, out double v) ? v : 0) + budget / (double)tags.Length;
            foreach (var t in score.OrderByDescending(kv => kv.Value).ThenBy(kv => kv.Key, StringComparer.Ordinal).Take(Top).Select(kv => kv.Key))
                need[t] = K;
            foreach (var t in need.Keys.ToList())
            {
                int left = need[t] - Have(t);
                if (left > 0) need[t] = left;
                else need.Remove(t);
            }
            return need;
        }

        // The seller's goods of short types, most valuable per cell first, up to the units each type still wants.
        public static List<GameItem> Pick(IList<GameItem> lot, int day)
        {
            var need = Short(day);
            var picked = new List<GameItem>();
            if (need.Count == 0) return picked;
            foreach (var i in lot.Where(i => ContrabandHelper.GetContrabandLevelInInt(i) == 0 && !StolenHelper.IsStolenItem(i))
                .OrderByDescending(i => PlayerRole.Est(i) / (double)PlayerRole.Cells(i)).ThenBy(i => i.uniqueId))
            {
                var t = need.Keys.Where(i.IsTag).OrderBy(x => x, StringComparer.Ordinal).FirstOrDefault(x => need[x] > 0);
                if (t == null) continue;
                picked.Add(i);
                foreach (var x in need.Keys.Where(i.IsTag).ToList()) need[x]--;
            }
            return picked;
        }

        public static bool Covers(IEnumerable<GameItem> staged, int day)
        {
            var need = Short(day);
            return need.Count > 0 && staged.All(i => need.Keys.Any(i.IsTag));
        }
    }

    // `storage_bays: true` (gym): owned storage bays (a 10 x 10 or 17 x 10 grid inside, ShipSystemDirectory) are kept
    // as fixtures, not stock; each morning ordinary stock goes into them, most valuable per cell first (the cheap goods stay out for the
    // vending machine), the case still draws on them, and goods in
    // them are staged for buyers straight from the bay, as a player drags them out of its open window.
    internal static class Storage
    {
        public static int Stored;

        public static void Reset() => Stored = 0;

        public static bool On => Directives.Current.Raw.Value<bool?>("storage_bays") ?? false;

        public static bool IsBay(GameItem i) => (i.identifier ?? "").StartsWith("storage_bay", StringComparison.Ordinal) && i.contentWindow != null;

        static List<GameItem> Bays() => EmporiumEntry.Instance.invElement.childItems.Where(i => i != null && GeneralHelper.IsItemOwned(i) && IsBay(i)).ToList();

        public static IEnumerable<GameItem> Contents() => On
            ? Bays().SelectMany(b => GraphUtils.FindChildrenType<GameInventory>(b)).SelectMany(inv => inv.childItems).Where(i => i != null && GeneralHelper.IsItemOwned(i))
            : Enumerable.Empty<GameItem>();

        public static bool InBay(GameItem i) => i.parentInventory?.GetParentItem() is GameItem p && IsBay(p);

        public static void Morning()
        {
            if (!On) return;
            var bays = Bays();
            if (bays.Count == 0) return;
            var stock = EmporiumEntry.Instance.invElement;
            var p = PlayerDay.Current;
            var move = stock.childItems.Where(i => i != null && GeneralHelper.IsItemOwned(i) && !IsBay(i) && i.contentWindow == null && !PlayerRole.IsSign(i)
                    && !Production.Reserved(i) && !Production.IsInput(i.identifier) && !(p?.Reserved(i) ?? false) && !StolenHelper.IsStolenItem(i) && ContrabandHelper.GetContrabandLevelInInt(i) == 0
                    && !i.IsGameItemType("MACHINE") && !Directives.Current.HoldUids.Contains(i.uniqueId))
                .OrderByDescending(i => PlayerRole.Est(i) / (double)PlayerRole.Cells(i)).ThenBy(i => i.uniqueId).ToList();
            foreach (var i in move)
                foreach (var inv in bays.SelectMany(b => GraphUtils.FindChildrenType<GameInventory>(b)))
                    if (Chore.Accept(inv, i, -1) == 0 && InBay(i)) { Stored++; break; }
        }
    }
}
