using System;
using System.Collections.Generic;
using System.Linq;
using Fence.Gym;
using Fence.Sim;

namespace Fence.Lab
{
    // docs/re/STATE.md, DAY.md, docs/sim/SPEC.md §6: heat, concealment, inspections, rent, buyout, market clients.
    public static class StateExps
    {
        static StoreClientManager Mgr => W.PS.storeClientManager;

        // Heat decays 18 per night, less 2/4/6/8 for contraband LOW/MID/HIGH/CRITICAL; 0 = clean. Real nights.
        public static void Heat(int seed)
        {
            W.New(seed);
            W.Cash = 100000;
            var items = new List<(string id, GameItem it)>();
            foreach (var id in new[] { "metal_ingot", "eng_keycard", "dream_dust", "c4", "crypto_module_cmd" })
            {
                var it = W.Own(id);
                StolenHelper.InitStolenItem(it, 100);
                items.Add((id, it));
            }
            var trace = items.ToDictionary(x => x.id, x => new List<int> { StolenHelper.GetFoundChance(x.it) });
            for (int night = 0; night < 9 && !W.Over; night++)
            {
                W.QuietDay();
                foreach (var (id, it) in items) trace[id].Add(StolenHelper.IsStolenItem(it) ? StolenHelper.GetFoundChance(it) : 0);
            }
            foreach (var (id, it) in items)
            {
                int lvl = ContrabandHelper.GetContrabandLevelInInt(it);
                int step = 18 - 2 * lvl;
                var t = trace[id];
                var want = Enumerable.Range(0, t.Count).Select(k => Math.Max(0, 100 - step * k)).ToList();
                R.Exact($"heat decay, {id} (contraband {lvl}): 100 then -{step}/night, clean at 0", string.Join(",", want), string.Join(",", t), t.Count - 1,
                    "still stolen after: " + StolenHelper.IsStolenItem(it));
            }
            // Initial heat by base value.
            foreach (var (id, want) in new[] { ("scrap_metal", 0), ("common_ore", 0), ("flux_agent", 0), ("metal_ingot", 50), ("surgery_tool", 100), ("caviar", 100), ("large_fish_fillet", 100), ("box_tampon", 75), ("nudka", 75), ("pipe_weapon", 0), ("box_cutter", 25) })
            {
                var it = W.Make(id);
                StolenHelper.InitStolenItem(it);
                R.Exact($"initial heat of stolen {id} (base {it.GetBaseValue()})", want, StolenHelper.IsStolenItem(it) ? StolenHelper.GetFoundChance(it) : 0);
            }
        }

        // Smuggler bays (and ITEM_HIDDEN_TAG containers) block the search; the hidden stash is searched only with the poster pulled.
        public static void Conceal(int seed)
        {
            W.New(seed);
            foreach (var bayId in new[] { "smuggler_bay", "smuggler_bay_mini", "smuggler_bay_mod", "none" })
            {
                W.ClearInventory(W.E.invElement);
                var contra = W.Make("c4");
                GeneralHelper.SetItemOwned(contra, true);
                GameItem bay = null;
                if (bayId != "none")
                {
                    bay = W.Own(bayId);
                    var inner = GraphUtils.FindChildrenType<GameInventory>(bay).FirstOrDefault();
                    if (inner == null || GraphUtils.TryAcceptAll(inner, contra, -1) != 0)
                    {
                        R.Info($"conceal: could not put c4 into {bayId}", inner == null ? "no inner inventory" : "rejected");
                        continue;
                    }
                }
                else W.Place(W.E.invElement, contra);
                var found = new List<GameItem>(); var concealed = new List<GameItem>();
                foreach (var it in W.E.invElement.childItems) ContrabandHelper.RecursiveInspect(it, found, concealed);
                R.Info($"inspection search, c4 inside {bayId} in stock: found / concealed", $"found={found.Contains(contra)} concealed={concealed.Contains(contra)}", 1,
                    bay == null ? null : $"bay tags hidden={bay.IsTag("ITEM_HIDDEN_TAG")} IsSmugglerBay={ContrabandHelper.IsSmugglerBay(bay)}");
            }
            // Poster stash.
            foreach (bool pulled in new[] { false, true })
            {
                var stash = W.E.hiddenElement;
                W.ClearInventory(stash);
                var contra = W.Make("c4");
                GeneralHelper.SetItemOwned(contra, true);
                if (!W.Place(stash, contra))
                {
                    // Real footprints: the search rule is what is under test, so the stash is enlarged for it.
                    R.Info("c4 fits the poster stash", $"no: stash {stash.inventoryShape.width}x{stash.inventoryShape.height}, c4 {contra.GetItemSize()} cells");
                    stash.SetShape(Math.Max(stash.inventoryShape.width, 8), Math.Max(stash.inventoryShape.height, 8));
                    if (!W.Place(stash, contra)) throw new InvalidOperationException("could not place c4 in the enlarged stash");
                }
                StorePoster.Instance.isPulled = pulled;
                // StartInspection with a plain inspector seated; count c4 seized (it leaves the stash).
                W.Seat("merchant", c => { c.clientIntent = StoreClient.ClientIntent.INSPECTION; c.isLazyInspector = false; });
                int before = W.E.hiddenElement.childItems.Count;
                bool over = false;
                try { ContrabandHelper.StartInspection(0); } catch (Exception e) { R.Info("StartInspection threw", Game.Root(e)); }
                Scheduler.Settle();
                over = W.Over;
                R.Info($"full StartInspection, c4 in the poster stash, poster pulled={pulled}: c4 still in stash / game over",
                    $"{W.E.hiddenElement.childItems.Contains(contra)} / {Episode.GameOverReason() ?? "no"}");
                W.Unseat();
                if (over) break;
            }
        }

