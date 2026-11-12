using System;
using System.Collections.Generic;
using System.Linq;
using Fence.Sim;
using static Fence.Lab.ProductionExps;

namespace Fence.Lab
{
    // docs/research/FORMULA-EXPLOITS.md: what an optimising player does with each formula (reusable terms,
    // staging, thresholds), measured on the game's own machines against the way the guide runs them.
    public static class FormulaExps
    {
        static int P(GameItem c, string liquid) => c.GetTagReadonly(Liquid.GetCurrentPartFromId(liquid)).GetInt();

        static double Ml(GameItem c, string liquid) => P(c, liquid) / 1000.0;

        static string Parts(GameItem c) => string.Join(" ", Liquid.Liquids.Select(l => (l.identifier, v: P(c, l.identifier)))
            .Where(x => x.v > 0).Select(x => $"{x.identifier}={x.v / 1000.0:0.###}ml"));

        static int Evaporative(GameItem c) => Liquid.Liquids.Where(l => l.isEvaporative).Sum(l => P(c, l.identifier));

        static List<MachineryButton> Buttons(GameItem m)
        {
            var found = new List<MachineryButton>();
            void Walk(PixelElement e)
            {
                switch (e)
                {
                    case MachineryButton b: found.Add(b); break;
                    case MachineryPanelRow row: foreach (var p in row.Panels) Walk(p); break;
                    case MachineryPanel p: Walk(p.content); break;
                    case MachineryStack s: foreach (var c in s.elements) Walk(c); break;
                }
            }
            Walk(m.contentWindow.childElement);
            return found;
        }

        static void Press(MachineryButton b) => W.Get<Action>(b, "callback")();

        static GraphNodeStorage Storage(GameItem m, int i) => (GraphNodeStorage)MachineHelper.GetBodyStorage(m, i);

        static GameItem ChemMachine(string id, int modules)
        {
            var m = W.Own(id);
            for (int i = 0; i < modules; i++)
                if (!Put(Storage(m, 0), Owned("chem_module"))) throw new InvalidOperationException($"{id}: module {i + 1} rejected");
            return m;
        }

        static void Lift(GameItem it) => (it.parentInventory as GameInventory)?.Expel(it);

        static void Shelf(GameItem c) { Lift(c); W.Place(W.E.invElement, c); }

        static void Pour(GameItem from, GameItem to, int ul)
        {
            foreach (var kv in WaterHelper.RemoveAndReturn(from, ul)) WaterHelper.AddLiquid(to, kv.Key, kv.Value);
        }

        static GameItem Supply(string builder) { var b = (GameItem)W.Call(typeof(PreBuildChemHelper), builder); GeneralHelper.SetItemOwned(b, true); return b; }

        static void Blend(GameItem blender, GameItem container, string itemId, int count)
        {
            Lift(container);
            if (!Put(Storage(blender, 2), container)) throw new InvalidOperationException("blender: container rejected");
            for (int i = 0; i < count; i++)
                if (!Put(Storage(blender, 1), Owned(itemId))) throw new InvalidOperationException("blender: input rejected");
            Press(Buttons(blender)[0]);
            Shelf(container);
        }

        static GameItem Plate(int target)
        {
            var p = ChemMachine("heating_plate", 4);
            p.onOneSecondItemFunc(p, null, null);
            p.ModifyTag("CURRENT_TARGET_TEMP", s => s.Enable().SetInt(target));
            p.ModifyTag("HEATING_PLATE_STATE", s => s.Enable().SetInt(1));
            return p;
        }

        static void Hold(GameItem c, GameItem plate, int seconds)
        {
            Lift(c);
            if (!Put(Storage(plate, 1), c)) throw new InvalidOperationException("plate: container rejected");
            for (int i = 0; i < seconds; i++)
            {
                plate.onOneSecondItemFunc(plate, null, null);
                c.onOneSecondItemFunc(c, null, null);
            }
            Shelf(c);
        }

        static GameItem Evaporator(params string[] modules)
        {
            var ev = W.Own("evaporator");
            Put(MachineHelper.GetBatterySlot(ev), Battery(1000000));
            foreach (var m in modules)
                if (!Put(Storage(ev, 1), Owned(m))) throw new InvalidOperationException("evaporator: module rejected " + m);
            return ev;
        }

        // Nights until the container is finisher-dry; the distillate slot collects what the modules keep.
        static int Dry(GameItem ev, GameItem c, GameItem distillate = null, int max = 20)
        {
            Lift(c);
            if (!Put(Storage(ev, 2), c)) throw new InvalidOperationException("evaporator: container rejected");
            if (distillate != null && !Put(Storage(ev, 3), distillate)) throw new InvalidOperationException("evaporator: distillate rejected");
            int n = 0;
            while (WaterHelper.IsWet(c) && n < max) { W.QuietDay(); n++; }
            Shelf(c);
            if (distillate != null) Shelf(distillate);
            return n;
        }

        // ---- chemistry: the solvent is not a reactant ------------------------------------------

