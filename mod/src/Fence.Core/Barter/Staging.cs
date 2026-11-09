using System;
using System.Collections.Generic;
using System.Globalization;
using System.Linq;
using Fence.Protocol;
using Newtonsoft.Json.Linq;

namespace Fence.Barter
{
    public static class StageWhere
    {
        public const string Stock = "stock";
        public const string Showcase = "showcase";
        // Inside a smuggler bay on a shelf: inspections do not search it.
        public const string SmugglerBay = "smuggler_bay";
    }

    // An owned item the player could put on the back counter for a buying customer.
    public sealed class ShopItem
    {
        // Price is the expected counter price for this customer: the game sets the final one once staged.
        // ClientWants is StoreClient.IsClientBuyingThisItem (interests, restrictions, no-contraband, variety).
        public ItemInfo Item;
        public string Where;
        // The customer's interest tags (game ids) this item carries.
        public List<string> Matches = new List<string>();
    }

    public sealed class StageCandidate
    {
        public ShopItem Shop;
        public bool Attracted;
        public bool OverBudget;
    }

    public static class StageOptionIds
    {
        public const string NothingFits = "nothing_fits";
        public const string OfferPrefix = "offer_";

        public static string Offer(int uid) => OfferPrefix + uid.ToString(CultureInfo.InvariantCulture);
    }

    // The stage decision (docs/PROTOCOL.md): a buying customer waits on the shop and nothing they want is
    // staged, or budget is left for more. The question is which one item to put on the back counter next.
    // Advice only: the mod never stages an item.
    public static class StageContext
    {
        public const string Kind = "stage";
        public const int MaxCandidates = 12;

        // True when the buy side is open: a buying customer (a sell_and_buy one once their own items are
        // gone), the tablet on the selling half, budget left after what they want is already staged.
        public static bool IsMoment(ClientInfo c, IList<ItemInfo> counter, UiState ui, out long? budgetLeft, out bool suitableStaged)
        {
            budgetLeft = null;
            suitableStaged = false;
            if (c == null || ui == null || counter == null || c.BusinessComplete || ui.TradeLocked) return false;
            bool buying = c.Intent == ClientIntent.Buy || c.Intent == ClientIntent.SellAndBuy && !counter.Any(i => !i.Owned);
            if (!buying || BarterRules.ResolveMode(c, counter) != TabletMode.ClientBuys) return false;
            var staged = counter.Where(i => i.Owned && i.OnBack && i.ClientWants).ToList();
            suitableStaged = staged.Count > 0;
            if (!c.UsesBudget) return true;
            budgetLeft = c.Budget - staged.Sum(i => i.Price);
            return budgetLeft > 0;
        }

        // The barter signature already covers the counter; the budget changes on its own (discounts, sales).
        public static string Signature(string barterSignature, long? budgetLeft) =>
            "s" + barterSignature + ":g" + (budgetLeft.HasValue ? budgetLeft.Value.ToString(CultureInfo.InvariantCulture) : "-");

        // Never offer what the customer refuses, nor stolen or contraband goods to security.
        public static bool Allowed(ClientInfo c, ItemInfo i)
        {
            if (!i.ClientWants || i.Price <= 0) return false;
            bool contraband = i.ContrabandLevel > 0 || i.SeriousContraband;
            if (c.IsSecurity && (contraband || i.StolenHeat.HasValue)) return false;
            if (c.NoContraband && contraband) return false;
            return true;
        }

        // Best fit first: what they came for, within budget, clean, matching an interest, then margin.
        public static List<StageCandidate> Rank(ClientInfo c, IEnumerable<ShopItem> shop, long? budgetLeft, out int total)
        {
            var all = (shop ?? Enumerable.Empty<ShopItem>())
                .Where(s => s?.Item != null && s.Item.Owned && !s.Item.OnBack && Allowed(c, s.Item))
                .Select(s => new StageCandidate
                {
                    Shop = s,
                    Attracted = !string.IsNullOrEmpty(c.AttractedItemId) && s.Item.Id == c.AttractedItemId,
                    OverBudget = budgetLeft.HasValue && s.Item.Price > budgetLeft.Value,
                })
                .OrderByDescending(x => x.Attracted)
                .ThenBy(x => x.OverBudget)
                .ThenBy(x => x.Shop.Item.StolenHeat.HasValue)
                .ThenByDescending(x => x.Shop.Matches.Count > 0)
                .ThenByDescending(x => Margin(x.Shop.Item))
                .ThenBy(x => x.Shop.Item.Uid)
                .ToList();
            total = all.Count;
            return all.Take(MaxCandidates).ToList();
        }

