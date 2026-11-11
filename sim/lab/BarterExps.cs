using System;
using System.Collections.Generic;
using System.Linq;

namespace Fence.Lab
{
    // docs/re/BARTER.md: bargain rolls, budget effects, coercion, rep deltas.
    public static class BarterExps
    {
        const string LL = "FACTION_LOWER_LEVEL", BM = "FACTION_BLACK_MARKET", UL = "FACTION_UPPER_LEVEL";
        static BargainUIManager B => BargainUIManager.Instance;

        static int Clamp100(int v) => Math.Max(0, Math.Min(100, v));

        // BARTER.md: rep/6 (x2 if negative); integer division truncates toward zero.
        static int RepBonus(int rep) { int b = rep / 6; return b < 0 ? b * 2 : b; }
        static int RepBonusBm(int rep) { int b = rep / 4; return b < 0 ? b * 2 : b; }

        // Add Markup (client buying) and Demand Discount (client selling): accept / stay / walk.
        public static void Rolls(int seed)
        {
            W.New(seed);
            int n = Program.N;
            var cases = new List<(string kind, int pct, int rep, bool ev)>();
            foreach (var pct in new[] { 10, 25, 50, 75 }) cases.Add(("markup", pct, 0, false));
            foreach (var pct in new[] { 25, 50 }) { cases.Add(("markup", pct, 60, false)); cases.Add(("markup", pct, -30, false)); cases.Add(("markup", pct, 0, true)); }
            foreach (var pct in new[] { 10, 25, 50, 75 }) cases.Add(("demand", pct, 0, false));
            cases.Add(("demand", 50, 60, false));
            cases.Add(("demand", 50, -30, false));
            cases.Add(("demand", 50, 0, true));
            foreach (var (kind, pct, rep, ev) in cases)
            {
                int acc = 0, stay = 0, walk = 0;
                var walkDeltas = new List<double>();
                var cuts = new List<double>();
                for (int i = 0; i < n; i++)
                {
                    W.SetRep(LL, rep);
                    var c = kind == "markup"
                        ? W.Seat("merchant", x => { if (ev) x.eventSourceId = "lab"; })
                        : W.Seat("scavGeneral", x => { x.clientIntent = StoreClient.ClientIntent.SELL; if (ev) x.eventSourceId = "lab"; });
                    int b0 = c.GetBudget();
                    double r0 = W.Rep(LL);
                    if (kind == "markup") B.OfferMarkup(pct); else B.DemandBuyingDiscount(pct);
                    bool applied = W.Get<bool>(B, kind == "markup" ? "markupApplied" : "buyingDiscountApplied");
                    if (applied)
                    {
                        acc++;
                        if (kind == "markup" && b0 > 0) cuts.Add(1.0 - (double)c.GetBudget() / b0);
                    }
                    else if (c.clientIntent == StoreClient.ClientIntent.DIALOGUE) { walk++; walkDeltas.Add(W.Rep(LL) - r0); }
                    else stay++;
                    W.Unseat();
                }
                int bonus = RepBonus(rep) + (ev ? 15 : 0);
                int pAcc = Clamp100((int)Math.Round(100 - 2.0 * pct + bonus, MidpointRounding.ToEven));
                int pStay = Clamp100((int)Math.Round(100 - 1.2 * pct + bonus, MidpointRounding.ToEven));
                string tag = $"{kind} {pct}% rep {rep}{(ev ? " event" : "")}";
                R.Rate(tag + ": accept = 100-2p+rep/6'+event", pAcc / 100.0, acc, n);
                R.Rate(tag + ": refuse-but-stay = (100-1.2p+rep/6'+event) - accept", Math.Max(0, pStay - pAcc) / 100.0, stay, n);
                R.Rate(tag + ": walk-out = 1 - stay band", (100 - Math.Max(pStay, pAcc)) / 100.0, walk, n);
                if (walkDeltas.Count > 0) R.Exact(tag + ": walk-out rep delta (LL, normal)", -3.0, walkDeltas.Average(), walkDeltas.Count,
                    $"min {walkDeltas.Min():0.##} max {walkDeltas.Max():0.##}");
                if (cuts.Count > 0)
                {
                    double want = pct switch { 10 => 0.10, 25 => 0.15, 50 => 0.20, _ => 0.25 };
                    R.Near(tag + ": markup success cuts budget by 10/15/20/25%", want, cuts.Average(), 0.01, cuts.Count, $"min {cuts.Min():0.###} max {cuts.Max():0.###}");
                }
            }
        }