        // R1 reads alcohol against the mush still in the vessel, and never consumes alcohol. So the
        // optimal vessel holds 5x one cap's mush of alcohol and takes caps one at a time.
        public static void DreamBatch(int seed)
        {
            W.New(seed);
            var blender = ChemMachine("blender", 4);
            var plate = Plate(45);
            void Run(string name, int caps, int perStep, int alcoholUl)
            {
                var c = W.Own("water_pitcher");
                var mini = Supply("CreateMiniAlcoholBottle");
                int needMix = (int)(alcoholUl / (P(mini, "alcohol") / (double)WaterHelper.GetTotalVolume(mini)));
                while (P(c, "alcohol") < alcoholUl - 500)
                {
                    if (WaterHelper.GetTotalVolume(mini) == 0) mini = Supply("CreateMiniAlcoholBottle");
                    int want = (int)((alcoholUl - P(c, "alcohol")) / (P(mini, "alcohol") / (double)WaterHelper.GetTotalVolume(mini)));
                    Pour(mini, c, Math.Min(want, WaterHelper.GetTotalVolume(mini)));
                }
                int mushIn = 0, seconds = 0;
                for (int done = 0; done < caps; done += perStep)
                {
                    int before = P(c, "dream_cap_mush");
                    Blend(blender, c, "dream_cap", Math.Min(perStep, caps - done));
                    mushIn += P(c, "dream_cap_mush") - before;
                    int s = perStep == caps ? 120 : 20;
                    Hold(c, plate, s);
                    seconds += s;
                }
                int evap = Evaporative(c);
                var ev = Evaporator("evaporator_module_solvent_recovery");
                var d = W.Own("water_pitcher");
                int nights = Dry(ev, c, d);
                R.Info($"dream extraction '{name}': {caps} caps, {perStep} per step, alcohol {alcoholUl / 1000} ml",
                    $"extract {Ml(c, "dream_cap_extract"):0.0} ml = {100.0 * P(c, "dream_cap_extract") / Math.Max(1, mushIn):0.0}% of {mushIn / 1000.0:0.0} ml mush; mush left {Ml(c, "dream_cap_mush"):0.0}; plate {seconds} s; solvent to evaporate {evap / 1000.0:0} ml; nights to dry {nights}; alcohol recovered {Ml(d, "alcohol"):0.0} ml",
                    caps, "rest: " + Parts(c));
            }
            Run("manual: all at once, 5.3x", 6, 6, 700000);
            Run("all at once, same alcohol as one cap's 5x", 6, 6, 120000);
            Run("one cap at a time, 5x one cap", 6, 1, 120000);
            Run("one cap at a time, 2.5x one cap", 6, 1, 60000);
            Run("two caps at a time, 5x two caps", 6, 2, 240000);
            Run("one cap at a time, 5x, 12 caps", 12, 1, 120000);
        }


        static GameItem Filter(GameItem c, string filterId)
        {
            var pitcher = W.Own("water_pitcher");
            if (!Put(pitcher, Owned(filterId))) throw new InvalidOperationException("filter rejected by pitcher");
            WaterHelper.TransferLiquid(c, pitcher);
            return pitcher;
        }

        static List<GameItem> Finish(GameItem c, string recipe)
        {
            var f = ChemMachine("chem_finisher", 4);
            f.ModifyTag("SELECTED_CHEM_ID_TAG", s => s.Enable().SetString(recipe));
            Lift(c);
            if (!Put(Storage(f, 1), c)) throw new InvalidOperationException("finisher: container rejected");
            var output = (GameInventory)Storage(f, 2);
            var outBtn = Buttons(f)[1];
            var made = new List<GameItem>();
            for (int i = 0; i < 400; i++)
            {
                if (!MachineChemFinisher.CanOutput(f, (GameSlotInventory)Storage(f, 1))) break;
                Press(outBtn);
                foreach (var it in output.childItems.ToList()) { made.Add(it); output.Expel(it); }
            }
            return made;
        }

        static string Summary(List<GameItem> products)
        {
            if (products.Count == 0) return "0 made";
            var pur = products.Select(ChemicalProductHelper.GetPurity).ToList();
            var grades = string.Join(",", products.GroupBy(ChemicalProductHelper.GetCurrentChemicalConditionId).Select(g => $"{g.Key}x{g.Count()}"));
            long value = products.Sum(p => p.GetCurrentValue(false, true, true, false, 0));
            return $"{products.Count} made; purity {pur.Min():0.0}-{pur.Max():0.0}% (mean {pur.Average():0.0}); {grades}; value total {value}";
        }

