using System;
using System.Collections.Generic;
using System.Linq;
using Newtonsoft.Json.Linq;

namespace Fence.Gym
{
    // `arms: {…}` (docs/re/ARMS.md): the Armourer's engine, off unless the directives carry an `arms` object.
    //   `window`: the weapon permits held go in the display case first, while a legal gun is in stock
    //     (`window_when_stocked`): each brings its tier's gun-order client (ShowcaseHelper.GUN_BUYERS), nothing else does.
    //   `orders`: a gun-order client is served from stock: the legal gun (with loose mods attached, and up to
    //     `repair_parts` gun parts when a repair step would clear the last threshold) whose reward beats `min_pct` % of
    //     the gun and its mods at their estimates; a service order takes mods on the client's gun when the bonuses pay.
    //     Otherwise the client is sent away (it stays as a melee buyer).
    //   `permits`: Weapon Permits up to `permit_max` (default 2) from day `permit_from`, bought once Security is within
    //     `permit_margin` (default 5) of the gate (50, 80): a permit only arrives at a shutter opening at its gate.
    //   `keep_guns` (default 4): legal guns held for orders, bought as inputs while fewer are held; under Permit II two
    //     of each Mid mod are bought too (`buy_mods`). Saving for the buyout or selling down, nothing is held.
    //   `print_guns`: with `printer: "run"`, the pistol chips print first while fewer than `keep_guns` are held.
    //   `show_weapons` (default off): held guns and mods may sit in the display case.
    //   `forge` (FORMULA-EXPLOITS §2, docs/re/ARMS.md §6): build the order guns instead of buying them. A new printer from
    //     the retired gunsmith (the Gunsmith start's old one has no room beside its stuck modules for the metal and CNC
    //     modules) prints frames from the furnace's ingots and gun parts; a frame finished with a part is a New gun with
    //     base stats x (100 + the ingot's quality bonus) %. Each permit goes on display only while a gun of its tier is
    //     held (`window_per_tier`); guns are no longer bought (`buy_guns`). Options: `forge_from` (day), `forge_printers`
    //     (1), `keep_handmade` (3), `keep_heavy` (3), `forge_reserve` (cash kept above the rent when buying the kit, 500),
    //     `forge_until` (70).
    // Weapons above the permit stay contraband, which the executor neither buys nor sells.
    internal static class Arms
    {
        static JObject O => Directives.Current.Raw["arms"] as JObject;
        public static bool On => O != null && (O.Value<bool?>("on") ?? true);
        static bool Opt(string k) => On && (O.Value<bool?>(k) ?? true);
        static int Num(string k, int d) => O?.Value<int?>(k) ?? d;

        public static int KeepGuns => Num("keep_guns", Forge ? KeepHandmade + KeepHeavy : 4);

        public static int Orders, OrderIncome, GivenValue, Service, Best, Sent, PartsUsed, Repairs, ArmsSales;
        public static int ForgeDay, Finished, ForgeCapital, ForgeOrders, ForgeIncome;
        static readonly SortedDictionary<string, int> finishedByQ = new SortedDictionary<string, int>(StringComparer.Ordinal);
        public static int Permit2Day, Permit3Day, PermitLostDay;
        public static readonly int[] ByTier = new int[4], IncomeByTier = new int[4], Offered = new int[4];
        static readonly SortedDictionary<string, int> refusals = new SortedDictionary<string, int>(StringComparer.Ordinal);

        public static void Reset()
        {
            Orders = OrderIncome = GivenValue = Service = Best = Sent = PartsUsed = Repairs = ArmsSales = 0;
            Permit2Day = Permit3Day = PermitLostDay = 0;
            ForgeDay = Finished = ForgeCapital = ForgeOrders = ForgeIncome = 0;
            finishedByQ.Clear();
            knownIngots.Clear();
            kitBought.Clear();
            Array.Clear(ByTier, 0, 4);
            Array.Clear(IncomeByTier, 0, 4);
            Array.Clear(Offered, 0, 4);
            refusals.Clear();
        }

        static IEnumerable<GameItem> Owned() => EmporiumEntry.Instance.GetAllOwnedItems(false, true).Where(i => i != null);
        static GameItem Parent(GameItem i) => i.parentInventory?.GetParentItem();

        public static int Tier => PermitHelper.IsGunPermitIII() ? 3 : PermitHelper.IsGunPermitII() ? 2 : PermitHelper.IsGunPermitI() ? 1 : 0;