        // Inspection cadence: interval between scheduled inspectors by trafficking total.
        public static void Inspections(int seed)
        {
            W.New(seed);
            var sec = W.PS.secData;
            foreach (var (crime, total, lo, hi) in new[] { ("DRUG_TRAFFICKING", 40, 0, 0), ("DRUG_TRAFFICKING", 100, 3, 5), ("DRUG_TRAFFICKING", 200, 3, 4), ("DRUG_TRAFFICKING", 350, 3, 3), ("DRUG_TRAFFICKING", 500, 2, 3), ("DRUG_TRAFFICKING", 650, 2, 2), ("DRUG_TRAFFICKING", 800, 1, 2), ("FENCING", 200, 3, 4), ("FRAUD", 800, 0, 0) })
            {
                sec.crimeTypes.Clear(); sec.crimeAmount.Clear();
                sec.crimeTypes.Add(crime); sec.crimeAmount.Add(total);
                W.Set(Mgr, "inspectionSeeded", false);
                var gaps = new List<int>();
                int last = -1;
                for (int day = 0; day < 4000 && gaps.Count < 600; day++)
                {
                    Mgr.clientStack.Clear();
                    Mgr.HandleInspectionClient();
                    if (Mgr.clientStack.Any(c => c.clientIntent == StoreClient.ClientIntent.INSPECTION))
                    {
                        if (last >= 0) gaps.Add(day - last);
                        last = day;
                    }
                }
                Mgr.clientStack.Clear();
                string hist = string.Join(",", gaps.GroupBy(g => g).OrderBy(g => g.Key).Select(g => g.Key + ":" + g.Count()));
                if (hi == 0) { R.Exact($"inspections, {crime} {total}: none scheduled", 0, gaps.Count + (last >= 0 ? 1 : 0), 4000); continue; }
                bool ok = gaps.Count > 0 && gaps.Min() == lo && gaps.Max() == hi;
                R.Exact($"inspections, {crime} {total}: interval range (doc {lo}-{hi} days)", $"{lo}-{hi}", gaps.Count == 0 ? "none" : $"{gaps.Min()}-{gaps.Max()}", gaps.Count, hist);
                if (hi > lo)
                {
                    int k = gaps.Count(g => g == lo);
                    R.Rate($"inspections, {crime} {total}: uniform over {lo}..{hi} (share of {lo})", 1.0 / (hi - lo + 1), k, gaps.Count);
                }
            }
        }

        // Real days at full gym play: rent each landlord visit, buyout price by day.
        public static void Rent(int seed) => RentFor(seed, "normal", 1.05);
        public static void RentCozy(int seed) => RentFor(seed, "cozy", 1.025);
        public static void RentHard(int seed) => RentFor(seed, "hard", 1.1);