        // The batched extraction through the full lab-pure chain: does it still press lab pure, in one night?
        public static void DreamBatchPure(int seed)
        {
            W.New(seed);
            var blender = ChemMachine("blender", 4);
            var plate = Plate(45);
            var c = W.Own("water_pitcher");
            var mini = Supply("CreateMiniAlcoholBottle");
            Pour(mini, c, (int)(120000 / (P(mini, "alcohol") / (double)WaterHelper.GetTotalVolume(mini))));
            for (int i = 0; i < 6; i++) { Blend(blender, c, "dream_cap", 1); Hold(c, plate, 20); }
            string extracted = Parts(c);
            var acid = Supply("CreateAcidBottle");
            Pour(acid, c, (int)(P(c, "protein") / (P(acid, "acid") / (double)WaterHelper.GetTotalVolume(acid))));
            for (int i = 0; i < 60; i++) c.onOneSecondItemFunc(c, null, null);
            if (P(c, "acid") > 0)
            {
                var b = Supply("CreateBaseBottle");
                Pour(b, c, (int)(P(c, "acid") / (P(b, "base") / (double)WaterHelper.GetTotalVolume(b))));
                for (int i = 0; i < 30; i++) c.onOneSecondItemFunc(c, null, null);
            }
            c = Filter(c, "kotton_fabric");
            c = Filter(c, "water_filter");
            var ev = Evaporator("evaporator_module_solvent_recovery");
            var d = W.Own("water_pitcher");
            int nights = Dry(ev, c, d);
            var made = Finish(c, "dream_dust");
            R.Info("dream dust, batched: 120 ml alcohol, 6 caps blended one at a time into it (20 s at 45 C each), acid = protein, base, kotton + water filter, 4 modules",
                Summary(made), made.Count, $"nights {nights}; alcohol back {Ml(d, "alcohol"):0.0} ml; after extraction: {extracted}");
        }

        // R7 reads alcohol against this second's 15 % chunk of coffee mush, not the whole mush.
        public static void CoffeeRatio(int seed)
        {
            W.New(seed);
            var blender = ChemMachine("blender", 4);
            var plate = Plate(60);
            foreach (double r in new[] { 0.1, 0.25, 0.5, 0.75, 1.0, 1.3 })
            {
                var c = W.Own("water_pitcher");
                Blend(blender, c, "coffee_bean", 3);
                int mush = P(c, "coffee_mush"), caf0 = P(c, "caffeine");
                WaterHelper.AddLiquid(c, "alcohol", (int)(mush * r));
                Hold(c, plate, 120);
                R.Info($"coffee extraction, 3 beans, alcohol {r}x the mush, 60 C, 120 s: caffeine made as % of mush (75 % is the cap)",
                    $"{100.0 * (P(c, "caffeine") - caf0) / mush:0.0}%", 3, $"mush {mush / 1000.0:0.0} ml, alcohol {mush * r / 1000:0.0} ml; " + Parts(c));
            }
        }

        // Evaporator distillate: Q % of every evaporated liquid; the solvent-recovery module keeps only alcohol.
        // Universal quality modules raise Q without discarding water, so the distillate is clean water + alcohol.
        public static void Distillate(int seed)
        {
            W.New(seed);
            var cases = new (string name, string[] mods)[]
            {
                ("none", new string[0]),
                ("solvent recovery", new[] { "evaporator_module_solvent_recovery" }),
                ("solvent recovery + 2 quality", new[] { "evaporator_module_solvent_recovery", "system_module_quality", "system_module_quality" }),
                ("4 quality", new[] { "system_module_quality", "system_module_quality", "system_module_quality", "system_module_quality" }),
                ("solvent recovery + 2 fineness", new[] { "evaporator_module_solvent_recovery", "system_module_fineness", "system_module_fineness" }),
                ("4 fineness", new[] { "system_module_fineness", "system_module_fineness", "system_module_fineness", "system_module_fineness" }),
            };
            foreach (var (name, mods) in cases)
            {
                GameItem ev;
                try { ev = Evaporator(mods); }
                catch (Exception e) { R.Info($"evaporator distillate, {name}", "could not seat: " + e.Message); continue; }
                var c = W.Own("water_pitcher");
                WaterHelper.AddLiquid(c, "water", 200000);
                WaterHelper.AddLiquid(c, "alcohol", 100000);
                WaterHelper.AddLiquid(c, "mineral", 5000);
                WaterHelper.AddLiquid(c, "microbe", 5000);
                var d = W.Own("water_pitcher");
                Lift(c); Put(Storage(ev, 2), c); Put(Storage(ev, 3), d);
                W.QuietDay();
                R.Info($"evaporator distillate after one night, {name} (Q {MachineryHelper.GetCurrentQualityBonus(ev)}, P {MachineryHelper.GetCurrentPerformanceBonus(ev)}): 200 ml water + 100 ml alcohol + 5 ml mineral + 5 ml microbe",
                    $"distillate: {Parts(d)}; water purity {100 * WaterHelper.GetWaterPurity(d):0.00}%", 1, "left: " + Parts(c));
            }
            // AddLiquid caps each liquid at the capacity, not the sum: a small distillate vessel overfills.
            var ev2 = Evaporator("system_module_quality", "system_module_quality", "system_module_quality", "system_module_quality");
            var c2 = W.Own("water_pitcher");
            WaterHelper.AddLiquid(c2, "water", 150000);
            WaterHelper.AddLiquid(c2, "alcohol", 150000);
            var small = W.Own("100ml_cylinder");
            Lift(c2); Put(Storage(ev2, 2), c2); Put(Storage(ev2, 3), small);
            W.QuietDay();
            R.Info("distillate into a 100 ml cylinder (150 ml water + 150 ml alcohol evaporated, 4 quality modules)",
                $"cylinder holds {WaterHelper.GetTotalVolume(small) / 1000.0:0.0} ml of 100: {Parts(small)}");
        }
    
