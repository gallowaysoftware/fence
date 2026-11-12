using System;
using System.Collections.Generic;
using System.Linq;
using Newtonsoft.Json.Linq;

namespace Fence.Gym
{
    // `engine: furnace` (docs/re/PRODUCTION.md §3): Dr. Jackson's furnace (offered on day 6 at the
    // new-customer discount, then on his visits) smelts scrap and ore into ingots each night; the junk
    // module turns six junk from the dumping ground into four scrap on nights it does not smelt. Each
    // morning, as a player at the furnace window: ingots out to stock, junk-made scrap back to the input,
    // modules and a charged cell in, a flux and every owned scrap and ore loaded, and six junk on nights
    // with too little metal to smelt. Ingots are ordinary MATERIAL stock; inputs, modules, a spare cell
    // and two conversions' worth of junk are held.
    internal static class Furnace
    {
        public static bool On => (Directives.Current.Engine ?? "").Split('+').Contains("furnace");

        // `furnaces` (gym, default 1): how many furnaces to run. `furnace_blast` (default true): fit the
        // blast module (3 inputs -> 2 ingots, double power).
        static int Target => Directives.Current.Raw.Value<int?>("furnaces") ?? 1;
        static bool BlastOn => Directives.Current.Raw.Value<bool?>("furnace_blast") ?? true;
        // `ore_phone` (default false): buy Wild's MINER introduction and phone Caulson for ore whenever his line
        // is free (PhoneDialogList.MiningIntro / MiningRegular: 8 free ore, then 6 a call, 12 once 350 is spent
        // with him; 5-day cooldown; wholesale -20 %). `furnace_metal`: scrap and ore held per furnace
        // (default 9, 21 with the phone so a 12-ore delivery fits on top of a full input).
        static bool PhoneOn => Directives.Current.Raw.Value<bool?>("ore_phone") ?? false;
        // `blast_first` (default true with the phone): the blast module without waiting for the junk module, since
        // phoned ore, not junk, is what binds then.
        static bool BlastFirst => Directives.Current.Raw.Value<bool?>("blast_first") ?? PhoneOn;
        static int MetalCap => Directives.Current.Raw.Value<int?>("furnace_metal") ?? (PhoneOn ? 21 : MetalPerFurnace);

        const string Scrap = "scrap_metal", Ore = "common_ore", Junk = "junk", Flux = "flux_agent", AdvFlux = "advanced_flux_agent";
        const string JunkModule = "furnace_module_junk", BlastModule = "furnace_module_blast", Ingot = "metal_ingot";

        public static int Smelts, Conversions, IngotsMade, Charge, OreOrdered, PhoneOrders, PhoneHeldMetal, PhoneHeldCash, PhoneHeldRoom;
        public static long IngotValue, InputValue, IngotSales, IngotSold, InputSpend, CapitalSpend, ChargeSpend, ChargeBought;
        public static int FirstDay;
        static readonly Dictionary<int, int> loaded = new Dictionary<int, int>();
        static readonly Dictionary<int, int> lastCharge = new Dictionary<int, int>();

        public static void Reset()
        {
            Smelts = Conversions = IngotsMade = Charge = FirstDay = OreOrdered = PhoneOrders = PhoneHeldMetal = PhoneHeldCash = PhoneHeldRoom = 0;
            IngotValue = InputValue = IngotSales = IngotSold = InputSpend = CapitalSpend = ChargeSpend = ChargeBought = 0;
            loaded.Clear();
            lastCharge.Clear();
            keepJunk.Clear();
        }

        static IEnumerable<GameItem> Owned() => EmporiumEntry.Instance.GetAllOwnedItems(false, true).Where(i => i != null);
        static GameItem Parent(GameItem i) => i.parentInventory?.GetParentItem();
        static bool Working(GameItem i) => i.identifier == "furnace" && !i.IsTag("BROKEN_MACHINE_TAG");
        static bool Metal(GameItem i) => i.identifier == Scrap || i.identifier == Ore;

        public static List<GameItem> Furnaces() => EmporiumEntry.Instance == null ? new List<GameItem>() : Owned().Where(i => Working(i) && Parent(i) == null).ToList();

