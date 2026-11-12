using System;
using System.Collections.Generic;
using System.Linq;
using Newtonsoft.Json.Linq;

namespace Fence.Gym
{
    // The Cook's way out of the cartel (`lab.cartel: {...}`, docs/re/CHEMISTRY.md §7.3). The weekly handler takes every
    // dream dust on the counters and judges the count and the mean true purity; rep peaks at the required purity + 10, a
    // met count adds the whole count, and the exit needs rep 100 and then five more weekly visits, each raising the bar by
    // 25 to 200. So the dust is made for the visit, not ahead of it:
    //  - `bank` (default true): a dried batch is poured into a bank container instead of pressed. A container of residue
    //    carries no contraband tag, so an inspection between visits finds nothing (pressed dust in the stock grid is
    //    seized, and a visit with none ends the run).
    //  - On the visit's morning the bank is pressed: K units at the bank's purity and N − K blanks from an acid or base
    //    bottle (purity 0, and the handler counts them), chosen so the mean lands near the required purity + 10 and the
    //    count is met, keeping in the bank what the next `reserve_weeks` (default 2) visits need; the surplus goes out as
    //    extra units (+1 rep each). Cutting the bank instead would spend the same extract and dilute later weeks.
    //  - The check-in's materials (3 caps and a pack for 12 rep, 6 with Reliable Chemist) when the bank, the batch in
    //    hand and the caps held would not make the next visit's count.
    //  - Chem modules into the blender first (`blender_first`, default 2: its loss is 35 % → 20 → 10, the finisher's
    //    12 → 6 → 3), since the count wants extract and the blanks set the purity.
    //  - After the exit the lab makes `after_exit` (default: nothing), so no dust is ever sold to a buyer.
    static partial class Lab
    {
        static JObject CartelCfg => Cfg?["cartel"] as JObject;
        static bool CartelPlan => CartelCfg != null && NewGameData.Instance != null && NewGameData.Instance.startType == NewGameData.StartType.DRUG_DEALER;
        static bool Banking => Cartel && CartelCfg != null && (CartelCfg.Value<bool?>("bank") ?? true) && Makes(Dust);
        // Caps held at most: past the smuggler's bay an inspection seizes them (Security rep −3 a cap, ×1.25 on hard) and
        // below −60 the inspections come every few days.
        static int CapsMax => CartelCfg?.Value<int?>("caps_max") ?? 10;
        static int BlenderFirst => CartelCfg?.Value<int?>("blender_first") ?? 2;
        static int bank;
        public static int BankPours, Blanks, BankUnits, Extras;
        static readonly List<JObject> visits = new List<JObject>();

        static void ResetCartel()
        {
            bank = 0;
            BankPours = Blanks = BankUnits = Extras = 0;
            visits.Clear();
        }

        static bool QuotaDay(int day) => day >= 8 && day % 7 == 1;

        static IEnumerable<GameItem> BankCandidates() => Owned().Where(i => (WorkVessels.Contains(i.identifier) || (i.IsTag("LIQUID_CONTAINER_TAG") && !Glass.Contains(i.identifier) && i.identifier != "wine_bottle"))
            && InStock(i) && Total(i) == 0 && i.uniqueId != vessel
            && i.uniqueId != filterPitcher && i.uniqueId != bank && !Production.HeldForWine(i) && Capacity(i) >= 200000).OrderByDescending(Capacity).ThenBy(i => i.uniqueId);

        // In the stock grid or the chem bay, not on a counter or in the case, where a buyer takes it.
        static bool InStock(GameItem i) => ReferenceEquals(i.parentInventory, EmporiumEntry.Instance.invElement) || Parent(i)?.identifier == ChemBay;