        // ---- trading ------------------------------------------------------------------------

        // The appraisal visit draws its type uniformly from the unblocked pool; from Master any type can be blocked.
        public static void AppraisalBlock(int seed)
        {
            W.New(seed);
            W.PS.appraiserLevel = 3;
            foreach (var t in AppraisalItemRegistry.GetAll())
            {
                long sum = 0, fees = 0; int n = 2000;
                for (int i = 0; i < n; i++)
                {
                    var it = t.createRandom();
                    int v = AppraisalHelper.GetCorrectBaseValue(it);
                    sum += v;
                    int est = RNG.CalculatePercentage(v, 105);
                    fees += AppraisalRankHelper.GetBaseFee() + RNG.CalculatePercentage(est, AppraisalRankHelper.GetFeePercent(1));
                }
                R.Info($"appraisal at Master, type {t.id}: mean true value / mean fee per item at a perfect 105 % estimate", $"{(double)sum / n:0} / {(double)fees / n:0.0}", n);
            }
            foreach (var block in new[] { new string[0], new[] { "hand", "ribwich" } })
            {
                W.PS.appraisalBlacklist = block.ToList();
                var pool = AppraisalItemRegistry.GetPool(3).Select(t => t.id).ToList();
                R.Info($"appraisal pool at Master, blocked [{string.Join(",", block)}]", string.Join(",", pool));
            }
            W.PS.appraiserLevel = 2;
            W.PS.appraisalBlacklist = new List<string> { "hand", "ribwich" };
            R.Info("appraisal pool at Senior (rank 2), blocked [hand,ribwich]", string.Join(",", AppraisalItemRegistry.GetPool(2).Select(t => t.id)));
        }

        // The window ranks types by summed value and picks each type's tier from its summed value, and an item
        // with several display types counts in full for every one of them.
        public static void WindowMultiType(int seed)
        {
            W.New(seed);
            W.Day = 20;
            W.PS.networkUpgrade["SHOWCASE_I"].state = 2;
            var shelf = W.E.showcaseElement;
            void Case(string id, int n) { W.ClearInventory(shelf); for (int i = 0; i < n; i++) if (!W.Place(shelf, Owned(id))) break; }
            foreach (var (id, n) in new[] { ("caffeine_pill", 35), ("caffeine_pill", 15), ("kitchen_cleaver", 3), ("surgery_tool", 2) })
            {
                Case(id, n);
                var seen = new Dictionary<string, int>();
                for (int i = 0; i < 300; i++)
                    foreach (var (cid, type) in ShowcaseHelper.GetDisplayCaseCustomer())
                        seen[$"{type}:{cid}"] = seen.GetValueOrDefault($"{type}:{cid}") + 1;
                long total = shelf.childItems.Sum(x => x.GetCurrentValue(false, true, true, false, 0));
                R.Info($"window, SHOWCASE_I, day 20, case = {n} x {id} (case value {total})",
                    string.Join(", ", seen.OrderByDescending(kv => kv.Value).Select(kv => $"{kv.Key} {kv.Value}")), 300);
            }
        }
    
        // ---- water and fish ---------------------------------------------------------------

        static GameItem RustThroughPitcher(int passes)
        {
            var c = W.Own("water_jug");
            WaterHelper.FillWithRustWater(c);
            var outJug = c;
            // The 3 L pitcher filters one pitcher-full at a time into the jug.
            for (int pass = 0; pass < passes; pass++)
            {
                outJug = W.Own("water_jug");
                while (WaterHelper.GetTotalVolume(c) > 0)
                {
                    var pitcher = W.Own("water_pitcher");
                    Put(pitcher, Owned("water_filter"));
                    WaterHelper.TransferLiquid(c, pitcher);
                    WaterHelper.TransferLiquid(pitcher, outJug);
                }
                c = outJug;
            }
            return c;
        }

        static string Bp(GameItem c, params string[] ids) => string.Join(" ", ids.Select(id => $"{id}={P(c, id) * 10000.0 / Math.Max(1, P(c, "water")):0.0}bp"));