        // Offer Discount and Offer Markup (you buy) are always accepted; Offer Discount raises the budget 2*p %.
        public static void Generous(int seed)
        {
            W.New(seed);
            int n = Program.N / 4;
            foreach (var pct in new[] { 10, 25, 50, 75 })
            {
                int ok = 0; var raise = new List<double>();
                for (int i = 0; i < n; i++)
                {
                    var c = W.Seat("merchant");
                    int b0 = c.GetBudget();
                    B.OfferDiscount(pct);
                    if (W.Get<bool>(B, "discountOffered") && c.clientIntent == StoreClient.ClientIntent.BUY) ok++;
                    raise.Add((double)c.GetBudget() / b0 - 1);
                    W.Unseat();
                }
                R.Rate($"offer discount {pct}%: always accepted (client stays BUY)", 1.0, ok, n);
                R.Near($"offer discount {pct}%: budget raised by 2p%", 2.0 * pct / 100, raise.Average(), 0.01, n, $"min {raise.Min():0.###} max {raise.Max():0.###}");
            }
            foreach (var pct in new[] { 25, 75 })
            {
                int ok = 0;
                for (int i = 0; i < n; i++)
                {
                    var c = W.Seat("scavGeneral", x => x.clientIntent = StoreClient.ClientIntent.SELL);
                    B.OfferBuyingMarkup(pct);
                    if (W.Get<bool>(B, "buyingMarkupOffered") && c.clientIntent == StoreClient.ClientIntent.SELL) ok++;
                    W.Unseat();
                }
                R.Rate($"offer markup (you buy) {pct}%: always accepted", 1.0, ok, n);
            }
        }