        // MachineFurnace.CreateMachineInventoryWindow: battery, module grid, input, output, note.
        static GameGridInventory Modules(GameItem f) => MachineHelper.GetBodyStorage(f, 1) as GameGridInventory;
        static GameGridInventory Input(GameItem f) => MachineHelper.GetBodyStorage(f, 2) as GameGridInventory;
        static GameGridInventory Output(GameItem f) => MachineHelper.GetBodyStorage(f, 3) as GameGridInventory;

        static bool Has(GameItem f, string module) => Modules(f)?.childItems.Any(m => m != null && m.identifier == module) ?? false;

        static int Energy(GameItem cell) => cell == null ? 0 : PowerHelper.GetAvailableEnergyFromItem(cell, false);
        static int BatteryCharge(GameItem f) => Energy(MachineHelper.GetBatterySlot(f)?.childItem);
        static int PerSmelt(GameItem f) => Has(f, BlastModule) ? 16 : 8;

        static bool Takes(string id) => id == "furnace" || id == Scrap || id == Ore || id == Flux || id == AdvFlux || id == JunkModule || id == BlastModule;

        // Junk, scrap, ore and ingots are all 3x2 (six cells; the input and output grids hold nine): junk is held
        // only for the next two conversions, the rest is stock.
        const int JunkPerFurnace = 12, MetalPerFurnace = 9;
        static readonly HashSet<int> keepJunk = new HashSet<int>();

        // What the furnace holds back from sale: the machine, its modules and inputs, and one spare cell.
        public static bool Reserved(GameItem i)
        {
            if (!On || EmporiumEntry.Instance == null) return false;
            if (Working(i)) return Furnaces().Count > 0 && Furnaces().Take(Target).Contains(i);
            if (i.identifier == "energy_credit") return Parent(i) == null && Furnaces().Count > 0 && SpareCell()?.uniqueId == i.uniqueId;
            if (i.identifier == Junk) return keepJunk.Contains(i.uniqueId);
            return Takes(i.identifier);
        }

        static GameItem SpareCell() => Owned().Where(i => i.identifier == "energy_credit" && Parent(i) == null && Energy(i) >= 16)
            .OrderByDescending(Energy).ThenBy(i => i.uniqueId).FirstOrDefault();

        public static bool IsInput(string id) => On && id != null && (Takes(id) || (id == "energy_credit" && Furnaces().Count > 0));

        // The seller's goods staged on the back counter are all furnace inputs.
        public static bool BuyingInputs(JArray theirs)
        {
            if (!On) return false;
            var back = PlayerStock.BackUids();
            var staged = theirs.Where(i => back.Contains(i.Value<int?>("uid") ?? -1)).ToList();
            return staged.Count > 0 && staged.All(i => IsInput(i.Value<string>("id")));
        }

        static int Cash => PlayerStore.Instance.playerCash;
        static int RentDue => PlayerDay.Current?.RentDue ?? PlayerStore.Instance.GetRentValue();

        // How many more of an item the furnace wants; 0 when the engine is off.
        public static int Wants(string id)
        {
            if (!On || id == null || EmporiumEntry.Instance == null) return 0;
            var owned = Owned().ToList();
            int n = Furnaces().Count;
            switch (id)
            {
                // Capital while the cash covers the next rent (the counter keeps the rent floor after the buy); the junk
                // module comes first, since scrap is what binds, then the blast module; one of each per furnace run.
                // One at a time: a seller's lot is bought whole or not at all.
                case "furnace": return n < Target && Cash >= RentDue ? 1 : 0;
                case JunkModule: return n > 0 && owned.Count(i => i.identifier == JunkModule) < Math.Min(n, Target) && Cash >= RentDue ? 1 : 0;
                case BlastModule: return BlastOn && n > 0 && (BlastFirst || owned.Any(i => i.identifier == JunkModule)) && owned.Count(i => i.identifier == BlastModule) < Math.Min(n, Target) && Cash >= RentDue ? 1 : 0;
                // Two weeks of smelting per furnace (three inputs a night with the blast module).
                case Scrap:
                case Ore: return Math.Max(0, Math.Max(1, n) * MetalCap - owned.Count(Metal));
                case Flux: return Math.Max(0, Math.Max(1, n) * 7 - owned.Count(i => i.identifier == Flux));
                // Reusable: worth it after ~75 smelts against 4-credit flux; only from a real surplus.
                case AdvFlux: return n > 0 && owned.Any(i => i.identifier == BlastModule) && !owned.Any(i => i.identifier == AdvFlux) && Cash >= RentDue + 1500 ? 1 : 0;
                case "energy_credit": return n > 0 && SpareCell() == null && Furnaces().Any(f => BatteryCharge(f) < 3 * PerSmelt(f)) ? 1 : 0;
            }
            return 0;
        }