        public static bool IsGun(GameItem i) => i != null && i.IsTag("ADV_GUN_TAG") && !GunHelper.IsServiceGun(i);
        static bool IsMod(GameItem i) => i != null && i.IsTag("WEAPON_MOD_TAG");
        public static bool IsPermit(GameItem i) => i != null && i.IsTag(PermitHelper.PERMIT_ITEM_TAG);
        static bool Legal(GameItem i) => ContrabandHelper.GetContrabandLevelInInt(i) == 0 && !StolenHelper.IsStolenItem(i);

        // The guns held for orders: legal, loose, the most valuable first (an order's thresholds favour the better gun).
        static List<GameItem> HeldGuns() => Owned().Where(i => IsGun(i) && Legal(i) && Parent(i) == null)
            .OrderByDescending(PlayerRole.Est).ThenBy(i => i.uniqueId).Take(KeepGuns).ToList();

        public static bool Reserved(GameItem i)
        {
            if (!On || i == null) return false;
            if (IsPermit(i) || i.identifier == "screwdriver") return true;
            if (!Opt("orders")) return false;
            // Saving for the buyout or selling down for rent, the stock held for orders is cash like any other.
            var day = PlayerDay.Current;
            if (Forge && !(day?.SavingForBuyout ?? false) && ForgeKit(i)) return true;
            if (day != null && (day.SavingForBuyout || day.Liquidating)) return false;
            if (IsGun(i)) return HeldGuns().Any(g => g.uniqueId == i.uniqueId);
            // Two of each mod and four gun parts: what an order's bonuses and a repair use; the rest sells.
            if (IsMod(i) && Parent(i) == null) return Owned().Where(m => m.identifier == i.identifier && IsMod(m) && Parent(m) == null).OrderBy(m => m.uniqueId).Take(2).Any(m => m.uniqueId == i.uniqueId);
            if (i.identifier == "gun_part") return Owned().Where(m => m.identifier == "gun_part").OrderBy(m => m.uniqueId).Take(4).Any(m => m.uniqueId == i.uniqueId);
            return false;
        }

        static readonly string[] GunIds = { "printed_gun", "handmade_pistol", "heavy_handmade_pistol", "glock_receiver", "revolver", "smg", "shotgun" };

        static int HeldCount() => Owned().Count(i => IsGun(i) && Legal(i) && Parent(i) == null);

        // The mods that carry a printed or handmade pistol past the Mid rows (FP > 25/26, REL > 32, ACC > 33).
        static readonly string[] MidMods = { "mag_22_high", "mag_22_ext", "internal_springs", "rds", "match_barrel" };

        static bool MidMod(string id) => PermitHelper.IsGunPermitII() && Array.IndexOf(MidMods, id) >= 0;

        static bool BuyGuns => O?.Value<bool?>("buy_guns") ?? !Forge;

        public static bool IsInput(string id) => Opt("orders") && id != null && ((BuyGuns && Array.IndexOf(GunIds, id) >= 0) || (Opt("buy_mods") && MidMod(id)) || (Forge && (Array.IndexOf(ForgeKitIds, id) >= 0 || id == "scrap_metal")));

        // Guns are bought (legal ones only reach the counter's buy path) while fewer than `keep_guns` are held; under
        // Permit II, two of each Mid mod (`buy_mods`).
        public static int Wants(string id)
        {
            if (!IsInput(id)) return 0;
            if (Array.IndexOf(ForgeKitIds, id) >= 0) return ForgeWants(id);
            // A heavy frame takes a scrap, which the furnace otherwise smelts: two held for the next frames.
            if (id == "scrap_metal") return Forge && Owns(HeavyChip) > 0 && PermitHelper.IsGunPermitII() ? Math.Max(0, 2 - Owned().Count(i => i.identifier == id && Parent(i) == null)) : 0;
            if (Array.IndexOf(GunIds, id) >= 0) return Math.Max(0, KeepGuns - HeldCount());
            return Math.Max(0, 2 - Owned().Count(i => i.identifier == id && Parent(i) == null));
        }

        public static bool PrintGuns => Opt("print_guns") && PermitHelper.IsGunPermitI() && HeldCount() < KeepGuns;

        public static readonly string[] GunChips = { "printer_chip_pistol_double", "printer_chip_pistol" };

        // ---- the frame forge ----

        public const string HandmadeChip = "printer_chip_handmade_pistol_frame", HeavyChip = "printer_chip_heavy_handmade_pistol_frame", PartChip = "printer_chip_gun_part";
        const string MetalModule = "printer_module_metal", CncModule = "printer_module_cnc", Frame = "GUN_FRAME_TAG", Ingot = "metal_ingot", Part = "gun_part";
        static readonly string[] ForgeKitIds = { "printer", MetalModule, CncModule, HeavyChip, HandmadeChip, PartChip };

