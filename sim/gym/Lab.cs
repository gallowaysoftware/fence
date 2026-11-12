using System;
using System.Collections.Generic;
using System.Linq;
using Newtonsoft.Json.Linq;

namespace Fence.Gym
{
    // The chemistry engine (`lab: {...}`, docs/research/CHEMISTRY-ENGINE.md; the rules in docs/re/CHEMISTRY.md): buy the
    // Retired Chemist and run one batch at a time through his machines, a step each morning while the day's seconds
    // run the reactions, at the highest grade the kit allows. `lab: "caffeine"` is the older spelling of
    // `lab: {products: ["caffeine_pill"]}` and keeps its old rules. (`chem: "phone"` is the street-dust courier, Suppliers.cs.)
    //
    // It plays on what a player sees: a container's composition only through the composition spectrometer (owned with
    // the Retired Chemist; without it the doses come from the recipe's nominal amounts), temperature only on the plate's
    // readout (whole degrees), the finisher's status line (ready / wet / not enough), a unit's grade label, and the
    // chemistry a knowledgeable player knows (yields, windows, ratios). The grade histogram reads each unit's label.
    internal static class Lab
    {
        const string Caffeine = "caffeine_pill", Dust = "dream_dust", Phanta = "fanta_pill";

        sealed class Recipe
        {
            public string Id, Active, Input;
            public int DrawUl, Base;
            public double Markup, UnitsPerInput, Residue, ResidueKotton;
        }

        // Units per input at 0 blender loss, and residue purities through kotton and the water filter / kotton alone,
        // from the lab (sim/lab/ChemExps.cs): 25 pills from 3 beans at 99.1 %, 33 dusts from 6 caps at 99.2 %, 39 pills
        // from one precursor bottle at 61.6 %.
        static readonly Recipe[] Recipes =
        {
            new Recipe { Id = Caffeine, Active = "caffeine", Input = "coffee_bean", DrawUl = 5000, Base = 10, Markup = 0, UnitsPerInput = 8.3, Residue = 0.993, ResidueKotton = 0.83 },
            new Recipe { Id = Dust, Active = "dream_cap_extract", Input = "dream_cap", DrawUl = 2000, Base = 18, Markup = 0.70, UnitsPerInput = 5.5, Residue = 0.992, ResidueKotton = 0.83 },
            new Recipe { Id = Phanta, Active = "fanta", Input = Precursor, DrawUl = 500, Base = 12, Markup = 1.00, UnitsPerInput = 39, Residue = 0.62, ResidueKotton = 0.60 },
        };

        static Recipe R(string id) => Recipes.First(r => r.Id == id);

        // ---- directives ----

        static JObject Cfg => Directives.Current.Raw["lab"] as JObject;

        static List<string> Products
        {
            get
            {
                if (Cfg?["products"] is JArray a) return a.Select(x => (string)x).Where(x => Recipes.Any(r => r.Id == x)).ToList();
                return Directives.Current.Raw.Value<string>("lab") == "caffeine" ? new List<string> { Caffeine } : new List<string>();
            }
        }

        public static bool On => Products.Count > 0;
        static bool Makes(string id) => Products.Contains(id);
        static bool Legacy => Cfg == null;

        static int MaxInputs => Cfg?.Value<int?>("inputs") ?? Directives.Current.Raw.Value<int?>("lab_beans") ?? 12;
        static int Reserve => Cfg?.Value<int?>("reserve") ?? Directives.Current.Raw.Value<int?>("lab_reserve") ?? 500;
        static bool AfterPermit => Cfg?.Value<bool?>("after_permit") ?? Directives.Current.Raw.Value<bool?>("lab_after_permit") ?? true;
        // "auto" buys the next chem module when its grade or yield gain pays it back within `payback_weeks`; "none"
        // never; "max" up to four in the finisher and four in the blender.
        static string ModulePolicy => Legacy ? "max" : Cfg.Value<string>("modules") ?? "auto";
        static double PaybackWeeks => Cfg?.Value<double?>("payback_weeks") ?? 4;
        // Alcohol : dream mush. 5 is the most extract per cap; lower spends fewer evaporator nights per unit.
        // Under the cartel the evaporator is the bottleneck (one vessel, 300 ml a night), so 3: 89 % of the extract for
        // 60 % of the solvent.
        static double DustRatio => Math.Max(1, Math.Min(5, Cfg?.Value<double?>("ratio") ?? (Cartel ? 3 : 5)));
        // Mornings a filtered batch waits for a water filter before it is poured through kotton alone.
        static int WaitFilterDays => Cfg?.Value<int?>("wait_filter_days") ?? (Legacy ? 0 : 7);
        // The final purity a Phanta pill is cut to when it would come out at 75 % or more (toxic).
        static double PhantaTarget => Cfg?.Value<double?>("phanta_target") ?? 0.66;
        // Our own contraband units (dust, Phanta) are staged for buyers without the plan's `sell.sell_contraband`.
        static bool SellOwn => Cfg?.Value<bool?>("sell") ?? true;

        // Set by a driver that calls Second() through the morning and EndMorning() after it (the gym); without it a plate
        // that cannot hold a window (< 2 modules) is not used for dream extraction or Phanta.
        public static bool Thermostat;

        const string Pack = "common_chemical", Restricted = "restricted_chemical", Precursor = "med_bottle_violet", Module = "chem_module",
            Pitcher = "water_pitcher", Scanner = "chem_scanner";
        static readonly string[] Machines = { "blender", "heating_plate", "evaporator", "chem_finisher", Scanner };
        static readonly string[] Glass = { "100ml_cylinder", "50ml_cylinder", "25ml_cylinder", "5ml_dropper", "1ml_dropper" };
        static readonly string[] Vessels = { Pitcher, "large_bottled_water", "water_jug", "bottled_water" };
        static readonly string[] Reagents = { "alcohol", "acid", "base", "lipobond_agent", "reducing_agent" };

        enum Step { Idle, Mixed, Heating, Dried, Drying }

        static Step step;
        static string product, lastProduct;
        static int vessel, filterPitcher, batchInputs, batchDay, filterWait, thermoSeconds;
        static double batchExpect;
        static string batchFilter;
        // Nominal amounts (µL) of what went into the batch: the doses when no spectrometer is owned.
        static readonly Dictionary<string, int> nominal = new Dictionary<string, int>(StringComparer.Ordinal);

        public static int Batches, Pills, BeansUsed, PacksOpened, ModulesFitted, Stalls;
        public static long PillValue, PillSales, PillsSold;
        static readonly Dictionary<string, int> stalls = new Dictionary<string, int>(StringComparer.Ordinal);
        static readonly HashSet<int> made = new HashSet<int>();
        static readonly Dictionary<string, JObject> per = new Dictionary<string, JObject>(StringComparer.Ordinal);
        static JObject modulePlan;

        public static void Reset()
        {
            step = Step.Idle;
            product = lastProduct = null;
            vessel = filterPitcher = batchInputs = batchDay = filterWait = thermoSeconds = 0;
            batchExpect = 0;
            nominal.Clear();
            Batches = Pills = BeansUsed = PacksOpened = ModulesFitted = Stalls = QuotaStaged = QuotaVisits = 0;
            PillValue = PillSales = PillsSold = CutUl = 0;
            stalls.Clear();
            made.Clear();
            per.Clear();
            modulePlan = null;
        }

        static bool Up => NetworkUpgrade.GetUpgradeById("RETIRED_CHEMIST")?.IsUnlocked() ?? false;

