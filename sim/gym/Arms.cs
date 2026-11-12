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
    // Weapons above the permit stay contraband, which the executor neither buys nor sells.
    internal static class Arms
    {
        static JObject O => Directives.Current.Raw["arms"] as JObject;
        public static bool On => O != null && (O.Value<bool?>("on") ?? true);
        static bool Opt(string k) => On && (O.Value<bool?>(k) ?? true);
        static int Num(string k, int d) => O?.Value<int?>(k) ?? d;

        public static int KeepGuns => Num("keep_guns", 4);

        public static int Orders, OrderIncome, GivenValue, Service, Best, Sent, PartsUsed, Repairs, ArmsSales;
        public static int Permit2Day, Permit3Day, PermitLostDay;
        public static readonly int[] ByTier = new int[4], IncomeByTier = new int[4], Offered = new int[4];
        static readonly SortedDictionary<string, int> refusals = new SortedDictionary<string, int>(StringComparer.Ordinal);

        public static void Reset()
        {
            Orders = OrderIncome = GivenValue = Service = Best = Sent = PartsUsed = Repairs = ArmsSales = 0;
            Permit2Day = Permit3Day = PermitLostDay = 0;
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

        public static bool IsInput(string id) => Opt("orders") && id != null && (Array.IndexOf(GunIds, id) >= 0 || (Opt("buy_mods") && MidMod(id)));

        // Guns are bought (legal ones only reach the counter's buy path) while fewer than `keep_guns` are held; under
        // Permit II, two of each Mid mod (`buy_mods`).
        public static int Wants(string id)
        {
            if (!IsInput(id)) return 0;
            if (Array.IndexOf(GunIds, id) >= 0) return Math.Max(0, KeepGuns - HeldCount());
            return Math.Max(0, 2 - Owned().Count(i => i.identifier == id && Parent(i) == null));
        }

        public static bool PrintGuns => Opt("print_guns") && PermitHelper.IsGunPermitI() && HeldCount() < KeepGuns;

        public static readonly string[] GunChips = { "printer_chip_pistol_double", "printer_chip_pistol" };

        // ---- the display case ----

        public static List<GameItem> WindowFirst(IEnumerable<GameItem> owned)
        {
            if (!Opt("window")) return new List<GameItem>();
            // An order client takes one of the day's customer slots: with no legal gun in stock it would only be sent
            // away, so the permits come off display until one is (`window_when_stocked`, default on).
            if ((O.Value<bool?>("window_when_stocked") ?? true) && HeldCount() == 0) return new List<GameItem>();
            return owned.Where(i => IsPermit(i) && PermitHelper.HavePermit("FIREARM", i.GetTagReadonly(PermitHelper.PERMIT_ITEM_LEVEL).GetInt()))
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
            int budget = Num("repair_parts", 2);
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
                    int r = Reward(o, g), given = PlayerRole.Est(g);
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
            };
        }
    }
}