        // Chlorine: each tick spends up to 15 ml whether or not anything is left to react; the rest decays.
        public static void Tablets(int seed)
        {
            W.New(seed);
            foreach (var (label, uv, tabs) in new[] { ("6 tablets together", false, 6), ("2 tablets", false, 2), ("UV then 2 tablets", true, 2), ("3 tablets", false, 3), ("UV then 1 tablet", true, 1) })
            {
                var c = RustThroughPitcher(1);
                int vol = WaterHelper.GetTotalVolume(c);
                string before = Bp(c, "chemical_contaminant", "microbe");
                if (uv) WaterHelper.OnUVUsed(c);
                for (int i = 0; i < tabs; i++) ChemicalHelper.DissolveInto(Owned("water_tablet"), c);
                for (int i = 0; i < 360; i++) c.onOneSecondItemFunc(c, null, null);
                R.Info($"water tablets on {vol / 1000.0:0.0} L of filtered rust water, {label}: purity (bp) after 360 s",
                    WaterHelper.GetWaterPurity(c), tabs, $"before {before}; after {Bp(c, "chemical_contaminant", "microbe", "chlorine")}");
            }
        }

        // Pure without modules: filter, hydroreed to its floors (rank 4), then UV.
        public static void ReedPure(int seed)
        {
            W.New(seed);
            foreach (int passes in new[] { 1, 2 })
            {
                var c = RustThroughPitcher(passes);
                string after = Bp(c, "organic_waste", "physical_contaminant", "mineral", "chemical_contaminant", "heavy_metal", "microbe");
                var trace = new List<int>();
                for (int n = 0; n < 25; n++) { PlantWaterQuality.Purify(c, 4); trace.Add(WaterHelper.GetWaterPurity(c)); }
                int reed = WaterHelper.GetWaterPurity(c);
                WaterHelper.OnUVUsed(c);
                R.Info($"rust water, {passes} pitcher pass(es), hydroreed rank IV x25 nights, then UV: purity (bp; 9990 = Pure)",
                    WaterHelper.GetWaterPurity(c), 1, $"after filter {after}; reed nights {string.Join(",", trace)}; before UV {reed}; final {Bp(c, "organic_waste", "physical_contaminant", "mineral", "chemical_contaminant", "heavy_metal", "microbe")}");
            }
        }

        // Pellet taint is strictly above 25 % of lifetime kcal; growth speed has no pellet term.
        public static void PelletShare(int seed)
        {
            W.New(seed);
            var sp = FishSpecies.All.Values.First(f => f.fishIdentifier == "tilapia");
            foreach (float share in new[] { 0.20f, 0.24f, 0.25f, 0.2501f, 0.27f })
            {
                var f = AquacultureItems.SpawnFish(sp, "female", 3);
                f.ModifyTag("FISH_KCAL_EATEN_TAG", st => st.Enable().SetFloat(10000f));
                f.ModifyTag("FISH_KCAL_PELLET_TAG", st => st.Enable().SetFloat(10000f * share));
                R.Info($"{sp.speciesID}: pellet share {share:0.0000} of lifetime kcal -> trash-grade fillets?", AquacultureHelper.IsPelletTainted(f), 1,
                    $"growth speed {AquacultureHelper.GetGrowthSpeed(f):0.00}");
            }
        }
    
        // ---- day, rent, services ------------------------------------------------------------

        // The buyout charges only the price: rent deferred with Grace (temporaryRent) is never collected.
        public static void BuyoutArrears(int seed)
        {
            W.New(seed, "hard");
            W.Day = 91;
            W.PS.temporaryRent = 621;
            int price = W.PS.GetStoreBuyPrice();
            W.Cash = price;
            bool can = W.PS.CanBuyOutStore();
            W.PS.BuyOutStore();
            int after = W.Cash;
            for (int i = 0; i < 8; i++) W.QuietDay();
            R.Info("buyout with 621 of deferred rent outstanding: can buy with cash = price; cash right after; cash and temporaryRent 8 days later",
                $"can {can}; after {after}; 8 days later cash {W.Cash}, temporaryRent {W.PS.temporaryRent}, paid {W.PS.IsPropertyPaid}", 1, $"price {price}; over {Fence.Gym.Episode.GameOverReason()}");
        }

        // The permit official's weekly donation demand from day 45: 25 + 10 x floor(day / 7), uncapped.
        public static void PermitDonation(int seed)
        {
            W.New(seed, "hard");
            var rows = new List<string>();
            int sum = 0;
            for (int d = 45; d <= 185; d += 7)
            {
                W.Day = d;
                int a = StoreClientList.GetBribeAmount();
                if (d <= 94) sum += a;
                rows.Add($"d{d}:{a}");
            }
            R.Info("permit donation added per visit (no permit), by day", string.Join(" ", rows), 1, $"sum days 45-94 = {sum} (permit price on hard 2000)");
        }

