using System;
using System.Collections.Generic;
using System.Linq;
using Newtonsoft.Json.Linq;

namespace Fence.Gym
{
    // `clean_water` (docs/research/QUALITY.md, "The staged clean-water build"): the taps after Georgio's loan, then hydroreed
    // bays whose innate purifying IV strips ~29 % of every contaminant but minerals from their own water slot each
    // night, at any volume (docs/re/PLANTS.md §4; lab verdict.reedcure). Each reed holds a vessel (a 20 L tank once one
    // is bought, a 6 L jug until then) filled at the faucet through the pitcher's filter; when the rule says it is clean
    // it is tested (the label becomes the truth), decanted into loose store vessels and refilled. The bays drink the
    // cured water, the bottles take it when it tests High Quality, and the water engine's fountain pours it.
    //   reeds (1)        reed bays from the start of the build
    //   reeds_max        more reed bays once `grow_cash` (2 500) is in hand
    //   reserve (1000)   cash kept over the rent when a tap or power upgrade is bought
    //   tap ("rust")     "rust" buys RUSTWATER_TAP too: filtered rust is HQ by night 6 and plant-safe by 10,
    //                    filtered gutter by 11 and 12; "none" buys no tap and cures bought water for the bays
    //   reed_after (1)   bloomberry bays planted before a cellar's bay goes to a reed
    //   from_day (44)    the first day the taps and the power kit may be bought. Georgio borrows half the cash on day 43
    //                    and repays four times it, so a credit spent before then costs three; the permit (day 38)
    //                    keeps its fee in hand when the build starts earlier
    //   cure             "safe" (a cellar's default) decants once plant-safe (99.5 % from filtered rust); "hq" (a water
    //                    route's) once a strip reads High Quality, into the bottles and jugs it sells
    //   store_l          litres of loose jugs and tanks kept to decant into (24 for a cellar, 0 for a water route)
    //   tank_l (20)      scrap metal grows the reed's empty tank to this many litres (+2 L a scrap, 50 at most)
    //   bottles (8)      empty wine bottles to keep in hand while the cured water covers them
    //   treat (true)     the filter + tablet pipeline (Water.cs) fills plain bottles with Basewater while the reeds ramp
    //   pure (true)      a water route with a fountain filters twice, cures to the reed's floors and uses the UV filter
    //                    (bought once, never used up): Pure for the fountain's Pure slot
    internal static partial class Production
    {
        static JObject CleanPlan => Directives.Current.Raw["clean_water"] as JObject;
        static bool CleanOn => CleanPlan != null && (WineOn || WaterSellOn);
        static int Reeds => CleanOn ? Math.Max(0, CleanPlan.Value<int?>("reeds") ?? 1) : 0;
        static bool ReedOn => Reeds > 0;
        static bool RustTap => (CleanPlan?.Value<string>("tap") ?? "rust") == "rust";
        // `tap: none`: no taps; the reed cures bought water (any label from Rustwater up) for the bays alone.
        static bool NoTap => CleanPlan?.Value<string>("tap") == "none";
        static bool WaterReady => TapOn || NoTap;
        static int CleanFromDay => CleanPlan?.Value<int?>("from_day") ?? 44;
        // A water route sells what the reeds cure: decanted into its bottles and jugs, at the High Quality test.
        static bool SellsCured => WaterSellOn && !WineOn;
        static bool CureHq => (CleanPlan?.Value<string>("cure") ?? (SellsCured ? "hq" : "safe")) == "hq";
        static int StoreMl => (CleanPlan?.Value<int?>("store_l") ?? (SellsCured || CleanPlan?.Value<string>("tap") == "none" ? 0 : 24)) * 1000;
        static bool CleanTreat => CleanPlan?.Value<bool?>("treat") ?? true;
        static int BottleCap => CleanOn && WineOn ? CleanPlan.Value<int?>("bottles") ?? 8 : 8;

        static int ReedTarget()
        {
            int max = Math.Max(Reeds, CleanPlan?.Value<int?>("reeds_max") ?? Reeds);
            return max > Reeds && PlayerStore.Instance.playerCash >= (CleanPlan.Value<int?>("grow_cash") ?? 2500) ? max : Reeds;
        }

        static bool TapForReedsOnly => ReedOn && !WaterSellOn && !(Directives.Current.Raw.Value<bool?>("tap") ?? false) && !CleanTreat;

        // ---- the taps ----------------------------------------------------------------------------------