        static void RentFor(int seed, string diff, double mult)
        {
            W.New(seed, diff);
            var seen = new List<(int day, int rent)>();
            int prevRent = W.PS.rentValue, paid = 0;
            while (W.Day < 36 && !W.Over)
            {
                W.Cash = Math.Max(W.Cash, 5000);
                if (!W.Ep.Step()) break;
                if (W.PS.rentValue != prevRent) { seen.Add((W.Day, W.PS.rentValue)); prevRent = W.PS.rentValue; }
            }
            var rents = new List<int> { W.PS.startingRent };
            rents.AddRange(seen.Select(s => s.rent));
            var want = new List<int> { rents[0] };
            for (int i = 1; i < rents.Count; i++) want.Add(rents[i - 1]);
            string ratios = string.Join(",", Enumerable.Range(1, rents.Count - 1).Select(i => ((double)rents[i] / rents[i - 1]).ToString("0.####")));
            R.Info($"{diff}: rent value after each change (start {rents[0]}), days of change", string.Join(",", seen.Select(s => $"d{s.day}:{s.rent}")), seen.Count, "ratios " + ratios);
            bool ok = rents.Count > 1 && Enumerable.Range(1, rents.Count - 1).All(i => Math.Abs(rents[i] - rents[i - 1] * mult) <= 1.0);
            R.Exact($"{diff}: each landlord visit multiplies rent by {mult} (rounded)", true, ok, rents.Count - 1, ratios);
            R.Info($"{diff}: game over by day {W.Day}?", Episode.GameOverReason() ?? "no");
        }

        public static void Buyout(int seed)
        {
            foreach (var (diff, b, per, cap) in new[] { ("normal", 12000, 50, 5000) })
            {
                W.New(seed, diff);
                var got = new List<string>(); bool ok = true;
                foreach (var d in new[] { 1, 50, 51, 60, 100, 150, 200 })
                {
                    W.Day = d;
                    int want = b + Math.Min(cap, Math.Max(0, d - 50) * per);
                    int g = W.PS.GetStoreBuyPrice(false);
                    got.Add($"d{d}:{g}");
                    ok &= g == want;
                }
                R.Exact($"{diff}: buyout price = {b} + {per}/day after day 50, cap +{cap}", true, ok, 7, string.Join(",", got));
            }
        }

        // Many real openings (queue generation only): contraband refusal shares, shortage clients under natural events.
        public static void Queue(int seed)
        {
            W.New(seed);
            var em = StoreStation.Instance.storeEventManager;
            int llBuy = 0, llNo = 0, scavBuy = 0, scavNo = 0, shortage = 0, eventDays = 0, days = 0;
            var shortageIds = new HashSet<string>();
            var normalCounts = new List<int>();
            while (days < 400 && !W.Over)
            {
                W.Cash = Math.Max(W.Cash, 3000);
                int normal = W.PS.storeClientManager.GetNormalCustomerCount();
                bool bigEvent = em.GetActiveEvents().SelectMany(e => e.negociationDatas).GroupBy(nd => nd.itemType).Any(gr => Math.Abs(gr.Sum(nd => nd.value)) >= 30);
                if (bigEvent) eventDays++;
                var q = W.QuietDay();
                days++;
                normalCounts.Add(normal);
                foreach (var c in q)
                {
                    bool buys = c.clientIntent == StoreClient.ClientIntent.BUY || c.clientIntent == StoreClient.ClientIntent.SELLNBUY;
                    if (c.IsTag("LOWER_LEVEL_CITIZEN") && buys) { llBuy++; if (c.noAcceptingContraband) llNo++; }
                    if (c.IsTag("SCAV") && buys) { scavBuy++; if (c.noAcceptingContraband) scavNo++; }
                    if (c.identifier != null && (c.identifier.Contains("shortage") || c.identifier.Contains("surplus"))) { shortage++; shortageIds.Add(c.identifier); }
                }
            }
            R.Rate("LOWER_LEVEL_CITIZEN buyers in real queues: noAcceptingContraband 35%", 0.35, llNo, llBuy);
            R.Info("SCAV buyers in real queues: noAcceptingContraband share", $"{100.0 * scavNo / Math.Max(1, scavBuy):0.0}%", scavBuy);
            R.Exact($"{days} real openings, {eventDays} with a >=30% type event active: shortage/surplus clients in the queue", 0, shortage, days, string.Join(",", shortageIds));
            R.Info("normal customer count per opening (attractiveness 250 base)", string.Join(",", normalCounts.GroupBy(x => x).OrderBy(x => x.Key).Select(gr => gr.Key + ":" + gr.Count())), days,
                $"game over: {Episode.GameOverReason() ?? "no"}");
        }