        public static void Morning(IExecutorHost ep)
        {
            if (!On) return;
            var furnaces = Furnaces();
            if (furnaces.Count > 0 && FirstDay == 0) FirstDay = ep.Day;
            var stock = EmporiumEntry.Instance.invElement;
            foreach (var f in furnaces.Take(Target))
            {
                try { Tend(ep, f, stock); }
                catch (Exception e) { ep.ProductionError(e); }
            }
            try { Phone(ep, furnaces.Count); }
            catch (Exception e) { ep.ProductionError(e); }
            keepJunk.Clear();
            var mine = furnaces.Take(Target).Where(f => Has(f, JunkModule)).ToList();
            foreach (var j in Owned().Where(i => i.identifier == Junk && (Parent(i) == null || mine.Contains(Parent(i))))
                         .OrderByDescending(i => Parent(i) != null).ThenBy(i => i.uniqueId).Take(JunkPerFurnace * mine.Count))
                keepJunk.Add(j.uniqueId);
        }

        // Bought with cash as an essential (375 on hard) once a furnace runs: the phone is the only steady ore.
        public static bool WantsMiner => On && PhoneOn && Furnaces().Count > 0 && !(NetworkUpgrade.GetUpgradeById("MINER")?.IsUnlocked() ?? true);

        // The order the call places (the host makes the call); the courier sells at the counter as any seller.
        static void Phone(IExecutorHost ep, int n)
        {
            if (!PhoneOn || n == 0 || !(NetworkUpgrade.GetUpgradeById("MINER")?.IsUnlocked() ?? false)) return;
            var c = StorePhoneClient.GetMiningPhoneClient();
            string courier;
            int size, days, choice = -1;
            if (c.phoneState == StorePhoneClient.PhoneState.Introduction)
                (courier, size, days) = ("courrierMiningFree", 8, 1);
            else if (c.phoneState == StorePhoneClient.PhoneState.Regular)
            {
                bool big = c.amountSpent >= 350;
                (courier, size, days) = ("courrierMining", big ? 12 : 6, big ? 2 : 1);
                // The courier's lot is bought only if it fits the stock grid (ore is 3x2).
                if (Owned().Count(Metal) + size > n * MetalCap) { PhoneHeldMetal++; return; }
                if (Cash < RentDue + 15 * size || (PlayerDay.Current?.Cuts("inputs") ?? false)) { PhoneHeldCash++; return; }
                if (PlayerStock.FreeCells() < 6 * size) { PhoneHeldRoom++; return; }
                choice = big ? 1 : 0;
            }
            else return;
            ep.PhoneOrder(c, StorePhoneClient.MINING_NUMBER, choice, courier, days);
            PhoneOrders++;
            OreOrdered += size;
            ep.Human("phone_order", null, null, new JObject { ["supplier"] = "mining", ["courier_id"] = courier, ["in_days"] = days, ["ore"] = size }, "player:furnace");
        }

        static int FreeCells(GameGridInventory g) => g.inventoryShape.width * g.inventoryShape.height - g.childItems.Where(i => i != null).Sum(PlayerRole.Cells);