        // Only the ones wanted today: an id in this list is bought as an essential (PlayerRole.Upgrades), past every gate.
        public static IEnumerable<string> ReedUpgrades() => ReedUpgradeIds().Where(WantsReedUpgrade);

        static IEnumerable<string> ReedUpgradeIds()
        {
            if (!ReedOn) yield break;
            // The reed seeds and bays come from the Retired Farmer (a wine route has him already).
            if (!WineOn) yield return "RETIRED_FARMER";
            if (!NoTap)
            {
                yield return "WATER_TRADER";
                yield return "GUTTERFLOW_TAP";
                if (RustTap) yield return "RUSTWATER_TAP";
            }
            foreach (var id in PowerUpgrades()) yield return id;
        }

        // The permit official first comes on day 38 and asks 2 000 on hard: until it is bought, a tap leaves the fee in
        // hand, or the shop cannot pay, the refusal books OPERATING_WITHOUT_LICENSE and the record ends in an arrest
        // (docs/research/PRODUCTION-VERDICTS.md §8).
        public static bool WantsReedUpgrade(string id)
        {
            if (!ReedOn || (PlayerDay.Current?.Day ?? 0) < CleanFromDay || !ReedUpgradeIds().Contains(id)) return false;
            var ps = PlayerStore.Instance;
            int cost = NetworkUpgrade.GetUpgradeById(id)?.GetCost() ?? 0;
            // Cash is the counter's stock money too: the build waits for `reserve` (1 000) over the rent.
            int keep = (PlayerDay.Current?.RentDue ?? 0) + (CleanPlan.Value<int?>("reserve") ?? 1000);
            if (!ps.hasBusinessPermit && (Directives.Current.Permit ?? "buy") == "buy") keep += PlayerStore.GetBusinessPermitPrice();
            return ps.playerCash - cost >= keep;
        }

        // ---- what the build buys -------------------------------------------------------------------------

        static bool StoreVessel(GameItem i) => (i.identifier == "water_jug" || i.identifier == "water_tank") && Parent(i) == null && !Lab.Holds(i);

        static int StoreRoomMl() => Owned().Where(i => StoreVessel(i) && (WaterHelper.IsEmpty(i) || cleanStore.Contains(i.uniqueId))).Sum(Capacity) / 1000;

        // The engine's own suppliers. A general seller's lot that holds a wanted input is bought for the inputs alone
        // (PlayerRole.StageFromSeller), so the kit is bought only from these: the counter trade keeps its stock money.
        static readonly HashSet<string> KitSellers = new HashSet<string>(StringComparer.Ordinal)
            { "retired_farmer", "retired_water_merchant", "retired_winemaker", "retired_chemist", "waterSeller", "inventor" };

        static bool AtKitSeller()
        {
            string who = PlayerStore.Instance?.currentClientInstance?.GetClientBlueprint()?.identifier;
            return who == null || KitSellers.Contains(who);
        }

        // How many of an id the build wants (-1: not the build's to answer).
        static int CleanWants(string id)
        {
            if (!ReedOn || id == null) return -1;
            if (CleanInput(id) && id != "energy_credit" && id != "hydroponic" && !AtKitSeller()) return 0;
            var owned = Owned().ToList();
            int reeds = ReedBays().Count;
            switch (id)
            {
                case ReedSeedId: return Math.Max(0, 1 + ReedTarget() - reeds - owned.Count(i => i.identifier == ReedSeedId && Parent(i) == null));
                // A reed's tank: one each (a tank cures 20 L in the nights a jug cures 6).
                case "water_tank": return TapOn && !NoTap ? Math.Max(0, ReedTarget() + (StoreMl >= 20000 ? 1 : 0) - owned.Count(i => i.identifier == "water_tank")) : 0;
                case "water_jug":
                    if (!WaterReady) return 0;
                    int room = StoreRoomMl();
                    bool reedVessel = owned.Any(i => i.identifier == "water_tank") || owned.Any(i => i.identifier == "water_jug" && Parent(i) != null && IsReed(MachineHydroponic.GetInputSlot(Parent(i))?.childItem));
                    return Math.Max(0, (StoreMl - room + 5999) / 6000) + (reedVessel ? 0 : 1);
                case "hydroponic":
                    // The reed's bay comes first once the taps are on; the cellar's bays after it as before.
                    if (WaterReady && reeds < ReedTarget() && !Bays().Any(b => MachineHydroponic.GetInputSlot(b).childItem == null))
                        return PlayerStore.Instance.playerCash > (PlayerDay.Current?.RentDue ?? 0) + 900 ? 1 : 0;
                    return WineOn ? -1 : 0;
                case "uv_filter": return PureOn && Uv() == null ? 1 : 0;
                case "energy_credit":
                    int cells = PowerCellWants();
                    if (cells >= 0) return cells;
                    return WineOn ? -1 : Math.Max(0, reeds + 1 - owned.Count(i => Charged(i) && Parent(i) == null));
            }
            return -1;
        }

