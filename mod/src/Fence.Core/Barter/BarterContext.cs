using System;
using System.Collections.Generic;
using System.Globalization;
using System.Linq;
using System.Text;
using Fence.Protocol;
using Newtonsoft.Json.Linq;

namespace Fence.Barter
{
    public sealed class BarterSnapshot
    {
        public ClientInfo Client;
        public bool Ready;
        // Null when covered.
        public string CoverageGap;
        public string Signature;
        public JObject Context;
        public JObject Hidden;
        public List<DecisionOption> Options;
        public int GameDay;
        // "barter" or StageContext.Kind.
        public string Kind = BarterContext.Kind;
        // What the snapshot was read from; null when not Ready.
        public IList<ItemInfo> Items;
        public UiState Ui;
    }

    // Builds the v1 barter context (docs/PROTOCOL.md, "Barter context (v1)") from player-visible
    // reads only; internal values go to `hidden`, which the bridge never prompts with.
    public static class BarterContext
    {
        public const string Kind = "barter";

        public static BarterSnapshot Read(IBarterGame game, int clientSeq)
        {
            var client = game.Client();
            var snap = new BarterSnapshot { Client = client, CoverageGap = BarterRules.CoverageGap(client) };
            if (client == null) return snap;
            var ui = game.Ui();
            var items = game.CounterItems() ?? new List<ItemInfo>();
            snap.Ready = snap.CoverageGap == null && BarterRules.DecisionReady(ui, client, items);
            if (!snap.Ready) return snap;

            var store = game.Store() ?? new StoreInfo();
            var flags = game.Bargain() ?? new BargainFlags();
            snap.GameDay = store.Day ?? 0;
            snap.Options = BarterRules.Options(store, client, items, flags, ui, game.Odds);
            snap.Signature = Signature(clientSeq, items, flags, BarterRules.ResolveMode(client, items));
            snap.Context = Context(store, game.Reputations(), client, items, flags, game.MarketEvents());
            snap.Hidden = Hidden(client, items, game.DaysToInspection());
            snap.Items = items;
            snap.Ui = ui;
            return snap;
        }

        // Changes whenever anything the options or prices depend on changes; item order on the counter does not count.
        public static string Signature(int clientSeq, IList<ItemInfo> items, BargainFlags f, TabletMode mode)
        {
            var sb = new StringBuilder();
            sb.Append('c').Append(clientSeq.ToString(CultureInfo.InvariantCulture)).Append(":i");
            bool first = true;
            foreach (var i in items.OrderBy(i => i.Uid))
            {
                if (!first) sb.Append(',');
                first = false;
                sb.Append(i.Uid.ToString(CultureInfo.InvariantCulture)).Append(i.Owned ? 'o' : 't').Append(i.OnBack ? 'b' : 'f')
                    .Append('@').Append(i.Price.ToString(CultureInfo.InvariantCulture));
            }
            sb.Append(":b").Append(Bit(f.SellSideUsed)).Append(Bit(f.BuySideUsed)).Append(Bit(f.BlackmailOffered)).Append(Bit(f.ThreatenOffered));
            sb.Append(":m").Append(mode == TabletMode.ClientBuys ? 'b' : mode == TabletMode.ClientSells ? 's' : 'n');
            return sb.ToString();
        }

        static char Bit(bool b) => b ? '1' : '0';

        public static JObject Context(StoreInfo store, IList<FactionRep> reps, ClientInfo c, IList<ItemInfo> items, BargainFlags f, IList<MarketEvent> events)
        {
            var s = new JObject();
            Put(s, "cash", store.Cash);
            Put(s, "day", store.Day);
            Put(s, "retail_markup", store.RetailMarkup);
            Put(s, "rent_due", store.RentDue);
            Put(s, "rent_days_left", store.RentDaysLeft);

            var rep = new JObject();
            foreach (var r in reps ?? new List<FactionRep>())
            {
                string name = Factions.Name(r.FactionId);
                if (name == null || !Factions.WithRepBar.Contains(name)) continue;
                rep[name] = new JObject { ["value"] = Math.Round(r.Value, 2), ["status"] = r.Status };
            }

            bool buying = BarterRules.IsBuying(c.Intent);
            var client = new JObject
            {
                ["name"] = c.Name,
                ["faction"] = Factions.Name(c.FactionId),
                ["intent"] = Intent(c.Intent),
                ["budget"] = buying && c.UsesBudget ? new JValue(c.Budget) : JValue.CreateNull(),
                ["interests"] = new JArray((buying ? c.InterestTags : new List<string>()).Select(TypeTags.Name).Cast<object>().ToArray()),
                ["restrictions"] = new JArray((buying ? c.RestrictionTags : new List<string>()).Select(TypeTags.Name).Cast<object>().ToArray()),
                ["no_contraband"] = c.NoContraband,
                ["requires_variety"] = c.RequiresVariety,
                ["is_security"] = c.IsSecurity,
                // A stealth-wanted client has no poster, so the player cannot know.
                ["on_wanted_poster"] = c.IsWanted && !c.IsStealthWanted,
                ["attracted_by"] = !string.IsNullOrEmpty(c.AttractedItemId) ? new JValue(c.AttractedItemId)
                    : !string.IsNullOrEmpty(c.AttractedType) ? new JValue(TypeTags.Name(c.AttractedType)) : JValue.CreateNull(),
                ["wholesale"] = c.OffersWholesale && c.WholesalePrice.HasValue && items.Any(i => !i.Owned)
                    ? new JObject { ["price"] = c.WholesalePrice.Value, ["discount_pct"] = Math.Abs(c.WholesaleDiscount) }
                    : (JToken)JValue.CreateNull(),
            };

            var counter = new JObject
            {
                ["theirs"] = new JArray(items.Where(i => !i.Owned).Select(VisibleItem).Cast<object>().ToArray()),
                ["yours_staged"] = new JArray(items.Where(i => i.Owned && i.OnBack).Select(VisibleItem).Cast<object>().ToArray()),
            };

            var bargain = new JObject
            {
                ["sell_side_used"] = f.SellSideUsed,
                ["buy_side_used"] = f.BuySideUsed,
                ["blackmail_used"] = f.BlackmailOffered,
                ["threaten_used"] = f.ThreatenOffered,
            };

            var market = new JArray((events ?? new List<MarketEvent>())
                .Select(e => (object)new JObject { ["item_type"] = TypeTags.Name(e.ItemType), ["value_pct"] = e.ValuePct }).ToArray());

            return new JObject
            {
                ["store"] = s,
                ["reputation"] = rep,
                ["client"] = client,
                ["counter"] = counter,
                ["bargain"] = bargain,
                ["market_events"] = market,
            };
        }