        // Blackmail: criminals 60 + rep/4' + event; failure -5 rep, stays. Innocent: -7 or (50%) -10 and walks.
        public static void Blackmail(int seed)
        {
            W.New(seed);
            int n = Program.N;
            foreach (var (rep, ev, fac, id) in new[] { (0, false, LL, "merchant"), (40, false, LL, "merchant"), (-20, false, LL, "merchant"), (0, true, LL, "merchant"), (0, false, BM, "criminalBuyerThief") })
            {
                int succ = 0; var failDelta = new List<double>(); int failWalk = 0;
                for (int i = 0; i < n; i++)
                {
                    W.SetRep(fac, rep);
                    var c = W.Seat(id, x => { x.isWanted = true; if (ev) x.eventSourceId = "lab"; });
                    double r0 = W.Rep(fac);
                    B.Blackmail();
                    if (W.Get<bool>(B, "blackmailApplied")) succ++;
                    else
                    {
                        failDelta.Add(W.Rep(fac) - r0);
                        if (c.clientIntent == StoreClient.ClientIntent.DIALOGUE) failWalk++;
                    }
                    W.Unseat();
                }
                string tag = $"blackmail criminal ({fac.Replace("FACTION_", "")}) rep {rep}{(ev ? " event" : "")}";
                R.Rate(tag + ": success = 60 + rep/4' + event", Clamp100(60 + RepBonusBm(rep) + (ev ? 15 : 0)) / 100.0, succ, n);
                if (failDelta.Count > 0)
                {
                    R.Exact(tag + ": failure rep delta -5 (BARTER Q&A #1; BM x2 'presumed')", fac == BM ? -10.0 : -5.0, Math.Round(failDelta.Average(), 3), failDelta.Count);
                    R.Rate(tag + ": failure, client stays", 0, failWalk, failDelta.Count);
                }
            }
            // Innocent client and false threaten share the structure.
            foreach (var (kind, fac, id) in new[] { ("blackmail innocent", LL, "merchant"), ("threaten non-aug", LL, "merchant"), ("blackmail innocent", BM, "criminalBuyerThief") })
            {
                int walk = 0, ten = 0, seven = 0, other = 0, walkTen = 0;
                for (int i = 0; i < n; i++)
                {
                    W.SetRep(fac, 0);
                    var c = W.Seat(id, x => { x.isWanted = false; x.isAug = false; });
                    if (kind.StartsWith("blackmail")) B.Blackmail(); else B.ThreatenAug();
                    double d = W.Rep(fac);
                    bool walked = c.clientIntent == StoreClient.ClientIntent.DIALOGUE;
                    if (walked) walk++;
                    double mult = fac == BM ? 2 : 1;
                    if (Math.Abs(d + 10 * mult) < 1e-6) { ten++; if (walked) walkTen++; }
                    else if (Math.Abs(d + 7 * mult) < 1e-6) seven++;
                    else other++;
                    W.Unseat();
                }
                string tag = $"{kind} ({fac.Replace("FACTION_", "")})";
                R.Rate(tag + ": walks out 50%", 0.5, walk, n);
                R.Rate(tag + $": rep {(fac == BM ? "-20 (x2 BM)" : "-10")} exactly when walking", 1.0, walkTen, walk);
                R.Exact(tag + ": every outcome is -10 or -7 (x faction mult)", 0, other, n, $"-10: {ten}, -7: {seven}");
            }
            // Threaten an aug: always succeeds.
            {
                int ok = 0, nn = n / 4;
                for (int i = 0; i < nn; i++)
                {
                    W.Seat("merchant", x => x.isAug = true);
                    B.ThreatenAug();
                    if (W.Get<bool>(B, "threatenApplied")) ok++;
                    W.Unseat();
                }
                R.Rate("threaten aug: always succeeds", 1.0, ok, nn);
            }
            // Aug clients refuse Add Markup without walking.
            {
                int acc = 0, walk = 0, nn = n / 4;
                for (int i = 0; i < nn; i++)
                {
                    var c = W.Seat("merchant", x => x.isAug = true);
                    B.OfferMarkup(10);
                    if (W.Get<bool>(B, "markupApplied")) acc++;
                    if (c.clientIntent == StoreClient.ClientIntent.DIALOGUE) walk++;
                    W.Unseat();
                }
                R.Rate("aug + add markup 10%: always refuses", 0, acc, nn);
                R.Rate("aug + add markup 10%: never walks", 0, walk, nn);
            }
            // One-shot guards.
            {
                W.Seat("merchant", x => x.isWanted = true);
                B.Blackmail();
                bool again = B.CanBlackmail(), threat = B.CanThreatenAug(), disc = B.CanOfferDiscount(), mark = B.CanOfferMarkup();
                R.Exact("after blackmail: CanBlackmail false", false, again);
                R.Exact("after blackmail: CanThreatenAug still true (Threaten checks threatenOffered only)", true, threat);
                R.Exact("after blackmail: CanOfferDiscount false (no discount after coercion)", false, disc);
                R.Exact("after blackmail: CanOfferMarkup still true", true, mark);
                W.Unseat();
                W.Seat("merchant", x => x.isSecurity = true);
                R.Exact("security client: CanBlackmail false", false, B.CanBlackmail());
                R.Exact("security client: CanThreatenAug false", false, B.CanThreatenAug());
                W.Unseat();
                W.Seat("merchant");
                B.OfferMarkup(10);
                bool m2 = B.CanOfferMarkup(), d2 = B.CanOfferDiscount();
                R.Exact("after add markup (any result): second markup refused", false, m2);
                R.Exact("after add markup (any result): offer discount refused (one per side)", false, d2);
                W.Unseat();
            }
            // IsWanted: selling stolen or serious contraband makes a non-wanted seller a 'criminal'.
            {
                var c = W.Seat("scavGeneral", x => { x.isWanted = false; x.clientIntent = StoreClient.ClientIntent.SELL; });
                bool before = c.IsWanted();
                var it = W.Make("metal_ingot");
                StolenHelper.InitStolenItem(it, 50);
                W.Place(W.E.frontInvinvElement, it);
                bool after = c.IsWanted();
                R.Exact("IsWanted: non-wanted seller", false, before);
                R.Exact("IsWanted: same seller with a stolen item on the front counter", true, after);
                W.E.frontInvinvElement.Expel(it);
                W.Unseat();
            }
        }

