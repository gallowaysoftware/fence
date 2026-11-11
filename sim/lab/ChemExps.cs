using System;
using System.Collections.Generic;
using System.Linq;
using static Fence.Lab.ProductionExps;

namespace Fence.Lab
{
    // docs/re/CHEMISTRY.md: extraction windows, purification steps and full product chains, run on the
    // game's own machines (blender/finisher buttons, plate and container ticks, the evaporator's night).
    public static class ChemExps
    {
        // ---- kit ----------------------------------------------------------------------------

        static int P(GameItem c, string liquid) => c.GetTagReadonly(Liquid.GetCurrentPartFromId(liquid)).GetInt();

        static string Parts(GameItem c) => string.Join(" ", Liquid.Liquids.Select(l => (l.identifier, v: P(c, l.identifier)))
            .Where(x => x.v > 0).Select(x => $"{x.identifier}={x.v / 1000.0:0.###}ml"));

        static double Temp(GameItem c) => c.GetTagReadonly("CONTAINER_TEMP_TAG").GetDouble();

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

        // Chem modules go in storage 0 of the blender, plate and finisher.
        static GameItem ChemMachine(string id, int modules)
        {
            var m = W.Own(id);
            for (int i = 0; i < modules; i++)
                if (!Put(Storage(m, 0), Owned("chem_module"))) throw new InvalidOperationException($"{id}: module {i + 1} rejected");
            return m;
        }

        static void Lift(GameItem it)
        {
            (it.parentInventory as GameInventory)?.Expel(it);
        }

        static GameItem Container(string id = "water_pitcher") => W.Own(id);

        // A graduated pour (dropper/cylinder): exactly ul of the source's mix, proportionally.
        static void Pour(GameItem from, GameItem to, int ul)
        {
            foreach (var kv in WaterHelper.RemoveAndReturn(from, ul)) WaterHelper.AddLiquid(to, kv.Key, kv.Value);
        }

        static void PourAll(GameItem from, GameItem to) => WaterHelper.TransferLiquid(from, to);

        // Supply bottles exactly as unpacking a chemical supply pack makes them.
        static GameItem Supply(string builder) { var b = (GameItem)W.Call(typeof(PreBuildChemHelper), builder); GeneralHelper.SetItemOwned(b, true); return b; }

        static void Blend(GameItem container, string itemId, int count, int modules)
        {
            var bl = ChemMachine("blender", modules);
            if (!Put(Storage(bl, 2), container)) throw new InvalidOperationException("blender: container rejected");
            for (int i = 0; i < count; i++)
                if (!Put(Storage(bl, 1), Owned(itemId))) throw new InvalidOperationException("blender: input rejected");
            Press(Buttons(bl)[0]);
            Lift(container);
            W.Place(W.E.invElement, container);
        }

        static GameItem Plate(int modules, int target, int mode = 1)
        {
            var p = ChemMachine("heating_plate", modules);
            p.onOneSecondItemFunc(p, null, null);
            if (modules >= 2) p.ModifyTag("CURRENT_TARGET_TEMP", s => s.Enable().SetInt(target));
            p.ModifyTag("HEATING_PLATE_STATE", s => s.Enable().SetInt(mode));
            return p;
        }

        static void OnPlate(GameItem plate, GameItem c)
        {
            Lift(c);
            if (!Put(Storage(plate, 1), c)) throw new InvalidOperationException("plate: container rejected");
        }

        // One real second: the plate heats, then the container cools and reacts (game order: machine, then its child).
        static void Seconds(GameItem c, int n, GameItem plate = null)
        {
            for (int i = 0; i < n; i++)
            {
                plate?.onOneSecondItemFunc(plate, null, null);
                c.onOneSecondItemFunc(c, null, null);
            }
        }

        static int HeatUntil(GameItem c, GameItem plate, double t, int max = 600)
        {
            for (int s = 1; s <= max; s++) { Seconds(c, 1, plate); if (Temp(c) >= t) return s; }
            return -1;
        }

        static void OffPlate(GameItem c)
        {
            Lift(c);
            W.Place(W.E.invElement, c);
        }