        static bool CleanInput(string id) => ReedOn && (id == ReedSeedId || id == "water_tank" || id == "water_jug" || id == "hydroponic" || id == "energy_credit" || (PureOn && id == "uv_filter"));

        // ---- the hydroreed bay ---------------------------------------------------------------------------

        const string ReedSeedId = "hydroreed_seed";
        static bool IsReed(GameItem seed) => seed != null && seed.identifier == ReedSeedId;
        static List<GameItem> ReedBays() => Bays().Where(IsReedBay).ToList();

        // A bay once given a reed stays the reed's (the reed is harvested for its seeds every few nights and replanted;
        // its vessel stays in the water slot).
        static readonly HashSet<int> reedBayIds = new HashSet<int>();

        static bool IsReedBay(GameItem bay) => reedBayIds.Contains(bay.uniqueId) || IsReed(MachineHydroponic.GetInputSlot(bay).childItem);

        // An empty bay becomes a reed's while the taps are on and fewer reed bays stand than the build wants.
        static bool ReedBayDue(GameItem bay) => ReedOn && (reedBayIds.Contains(bay.uniqueId) || (WaterReady && ReedBays().Count < ReedTarget()
            && (!WineOn || Bays().Count(b => MachineHydroponic.GetInputSlot(b).childItem?.identifier == "bloomberry_seed") >= (CleanPlan.Value<int?>("reed_after") ?? 1))));

        static GameItem ReedSeed() => Owned().Where(i => i.identifier == ReedSeedId && Parent(i) == null).OrderBy(i => i.uniqueId).FirstOrDefault();

        sealed class Batch
        {
            public int Day, From, Nights;
            public bool Filtered, Twice, Tested, Cured;
        }

        // `pure` (on for a water route with a fountain): batches filtered twice and cured to Pure for its Pure slot.
        static bool PureOn => SellsCured && Fountain.On && (CleanPlan?.Value<bool?>("pure") ?? true);
        const int PureNights = 12;
        static GameItem Uv() => Owned().FirstOrDefault(i => i.identifier == "uv_filter");
        public static int PureBatches;

        static readonly Dictionary<int, Batch> batches = new Dictionary<int, Batch>();
        static readonly HashSet<int> reedClean = new HashSet<int>(), reedHq = new HashSet<int>(), cleanStore = new HashSet<int>(), poweredBay = new HashSet<int>();
        public static int ReedCured, ReedCuredMl, ReedFirstDay, ReedFills, ReedFiltered, TankGrowths;

        static void ResetCellar()
        {
            batches.Clear();
            reedBayIds.Clear();
            reedClean.Clear();
            reedHq.Clear();
            cleanStore.Clear();
            poweredBay.Clear();
            ReedCured = ReedCuredMl = ReedFirstDay = ReedFills = ReedFiltered = TankGrowths = PureBatches = 0;
        }

        static bool PlantSafe(GameItem c) => reedClean.Contains(c.uniqueId);

        // µl, as the tag holds it.
        static int Capacity(GameItem i) => i.GetTagReadonly("LIQUID_CONTAINER_CAPACITY").GetInt();

        // Nights until the worst contaminant is under 10 bp of water: faucet water carries 30 % of its impurity as chemical,
        // the slowest to go, and the reed takes ~29 % of what is left a night (lab verdict.reedcure: rust 90.75 % in 10,
        // gutter 85 % in 12; the filter leaves the chemical as it is).
        static int SafeNights(int purityBp)
        {
            double chem = 0.30 * (10000 - purityBp) * 10000.0 / Math.Max(1, purityBp);
            return chem <= 10 ? 1 : (int)Math.Ceiling(Math.Log(10 / chem) / Math.Log(0.71));
        }

        // The tap's minerals (15 % of its impurity, WATER.md §1.4) survive the reed; the pitcher's filter takes most of them
        // (lab: rust 152 -> 21 bp, gutter 264 -> 79), which the bays' tablets then make up.
        static int TapMineralBp(int purityBp, bool filtered)
        {
            int bp = (int)(0.15 * (10000 - purityBp) * 10000.0 / Math.Max(1, purityBp));
            return filtered ? bp / 4 : bp;
        }

