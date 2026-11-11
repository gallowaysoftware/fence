using System;
using System.Collections.Generic;
using System.Linq;
using Newtonsoft.Json.Linq;

namespace Fence.Gym
{
    // The labeler (`labeler`, docs/re/LABELER.md): it rewrites the claimed condition of an owned item's feature
    // (LabelerElement.OnDropdownValueChanged: fakeCondition = the choice, then SetExposed(false)). Headless the
    // labeler's window is stubbed, so the executor applies that same change. Two gym levers, both off by default:
    // - `fakes: "honest"`: each morning the magnifier's view of every owned Immunivax injector and Hemingway pack
    //   claimed genuine is read (indicator, body, label, serial; seal, lettering, logo, box), and one showing a
    //   fault is relabelled to what the fault means (expired / unusable / counterfeit). Its claim then matches the
    //   truth: no buyer can expose it, and it sells to anyone at its true value.
    // - `labeler: "per_buyer"` (implies `fakes: "honest"`): the identified fakes stay honestly labelled, except
    //   while a buyer who cannot expose a genuine claim is at the counter: for him they are labelled genuine
    //   (ClientCanExposeFunc: a buyer's capacity is fixed by his client type), and back after he leaves. Each
    //   morning every owned item's rust, dent, expiry, open-box and replica claims are also relabelled to the
    //   best condition of their category: no counter buyer checks those (only the trade board official, whose
    //   donations skip anything he would expose). Relabelled goods stay out of the vending machine, whose
    //   mislabelled sales are FRAUD.
    internal static class Labels
    {
        static Directives D => Directives.Current;
        static bool Upgrade => D.Raw.Value<string>("labeler") == "per_buyer";
        static bool Honest => D.Raw.Value<string>("fakes") == "honest" || Upgrade;
        public static bool On => Honest || Upgrade;

        static readonly string[] UpgradeCategories = { "CATEGORY_RUSTED", "CATEGORY_DENTED", "CATEGORY_EXPIRED", "CATEGORY_OPEN_BOX", "CATEGORY_REPLICA" };

        public static int HonestItems, UpgradedItems, NoTool, FakeSales;
        public static long HonestDelta, UpgradeDelta;
        static readonly HashSet<int> upgraded = new HashSet<int>();
        // Identified fakes: item uid -> (feature category, the condition its view shows).
        static readonly Dictionary<int, (string category, string truth)> known = new Dictionary<int, (string, string)>();
        static readonly HashSet<int> claimedGenuine = new HashSet<int>();

        public static void Reset()
        {
            HonestItems = UpgradedItems = NoTool = FakeSales = 0;
            known.Clear();
            claimedGenuine.Clear();
            HonestDelta = UpgradeDelta = 0;
            upgraded.Clear();
            reference = null;
        }

        public static bool IsTool(GameItem i) => On && (i.identifier == "labeler" || i.identifier == "magnifier");

        // Relabelled upward by the executor: what the vending machine must not sell.
        public static bool Upgraded(GameItem i) => upgraded.Contains(i.uniqueId) || known.ContainsKey(i.uniqueId);

        static IEnumerable<GameItem> Owned() => EmporiumEntry.Instance.GetAllOwnedItems(false, true).Where(i => i != null);

        public static void Morning(IExecutorHost ep)
        {
            if (!On) return;
            var owned = Owned().ToList();
            bool labeler = owned.Any(i => i.identifier == "labeler"), magnifier = owned.Any(i => i.identifier == "magnifier");
            if (!labeler) { NoTool++; return; }
            foreach (var i in owned.OrderBy(i => i.uniqueId))
            {
                if (Honest && magnifier) HonestLabel(ep, i);
                if (Upgrade) UpgradeLabel(ep, i);
            }
        }

        // What the inspection view shows means one condition (InsInjectorHelper.SetExpired/SetUnusable/SetCounterfeit,
        // InsCigaretteHelper.SetConterfeit; docs/re/INSPECT.md).
        static string InjectorTruth(GameItem i)
        {
            int indicator = i.GetTagReadonly("INJECTOR_INDICATOR").GetInt(), body = i.GetTagReadonly("INJECTOR_BODY").GetInt();
            if (indicator == 1) return "expiredInjector";
            if (indicator == 3 || (body >= 1 && body <= 3)) return "unusableInjector";
            if (body == -1) return "counterfeitInjector";
            // The guide's reference: a freshly made injector is genuine by construction.
            if (reference == null)
            {
                var g = DirectoryMaster.Item("large_purple_injector");
                reference = (g.GetTagReadonly("INJECTOR_COMPANY").GetString(), g.GetTagReadonly("INJECTOR_MODEL").GetString());
            }
            if (i.GetTagReadonly("INJECTOR_COMPANY").GetString() != reference.Value.company || i.GetTagReadonly("INJECTOR_MODEL").GetString() != reference.Value.model) return "counterfeitInjector";
            string serial = i.GetTagReadonly("INJECTOR_SERIAL").GetString() ?? "";
            if (serial.Length != 7 || !serial.All(char.IsDigit)) return "counterfeitInjector";
            int prefix = int.Parse(serial.Substring(0, 3));
            if (prefix < 125 || prefix > 455) return "counterfeitInjector";
            if (InsInjectorHelper.RECALLED_PREFIX.Contains(prefix)) return "unusableInjector";
            return "genuineInjector";
        }

