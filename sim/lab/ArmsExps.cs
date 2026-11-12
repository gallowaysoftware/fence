using System;
using System.Collections.Generic;
using System.Linq;
using Fence.Gym;
using Fence.Sim;

namespace Fence.Lab
{
    // docs/re/ARMS.md: weapon permits, gun orders, gun mods and the Gunsmith start's printer, on hard.
    public static class ArmsExps
    {
        static EmporiumEntry E => W.E;

        // DirectoryMaster.Item, as the game makes items: it sets the identifier and applies the permits held.
        static GameItem Owned(string id)
        {
            var it = DirectoryMaster.Item(id);
            GeneralHelper.SetItemOwned(it, true);
            return it;
        }

        static GameItem Own(string id)
        {
            var it = Owned(id);
            GraphUtils.TryAcceptAll(E.invElement, it, -1);
            return it;
        }

        static int Level(GameItem i) => ContrabandHelper.GetContrabandLevelInInt(i);
        static int Level(string id) => Level(DirectoryMaster.Item(id));

        static void SetSec(double v) => W.SetRep("FACTION_SECURITY", v);

        // ARMS.md §2: each permit covers its own contraband level only, for guns, frames and ammo made after it.
        public static void Permits(int seed)
        {
            W.New(seed, "hard", true, "gunsmith");
            R.Exact("gunsmith start: Permit I active", true, PermitHelper.IsGunPermitI());
            R.Exact("permit I: a new printed gun is legal (level 0)", 0, Level("printed_gun"));
            R.Exact("permit I: a new printed gun is RESTRICTED_1", true, DirectoryMaster.Item("printed_gun").IsGameItemType("RESTRICTED_1"));
            R.Exact("permit I: a new handmade pistol frame is legal", 0, Level("handmade_pistol_frame"));
            R.Exact("permit I: lethal .22 ammo is legal", 0, Level("small_pistol_ammo"));
            R.Exact("permit I: a heavy handmade pistol stays contraband 2", 2, Level("heavy_handmade_pistol"));
            R.Exact("permit I: armour-piercing .22 stays contraband 2", 2, Level("small_pistol_ammo_p"));
            R.Exact("permit I: an SMG stays contraband 3", 3, Level("smg"));

            PermitHelper.GrantedFromPermit(W.Make("permit_gun_2"));
            R.Exact("permit II: a new heavy handmade pistol is legal", 0, Level("heavy_handmade_pistol"));
            R.Exact("permit II: a new glock is legal", 0, Level("glock_receiver"));
            R.Exact("permit II: an SMG stays contraband 3", 3, Level("smg"));

            var kitGun = Own("printed_gun");
            PermitHelper.OnPermitRevoked(1, "FIREARM");
            R.Exact("permit I revoked, II held: a new printed gun is contraband 1", 1, Level("printed_gun"));
            R.Exact("permit I revoked: an owned restricted printed gun turns contraband 1", 1, Level(kitGun));
            R.Exact("permit I revoked: new lethal .22 ammo stays legal (only FIREARM is revoked)", 0, Level("small_pistol_ammo"));

            // The shutter's grant: the network upgrade and Security at the gate.
            E.frontInvinvElement.childItems.Where(i => i != null).ToList().ForEach(i => E.frontInvinvElement.Expel(i));
            W.PS.networkUpgrade["GUN_PERMIT_III"].state = 2;
            SetSec(79);
            StoreShutterButton.Instance.HandlePermit();
            R.Exact("permit III upgrade owned, Security 79: not granted", false, PermitHelper.IsGunPermitIII());
            R.Exact("permit I upgrade owned, Security 79: re-granted at the shutter", true, PermitHelper.IsGunPermitI());
            SetSec(80);
            StoreShutterButton.Instance.HandlePermit();
            R.Exact("permit III upgrade owned, Security 80: granted", true, PermitHelper.IsGunPermitIII());
            R.Exact("a granted permit is handed over on the front counter", true, E.frontInvinvElement.childItems.Any(i => i != null && i.identifier == "permit_gun_3" && GeneralHelper.IsItemOwned(i)));
            R.Exact("permit III: a new SMG is legal", 0, Level("smg"));
            R.Info("permit upgrade costs on hard (I, II, III)", string.Join(", ", new[] { "GUN_PERMIT_I", "GUN_PERMIT_II", "GUN_PERMIT_III", "RETIRED_GUNSMITH" }.Select(id => id + " " + W.PS.networkUpgrade[id].GetCost())));
        }