        // Each reed bay, after its power: count last night if the bay ran, cure, decant, refill.
        static void TendReed(IExecutorHost ep, GameItem bay)
        {
            reedBayIds.Add(bay.uniqueId);
            var ws = MachineHydroponic.GetWaterSlot(bay);
            var c = ws.childItem;
            var stock = EmporiumEntry.Instance.invElement;
            // A tank takes over from the jug once the jug's batch is out (a reed cures any volume in the same nights).
            var bigger = Owned().Where(i => StoreVessel(i) && WaterHelper.IsEmpty(i) && (c == null || Capacity(i) > Capacity(c)))
                .OrderByDescending(Capacity).ThenBy(i => i.uniqueId).FirstOrDefault();
            if (bigger != null && (c == null || WaterHelper.GetCurrentCapacityML(c) < 200))
            {
                if (c != null && Chore.Accept(stock, c, -1) == 0)
                {
                    if (!WaterHelper.IsEmpty(c)) Chore.Act("empty", c, () => WaterHelper.EmptyContainer(c));
                    Forget(c);
                    c = null;
                }
                if (c == null && Chore.Accept(ws, bigger, -1) == 0) c = bigger;
                else c = ws.childItem;
            }
            // No jug or tank yet: an empty bought bottle, or (tapless) a bought container moved in whole.
            if (c == null)
            {
                var v = Owned().Where(i => WaterVessel(i) && WaterHelper.IsEmpty(i)).OrderByDescending(Capacity).ThenBy(i => i.uniqueId).FirstOrDefault()
                    ?? (NoTap ? Owned().Where(i => KnownClean(i, 4) && !PlantSafe(i) && !cleanStore.Contains(i.uniqueId)).OrderByDescending(Capacity).ThenBy(Label).ThenBy(i => i.uniqueId).FirstOrDefault() : null);
                if (v != null && Chore.Accept(ws, v, -1) == 0)
                {
                    c = v;
                    if (!WaterHelper.IsEmpty(c))
                    {
                        batches[c.uniqueId] = new Batch { Day = ep.Day, From = Label(c) };
                        ReedFills++;
                    }
                }
            }
            if (c == null) return;
            // Water the build did not put there (a bloomberry's, before the reed): out to the stock, or poured away.
            if (!batches.ContainsKey(c.uniqueId) && !WaterHelper.IsEmpty(c))
            {
                if (!StoreVessel(c) && Chore.Accept(stock, c, -1) == 0)
                {
                    c = Owned().Where(i => StoreVessel(i) && WaterHelper.IsEmpty(i)).OrderByDescending(Capacity).ThenBy(i => i.uniqueId).FirstOrDefault();
                    if (c == null || Chore.Accept(ws, c, -1) != 0) return;
                }
                else Chore.Act("empty", c, () => WaterHelper.EmptyContainer(c));
            }
            if (batches.TryGetValue(c.uniqueId, out var b) && !b.Cured)
            {
                if (poweredBay.Contains(bay.uniqueId)) b.Nights++;
                int need = SafeNights(b.From);
                bool cured = Scanner()
                    ? new[] { "heavy_metal", "chemical_contaminant", "microbe", "organic_waste", "physical_contaminant" }.All(id => WaterHelper.GetContaminantPercentage(c, id) <= 10)
                    : b.Nights >= need;
                // `pure`: a twice-filtered batch stays to the reed's floors (~12 nights), then the UV filter turns its
                // microbes to water: 9990 bp, Pure (docs/research/FORMULA-EXPLOITS.md §7).
                bool pure = PureOn && b.Filtered && b.Twice && Uv() != null;
                if (pure) cured = b.Nights >= Math.Max(need, PureNights);
                if (cured && pure)
                {
                    Chore.Act("uv", c, () => WaterHelper.OnUVUsed(c));
                    b.Tested = TestWater(c);
                    if (b.Tested && LabelGrade(c) == 0) PureBatches++;
                }
                // `cure: hq`: filtered water is decanted as soon as a strip reads High Quality (night ~6 from rust).
                if (!cured && !pure && CureHq && b.Filtered && b.Nights >= need - 4 && TestWater(c))
                {
                    b.Tested = true;
                    cured = LabelGrade(c) <= 1;
                }
                if (cured) Cure(ep, bay, c, b, plantSafe: b.Nights >= need || Scanner());
            }
            if (batches.TryGetValue(c.uniqueId, out b) && b.Cured) Decant(c, b);
            if (WaterHelper.GetCurrentCapacityML(c) < 200) Refill(ep, c);
            var bat = MachineHelper.GetBatterySlot(bay)?.childItem;
            if (bat != null && PowerHelper.GetAvailableEnergyFromItem(bat, false) >= MachineryHelper.GetMachinePowerUsage(bay)) poweredBay.Add(bay.uniqueId);
            else poweredBay.Remove(bay.uniqueId);
            if (Environment.GetEnvironmentVariable("FENCE_BAY_DEBUG") != null)
                Console.Error.WriteLine($"reed d{ep.Day} {bay.uniqueId} {c.identifier}#{c.uniqueId} ml={WaterHelper.GetCurrentCapacityML(c)} n={b?.Nights ?? -1} filt={b?.Filtered} cured={b?.Cured} label={Label(c)} true={WaterHelper.GetWaterPurity(c)} store={StoreRoomMl()} tap={TapOn}");
        }