        // The bank: the largest empty vessel left once the work vessel and the filter pitcher are picked; a fuller bank
        // moves into a larger one when one comes free.
        static GameItem Bank()
        {
            if (ById(bank) is GameItem b)
            {
                if (Total(b) > Capacity(b) * 0.6 && BankCandidates().FirstOrDefault() is GameItem big && Capacity(big) > Capacity(b))
                {
                    Chore.Act("pour", b, () => WaterHelper.TransferLiquid(b, big));
                    if (Total(b) == 0) bank = big.uniqueId;
                }
                return ById(bank);
            }
            bank = BankCandidates().FirstOrDefault()?.uniqueId ?? 0;
            // None free: two part-used alcohol minis poured into one frees the other.
            if (bank == 0)
            {
                var minis = Owned().Where(i => i.identifier == "mini_bottle" && InStock(i) && Total(i) > 0 && !Mine(i)
                    && Tag(i, "alcohol") + Tag(i, "water") >= Total(i)).OrderBy(Total).ThenBy(i => i.uniqueId).ToList();
                foreach (var a in minis)
                {
                    var into = minis.FirstOrDefault(b => b.uniqueId != a.uniqueId && Total(b) + Total(a) <= Capacity(b));
                    if (into == null) continue;
                    Chore.Act("pour", a, () => WaterHelper.TransferLiquid(a, into));
                    if (Total(a) == 0) { bank = a.uniqueId; Stall("bank_from_minis"); break; }
                }
            }
            // None free (the start's kit is a pitcher and two bottles of water): a bottle of plain water is poured out.
            if (bank == 0 && Owned().Where(i => Vessels.Contains(i.identifier) && InStock(i) && i.uniqueId != vessel && i.uniqueId != filterPitcher && !Production.HeldForWine(i)
                && Capacity(i) >= 200000 && Total(i) > 0 && Tag(i, "water") >= Total(i) * 0.9).OrderByDescending(Capacity).FirstOrDefault() is GameItem w)
            {
                Chore.Act("empty", w, () => WaterHelper.EmptyContainer(w));
                bank = w.uniqueId;
                Stall("bank_from_water");
            }
            // Last, the 100 ml cylinder (50 units; the doses use the smaller glass): the bank moves on to a larger vessel
            // as one comes free.
            if (bank == 0 && Owned().Count(i => i.identifier == "100ml_cylinder") > 0 && Glass.Skip(1).Any(g => Find(g) != null)
                && Owned().FirstOrDefault(i => i.identifier == "100ml_cylinder" && InStock(i) && Total(i) == 0) is GameItem cyl)
            {
                bank = cyl.uniqueId;
                Stall("bank_cylinder");
            }
            return ById(bank);
        }

        // A dried dust batch into the bank; the vessel is free for the next one.
        static bool BankBatch(IExecutorHost ep, GameItem v)
        {
            if (!Banking || product != Dust || v == null || WaterHelper.IsWet(v)) return false;
            var b = Bank();
            if (b == null) { Stall("bank_container"); return false; }
            // Poured where it sits: the vessel waits in the evaporator for the next batch (out of it, a full grid would
            // leave it on the back counter for a buyer's "sell all").
            int before = Total(v);
            Chore.Act("pour", v, () => WaterHelper.TransferLiquid(v, b));
            if (Total(v) > 0) Stall("bank_full");
            BankPours++;
            Batches++;
            var o = Per(Dust);
            Add(o, "batches", 1);
            var log = o["log"] as JArray ?? (JArray)(o["log"] = new JArray());
            if (log.Count < 60) log.Add(new JObject { ["day"] = ep.Day, ["inputs"] = batchInputs, ["banked_ml"] = (before - Total(v)) / 1000.0, ["filter"] = batchFilter, ["b"] = Fitted("blender") });
            step = Step.Idle;
            bathDone = feeding = false;
            product = null;
            return true;
        }

        // Out of a machine slot into the stock grid (room made), the chem bay or the case: a vessel left in the
        // evaporator's slot blocks every later batch.
        static void Out(GameItem i)
        {
            var e = EmporiumEntry.Instance;
            if (ReferenceEquals(i.parentInventory, e.invElement)) return;
            // Not the back counter: a buyer's "sell all" takes what is there (the bank went that way once, a week's dust in it).
            if (Chore.Accept(e.invElement, i, -1) == 0) return;
            if (MakeRoom(PlayerRole.Cells(i)) && Chore.Accept(e.invElement, i, -1) == 0) return;
            if (Find(ChemBay) is GameItem bay && GraphUtils.FindChildrenType<GameInventory>(bay).FirstOrDefault() is GameInventory inside && Chore.Accept(inside, i, -1) == 0) return;
            if (Chore.Accept(e.showcaseElement, i, -1) != 0) { Chore.Accept(e.backInvinvElement, i, -1); Stall("out_to_counter"); }
        }