        // Against what was paid, or the estimate when that is unknown.
        static long Margin(ItemInfo i) => i.Price - (i.BoughtAt.HasValue && i.BoughtAt.Value > 0 ? i.BoughtAt.Value : i.EstimatedValue ?? i.Price);

        // Stage only when it can help: nothing they want is staged yet, or there is more to offer and the
        // bridge has not already said nothing more fits (or deferred) for this counter.
        public static bool Ask(bool suitableStaged, int candidates, Advice answered)
        {
            if (!suitableStaged) return true;
            if (candidates == 0) return false;
            return answered == null || !answered.Deferred && answered.OptionId != StageOptionIds.NothingFits;
        }

        // The stage question for the counter `barter` was read from. Context is the barter context without
        // the bargain block, plus the candidates; hidden adds the candidates' true values.
        public static BarterSnapshot Build(BarterSnapshot barter, IList<StageCandidate> candidates, int total, long? budgetLeft, bool suitableStaged)
        {
            var c = barter.Client;
            var context = (JObject)barter.Context.DeepClone();
            context.Remove("bargain");
            context["budget_left"] = budgetLeft.HasValue ? new JValue(budgetLeft.Value) : JValue.CreateNull();
            context["candidates"] = new JArray(candidates.Select(x => (object)Candidate(x)).ToArray());
            if (total > candidates.Count) context["candidates_omitted"] = total - candidates.Count;

            var hidden = (JObject)barter.Hidden?.DeepClone() ?? new JObject();
            var hiddenItems = hidden["items"] as JArray ?? new JArray();
            foreach (var x in candidates) hiddenItems.Add(BarterContext.HiddenItem(x.Shop.Item));
            hidden["items"] = hiddenItems;

            var options = candidates.Select(x => new DecisionOption { Id = StageOptionIds.Offer(x.Shop.Item.Uid), Label = Label(x, budgetLeft) }).ToList();
            options.Add(new DecisionOption
            {
                Id = StageOptionIds.NothingFits,
                Label = suitableStaged ? "Nothing more to offer: sell what is staged" : "Nothing in the shop fits this customer",
            });
            var dismiss = barter.Options?.FirstOrDefault(o => o.Id == OptionIds.Dismiss);
            if (dismiss != null) options.Add(new DecisionOption { Id = dismiss.Id, Label = dismiss.Label });

            return new BarterSnapshot
            {
                Kind = Kind,
                Client = c,
                Ready = true,
                Signature = Signature(barter.Signature, budgetLeft),
                Context = context,
                Hidden = hidden,
                Options = options,
                GameDay = barter.GameDay,
                Items = barter.Items,
                Ui = barter.Ui,
            };
        }

        static JObject Candidate(StageCandidate x)
        {
            var o = BarterContext.VisibleItem(x.Shop.Item, true, x.Shop.Where);
            o["hidden_from_inspection"] = x.Shop.Where == StageWhere.SmugglerBay;
            o["matches"] = new JArray(x.Shop.Matches.Select(TypeTags.Name).Cast<object>().ToArray());
            o["attracted"] = x.Attracted;
            o["over_budget"] = x.OverBudget;
            return o;
        }

        // "Radio #2240 (showcase): ~598, est 520, paid 300; wants electronics"
        public static string Label(StageCandidate x, long? budgetLeft)
        {
            var i = x.Shop.Item;
            var parts = new List<string>();
            string head = FeatureText.Truncate(i.Name ?? i.Id, 40) + " #" + N(i.Uid) + " (" + WhereText(x.Shop.Where) + "): ~" + N(i.Price);
            if (i.EstimatedValue.HasValue) head += ", est " + N(i.EstimatedValue.Value);
            if (i.BoughtAt.HasValue && i.BoughtAt.Value > 0) head += ", paid " + N(i.BoughtAt.Value);
            parts.Add(head);
            if (x.Attracted) parts.Add("they came for it");
            else if (x.Shop.Matches.Count > 0) parts.Add("wants " + string.Join(", ", x.Shop.Matches.Select(TypeTags.Name)));
            if (x.OverBudget && budgetLeft.HasValue) parts.Add("over budget: they pay " + N(budgetLeft.Value));
            if (i.StolenHeat.HasValue) parts.Add("STOLEN heat " + N(i.StolenHeat.Value));
            if (i.ContrabandLevel > 0) parts.Add("CONTRABAND level " + N(i.ContrabandLevel));
            return string.Join("; ", parts);
        }

        static string WhereText(string where) => where == StageWhere.SmugglerBay ? "smuggler bay, hidden from inspection" : where;

        static string N(long n) => n.ToString(CultureInfo.InvariantCulture);
    }
}