        static void Forget(GameItem c)
        {
            batches.Remove(c.uniqueId);
            reedClean.Remove(c.uniqueId);
            reedHq.Remove(c.uniqueId);
            cleanStore.Remove(c.uniqueId);
            mineralBp.Remove(c.uniqueId);
        }

        static void Cure(IExecutorHost ep, GameItem bay, GameItem c, Batch b, bool plantSafe)
        {
            b.Cured = true;
            // The strip puts the true grade on the label (the reed never relabels): the bottles and the fountain read it.
            if (!b.Tested) b.Tested = TestWater(c);
            if (b.Tested && LabelGrade(c) <= 1) reedHq.Add(c.uniqueId);
            if (plantSafe) reedClean.Add(c.uniqueId);
            if (!Scanner()) mineralBp[c.uniqueId] = TapMineralBp(b.From, b.Filtered);
            ReedCured++;
            ReedCuredMl += WaterHelper.GetCurrentCapacityML(c);
            if (ReedFirstDay == 0) ReedFirstDay = ep.Day;
            ep.Human("production", null, ep.Uid(bay), new JObject { ["action"] = "reed_cured", ["ml"] = WaterHelper.GetCurrentCapacityML(c), ["nights"] = b.Nights, ["label"] = Label(c) }, "player:bay");
        }

        // The cured batch into the loose stores (empty ones, or ones already holding cured water), so the reed can start
        // the next. Pouring into an empty vessel copies the tested label; into a cured one it keeps that one's.
        static void Decant(GameItem c, Batch b)
        {
            bool safe = PlantSafe(c), hq = reedHq.Contains(c.uniqueId);
            int bp = mineralBp.TryGetValue(c.uniqueId, out int m) ? m : 0;
            foreach (var s in Owned().Where(i => (StoreVessel(i) || SellsCured && WaterVessel(i) && !treatedOn.ContainsKey(i.uniqueId)) && i != c && !WaterHelper.IsFull(i)
                         && (WaterHelper.IsEmpty(i) || (cleanStore.Contains(i.uniqueId) && PlantSafe(i) == safe && reedHq.Contains(i.uniqueId) == hq)))
                         .OrderByDescending(i => WaterHelper.IsEmpty(i) ? 0 : 1).ThenByDescending(Capacity).ThenBy(i => i.uniqueId).ToList())
            {
                if (WaterHelper.GetCurrentCapacityML(c) <= 0) break;
                int before = WaterHelper.GetCurrentCapacityML(s);
                Chore.Act("pour", s, () => WaterHelper.TransferLiquid(c, s));
                int added = WaterHelper.GetCurrentCapacityML(s) - before;
                if (added <= 0) continue;
                cleanStore.Add(s.uniqueId);
                if (safe) reedClean.Add(s.uniqueId);
                if (hq) reedHq.Add(s.uniqueId);
                if (b.Tested) Vouch(s);
                int sb = mineralBp.TryGetValue(s.uniqueId, out int x) ? x : 0;
                mineralBp[s.uniqueId] = (sb * before + bp * added) / (before + added);
            }
        }