        public static bool Forge => On && O["forge"] is JToken f && f.Type != JTokenType.Null && !(f.Type == JTokenType.Boolean && !(bool)f);
        static bool ForgeLive => Forge && (PlayerDay.Current?.Day ?? 0) >= Num("forge_from", 0) && !(PlayerDay.Current?.SavingForBuyout ?? false);
        static int ForgePrinters => Num("forge_printers", 1);
        static int KeepHandmade => Num("keep_handmade", 3);
        static int KeepHeavy => Num("keep_heavy", 3);

        // The old printer's two stuck modules leave no room for both the metal and the CNC module (lab, forge.printers).
        public static bool Forgeable(GameItem printer) => printer != null && MachineHelper.GetBodyStorage(printer, 1) is GameGridInventory bay && !bay.childItems.Any(m => m != null && m.IsTag("MODULE_STUCK_TAG"));

        static int Owns(string id) => Owned().Count(i => i.identifier == id);

        static bool CashFor => PlayerStore.Instance.playerCash >= (PlayerDay.Current?.RentDue ?? 0) + Num("forge_reserve", 500) && !(PlayerDay.Current?.Cuts("upgrades") ?? false);

        // The kit, one of each per forge printer: a new printer, the metal and CNC modules; one of each chip. The heavy
        // chip only on a route that buys Permit II (a heavy frame is level 2).
        // Each piece is bought at most once per forge printer: a piece lost to a burglar is not bought again, and nothing
        // is bought after `forge_until` (default 70), when the ~70 printer-nights a Mid kit needs to pay back are gone.
        static int ForgeWants(string id)
        {
            if (!ForgeLive || !CashFor || (PlayerDay.Current?.Day ?? 0) > Num("forge_until", 70)) return 0;
            if ((kitBought.TryGetValue(id, out int had) ? had : 0) >= (id == HandmadeChip || id == HeavyChip || id == PartChip ? 1 : ForgePrinters)) return 0;
            int printers = Printer.Printers().Count(Forgeable);
            switch (id)
            {
                // The printer is always in his stock: it waits until both modules and a frame chip are in hand.
                case "printer": return printers < ForgePrinters && Owns(MetalModule) > 0 && Owns(CncModule) > 0 && (Owns(HandmadeChip) > 0 || Owns(HeavyChip) > 0) ? 1 : 0;
                case MetalModule:
                case CncModule: return Owns(id) < ForgePrinters ? 1 : 0;
                case HeavyChip: return Owns(id) == 0 && KeepHeavy > 0 && (PermitHelper.IsGunPermitII() || Num("permit_max", 2) >= 2) ? 1 : 0;
                case HandmadeChip: return Owns(id) == 0 && KeepHandmade > 0 ? 1 : 0;
                case PartChip: return Owns(id) == 0 ? 1 : 0;
            }
            return 0;
        }

        static bool IsFrame(GameItem i) => i != null && i.IsTag(Frame);
        static string FrameGun(GameItem frame) => frame.GetTagReadonly("GUN_FRAME_GUN_ID_STRING").GetString();
        static bool IsNew(GameItem g) => g.FindItemFeatureByCategory("CATEGORY_EQUIPMENT_CONDITION")?.realCondition?.identifier == "equipment_condition_new";

        // A frame-built gun: New, which no seller's gun is (sellers bring Poor to Good).
        static bool Built(GameItem g) => IsGun(g) && IsNew(g) && (g.identifier == "handmade_pistol" || g.identifier == "heavy_handmade_pistol");

        // Guns and frames of a kind on hand: what the next frame print tops up to `keep_*`.
        static int OnHand(string gunId) => Owned().Count(i => Parent(i) == null && Legal(i) && ((IsGun(i) && i.identifier == gunId && IsNew(i)) || (IsFrame(i) && FrameGun(i) == gunId)));

        static readonly string[][] TierGuns = { new string[0], new[] { "printed_gun", "handmade_pistol" }, new[] { "heavy_handmade_pistol", "glock_receiver", "revolver" }, new[] { "smg", "shotgun" } };

        static bool HoldsTier(int tier) => Owned().Any(i => IsGun(i) && Legal(i) && Parent(i) == null && Array.IndexOf(TierGuns[Math.Max(0, Math.Min(3, tier))], i.identifier) >= 0);