        static JObject VisibleItem(ItemInfo i) => VisibleItem(i, true, null);

        // The tablet / tooltip view of an item. `where` places an owned item in the shop (day snapshot).
        public static JObject VisibleItem(ItemInfo i, bool withPrice, string where)
        {
            var o = new JObject
            {
                ["uid"] = i.Uid,
                ["id"] = i.Id,
                ["name"] = i.Name,
            };
            if (withPrice) o["price"] = i.Price;
            if (where != null) o["where"] = where;
            if (i.EstimatedValue.HasValue) o["estimated_value"] = i.EstimatedValue.Value;
            if (i.Owned && i.BoughtAt.HasValue && i.BoughtAt.Value > 0) o["bought_at"] = i.BoughtAt.Value;
            o["features"] = new JArray(i.Features.Where(x => x.Visible && !x.Disabled)
                .Select(x => new { text = FeatureText.Plain(x.Shown), group = Group(x.Group) })
                .Where(x => x.text.Length > 0)
                .Select(x => (object)new JObject { ["text"] = x.text, ["group"] = x.group }).ToArray());
            o["stolen_heat"] = i.StolenHeat.HasValue ? new JValue(i.StolenHeat.Value) : JValue.CreateNull();
            o["contraband_level"] = i.ContrabandLevel;
            o["serious_contraband"] = i.SeriousContraband;
            return o;
        }

        public static JObject Hidden(ClientInfo c, IList<ItemInfo> items, int? daysToInspection)
        {
            var hidden = new JObject
            {
                ["client"] = new JObject
                {
                    ["identifier"] = c.Identifier,
                    ["is_aug"] = c.IsAug,
                    ["stealth_wanted"] = c.IsStealthWanted,
                },
                ["items"] = new JArray(items.Where(i => !i.Owned || i.OnBack).Select(HiddenItem).Cast<object>().ToArray()),
            };
            if (daysToInspection.HasValue) hidden["days_to_inspection"] = daysToInspection.Value;
            return hidden;
        }

        public static JObject HiddenItem(ItemInfo i)
        {
            var o = new JObject { ["uid"] = i.Uid };
            long? truth = TrueValue(i);
            if (truth.HasValue) o["true_value"] = truth.Value;
            o["real_features"] = new JArray(i.Features.Where(x => !x.Disabled).Select(x => FeatureText.Plain(x.Real))
                .Where(t => t.Length > 0).Cast<object>().ToArray());
            return o;
        }

        // The appraisal value when the game has one; otherwise the estimated value re-staged with every
        // feature's true modifier (GameItem.ComposeStagedValue, misc value ignored). A training label only.
        public static long? TrueValue(ItemInfo i)
        {
            if (i.AppraisalValue > 0) return i.AppraisalValue;
            if (!i.EstimatedValue.HasValue) return null;
            double ratio = 1.0;
            foreach (FeatureGroup g in Enum.GetValues(typeof(FeatureGroup)))
            {
                var fs = i.Features.Where(x => !x.Disabled && x.Group == g).ToList();
                if (g == FeatureGroup.Bargain)
                {
                    foreach (var x in fs)
                    {
                        double cur = Math.Max(0.0, 1.0 + x.CurrentMod / 100.0), act = Math.Max(0.0, 1.0 + x.ActualMod / 100.0);
                        if (cur <= 0.0) return null;
                        ratio *= act / cur;
                    }
                    continue;
                }
                double curSum = Math.Max(0.0, 1.0 + fs.Sum(x => x.CurrentMod) / 100.0), actSum = Math.Max(0.0, 1.0 + fs.Sum(x => x.ActualMod) / 100.0);
                if (curSum <= 0.0) return null;
                ratio *= actSum / curSum;
            }
            return (long)Math.Round(i.EstimatedValue.Value * ratio);
        }

        static string Intent(ClientIntent i)
        {
            switch (i)
            {
                case ClientIntent.Buy: return "buy";
                case ClientIntent.Sell: return "sell";
                case ClientIntent.SellAndBuy: return "sell_and_buy";
                case ClientIntent.Dialogue: return "dialogue";
                default: return "other";
            }
        }

        static string Group(FeatureGroup g) => g == FeatureGroup.Market ? "market" : g == FeatureGroup.Bargain ? "bargain" : "condition";

        static void Put(JObject o, string key, int? value)
        {
            if (value.HasValue) o[key] = value.Value;
        }

        public static DecideRequest Request(BarterSnapshot snap, string decisionId, int deadlineMs) => new DecideRequest
        {
            DecisionId = decisionId,
            Kind = snap.Kind ?? Kind,
            GameDay = snap.GameDay,
            DeadlineMs = deadlineMs,
            StateSignature = snap.Signature,
            Context = snap.Context,
            Hidden = snap.Hidden,
            Options = snap.Options,
        };
    }
}