        // At the faucet: through the pitcher's filter when one has durability (minerals, organics and particles out, so
        // the reed has only chemical, heavy metal and microbes to take), else raw. A filtered batch never gets raw water
        // on top (it would carry the minerals and organics back in).
        static void Refill(IExecutorHost ep, GameItem c)
        {
            if (NoTap) { RefillBought(ep, c); return; }
            if (!TapOn) return;
            faucet ??= Chore.Faucet();
            if (!WaterHelper.IsEmpty(c)) Chore.Act("empty", c, () => WaterHelper.EmptyContainer(c));
            Forget(c);
            GrowTank(c);
            var pitcher = Pitcher();
            bool filtered = false;
            bool twice = false;
            if (pitcher != null && FreshFilter(pitcher))
            {
                if (!WaterHelper.IsEmpty(pitcher)) Chore.Act("empty", pitcher, () => WaterHelper.EmptyContainer(pitcher));
                // A second pass needs a vessel to hold a pitcherful between the faucet and the pitcher.
                var hold = PureOn && Uv() != null ? Owned().Where(i => WaterVessel(i) && WaterHelper.IsEmpty(i) && i != c && Capacity(i) >= 2000000)
                    .OrderBy(Capacity).ThenBy(i => i.uniqueId).FirstOrDefault() : null;
                twice = hold != null;
                int guard = 0;
                while (!WaterHelper.IsFull(c) && guard++ < 40 && (FilterLeft(pitcher) > 0 || FreshFilter(pitcher)))
                {
                    Chore.Act("tap", pitcher, () => pitcher.onThisTargetItemFunc?.Invoke(pitcher, faucet));
                    if (WaterHelper.IsEmpty(pitcher)) break;
                    if (twice)
                    {
                        Chore.Act("pour", hold, () => WaterHelper.TransferLiquid(pitcher, hold));
                        if (FilterLeft(pitcher) <= 0 && !FreshFilter(pitcher)) { twice = false; Chore.Act("pour", c, () => WaterHelper.TransferLiquid(hold, c)); break; }
                        Chore.Act("pour", pitcher, () => WaterHelper.TransferLiquid(hold, pitcher));
                    }
                    Chore.Act("pour", c, () => WaterHelper.TransferLiquid(pitcher, c));
                }
                if (!WaterHelper.IsEmpty(pitcher)) Chore.Act("empty", pitcher, () => WaterHelper.EmptyContainer(pitcher));
                if (hold != null && !WaterHelper.IsEmpty(hold)) Chore.Act("empty", hold, () => WaterHelper.EmptyContainer(hold));
                filtered = !WaterHelper.IsEmpty(c);
            }
            else
                for (int k = 0; k < 6 && !WaterHelper.IsFull(c); k++)
                {
                    int before = WaterHelper.GetCurrentCapacityML(c);
                    Chore.Act("tap", c, () => c.onThisTargetItemFunc?.Invoke(c, faucet));
                    if (WaterHelper.GetCurrentCapacityML(c) <= before) break;
                }
            if (WaterHelper.IsEmpty(c)) return;
            // The faucet's fill is labelled with the tap's purity (filtered or not): today's truth before the filter.
            batches[c.uniqueId] = new Batch { Day = ep.Day, From = Label(c), Filtered = filtered, Twice = twice };
            Vouch(c);
            ReedFills++;
            if (filtered) ReedFiltered++;
        }

        // Without a tap: the poorest owned water first (Rustwater up; the reed takes what the bays could not use), the
        // batch as slow as its worst label.
        static void RefillBought(IExecutorHost ep, GameItem c)
        {
            if (!WaterHelper.IsEmpty(c)) Chore.Act("empty", c, () => WaterHelper.EmptyContainer(c));
            Forget(c);
            int from = 10000;
            foreach (var src in Owned().Where(i => KnownClean(i, 4) && i != c && !PlantSafe(i) && !cleanStore.Contains(i.uniqueId))
                         .OrderBy(Label).ThenByDescending(WaterHelper.GetCurrentCapacityML).ThenBy(i => i.uniqueId).ToList())
            {
                if (WaterHelper.IsFull(c)) break;
                from = Math.Min(from, Label(src));
                Chore.Act("pour", c, () => WaterHelper.TransferLiquid(src, c));
            }
            if (WaterHelper.GetCurrentCapacityML(c) < 1000) return;
            batches[c.uniqueId] = new Batch { Day = ep.Day, From = from, Filtered = false };
            ReedFills++;
        }

        // `tank_l` (20): scrap metal used on the empty reed tank, +2 L each, up to this many litres (50 at most): the reed
        // cures any volume in the same nights, so a bigger tank is more water a batch for the same filters a litre.
        static void GrowTank(GameItem tank)
        {
            int want = Math.Min(50, CleanPlan?.Value<int?>("tank_l") ?? 20) * 1000;
            while (WaterTankHelper.CanUpgrade(tank) && WaterTankHelper.GetCapacityMl(tank) + 2000 <= want)
            {
                var scrap = Owned().Where(i => i.identifier == "scrap_metal" && Parent(i) == null).OrderBy(i => i.uniqueId).FirstOrDefault();
                if (scrap == null) break;
                int before = WaterTankHelper.GetCapacityMl(tank);
                Chore.Act("tank_upgrade", tank, () => WaterTankHelper.Upgrade(tank, scrap));
                if (WaterTankHelper.GetCapacityMl(tank) <= before) break;
                TankGrowths++;
            }
        }