        static void Tend(IExecutorHost ep, GameItem f, GameGridInventory stock)
        {
            if (!ReferenceEquals(f.parentInventory, stock) && Chore.Accept(stock, f, -1) != 0 && !ReferenceEquals(f.parentInventory, EmporiumEntry.Instance.showcaseElement))
                Chore.Accept(EmporiumEntry.Instance.showcaseElement, f, -1);
            var input = Input(f);
            var output = Output(f);
            if (input == null || output == null) return;

            // Last night's work: what left the input was consumed, what is in the output was made.
            var inputNow = new HashSet<int>(input.childItems.Where(i => i != null).Select(i => i.uniqueId));
            foreach (var kv in loaded.Where(kv => !inputNow.Contains(kv.Key)).ToList())
            {
                InputValue += kv.Value;
                loaded.Remove(kv.Key);
            }
            int made = 0, scrapMade = 0;
            foreach (var it in output.childItems.Where(i => i != null).ToList())
            {
                if (it.identifier == Ingot)
                {
                    int v = PlayerRole.Est(it);
                    // A full output grid stops the furnace: with no room in stock the ingot goes on display.
                    if (Chore.Accept(stock, it, -1) == 0 || Chore.Accept(EmporiumEntry.Instance.showcaseElement, it, -1) == 0) { made++; IngotsMade++; IngotValue += v; Quality.Ingot(it); }
                }
                else if (it.identifier == Scrap)
                {
                    scrapMade++;
                    if (Chore.Accept(input, it, -1) == 0) loaded[it.uniqueId] = 10;
                    else Chore.Accept(stock, it, -1);
                }
                else Chore.Accept(stock, it, -1);
            }
            Smelts += Has(f, BlastModule) ? (made + 1) / 2 : made;
            Conversions += scrapMade / 4;
            // Scrap made from junk is credited here and charged again when it is smelted.
            InputValue -= 10L * scrapMade;

            foreach (var id in new[] { JunkModule, BlastModule })
            {
                if (Has(f, id) || (id == BlastModule && !BlastOn)) continue;
                var m = Owned().FirstOrDefault(i => i.identifier == id && Parent(i) == null);
                if (m != null) Chore.Accept(Modules(f), m, -1);
            }
            if (!BlastOn && Has(f, BlastModule))
            {
                var m = Modules(f).childItems.First(i => i != null && i.identifier == BlastModule);
                Chore.Accept(stock, m, -1);
            }

            var slot = MachineHelper.GetBatterySlot(f);
            int charge = BatteryCharge(f);
            if (lastCharge.TryGetValue(f.uniqueId, out int before) && before > charge) Charge += before - charge;
            if (slot != null && charge < 2 * PerSmelt(f))
            {
                var cell = SpareCell();
                if (cell != null && Energy(cell) > charge)
                {
                    var old = slot.childItem;
                    if (old == null || Chore.Accept(stock, old, -1) == 0) Chore.Accept(slot, cell, -1);
                }
            }
            lastCharge[f.uniqueId] = BatteryCharge(f);

            // One job a night: a smelt whenever the input holds enough metal, else (junk module) six junk into
            // four scrap. A smelt night needs room for the metal and a flux, so junk waits in stock.
            int num = Has(f, BlastModule) ? 3 : 2;
            var inInput = input.childItems.Where(i => i != null).ToList();
            int metalIn = inInput.Count(Metal);
            var freeMetal = Owned().Where(i => Parent(i) == null && Metal(i)).OrderBy(i => i.identifier == Ore ? 0 : 1).ThenBy(i => i.uniqueId).ToList();
            bool smelt = metalIn + freeMetal.Count >= num;
            bool fluxIn = inInput.Any(i => i.identifier == Flux || i.identifier == AdvFlux);
            var flux = Owned().Where(i => Parent(i) == null && (i.identifier == AdvFlux || i.identifier == Flux)).OrderBy(i => i.identifier == AdvFlux ? 0 : 1).ThenBy(i => i.uniqueId).FirstOrDefault();
            if (smelt)
            {
                int need = (fluxIn || flux == null ? 0 : 1) + 6 * Math.Max(0, num - metalIn);
                foreach (var j in inInput.Where(i => i.identifier == Junk).ToList())
                {
                    if (FreeCells(input) >= need) break;
                    if (Chore.Accept(stock, j, -1) == 0) loaded.Remove(j.uniqueId);
                }
            }
            void Load(GameItem it)
            {
                if (Chore.Accept(input, it, -1) != 0 || !ReferenceEquals(Parent(it), f)) return;
                if (it.identifier != AdvFlux) loaded[it.uniqueId] = PlayerRole.Est(it);
            }
            if (!fluxIn && flux != null) Load(flux);
            foreach (var m in freeMetal) Load(m);
            if (!smelt && Has(f, JunkModule))
                foreach (var j in Owned().Where(i => Parent(i) == null && i.identifier == Junk).OrderBy(i => i.uniqueId).Take(Math.Max(0, 6 - inInput.Count(i => i.identifier == Junk))).ToList())
                    Load(j);
            if (made + scrapMade > 0)
                ep.Human("production", null, ep.Uid(f), new JObject { ["action"] = "furnace", ["ingots"] = made, ["scrap_from_junk"] = scrapMade, ["charge"] = BatteryCharge(f) }, "player:furnace");
        }

