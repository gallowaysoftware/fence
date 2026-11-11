using System;
using System.Collections.Generic;
using System.Linq;

namespace Fence.Lab
{
    // docs/re/WINDOW.md: display-type buyers, tiers, discerning shopper, showcase inspections.
    public static class WindowExps
    {
        static StoreClientManager Mgr => W.PS.storeClientManager;
        static GameGridInventory Case => W.E.showcaseElement;

        static List<StoreClient> Run(Action handler)
        {
            Mgr.clientStack.Clear();
            handler();
            var q = Mgr.clientStack.ToList();
            Mgr.clientStack.Clear();
            return q;
        }

        static void Fill(params string[] ids)
        {
            W.ClearInventory(Case);
            foreach (var id in ids) W.Own(id, Case);
        }

        static void SetUpgrade(string id, bool on)
        {
            var u = W.PS.networkUpgrade[id];
            u.state = on ? 2 : 0;
        }

        public static void Buyers(int seed)
        {
            W.New(seed);
            W.Day = 20;
            int n = 200;
            // 3 FOOD (caviar 240), 1 MEDICAL (surgery_tool 200), 1 MATERIAL (metal_ingot 50), 1 WEAPON (pipe_weapon 15)
            var setup = new[] { "caviar", "surgery_tool", "metal_ingot", "pipe_weapon" };
            var referral = StoreService.GetBuyReferalService();
            foreach (var (label, refOn, s1, s2, vand, wantN) in new[]
            {
                ("base", false, false, false, false, 1),
                ("buy referral", true, false, false, false, 2),
                ("showcase I", false, true, false, false, 2),
                ("referral + I + II", true, true, true, false, 4),
                ("referral + I + II, vandalized", true, true, true, true, 1),
                ("I, vandalized", false, true, false, true, 1),
            })
            {
                Fill(setup);
                referral.unlocked = true;
                referral.isUsing = refOn;
                SetUpgrade("SHOWCASE_I", s1);
                SetUpgrade("SHOWCASE_II", s2);
                W.PS.isVandalized = vand;
                int formula = W.PS.GetShowcaseTypeVisitorCount();
                var counts = new List<int>();
                var types = new Dictionary<string, int>();
                for (int i = 0; i < n; i++)
                {
                    var q = Run(Mgr.HandleShowcaseClient);
                    counts.Add(q.Count);
                    foreach (var c in q) types[c.attractedType ?? "?"] = types.GetValueOrDefault(c.attractedType ?? "?") + 1;
                }
                // Doc formula: 1 + referral + I + II - (1 per tier if vandalized) - 1 if vandalized, floor 1.
                int doc = 1 + (refOn ? 1 : 0) + (s1 ? 1 : 0) + (s2 ? 1 : 0) - (vand ? (s1 ? 1 : 0) + (s2 ? 1 : 0) + 1 : 0);
                doc = Math.Max(1, doc);
                R.Exact($"display buyers, 4 types in case, {label}: N per opening (doc formula {doc})", doc, counts.Distinct().Count() == 1 ? counts[0] : -1, n,
                    "types seen: " + string.Join(",", types.Select(kv => kv.Key + "=" + kv.Value)) + $"; GetShowcaseTypeVisitorCount={formula}");
            }
            SetUpgrade("SHOWCASE_I", false); SetUpgrade("SHOWCASE_II", false); W.PS.isVandalized = false; referral.isUsing = false;
            // Ranking by summed displayed value: 3 x caviar FOOD beats surgery_tool MEDICAL.
            {
                Fill("surgery_tool", "nutrifruit", "nutrifruit");
                var q = Run(Mgr.HandleShowcaseClient);
                R.Exact("N=1, MEDICAL 200 vs FOOD 2x12: top type by value", "MEDICAL", q.Select(c => c.attractedType).FirstOrDefault());
                Fill("surgery_tool", "caviar", "caviar");
                q = Run(Mgr.HandleShowcaseClient);
                R.Exact("N=1, MEDICAL 200 vs FOOD 2x240: top type by value", "FOOD", q.Select(c => c.attractedType).FirstOrDefault());
            }
            // Attractiveness gate.
            foreach (var (att, want) in new[] { (99, 0), (100, 1) })
            {
                Fill("caviar");
                W.PS.projectorAttractivenessBonus = att - W.PS.baseStoreAttractiveness + W.PS.GetTrashAttPenalty();
                var q = Run(Mgr.HandleShowcaseClient);
                R.Exact($"attractiveness {W.PS.GetCurrentStoreAttractiveness()}: display buyers", want, q.Count);
            }
            W.PS.projectorAttractivenessBonus = 0;
            // Empty case.
            {
                W.ClearInventory(Case);
                R.Exact("empty case: no display buyer", 0, Run(Mgr.HandleShowcaseClient).Count);
            }
            // Contribution of non-major types: a LUXURY-only item (box_tampon is HOUSEHOLD_GOOD+LUXURY) counts as HOUSEHOLD.
            {
                Fill("box_tampon");
                var q = Run(Mgr.HandleShowcaseClient);
                R.Exact("box_tampon (HOUSEHOLD_GOOD + LUXURY_ITEM) alone: attracted type", "HOUSEHOLD_GOOD", q.Select(c => c.attractedType).FirstOrDefault());
            }
        }