        public static IEnumerable<string> Upgrades()
        {
            if (WantsUpgrade("RETIRED_CHEMIST")) yield return "RETIRED_CHEMIST";
        }

        // `after_permit` (default true): a shop that means to buy the permit buys it first; the 1 500 spent before it
        // starves the permit official's bribes, and OPERATING_WITHOUT_LICENSE piles up to an arrest.
        public static bool WantsUpgrade(string id) => On && id == "RETIRED_CHEMIST"
            && (PlayerStore.Instance.hasBusinessPermit || Directives.Current.Raw.Value<string>("permit") != "buy" || !AfterPermit);

        public static int UpgradeReserve => Reserve;

        static IEnumerable<GameItem> Owned() => EmporiumEntry.Instance.GetAllOwnedItems(false, true).Where(i => i != null);
        static GameItem Parent(GameItem i) => i.parentInventory?.GetParentItem();
        static GameItem Find(string id) => Owned().Where(i => i.identifier == id).OrderBy(i => i.uniqueId).FirstOrDefault();
        static GameItem ById(int uid) => uid == 0 ? null : Owned().FirstOrDefault(i => i.uniqueId == uid);
        static bool Loose(GameItem i) => Parent(i) == null;

        static int Total(GameItem c) => WaterHelper.GetTotalVolume(c);
        static int Capacity(GameItem c) => c.GetTagReadonly("LIQUID_CONTAINER_CAPACITY").GetInt();
        static int Tag(GameItem c, string liquid) => c.GetTagReadonly("CURRENT_PART_" + liquid.ToUpperInvariant()).GetInt();

        static bool Scans => Owned().Any(i => i.identifier == Scanner);

        // What the player knows of a liquid in the work vessel: the spectrometer's reading, or the recipe's nominal amount.
        static int Part(GameItem c, string liquid)
        {
            if (Scans) return Tag(c, liquid);
            return c != null && c.uniqueId == vessel && nominal.TryGetValue(liquid, out int n) ? n : 0;
        }

        static void Nominal(string liquid, int delta) => nominal[liquid] = Math.Max(0, (nominal.TryGetValue(liquid, out int n) ? n : 0) + delta);

        // A supply bottle: a loose container that is mostly one reagent (the bottle's label says which). Its share is the
        // spectrometer's reading, or the supply bottles' typical 70 % (mini alcohol), 48 % (precursor), 95 % (the rest).
        static IEnumerable<GameItem> Bottles(string liquid) => Owned().Where(i => Loose(i) && i.IsTag("LIQUID_CONTAINER_TAG") && i.identifier != "wine_bottle"
            && !Vessels.Contains(i.identifier) && !Glass.Contains(i.identifier) && Total(i) > 0 && Tag(i, liquid) >= Total(i) * (liquid == "alcohol" ? 0.6 : 0.4)).OrderBy(i => i.uniqueId);

        static double Share(GameItem b, string liquid)
        {
            int t = Total(b);
            if (t <= 0) return 0;
            if (Scans) return Tag(b, liquid) / (double)t;
            return liquid == "alcohol" ? 0.7 : liquid == "methylol" || liquid == "stabilizing_agent" ? 0.48 : 0.95;
        }

        static int Have(string liquid) => (int)Bottles(liquid).Sum(b => Total(b) * Share(b, liquid));

        static bool Mine(GameItem i) => i.uniqueId == vessel || i.uniqueId == filterPitcher;

        // The lab's own vessels stay out of the water engine.
        public static bool Holds(GameItem i) => On && Mine(i);

        static bool IsReagentBottle(GameItem i) => i.IsTag("LIQUID_CONTAINER_TAG") && Loose(i) && Total(i) > 0 && i.identifier != "wine_bottle"
            && (Reagents.Any(l => Tag(i, l) >= Total(i) * (l == "alcohol" ? 0.6 : 0.85)) || (Makes(Phanta) && Tag(i, "methylol") > 0));

        static bool IsInputGood(GameItem i) => Recipes.Any(r => r.Input == i.identifier && r.Id != Phanta);

        public static bool Reserved(GameItem i)
        {
            if (!On) return false;
            string id = i.identifier ?? "";
            if (Mine(i) || Machines.Contains(id) || Glass.Contains(id) || id == Pack || id == Module || (Makes(Phanta) && id == Restricted) || QuotaHeld(i)) return true;
            if (Recipes.Any(r => Makes(r.Id) && r.Input == id && r.Id != Phanta)) return Owned().Count(x => x.identifier == id && Loose(x) && x.uniqueId <= i.uniqueId) <= (Up ? MaxInputs : 2);
            if (id == Pitcher) return Owned().Count(x => x.identifier == Pitcher) <= 2;
            if (!Legacy && Cell(i) && Loose(i)) return Owned().Count(x => Cell(x) && Loose(x) && x.uniqueId <= i.uniqueId) <= 2;
            if (id == "water_filter" || id == "kotton_fabric") return Owned().Count(x => x.identifier == id && Loose(x)) <= 2;
            return IsReagentBottle(i);
        }

        static readonly string[] Kit = { Pack, Module, Pitcher, "5ml_dropper", "25ml_cylinder", "1ml_dropper", "water_filter", "kotton_fabric", "energy_credit", "energy_credit_breeder", "energy_credit_dis", "blender", "heating_plate", "evaporator", "chem_finisher", Scanner };

        public static bool IsInput(string id) => On && id != null && (Kit.Contains(id) || Recipes.Any(r => Makes(r.Id) && r.Input == id) || (Makes(Phanta) && id == Restricted));

        // A contraband input the plan makes something from (dream caps, restricted supplies) is bought as an input.
        public static bool ContrabandInput(string id) => On && id != null && ((Makes(Dust) && id == "dream_cap") || (Makes(Phanta) && id == Restricted));

        // Our own pressed units go to buyers even when the plan does not sell contraband in general.
        public static bool SellsOwn(GameItem i) => On && SellOwn && i != null && made.Contains(i.uniqueId);

        // ---- the Chemist start's cartel ----

        static bool Cartel => On && NewGameData.Instance != null && NewGameData.Instance.startType == NewGameData.StartType.DRUG_DEALER && !PlayerStore.Instance.isCartelExited;
        static int QuotaCount => PlayerStore.Instance.cartelRequiredCount;
        static int QuotaPurity => PlayerStore.Instance.cartelRequiredPurity;
        // Dust kept for the weekly count: the handler's number (he says it, and the next one, at each visit) and
        // `lab.quota_extra` (default 10) more, each +1 cartel rep; the rest sells.
        static int QuotaKeep => QuotaCount + (Cfg?.Value<int?>("quota_extra") ?? 10);

        static List<GameItem> DustHeld() => Owned().Where(i => i.identifier == Dust && !StolenHelper.IsStolenItem(i)).OrderBy(i => i.uniqueId).ToList();

        static bool QuotaHeld(GameItem i) => Cartel && i.identifier == Dust && DustHeld().TakeWhile(x => x.uniqueId != i.uniqueId).Count() < QuotaKeep;

        // The check-in's materials (3 caps and a supply pack for 12 cartel rep, 6 with Reliable Chemist) when the dust
        // held and the caps at hand would not make the next count.
        public static bool CartelMaterials()
        {
            if (!Cartel) return false;
            int caps = Owned().Count(i => i.identifier == "dream_cap" && Loose(i));
            return DustHeld().Count + caps * 3 < QuotaCount;
        }