        static bool Attach(GameItem gun, GameItem mod)
        {
            foreach (var slot in GraphUtils.FindChildrenType<GameSlotInventory>(gun).ToList())
                if (slot.childItem == null && GraphUtils.TryAcceptAll(slot, mod, -1) == 0 && ReferenceEquals(mod.parentInventory, slot))
                {
                    GunHelper.UpdateCurrentGunStats(gun);
                    return true;
                }
            return false;
        }

        static int FP(GameItem g) => g.GetTagReadonly("GUN_CURRENT_FIREPOWER_TAG").GetInt();

        // ARMS.md §3: a mod adds its own value to the gun's and its percentages and flat points to the stats.
        public static void Mods(int seed)
        {
            W.New(seed, "hard", true, "gunsmith");
            var gun = Own("printed_gun");
            int v0 = PlayerRole.Est(gun), fp0 = FP(gun);
            R.Exact("printed gun, New: firepower 15 x 130 %", 19, fp0);
            var mag = Own("mag_22_ext");
            int mv = PlayerRole.Est(mag);
            R.Exact("an owned .22 extended mag fits a printed gun", true, Attach(gun, mag));
            R.Exact("printed gun + .22 extended mag: value is the sum", v0 + mv, PlayerRole.Est(gun));
            R.Exact("printed gun + .22 extended mag: firepower 15 x 190 % + 18", 46, FP(gun));
            var ten = Own("mag_10mm");
            R.Exact("a 10mm mag does not fit a .22 printed gun", false, Attach(gun, ten));
            var springs = Own("internal_springs_improved");
            var springs2 = Own("internal_springs");
            R.Exact("improved springs fit", true, Attach(gun, springs));
            R.Exact("plain springs are refused next to improved springs", false, Attach(gun, springs2));
            R.Info("mod values (silencer, rds, laser, mag_22_ext, internal_springs_improved, internal_springs)", string.Join(", ", new[] { "silencer", "rds", "laser", "mag_22_ext", "internal_springs_improved", "internal_springs" }.Select(id => id + " " + PlayerRole.Est(W.Make(id)))));
        }

        static void Seat(StoreClient c)
        {
            c.spriteName ??= "";
            c.possibleSprites ??= new List<string>();
            W.PS.currentClientInstance = StoreClientInstance.CreateClientInstance(c);
            W.PS.isClientArrived = true;
        }

        static int Deliver(GunOrderData order, GameItem gun, out bool destroyed)
        {
            var c = StoreClientListGun.CreateLLGunBuyer(order);
            Seat(c);
            GraphUtils.TryAcceptAll(E.backInvinvElement, gun, -1);
            int cash0 = W.Cash;
            NegociationUIManager.Instance.OnOfferAcceptClick();
            Scheduler.Settle();
            destroyed = !E.backInvinvElement.childItems.Contains(gun);
            int paid = W.Cash - cash0;
            W.PS.DismissCurrentClient(false, true, true);
            Scheduler.Settle();
            foreach (var i in E.backInvinvElement.childItems.Where(i => i != null).ToList()) E.backInvinvElement.Expel(i);
            return paid;
        }