        // What a gun given to this order costs: its estimate, except that under Permit II a 10 mm gun is worth a Mid order
        // (300 to 430 bare at Q 0 to 30, lab), which a Basic order never pays.
        static int Cost(GunOrderData o, GameItem g)
        {
            int est = PlayerRole.Est(g);
            if (Forge && o.tier < 2 && PermitHelper.IsGunPermitII() && Array.IndexOf(TierGuns[2], g.identifier) >= 0) return Math.Max(est, Num("mid_gun_value", 300));
            return est;
        }

        // Ingots our own furnace made: their purity follows from what was loaded (MachineFurnace: Low + 2 x ore share +
        // flux), so the player knows it; any other ingot ranks as unknown.
        static readonly HashSet<int> knownIngots = new HashSet<int>();
        static readonly Dictionary<string, int> kitBought = new Dictionary<string, int>(StringComparer.Ordinal);

        public static void KnowIngot(GameItem i) => knownIngots.Add(i.uniqueId);

        public static void NoteIngots()
        {
            foreach (var f in Furnace.Furnaces())
                foreach (var i in GraphUtils.FindAllChildrenType(f, (GameItem x) => x.identifier == Ingot)) knownIngots.Add(i.uniqueId);
        }

        static readonly string[] Purities = { "INGOT_PURITY_VERY_LOW", "INGOT_PURITY_LOW", "INGOT_PURITY_FAIR", "INGOT_PURITY_HIGH", "INGOT_PURITY_VERY_HIGH", "INGOT_PURITY_PERFECT" };
        static readonly int[] PurityBonus = { -30, -20, 0, 20, 30, 100 };

        // MachinePrinter.INGOT_QUALITY_BONUS for a known ingot; null when the player cannot know it.
        static int? Bonus(GameItem ingot)
        {
            if (!knownIngots.Contains(ingot.uniqueId)) return null;
            int k = Array.IndexOf(Purities, IngotPurityHelper.GetPurity(ingot));
            return k < 0 ? (int?)null : PurityBonus[k];
        }

        // Which ingots a chip gets. A frame takes the purest (its bonus is the gun's print quality); a gun-part run the
        // cheapest that still gives 3 parts (Low, Fair); a melee run one High or better and never a Low one
        // (condition = 20-30 + the integer mean bonus: one High in three clears Poor, one Low guarantees it).
        public static List<GameItem> PickIngots(GameItem chip, IEnumerable<GameItem> ingots, int n) =>
            PickIngots(chip.IsTag("PRINTER_FRAME_CHIP_TAG") ? 'f' : chip.IsTag("PRINTER_GUN_PART_CHIP_TAG") ? 'p' : chip.IsTag("PRINTER_MELEE_CHIP_TAG") ? 'm' : '-', ingots, n);

        static List<GameItem> PickIngots(char kind, IEnumerable<GameItem> ingots, int n)
        {
            var l = ingots.ToList();
            int B(GameItem i) => Bonus(i) ?? 0;
            if (kind == 'f') return l.OrderByDescending(B).ThenBy(i => i.uniqueId).Take(n).ToList();
            if (kind == 'p')
                return l.OrderBy(i => Bonus(i) switch { -20 => 0, 0 => 1, null => 2, 20 => 3, -30 => 4, 30 => 5, _ => 6 }).ThenBy(i => i.uniqueId).Take(n).ToList();
            if (kind == 'm')
            {
                var safe = l.Where(i => B(i) >= 0).ToList();
                var lead = safe.Where(i => B(i) >= 20).OrderBy(B).ThenBy(i => i.uniqueId).Take(1).ToList();
                return lead.Concat(safe.Except(lead).OrderBy(B).ThenBy(i => i.uniqueId)).Take(n).ToList();
            }
            return l.OrderBy(i => i.uniqueId).Take(n).ToList();
        }

        // The forge printer's next job: gun parts when the frames on hand would wait for one, then the heavy frame under
        // Permit II, then the handmade frame, each up to its `keep_*`. A frame is printed only when a part can finish it.
        public static IEnumerable<string> ForgeQueue(GameItem printer)
        {
            if (!ForgeLive || !Forgeable(printer)) yield break;
            int parts = Owned().Count(i => i.identifier == Part && Parent(i) == null);
            int frames = Owned().Count(i => IsFrame(i) && Parent(i) == null);
            bool partChip = Owns(PartChip) > 0;
            bool heavy = PermitHelper.IsGunPermitII() && OnHand("heavy_handmade_pistol") < KeepHeavy;
            bool hand = PermitHelper.IsGunPermitI() && OnHand("handmade_pistol") < KeepHandmade;
            if (partChip && parts <= frames && (heavy || hand || frames > 0)) yield return PartChip;
            if (!partChip && parts <= frames) yield break;
            if (heavy) yield return HeavyChip;
            if (hand) yield return HandmadeChip;
        }

