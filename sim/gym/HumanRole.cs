using System;
using System.Collections.Generic;
using System.Linq;
using Fence.Barter;

namespace Fence.Gym
{
    // Every decision the live mod leaves to the player, as a pluggable policy. The gym records each
    // default it takes (`Defaults`) so we can see which choices are not yet policy-driven; a later
    // implementation can route any of these to the bridge as a new decide kind.
    internal interface IHumanRole
    {
        string Name { get; }

        // Owned items to put on the back counter for a client who wants to buy.
        IList<GameItem> StageForBuyer(StoreClient client, IList<GameItem> owned);

        // The client's items to move to the back counter (the shop considers buying those).
        IList<GameItem> StageFromSeller(StoreClient client, IList<GameItem> theirs, int cash);

        // The display case before opening: the owned items it should hold.
        IList<GameItem> Window(IList<GameItem> owned, int capacityHint);

        // A dialogue waiting on a choice: the index to pick.
        int Dialogue(DialogueView view);

        // The bridge deferred (or failed) on a covered customer: an option id from `options`.
        string CounterFallback(BarterSnapshot snap);

        // The night: where to go and whether to phone Wild.
        NightPlan Night(NightView view);

        // The end-of-day tablet, after the night out: the optional services to have on tonight (null keeps
        // the current selection). The tablet's own rules apply after it (mandatory on, cooling-down off,
        // the dearest dropped while the total exceeds cash).
        ISet<string> Services(NightView view);

        // Wild's Network is open (Joe's visit, or the night phone call): upgrade ids to buy, in order.
        IList<string> Upgrades(UpgradeView view);
    }

    internal sealed class ChoiceView
    {
        public int Index;
        public string Label;
        public string Text;
        public bool Enabled;
    }

    internal sealed class DialogueView
    {
        public string ClientId;
        public string ClientName;
        public string Line;
        public List<ChoiceView> Choices = new List<ChoiceView>();
        public int Cash, RentDue;
        public bool HoldsSeriousContraband;
        public bool HoldsStolen;
    }

    internal sealed class ServiceView
    {
        public string Id;
        public int Cost;
        public bool Using, Mandatory, Unlocked;
        public int Cooldown;
    }

    internal sealed class NightView
    {
        public int Day, Cash, RentDue;
        public List<ServiceView> Services = new List<ServiceView>();
        public List<string> Locations = new List<string>();
    }

    internal sealed class UpgradeOption
    {
        public string Id;
        public int Cost, State;
        public bool Unlocked, Repeatable, Buyable, Ready;
        public int Cooldown;
    }

    internal sealed class UpgradeView
    {
        public int Cash, Favor, RentDue;
        // "joe" (his visit opened the window) or "phone" (the night call).
        public string Via;
        public List<UpgradeOption> Upgrades = new List<UpgradeOption>();
    }

    internal sealed class NightPlan
    {
        // An after-hours location id to visit tonight (null: stay in).
        public string Outing;
        public bool Scavenge;
        // Stop scavenging once the next search's minor + major wound chance (%) exceeds this.
        public double MaxWoundPct = double.MaxValue;
        // Phone Wild to browse the network tonight (resets the game's days-since-browse counter).
        public bool CallWild;
    }

    // Counts and logs every default taken.
    internal sealed class Defaults
    {
        public readonly SortedDictionary<string, int> Counts = new SortedDictionary<string, int>(StringComparer.Ordinal);
        public Action<string, string> Log;

        public void Take(string key, string detail = null)
        {
            Counts[key] = Counts.TryGetValue(key, out var n) ? n + 1 : 1;
            Log?.Invoke(key, detail);
        }
    }

    // The documented defaults (sim/gym/README.md, "Human-role defaults").
    internal class DefaultHumanRole : IHumanRole
    {
        public const int MaxStaged = 6;
        protected static readonly string[] WindowTypes = { "FOOD", "MEDICAL", "MATERIAL", "HOUSEHOLD_GOOD", "SUBSTANCE", "WEAPON", "WATER", "ARMOR", "TOOL", "MODULE", "AMMUNITION" };

        protected readonly Defaults defaults;

        public DefaultHumanRole(Defaults defaults) => this.defaults = defaults;

        public virtual string Name => "default";

        protected static bool Hot(GameItem i) => StolenHelper.IsStolenItem(i) || ContrabandHelper.GetContrabandLevelInInt(i) > 0;

        public virtual IList<GameItem> StageForBuyer(StoreClient client, IList<GameItem> owned)
        {
            var wanted = owned.Where(i => client.IsClientBuyingThisItem(i, false) && !(client.isSecurity && Hot(i)))
                .OrderByDescending(i => i.GetNegociatedValue()).ThenBy(i => i.uniqueId).ToList();
            var picked = new List<GameItem>();
            long budget = client.useClientBudget ? client.GetBudget() : long.MaxValue, sum = 0;
            foreach (var i in wanted)
            {
                if (picked.Count >= MaxStaged) break;
                long v = i.GetNegociatedValue();
                if (sum + v > budget) continue;
                picked.Add(i);
                sum += v;
            }
            defaults.Take("stage_for_buyer", picked.Count + " of " + wanted.Count + " wanted items within budget " + (client.useClientBudget ? budget.ToString() : "none"));
            return picked;
        }

