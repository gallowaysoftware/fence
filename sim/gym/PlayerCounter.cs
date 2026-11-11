using System;
using System.Collections.Generic;
using System.Linq;

namespace Fence.Gym
{
    // The executor's counter-adjacent decisions (docs/STREAM.md refactor 2): the permit official's donations,
    // the items a buyer would expose, Dr. Jackson's shopping list. The drivers make the moves: the gym's
    // Episode headless, the autopilot's hands live.
    internal static class PlayerCounter
    {
        static IEnumerable<GameItem> In(GameInventory inv) => inv?.childItems?.Where(i => i != null && GeneralHelper.IsItemOwned(i)) ?? Enumerable.Empty<GameItem>();

        static IEnumerable<GameItem> StockAndCase()
        {
            var e = EmporiumEntry.Instance;
            return In(e.invElement).Concat(In(e.showcaseElement));
        }

        // Goods set aside for the permit official's next visit (from day 40 while bribing): the cheapest
        // items covering what is owed plus the week's maximum (95), so the shelf is never bare on his day.
        public static HashSet<int> BribeReserve(int day)
        {
            var ps = PlayerStore.Instance;
            var held = new HashSet<int>();
            if (ps.hasBusinessPermit || day < 40 || Directives.Current.Permit == "buy" || Directives.Current.Permit == "refuse") return held;
            int need = Math.Max(0, ps.storeClientData.businessPermitBribeAmount) + 95;
            long sum = 0;
            foreach (var i in StockAndCase().Where(i => !Production.Reserved(i) && !StolenHelper.IsStolenItem(i) && ContrabandHelper.GetContrabandLevelInInt(i) == 0)
                         .OrderBy(i => i.GetNegociatedValue()).ThenBy(i => i.uniqueId))
            {
                if (sum >= need) break;
                if (i.GetNegociatedValue() <= 0) continue;
                held.Add(i.uniqueId);
                sum += i.GetNegociatedValue();
            }
            return held;
        }

        // From day 45 the permit official accepts goods marked as donations until the week's bribe is covered,
        // which spares the week's OPERATING_WITHOUT_LICENSE. He turns buyer on his last line; the player stages
        // goods worth the bribe and "sells" them (the Donate button) instead of answering.
        public static bool BribeDue(StoreClient c, int cash)
        {
            var ps = PlayerStore.Instance;
            if (c == null || c.identifier != "businessPermitOfficial") return false;
            if (c.clientIntent != StoreClient.ClientIntent.BUY || ps.storeClientData.businessPermitBribeAmount <= 0) return false;
            return PlayerRole.PermitMode(cash, ps.GetRentValue()) == "bribe";
        }

        // The goods to donate, or null when there are too few and the permit is affordable instead (the
        // shortfall is taken as `player:bribe_short`). `need` is what is owed, `sum` the picks' donation credit.
        public static List<GameItem> BribePicks(StoreClient c, int cash, Defaults defaults, out int need, out long sum)
        {
            var ps = PlayerStore.Instance;
            need = ps.storeClientData.businessPermitBribeAmount;
            // Cheapest goods first that the official takes, never an engine's input or a held item.
            var pool = StockAndCase()
                .Where(i => !Production.Reserved(i) && !Directives.Current.HoldUids.Contains(i.uniqueId) && !StolenHelper.IsStolenItem(i) && ContrabandHelper.GetContrabandLevelInInt(i) == 0 && c.IsClientBuyingThisItem(i, false) && !c.CanClientExposeAnyFeatureDeep(i))
                // The official checks what he is given on placement (TradeBoardExposeCapacity): each fake he
                // finds costs Lower Levels rep (-5 on hard). Own goods (wine, berries) carry no fakes; they go first.
                .OrderBy(i => i.identifier == "wine_bottle" || i.identifier == "wine_berry" ? 0 : 1).ThenBy(i => i.GetDonationCreditValue()).ThenBy(i => i.uniqueId).ToList();
            var picks = new List<GameItem>();
            sum = 0;
            // Over his budget the sale needs the confirm (a price cut, which costs Lower Levels rep): aim at the
            // debt itself; a short first round is topped up by the second.
            long aim = need;
            foreach (var i in pool)
            {
                if (sum >= aim) break;
                picks.Add(i);
                sum += i.GetDonationCreditValue();
            }
            // Short of goods: donate what there is anyway (it pays the debt down), unless the permit is affordable.
            PlayerRole.BribeFailing = sum < need;
            if (sum < need && (picks.Count == 0 || cash >= PlayerStore.GetBusinessPermitPrice() + ps.GetRentValue()))
            {
                var all = StockAndCase().ToList();
                defaults.Take("player:bribe_short", sum + " of " + need + $" (owned {all.Count}, not kept {all.Count(i => !PlayerRole.Keep(i, c))}, wanted {all.Count(i => c.IsClientBuyingThisItem(i, false))})");
                return null;
            }
            // Drop the overshoot from the cheap end while the aim is still covered.
            foreach (var i in picks.ToList())
                if (sum - i.GetDonationCreditValue() >= aim) { picks.Remove(i); sum -= i.GetDonationCreditValue(); }
            return picks;
        }

        // The buyer re-inspects each item before a sale (TryReinspectBeforeSale) and exposes what he can: -5
        // rep a find on hard, sale or not. Some fakes only become exposable once on the counter. Reads what the
        // buyer could expose (a hidden value; the feed marks it).
        public static List<GameItem> Exposable(StoreClient c) =>
            In(EmporiumEntry.Instance.backInvinvElement).Where(i => GeneralHelper.IsItemOwned(i) && c.CanClientExposeAnyFeatureDeep(i)).ToList();

        // Dr. Jackson's goods worth buying tonight, in the order they are staged: `stage` moves one to his back
        // counter and says whether it went (headless the move itself; live a queued move, assumed to go).
        public static List<GameItem> JacksonStage(IList<GameItem> offered, long cash, Func<GameItem, bool> stage)
        {
            var staged = new Dictionary<string, int>(StringComparer.Ordinal);
            long floor = (PlayerDay.Current?.Floor ?? 0) + PlayerStore.Instance.GetRentValue();
            var moved = new List<GameItem>();
            foreach (var i in offered.OrderBy(i => i.uniqueId))
            {
                staged.TryGetValue(i.identifier, out int n);
                if (!PlayerRole.JacksonWants(i, n)) continue;
                long price = i.GetNegociatedValue();
                if (cash - price < floor) continue;
                if (stage(i)) { moved.Add(i); staged[i.identifier] = n + 1; cash -= price; }
            }
            return moved;
        }
    }
}