        // Trade rep: value x tradeMult / 400 x faction factor (LL 0.66, UL 1.2), BM losses x2, bargains change the multiplier.
        public static void TradeRep(int seed)
        {
            W.New(seed);
            W.Cash = 100000;
            foreach (var (fac, id, factor) in new[] { (LL, "merchant", 0.66), (UL, "hospitalAdmin", 1.2), (BM, "criminalBuyerThief", 1.0) })
            {
                foreach (var bargain in new[] { "none", "markup25", "discount25", "blackmail" })
                {
                    var deltas = new List<double>(); var expect = new List<double>();
                    for (int i = 0; i < 40; i++)
                    {
                        W.SetRep(fac, 0);
                        var c = W.Seat(id, x => { x.isWanted = true; x.useClientBudget = true; });
                        W.Set(c, "clientBudget", 100000);
                        var it = W.Own("metal_ingot", W.E.backInvinvElement);
                        long pre = it.GetNegociatedValue();
                        bool applied = true;
                        if (bargain == "markup25") { B.OfferMarkup(25); applied = W.Get<bool>(B, "markupApplied"); }
                        if (bargain == "discount25") B.OfferDiscount(25);
                        if (bargain == "blackmail") { B.Blackmail(); applied = W.Get<bool>(B, "blackmailApplied"); }
                        Fence.Sim.Scheduler.Settle();
                        if (!applied || W.PS.currentClientInstance == null) { W.Unseat(); W.ClearInventory(W.E.backInvinvElement); continue; }
                        double r0 = W.Rep(fac);
                        W.PS.SellItem(it);
                        deltas.Add(W.Rep(fac) - r0);
                        // BARTER.md: nice +2 at <=25 → mult 1+1.2? use the doc's multiplier table on top of 1.
                        double mult = bargain switch { "none" => 1, "markup25" => -1.5, "discount25" => 1 + 1.2, "blackmail" => -6, _ => 1 };
                        double v = Math.Round(pre * mult, MidpointRounding.ToEven);
                        v = Math.Round(v * factor, MidpointRounding.ToEven);
                        if (v < 0 && fac == BM) v *= 2;
                        expect.Add(v / 400.0);
                        W.Unseat();
                        W.ClearInventory(W.E.backInvinvElement);
                    }
                    if (deltas.Count == 0) { R.Info($"trade rep {fac} {bargain}", "no applied trials"); continue; }
                    R.Near($"trade rep, sell ingot to {fac.Replace("FACTION_", "")} after {bargain}: delta = value x mult x faction / 400 (pre-bargain value)",
                        expect.Average(), deltas.Average(), Math.Max(0.002, Math.Abs(expect.Average()) * 0.02), deltas.Count,
                        $"measured min {deltas.Min():0.####} max {deltas.Max():0.####}");
                }
            }
            // Buying: stolen multiplier 1.5/2/2.5/3 by heat, serious contraband level/2+1.
            foreach (var heat in new[] { 0, 25, 50, 75, 100 })
            {
                W.SetRep(LL, 0);
                W.Seat("scavGeneral", x => x.clientIntent = StoreClient.ClientIntent.SELL);
                var it = W.Make("metal_ingot");
                if (heat > 0) StolenHelper.InitStolenItem(it, heat);
                W.Place(W.E.backInvinvElement, it);
                long v = it.GetNegociatedValue();
                double r0 = W.Rep(LL);
                W.PS.BuyItem(it);
                double d = W.Rep(LL) - r0;
                double m = heat == 0 ? 1 : heat <= 25 ? 1.5 : heat <= 50 ? 2 : heat <= 75 ? 2.5 : 3;
                double e = Math.Round(Math.Round(Math.Round((double)v) * m, MidpointRounding.ToEven) * 0.66, MidpointRounding.ToEven) / 400;
                R.Near($"trade rep, BUY ingot (value {v}) from LL, heat {heat}: x{m} stolen bonus", e, d, 0.002, 1);
                W.Unseat();
                W.ClearInventory(W.E.backInvinvElement);
                W.ClearInventory(W.E.invElement);
            }
        }