        public virtual IList<GameItem> StageFromSeller(StoreClient client, IList<GameItem> theirs, int cash)
        {
            var picked = theirs.Where(i => i.GetCurrentValue(false, true, true, false, 0L) >= i.GetNegociatedValue()).OrderBy(i => i.uniqueId).ToList();
            defaults.Take("stage_from_seller", picked.Count + " of " + theirs.Count + " priced at or under their estimate");
            return picked;
        }

        // One item per major type, the most valuable of each, never stolen or contraband (docs/re/WINDOW.md).
        public virtual IList<GameItem> Window(IList<GameItem> owned, int capacityHint)
        {
            var picked = new List<GameItem>();
            foreach (var type in WindowTypes)
            {
                var best = owned.Where(i => !Hot(i) && !picked.Contains(i) && i.GetGameItemType().Contains(type))
                    .OrderByDescending(i => i.GetNegociatedValue()).ThenBy(i => i.uniqueId).FirstOrDefault();
                if (best != null && picked.Count < capacityHint) picked.Add(best);
            }
            defaults.Take("window", picked.Count + " items, top value per type");
            return picked;
        }

        public virtual int Dialogue(DialogueView v)
        {
            int Find(Func<ChoiceView, bool> f) => v.Choices.FirstOrDefault(c => c.Enabled && f(c))?.Index ?? -1;
            bool Label(ChoiceView c, string s) => (c.Label ?? "").IndexOf(s, StringComparison.OrdinalIgnoreCase) >= 0;
            bool Text(ChoiceView c, string s) => (c.Text ?? "").IndexOf(s, StringComparison.OrdinalIgnoreCase) >= 0;

            int i;
            if (v.ClientId == "businessPermitOfficial" && (i = PermitChoice(v)) >= 0) return i;
            if ((i = Find(c => Label(c, "buyout"))) >= 0) return Took("dialogue:buyout", v, i);
            if ((i = Find(c => Label(c, "pay rent"))) >= 0) return Took("dialogue:pay_rent", v, i);
            if ((i = Find(c => Label(c, "don't pay") || Label(c, "dont pay"))) >= 0 && v.Choices.Any(c => Label(c, "pay rent"))) return Took("dialogue:cannot_pay_rent", v, i);
            if (v.ClientId != null && v.ClientId.StartsWith("loan_shark") && (i = Find(c => Label(c, "no choice"))) >= 0) return Took("dialogue:decline_mortgage", v, i);
            var bribe = v.Choices.FirstOrDefault(c => c.Enabled && (Label(c, "bribe") || Text(c, "bribe") || Text(c, "credits")));
            if (bribe != null && v.ClientId == "securityInspector")
            {
                if (v.HoldsSeriousContraband) return Took("dialogue:bribe_inspector", v, bribe.Index);
                if ((i = Find(c => c != bribe)) >= 0) return Took("dialogue:decline_bribe", v, i);
            }
            i = Find(c => true);
            return Took("dialogue:first_enabled", v, i < 0 ? 0 : i);
        }

        protected int Took(string key, DialogueView v, int index)
        {
            var c = v.Choices.FirstOrDefault(x => x.Index == index);
            defaults.Take(key, (v.ClientId ?? "?") + ": [" + index + "] " + (c?.Label ?? "") + " | " + Trim(c?.Text));
            return index;
        }

        // The trade board official's last line offers: buy the permit (enabled while cash covers it), refuse
        // (OPERATING_WITHOUT_LICENSE, 50-170 evidence), and on the first visit only "later". Choices are
        // told apart by their reply text, which is the localized line for each option.
        public string Permit = "buy";

        int PermitChoice(DialogueView v)
        {
            int price = PlayerStore.GetBusinessPermitPrice();
            string buy = LocHelper.GetLocalizedDialogue("dialog_biz_permit_opt1_player", price);
            string later = LocHelper.GetLocalizedDialogue("dialog_biz_permit_opt4_player");
            var buyChoice = v.Choices.FirstOrDefault(c => c.Text == buy);
            if (buyChoice == null) return -1;
            if (Permit == "buy" && buyChoice.Enabled && v.Cash >= price + v.RentDue) return Took("dialogue:permit_buy", v, buyChoice.Index);
            var laterChoice = v.Choices.FirstOrDefault(c => c.Enabled && c.Text == later);
            if (laterChoice != null) return Took("dialogue:permit_later", v, laterChoice.Index);
            var refuse = v.Choices.FirstOrDefault(c => c.Enabled && c != buyChoice);
            return refuse == null ? -1 : Took("dialogue:permit_refuse", v, refuse.Index);
        }