        // Every dust held onto the back counter: the handler takes all of it and judges count and mean purity.
        public static void StageQuota()
        {
            if (!Cartel) return;
            var back = EmporiumEntry.Instance.backInvinvElement;
            int n = 0;
            // The count exactly (more is +1 rep a unit, but the next week's visit with none is the end of the run).
            foreach (var d in DustHeld().Where(i => !ReferenceEquals(i.parentInventory, back)).OrderBy(ChemicalProductHelper.GetPurity).Take(Math.Max(1, QuotaCount)))
                if (Chore.Accept(back, d, -1) == 0) n++;
            QuotaStaged += n;
            QuotaVisits++;
        }

        public static int QuotaStaged, QuotaVisits;

        // `lab.on_shelf` (default 6) of our own contraband units of each kind stay out for buyers; the rest go to the
        // smuggler's bays with the conceal policy (an inspection searches machine grids, so the finisher is no hiding place).
        public static bool Shelf(GameItem i)
        {
            if (!SellsOwn(i) || ContrabandHelper.GetContrabandLevelInInt(i) == 0 || QuotaHeld(i)) return false;
            int k = Cfg?.Value<int?>("on_shelf") ?? 6;
            return made.Count(u => u < i.uniqueId && ById(u)?.identifier == i.identifier) < k;
        }

        // The kit and the lab's goods off the counters (a buyer's "sell all" takes what is left there): into stock, or
        // the display case when stock is full (no window buyer wants a machine).
        public static void Stow()
        {
            if (!On || !Up) return;
            var e = EmporiumEntry.Instance;
            foreach (var i in new[] { e.backInvinvElement, e.frontInvinvElement }.SelectMany(inv => inv.childItems).Where(i => i != null && GeneralHelper.IsItemOwned(i) && (Reserved(i) && (!IsInputGood(i) || !Legacy) || (!Legacy && (Recipes.Any(r => Makes(r.Id) && r.Input == i.identifier) || i.identifier == Module || i.identifier == Pack)))).ToList())
                if (Chore.Accept(e.invElement, i, -1) != 0) Chore.Accept(e.showcaseElement, i, -1);
        }

        // The blender, plate and finisher keep their chem modules in storage 0 (MachineHelper.GetModuleInv assumes a battery there).
        static GameInventory Modules(GameItem m) => Slot(m, 0) as GameInventory;

        static int Fitted(string machine) { var m = Find(machine); return m == null ? 0 : Modules(m)?.childItems.Count(x => x != null && x.identifier == Module) ?? 0; }

        // ---- what the kit can make (player knowledge: docs/re/CHEMISTRY.md §3–4) ----

        static readonly double[] FinisherLoss = { 0.12, 0.06, 0.03, 0.01, 0 };
        static readonly double[] BlenderLoss = { 0.35, 0.20, 0.10, 0.05, 0 };
        static readonly double[] GradeFloor = { 0.99, 0.90, 0.80, 0.60, 0.45, 0.30, 0 };
        static readonly string[] GradeName = { "lab_pure", "pharma_grade", "near_pharma", "street_standard", "cut_product", "junk_grade", "starch_grade" };
        static readonly double[] GradeMod = { 1.35, 0.85, 0.40, 0, -0.45, -0.75, -0.95 };

        static int Band(double purity)
        {
            for (int k = 0; k < GradeFloor.Length; k++)
                if (purity >= GradeFloor[k] - 1e-9) return k;
            return GradeFloor.Length - 1;
        }

        static int Clamp4(int n) => Math.Max(0, Math.Min(4, n));

        // The purity a unit comes out at, and the units per input, for this kit.
        static (double purity, double units) Expect(Recipe r, int finisher, int blender, bool waterFilter)
        {
            double residue = waterFilter ? r.Residue : r.ResidueKotton;
            double purity = residue * (1 - FinisherLoss[Clamp4(finisher)]);
            double units = r.UnitsPerInput * (r.Id == Phanta ? 1 : 1 - BlenderLoss[Clamp4(blender)]);
            if (r.Id == Dust) units *= DustYield(DustRatio);
            if (r.Id == Phanta)
            {
                // Toxic from 75 %: cut down to the target, which stretches the active into more pills.
                if (purity >= 0.745) { units *= purity / PhantaTarget; purity = PhantaTarget; }
                else units *= r.Residue / Math.Max(0.01, residue);
            }
            return (purity, units);
        }

        // Extract per cap by alcohol : mush ratio, relative to ratio 5 (lab: 1…5 → 25.3 / 37.4 / 44.5 / 48.4 / 50 % of the mush).
        static double DustYield(double ratio)
        {
            double[] at = { 0.506, 0.748, 0.89, 0.968, 1.0 };
            int lo = Math.Max(1, Math.Min(5, (int)Math.Floor(ratio)));
            int hi = Math.Min(5, lo + 1);
            return at[lo - 1] + (at[hi - 1] - at[lo - 1]) * (ratio - lo);
        }

        static double Price(Recipe r, double purity) => r.Base * (1 + GradeMod[Band(purity)]) * (1 + r.Markup);

        static double ValuePerInput(Recipe r, int finisher, int blender, bool waterFilter)
        {
            var (p, u) = Expect(r, finisher, blender, waterFilter);
            // Narcotic buyers refuse junk and starch.
            if (r.Markup > 0 && Band(p) >= 5) return 0;
            return u * Price(r, p);
        }

        // Inputs a week each product has used since its first batch (4 before a week of history).
        static double Rate(Recipe r)
        {
            var o = per.TryGetValue(r.Id, out var x) ? x : null;
            int day = PlayerDay.Current?.Day ?? 0;
            int first = o?.Value<int?>("first_day") ?? day;
            int inputs = o?.Value<int?>("inputs") ?? 0;
            if (inputs == 0 || day - first < 7) return 4;
            return inputs * 7.0 / Math.Max(7, day - first);
        }

        // After the finisher's first, one plate module when the lab makes caffeine: a plate at 160 °C dries the filtered
        // batch overnight (water boils at 100; caffeine never degrades), so the evaporator and its cells drop out.
        // Dream extract dies from 60 °C and needs the evaporator either way.
        static bool PlateFirst => Makes(Caffeine) && Fitted("chem_finisher") >= 1 && Fitted("heating_plate") == 0 && Find("heating_plate") != null;

        // The next chem module worth buying: the finisher's purity cap or the blender's yield, whichever adds more a
        // week across the products made. Finisher steps that cross no grade band are weighed together with the step
        // that completes one (two to four modules for lab pure), per module.
        static (string machine, double perWeek, int modules) NextModule()
        {
            int f = Fitted("chem_finisher"), b = Fitted("blender");
            bool wf = Owned().Any(i => i.identifier == "water_filter");
            double Week(int ff, int bb) => Recipes.Where(r => Makes(r.Id)).Sum(r => Rate(r) * ValuePerInput(r, ff, bb, wf));
            double now = Week(f, b);
            string machine = null;
            double best = 0;
            int bundle = 0;
            for (int k = 1; f + k <= 4; k++)
            {
                double g = (Week(f + k, b) - now) / k;
                if (g > best + 1e-6) { machine = "chem_finisher"; best = g; bundle = k; }
            }
            if (b < 4)
            {
                double g = Week(f, b + 1) - now;
                if (g > best + 1e-6) { machine = "blender"; best = g; bundle = 1; }
            }
            return (machine, best, bundle);
        }

        // Chem modules: the retired chemist's weekly counter (one, then 50/50/25 %, last on a list that fills his counter)
        // at +45 % on hard, and Dr. Jackson (with the Retired Chemist 50 % and 25 % a visit) at +50 %.
        static int ModulePrice()
        {
            string d = PlayerStore.Instance.difficultyId;
            return d == "hard" || d == "challenge" ? 375 : 250;
        }

