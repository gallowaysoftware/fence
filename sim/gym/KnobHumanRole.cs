using System;
using System.Collections.Generic;
using System.Linq;

namespace Fence.Gym
{
    // The default human role with each strategy choice behind a knob (Knobs.cs). Unset knobs fall
    // through to DefaultHumanRole, so `-human knobs` with no knobs plays exactly like `-human default`.
    internal class KnobHumanRole : DefaultHumanRole
    {
        readonly Knobs k;

        public KnobHumanRole(Defaults defaults, Knobs knobs) : base(defaults)
        {
            k = knobs;
            if (k.Has("permit") && k.S("permit", null) is var p && p != "buy" && p != "refuse") throw new ArgumentException("permit=" + p + " (buy|refuse)");
        }

        public override string Name => "knobs";

        static bool Stolen(GameItem i) => StolenHelper.IsStolenItem(i);
        static bool Contraband(GameItem i) => ContrabandHelper.GetContrabandLevelInInt(i) > 0;

        bool Sellable(GameItem i, StoreClient c)
        {
            if (c.isSecurity && Hot(i)) return false;
            if (Stolen(i) && !k.B("sell_hot", true)) return false;
            if (Contraband(i) && !k.B("sell_contraband", true)) return false;
            // Speculation: hold anything a market event currently depresses until the event ends.
            if (k.B("spec", false) && i.GetCurrentValue(false, true, true, false, 0L) < i.GetCurrentValue(false, true, false, false, 0L)) return false;
            return true;
        }

        public override IList<GameItem> StageForBuyer(StoreClient client, IList<GameItem> owned)
        {
            if (!k.Has("sell_hot") && !k.Has("sell_contraband") && !k.Has("max_stage") && !k.Has("spec")) return base.StageForBuyer(client, owned);
            int max = k.I("max_stage", MaxStaged);
            var wanted = owned.Where(i => client.IsClientBuyingThisItem(i, false) && Sellable(i, client))
                .OrderByDescending(i => i.GetNegociatedValue()).ThenBy(i => i.uniqueId).ToList();
            var picked = new List<GameItem>();
            long budget = client.useClientBudget ? client.GetBudget() : long.MaxValue, sum = 0;
            foreach (var i in wanted)
            {
                if (picked.Count >= max) break;
                long v = i.GetNegociatedValue();
                if (sum + v > budget) continue;
                picked.Add(i);
                sum += v;
            }
            defaults.Take("stage_for_buyer", picked.Count + " of " + wanted.Count);
            return picked;
        }

        public override IList<GameItem> Window(IList<GameItem> owned, int capacityHint)
        {
            string mode = k.S("window", "types");
            bool hot = k.B("window_hot", false);
            int n = Math.Min(capacityHint, k.I("window_n", capacityHint));
            var pool = owned.Where(i => hot || !Hot(i)).ToList();
            List<GameItem> picked;
            switch (mode)
            {
                case "types":
                    if (!hot && !k.Has("window_n")) return base.Window(owned, capacityHint);
                    picked = new List<GameItem>();
                    foreach (var type in WindowTypes)
                    {
                        var best = pool.Where(i => !picked.Contains(i) && i.GetGameItemType().Contains(type))
                            .OrderByDescending(i => i.GetNegociatedValue()).ThenBy(i => i.uniqueId).FirstOrDefault();
                        if (best != null && picked.Count < n) picked.Add(best);
                    }
                    break;
                case "empty":
                    picked = new List<GameItem>();
                    break;
                case "top":
                    picked = pool.OrderByDescending(i => i.GetNegociatedValue()).ThenBy(i => i.uniqueId).Take(n).ToList();
                    break;
                case "concentrate":
                {
                    // The showcase client follows the type with the largest summed value (SPEC §4.4).
                    string bestType = null;
                    long bestSum = -1;
                    foreach (var type in WindowTypes)
                    {
                        long sum = pool.Where(i => i.GetGameItemType().Contains(type)).OrderByDescending(i => i.GetNegociatedValue()).Take(n).Sum(i => i.GetNegociatedValue());
                        if (sum > bestSum) { bestSum = sum; bestType = type; }
                    }
                    picked = pool.Where(i => i.GetGameItemType().Contains(bestType)).OrderByDescending(i => i.GetNegociatedValue()).ThenBy(i => i.uniqueId).Take(n).ToList();
                    break;
                }
                default:
                    throw new ArgumentException("window=" + mode + " (types|empty|top|concentrate)");
            }
            defaults.Take("window", mode + " " + picked.Count);
            return picked;
        }

