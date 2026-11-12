using System;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using Newtonsoft.Json;
using Newtonsoft.Json.Linq;

namespace Fence.Gym
{
    // `FENCE_DEMAND_TRACE=DIR`: one JSON line per buyer and seller (and one per day) in DIR/s<seed>.jsonl, for the
    // stock-breadth analysis. Read-only: it never changes a decision.
    internal static class DemandTrace
    {
        static StreamWriter w;
        static JObject cur;
        public static bool On => w != null;

        public static void Open(int seed)
        {
            string dir = Environment.GetEnvironmentVariable("FENCE_DEMAND_TRACE");
            if (string.IsNullOrEmpty(dir) || w != null) return;
            System.IO.Directory.CreateDirectory(dir);
            w = new StreamWriter(Path.Combine(dir, "s" + seed + ".jsonl")) { AutoFlush = false };
        }

        public static void Close()
        {
            Flush();
            w?.Dispose();
            w = null;
        }

        static void Write(JObject o) => w?.WriteLine(o.ToString(Formatting.None));

        static JArray Types(GameItem i) => new JArray(i.GetGameItemType().ToArray());

        static JObject Item(GameItem i) => new JObject { ["id"] = i.identifier, ["t"] = Types(i), ["est"] = PlayerRole.Est(i) };

        public static void Buyer(StoreClient c, IList<GameItem> owned, IList<GameItem> picked, long room)
        {
            if (!On) return;
            Flush();
            var known = TypeHelper.ALL_TYPES_LIST;
            var tags = (c.clientBuyingTagList ?? new List<string>()).Where(known.Contains).ToList();
            var why = new JObject();
            var blocked = new JArray();
            int wanted = 0, overRoom = 0;
            foreach (var i in owned.Where(i => c.IsClientBuyingThisItem(i, false)))
            {
                wanted++;
                string k = PlayerRole.KeepWhy(i, c);
                if (k == null && i.GetNegociatedValue() > room) k = "over_room";
                if (k == null) continue;
                why[k] = (why.Value<int?>(k) ?? 0) + 1;
                if (blocked.Count < 8) blocked.Add(i.identifier + ":" + k + (k == "exposable" ? ":" + string.Join(",", i.itemFeatures.Where(f => !f.isDisabled && !f.IsExposed() && f.useCondition).Select(f => f.identifier)) : ""));
                if (k == "over_room") overRoom++;
            }
            var top = new HashSet<GameItem>(owned);
            var nested = EmporiumEntry.Instance.GetAllOwnedItems(false, true).Where(i => i != null && !top.Contains(i) && i.parentInventory?.GetParentItem() != null && c.IsClientBuyingThisItem(i, false)).ToList();
            cur = new JObject
            {
                ["k"] = "buyer", ["day"] = PlayerDay.Current?.Day ?? 0, ["id"] = c.identifier, ["faction"] = c.clientFaction,
                ["budget"] = c.useClientBudget ? c.GetBudget() : -1, ["tags"] = new JArray(tags.ToArray()),
                ["ids"] = new JArray((c.clientBuyingIdList ?? new List<string>()).ToArray()), ["all"] = (c.clientBuyingTagList ?? new List<string>()).Any(t => t.StartsWith("ALL_TYPE")),
                ["wanted"] = wanted, ["why"] = why, ["blocked"] = blocked, ["picked"] = picked.Count, ["picked_value"] = picked.Sum(i => (long)i.GetNegociatedValue()),
                ["nested"] = nested.Count, ["nested_parents"] = new JArray(nested.Select(i => i.parentInventory.GetParentItem().identifier).Distinct().ToArray()),
                ["sold"] = 0, ["sold_items"] = 0,
            };
        }

        public static void Seller(StoreClient c, IList<GameItem> theirs)
        {
            if (!On) return;
            Flush();
            cur = new JObject
            {
                ["k"] = "seller", ["day"] = PlayerDay.Current?.Day ?? 0, ["id"] = c?.identifier, ["wholesale"] = c?.isOfferWholesale ?? false,
                ["items"] = new JArray(theirs.Select(Item).ToArray()), ["bought"] = new JArray(),
            };
        }

        // What each traded item cost the shop and when, by uid: a sale's margin and holding time (absent: made,
        // scavenged or a gift).
        static readonly Dictionary<int, (int paid, int day)> basis = new Dictionary<int, (int, int)>();

        public static void Trade(bool sold, GameItem item, int amount)
        {
            if (!On) return;
            int day = PlayerDay.Current?.Day ?? 0;
            if (!sold) basis[item.uniqueId] = (amount, day);
            if (cur == null) return;
            if (sold && (string)cur["k"] == "buyer")
            {
                cur["sold"] = (int)cur["sold"] + amount;
                cur["sold_items"] = (int)cur["sold_items"] + 1;
                var o = Item(item);
                o["got"] = amount;
                o["base"] = (int)item.GetCurrentValue(false, true, false, false, 0L);
                if (item.itemFeatures.Any(f => f != null && f.identifier == "tips")) o["tips"] = true;
                if (basis.TryGetValue(item.uniqueId, out var b)) { o["paid"] = b.paid; o["held"] = day - b.day; }
                var list = cur["items"] as JArray ?? new JArray();
                list.Add(o);
                cur["items"] = list;
            }
            else if (!sold && (string)cur["k"] == "seller")
            {
                var o = Item(item);
                o["paid"] = amount;
                if (StolenHelper.IsStolenItem(item)) o["hot"] = PlayerRole.Heat(item);
                ((JArray)cur["bought"]).Add(o);
            }
        }

        public static void Why(string why)
        {
            if (cur == null || why == null) return;
            cur["choice"] = why.Length > 80 ? why.Substring(0, 80) : why;
            var all = cur["choices"] as JArray ?? new JArray();
            all.Add(why.Split(' ')[0]);
            cur["choices"] = all;
        }

        public static void Flush()
        {
            if (cur != null) Write(cur);
            cur = null;
        }

        // The stock at the end of a day by type: units and value at estimate (trading stock only).
        public static void Day(int day)
        {
            if (!On) return;
            Flush();
            var by = new JObject();
            int units = 0, worth = 0;
            foreach (var i in EmporiumEntry.Instance.GetAllOwnedItems(false, true).Where(i => i != null && !Production.Reserved(i)))
            {
                units++;
                worth += PlayerRole.Est(i);
                foreach (var t in i.GetGameItemType().Distinct())
                {
                    var a = by[t] as JArray ?? new JArray(0, 0);
                    by[t] = new JArray((int)a[0] + 1, (int)a[1] + PlayerRole.Est(i));
                }
            }
            var events = new JArray();
            try
            {
                foreach (var ev in StoreStation.Instance.storeEventManager.GetActiveEventOnDay(day, true, true))
                    foreach (var n in ev.negociationDatas ?? new List<NegociationData>())
                        if (n?.itemType != null) events.Add(n.itemType + ":" + n.value);
            }
            catch (Exception) { }
            Write(new JObject { ["k"] = "day", ["day"] = day, ["stock"] = by, ["cash"] = PlayerStore.Instance.playerCash, ["units"] = units, ["worth"] = worth, ["events"] = events });
            w.Flush();
        }
    }
}