        // Dr. Jackson's goods the lab buys at his shop or his visits: chem modules, and a missing scanner or filter.
        public static bool JacksonBuys(string id) => On && !Legacy && (id == Module || id == Scanner || id == "water_filter" || id == "kotton_fabric" || id == "energy_credit");

        // `lab.jackson_need` (default 7; 0 never): nights between after-hours trips to Dr. Jackson while the plan wants
        // a module.
        public static int? JacksonNeed => On && !Legacy && (Cfg.Value<int?>("jackson_need") ?? 7) is int n && n > 0 ? n : (int?)null;

        static bool ModulePays()
        {
            var (machine, perWeek, k) = NextModule();
            int price = ModulePrice();
            modulePlan = new JObject
            {
                ["day"] = PlayerDay.Current?.Day, ["next"] = machine, ["gain_per_week"] = Math.Round(perWeek, 1), ["price"] = price,
                ["payback_weeks"] = perWeek > 0 ? Math.Round(price / perWeek, 1) : (double?)null, ["bundle"] = k,
            };
            return machine != null && price <= perWeek * PaybackWeeks;
        }

        public static int Wants(string id)
        {
            if (!On || id == null) return 0;
            int cash = (int)PlayerStore.Instance.playerCash, rent = PlayerDay.Current?.RentDue ?? PlayerStore.Instance.GetRentValue();
            switch (id)
            {
                case "coffee_bean":
                case "dream_cap":
                    if (!Makes(id == "coffee_bean" ? Caffeine : Dust)) return 0;
                    return Math.Max(0, (Up ? MaxInputs : 2) - Owned().Count(i => i.identifier == id && Loose(i)));
                case Pack:
                    return Up && Owned().Count(i => i.identifier == Pack) == 0 && (Have("alcohol") < 350000 || !Bottles("acid").Any() || (Makes(Caffeine) && !Bottles("lipobond_agent").Any())) ? 1 : 0;
                case Restricted:
                    return Up && Makes(Phanta) && PhantaPossible() && !Owned().Any(i => i.identifier == Restricted) && !Bottles("methylol").Any() ? 1 : 0;
                case Pitcher: return Owned().Count(i => i.identifier == Pitcher) < 2 ? 1 : 0;
                case "5ml_dropper":
                case "25ml_cylinder":
                case "1ml_dropper":
                    return Up && !Legacy && Find(id) == null ? 1 : 0;
                case "water_filter":
                    if (Legacy) return Up && !Owned().Any(i => i.identifier == "water_filter") ? 1 : 0;
                    return Up && Owned().Count(i => i.identifier == "water_filter") < 2 ? 1 : 0;
                case "kotton_fabric": return Up && !Legacy && !Owned().Any(i => i.identifier == "kotton_fabric") ? 1 : 0;
                case Module:
                    int loose = Owned().Count(i => i.identifier == Module && Loose(i));
                    if (!Up || loose > 0 || cash <= rent + 600) return 0;
                    if (ModulePolicy == "none") return 0;
                    if (ModulePolicy == "max") return Fitted("chem_finisher") + Fitted("blender") < 8 ? 1 : 0;
                    return ModulePays() | PlateFirst ? 1 : 0;
                case "blender":
                case "heating_plate":
                case "evaporator":
                case "chem_finisher":
                    return Up && Find(id) == null && cash > rent + 600 ? 1 : 0;
                case Scanner:
                    return Up && !Legacy && Find(id) == null && cash > rent + 600 ? 1 : 0;
                case "energy_credit":
                case "energy_credit_breeder":
                case "energy_credit_dis":
                    var ev = Find("evaporator");
                    return Up && ev != null && Charge(ev) < 12 && !Owned().Any(i => Cell(i) && Loose(i)) ? 1 : 0;
            }
            return 0;
        }

        // Any energy cell (plain, breeder, discount, hypertech) with a night's draw (6) in it.
        static bool Cell(GameItem i) => i.identifier != null && i.identifier.StartsWith("energy_credit", StringComparison.Ordinal) && PowerHelper.GetAvailableEnergyFromItem(i, false) >= 6;

        static int Charge(GameItem m)
        {
            var b = MachineHelper.GetBatterySlot(m)?.childItem;
            return b == null ? 0 : PowerHelper.GetAvailableEnergyFromItem(b, false);
        }

        static GraphNodeStorage Slot(GameItem m, int i) => (GraphNodeStorage)MachineHelper.GetBodyStorage(m, i);

        static void Stall(string why)
        {
            Stalls++;
            stalls[why] = (stalls.TryGetValue(why, out int n) ? n : 0) + 1;
        }

        static JObject Per(string id)
        {
            if (!per.TryGetValue(id, out var o))
                per[id] = o = new JObject { ["batches"] = 0, ["inputs"] = 0, ["units"] = 0, ["value"] = 0, ["sold"] = 0, ["sales"] = 0, ["grades"] = new JObject(), ["expected"] = new JObject() };
            return o;
        }

        static void Add(JObject o, string k, long n) => o[k] = (o.Value<long?>(k) ?? 0) + n;

        // ---- the morning ----

        public static void Morning(IExecutorHost ep)
        {
            if (!On || !Up) return;
            try { Run(ep); }
            catch (Exception e) { ep.ProductionError(e); }
        }

        static void Run(IExecutorHost ep)
        {
            var stock = EmporiumEntry.Instance.invElement;
            // The kit lands on the back counter, where a buyer's "sell all" would take it.
            foreach (var i in Owned().Where(i => Loose(i) && !ReferenceEquals(i.parentInventory, stock) && Reserved(i) && !IsInputGood(i)).ToList())
                Chore.Accept(stock, i, -1);
            FitModules();
            var blender = Find("blender");
            var plate = Find("heating_plate");
            var evap = Find("evaporator");
            var finisher = Find("chem_finisher");
            if (blender == null || plate == null || evap == null || finisher == null) { Stall("machines:" + string.Join("+", new[] { ("blender", blender), ("heating_plate", plate), ("evaporator", evap), ("chem_finisher", finisher) }.Where(x => x.Item2 == null).Select(x => x.Item1))); return; }
            // Live, machine windows' buttons are not pressed (LiveHost.MachineButton): the lab does nothing rather than
            // load a blender it cannot start (docs/research/CHEMISTRY-ENGINE.md, "Live").
            if (!Legacy && ep.MachineButton(blender, LocHelper.GetLocalizedMechanic("mech_blender_btn_start")) == null) { Stall("no_buttons"); return; }
            if (ById(vessel) == null) { step = Step.Idle; product = null; }
            switch (step)
            {
                case Step.Mixed: Heat(ep, plate, evap); return;
                case Step.Heating: Filter(ep, plate, evap); return;
                case Step.Dried:
                case Step.Drying:
                    if (!Finish(ep, plate, evap, finisher)) return;
                    break;
            }
            Start(ep, blender, plate);
        }

        // The finisher's first module (its impurity caps every product), then where the plan's next module goes.
        static void FitModules()
        {
            foreach (var mod in Owned().Where(i => i.identifier == Module && Loose(i)).ToList())
            {
                int f = Fitted("chem_finisher");
                string to = f < 4 && (Legacy || f == 0) ? "chem_finisher" : Legacy ? "blender" : PlateFirst ? "heating_plate" : NextModule().machine ?? (f < 4 ? "chem_finisher" : "blender");
                var m = Find(to);
                if (m == null || Fitted(to) >= 4) continue;
                if (Chore.Accept(Modules(m), mod, -1) == 0) ModulesFitted++;
            }
        }