        // The kit a forge holds: frames, gun parts (up to 6) and the ingots the next frames and part run take.
        static bool ForgeKit(GameItem i)
        {
            if (IsFrame(i)) return true;
            if (i.identifier == Part) return Owned().Where(m => m.identifier == Part && Parent(m) == null).OrderBy(m => m.uniqueId).Take(6).Any(m => m.uniqueId == i.uniqueId);
            if (i.identifier != Ingot || Parent(i) != null || StolenHelper.IsStolenItem(i) || Owns(HandmadeChip) + Owns(HeavyChip) == 0) return false;
            var loose = Owned().Where(m => m.identifier == Ingot && Parent(m) == null && !StolenHelper.IsStolenItem(m)).ToList();
            var keep = loose.OrderByDescending(m => Bonus(m) ?? 0).ThenBy(m => m.uniqueId).Take(2).ToList();
            if (Owns(PartChip) > 0) keep.AddRange(PickIngots('p', loose.Except(keep), 1));
            return keep.Any(m => m.uniqueId == i.uniqueId);
        }

        // Each morning a gun part goes on every frame (the part's own use on the frame: GunFrameHelper.Finish), giving
        // a New gun at the frame's print quality; it is legal at birth under the permit that covers it.
        static void FinishFrames(IExecutorHost ep)
        {
            foreach (var frame in Owned().Where(i => IsFrame(i) && Parent(i) == null && Legal(i)).OrderBy(i => i.uniqueId).ToList())
            {
                var part = Owned().FirstOrDefault(i => i.identifier == Part && Parent(i) == null && !StolenHelper.IsStolenItem(i));
                if (part == null || part.canThisTargetItemFunc == null || !part.canThisTargetItemFunc(part, frame)) return;
                string gunId = FrameGun(frame);
                int q = frame.GetTagReadonly("GUN_FRAME_PRINT_QUALITY_INT").GetInt(), uid = ep.Uid(frame);
                var before = new HashSet<int>(Owned().Where(i => i.identifier == gunId).Select(i => i.uniqueId));
                Chore.Act("finish_frame", frame, () => part.onThisTargetItemFunc?.Invoke(part, frame));
                var gun = Owned().FirstOrDefault(i => i.identifier == gunId && !before.Contains(i.uniqueId));
                if (gun == null) throw new InvalidOperationException("finishing " + gunId + " frame made no gun");
                if (ForgeDay == 0) ForgeDay = ep.Day;
                Finished++;
                string key = gunId + "_q" + q;
                finishedByQ[key] = (finishedByQ.TryGetValue(key, out int c) ? c : 0) + 1;
                ep.Human("production", null, uid, new JObject { ["action"] = "finish_frame", ["gun"] = gunId, ["print_quality"] = q, ["gun_uid"] = ep.Uid(gun) }, "player:forge");
            }
        }

        // ---- the display case ----

        public static List<GameItem> WindowFirst(IEnumerable<GameItem> owned)
        {
            if (!Opt("window")) return new List<GameItem>();
            // An order client takes one of the day's customer slots: with no legal gun in stock it would only be sent
            // away, so the permits come off display until one is (`window_when_stocked`, default on).
            if ((O.Value<bool?>("window_when_stocked") ?? true) && HeldCount() == 0) return new List<GameItem>();
            // Per tier: a Mid client with only Basic guns in stock is a customer's slot spent on a refusal.
            bool perTier = O.Value<bool?>("window_per_tier") ?? Forge;
            return owned.Where(i => IsPermit(i) && PermitHelper.HavePermit("FIREARM", i.GetTagReadonly(PermitHelper.PERMIT_ITEM_LEVEL).GetInt()))
                .Where(i => !perTier || HoldsTier(i.GetTagReadonly(PermitHelper.PERMIT_ITEM_LEVEL).GetInt()))
                .GroupBy(i => i.identifier).Select(g => g.OrderBy(i => i.uniqueId).First())
                .OrderByDescending(i => i.GetTagReadonly(PermitHelper.PERMIT_ITEM_LEVEL).GetInt()).ToList();
        }

        // A WEAPON case worth 350 from day 14 brings the top weapon buyers (ShowcaseHelper.CreateClientByStockType); guns
        // and mods there still serve orders.
        public static bool Showable(GameItem i) => On && (O.Value<bool?>("show_weapons") ?? false) && (IsGun(i) || IsMod(i)) && Legal(i);

