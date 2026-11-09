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

        // The night: outings, services, upgrades.
        NightPlan Night(NightView view);
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
        public int Cash;
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
        public List<(string id, int cost, bool affordable)> Upgrades = new List<(string, int, bool)>();
    }

    internal sealed class NightPlan
    {
        // An after-hours location id to visit tonight (null: stay in).
        public string Outing;
        public bool Scavenge;
        // Services to have on at sleep; null keeps the current selection.
        public HashSet<string> Services;
        public List<string> Upgrades = new List<string>();
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
        static readonly string[] WindowTypes = { "FOOD", "MEDICAL", "MATERIAL", "HOUSEHOLD_GOOD", "SUBSTANCE", "WEAPON", "WATER", "ARMOR", "TOOL", "MODULE", "AMMUNITION" };

        protected readonly Defaults defaults;

        public DefaultHumanRole(Defaults defaults) => this.defaults = defaults;

        public virtual string Name => "default";

        static bool Hot(GameItem i) => StolenHelper.IsStolenItem(i) || ContrabandHelper.GetContrabandLevelInInt(i) > 0;

        public IList<GameItem> StageForBuyer(StoreClient client, IList<GameItem> owned)
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

        public IList<GameItem> StageFromSeller(StoreClient client, IList<GameItem> theirs, int cash)
        {
            var picked = theirs.Where(i => i.GetCurrentValue(false, true, true, false, 0L) >= i.GetNegociatedValue()).OrderBy(i => i.uniqueId).ToList();
            defaults.Take("stage_from_seller", picked.Count + " of " + theirs.Count + " priced at or under their estimate");
            return picked;
        }

        // One item per major type, the most valuable of each, never stolen or contraband (docs/re/WINDOW.md).
        public IList<GameItem> Window(IList<GameItem> owned, int capacityHint)
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

        public int Dialogue(DialogueView v)
        {
            int Find(Func<ChoiceView, bool> f) => v.Choices.FirstOrDefault(c => c.Enabled && f(c))?.Index ?? -1;
            bool Label(ChoiceView c, string s) => (c.Label ?? "").IndexOf(s, StringComparison.OrdinalIgnoreCase) >= 0;
            bool Text(ChoiceView c, string s) => (c.Text ?? "").IndexOf(s, StringComparison.OrdinalIgnoreCase) >= 0;

            int i;
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

        int Took(string key, DialogueView v, int index)
        {
            var c = v.Choices.FirstOrDefault(x => x.Index == index);
            defaults.Take(key, (v.ClientId ?? "?") + ": [" + index + "] " + (c?.Label ?? "") + " | " + Trim(c?.Text));
            return index;
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
            Services(v, plan);
            defaults.Take("night:no_upgrades");
            return plan;
        }

        protected virtual void Services(NightView v, NightPlan plan) => defaults.Take("night:services_unchanged");
    }
}

namespace Fence.Gym
{
    // The default plus the two services whose absence the default runs show bleeding value every
    // night: insurance (uninsured stores are burgled nightly) and janitorial (garbage halves
    // attractiveness, and with it the queue). Each is kept on while cash covers a week of it on top
    // of the rent due.
    internal sealed class PrudentHumanRole : DefaultHumanRole
    {
        static readonly string[] Wanted = { "INSURANCE_SERVICE", "JANITORIAL_SERVICE" };

        public PrudentHumanRole(Defaults defaults) : base(defaults) { }

        public override string Name => "prudent";

        protected override void Services(NightView v, NightPlan plan)
        {
            plan.Services = new HashSet<string>(v.Services.Where(s => s.Using).Select(s => s.Id), StringComparer.Ordinal);
            int reserve = v.RentDue;
            foreach (var id in Wanted)
            {
                var s = v.Services.FirstOrDefault(x => x.Id == id && x.Unlocked);
                if (s == null) continue;
                bool on = v.Cash >= reserve + 7 * s.Cost;
                if (on) plan.Services.Add(id);
                else plan.Services.Remove(id);
                defaults.Take("night:prudent_" + id.ToLowerInvariant(), on ? "on" : "off");
            }
        }
    }
}