        // Pour through a filter seated in a pitcher; returns the pitcher holding the filtrate.
        static GameItem Filter(GameItem c, string filterId)
        {
            var pitcher = Container("water_pitcher");
            if (!Put(pitcher, Owned(filterId))) throw new InvalidOperationException("filter rejected by pitcher");
            PourAll(c, pitcher);
            return pitcher;
        }

        // Nights in a powered evaporator until the container is dry (finisher-ready) or maxNights pass.
        static int Evaporate(GameItem c, int maxNights = 20, int modules = 0, string module = null)
        {
            var ev = W.Own("evaporator");
            Put(MachineHelper.GetBatterySlot(ev), Battery(100000));
            if (module != null) Put(Storage(ev, 1), Owned(module));
            Lift(c);
            if (!Put(Storage(ev, 2), c)) throw new InvalidOperationException("evaporator: container rejected");
            int n = 0;
            while (WaterHelper.IsWet(c) && n < maxNights) { W.QuietDay(); n++; }
            Lift(c);
            W.Place(W.E.invElement, c);
            return n;
        }

        static List<GameItem> Finish(GameItem c, string recipe, int modules, int max = 200)
        {
            var f = ChemMachine("chem_finisher", modules);
            f.ModifyTag("SELECTED_CHEM_ID_TAG", s => s.Enable().SetString(recipe));
            Lift(c);
            if (!Put(Storage(f, 1), c)) throw new InvalidOperationException("finisher: container rejected");
            var output = (GameInventory)Storage(f, 2);
            var outBtn = Buttons(f)[1];
            var made = new List<GameItem>();
            for (int i = 0; i < max; i++)
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
            return $"{products.Count} made; purity {pur.Min():0.0}-{pur.Max():0.0}% (mean {pur.Average():0.0}); {grades}; value total {value} ({(double)value / products.Count:0.0} each)";
        }

        static double Purity(GameItem c, string active) => 100.0 * P(c, active) / Math.Max(1, WaterHelper.GetTotalVolume(c));

        // ---- experiments --------------------------------------------------------------------

        // Dream cap extraction yield by held temperature (the plate holds the target; 2 modules unlock it).
        public static void ExtractionTemperature(int seed)
        {
            W.New(seed);
            foreach (int t in new[] { 35, 40, 45, 49, 50, 55, 60, 65, 70 })
            {
                var c = Container();
                Blend(c, "dream_cap", 4, 4);
                int mush0 = P(c, "dream_cap_mush");
                for (int i = 0; i < 3; i++) PourAll(Supply("CreateMiniAlcoholBottle"), c);
                var plate = Plate(2, t, 2);
                OnPlate(plate, c);
                Seconds(c, 180, plate);
                R.Info($"extraction held at {t} C for 180 s (4 caps, alcohol:mush {(double)P(c, "alcohol") / mush0:0.0}): extract / mush left / plant matter as % of starting mush",
                    $"{100.0 * P(c, "dream_cap_extract") / mush0:0.0}% / {100.0 * P(c, "dream_cap_mush") / mush0:0.0}% / {100.0 * P(c, "plant_matter") / mush0:0.0}%", 1,
                    $"temp at end {Temp(c):0.0}; mush0 {mush0 / 1000.0} ml");
            }
        }

        // Yield by starting alcohol:mush ratio at 45 C (the reaction re-reads the ratio against the mush left each second).
        public static void ExtractionRatio(int seed)
        {
            W.New(seed);
            foreach (double r in new[] { 0.5, 1, 2, 3, 4, 5, 6, 8 })
            {
                var c = Container();
                Blend(c, "dream_cap", 4, 4);
                int mush0 = P(c, "dream_cap_mush");
                WaterHelper.AddLiquid(c, "alcohol", (int)(mush0 * r));
                var plate = Plate(2, 45, 2);
                OnPlate(plate, c);
                Seconds(c, 240, plate);
                double ex = P(c, "dream_cap_extract");
                R.Info($"extraction at 45 C, start ratio alcohol:mush {r}: extract as % of mush; extract per ml of alcohol",
                    $"{100 * ex / mush0:0.0}%; {ex / Math.Max(1, mush0 * r):0.000}", 1, $"mush left {P(c, "dream_cap_mush") / 1000.0} ml");
            }
            R.Info("doc claim: final yield is 50% only at ratio >= 5 measured against the mush still unreacted; lower ratios recover as mush shrinks", "see rows above");
        }

