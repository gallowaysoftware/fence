using System.Collections.Generic;
using System.Globalization;
using System.Linq;
using Fence.Barter;
using Fence.Protocol;
using Newtonsoft.Json.Linq;

namespace Fence.Inspect
{
    // What the appraisal-service panel and the rank show (docs/re/INSPECT.md, Appraisal service).
    public sealed class AppraisalTerms
    {
        public int? TheirFigure;
        public int Rank;
        public int? BaseFee;
        // Grade 1 (perfect) .. 5 (awful) -> fee percent; 0 means the game refuses that grade at this rank.
        public Dictionary<int, int> FeePct = new Dictionary<int, int>();
        public int Refusals;
        public int? LastEstimate;
        // "too_high" / "too_low": what the customer said to the last estimate.
        public string LastFeedback;
    }

    // The rest of what the player sees around the inspected item.
    public sealed class InspectSituation
    {
        public int ClientSeq;
        public int Uid;
        public string ItemId;
        public string Name;
        public long? Price;
        public bool Owned;
        public ClientInfo Client;
        public IList<FactionRep> Reputations;
        // The game's own CanOpenConfrontPanel outcome: the panel is showing for this item.
        public bool ConfrontPanelOpen;
        // The customer came for the appraisal service (StoreClient.ClientIntent.APPRAISAL_SERVICE).
        public bool AppraisalService;
        public AppraisalTerms Appraisal;
        // HIDDEN: AppraisalHelper.GetCorrectBaseValue.
        public int? TrueValue;
        public int GameDay;
    }

    public sealed class InspectQuestion
    {
        public string Kind;
        public string ItemKind;
        public string Mode;
        public string Claim;
        public int Uid;
        public string ItemId;
        public string Signature;
        public JObject Context;
        public JObject Hidden;
        public List<DecisionOption> Options;
        public int GameDay;
    }

    public static class InspectOptionIds
    {
        public const string Genuine = "genuine";
        public const string Counterfeit = "counterfeit";
        public const string ConfrontPrefix = "confront_";
        public const string Estimate = "estimate";
        public const string EstimateParam = "value";
        // Every true value is under 1500 (docs/re/INSPECT.md formulas); the game's field takes any int.
        public const long EstimateMin = 1, EstimateMax = 5000;
    }

    // The inspect decision (docs/re/INSPECT.md, "Proposal"): asked when the magnifier opens a customer's
    // item the player can act on: the confront panel (cigarettes, stamps, injectors) or the appraisal
    // panel. Advice only: the mod never confronts or estimates.
    public static class InspectContext
    {
        public const string Kind = "inspect";
        public const string ModeAuthenticity = "authenticity";
        public const string ModeAppraisal = "appraisal";

        static readonly string[] GradeNames = { "perfect", "excellent", "good", "mediocre", "awful" };
        static readonly string[] RankNames = { "junior", "appraiser", "senior", "master" };

        // The question for this inspection, or null when the game offers no verdict or estimate on it.
        public static InspectQuestion Build(IInspectItem item, InspectSituation s)
        {
            if (item == null || s == null || s.Owned) return null;
            string kind = ItemKinds.Of(item);
            if (kind == null) return null;
            if (ItemKinds.IsAuthenticity(kind))
            {
                if (!s.ConfrontPanelOpen) return null;
                string shown = item.ShownCondition(ItemKinds.ClaimCategory(kind));
                if (string.IsNullOrEmpty(shown)) return null;
                bool genuine = shown.StartsWith("genuine", System.StringComparison.Ordinal);
                // An injector claimed anything but genuine gets no panel (OpenInjectorConfrontPanel).
                if (kind == ItemKinds.Injector && !genuine) return null;
                return Authenticity(item, s, kind, genuine ? "genuine" : "counterfeit");
            }
            if (ItemKinds.IsAppraisal(kind) && s.AppraisalService && item.IsTag("APPRAISAL_ITEM")) return Appraisal(item, s, kind);
            return null;
        }