        // Q&A #2-#4 and SPEC §6.4: contraband refusal, demand discount vs wholesale, crime on buy/sell.
        public static void Crime(int seed)
        {
            W.New(seed);
            W.Cash = 100000;
            foreach (var id in new[] { "merchant", "lowerRepairman", "familyMother", "hungrySpacer" })
            {
                int n = 2000, k = 0;
                for (int i = 0; i < n; i++) if (W.NewClient(id).noAcceptingContraband) k++;
                R.Info($"noAcceptingContraband share, {id} (BARTER Q&A #3: 35% of LOWER_LEVEL_CITIZEN clients)", $"{100.0 * k / n:0.0}%", n);
            }
            {
                var c = W.Seat("merchant", x => { x.noAcceptingContraband = true; x.clientBuyingTagList.Add("WEAPON"); });
                var it = W.Own("heavy_handmade_pistol", W.E.backInvinvElement);
                R.Exact("noAcceptingContraband client + contraband pistol: IsClientBuyingThisItem", false, c.IsClientBuyingThisItem(it, false));
                R.Exact("... PlayerStore.CanSellThisItem", false, W.PS.CanSellThisItem(it, false));
                W.Unseat(); W.ClearInventory(W.E.backInvinvElement);
            }
            {
                int w0 = 0, w1 = 0;
                for (int tries = 0; tries < 50; tries++)
                {
                    W.Seat("scavGeneral", x => { x.clientIntent = StoreClient.ClientIntent.SELL; x.isOfferWholesale = true; x.wholesaleDiscount = -15; });
                    for (int i = 0; i < 3; i++) W.Place(W.E.frontInvinvElement, W.Make("metal_ingot"));
                    w0 = W.PS.GetWholesalePrice();
                    B.DemandBuyingDiscount(25);
                    bool ok = W.Get<bool>(B, "buyingDiscountApplied");
                    w1 = W.PS.GetWholesalePrice();
                    W.Unseat(); W.ClearInventory(W.E.frontInvinvElement);
                    if (ok) break;
                }
                R.Near("demand discount 25% accepted: wholesale price falls ~25% (BARTER Q&A #2)", 0.75, (double)w1 / w0, 0.02, 1, $"{w0} -> {w1}");
            }
            var sec = W.PS.secData;
            int Total() => sec.crimeAmount.Sum();
            string Crimes() => string.Join(",", sec.crimeTypes.Select((t, i) => t + "=" + sec.crimeAmount[i]));
            {
                W.Seat("scavGeneral", x => x.clientIntent = StoreClient.ClientIntent.SELL);
                var it = W.Make("surgery_tool"); StolenHelper.InitStolenItem(it, 80);
                W.Place(W.E.backInvinvElement, it);
                int t0 = Total();
                W.PS.BuyItem(it);
                R.Exact("buying a stolen item adds no crime (Q&A #4)", 0, Total() - t0, 1, "crimes: " + Crimes());
                W.Unseat(); W.ClearInventory(W.E.invElement);
            }
            foreach (var (buyer, fac) in new[] { ("merchant", "LL"), ("criminalBuyerThief", "BM") })
            {
                sec.crimeTypes.Clear(); sec.crimeAmount.Clear();
                var c = W.Seat(buyer, x => { x.useClientBudget = true; x.clientBuyingTagList.Add("MEDICAL"); });
                W.Set(c, "clientBudget", 100000);
                var s = W.Own("surgery_tool", W.E.backInvinvElement); StolenHelper.InitStolenItem(s, 80);
                long v = s.GetNegociatedValue();
                W.PS.SellItem(s);
                int fencing = sec.crimeTypes.Select((t, i) => t == "FENCING" ? sec.crimeAmount[i] : 0).Sum();
                double want = Math.Round(v * 0.8) / (fac == "BM" ? 2 : 1);
                R.Near($"sell stolen (heat 80, value {v}) to {fac}: FENCING = value x heat/100{(fac == "BM" ? " / 2" : "")}", want, fencing, 1.01, 1, "crimes: " + Crimes());
                W.Unseat(); W.ClearInventory(W.E.backInvinvElement);

                sec.crimeTypes.Clear(); sec.crimeAmount.Clear();
                c = W.Seat(buyer, x => { x.useClientBudget = true; x.noAcceptingContraband = false; x.clientBuyingTagList.Add("WEAPON"); });
                W.Set(c, "clientBudget", 100000);
                var g = W.Own("heavy_handmade_pistol", W.E.backInvinvElement);
                long gv = g.GetNegociatedValue();
                W.PS.SellItem(g);
                R.Near($"sell contraband pistol (value {gv}) to {fac}: trafficking = value{(fac == "BM" ? " / 2" : "")}", fac == "BM" ? gv / 2.0 : gv, sec.GetTotalTraffickingCrimes(), 1.01, 1, "crimes: " + Crimes());
                W.Unseat(); W.ClearInventory(W.E.backInvinvElement);
            }
        }

        // Threaten unlock day: when isAugIntroduced flips during ordinary (gym) play, by seed.
        public static void ThreatenDay(int seed) => AugDay(seed, false);
        public static void ThreatenDaySkipIntro(int seed) => AugDay(seed, true);

        static void AugDay(int seed, bool skip)
        {
            for (int s = 1; s <= 1; s++)
            {
                W.New(seed + s, skipIntro: skip);
                int found = -1;
                int augDay = PlayerStore.GetAugIntroducedDay();
                while (W.Day < 30 && !W.Over)
                {
                    if (W.PS.isAugIntroduced) { found = W.Day; break; }
                    if (!W.Ep.Step()) break;
                }
                R.Info($"isAugIntroduced first true (seed {seed + s}, gym heuristic play, skipIntro={skip})", found < 0 ? "not by day 30" : "day " + found, 1, $"GetAugIntroducedDay() = {augDay}");
            }
        }
    }
}