        // Burglary when uninsured: 33 + 5n % with n consecutive uninsured nights; one insured night resets n.
        public static void InsurancePulse(int seed)
        {
            W.New(seed, "hard");
            W.Day = 30;
            var ins = StoreService.GetInsuranceService();
            ins.unlocked = true;
            for (int i = 0; i < 40; i++) W.Own("metal_ingot");
            foreach (int k in new[] { 0, 1, 2, 3, 4, 5, 1000 })
            {
                int thefts = 0, insured = 0, nights = 6000;
                W.PS.dayWithoutInsurance = 0;
                for (int n = 0; n < nights; n++)
                {
                    ins.isUsing = k == 0 || n % (k + 1) == k;
                    if (ins.isUsing) insured++;
                    int before = W.E.invElement.childItems.Count;
                    W.PS.HandleInsurance();
                    int lost = before - W.E.invElement.childItems.Count;
                    thefts += lost;
                    for (int j = 0; j < lost; j++) W.Own("metal_ingot");
                }
                R.Info($"insurance: insured 1 night in {(k == 1000 ? "never" : (k + 1).ToString())}: thefts per night / insured share",
                    $"{(double)thefts / nights:0.000} / {(double)insured / nights:0.000}", nights,
                    "cost per night = share x fee + thefts x loss; fee 30, loss 30 -> " + $"{30.0 * insured / nights + 30.0 * thefts / nights:0.0}");
            }
        }

        // ---- production ---------------------------------------------------------------------

        // A learning-algorithm module gains +1 on each positive stat every night its machine actions, to 2x its base;
        // the security alarm actions every night, powered or not.
        public static void LearningAlarm(int seed)
        {
            W.New(seed);
            var alarm = W.Own("security_alarm");
            var m = Owned("system_module_fineness");
            ModuleEffectHelper.AddEffectToModule(m, "MODULE_EFFECT_LEARNING_ALGORITHM");
            int q0 = Tag(m, "BONUS_PERCENTAGE_QUALITY_INT");
            bool seated = Put(alarm, m);
            var trace = new List<int>();
            for (int n = 0; n < 55; n++) { W.QuietDay(); trace.Add(Tag(m, "BONUS_PERCENTAGE_QUALITY_INT")); }
            R.Info("learning fineness module in an unpowered security alarm: quality by night (base 25, cap 50)",
                string.Join(",", trace.Where((x, i) => i % 5 == 4)), 55, $"seated {seated}; start {q0}; P {Tag(m, "BONUS_PERCENTAGE_PERFORMANCE_INT")} E {Tag(m, "BONUS_PERCENTAGE_EFFICIENCY_INT")}; alarm battery none");
        }

        // Printed melee condition: 20-30 + Q/4 + the integer average of the ingot bonuses; < 25 Poor, < 60 Fair.
        public static void PrinterIngotMix(int seed)
        {
            W.New(seed);
            var printer = W.Own("printer");
            foreach (var (label, bonus) in new[] { ("3 Fair", 0), ("2 Fair + 1 High (20/3)", 6), ("2 Fair + 1 Low (-20/3)", -6), ("1 Fair + 1 Low (knife)", -10), ("1 Fair + 1 High (knife)", 10) })
            {
                printer.ModifyTag("PRINTER_INGOT_QUALITY_BONUS_INT", st => st.Enable().SetInt(bonus));
                int poor = 0, n = 3000;
                var w = W.Make("combat_machete");
                for (int i = 0; i < n; i++)
                {
                    MachinePrinter.ApplyPrintedMeleeCondition(w, printer);
                    var f = w.FindItemFeatureByCategory("CATEGORY_EQUIPMENT_CONDITION");
                    if (f != null && f.realCondition.identifier.ToLowerInvariant().Contains("poor")) poor++;
                }
                R.Info($"printed melee, unmodded printer, ingots {label} (bonus {bonus}): share Poor", $"{100.0 * poor / n:0.0}%", n);
            }
        }

        // Two POISON items at the exchange mint a cell charged with their summed market value, capped at 100.
        public static void PoisonCell(int seed)
        {
            W.New(seed);
            foreach (var pair in new[] { new[] { "nightmare_cap", "nightmare_cap" } })
            {
                var items = pair.Select(Owned).ToList();
                var b = ExchangeList.PoisonBarter();
                int before = W.PS.exchangeManager.itemsBuffer.Count;
                b.onAccept(items);
                var cell = W.PS.exchangeManager.itemsBuffer.Last();
                R.Info($"poison barter with {string.Join("+", pair)}: cell minted (charge, value)",
                    $"{cell.identifier} charge {PowerHelper.GetAvailableEnergyFromItem(cell, false)} value {cell.GetCurrentValue(false, true, true, false, 0)}", 1,
                    "each item: " + string.Join(",", items.Select(i => i.GetCurrentValue(false, true, true, false, 0))));
            }
        }
    