        // The unmodded plate fixes its target at 99 C, and the night runs every container for 360 s.
        public static void PlateOvernight(int seed)
        {
            W.New(seed);
            var cases = new[] { ("left on an unmodded plate (99 C), low", 0, 1), ("left on a 2-module plate set to 45 C", 2, 1), ("off the plate, in stock", -1, 0) };
            foreach (var (label, modules, mode) in cases)
            {
                var c = Container();
                Blend(c, "dream_cap", 4, 4);
                PourAll(Supply("CreateMiniAlcoholBottle"), c);
                PourAll(Supply("CreateMiniAlcoholBottle"), c);
                var hold = Plate(2, 45, 2);
                OnPlate(hold, c);
                Seconds(c, 200, hold);
                OffPlate(c);
                int ex0 = P(c, "dream_cap_extract");
                if (modules >= 0) OnPlate(Plate(modules, 45, mode), c);
                W.QuietDay();
                R.Info($"dream extract overnight, {label}: extract kept", $"{100.0 * P(c, "dream_cap_extract") / Math.Max(1, ex0):0.0}%", 1,
                    $"{ex0 / 1000.0} -> {P(c, "dream_cap_extract") / 1000.0} ml; temp {Temp(c):0.0} C; {Parts(c)}");
            }
        }

        // Protein: acid/base excess denatures 1:1 into debris; lipobond binds 8 fat each; debris filters out.
        public static void ProteinFat(int seed)
        {
            W.New(seed);
            foreach (var (acid, label) in new[] { (5000, "acid 0.5x protein"), (10000, "acid 1x"), (15000, "acid 1.5x") })
            {
                var c = Container();
                WaterHelper.AddLiquid(c, "water", 500000);
                WaterHelper.AddLiquid(c, "protein", 10000);
                WaterHelper.AddLiquid(c, "acid", acid);
                var trace = new List<string>();
                for (int s = 1; s <= 60; s++) { Seconds(c, 1); if (s is 1 or 5 or 10 or 30 or 60) trace.Add($"{s}s:p{P(c, "protein") / 1000.0}/a{P(c, "acid") / 1000.0}/d{P(c, "debris") / 1000.0}"); }
                R.Info($"10 ml protein + {label} in 500 ml water, room temp: protein/acid/debris ml", string.Join(" ", trace));
            }
            {
                var c = Container();
                WaterHelper.AddLiquid(c, "water", 500000);
                WaterHelper.AddLiquid(c, "protein", 10000);
                WaterHelper.AddLiquid(c, "acid", 10000);
                WaterHelper.AddLiquid(c, "base", 10000);
                Seconds(c, 60);
                R.Info("10 ml protein + 10 ml acid + 10 ml base together: protein left (equal acid and base cancel, no excess to denature)", $"{P(c, "protein") / 1000.0} ml", 1, Parts(c));
            }
            foreach (var (lipo, label) in new[] { (1000, "lipobond 1:8 fat"), (2000, "lipobond 1:4 fat") })
            {
                var c = Container();
                WaterHelper.AddLiquid(c, "water", 500000);
                WaterHelper.AddLiquid(c, "fat", 8000);
                WaterHelper.AddLiquid(c, "lipobond_agent", lipo);
                Seconds(c, 60);
                R.Info($"8 ml fat + {label}, 60 s: fat / lipobond / debris ml", $"{P(c, "fat") / 1000.0} / {P(c, "lipobond_agent") / 1000.0} / {P(c, "debris") / 1000.0}");
            }
            {
                var c = Container();
                WaterHelper.AddLiquid(c, "water", 500000);
                WaterHelper.AddLiquid(c, "debris", 20000);
                WaterHelper.AddLiquid(c, "plant_matter", 20000);
                WaterHelper.AddLiquid(c, "physical_contaminant", 5000);
                WaterHelper.AddLiquid(c, "mineral", 5000);
                WaterHelper.AddLiquid(c, "protein", 5000);
                WaterHelper.AddLiquid(c, "dream_cap_extract", 20000);
                var c2 = Container(); Pour(c, c2, WaterHelper.GetTotalVolume(c) / 2);
                var k = Filter(c, "kotton_fabric");
                var wf = Filter(c2, "water_filter");
                R.Info("kotton fabric (size 150) pour: what passes", Parts(k));
                R.Info("water filter (size 22) pour: what passes", Parts(wf));
            }
            {
                var c = Container();
                WaterHelper.AddLiquid(c, "water", 500000);
                WaterHelper.AddLiquid(c, "acid", 20000);
                WaterHelper.AddLiquid(c, "base", 12000);
                var trace = new List<string>();
                for (int s = 1; s <= 20; s++) { Seconds(c, 1); if (s is 1 or 2 or 5 or 10 or 20) trace.Add($"{s}s:a{P(c, "acid") / 1000.0}/b{P(c, "base") / 1000.0}"); }
                R.Info("20 ml acid + 12 ml base: neutralisation (10%/s of the smaller, min 1 ml, no product)", string.Join(" ", trace));
            }
        }