        // The filter pitcher, and the work vessel: the largest other empty pitcher, jug or water bottle.
        static bool PickVessels()
        {
            var cands = Owned().Where(i => Vessels.Contains(i.identifier) && (Loose(i) || Mine(i)) && !Production.HeldForWine(i)).OrderBy(i => i.uniqueId).ToList();
            if (ById(filterPitcher) == null) filterPitcher = cands.FirstOrDefault(i => i.identifier == Pitcher && i.uniqueId != vessel)?.uniqueId ?? 0;
            if (ById(vessel) == null) vessel = cands.Where(i => i.uniqueId != filterPitcher && Total(i) < 20000).OrderByDescending(Capacity).ThenBy(i => i.uniqueId).FirstOrDefault()?.uniqueId ?? 0;
            return vessel != 0 && filterPitcher != 0;
        }

        static int PlateModules(GameItem plate) => Modules(plate)?.childItems.Count(x => x != null && x.identifier == Module) ?? 0;

        // Dream extraction (40–49 °C) and Phanta (120–129 °C) need a plate that holds a window: two modules, or a
        // player at the readout switching it on and off (Thermostat). An unmodded plate never reaches 120 °C.
        static bool CanHold(GameItem plate, string id) => PlateModules(plate) >= 2 || (Thermostat && (id == Dust || PlateModules(plate) >= 1));

        // Without the spectrometer a precursor bottle's impurities (0.4–17 ml) are unknown and a pill may come out toxic.
        static bool PhantaPossible()
        {
            var plate = Find("heating_plate");
            return plate != null && Scans && CanHold(plate, Phanta);
        }

        static void Start(IExecutorHost ep, GameItem blender, GameItem plate)
        {
            if (!PickVessels()) { Stall("vessels"); return; }
            OpenPack(Pack, () => Have("alcohol") >= 150000 && Bottles("acid").Any() && Bottles("base").Any() && Bottles("lipobond_agent").Any());
            if (Makes(Phanta)) OpenPack(Restricted, () => Bottles("methylol").Any() || !PhantaPossible());
            // The cartel's count comes first: nothing else takes the vessel while the dust held is short of it.
            bool quotaFirst = Cartel && Makes(Dust) && DustHeld().Count < QuotaKeep;
            foreach (var id in Products.Where(p => !quotaFirst || p == Dust))
            {
                bool started = id == Phanta ? StartPhanta(ep, plate) : StartBlend(ep, blender, plate, R(id));
                if (started) return;
            }
        }

        // Less than a unit's draw stays in the vessel after pressing; another product's would dilute the next batch.
        static void Rinse(GameItem v, string next)
        {
            if (Total(v) > 0 && lastProduct != null && lastProduct != next) Chore.Act("empty", v, () => WaterHelper.EmptyContainer(v));
        }

        static bool StartBlend(IExecutorHost ep, GameItem blender, GameItem plate, Recipe r)
        {
            var v = ById(vessel);
            var inputs = Owned().Where(i => i.identifier == r.Input && Loose(i) && !Crops.Reserved(i) && !StolenHelper.IsStolenItem(i)).OrderBy(i => i.uniqueId).ToList();
            if (inputs.Count == 0) { Stall(r.Id == Caffeine ? "beans" : "caps"); return false; }
            if (r.Id == Dust && !CanHold(plate, Dust)) { Stall("plate_window"); return false; }
            int alcohol = Have("alcohol");
            if (alcohol < 20000 || !Bottles("acid").Any() || (r.Id == Caffeine && !Bottles("lipobond_agent").Any())) { Stall("reagents"); return false; }
            // Doses are measured in graduated glassware; without any, acid and lipobond would go in by the bottle.
            if (!Legacy && !Glass.Any(g => Find(g) != null)) { Stall("glassware"); return false; }
            Rinse(v, r.Id);
            int loss = MachineBlender.GetLoss(blender);
            double keep = (100 - loss) / 100.0;
            // Per input: 44.8 ml of coffee mush (alcohol 1× it: the full 75 % even as it boils off) or 22 ml of dream
            // mush (alcohol `ratio`× it), less the blender's loss; the batch shrinks to the alcohol at hand and the
            // vessel's room (a mini bottle brings 3/7 of its alcohol again in water).
            double ratio = Legacy ? 0.75 : r.Id == Caffeine ? 1.0 : DustRatio;
            int mushPer = (int)((r.Id == Caffeine ? 44800 : 22000) * keep);
            int perInput = (int)(mushPer * (1 + ratio + ratio * 3 / 7) + (r.Id == Caffeine ? 35000 : 10000));
            int n = Math.Min(inputs.Count, MaxInputs);
            n = Math.Min(n, Math.Max(1, (int)(alcohol / (mushPer * Math.Min(ratio, 1.0)))));
            n = Math.Min(n, Math.Max(1, (Capacity(v) - Total(v)) / Math.Max(1, perInput)));
            var input = Slot(blender, 1);
            var cslot = Slot(blender, 2);
            if (Chore.Accept(cslot, v, -1) != 0) { Stall("blender"); return false; }
            int loaded = 0;
            foreach (var b in inputs.Take(n))
                if (Chore.Accept(input, b, -1) == 0) loaded++;
            var press = ep.MachineButton(blender, LocHelper.GetLocalizedMechanic("mech_blender_btn_start"));
            press?.Invoke();
            string mushId = r.Id == Caffeine ? "coffee_mush" : "dream_cap_mush";
            nominal.Clear();
            // Per input as blended (docs/re/CHEMISTRY.md §7.6): a bean 44.8 ml mush and 8 each of caffeine, fat,
            // protein, physical; a cap 22 mush, 2 protein.
            if (r.Id == Caffeine) { Nominal(mushId, (int)(44800 * keep * loaded)); Nominal("protein", (int)(8000 * keep * loaded)); Nominal("fat", (int)(8000 * keep * loaded)); }
            else { Nominal(mushId, (int)(22000 * keep * loaded)); Nominal("protein", (int)(2000 * keep * loaded)); }
            if (press == null || Tag(v, mushId) == 0) { Stall("blend"); return false; }
            int mush = Part(v, mushId);
            // Acid equal to the protein (1:1 on the net excess) and lipobond an eighth of the fat, both cold: lipobond
            // boils from 55 °C before it binds.
            Dose("acid", v, Part(v, "protein") - Part(v, "acid") + Part(v, "base"));
            React("acid", "protein", 1);
            if (r.Id == Caffeine) { Dose("lipobond_agent", v, Part(v, "fat") / 8 + 500, over: true); React("lipobond_agent", "fat", 8); }
            Dose("alcohol", v, Math.Min(Have("alcohol"), (int)(mush * ratio)), over: true);
            Begin(ep, r, loaded, v);
            if (r.Id == Dust) HoldWindow(plate, v, 44);
            return true;
        }

        static void Begin(IExecutorHost ep, Recipe r, int inputs, GameItem v)
        {
            product = lastProduct = r.Id;
            batchInputs = inputs;
            batchDay = ep.Day;
            filterWait = 0;
            if (r.Id == Caffeine) BeansUsed += inputs;
            var o = Per(r.Id);
            if (o["first_day"] == null) o["first_day"] = ep.Day;
            Add(o, "inputs", inputs);
            step = Step.Mixed;
            var kit = KitNow();
            var (p, _) = Expect(r, kit.f, kit.b, kit.wf);
            batchExpect = p;
            ep.Human("production", null, ep.Uid(v), new JObject { ["action"] = "lab_blend", ["product"] = r.Id, ["inputs"] = inputs, ["expect_purity"] = Math.Round(p * 100, 1), ["expect_grade"] = GradeName[Band(p)] }, "player:lab");
        }