        // The fridge: +20 % innate on chillable drinks (and +25 % ice-cold on nudka in the freeze zone), kept while
        // the item stays in a powered fridge, lost the night after it leaves.
        public static void Fridge(int seed)
        {
            W.New(seed);
            var fr = W.Own("fridge");
            Put(MachineHelper.GetBatterySlot(fr), Battery(1000));
            var ids = new[] { "soda_red", "water_ration", "red_beer", "processed_milk", "bottled_water_premium", "energy_drink", "nudka" };
            var items = new List<GameItem>();
            foreach (var id in ids)
            {
                foreach (var zone in new[] { "chill", "freeze" })
                {
                    var f = W.Own("fridge");
                    Put(MachineHelper.GetBatterySlot(f), Battery(1000));
                    var it = Owned(id);
                    long before = it.GetCurrentValue(false, true, true, false, 0);
                    bool placed = W.Place(zone == "chill" ? MachineFridge.GetChillZone(f) : MachineFridge.GetFreezeZone(f), it);
                    W.QuietDay();
                    R.Info($"fridge {zone} zone, {id}: placed; value before -> after one powered night", $"{placed}; {before} -> {it.GetCurrentValue(false, true, true, false, 0)}");
                    if (zone == "chill") items.Add(it);
                }
            }
            var wine = Owned("wine_bottle");
            R.Info("fridge accepts a wine bottle", W.Place(MachineFridge.GetChillZone(fr), wine));
            foreach (var it in items) Shelf(it);
            var trace = new List<long>();
            for (int d = 0; d < 3; d++) { trace.Add(items.Sum(i => i.GetCurrentValue(false, true, true, false, 0))); W.QuietDay(); }
            R.Info("after taking them out: summed value on the following mornings", string.Join(",", trace), 3, $"fridge draw {MachineryHelper.GetMachinePowerUsage(fr)} a night");
        }
    
        // ---- weapons: build, don't buy -------------------------------------------------------

        static GameItem ArmsOwn(string id)
        {
            var it = DirectoryMaster.Item(id);
            GeneralHelper.SetItemOwned(it, true);
            GraphUtils.TryAcceptAll(W.E.invElement, it, -1);
            return it;
        }

        static bool Attach(GameItem gun, string modId)
        {
            var mod = ArmsOwn(modId);
            foreach (var slot in GraphUtils.FindChildrenType<GameSlotInventory>(gun).ToList())
                if (slot.childItem == null && GraphUtils.TryAcceptAll(slot, mod, -1) == 0 && ReferenceEquals(mod.parentInventory, slot))
                {
                    GunHelper.UpdateCurrentGunStats(gun);
                    return true;
                }
            return false;
        }

        // A printed frame finished with a gun part: a New gun, base stats x (100 + quality) %.
        static GameItem Framed(string frameId, int quality)
        {
            var frame = ArmsOwn(frameId);
            GunFrameHelper.SetPrintQuality(frame, quality);
            string gunId = frame.GetTagReadonly("GUN_FRAME_GUN_ID_STRING").GetString();
            var before = W.E.invElement.childItems.Where(i => i != null).Select(i => i.uniqueId).ToHashSet();
            GunFrameHelper.Finish(frame, ArmsOwn("gun_part"));
            return W.E.invElement.childItems.First(i => i != null && i.identifier == gunId && !before.Contains(i.uniqueId));
        }

        static string Stats(GameItem g) => string.Join("/", new[] { "FIREPOWER", "STYLE", "RELIABILITY", "RECOIL", "ERGONOMICS", "ACCURACY" }
            .Select(t => g.GetTagReadonly($"GUN_CURRENT_{t}_TAG").GetInt()));

        static string Cond(GameItem g) => g.FindItemFeatureByCategory("CATEGORY_EQUIPMENT_CONDITION")?.realCondition?.identifier?.Replace("equipment_condition_", "");

        static int Paid(GunOrderData order, GameItem gun)
        {
            var c = StoreClientListGun.CreateLLGunBuyer(order);
            c.spriteName ??= ""; c.possibleSprites ??= new List<string>();
            W.PS.currentClientInstance = StoreClientInstance.CreateClientInstance(c);
            W.PS.isClientArrived = true;
            Lift(gun);
            GraphUtils.TryAcceptAll(W.E.backInvinvElement, gun, -1);
            int cash0 = W.Cash;
            NegociationUIManager.Instance.OnOfferAcceptClick();
            Scheduler.Settle();
            int paid = W.Cash - cash0;
            W.PS.DismissCurrentClient(false, true, true);
            Scheduler.Settle();
            foreach (var i in W.E.backInvinvElement.childItems.Where(i => i != null).ToList()) W.E.backInvinvElement.Expel(i);
            return paid;
        }

        static void Order(string label, Func<GunOrderData> make, Func<GameItem> build, int n = 10)
        {
            int met = 0; long paid = 0; string rows = null, stats = null;
            for (int k = 0; k < n; k++)
            {
                var o = make();
                var g = build();
                stats ??= Stats(g) + " " + Cond(g);
                bool ok = o.mandatoryRequirements.All(r => r.IsMet(g));
                rows ??= $"base {o.GetBaseReward()}, rows met {o.bonusRequirements.Count(r => r.IsMet(g))}/{o.bonusRequirements.Count}";
                if (ok) met++;
                paid += Paid(o, g);
            }
            R.Info($"order {label}", $"mandatory met {met}/{n}; mean paid {(double)paid / n:0}", n, $"gun {stats}; first order {rows}");
        }