        // Evaporator: solvent removed per night and what the distillate slot collects.
        public static void Evaporator(int seed)
        {
            W.New(seed);
            foreach (var module in new string[] { null, "evaporator_module_solvent_recovery" })
            {
                var ev = W.Own("evaporator");
                Put(MachineHelper.GetBatterySlot(ev), Battery(100000));
                if (module != null) R.Info($"evaporator: {module} seated", Put(Storage(ev, 1), Owned(module)));
                var c = Container();
                WaterHelper.AddLiquid(c, "water", 400000);
                WaterHelper.AddLiquid(c, "alcohol", 400000);
                WaterHelper.AddLiquid(c, "dream_cap_extract", 20000);
                Put(Storage(ev, 2), c);
                var d = Owned("water_pitcher");
                Put(Storage(ev, 3), d);
                var trace = new List<string>();
                for (int n = 1; n <= 3; n++)
                {
                    W.QuietDay();
                    trace.Add($"n{n}: water {P(c, "water") / 1000.0} alc {P(c, "alcohol") / 1000.0} extract {P(c, "dream_cap_extract") / 1000.0} | distillate {Parts(d)}");
                }
                R.Info($"evaporator ({module ?? "no module"}), 400 ml water + 400 ml alcohol + 20 ml extract", string.Join(" ; ", trace), 3,
                    $"quality bonus {MachineryHelper.GetCurrentQualityBonus(ev)}");
            }
        }

        // Finisher impurity by module count, grade thresholds and value by grade.
        public static void FinisherGrades(int seed)
        {
            W.New(seed);
            for (int m = 0; m <= 4; m++)
            {
                var c = Container("100ml_cylinder");
                WaterHelper.AddLiquid(c, "dream_cap_extract", 20000);
                var made = Finish(c, "dream_dust", m);
                R.Exact($"finisher with {m} chem modules on 100% dry extract: product purity", new[] { 88, 94, 97, 99, 100 }[m], (int)Math.Round(made.Select(ChemicalProductHelper.GetPurity).Average()), made.Count, Summary(made));
            }
            {
                var c = Container("100ml_cylinder");
                WaterHelper.AddLiquid(c, "dream_cap_extract", 20000);
                WaterHelper.AddLiquid(c, "water", 101);
                var made = Finish(c, "dream_dust", 4);
                R.Exact("finisher refuses a container with >100 uL of water/alcohol/lipobond (wet)", 0, made.Count);
            }
            foreach (int pct in new[] { 100, 99, 95, 90, 85, 80, 70, 60, 50, 45, 40, 30, 20 })
            {
                var c = Container("25ml_cylinder");
                WaterHelper.AddLiquid(c, "dream_cap_extract", 20 * pct);
                WaterHelper.AddLiquid(c, "starch", 20 * (100 - pct));
                var made = Finish(c, "dream_dust", 4);
                var p = made.First();
                R.Info($"dream dust at {pct}% purity: grade, value, euphoria level, accepted by grade-1/2/3 narcotic buyers",
                    $"{ChemicalProductHelper.GetCurrentChemicalConditionId(p)}, {p.GetCurrentValue(false, true, true, false, 0)}, {ChemicalFeedbackHelper.GetEuphoriaLevel(p)}, " +
                    $"{ChemicalProductHelper.WillAcceptNarcotic(p, 1)}/{ChemicalProductHelper.WillAcceptNarcotic(p, 2)}/{ChemicalProductHelper.WillAcceptNarcotic(p, 3)}");
            }
            foreach (int pct in new[] { 100, 80, 75, 74, 60, 40 })
            {
                var c = Container("25ml_cylinder");
                WaterHelper.AddLiquid(c, "fanta", 5 * pct);
                WaterHelper.AddLiquid(c, "starch", 5 * (100 - pct));
                var p = Finish(c, "fanta_pill", 4).First();
                R.Info($"fanta pill at {pct}% purity: grade, value, euphoria, toxic, lethal",
                    $"{ChemicalProductHelper.GetCurrentChemicalConditionId(p)}, {p.GetCurrentValue(false, true, true, false, 0)}, {ChemicalFeedbackHelper.GetEuphoriaLevel(p)}, {ChemicalFeedbackHelper.IsToxic(p)}, {ChemicalFeedbackHelper.IsLethal(p)}");
            }
        }