        // Each morning: a container in the evaporator that is not the work vessel comes out.
        static void Tidy(GameItem evap)
        {
            if (!Banking || evap == null) return;
            if ((Slot(evap, 2) as GameSlotInventory)?.childItem is GameItem c && c.uniqueId != vessel) { Out(c); Stall("evaporator_tidy"); }
        }

        static double BankPurity(GameItem b)
        {
            int t = Total(b);
            return t <= 0 ? 0 : Part(b, "dream_cap_extract") / (double)t * (1 - FinisherLoss[Clamp4(Fitted("chem_finisher"))]) * 100;
        }

        static double Gain => Difficulty.Rules.reputationGainMultiplier;
        static double Loss => Difficulty.Rules.reputationLossMultiplier;
        static double Mod(int v) => v > 0 ? v * Gain : v * Loss;

        // The rep after a visit of n units at mean purity `mean` (StoreClientListCartel.CreateCartelHandlerWeekly,
        // BuildPurityChain), with the difficulty's multipliers: the count's change, then the purity's, each clamped to
        // ±200 as it lands, so a purity loss after a count that hit the clamp leaves the rep under 200. Null for a game
        // over, with a margin over the source's req / 4.
        static double? Verdict(double rep0, int n, double mean, int count, int req)
        {
            if (n <= 0 || mean <= req / 4 + 5) return null;
            double rep = Math.Max(-200, Math.Min(200, rep0 + Mod(n < count ? -4 * (count - n) : n)));
            rep += mean <= req + 10 ? Mod(-(int)(req - mean)) : Mod((int)(mean - req) / 4);
            return Math.Max(-200, Math.Min(200, rep));
        }

        // Bank units the next `weeks` visits need at their required purity + 10.
        static double QuotaReserve(double pb, int weeks)
        {
            double need = 0;
            for (int w = 1; w <= weeks; w++)
                need += (QuotaCount + 5 * w) * Math.Min(70, QuotaPurity + 5 * w + 10) / Math.Max(1, pb);
            return need;
        }

        static bool pressedToday;