        public static void ArmsBuild(int seed)
        {
            W.New(seed, "hard", true, "gunsmith");
            PermitHelper.GrantedFromPermit(W.Make("permit_gun_2"));
            PermitHelper.GrantedFromPermit(W.Make("permit_gun_3"));
            W.Cash = 100000;
            foreach (int q in new[] { 0, 30 })
                R.Info($"heavy handmade pistol from a printed frame, quality {q}: stats FP/STY/REL/RCL/ERG/ACC, condition, estimate",
                    $"{Stats(Framed("heavy_handmade_pistol_frame", q))} {Cond(Framed("heavy_handmade_pistol_frame", q))}");
            var bought = ArmsOwn("heavy_handmade_pistol");
            GunHelper.SetEquipmentCondition(bought, ItemFeatureList.EquipmentCondition.Fair);
            GunHelper.UpdateCurrentGunStats(bought);
            R.Info("heavy handmade pistol bought Fair (as gun sellers sell it): stats", Stats(bought));
            Func<GameItem> heavy0 = () => Framed("heavy_handmade_pistol_frame", 0);
            Func<GameItem> heavy30 = () => Framed("heavy_handmade_pistol_frame", 30);
            Order("Mid general, bare heavy Q0", GunOrderData.CreateMidOrderGeneral, heavy0);
            Order("Mid firepower, bare heavy Q0", GunOrderData.CreateMidOrderFirepower, heavy0);
            Order("Mid stylish, bare heavy Q0", GunOrderData.CreateMidOrderStylish, heavy0);
            Order("Mid suppressed, bare heavy Q0", GunOrderData.CreateMidOrderSuppressed, heavy0);
            Order("Mid big, bare heavy Q0", GunOrderData.CreateMidOrderBig, heavy0);
            Order("Mid handling, bare heavy Q0", GunOrderData.CreateMidOrderHandling, heavy0);
            Order("Mid best (10 mm), bare heavy Q0", () => GunOrderData.CreateBestOrder(2, 10), heavy0);
            Order("Mid general, bare heavy Q30", GunOrderData.CreateMidOrderGeneral, heavy30);
            Order("Mid general, heavy bought Fair", GunOrderData.CreateMidOrderGeneral, () =>
            {
                var g = ArmsOwn("heavy_handmade_pistol");
                GunHelper.SetEquipmentCondition(g, ItemFeatureList.EquipmentCondition.Fair);
                GunHelper.UpdateCurrentGunStats(g);
                return g;
            });
            Order("High firepower, printed gun + mag_22_ext + pistol_switch", GunOrderData.CreateHighOrderFirepower, () =>
            {
                var g = ArmsOwn("printed_gun"); Attach(g, "mag_22_ext"); Attach(g, "pistol_switch"); return g;
            });
            Order("High reliability, heavy Q30 + internal_springs + silencer", GunOrderData.CreateHighOrderReliability, () =>
            {
                var g = heavy30(); Attach(g, "internal_springs"); Attach(g, "silencer"); return g;
            });
            Order("High handling, heavy Q36 + trigger_group + compensator", GunOrderData.CreateHighOrderHandling, () =>
            {
                var g = Framed("heavy_handmade_pistol_frame", 36); Attach(g, "trigger_group"); Attach(g, "compensator"); return g;
            });
            Order("Basic general, handmade Q0", GunOrderData.CreateBasicOrderGeneral, () => Framed("handmade_pistol_frame", 0));
            Order("Basic general, printed gun", GunOrderData.CreateBasicOrderGeneral, () => ArmsOwn("printed_gun"));
        }

        // Repair: chance = repairability + what earlier failures carried; the part is spent either way.
        public static void RepairCost(int seed)
        {
            W.New(seed, "hard", true, "gunsmith");
            foreach (var id in new[] { "handmade_pistol", "heavy_handmade_pistol", "glock_receiver", "revolver", "smg", "shotgun" })
            {
                int n = 400; long parts = 0, gain = 0;
                for (int k = 0; k < n; k++)
                {
                    var g = ArmsOwn(id);
                    GunHelper.SetEquipmentCondition(g, ItemFeatureList.EquipmentCondition.Poor);
                    GunHelper.UpdateCurrentGunStats(g);
                    long v0 = g.GetCurrentValue(false, true, true, false, 0);
                    string c0 = Cond(g);
                    while (Cond(g) == c0 && parts < 1000000) { GunHelper.TryRepairGun(g, ArmsOwn("gun_part")); parts++; }
                    gain += g.GetCurrentValue(false, true, true, false, 0) - v0;
                    Lift(g);
                }
                R.Info($"repair {id} Poor -> Fair: parts per step / value gained per step",
                    $"{(double)parts / n:0.00} / {(double)gain / n:0.0}", n, $"break-even part price {(double)gain / parts:0.0}");
            }
        }
    }
}