        static (int f, int b, bool wf) KitNow() => (Fitted("chem_finisher"), Fitted("blender"), Owned().Any(i => i.identifier == "water_filter"));

        // One precursor bottle with acid at a quarter of its stabiliser (R12 removes a fifth of the stabiliser a second
        // and uses only 5 % of the acid); the reducing agent and the heat come the next morning.
        static bool StartPhanta(IExecutorHost ep, GameItem plate)
        {
            if (!PhantaPossible()) { Stall("phanta_kit"); return false; }
            var bottle = Bottles("methylol").FirstOrDefault();
            if (bottle == null) { Stall("precursor"); return false; }
            if (!Bottles("acid").Any() || !Bottles("base").Any() || Have("reducing_agent") < 60000) { Stall("reagents"); return false; }
            var v = ById(vessel);
            Rinse(v, Phanta);
            nominal.Clear();
            Chore.Act("pour", bottle, () => WaterHelper.TransferLiquid(bottle, v));
            Dose("acid", v, Part(v, "stabilizing_agent") / 4);
            Begin(ep, R(Phanta), 1, v);
            return true;
        }

        static void OpenPack(string id, Func<bool> enough)
        {
            if (enough()) return;
            var pack = Owned().Where(i => i.identifier == id && !StolenHelper.IsStolenItem(i)).OrderBy(i => i.uniqueId).FirstOrDefault();
            if (pack?.onActivateSlotItemFunc == null) return;
            var stock = EmporiumEntry.Instance.invElement;
            if (!ReferenceEquals(pack.parentInventory, stock)) Chore.Accept(stock, pack, -1);
            Chore.Act("unpack", pack, () => pack.onActivateSlotItemFunc(pack, pack.parentInventory as GameInventory, null));
            PacksOpened++;
        }

        // A measured pour of `ul` of a reagent: the supply bottle's mix through the graduated glassware, largest first,
        // short by less than the smallest glass owned. `over`: the last part may overshoot by up to half that glass
        // (lipobond and alcohol boil off later; an acid or base excess would stay in the residue).
        static int Dose(string liquid, GameItem into, int ul, bool over = false)
        {
            if (ul <= 0) return 0;
            int want = ul;
            var glass = Glass.Select(Find).Where(g => g != null).ToList();
            foreach (var src in Bottles(liquid).ToList())
            {
                double share = Share(src, liquid);
                if (share <= 0) continue;
                int mix = (int)(ul / share);
                for (int gi = 0; gi < glass.Count; gi++)
                {
                    var g = glass[gi];
                    int cap = Capacity(g);
                    bool last = gi == glass.Count - 1;
                    while ((mix >= cap || (over && last && mix * 2 >= cap)) && Total(src) > 0)
                    {
                        if (Total(g) > 0) Chore.Act("pour", g, () => WaterHelper.TransferLiquid(g, into));
                        int before = Total(src);
                        Chore.Act("pour", src, () => WaterHelper.TransferLiquid(src, g));
                        int took = before - Total(src);
                        Chore.Act("pour", g, () => WaterHelper.TransferLiquid(g, into));
                        int moved = (int)(took * share);
                        if (moved <= 0) break;
                        ul -= moved;
                        mix = (int)(ul / share);
                    }
                }
                if (ul < 1000) break;
            }
            int dosed = want - Math.Max(0, ul);
            Nominal(liquid, dosed);
            return dosed;
        }

        // Without the spectrometer the batch's nominal amounts follow the reactions a player knows run to completion
        // on the shelf: acid 1:1 with protein, lipobond 1:8 with fat, acid and base 1:1.
        static void React(string a, string b, int perA)
        {
            int na = nominal.TryGetValue(a, out int x) ? x : 0, nb = nominal.TryGetValue(b, out int y) ? y : 0;
            int used = Math.Min(na, nb / perA);
            Nominal(a, -used);
            Nominal(b, -used * perA);
        }

        // The vessel onto the plate, set to hold `target` (≥ 2 modules: the ± buttons in 5 °C steps), or switched on
        // low for the morning's thermostat.
        static bool HoldWindow(GameItem plate, GameItem v, int target)
        {
            if (Chore.Accept(Slot(plate, 1), v, -1) != 0) { Stall("plate"); return false; }
            if (PlateModules(plate) >= 2) SetTarget(plate, target);
            thermoSeconds = 0;
            int state = PlateModules(plate) >= 2 ? 2 : 1;
            Chore.Act("plate_on", plate, () => plate.ModifyTag("HEATING_PLATE_STATE", s => s.Enable().SetInt(state)));
            return true;
        }

        // The setting nearest `want` from above on the plate's ± buttons (5 °C steps, 26…160 °C).
        static void SetTarget(GameItem plate, int want)
        {
            for (int k = 0; k < 40; k++)
            {
                int t = plate.GetTagReadonly("CURRENT_TARGET_TEMP").GetInt();
                int next = t < want ? Math.Min(t + 5, 160) : t - 5 >= want ? Math.Max(t - 5, 26) : t;
                if (next == t) break;
                Chore.Act(next > t ? "plate_plus" : "plate_minus", plate, () => plate.ModifyTag("CURRENT_TARGET_TEMP", s => s.Enable().SetInt(next)));
            }
        }

        static void PlateOff(GameItem plate)
        {
            if (plate.GetTagReadonly("HEATING_PLATE_STATE").GetInt() != 0)
                Chore.Act("plate_off", plate, () => plate.ModifyTag("HEATING_PLATE_STATE", s => s.Enable().SetInt(0)));
        }

        static bool OnThermostat(out GameItem plate, out GameItem v)
        {
            plate = null;
            v = null;
            if (!On || !Thermostat || product == null) return false;
            if (!(product == Dust && step == Step.Mixed || product == Phanta && step == Step.Heating)) return false;
            plate = Find("heating_plate");
            v = ById(vessel);
            return plate != null && v != null && PlateModules(plate) < 2 && ReferenceEquals((Slot(plate, 1) as GameSlotInventory)?.childItem, v);
        }

        // Once a second through the morning (the gym's driver): at a plate that cannot hold the window, the player
        // watches the readout (whole degrees) and switches it between low and off. After `thermo_s` seconds (default
        // 150; extraction finishes within about a minute at temperature) it stays off.
        public static void Second()
        {
            if (!OnThermostat(out var plate, out var v)) return;
            int state = plate.GetTagReadonly("HEATING_PLATE_STATE").GetInt();
            if (++thermoSeconds > (Cfg?.Value<int?>("thermo_s") ?? 150)) { PlateOff(plate); return; }
            int t = Readout(v);
            int on = product == Dust ? 42 : 121, off = product == Dust ? (PlateModules(plate) == 0 ? 46 : 45) : 124;
            if (state != 0 && t >= off) PlateOff(plate);
            else if (state == 0 && t <= on) Chore.Act("plate_on", plate, () => plate.ModifyTag("HEATING_PLATE_STATE", s => s.Enable().SetInt(1)));
        }

        // Nothing stays on a plate that cannot hold the window through the day and the 360 s of the night (an unmodded
        // plate holds 99 °C: dream extract dies from 60).
        public static void EndMorning()
        {
            if (OnThermostat(out var plate, out _)) PlateOff(plate);
        }