        // A permit is handed over on the counter at the shutter (StoreShutterButton.HandlePermit): into stock.
        public static void Morning(IExecutorHost ep)
        {
            if (!On) return;
            int tier = Tier;
            if (tier >= 2 && Permit2Day == 0) Permit2Day = ep.Day;
            if (tier >= 3 && Permit3Day == 0) Permit3Day = ep.Day;
            if (tier == 0 && PermitLostDay == 0) PermitLostDay = ep.Day;
            if (Environment.GetEnvironmentVariable("FENCE_ARMS_DEBUG") != null)
                Console.Error.WriteLine($"arms d{ep.Day} tier={tier} sec={StoreReputation.GetSecReputation()} permits=" + string.Join(",", Owned().Where(IsPermit).Select(i => i.identifier + "@" + (i.parentInventory == EmporiumEntry.Instance.showcaseElement ? "case" : i.parentInventory == EmporiumEntry.Instance.invElement ? "stock" : i.parentInventory == EmporiumEntry.Instance.frontInvinvElement ? "front" : i.parentInventory == EmporiumEntry.Instance.backInvinvElement ? "back" : "other")))
                    + " guns=" + string.Join(",", Owned().Where(IsGun).Select(i => i.identifier + (Legal(i) ? "" : "!"))));
            var stock = EmporiumEntry.Instance.invElement;
            foreach (var p in EmporiumEntry.Instance.frontInvinvElement.childItems.Where(i => i != null && IsPermit(i) && GeneralHelper.IsItemOwned(i)).ToList())
                Chore.Accept(stock, p, -1);
            if (Forge)
            {
                try { FinishFrames(ep); }
                catch (Exception e) { ep.ProductionError(e); }
            }
        }

        // ---- Wild ----

        static readonly (string id, int gate)[] Permits = { ("GUN_PERMIT_I", 20), ("GUN_PERMIT_II", 50), ("GUN_PERMIT_III", 80) };

        public static IEnumerable<string> Upgrades()
        {
            if (!Opt("permits") || (PlayerDay.Current?.Day ?? 0) < Num("permit_from", 0)) yield break;
            int sec = StoreReputation.GetSecReputation();
            // Permit III by default stops at II (`permit_max`): High orders want an SMG or a shotgun, which nothing
            // the shop makes or reliably meets supplies.
            int max = Num("permit_max", 2);
            for (int t = 0; t < Math.Min(max, Permits.Length); t++)
                if (!NetworkUpgrade.IsUnlocked(Permits[t].id) && sec >= Permits[t].gate - Num("permit_margin", 5)) yield return Permits[t].id;
        }

        public static bool WantsUpgrade(string id) => Upgrades().Contains(id);

        public static int UpgradeReserve => Num("permit_reserve", 300);

        // ---- gun orders (GunOrderData, NegociationUIManager.OnOfferAcceptClick) ----

        public sealed class Plan
        {
            public GameItem Gun;
            public int Reward, Given;
            public bool Service;
            public string Why;
        }

        static string[] Stats = { "GUN_CURRENT_FIREPOWER_TAG", "GUN_CURRENT_STYLE_TAG", "GUN_CURRENT_RELIABILITY_TAG", "GUN_CURRENT_RECOIL_TAG", "GUN_CURRENT_ERGONOMICS_TAG", "GUN_CURRENT_ACCURACY_TAG" };

        static List<GameSlotInventory> Slots(GameItem gun) => GraphUtils.FindChildrenType<GameSlotInventory>(gun).ToList();

        static bool Attach(GameItem gun, GameItem mod)
        {
            foreach (var slot in Slots(gun))
            {
                if (slot.childItem != null) continue;
                if (Chore.Accept(slot, mod, -1) == 0 && ReferenceEquals(mod.parentInventory, slot))
                {
                    Chore.Act("gun_stats", gun, () => GunHelper.UpdateCurrentGunStats(gun));
                    return true;
                }
            }
            return false;
        }

        static void Detach(GameItem gun, GameItem mod)
        {
            Chore.Accept(EmporiumEntry.Instance.invElement, mod, -1);
            Chore.Act("gun_stats", gun, () => GunHelper.UpdateCurrentGunStats(gun));
        }