        // ARMS.md §4: an order pays its base and every bonus row the gun clears (strictly greater), takes the gun and its
        // mods; a best order pays the gun's market value with its mods, plus half.
        public static void Orders(int seed)
        {
            W.New(seed, "hard", true, "gunsmith");
            int n = 20, ok = 0, gone = 0, bestOk = 0;
            double sec0 = StoreReputation.GetSecReputation(), ll0 = W.Rep("FACTION_LOWER_LEVEL");
            for (int k = 0; k < n; k++)
            {
                var order = GunOrderData.CreateBasicOrderFirepower();
                var gun = Own("printed_gun");
                // FP 19 clears the mandatory > 11 and the bonus rows > 14 and > 17, not > 20 or > 22.
                int expected = order.GetBaseReward() + order.bonusRequirements.Where(r => r.value < 19).Sum(r => r.GetReward());
                if (Deliver(order, gun, out bool d) == expected) ok++;
                if (d) gone++;
            }
            R.Rate("basic firepower order pays base + bonus rows cleared (printed gun, FP 19)", 1.0, ok, n);
            R.Rate("a delivered gun is destroyed", 1.0, gone, n);
            R.Info("Security / Lower Levels change over 20 deliveries", $"{StoreReputation.GetSecReputation() - sec0:0.0} / {W.Rep("FACTION_LOWER_LEVEL") - ll0:0.0}", n);
            for (int k = 0; k < n; k++)
            {
                var gun = Own("printed_gun");
                Attach(gun, Own("mag_22_ext"));
                Attach(gun, Own("rds"));
                int market = (int)gun.GetCurrentValue(false, true, true, true, 0L);
                int expected = market + (int)Math.Round(market * 0.5, MidpointRounding.AwayFromZero);
                int est = PlayerRole.Est(gun);
                int paid = Deliver(GunOrderData.CreateBestOrder(1, 22), gun, out _);
                if (Math.Abs(paid - expected) <= 1) bestOk++;
                if (k == 0) R.Info("best .22 order: printed gun + extended mag + red dot, estimate / paid", est + " / " + paid);
            }
            R.Rate("best order pays 1.5 x the market value with mods", 1.0, bestOk, n);
            var hm = Own("handmade_pistol");
            R.Exact("a mid firepower order (> 26) refuses a plain handmade pistol", 0, Deliver(GunOrderData.CreateMidOrderFirepower(), hm, out _));
        }

        // ARMS.md §6: the Gunsmith start's old printer and what it prints under Permit I.
        public static void Printer(int seed)
        {
            W.New(seed, "hard", true, "gunsmith");
            var p = E.GetAllOwnedItems(false, true).FirstOrDefault(i => i != null && i.IsTag("PRINTER_TAG"));
            R.Exact("the Gunsmith start owns a 3D printer", true, p != null);
            if (p == null) return;
            R.Exact("old printer: power per working night", 7, MachineryHelper.GetMachinePowerUsage(p));
            var chip = (MachineHelper.GetBodyStorage(p, 3) as GameSlotInventory)?.childItem;
            R.Exact("old printer: a pistol chip in its slot", "printer_chip_pistol", chip?.identifier);
            var input = MachineHelper.GetBodyStorage(p, 2) as GameGridInventory;
            foreach (var i in input.childItems.Where(i => i != null).ToList()) input.Expel(i);
            for (int k = 0; k < 2; k++) GraphUtils.TryAcceptAll(input, Owned("printer_plastic"), -1);
            int nights = 0;
            var output = MachineHelper.GetBodyStorage(p, 4) as GameGridInventory;
            while (nights < 6 && !output.childItems.Any(i => i != null && i.identifier == "printed_gun")) { W.QuietDay(); nights++; }
            var gun = output.childItems.FirstOrDefault(i => i != null && i.identifier == "printed_gun");
            R.Exact("old printer: nights for a printed pistol (work 50 at 24 a night)", 3, nights);
            R.Exact("a printed pistol under Permit I is legal", 0, gun == null ? -1 : Level(gun));
            R.Info("printed pistol condition / estimate / firepower", gun == null ? "none" : gun.FindItemFeatureByCategory("CATEGORY_EQUIPMENT_CONDITION")?.realCondition?.identifier + " / " + PlayerRole.Est(gun) + " / " + FP(gun));
        }
    }
}