        // The plate's readout: the container's temperature in whole degrees, shown while it sits on the plate.
        static int Readout(GameItem v) => (int)v.GetTagReadonly("CONTAINER_TEMP_TAG").GetDouble();

        static void Heat(IExecutorHost ep, GameItem plate, GameItem evap)
        {
            var v = ById(vessel);
            if (product == Dust)
            {
                // Extracted yesterday: off the plate, base for any acid left (both pass the filters), filtered and dried.
                PlateOff(plate);
                Chore.Accept(EmporiumEntry.Instance.invElement, v, -1);
                Dose("base", v, Part(v, "acid") - Part(v, "base"));
                React("base", "acid", 1);
                step = Step.Heating;
                Filter(ep, plate, evap);
                return;
            }
            Dose("base", v, Part(v, "acid") - Part(v, "base"));
            React("base", "acid", 1);
            if (product == Phanta)
            {
                // Stabiliser gone: the debris through kotton, then reducing agent at 0.75× the methylol, at 124 °C.
                if (!Pass(v, "kotton_fabric") && !Pass(v, "water_filter")) { Stall("no_filter"); return; }
                Dose("reducing_agent", v, Part(v, "methylol") * 3 / 4);
                if (!HoldWindow(plate, v, 124)) return;
                step = Step.Heating;
                return;
            }
            // Caffeine: extracts from 50 °C and never degrades, so any plate will do (unmodded it holds 99 °C).
            if (Chore.Accept(Slot(plate, 1), v, -1) != 0) { Stall("plate"); return; }
            if (PlateModules(plate) >= 2) SetTarget(plate, 99);
            Chore.Act("plate_on", plate, () => plate.ModifyTag("HEATING_PLATE_STATE", s => s.Enable().SetInt(2)));
            step = Step.Heating;
        }

        // A pour through the filter pitcher with this filter seated, and back; false when none is at hand (a filter
        // spends 2 durability per ml removed and passes the rest once spent, so a worn one is changed first).
        static readonly string[] CoarseLiquids = { "debris", "plant_matter", "coffee_mush", "dream_cap_mush", "physical_contaminant", "mineral" };

        // Coarse matter's share of what will be left once the solvents are gone.
        static double Coarse(GameItem v)
        {
            int dry = Total(v) - Tag(v, "water") - Tag(v, "alcohol") - Tag(v, "lipobond_agent");
            return dry <= 0 ? 0 : CoarseLiquids.Sum(l => Tag(v, l)) / (double)dry;
        }

        static bool Pass(GameItem v, string filterId, bool fresh = false)
        {
            if (ById(filterPitcher) == null) PickVessels();
            var f = ById(filterPitcher);
            if (f == null) return false;
            var stock = EmporiumEntry.Instance.invElement;
            var seated = WaterHelper.GetFilterItem(f);
            if (seated == null || seated.identifier != filterId || DurabilityHelper.GetRealDurability(seated) < (fresh ? 200 : 60))
            {
                var next = Owned().Where(i => i.identifier == filterId && Loose(i) && DurabilityHelper.GetRealDurability(i) >= 60).OrderByDescending(i => DurabilityHelper.GetRealDurability(i)).ThenBy(i => i.uniqueId).FirstOrDefault();
                if (next == null) return false;
                if (seated != null) Chore.Accept(stock, seated, -1);
                foreach (var inv in GraphUtils.FindChildrenType<GameInventory>(f))
                    if (Chore.Accept(inv, next, -1) == 0) break;
            }
            if (WaterHelper.GetFilterItem(f)?.identifier != filterId) return false;
            if (Total(f) > 0) { Stall("filter_pitcher_full"); return false; }
            Chore.Act("pour", v, () => WaterHelper.TransferLiquid(v, f));
            Chore.Act("pour", f, () => WaterHelper.TransferLiquid(f, v));
            return true;
        }

        static bool Usable(string filterId) => Owned().Any(i => i.identifier == filterId && DurabilityHelper.GetRealDurability(i) >= 60);

        static void Filter(IExecutorHost ep, GameItem plate, GameItem evap)
        {
            var v = ById(vessel);
            var stock = EmporiumEntry.Instance.invElement;
            PlateOff(plate);
            if (product == Phanta)
            {
                // Reduced at 124 °C, where the water boils off too.
                Chore.Accept(stock, v, -1);
                step = Step.Dried;
                return;
            }
            // Kotton first takes the coarse load (mush, plant matter, debris), so the water filter's 400 durability goes
            // on the physical contaminant and mineral that kotton passes. Without a water filter the batch waits for one:
            // kotton alone leaves the physical contaminant in (caffeine 99 % → 83 %).
            if (!Usable("water_filter") && !Cartel && filterWait++ < WaitFilterDays) { Stall("wait_filter"); return; }
            Chore.Accept(stock, v, -1);
            bool k = !Legacy && Pass(v, "kotton_fabric");
            bool w = Pass(v, "water_filter");
            // A filter spent mid-pour passes the rest: while the spectrometer still shows coarse matter (or, without it,
            // when no kotton took the load), pour again through the next water filter.
            for (int again = 0; w && !Legacy && again < 2 && (Scans ? Coarse(v) > 0.002 : !k && again == 0); again++)
                if (!Pass(v, "water_filter", fresh: true)) break;
            if (!k && !w && !Pass(v, "kotton_fabric")) { Stall("no_filter"); return; }
            // Still cloudy (every filter spent): it waits for a fresh one rather than press the debris into the units.
            if (!Legacy && !Cartel && Scans && Coarse(v) > 0.01 && filterWait++ < WaitFilterDays) { Stall("filter_spent"); return; }
            if (!w) Stall("kotton_only");
            batchFilter = (k ? "kotton" : "") + (w ? "+water" : "");
            if (product == Caffeine && !Legacy && PlateModules(plate) >= 1 && Chore.Accept(Slot(plate, 1), v, -1) == 0)
            {
                // Dried on the plate above 100 °C (one module: fixed 160; two: set 104): no evaporator night.
                if (PlateModules(plate) >= 2) SetTarget(plate, 104);
                Chore.Act("plate_on", plate, () => plate.ModifyTag("HEATING_PLATE_STATE", s => s.Enable().SetInt(2)));
                step = Step.Dried;
                return;
            }
            Evaporate(evap, v);
        }

        static void Evaporate(GameItem evap, GameItem v)
        {
            var stock = EmporiumEntry.Instance.invElement;
            if (Charge(evap) < 6)
            {
                var cell = Owned().Where(i => Cell(i) && Loose(i)).OrderByDescending(i => PowerHelper.GetAvailableEnergyFromItem(i, false)).FirstOrDefault();
                var bat = MachineHelper.GetBatterySlot(evap);
                if (cell != null)
                {
                    if (bat.childItem != null) Chore.Accept(stock, bat.childItem, -1);
                    Chore.Accept(bat, cell, -1);
                }
            }
            if (Chore.Accept(Slot(evap, 2), v, -1) != 0) { Stall("evaporator"); return; }
            step = Step.Drying;
        }

        // Phanta is toxic from 75 %: a residue that would press at 74.5 % or more is cut with acid (inert in it; 0.1
        // lethality a µL is nothing at pill scale) down to `phanta_target`. Read on the spectrometer.
        static void CutPhanta(GameItem v, int finisherModules)
        {
            int total = Total(v), active = Part(v, "fanta");
            if (total <= 0 || active <= 0) return;
            double keep = 1 - FinisherLoss[Clamp4(finisherModules)];
            if (active / (double)total * keep < 0.745) return;
            Dose("acid", v, (int)(active * keep / PhantaTarget - total) + 1000);
        }