        // Filtered reed water tested HQ or better, wherever it sits (a reed slot's or a store).
        static List<GameItem> HqTanks(GameItem except) => Owned().Where(i => reedHq.Contains(i.uniqueId) && i != except && !WaterHelper.IsEmpty(i) && LabelGrade(i) <= 1)
            .OrderByDescending(WaterHelper.GetCurrentCapacityML).ThenBy(i => i.uniqueId).ToList();

        // Cured water the bays draw on: the stores and the reed slots' vessels, fullest first.
        static List<GameItem> Tanks(GameItem except) => Owned().Where(i => PlantSafe(i) && i != except && !WaterHelper.IsEmpty(i)
                && (Parent(i) == null || IsReed(MachineHydroponic.GetInputSlot(Parent(i))?.childItem)))
            .OrderByDescending(WaterHelper.GetCurrentCapacityML).ThenBy(i => i.uniqueId).ToList();

        // Plant-safe water for any engine's bays (docs/research/QUALITY.md, "Clean water supply"): loose owned
        // containers a reed has cured, largest first; tested, so the label is the truth (from raw tap water it reads
        // Basewater or Ghostwater, the tap's minerals counting against it).
        public static List<GameItem> PlantSafeWater() => Owned().Where(i => PlantSafe(i) && Parent(i) == null && !WaterHelper.IsEmpty(i))
            .OrderByDescending(WaterHelper.GetCurrentCapacityML).ThenBy(i => i.uniqueId).ToList();

        // The reeds on a water route with no cellar: planted, powered and tended like the cellar's bays.
        static void TendReedsAlone(IExecutorHost ep)
        {
            if (!ReedOn || WineOn) return;
            TendPower(ep, true);
            foreach (var bay in Bays())
                try { TendBay(ep, bay); }
                catch (Exception e) { ep.ProductionError(e); }
            TendPower(ep, false);
        }

        // The fill kit off the back counter: left there it goes with the next buyer's sale (a full stock grid refuses
        // it, so the display case takes it, where no window buyer is shown a reserved item).
        static void StowCellarKit()
        {
            if (!ReedOn) return;
            var e = EmporiumEntry.Instance;
            foreach (var i in e.backInvinvElement.childItems.Where(i => i != null && GeneralHelper.IsItemOwned(i) && Reserved(i)
                         && (i.identifier == "water_pitcher" || i.identifier == "water_filter" || i.identifier == "water_test_container" || i.identifier == "water_test_strip"
                             || i.identifier == "water_tank" || i.identifier == "water_jug" || i.identifier == ReedSeedId || i.identifier == "recharger" || i.identifier == "uv_filter")).ToList())
                if (Chore.Accept(e.invElement, i, -1) != 0) Chore.Accept(e.showcaseElement, i, -1);
        }

        // ---- the water stockpiler: HQ water in the display case -------------------------------------------------
        // A display case whose WATER items are worth 350 or more (or are mostly Pure) brings the water stockpiler as that
        // type's buyer every day from day 14, with 500-599 + the day to spend (ShowcaseHelper.CreateClientByStockType,
        // StoreClientListTier.CreateWaterStockPiler). `showcase` (true while the shop sells water): the smallest tested
        // High Quality containers whose value reaches 350, released for sale while they are shown.
        static readonly HashSet<int> shownWater = new HashSet<int>();

        public static List<GameItem> WindowFirst(IEnumerable<GameItem> owned)
        {
            shownWater.Clear();
            if (!ReedOn || !WaterSellOn || !(CleanPlan?.Value<bool?>("showcase") ?? true) || (PlayerDay.Current?.Day ?? 0) < 14) return new List<GameItem>();
            var pick = new List<GameItem>();
            long sum = 0;
            foreach (var c in owned.Where(i => i.IsTag("LIQUID_CONTAINER_TAG") && i.IsGameItemType("WATER") && Parent(i) == null && !WaterHelper.IsEmpty(i)
                             && Vouched(i) && LabelGrade(i) <= 1 && !HeldForWine(i))
                         .OrderBy(i => i.GetCurrentValue(false, true, true, false, 0L)).ThenBy(i => i.uniqueId))
            {
                if (sum >= 350 || pick.Count >= 3) break;
                pick.Add(c);
                sum += c.GetCurrentValue(false, true, true, false, 0L);
            }
            if (sum < 350) return new List<GameItem>();
            foreach (var c in pick) shownWater.Add(c.uniqueId);
            return pick;
        }