        // How close a gun is to the order: met mandatory rows first, then progress on the unmet ones, then the bonus
        // cash it would collect. All of it is on the order tablet.
        static double Score(GunOrderData o, GameItem gun)
        {
            double s = 0;
            foreach (var r in o.mandatoryRequirements)
            {
                if (r.IsMet(gun)) { s += 1000; continue; }
                if (r.isCaliber) continue;
                int v = gun.GetTagReadonly(r.id).GetInt();
                s += r.isMustBeLargerThan ? Math.Min(v, r.value + 1) * 10.0 / Math.Max(1, r.value + 1) : 0;
            }
            return s + o.bonusRequirements.Where(r => r.IsMet(gun)).Sum(r => r.GetReward());
        }

        static bool Passes(GunOrderData o, GameItem gun) => o.mandatoryRequirements.All(r => r.IsMet(gun));

        static int Reward(GunOrderData o, GameItem gun) => o.GetTotalReward(gun);

        // Greedy: the loose mod that raises the score most, net of what the mod is worth (it goes with the gun; on a
        // service gun it is paid at its value), until none helps. A best order pays every mod at 1.5 times its value.
        static List<GameItem> Build(GunOrderData o, GameItem gun, List<GameItem> loose)
        {
            var added = new List<GameItem>();
            if (!Opt("mods")) return added;
            for (int round = 0; round < 8; round++)
            {
                double base0 = Score(o, gun);
                GameItem best = null;
                double gain = 0;
                foreach (var m in loose.Where(m => !added.Contains(m) && m.parentInventory != null && Parent(m) == null).GroupBy(m => m.identifier).Select(g => g.First()).ToList())
                {
                    if (!Attach(gun, m)) continue;
                    // Under Permit II a Mid mod is worth more to a Mid order: a Basic one must pay it twice.
                    double cost = o.isServiceOrder ? 0 : PlayerRole.Est(m) * (o.tier < 2 && MidMod(m.identifier) ? 2 : 1);
                    double g = o.isBestOrder ? (Passes(o, gun) ? PlayerRole.Est(m) * 0.5 : 0) : Score(o, gun) - base0 - cost;
                    Detach(gun, m);
                    if (g > gain) { gain = g; best = m; }
                }
                if (best == null || !Attach(gun, best)) break;
                added.Add(best);
            }
            return added;
        }

        static void Undo(GameItem gun, List<GameItem> added)
        {
            foreach (var m in added) Detach(gun, m);
        }

        // A repair step raises every stat by 15 % of base; worth up to `repair_parts` (default 2) parts when the gun
        // misses only stat rows.
        static bool Repair(GunOrderData o, GameItem gun)
        {
            // The forge's parts finish frames; on a service gun a repair is paid, or its part comes back when declined.
            int budget = Forge && !GunHelper.IsServiceGun(gun) ? 0 : Num("repair_parts", 2);
            bool any = false;
            while (!Passes(o, gun) && budget-- > 0 && GunHelper.CanRepair(gun) && o.mandatoryRequirements.Where(r => !r.IsMet(gun)).All(r => !r.isCaliber && r.isMustBeLargerThan))
            {
                var part = Owned().FirstOrDefault(i => i.identifier == "gun_part" && Parent(i) == null);
                if (part == null || part.canThisTargetItemFunc == null || !part.canThisTargetItemFunc(part, gun)) break;
                Chore.Act("repair", gun, () => part.onThisTargetItemFunc?.Invoke(part, gun));
                PartsUsed++;
                any = true;
            }
            if (any) Repairs++;
            return Passes(o, gun);
        }