        static JObject Common(IInspectItem item, InspectSituation s, string kind, string mode)
        {
            var it = new JObject { ["uid"] = s.Uid, ["id"] = s.ItemId, ["name"] = s.Name ?? s.ItemId, ["kind"] = kind };
            if (s.Price.HasValue) it["price"] = s.Price.Value;
            var o = new JObject { ["mode"] = mode, ["item"] = it };
            return o;
        }

        static void Visible(JObject o, IInspectItem item, InspectSituation s, string kind)
        {
            if (s.Client != null)
            {
                var client = new JObject
                {
                    ["name"] = s.Client.Name,
                    ["faction"] = Factions.Visible(s.Client),
                    ["intent"] = IntentName(s.Client),
                };
                Fence.Aug.AugDetection.Describe(client, s.Client);
                o["client"] = client;
            }
            var cues = CueWords.Of(kind, item);
            if (cues.Text.Count > 0) o["text"] = new JObject(cues.Text.Select(kv => new JProperty(kv.Key, kv.Value)).ToArray());
            o["visual"] = new JObject(cues.Visual.Select(kv => new JProperty(kv.Key, kv.Value)).ToArray());
        }

        // InspectionUIManager.OnConfrontFailed: the client's faction, or Lower Levels when it has no bar.
        public static string PenaltyFaction(ClientInfo c)
        {
            string f = Factions.Visible(c);
            return f != null && Factions.WithRepBar.Contains(f) ? f : "lower_levels";
        }

        static InspectQuestion Authenticity(IInspectItem item, InspectSituation s, string kind, string claim)
        {
            var o = Common(item, s, kind, ModeAuthenticity);
            o["claim"] = claim;
            string faction = PenaltyFaction(s.Client);
            var rep = s.Reputations?.FirstOrDefault(r => Factions.Name(r.FactionId) == faction);
            var repo = new JObject { ["faction"] = faction };
            if (rep != null)
            {
                repo["value"] = rep.Value;
                repo["status"] = rep.Status;
            }
            o["reputation"] = repo;
            Visible(o, item, s, kind);
            var options = new List<DecisionOption>();
            if (claim == "genuine")
            {
                foreach (var issue in ConfrontIssues.Of(kind))
                    options.Add(new DecisionOption { Id = InspectOptionIds.ConfrontPrefix + issue.Id, Label = ConfrontLabel(issue, faction) });
                options.Add(new DecisionOption { Id = InspectOptionIds.Genuine, Label = "No confront: take it as genuine" + At(s.Price) });
            }
            else
            {
                options.Add(new DecisionOption
                {
                    Id = InspectOptionIds.Genuine,
                    Label = "Tell them it is genuine. Right: +2 rep with them, price rises to the genuine price less 15%. Wrong: they deny it, no penalty",
                });
                options.Add(new DecisionOption { Id = InspectOptionIds.Counterfeit, Label = "Agree it is counterfeit" + (s.Price.HasValue ? ": price stays " + N(s.Price.Value) : "") });
            }

            var hidden = new JObject
            {
                ["real_condition"] = item.RealCondition(ItemKinds.ClaimCategory(kind)),
                ["issue_tags"] = new JArray(CueWords.IssueTags(kind, item).Cast<object>().ToArray()),
                ["tags"] = RawTags(item, kind),
            };
            return new InspectQuestion
            {
                Kind = Kind,
                ItemKind = kind,
                Mode = ModeAuthenticity,
                Claim = claim,
                Uid = s.Uid,
                ItemId = s.ItemId,
                Signature = Signature(s, claim, null),
                Context = o,
                Hidden = hidden,
                Options = options,
                GameDay = s.GameDay,
            };
        }

        static string ConfrontLabel(ConfrontIssue issue, string faction) =>
            "Confront: " + issue.Caption + " issues. Right: exposed at its true grade, then 15% off. Wrong: -2 rep with " + faction + ", retry allowed";