        // ---- power: the bays' cells recharged, not bought (docs/research/PRODUCTION-VERDICTS.md §2) ----------------
        // A watered bay draws 8 a night and sellers offer about one cell (100) every two days, so once the reeds keep five
        // bays wet the cells run out. Dr. Jackson's recharger (two visits in three) charges the 8 cells in it +3 a night
        // (+6 with COMMERCIAL_POWER) while the store has power; POWER_HIJACK keeps the power on without the service fee.
        // `power` (false) runs it once two bays grow.

        static bool PowerKit => CleanOn && (CleanPlan.Value<bool?>("power") ?? false);
        static GameItem Recharger() => Owned().FirstOrDefault(i => i.identifier == "recharger" && Parent(i) == null);
        static bool PowerWanted() => PowerKit && (PlayerDay.Current?.Day ?? 0) >= CleanFromDay && Bays().Count(b => MachineHydroponic.GetInputSlot(b).childItem != null) >= 2;

        public static IEnumerable<string> PowerUpgrades()
        {
            if (!PowerWanted() || Recharger() == null) yield break;
            yield return "POWER_HIJACK";
            if (Bays().Count >= 4 && (CleanPlan.Value<bool?>("commercial_power") ?? false)) yield return "COMMERCIAL_POWER";
        }

        static int CellsOwned() => Owned().Count(i => i.identifier == "energy_credit");

        // Cells: one in each bay and four charging.
        static int PowerCellWants() => Recharger() == null ? -1 : Math.Max(0, Bays().Count + 4 - CellsOwned());

        // Before the bays: the recharger's fullest cells out for any bay that will need one. After: every loose cell short of
        // full back into it.
        static void TendPower(IExecutorHost ep, bool before)
        {
            var r = Recharger();
            if (!PowerKit || r == null) return;
            var stock = EmporiumEntry.Instance.invElement;
            if (before)
            {
                int low = Bays().Count(b => Charge(b) < 16);
                int loose = Owned().Count(i => Charged(i) && Parent(i) == null);
                foreach (var cell in GraphUtils.FindAllChildrenType<GameItem>(r).Where(i => i.identifier == "energy_credit" && PowerHelper.GetAvailableEnergyFromItem(i, false) >= 40)
                             .OrderByDescending(i => PowerHelper.GetAvailableEnergyFromItem(i, false)).ThenBy(i => i.uniqueId).Take(Math.Max(0, low - loose)).ToList())
                    Chore.Accept(stock, cell, -1);
                return;
            }
            var grid = GraphUtils.FindChildrenType<GameGridInventory>(r).FirstOrDefault();
            if (grid == null) return;
            // A full cell makes room for an empty one.
            var spare = Owned().Where(i => i.identifier == "energy_credit" && Parent(i) == null && PowerHelper.GetAvailableEnergyFromItem(i, false) < 100)
                .OrderBy(i => PowerHelper.GetAvailableEnergyFromItem(i, false)).ThenBy(i => i.uniqueId).ToList();
            foreach (var full in GraphUtils.FindAllChildrenType<GameItem>(r).Where(i => PowerHelper.GetAvailableEnergyFromItem(i, false) >= 100).Take(spare.Count).ToList())
                Chore.Accept(stock, full, -1);
            foreach (var cell in spare)
                if (Chore.Accept(grid, cell, -1) != 0) break;
        }

        static JObject CellarStatus() => !ReedOn ? null : new JObject
        {
            ["reeds"] = ReedBays().Count, ["reed_seeds"] = Owned().Count(i => i.identifier == ReedSeedId && Parent(i) == null), ["fills"] = ReedFills, ["filtered_fills"] = ReedFiltered, ["store_ml"] = StoreRoomMl(),
            ["cured_ml_in_hand"] = Owned().Where(i => cleanStore.Contains(i.uniqueId)).Sum(WaterHelper.GetCurrentCapacityML),
            ["pitcher"] = Pitcher() != null, ["filters"] = Owned().Count(i => i.identifier == "water_filter" && DurabilityHelper.GetRealDurability(i) > 0),
            ["strips"] = Strips().Count(), ["recharger"] = Recharger() != null, ["cells"] = CellsOwned(),
        };
    }
}