        // Dream dust as the manual teaches it, no modules anywhere, optional kotton filter.
        public static void DreamDustBasic(int seed)
        {
            foreach (bool kotton in new[] { false, true })
            {
                W.New(seed);
                var c = Container();
                Blend(c, "dream_cap", 6, 0);
                int mush = P(c, "dream_cap_mush");
                for (int i = 0; i < 3; i++) PourAll(Supply("CreateMiniAlcoholBottle"), c);
                var plate = Plate(0, 99, 1);
                OnPlate(plate, c);
                int s = HeatUntil(c, plate, 45);
                OffPlate(c);
                Seconds(c, 300);
                string after = Parts(c);
                if (kotton) c = Filter(c, "kotton_fabric");
                int nights = Evaporate(c);
                var made = Finish(c, "dream_dust", 0);
                R.Info($"dream dust, manual chain, 0 modules, 6 caps, 3 mini alcohol, plate to 45 C ({s} s) then off{(kotton ? ", kotton filter" : ", no filter")}",
                    Summary(made), made.Count, $"mush after blend {mush / 1000.0} ml; after extraction: {after}; evaporator nights {nights}; residue left {Parts(c)}");
            }
        }

        // Dream dust with every purification step and 4 chem modules per machine.
        public static void DreamDustLabPure(int seed)
        {
            W.New(seed);
            var c = Container();
            Blend(c, "dream_cap", 6, 4);
            int mush = P(c, "dream_cap_mush");
            for (int i = 0; i < 4; i++) PourAll(Supply("CreateMiniAlcoholBottle"), c);
            var plate = Plate(4, 45, 1);
            OnPlate(plate, c);
            Seconds(c, 120, plate);
            OffPlate(c);
            string extracted = Parts(c);
            // Protein: an acid dose equal to the protein, from the acid bottle by dropper, then let it work.
            var acid = Supply("CreateAcidBottle");
            int protein = P(c, "protein");
            Pour(acid, c, (int)(protein / (P(acid, "acid") / (double)WaterHelper.GetTotalVolume(acid))));
            Seconds(c, 60);
            string deproteined = Parts(c);
            int residualAcid = P(c, "acid");
            if (residualAcid > 0)
            {
                var b = Supply("CreateBaseBottle");
                Pour(b, c, (int)(residualAcid / (P(b, "base") / (double)WaterHelper.GetTotalVolume(b))));
                Seconds(c, 30);
            }
            c = Filter(c, "kotton_fabric");
            c = Filter(c, "water_filter");
            string filtered = Parts(c);
            int nights = Evaporate(c);
            string dry = Parts(c);
            double dryPurity = Purity(c, "dream_cap_extract");
            var made = Finish(c, "dream_dust", 4);
            R.Info("dream dust, full purification, 4 modules, 6 caps, 4 mini alcohol, 45 C hold, acid=protein then base, kotton + water filter",
                Summary(made), made.Count,
                $"mush {mush / 1000.0} ml; extracted: {extracted}; after acid: {deproteined}; filtered: {filtered}; nights {nights}; dry: {dry}");
            R.Info("dry residue purity before the finisher (dream_cap_extract share)", $"{dryPurity:0.00}%");
        }