        // The handler judges the count and the mean purity, best at the required purity + 10. While the residue would
        // press fewer units than the count still missing, it is cut with acid (inert in dust; 0.1 lethality a µL is
        // nothing at 2 ml) down to no lower than required + `quota_margin` (default 12), which stretches the extract
        // over more units. Read on the spectrometer.
        static void CutForQuota(GameItem v, int finisherModules)
        {
            if (!Cartel || !Scans) return;
            int missing = QuotaCount + 2 - DustHeld().Count;
            int total = Total(v), active = Part(v, "dream_cap_extract");
            if (missing <= 0 || total <= 0 || active <= 0) return;
            double keep = 1 - FinisherLoss[Clamp4(finisherModules)];
            double purity = active / (double)total * keep;
            double floor = (QuotaPurity + (Cfg?.Value<int?>("quota_margin") ?? 12)) / 100.0;
            int units = total / 2000;
            if (units >= missing || purity <= floor) return;
            double target = Math.Max(floor, purity * units / missing);
            int add = (int)(active * keep / target) - total;
            if (add > 1000) CutUl += Dose("acid", v, add);
        }

        public static long CutUl;

        // True once the batch is pressed and the vessel is free again.
        static bool Finish(IExecutorHost ep, GameItem plate, GameItem evap, GameItem finisher)
        {
            var v = ById(vessel);
            var stock = EmporiumEntry.Instance.invElement;
            if (step == Step.Dried && ReferenceEquals((Slot(plate, 1) as GameSlotInventory)?.childItem, v))
            {
                PlateOff(plate);
                Chore.Accept(stock, v, -1);
            }
            // The finisher's status line says "wet" over 100 µL of water, alcohol and lipobond.
            if (WaterHelper.IsWet(v))
            {
                if (step == Step.Dried) { Evaporate(evap, v); return false; }
                if (Charge(evap) < 6) Stall("evaporator_power");
                return false;
            }
            var r = R(product);
            if (r.Id == Phanta) CutPhanta(v, Fitted("chem_finisher"));
            if (r.Id == Dust) CutForQuota(v, Fitted("chem_finisher"));
            if (Chore.Accept(Slot(finisher, 1), v, -1) != 0) { Stall("finisher"); return false; }
            Chore.Act("configure", finisher, () => finisher.ModifyTag("SELECTED_CHEM_ID_TAG", s => s.Enable().SetString(r.Id)));
            var output = (GameInventory)Slot(finisher, 2);
            string label = LocHelper.GetLocalizedMechanic("mech_chem_finisher_btn_output");
            int pressed = 0;
            var o = Per(r.Id);
            var grades = (JObject)o["grades"];
            var batchGrades = new JObject();
            int residueTotal = Total(v);
            var residue = new JObject();
            if (residueTotal > 0)
                foreach (var l in Liquid.Liquids.Select(l => (l.identifier, n: Tag(v, l.identifier))).Where(x => x.n > 0).OrderByDescending(x => x.n).Take(6))
                    residue[l.identifier] = Math.Round(100.0 * l.n / residueTotal, 2);
            for (int k = 0; k < 400 && MachineChemFinisher.CanOutput(finisher, (GameSlotInventory)Slot(finisher, 1)); k++)
            {
                var press = ep.MachineButton(finisher, label);
                if (press == null) break;
                press();
                pressed++;
                foreach (var p in output.childItems.Where(x => x != null).ToList())
                {
                    if (!made.Add(p.uniqueId)) continue;
                    Chore.Act("own", p, () => GeneralHelper.SetItemOwned(p, true));
                    Pills++;
                    long est = PlayerRole.Est(p);
                    PillValue += est;
                    Add(o, "units", 1);
                    Add(o, "value", est);
                    Add(grades, ChemicalProductHelper.GetCurrentChemicalConditionId(p) ?? "none", 1);
                    Add(batchGrades, ChemicalProductHelper.GetCurrentChemicalConditionId(p) ?? "none", 1);
                    if (Chore.Accept(stock, p, -1) != 0) Chore.Accept(EmporiumEntry.Instance.backInvinvElement, p, -1);
                }
            }
            if (pressed == 0) { Stall("press"); return false; }
            if (!Legacy) Chore.Accept(stock, v, -1);
            Batches++;
            Add(o, "batches", 1);
            Add((JObject)o["expected"], GradeName[Band(batchExpect)], 1);
            var log = o["log"] as JArray ?? (JArray)(o["log"] = new JArray());
            if (log.Count < 60) log.Add(new JObject { ["day"] = ep.Day, ["inputs"] = batchInputs, ["units"] = pressed, ["expect"] = GradeName[Band(batchExpect)], ["filter"] = batchFilter, ["f"] = Fitted("chem_finisher"), ["b"] = Fitted("blender"), ["p"] = PlateModules(plate), ["grades"] = batchGrades, ["residue_ml"] = residueTotal / 1000, ["residue_pct"] = residue });
            step = Step.Idle;
            ep.Human("production", null, ep.Uid(v), new JObject { ["action"] = "lab_finish", ["product"] = r.Id, ["inputs"] = batchInputs, ["units"] = pressed, ["days"] = ep.Day - batchDay }, "player:lab");
            product = null;
            return true;
        }

        public static void Trade(bool sold, GameItem item, int amount)
        {
            if (!On || !sold || item == null || !made.Contains(item.uniqueId)) return;
            if (item.identifier == Caffeine) { PillsSold++; PillSales += amount; }
            var o = Per(item.identifier);
            Add(o, "sold", 1);
            Add(o, "sales", amount);
        }

        public static JObject Status()
        {
            if (!On) return null;
            var kit = KitNow();
            var plate = Find("heating_plate");
            var expect = new JObject();
            foreach (var r in Recipes.Where(r => Makes(r.Id)))
            {
                var (p, u) = Expect(r, kit.f, kit.b, kit.wf);
                expect[r.Id] = new JObject { ["purity"] = Math.Round(p * 100, 1), ["grade"] = GradeName[Band(p)], ["units_per_input"] = Math.Round(u, 1), ["value_per_input"] = Math.Round(ValuePerInput(r, kit.f, kit.b, kit.wf), 1) };
            }
            var o = new JObject
            {
                ["unlocked"] = Up, ["step"] = step.ToString(), ["product"] = product, ["batches"] = Batches, ["beans_used"] = BeansUsed, ["pills"] = Pills, ["pill_value"] = PillValue,
                ["pills_sold"] = PillsSold, ["pill_sales"] = PillSales, ["packs_opened"] = PacksOpened, ["modules_fitted"] = ModulesFitted,
                ["finisher_modules"] = kit.f, ["blender_modules"] = kit.b, ["plate_modules"] = plate == null ? 0 : PlateModules(plate), ["scanner"] = Scans, ["water_filter"] = kit.wf,
                ["stalls"] = JObject.FromObject(stalls), ["expect"] = expect, ["products"] = JObject.FromObject(per),
            };
            if (modulePlan != null) o["module_plan"] = modulePlan;
            if (NewGameData.Instance?.startType == NewGameData.StartType.DRUG_DEALER)
                o["cartel"] = new JObject { ["exited"] = PlayerStore.Instance.isCartelExited, ["rep"] = StoreReputation.GetCartelReputation(), ["count"] = QuotaCount, ["purity"] = QuotaPurity, ["visits"] = QuotaVisits, ["staged"] = QuotaStaged, ["cut_ml"] = CutUl / 1000, ["exit_stage"] = PlayerStore.Instance.cartelExitStage };
            return o;
        }
    }
}