        static string Trim(string s) => s == null ? "" : s.Length > 60 ? s.Substring(0, 60) : s;

        public string CounterFallback(BarterSnapshot snap)
        {
            var ids = snap.Options?.Select(o => o.Id).ToList() ?? new List<string>();
            string pick = ids.Contains(OptionIds.Dismiss) ? OptionIds.Dismiss : null;
            defaults.Take("counter_fallback", pick ?? "nothing offered");
            return pick;
        }

        public virtual NightPlan Night(NightView v)
        {
            var plan = new NightPlan();
            if (v.Locations.Contains("dumping_ground"))
            {
                plan.Outing = "dumping_ground";
                plan.Scavenge = true;
                defaults.Take("night:scavenge");
            }
            plan.CallWild = UpgradeOrder.Length > 0;
            return plan;
        }

        // Upgrades bought in this order whenever cash + favor covers one and still leaves the rent due.
        // Baseline: landlord grace (offered again after each missed rent) and the first display-case
        // extension; in 150-day runs it outlives buying nothing (median day 76 vs 69) while the growth
        // list (referrals, display case, renovations) costs more than it returns (median day 55).
        public static readonly string[] BaselineUpgrades = { "LANDLORD_GRACE", "SHOWCASE_I" };
        public static readonly string[] GrowthUpgrades = { "LANDLORD_GRACE", "BUY_REFERAL", "SELL_REFERAL", "SHOWCASE_I", "RENOVATION1", "SHOWCASE_II", "RENOVATION2", "RENOVATION3" };
        public string[] UpgradeOrder = BaselineUpgrades;

        public virtual IList<string> Upgrades(UpgradeView v)
        {
            var picks = new List<string>();
            int funds = v.Cash + v.Favor;
            foreach (var id in UpgradeOrder)
            {
                var u = v.Upgrades.FirstOrDefault(x => x.Id == id);
                // Grace is what a missed rent needs: it is bought even when the rent is not covered.
                if (u == null || !u.Buyable || (funds - u.Cost < v.RentDue && id != "LANDLORD_GRACE")) continue;
                picks.Add(id);
                funds -= u.Cost;
            }
            defaults.Take(picks.Count > 0 ? "wild:buy" : "wild:no_purchase", v.Via + ": " + (picks.Count > 0 ? string.Join(",", picks) : v.Upgrades.Count(u => u.Buyable) + " buyable"));
            return picks;
        }

        // Insurance (an uninsured store is burgled every night) and janitorial (only it clears garbage,
        // which halves attractiveness and with it the queue), each on while cash covers its nightly cost
        // on top of the rent due and the services already kept; everything else as it was.
        static readonly string[] Baseline = { "INSURANCE_SERVICE", "JANITORIAL_SERVICE" };

        public virtual ISet<string> Services(NightView v) => Keep(v, Baseline, nights: 1, "night:service_");

        protected ISet<string> Keep(NightView v, string[] wanted, int nights, string key)
        {
            var on = new HashSet<string>(v.Services.Where(s => s.Using && !wanted.Contains(s.Id)).Select(s => s.Id), StringComparer.Ordinal);
            int reserve = v.RentDue + v.Services.Where(s => on.Contains(s.Id) || s.Mandatory).Sum(s => s.Cost);
            foreach (var id in wanted)
            {
                var s = v.Services.FirstOrDefault(x => x.Id == id && x.Unlocked);
                if (s == null) continue;
                bool keep = v.Cash >= reserve + nights * s.Cost;
                if (keep)
                {
                    on.Add(id);
                    reserve += s.Cost;
                }
                defaults.Take(key + id.ToLowerInvariant(), keep ? "on" : "off");
            }
            return on;
        }
    }
}

namespace Fence.Gym
{
    // The baseline with a week of each service held in reserve on top of the rent.
    internal sealed class PrudentHumanRole : DefaultHumanRole
    {
        static readonly string[] Wanted = { "INSURANCE_SERVICE", "JANITORIAL_SERVICE" };

        public PrudentHumanRole(Defaults defaults) : base(defaults) { }

        public override string Name => "prudent";

        public override ISet<string> Services(NightView v) => Keep(v, Wanted, nights: 7, "night:prudent_");
    }

    // The tablet left as the game sets it: no optional services are ever switched on (the role every run
    // used before the baseline existed; kept for comparison).
    internal sealed class BareHumanRole : DefaultHumanRole
    {
        public BareHumanRole(Defaults defaults) : base(defaults) { }

        public override string Name => "bare";

        public override ISet<string> Services(NightView v)
        {
            defaults.Take("night:services_unchanged");
            return null;
        }
    }
}