        // Caffeine from coffee beans: alcohol at 50 C vs water at 80 C; fat via lipobond, protein via acid.
        public static void Caffeine(int seed)
        {
            foreach (var (alcohol, coolFirst) in new[] { (true, true), (true, false), (false, true) })
            {
                W.New(seed);
                var c = Container();
                Blend(c, "coffee_bean", 3, 4);
                string blended = Parts(c);
                int caf0 = P(c, "caffeine"), mush0 = P(c, "coffee_mush");
                if (alcohol) PourAll(Supply("CreateMiniAlcoholBottle"), c);
                else WaterHelper.AddPureWater(c, 400000);
                var plate = Plate(4, alcohol ? 60 : 90, 1);
                OnPlate(plate, c);
                Seconds(c, 120, plate);
                OffPlate(c);
                int caf1 = P(c, "caffeine");
                string extracted = Parts(c);
                double hot = Temp(c);
                if (coolFirst) while (Temp(c) >= 50) Seconds(c, 1);
                double atLipo = Temp(c);
                int fat = P(c, "fat");
                var lipo = Supply("CreateLipobondAgentBottle");
                Pour(lipo, c, (int)(fat / 8.0 / (P(lipo, "lipobond_agent") / (double)WaterHelper.GetTotalVolume(lipo))) + 200);
                var acid = Supply("CreateAcidBottle");
                Pour(acid, c, (int)(P(c, "protein") / (P(acid, "acid") / (double)WaterHelper.GetTotalVolume(acid))));
                Seconds(c, 90);
                int residualAcid = P(c, "acid");
                if (residualAcid > 0) { var b = Supply("CreateBaseBottle"); Pour(b, c, (int)(residualAcid / (P(b, "base") / (double)WaterHelper.GetTotalVolume(b)))); Seconds(c, 30); }
                c = Filter(c, "water_filter");
                int nights = Evaporate(c);
                string dry = Parts(c);
                var made = Finish(c, "caffeine_pill", 4);
                R.Info($"caffeine pills, 3 coffee beans, {(alcohol ? "1 mini alcohol at 60 C" : "400 ml water at 90 C")}, lipobond 1:8 fat added at {atLipo:0} C (off the plate at {hot:0} C), acid = protein, water filter, 4 modules",
                    Summary(made), made.Count,
                    $"blended {blended}; caffeine {caf0 / 1000.0} -> {caf1 / 1000.0} ml from mush {mush0 / 1000.0}; extracted {extracted}; nights {nights}; dry {dry}");
            }
        }