        public static void Tiers(int seed)
        {
            W.New(seed);
            var tier1 = new[] { "spacerFoodI", "hungrySpacer", "spacerChef" };
            foreach (var (day, ids, att, want) in new[]
            {
                (20, new[] { "nutrifruit" }, 250, "t1"),                                 // 12
                (20, new[] { "large_fish_fillet" }, 250, "t1"),                          // 143
                (20, new[] { "large_fish_fillet", "zerochew" }, 250, "expeditionSupplier"), // 151
                (20, new[] { "caviar", "large_fish_fillet" }, 250, "privateChef"),       // 383
                (13, new[] { "caviar", "large_fish_fillet" }, 250, "expeditionSupplier"),// day < 14
                (20, new[] { "nutrifruit" }, 400, "expeditionSupplier"),                 // attractiveness 400 promotes
            })
            {
                W.Day = day;
                W.PS.projectorAttractivenessBonus = att - W.PS.baseStoreAttractiveness + W.PS.GetTrashAttPenalty();
                Fill(ids);
                long total = Case.childItems.Sum(i => i.GetCurrentValue(false, true, true, false, 0));
                var seen = new Dictionary<string, int>();
                int n = 300;
                for (int i = 0; i < n; i++)
                {
                    var q = Run(Mgr.HandleShowcaseClient);
                    string id = q.Select(c => c.identifier).FirstOrDefault() ?? "none";
                    seen[id] = seen.GetValueOrDefault(id) + 1;
                }
                bool ok = want == "t1" ? seen.Keys.All(k => tier1.Contains(k)) : seen.Keys.SequenceEqual(new[] { want });
                R.Exact($"FOOD tier, case value {total}, day {day}, attractiveness {W.PS.GetCurrentStoreAttractiveness()}: buyer ({want})",
                    true, ok, n, string.Join(",", seen.Select(kv => kv.Key + "=" + kv.Value)));
            }
            W.PS.projectorAttractivenessBonus = 0;
        }