        // SPEC §14.2: seller stock vs reputation (stock spawned by the client's own dialogue actions, as in play).
        public static void SellerStock(int seed)
        {
            W.New(seed);
            W.Day = 20;
            foreach (var (id, fac) in new[] { ("scavGeneral", "FACTION_LOWER_LEVEL"), ("scavHaul", "FACTION_LOWER_LEVEL"), ("foodThief", "FACTION_BLACK_MARKET") })
            {
                foreach (var rep in new[] { -50, 0, 50, 100 })
                {
                    W.SetRep(fac, rep);
                    int n = 300; long items = 0, value = 0, stolen = 0;
                    for (int i = 0; i < n; i++)
                    {
                        W.ClearInventory(W.E.frontInvinvElement);
                        var c = W.Seat(id);
                        try { for (var d = c.mainDialogue; d != null; d = d.nextDialogue) { d.startAction?.Invoke(); d.endAction?.Invoke(); } }
                        catch (Exception) { }
                        foreach (var it in W.E.frontInvinvElement.childItems.Where(x => !GeneralHelper.IsItemOwned(x)))
                        {
                            items++; value += it.GetNegociatedValue();
                            if (StolenHelper.IsStolenItem(it)) stolen++;
                        }
                        W.Unseat();
                    }
                    R.Info($"seller stock, {id}, {fac.Replace("FACTION_", "")} rep {rep}: items / value / stolen share per visit",
                        $"{(double)items / n:0.00} items, {(double)value / n:0} cr, {(items == 0 ? 0 : 100.0 * stolen / items):0}% stolen", n);
                }
                W.SetRep(fac, 0);
            }
        }

        // SPEC/PRODUCTION: HandleMarketClient never adds shortage/surplus clients (GUID key bug).
        public static void Shortage(int seed)
        {
            W.New(seed);
            W.Day = 20;
            var mgr = W.PS.storeClientManager;
            var em = StoreStation.Instance.storeEventManager;
            var results = new List<string>();
            foreach (var evId in new[] { "badNutrifruitHarvest", "medicalSuppliesShortage", "construction_project", "miningExpeditionReturn" })
            {
                var bp = StoreEventManager.normalEventBlueprints.FirstOrDefault(e => e.identifier == evId);
                if (bp == null) { R.Info("shortage: event blueprint missing", evId); continue; }
                var active = W.Get<List<StoreEvent>>(em, "activeEvents");
                active.Clear();
                var ev = bp.storeEventFunc();
                em.ActivateEvent(ev, false, true);
                var mods = W.PS.GetMarketModifierType(TypeHelper.MAJOR_TYPES_LIST);
                mgr.clientStack.Clear();
                mgr.HandleMarketClient();
                var q = mgr.clientStack.Select(c => c.identifier).ToList();
                string modStr = string.Join(";", mods.Select(kv => kv.Key + "=" + kv.Value));
                string dataStr = string.Join(";", ev.negociationDatas.Select(nd => nd.itemType + ":" + nd.value + " id=" + nd.identifier));
                R.Exact($"shortage/surplus client never appears with {evId} active", 0, q.Count, 1,
                    $"queue [{string.Join(",", q)}]; GetMarketModifierType [{modStr}]; negociationDatas [{dataStr}]");
                mgr.clientStack.Clear();
                // The full opening with the event active: no shortage/surplus id in the queue; the event's own client is.
                var full = W.Open().Select(c => c.identifier).ToList();
                R.Info($"full opening with {evId} active: queue", string.Join(",", full), 1,
                    "shortage/surplus ids present: " + full.Any(id => id.Contains("shortage") || id.Contains("surplus")));
                W.CloseAndSleep();
                W.Day = 20;
                W.Get<List<StoreEvent>>(em, "activeEvents").Clear();
            }
        }
    }
}