        // Phanta (fanta) from restricted supplies: strip the stabiliser with acid, neutralise, filter, then reduce at 110-130 C.
        public static void Fanta(int seed)
        {
            var variants = new (string label, int modules, int target, int mode, double acidFrac, int tablets)[]
            {
                ("whole acid bottle, 2-module plate at 125 C high", 2, 125, 2, -1, 0),
                ("whole acid bottle, 1-module plate (fixed 160 C)", 1, 160, 2, -1, 0),
                ("acid = 1/4 of the stabiliser, 4-module plate at 128 C low", 4, 128, 1, 0.25, 0),
                ("acid = 1/4 of the stabiliser, 2 water tablets in 100 ml water, 4-module plate at 128 C low", 4, 128, 1, 0.25, 2),
            };
            foreach (var v in variants)
            {
                W.New(seed);
                var c = Container();
                PourAll(Supply("CreateFantaPrecusorBottle"), c);
                int meth0 = P(c, "methylol"), stab0 = P(c, "stabilizing_agent");
                var acid = Supply("CreateAcidBottle");
                if (v.acidFrac < 0) PourAll(acid, c);
                else Pour(acid, c, (int)(stab0 * v.acidFrac / (P(acid, "acid") / (double)WaterHelper.GetTotalVolume(acid))));
                int acidIn = P(c, "acid");
                var strip = new List<string>();
                for (int s = 1; s <= 120; s++) { Seconds(c, 1); if (s is 10 or 30 or 60 or 120) strip.Add($"{s}s stab {P(c, "stabilizing_agent") / 1000.0} acid {P(c, "acid") / 1000.0}"); }
                int acidLeft = P(c, "acid");
                if (acidLeft > 0)
                {
                    var b = Supply("CreateBaseBottle");
                    Pour(b, c, (int)(acidLeft / (P(b, "base") / (double)WaterHelper.GetTotalVolume(b))));
                    Seconds(c, 60);
                }
                c = Filter(c, "water_filter");
                if (v.tablets > 0)
                {
                    WaterHelper.AddPureWater(c, 100000);
                    for (int i = 0; i < v.tablets; i++) ChemicalHelper.DissolveInto(Owned("water_tablet"), c);
                    Seconds(c, 30);
                }
                string cleaned = Parts(c);
                var red = Supply("CreateReducingAgentBottle");
                int methLeft = P(c, "methylol");
                Pour(red, c, (int)(methLeft * 0.75 / (P(red, "reducing_agent") / (double)WaterHelper.GetTotalVolume(red))) + 1000);
                var plate = Plate(v.modules, v.target, v.mode);
                OnPlate(plate, c);
                var trace = new List<string>();
                for (int s = 1; s <= 240; s++)
                {
                    Seconds(c, 1, plate);
                    if (s is 10 or 20 or 40 or 60 or 120 or 240) trace.Add($"{s}s {Temp(c):0}C meth {P(c, "methylol") / 1000.0} fanta {P(c, "fanta") / 1000.0}");
                }
                OffPlate(c);
                string reacted = Parts(c);
                int nights = WaterHelper.IsWet(c) ? Evaporate(c) : 0;
                var made = Finish(c, "fanta_pill", 4);
                var tox = made.Count(ChemicalFeedbackHelper.IsToxic);
                R.Info($"phanta, 1 precursor bottle, {v.label}, base to neutral, water filter, reducing 0.75x methylol",
                    Summary(made), made.Count,
                    $"methylol {meth0 / 1000.0} stab {stab0 / 1000.0}; acid in {acidIn / 1000.0}; strip {string.Join(" | ", strip)}; cleaned {cleaned}; reaction {string.Join(" | ", trace)}; reacted {reacted}; nights {nights}; toxic pills {tox}");
            }
        }

        // Nomore (pink alt injector): dream extract + reducing agent + lipobond at 45-55 C with alcohol <= 10%.
        public static void Nomore(int seed)
        {
            foreach (var (t, lipoRatio, label) in new[] { (52, 0.25, "52 C, lipobond 1:4 extract"), (47, 0.25, "47 C, lipobond 1:4"), (52, 0.1, "52 C, lipobond 1:10") })
            {
                W.New(seed);
                var c = Container();
                WaterHelper.AddLiquid(c, "dream_cap_extract", 40000);
                WaterHelper.AddLiquid(c, "reducing_agent", 40000);
                WaterHelper.AddLiquid(c, "lipobond_agent", (int)(40000 * lipoRatio));
                WaterHelper.AddLiquid(c, "water", 100000);
                var plate = Plate(2, t, 1);
                OnPlate(plate, c);
                var trace = new List<string>();
                for (int s = 1; s <= 300; s++)
                {
                    Seconds(c, 1, plate);
                    if (s is 10 or 30 or 60 or 120 or 300) trace.Add($"{s}s {Temp(c):0}C ext {P(c, "dream_cap_extract") / 1000.0} nomore {P(c, "nomore") / 1000.0} lipo {P(c, "lipobond_agent") / 1000.0}");
                }
                R.Info($"nomore from 40 ml extract + 40 ml reducing, {label}, 100 ml water", string.Join(" | ", trace), 1, Parts(c));
            }
            {
                W.New(seed);
                var c = Container();
                WaterHelper.AddLiquid(c, "dream_cap_extract", 40000);
                WaterHelper.AddLiquid(c, "reducing_agent", 40000);
                WaterHelper.AddLiquid(c, "lipobond_agent", 10000);
                WaterHelper.AddLiquid(c, "alcohol", 30000);
                var plate = Plate(2, 52, 1);
                OnPlate(plate, c);
                Seconds(c, 120, plate);
                R.Info("nomore with alcohol at 25% of volume: blocked (needs alcohol <= 10%)", $"nomore {P(c, "nomore") / 1000.0} ml", 1, Parts(c));
            }
        }
    }
}