        // A choice's reply text is the localized line; amounts vary, so lines with an argument match on
        // the text before it.
        static bool Says(ChoiceView c, string key)
        {
            string line = LocHelper.GetLocalizedDialogue(key, "\u0001");
            int cut = line.IndexOf('\u0001');
            return cut < 0 ? c.Text == line : (c.Text ?? "").StartsWith(line.Substring(0, cut), StringComparison.Ordinal);
        }

        public override int Dialogue(DialogueView v)
        {
            if (k.B("dlglog", false))
                Console.Error.WriteLine("dlg " + v.ClientId + " cash=" + v.Cash + " | " + string.Join(" || ", v.Choices.Select(c => "[" + c.Index + (c.Enabled ? "" : " off") + "] " + c.Label + " | " + c.Text)));
            if (k.Has("permit")) Permit = k.S("permit", Permit);
            int Find(Func<ChoiceView, bool> f) => v.Choices.FirstOrDefault(c => c.Enabled && f(c))?.Index ?? -1;
            int i;
            if (k.S("dlg", "first") == "smart")
            {
                // Choices that only cost money for no modelled return.
                if (v.ClientId == "georgioLoan" && (i = Find(c => !Says(c, "dialog_georgio_loan_choice_offer"))) >= 0) return Took("dialogue:decline_loan", v, i);
                if (v.ClientId == "eleanor4" && (i = Find(c => !Says(c, "dialog_eleanor4_choice_credits"))) >= 0) return Took("dialogue:decline_gift", v, i);
            }
            // Reporting the cartel ends a drug-dealer run on the spot (cartel_retired).
            if (k.S("dlg", "first") != "first" && v.ClientId == "cartelSec" && (i = Find(c => Says(c, "dialog_cartel_sec_choice_no_report"))) >= 0)
                return Took("dialogue:cartel_no_report", v, i);
            return base.Dialogue(v);
        }

        public override NightPlan Night(NightView v)
        {
            if (k.S("scav", "always") == "always" && !k.Has("scav_risk")) return base.Night(v);
            var plan = base.Night(v);
            if (k.S("scav", "always") == "never")
            {
                plan.Outing = null;
                plan.Scavenge = false;
            }
            else if (k.S("scav", "always") != "always") throw new ArgumentException("scav=" + k.S("scav", null) + " (always|never)");
            plan.MaxWoundPct = k.D("scav_risk", plan.MaxWoundPct);
            return plan;
        }

        public override ISet<string> Services(NightView v)
        {
            string mode = k.S("services", "default");
            if (mode == "default" && !k.Has("svc_nights")) return base.Services(v);
            if (mode == "tablet")
            {
                defaults.Take("night:services_unchanged");
                return null;
            }
            var wanted = mode switch
            {
                "none" => new string[0],
                "ins" => new[] { "INSURANCE_SERVICE" },
                "jan" => new[] { "JANITORIAL_SERVICE" },
                "both" or "default" => new[] { "INSURANCE_SERVICE", "JANITORIAL_SERVICE" },
                _ => throw new ArgumentException("services=" + mode + " (default|tablet|none|ins|jan|both)"),
            };
            var on = Keep(v, wanted, nights: k.I("svc_nights", 1), "night:service_");
            foreach (var id in new[] { "INSURANCE_SERVICE", "JANITORIAL_SERVICE" }.Where(id => !wanted.Contains(id)))
                on.Remove(id);
            return on;
        }
    }
}