        // The visit's morning: the dried batch into the bank, then blanks and the bank pressed for the handler.
        static void QuotaMorning(IExecutorHost ep, GameItem finisher)
        {
            pressedToday = false;
            if (!Banking || !QuotaDay(ep.Day)) return;
            var v = ById(vessel);
            if (v != null && (step == Step.Drying || step == Step.Dried) && product == Dust && !WaterHelper.IsWet(v))
            {
                var plate = Find("heating_plate");
                if (step == Step.Dried && plate != null && ReferenceEquals((Slot(plate, 1) as GameSlotInventory)?.childItem, v)) PlateOff(plate);
                BankBatch(ep, v);
            }
            var b = ById(bank);
            int count = QuotaCount, req = QuotaPurity;
            double pb = b == null ? 0 : BankPurity(b);
            int units = b == null ? 0 : Total(b) / 2000;
            var acid = Bottles("acid").Concat(Bottles("base")).ToList();
            int blanksMax = acid.Sum(x => Total(x) / 2000);
            if (blanksMax < count) { OpenPack(Pack, () => false); acid = Bottles("acid").Concat(Bottles("base")).ToList(); blanksMax = acid.Sum(x => Total(x) / 2000); }
            // The plan: the best verdict (rep counts up to its 200 clamp) less the bank units it spends, each worth
            // `unit_value` (0.25 rep) while the bank holds more than the next `reserve_weeks` (2) visits need, and
            // `reserve_value` (3) below that.
            double reserve = QuotaReserve(pb, CartelCfg.Value<int?>("reserve_weeks") ?? 2);
            int extraMax = CartelCfg.Value<int?>("extra_max") ?? 40;
            double lo = CartelCfg.Value<double?>("unit_value") ?? 0.25, hi = CartelCfg.Value<double?>("reserve_value") ?? 3;
            double surplus = Math.Max(0, units - reserve), rep0 = StoreReputation.GetCartelReputation();
            var perk = StoreReputation.GetPerkByID("CARTEL_AUTHORIZED_EXIT");
            double exitBar = perk != null && (perk.isUnlocked || rep0 >= perk.perkThreshold - 60) ? perk.perkThreshold : 0;
            (int n, int k, double rep) best = (0, 0, double.NegativeInfinity);
            int room = CounterRoom() - 2;
            // Met count at a mean that still earns the module (over req − 5) first; only when the bank cannot reach it
            // does any verdict that keeps the run alive do.
            foreach (bool strict in new[] { true, false })
            {
                for (int n = 1; n <= Math.Min(count + extraMax, Math.Max(1, room)); n++)
                    for (int k = 0; k <= Math.Min(units, n); k++)
                    {
                        if (n - k > blanksMax) continue;
                        double mean = k * pb / n;
                        if (strict && (n < count || mean <= req - 5)) continue;
                        double? r = Verdict(rep0, n, mean, count, req);
                        if (r == null) continue;
                        // Authorized, the visit moves the exit on only while the rep holds the raised threshold.
                        double score = r.Value + (exitBar > 0 && r.Value >= exitBar ? 50 : 0) - lo * Math.Min(k, surplus) - hi * Math.Max(0, k - surplus);
                        if (score > best.rep + 1e-9) best = (n, k, score);
                    }
                if (best.n > 0) break;
            }
            var rec = new JObject { ["room"] = room, ["day"] = ep.Day, ["count"] = count, ["purity"] = req, ["rep_before"] = StoreReputation.GetCartelReputation(), ["bank_units"] = units, ["bank_purity"] = Math.Round(pb, 2), ["reserve"] = Math.Round(reserve, 1), ["plan_n"] = best.n, ["plan_k"] = best.k };
            visits.Add(rec);
            if (best.n == 0) { Stall("quota_nothing"); return; }
            pressedToday = true;
            // Blanks first, the bank last: it stays seated in the finisher (a full stock grid would leave it on the back
            // counter, where a buyer's "sell all" takes it, a week's dust in it).
            int blanks = 0;
            foreach (var a in acid)
            {
                if (blanks >= best.n - best.k) break;
                blanks += PressFrom(ep, finisher, a, best.n - best.k - blanks, out _);
            }
            // Blanks short (no room for the bottle, a bottle gone): the count from the bank rather than a short visit.
            int want = best.k + Math.Max(0, count - best.k - blanks);
            if (want > best.k) Stall("blanks_short");
            int pressed = PressFrom(ep, finisher, b, want, out double sum, leave: true);
            BankUnits += pressed;
            Blanks += blanks;
            Extras += Math.Max(0, pressed + blanks - count);
            rec["blanks_max"] = blanksMax;
            rec["pressed"] = pressed;
            rec["blanks"] = blanks;
            rec["mean"] = pressed + blanks > 0 ? Math.Round(sum / (pressed + blanks), 2) : 0;
        }

        // Every dust held onto the counters, the purest first (back, then front): should the counters fill, what is
        // staged still clears the purity.
        static int StageBanked()
        {
            var e = EmporiumEntry.Instance;
            int n = 0;
            var held = DustHeld().Where(i => !ReferenceEquals(i.parentInventory, e.backInvinvElement) && !ReferenceEquals(i.parentInventory, e.frontInvinvElement)).ToList();
            // Units pressed before the bank existed: the count, nearest the required purity + 10 first; the rest stays for
            // the next visit (a visit with none ends the run).
            if (!pressedToday) held = held.OrderBy(i => Math.Abs(Grade(i) - QuotaPurity - 10)).ThenBy(i => i.uniqueId).Take(QuotaCount).ToList();
            foreach (var d in held.OrderByDescending(Grade).ThenBy(i => i.uniqueId).ToList())
                if (Chore.Accept(e.backInvinvElement, d, -1) == 0 || Chore.Accept(e.frontInvinvElement, d, -1) == 0) n++;
                else Stall("counter_full");
            return n;
        }

        static int CounterRoom()
        {
            var e = EmporiumEntry.Instance;
            return Free(e.backInvinvElement) + Free(e.frontInvinvElement);
        }