        static (string company, string model)? reference;

        static string CigaretteTruth(GameItem i) =>
            new[] { "CIGARETTE_SEAL", "CIGARETTE_LETTERING", "CIGARETTE_LOGO", "CIGARETTE_BOX" }.Any(t => i.GetTagReadonly(t).GetInt() != 0) ? "counterfeitCigarette" : "genuineCigarette";

        static void HonestLabel(IExecutorHost ep, GameItem i)
        {
            ItemFeature f;
            string truth;
            if (i.IsTag("INS_INJECTOR_TAG") && (f = i.FindItemFeatureByCategory("CATEGORY_GENUINE_INJECTOR")) != null) truth = InjectorTruth(i);
            else if (i.IsTag("CIGARETTE_TAG") && (f = i.FindItemFeatureByCategory("CATEGORY_GENUINE_CIGARETTE")) != null) truth = CigaretteTruth(i);
            else return;
            if (f.IsExposed() || f.isDisabled || !LabelerHelper.IsEditableLabelFeature(f)) return;
            string claim = f.GetCurrentItemCondition()?.identifier;
            if (claim == truth || (claim != "genuineInjector" && claim != "genuineCigarette")) return;
            known[i.uniqueId] = (f.category, truth);
            Relabel(ep, i, f, truth, honest: true);
        }

        static ItemFeature Feature(GameItem i, string category) => i.FindItemFeatureByCategory(category) is ItemFeature f && !f.IsExposed() ? f : null;

        static void Set(ItemFeature f, string to)
        {
            if (f.GetCurrentItemCondition()?.identifier == to) return;
            f.fakeCondition = ItemConditionList.conditionLookup[f.category].First(c => c.identifier == to);
            f.SetExposed(false);
        }

        // Before staging for a buyer: identified fakes labelled genuine when his type cannot expose that claim.
        public static void ForBuyer(StoreClient c, IList<GameItem> owned)
        {
            if (!Upgrade || c == null) return;
            foreach (var i in owned.Where(i => known.ContainsKey(i.uniqueId)))
            {
                var (cat, truth) = known[i.uniqueId];
                var f = Feature(i, cat);
                if (f == null) continue;
                Set(f, cat == "CATEGORY_GENUINE_INJECTOR" ? "genuineInjector" : "genuineCigarette");
                if (c.CanClientExposeAnyFeatureDeep(i)) Set(f, truth);
                else claimedGenuine.Add(i.uniqueId);
            }
        }

        // After the customer: the unsold ones go back to their honest label; count those he bought as genuine.
        public static void AfterCustomer()
        {
            if (!Upgrade) return;
            foreach (var uid in known.Keys.ToList())
            {
                var i = EmporiumEntry.Instance.FindItemByUniqueID(uid);
                if (i == null || !GeneralHelper.IsItemOwned(i))
                {
                    known.Remove(uid);
                    if (claimedGenuine.Contains(uid)) FakeSales++;
                    continue;
                }
                var (cat, truth) = known[uid];
                if (Feature(i, cat) is ItemFeature f) Set(f, truth);
            }
            claimedGenuine.Clear();
        }

        static void UpgradeLabel(IExecutorHost ep, GameItem i)
        {
            foreach (var f in i.itemFeatures.Where(f => f != null && !f.isDisabled && UpgradeCategories.Contains(f.category) && LabelerHelper.IsEditableLabelFeature(f)).ToList())
            {
                var best = ItemConditionList.conditionLookup[f.category].OrderByDescending(c => c.modValue).First();
                var cur = f.GetCurrentItemCondition();
                if (cur == null || cur.modValue >= best.modValue) continue;
                Relabel(ep, i, f, best.identifier, honest: false);
            }
        }

        static void Relabel(IExecutorHost ep, GameItem i, ItemFeature f, string to, bool honest)
        {
            var cond = ItemConditionList.conditionLookup[f.category].FirstOrDefault(c => c.identifier == to);
            if (cond == null) return;
            long before = i.GetNegociatedValue();
            string from = f.GetCurrentItemCondition()?.identifier;
            Chore.Act("relabel", i, () =>
            {
                f.fakeCondition = cond;
                f.SetExposed(false);
            });
            long delta = i.GetNegociatedValue() - before;
            if (honest) { HonestItems++; HonestDelta += delta; }
            else { UpgradedItems++; UpgradeDelta += delta; upgraded.Add(i.uniqueId); }
            ep.Human("relabel", null, ep.Uid(i), new JObject { ["category"] = f.category, ["from"] = from, ["to"] = to, ["value_delta"] = delta }, honest ? "player:label_honest" : "player:label_upgrade");
        }

        public static JObject Status()
        {
            if (!On) return null;
            return new JObject
            {
                ["honest_items"] = HonestItems, ["fakes_sold_as_genuine"] = FakeSales, ["honest_value_delta"] = HonestDelta,
                ["upgraded_items"] = UpgradedItems, ["upgrade_value_delta"] = UpgradeDelta,
                ["upgraded_exposed"] = upgraded.Count(uid => EmporiumEntry.Instance.FindItemByUniqueID(uid) is GameItem g && g.itemFeatures.Any(f => f != null && UpgradeCategories.Contains(f.category) && f.IsExposed())),
                ["mornings_without_labeler"] = NoTool,
            };
        }
    }
}