        public static Plan Prepare(StoreClient c)
        {
            var o = c?.gunOrderData;
            if (!Opt("orders") || o == null) return null;
            Offered[Math.Max(0, Math.Min(3, o.tier))]++;
            var loose = Owned().Where(i => IsMod(i) && Parent(i) == null && Legal(i)).OrderBy(i => i.uniqueId).ToList();
            double minPct = Num("min_pct", 100) / 100.0;
            if (o.isServiceOrder)
            {
                var sg = o.GetPlacedServiceGun();
                if (sg == null) return new Plan { Why = "service gun not placed" };
                var added = Build(o, sg, loose);
                if (!Passes(o, sg)) Repair(o, sg);
                int given = added.Sum(PlayerRole.Est);
                if (Passes(o, sg)) return new Plan { Gun = sg, Service = true, Reward = Reward(o, sg), Given = given };
                Undo(sg, added);
                return new Plan { Why = "service thresholds out of reach" };
            }
            Plan best = null;
            GameItem near = null;
            foreach (var g in Owned().Where(i => IsGun(i) && Legal(i) && Parent(i) == null).OrderBy(i => i.uniqueId).ToList())
            {
                var added = Build(o, g, loose);
                if (Passes(o, g))
                {
                    int r = Reward(o, g), given = Cost(o, g);
                    if (r >= given * minPct && (best == null || r - given > best.Reward - best.Given)) best = new Plan { Gun = g, Reward = r, Given = given };
                }
                else if (near == null && o.mandatoryRequirements.Where(q => !q.IsMet(g)).All(q => !q.isCaliber && q.isMustBeLargerThan && g.GetTagReadonly(q.id).GetInt() * 100 >= q.value * 85)) near = g;
                Undo(g, added);
            }
            if (best == null && near != null && Repair(o, near))
            {
                int r = Reward(o, near), given = PlayerRole.Est(near);
                if (r >= given * minPct) best = new Plan { Gun = near, Reward = r, Given = given };
            }
            if (best == null && Environment.GetEnvironmentVariable("FENCE_ARMS_DEBUG") != null)
                Console.Error.WriteLine($"arms refuse tier={o.tier} best={o.isBestOrder} mand=" + string.Join(",", o.mandatoryRequirements.Select(r => r.id.Replace("GUN_CURRENT_", "").Replace("_TAG", "") + ">" + r.value))
                    + " guns=" + string.Join(",", Owned().Where(i => IsGun(i) && Legal(i) && Parent(i) == null).Select(i => i.identifier)) + " mods=" + string.Join(",", loose.Select(m => m.identifier)));
            if (best == null) return new Plan { Why = "no legal gun clears the order at a profit" };
            Build(o, best.Gun, loose);
            best.Reward = Reward(o, best.Gun);
            best.Given = PlayerRole.Est(best.Gun);
            return best;
        }

        public static void Delivered(StoreClient c, Plan p, int cash)
        {
            int tier = Math.Max(0, Math.Min(3, c.gunOrderData.tier));
            if (!p.Service && p.Gun != null && Built(p.Gun))
            {
                ForgeOrders++;
                ForgeIncome += cash;
            }
            Orders++;
            ByTier[tier]++;
            IncomeByTier[tier] += cash;
            OrderIncome += cash;
            GivenValue += p.Given;
            if (p.Service) Service++;
            if (c.gunOrderData.isBestOrder) Best++;
        }

        public static void Refused(string why)
        {
            Sent++;
            refusals[why] = (refusals.TryGetValue(why, out int n) ? n : 0) + 1;
        }

        public static void Trade(bool sold, GameItem item, int amount)
        {
            if (On && sold && item != null && (IsGun(item) || IsMod(item) || PlayerRole.TypeIs(item, new[] { "WEAPON", "AMMUNITION" }))) ArmsSales += amount;
            if (Forge && !sold && item != null && Array.IndexOf(ForgeKitIds, item.identifier) >= 0)
            {
                ForgeCapital += amount;
                kitBought[item.identifier] = (kitBought.TryGetValue(item.identifier, out int n) ? n : 0) + 1;
            }
        }

        public static JObject Status()
        {
            if (!On) return null;
            return new JObject
            {
                ["tier"] = Tier, ["security"] = StoreReputation.GetSecReputation(), ["permit2_day"] = Permit2Day, ["permit3_day"] = Permit3Day, ["permit_lost_day"] = PermitLostDay,
                ["orders"] = Orders, ["order_income"] = OrderIncome, ["given_value"] = GivenValue, ["service"] = Service, ["best"] = Best,
                ["by_tier"] = new JArray(ByTier.Skip(1)), ["income_by_tier"] = new JArray(IncomeByTier.Skip(1)), ["offered_by_tier"] = new JArray(Offered.Skip(1)),
                ["sent_away"] = Sent, ["refusals"] = JObject.FromObject(refusals), ["parts_used"] = PartsUsed, ["repairs"] = Repairs, ["arms_sales"] = ArmsSales,
                ["guns_held"] = Owned().Count(i => IsGun(i) && Legal(i) && Parent(i) == null),
                ["forge"] = !Forge ? null : new JObject
                {
                    ["first_day"] = ForgeDay, ["finished"] = Finished, ["finished_by_gun_q"] = JObject.FromObject(finishedByQ), ["capital"] = ForgeCapital,
                    ["orders"] = ForgeOrders, ["income"] = ForgeIncome, ["kit"] = new JArray(Owned().Where(i => Array.IndexOf(ForgeKitIds, i.identifier) >= 0).Select(i => i.identifier).OrderBy(x => x, StringComparer.Ordinal)),
                },
            };
        }
    }
}