        // Up to `n` dream dust from container `c` through the finisher; the units go to the stock grid, the counters when
        // it is full. `leave` keeps the container seated. Returns the units pressed and their summed spectrometer purity.
        static int PressFrom(IExecutorHost ep, GameItem finisher, GameItem c, int n, out double puritySum, bool leave = false)
        {
            puritySum = 0;
            if (c == null || n <= 0) return 0;
            var stock = EmporiumEntry.Instance.invElement;
            var back = EmporiumEntry.Instance.backInvinvElement;
            var slot = Slot(finisher, 1) as GameSlotInventory;
            if (slot?.childItem is GameItem old && old.uniqueId != c.uniqueId) Out(old);
            if (Chore.Accept(Slot(finisher, 1), c, -1) != 0) { Stall("finisher"); return 0; }
            Chore.Act("configure", finisher, () => finisher.ModifyTag("SELECTED_CHEM_ID_TAG", s => s.Enable().SetString(Dust)));
            var output = (GameInventory)Slot(finisher, 2);
            string label = LocHelper.GetLocalizedMechanic("mech_chem_finisher_btn_output");
            var o = Per(Dust);
            var grades = (JObject)o["grades"];
            int pressed = 0;
            for (int k = 0; k < n && MachineChemFinisher.CanOutput(finisher, slot); k++)
            {
                // A unit pressed into a full output grid is lost (MachineChemFinisher ignores the placement).
                if (output.childItems.Count(x => x != null) >= 20) { Stall("output_full"); break; }
                var press = ep.MachineButton(finisher, label);
                if (press == null) break;
                press();
                pressed++;
                foreach (var p in output.childItems.Where(x => x != null).ToList())
                {
                    if (!made.Add(p.uniqueId)) continue;
                    Chore.Act("own", p, () => GeneralHelper.SetItemOwned(p, true));
                    Pills++;
                    puritySum += Grade(p);
                    Add(o, "units", 1);
                    Add(grades, ChemicalProductHelper.GetCurrentChemicalConditionId(p) ?? "none", 1);
                    if (Chore.Accept(stock, p, -1) != 0 && Chore.Accept(back, p, -1) != 0) Chore.Accept(EmporiumEntry.Instance.frontInvinvElement, p, -1);
                }
            }
            if (!leave) Out(c);
            return pressed;
        }

        // The check-in's materials: when the bank, the batch in hand and the caps held come short of the next visit.
        static bool CartelMaterialsBanked()
        {
            string mode = CartelCfg.Value<string>("materials") ?? "need";
            if (mode == "always") return true;
            if (mode == "never") return false;
            var blender = Find("blender");
            int loss = blender == null ? 35 : MachineBlender.GetLoss(blender);
            double keep = 1 - FinisherLoss[Clamp4(Fitted("chem_finisher"))];
            // Extract (µL) at hand: the bank, the batch (its extract and the mush still to convert), the caps held.
            double extract = 0;
            if (ById(bank) is GameItem b) extract += Part(b, "dream_cap_extract");
            if (ById(vessel) is GameItem v && product == Dust) extract += Part(v, "dream_cap_extract") + Part(v, "dream_cap_mush") / 2.0;
            extract += Owned().Count(i => i.identifier == "dream_cap" && Loose(i)) * 22000 * (100 - loss) / 100.0 / 2;
            double need = (QuotaCount + (CartelCfg.Value<int?>("materials_extra") ?? 5)) * 2000 * Math.Min(100, QuotaPurity + 10) / 100.0 / keep;
            return extract < need * (CartelCfg.Value<double?>("materials_margin") ?? 1.5);
        }

        static JObject CartelStatus() => new JObject
        {
            ["bank_ml"] = ById(bank) is GameItem b ? Total(b) / 1000.0 : 0, ["bank_purity"] = ById(bank) is GameItem b2 ? Math.Round(BankPurity(b2), 2) : 0,
            ["pours"] = BankPours, ["bank_units"] = BankUnits, ["blanks"] = Blanks, ["extras"] = Extras, ["visits"] = new JArray(visits.Select(x => (JToken)x.DeepClone())),
        };
    }
}