        public static void Discerning(int seed)
        {
            W.New(seed);
            W.Day = 20;
            var ul = W.RepOf("FACTION_UPPER_LEVEL");
            foreach (var rep in new[] { 0, 50, 100 })
            {
                W.SetRep("FACTION_UPPER_LEVEL", rep);
                bool perk = StoreReputation.IsPerkUnlocked("UL_DISCERNING_TASTE");
                Fill("skincare_cream");
                int chance = ShowcaseHelper.GetDiscerningShopperChance();
                int n = 4000, hit = 0;
                for (int i = 0; i < n; i++)
                {
                    Mgr.clientTracker.Remove("ulDiscerningShopper");
                    var q = Run(Mgr.HandlePerkClient);
                    if (q.Any(c => c.identifier == "ulDiscerningShopper")) hit++;
                }
                R.Info($"UL rep {rep}: UL_DISCERNING_TASTE unlocked", perk);
                R.Rate($"discerning shopper, UL rep {rep}, luxury in case, day 20: chance 1 + rep/5 (game says {chance})", (1 + rep / 5) / 100.0, hit, n);
            }
            // Cooldown: after a visit, how many openings until the next possible roll.
            {
                W.SetRep("FACTION_UPPER_LEVEL", 200);
                Fill("skincare_cream");
                Mgr.clientTracker.Remove("ulDiscerningShopper");
                var gaps = new List<int>();
                int last = -1;
                for (int day = 0; day < 3000 && gaps.Count < 200; day++)
                {
                    // One opening: the tracker ticks, then the perk step rolls.
                    Mgr.UpdateTracker();
                    var q = Run(Mgr.HandlePerkClient);
                    var shopper = q.FirstOrDefault(c => c.identifier == "ulDiscerningShopper");
                    if (shopper != null)
                    {
                        // Cooldown starts on arrival (StoreClient.OnArrival).
                        Mgr.TrackClientVisit(shopper);
                        if (last >= 0) gaps.Add(day - last);
                        last = day;
                    }
                }
                R.Info("discerning shopper at UL 200 (41%): min openings between visits (doc: next possible at D+5)", gaps.Count > 0 ? gaps.Min() : -1, gaps.Count,
                    "gap histogram " + string.Join(",", gaps.GroupBy(g => g).OrderBy(g => g.Key).Take(8).Select(g => g.Key + ":" + g.Count())));
            }
        }

        public static void Security(int seed)
        {
            W.New(seed);
            W.Day = 20;
            foreach (var id in new[] { "dream_cap_seed", "dream_dust", "c4", "crypto_module_cmd" })
            {
                Fill(id);
                int lvl = ContrabandHelper.GetContrabandLevelInInt(Case.childItems[0]);
                int n = 200, hit = 0;
                for (int i = 0; i < n; i++)
                    if (Run(Mgr.HandleShowCaseSecurity).Any(c => c.clientIntent == StoreClient.ClientIntent.INSPECTION)) hit++;
                R.Rate($"contraband level {lvl} ({id}) in case: guaranteed showcase inspector", 1.0, hit, n);
            }
            foreach (var heat in new[] { 10, 25, 50, 75 })
            {
                int n = 4000, hit = 0, heat100 = 0;
                for (int i = 0; i < n; i++)
                {
                    Fill("metal_ingot");
                    var it = Case.childItems[0];
                    StolenHelper.InitStolenItem(it, heat);
                    if (Run(Mgr.HandleShowCaseSecurity).Any(c => c.clientIntent == StoreClient.ClientIntent.INSPECTION))
                    {
                        hit++;
                        if (StolenHelper.GetFoundChance(it) == 100) heat100++;
                    }
                }
                R.Rate($"stolen heat {heat} in case: showcase inspector chance = heat%", heat / 100.0, hit, n);
                R.Rate($"stolen heat {heat}: found item heat set to 100", 1.0, heat100, hit);
            }
            // A full opening with contraband in the case: the inspector is first in the queue (generated very early).
            {
                Fill("c4");
                var q = W.Open();
                var insp = q.FindIndex(c => c.clientIntent == StoreClient.ClientIntent.INSPECTION);
                R.Info("full opening, c4 in case: position of the inspector in the queue (0 = first)", insp, 1, "queue: " + string.Join(",", q.Select(c => c.identifier)));
                var inspector = insp >= 0 ? q[insp] : null;
                R.Info("showcase inspector identity", inspector == null ? "none" : inspector.identifier + " lazy=" + inspector.isLazyInspector + " name=" + inspector.displayName);
            }
        }
    }
}