        // Cash in and out of the furnace's line of business (Episode.Player.cs ItemTrade).
        public static void Trade(bool sold, GameItem item, int amount)
        {
            if (!On || item == null) return;
            if (sold && item.identifier == Ingot) { IngotSales += amount; IngotSold++; }
            if (sold) return;
            if (item.identifier == "furnace" || item.identifier == JunkModule || item.identifier == BlastModule || item.identifier == AdvFlux) CapitalSpend += amount;
            else if (item.identifier == "energy_credit") { ChargeSpend += amount; ChargeBought += Energy(item); }
            else if (Takes(item.identifier)) InputSpend += amount;
        }

        // Charge valued at what cells cost when bought, else ~1 credit a unit (PowerHelper.GetPowerSourceItemValue).
        static double ChargePrice => ChargeBought > 0 ? (double)ChargeSpend / ChargeBought : 1.0;

        public static JObject Status()
        {
            if (!On) return null;
            var owned = Owned().ToList();
            return new JObject
            {
                ["furnaces"] = Furnaces().Count,
                ["modules"] = new JArray(Furnaces().SelectMany(f => Modules(f)?.childItems.Where(m => m != null).Select(m => m.identifier) ?? Enumerable.Empty<string>())),
                ["battery"] = new JArray(Furnaces().Select(BatteryCharge)),
                ["scrap"] = owned.Count(i => i.identifier == Scrap),
                ["ore"] = owned.Count(i => i.identifier == Ore),
                ["junk"] = owned.Count(i => i.identifier == Junk),
                ["flux"] = owned.Count(i => i.identifier == Flux),
                ["ingots_in_stock"] = owned.Count(i => i.identifier == Ingot),
                ["ingots_made_total"] = IngotsMade,
            };
        }

        public static JObject Totals(int day)
        {
            double weeks = FirstDay > 0 ? Math.Max(1, day - FirstDay + 1) / 7.0 : 0;
            double net = IngotValue - InputValue - Charge * ChargePrice;
            return new JObject
            {
                ["first_day"] = FirstDay,
                ["smelts"] = Smelts,
                ["junk_conversions"] = Conversions,
                ["phone_orders"] = PhoneOrders,
                ["ore_ordered"] = OreOrdered,
                ["phone_held"] = new JObject { ["metal"] = PhoneHeldMetal, ["cash"] = PhoneHeldCash, ["room"] = PhoneHeldRoom },
                ["ingots_made"] = IngotsMade,
                ["ingot_value"] = IngotValue,
                ["inputs_consumed_value"] = InputValue,
                ["charge_used"] = Charge,
                ["charge_price"] = Math.Round(ChargePrice, 2),
                ["ingots_sold"] = IngotSold,
                ["ingot_sales"] = IngotSales,
                ["input_spend"] = InputSpend,
                ["charge_spend"] = ChargeSpend,
                ["capital_spend"] = CapitalSpend,
                ["net_value_added"] = Math.Round(net),
                ["net_per_week"] = weeks > 0 ? Math.Round(net / weeks, 1) : 0,
            };
        }
    }
}