        static string At(long? price) => price.HasValue ? " at " + N(price.Value) : "";

        static InspectQuestion Appraisal(IInspectItem item, InspectSituation s, string kind)
        {
            var o = Common(item, s, kind, ModeAppraisal);
            Visible(o, item, s, kind);
            var a = s.Appraisal ?? new AppraisalTerms();
            var terms = new JObject();
            if (a.TheirFigure.HasValue) terms["their_figure"] = a.TheirFigure.Value;
            terms["rank"] = RankNames[System.Math.Max(0, System.Math.Min(3, a.Rank))];
            if (a.BaseFee.HasValue) terms["base_fee"] = a.BaseFee.Value;
            if (a.FeePct.Count > 0)
                terms["fee_pct"] = new JObject(Enumerable.Range(1, 5).Where(a.FeePct.ContainsKey).Select(g => new JProperty(GradeNames[g - 1], a.FeePct[g])).ToArray());
            terms["refusals"] = a.Refusals;
            if (a.LastEstimate.HasValue) terms["last_estimate"] = a.LastEstimate.Value;
            if (!string.IsNullOrEmpty(a.LastFeedback)) terms["last_feedback"] = a.LastFeedback;
            o["appraisal"] = terms;

            string fee = a.BaseFee.HasValue ? "fee " + N(a.BaseFee.Value) + " + the grade's share of your estimate" : "fee = base + the grade's share of your estimate";
            var options = new List<DecisionOption>
            {
                new DecisionOption
                {
                    Id = InspectOptionIds.Estimate,
                    Label = "Submit an estimate: " + fee + "; refused under 50% or over 150% of the true value",
                    Params = new Dictionary<string, ParamSpec>
                    {
                        [InspectOptionIds.EstimateParam] = new ParamSpec { Type = ParamSpec.Int, Min = InspectOptionIds.EstimateMin, Max = InspectOptionIds.EstimateMax },
                    },
                },
            };
            var hidden = new JObject { ["tags"] = RawTags(item, kind) };
            if (s.TrueValue.HasValue) hidden["true_value"] = s.TrueValue.Value;
            return new InspectQuestion
            {
                Kind = Kind,
                ItemKind = kind,
                Mode = ModeAppraisal,
                Uid = s.Uid,
                ItemId = s.ItemId,
                Signature = Signature(s, "appraisal", a),
                Context = o,
                Hidden = hidden,
                Options = options,
                GameDay = s.GameDay,
            };
        }

        static JObject RawTags(IInspectItem item, string kind)
        {
            var o = new JObject();
            if (CueWords.IntTags.TryGetValue(kind, out var ints))
                foreach (var t in ints) o[t] = item.Int(t);
            if (CueWords.StringTags.TryGetValue(kind, out var strs))
                foreach (var t in strs) o[t] = item.Str(t);
            return o;
        }

        // A new decision when the item, its claim or the appraisal exchange changes.
        public static string Signature(InspectSituation s, string what, AppraisalTerms a) =>
            "i" + N(s.ClientSeq) + ":" + N(s.Uid) + ":" + what + ":r" + N(a?.Refusals ?? 0) + ":" + (a?.LastFeedback ?? "");

        public static DecideRequest Request(InspectQuestion q, string decisionId, int deadlineMs) => new DecideRequest
        {
            DecisionId = decisionId,
            Kind = q.Kind,
            GameDay = q.GameDay,
            DeadlineMs = deadlineMs,
            StateSignature = q.Signature,
            Context = q.Context,
            Hidden = q.Hidden,
            Options = q.Options,
        };

        static string IntentName(ClientInfo c)
        {
            switch (c.Intent)
            {
                case ClientIntent.Buy: return "buy";
                case ClientIntent.Sell: return "sell";
                case ClientIntent.SellAndBuy: return "sell_and_buy";
                default: return (c.RawIntent ?? "other").ToLowerInvariant();
            }
        }

        static string N(long n) => n.ToString(CultureInfo.InvariantCulture);
    }
}
