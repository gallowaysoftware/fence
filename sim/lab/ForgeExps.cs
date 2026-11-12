using System;
using System.Collections.Generic;
using System.Linq;
using Fence.Gym;
using Fence.Sim;

namespace Fence.Lab
{
    // The frame forge (docs/re/ARMS.md §6, docs/research/FORMULA-EXPLOITS.md §2): what the printer makes from frame
    // and gun-part chips on its real nights, what the frames finish into, and what those guns are paid by orders.
    public static class ForgeExps
    {
        static readonly string[] Purities = { "INGOT_PURITY_VERY_LOW", "INGOT_PURITY_LOW", "INGOT_PURITY_FAIR", "INGOT_PURITY_HIGH", "INGOT_PURITY_VERY_HIGH", "INGOT_PURITY_PERFECT" };

        static GameItem Owned(GameItem it)
        {
            GeneralHelper.SetItemOwned(it, true);
            return it;
        }

        static GameItem Owned(string id) => Owned(DirectoryMaster.Item(id));

        static GameItem Ingot(string purity) => Owned(PreBuiltItemHelper.RealIngotPurity(purity));

        static GameItem OldPrinter() => W.E.GetAllOwnedItems(false, true).First(i => i != null && i.IsTag("PRINTER_TAG"));

        static GameItem NewPrinter()
        {
            var p = Owned(PreBuiltItemHelper.CreatePrinter());
            W.Place(W.E.invElement, p);
            return p;
        }

        static GameGridInventory Bay(GameItem p) => (GameGridInventory)MachineHelper.GetBodyStorage(p, 1);
        static GameGridInventory Input(GameItem p) => (GameGridInventory)MachineHelper.GetBodyStorage(p, 2);
        static GameSlotInventory Chip(GameItem p) => (GameSlotInventory)MachineHelper.GetBodyStorage(p, 3);
        static GameGridInventory Output(GameItem p) => (GameGridInventory)MachineHelper.GetBodyStorage(p, 4);

        static void Clear(GameInventory inv)
        {
            foreach (var i in GraphUtils.FindAllChildrenType(inv, (GameItem _) => true).Where(i => i.parentInventory == inv).ToList()) inv.Expel(i);
        }

        static void Power(GameItem p)
        {
            var bat = MachineHelper.GetBatterySlot(p);
            if (bat.childItem != null) bat.Expel(bat.childItem);
            GraphUtils.TryAcceptAll(bat, Owned(PreBuiltItemHelper.EnergyCredit(100)), -1);
        }

        static string Fit(GameItem p, params string[] modules)
        {
            var r = new List<string>();
            foreach (var m in modules)
                r.Add(m + (GraphUtils.TryAcceptAll(Bay(p), Owned(m), -1) == 0 ? "" : " REFUSED"));
            MachineryHelper.UpdatePowerUsageProcessTypeMachine(p);
            return string.Join(", ", r);
        }

        // One print on the machine's own nights: chip in, inputs in, quiet days until the output grid fills.
        static (int nights, List<GameItem> made) Print(GameItem p, string chip, params GameItem[] inputs)
        {
            Clear(Output(p));
            Clear(Input(p));
            if (Chip(p).childItem != null) Chip(p).Expel(Chip(p).childItem);
            GraphUtils.TryAcceptAll(Chip(p), Owned(chip), -1);
            foreach (var i in inputs) GraphUtils.TryAcceptAll(Input(p), i, -1);
            Power(p);
            int nights = 0;
            while (nights < 12 && !Output(p).childItems.Any(i => i != null))
            {
                Power(p);
                W.QuietDay();
                nights++;
            }
            return (nights, Output(p).childItems.Where(i => i != null).ToList());
        }

        static string Stats(GameItem g) => string.Join("/", new[] { "FIREPOWER", "STYLE", "RELIABILITY", "RECOIL", "ERGONOMICS", "ACCURACY" }
            .Select(t => g.GetTagReadonly($"GUN_CURRENT_{t}_TAG").GetInt()));

        static GameItem Finish(GameItem frame)
        {
            if (frame.parentInventory != W.E.invElement) { (frame.parentInventory as GameInventory)?.Expel(frame); W.Place(W.E.invElement, frame); }
            string gunId = frame.GetTagReadonly("GUN_FRAME_GUN_ID_STRING").GetString();
            var before = W.E.GetAllOwnedItems(false, true).Where(i => i != null).Select(i => i.uniqueId).ToHashSet();
            var part = Owned("gun_part");
            W.Place(W.E.invElement, part);
            GunFrameHelper.Finish(frame, part);
            var gun = W.E.GetAllOwnedItems(false, true).First(i => i != null && i.identifier == gunId && !before.Contains(i.uniqueId));
            GunHelper.UpdateCurrentGunStats(gun);
            return gun;
        }

        static int Q(GameItem frame) => frame.GetTagReadonly("GUN_FRAME_PRINT_QUALITY_INT").GetInt();

        // The printer the Gunsmith start owns and a new one from the retired gunsmith, each with the metal and CNC
        // modules: power, nights per frame and per gun-part run, frame quality by ingot purity, gun stats.
        public static void Printers(int seed)
        {
            W.New(seed, "hard", true, "gunsmith");
            PermitHelper.GrantedFromPermit(W.Make("permit_gun_2"));
            W.Cash = 100000;
            var oldBay = OldPrinter();
            var fresh = NewPrinter();
            R.Info("old printer (two stuck modules): fit metal, then CNC", Fit(oldBay, "printer_module_metal", "printer_module_cnc"));
            foreach (var m in Bay(oldBay).childItems.Where(i => i != null && i.identifier.StartsWith("printer_module")).ToList()) Bay(oldBay).Expel(m);
            R.Info("old printer (two stuck modules): fit CNC, then metal", Fit(oldBay, "printer_module_cnc", "printer_module_metal"));
            foreach (var (label, p) in new[] { ("new printer", fresh) })
            {
                R.Info($"{label}: power a night bare", MachineryHelper.GetMachinePowerUsage(p));
                R.Info($"{label}: fit metal + CNC", Fit(p, "printer_module_metal", "printer_module_cnc"));
                R.Info($"{label}: power a night with metal + CNC / adjusted quality", MachineryHelper.GetMachinePowerUsage(p) + " / " + ModuleHelper.GetAjustedQuality(p));
                var (n, made) = Print(p, "printer_chip_heavy_handmade_pistol_frame", Ingot("INGOT_PURITY_FAIR"), Owned("scrap_metal"));
                var f = made.FirstOrDefault();
                R.Info($"{label}: heavy frame (Fair ingot + scrap): nights / output / Q / contraband / estimate",
                    $"{n} / {string.Join(",", made.Select(i => i.identifier))} / {(f == null ? -1 : Q(f))} / {(f == null ? -1 : ContrabandHelper.GetContrabandLevelInInt(f))} / {(f == null ? -1 : PlayerRole.Est(f))}");
                (n, made) = Print(p, "printer_chip_handmade_pistol_frame", Ingot("INGOT_PURITY_FAIR"));
                f = made.FirstOrDefault();
                R.Info($"{label}: handmade frame (Fair ingot): nights / output / Q / contraband",
                    $"{n} / {string.Join(",", made.Select(i => i.identifier))} / {(f == null ? -1 : Q(f))} / {(f == null ? -1 : ContrabandHelper.GetContrabandLevelInInt(f))}");
                foreach (var pur in new[] { "INGOT_PURITY_FAIR", "INGOT_PURITY_VERY_HIGH", "INGOT_PURITY_PERFECT" })
                {
                    (n, made) = Print(p, "printer_chip_gun_part", Ingot(pur));
                    R.Info($"{label}: gun-part run ({pur}): nights / parts", $"{n} / {made.Count(i => i.identifier == "gun_part")}");
                }
            }
            var old = fresh;
            foreach (var pur in Purities)
            {
                var (n, made) = Print(old, "printer_chip_heavy_handmade_pistol_frame", Ingot(pur), Owned("scrap_metal"));
                var f = made.First();
                int q = Q(f);
                var g = Finish(f);
                R.Info($"heavy frame, {pur} ingot: Q / gun FP/STY/REL/RCL/ERG/ACC / estimate", $"{q} / {Stats(g)} / {PlayerRole.Est(g)}");
                (g.parentInventory as GameInventory)?.Expel(g);
                (n, made) = Print(old, "printer_chip_handmade_pistol_frame", Ingot(pur));
                f = made.First();
                q = Q(f);
                g = Finish(f);
                R.Info($"handmade frame, {pur} ingot: Q / gun FP/STY/REL/RCL/ERG/ACC / estimate", $"{q} / {Stats(g)} / {PlayerRole.Est(g)}");
                (g.parentInventory as GameInventory)?.Expel(g);
            }
            // A quality module beside the metal and CNC modules: what the bay still takes and what it adds.
            var q1 = Owned(PreBuiltItemHelper.RandomQualityModule());
            bool fits = GraphUtils.TryAcceptAll(Bay(old), q1, -1) == 0;
            MachineryHelper.UpdatePowerUsageProcessTypeMachine(old);
            R.Info("new printer + metal + CNC: a random quality module fits / its Q bonus / adjusted quality / power",
                $"{fits} / {q1.GetTagReadonly("BONUS_PERCENTAGE_QUALITY_INT").GetInt()} / {ModuleHelper.GetAjustedQuality(old)} / {MachineryHelper.GetMachinePowerUsage(old)}");
        }

        static GameItem Framed(string frameId, int quality, params string[] mods)
        {
            var frame = W.Own(frameId);
            GunFrameHelper.SetPrintQuality(frame, quality);
            var g = Finish(frame);
            foreach (var m in mods)
            {
                var mod = W.Own(m);
                foreach (var slot in GraphUtils.FindChildrenType<GameSlotInventory>(g).ToList())
                    if (slot.childItem == null && GraphUtils.TryAcceptAll(slot, mod, -1) == 0 && ReferenceEquals(mod.parentInventory, slot)) break;
            }
            GunHelper.UpdateCurrentGunStats(g);
            return g;
        }

        static int Paid(GunOrderData order, GameItem gun)
        {
            var c = order.tier >= 3 ? StoreClientListGun.CreateULBuyer(order) : order.tier == 2 ? StoreClientListGun.CreateMercGunBuyer(order) : StoreClientListGun.CreateLLGunBuyer(order);
            c.spriteName ??= ""; c.possibleSprites ??= new List<string>();
            W.PS.currentClientInstance = StoreClientInstance.CreateClientInstance(c);
            W.PS.isClientArrived = true;
            (gun.parentInventory as GameInventory)?.Expel(gun);
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
            int met = 0; long paid = 0; string stats = null;
            for (int k = 0; k < n; k++)
            {
                var o = make();
                var g = build();
                stats ??= Stats(g) + " est " + PlayerRole.Est(g);
                if (o.mandatoryRequirements.All(r => r.IsMet(g))) met++;
                paid += Paid(o, g);
            }
            R.Info($"order {label}", $"met {met}/{n}; mean paid {(double)paid / n:0}", n, stats);
        }

        static readonly (string name, Func<GunOrderData> make)[] Basic =
        {
            ("accurate", GunOrderData.CreateBasicOrder), ("firepower", GunOrderData.CreateBasicOrderFirepower), ("big", GunOrderData.CreateBasicOrderBig),
            ("suppressed", GunOrderData.CreateBasicOrderSuppressed), ("stylish", GunOrderData.CreateBasicOrderStylish), ("general", GunOrderData.CreateBasicOrderGeneral),
            ("handling", GunOrderData.CreateBasicOrderHandling), ("best .22", () => GunOrderData.CreateBestOrder(1, 22)),
        };

        static readonly (string name, Func<GunOrderData> make)[] Mid =
        {
            ("suppressed", GunOrderData.CreateMidOrderSuppressed), ("stylish", GunOrderData.CreateMidOrderStylish), ("big", GunOrderData.CreateMidOrderBig),
            ("firepower", GunOrderData.CreateMidOrderFirepower), ("general", GunOrderData.CreateMidOrderGeneral), ("handling", GunOrderData.CreateMidOrderHandling),
            ("best 10 mm", () => GunOrderData.CreateBestOrder(2, 10)),
        };

        static readonly (string name, Func<GunOrderData> make)[] High =
        {
            ("firepower", GunOrderData.CreateHighOrderFirepower), ("style", GunOrderData.CreateHighOrderStyle), ("reliability", GunOrderData.CreateHighOrderReliability),
            ("handling", GunOrderData.CreateHighOrderHandling), ("general", GunOrderData.CreateHighOrderGeneral), ("best 10 mm", () => GunOrderData.CreateBestOrder(3, 10)),
        };

        // Every Basic, Mid and High order kind against the frame-built guns the forge makes: what a bare gun is paid
        // at each print quality the furnace's ingots give (Fair 0, High 20, Very High 30, Perfect 100).
        public static void Orders(int seed)
        {
            W.New(seed, "hard", true, "gunsmith");
            PermitHelper.GrantedFromPermit(W.Make("permit_gun_2"));
            PermitHelper.GrantedFromPermit(W.Make("permit_gun_3"));
            W.Cash = 100000;
            foreach (int q in new[] { 0, 30 })
                foreach (var (name, make) in Basic)
                    Order($"Basic {name}, handmade Q{q} bare", make, () => Framed("handmade_pistol_frame", q));
            foreach (int q in new[] { 0, 30, 100 })
                foreach (var (name, make) in Mid)
                    Order($"Mid {name}, heavy Q{q} bare", make, () => Framed("heavy_handmade_pistol_frame", q));
            foreach (int q in new[] { 30, 100 })
                foreach (var (name, make) in High)
                    Order($"High {name}, heavy Q{q} bare", make, () => Framed("heavy_handmade_pistol_frame", q));
            Order("Basic suppressed, handmade Q0 + silencer", GunOrderData.CreateBasicOrderSuppressed, () => Framed("handmade_pistol_frame", 0, "silencer"));
            Order("Mid suppressed, heavy Q30 + silencer", GunOrderData.CreateMidOrderSuppressed, () => Framed("heavy_handmade_pistol_frame", 30, "silencer"));
            Order("Mid general, heavy Q30 + silencer", GunOrderData.CreateMidOrderGeneral, () => Framed("heavy_handmade_pistol_frame", 30, "silencer"));
            Order("High reliability, heavy Q30 + internal_springs + silencer", GunOrderData.CreateHighOrderReliability, () => Framed("heavy_handmade_pistol_frame", 30, "internal_springs", "silencer"));
            Order("High reliability, heavy Q100 + silencer", GunOrderData.CreateHighOrderReliability, () => Framed("heavy_handmade_pistol_frame", 100, "silencer"));
            Order("High general, heavy Q100 + silencer", GunOrderData.CreateHighOrderGeneral, () => Framed("heavy_handmade_pistol_frame", 100, "silencer"));
            // What the same guns fetch as stock: the estimate a weapon buyer bargains from.
            foreach (var (id, q) in new[] { ("handmade_pistol_frame", 0), ("handmade_pistol_frame", 30), ("heavy_handmade_pistol_frame", 30), ("heavy_handmade_pistol_frame", 100) })
            {
                var g = Framed(id, q);
                R.Info($"{id.Replace("_frame", "")} Q{q} New: estimate", PlayerRole.Est(g));
                (g.parentInventory as GameInventory)?.Expel(g);
            }
        }

        // The executor's forge on the real machines (sim/gym/Arms.cs, Printer.cs): with the kit owned and Permit II held,
        // its mornings fit the metal and CNC modules into the new printer (never the old one), print heavy frames from
        // the purest known ingot and gun parts from a Low one, and finish each frame into a legal New heavy pistol.
        public static void Engine(int seed)
        {
            W.New(seed, "hard", true, "gunsmith");
            PermitHelper.GrantedFromPermit(W.Make("permit_gun_2"));
            W.Cash = 100000;
            Directives.Current = Directives.Parse(Newtonsoft.Json.Linq.JObject.Parse("{\"printer\": \"run\", \"printer_chips\": [\"printer_chip_pistol\"], \"arms\": {\"forge\": true, \"keep_heavy\": 2, \"keep_handmade\": 0}}"));
            PlayerDay.Current = new PlayerDay { Day = W.Day };
            var old = OldPrinter();
            var fresh = NewPrinter();
            foreach (var id in new[] { "printer_module_metal", "printer_module_cnc", "printer_chip_heavy_handmade_pistol_frame", "printer_chip_gun_part" }) W.Place(W.E.invElement, Owned(id));
            foreach (var pur in new[] { "INGOT_PURITY_FAIR", "INGOT_PURITY_VERY_HIGH", "INGOT_PURITY_LOW", "INGOT_PURITY_VERY_HIGH" })
            {
                var ingot = Ingot(pur);
                W.Place(W.E.invElement, ingot);
            }
            for (int k = 0; k < 4; k++) W.Place(W.E.invElement, Owned("scrap_metal"));
            for (int k = 0; k < 2; k++) W.Place(W.E.invElement, Owned(PreBuiltItemHelper.EnergyCredit(100)));
            foreach (var g in W.E.GetAllOwnedItems(false, true).Where(i => i != null && i.identifier == "gun_part").ToList()) (g.parentInventory as GameInventory)?.Expel(g);
            Arms.Reset(); Printer.Reset();
            foreach (var i in W.E.GetAllOwnedItems(false, true).Where(i => i != null && i.identifier == "metal_ingot")) Arms.KnowIngot(i);
            var log = new List<string>();
            for (int day = 0; day < 16; day++)
            {
                PlayerDay.Current = new PlayerDay { Day = W.Day };
                Printer.Morning(W.Ep);
                Arms.Morning(W.Ep);
                log.Add($"d{day}:{Chip(fresh).childItem?.identifier?.Replace("printer_chip_", "")}");
                foreach (var b in new[] { fresh, old })
                {
                    var bat = MachineHelper.GetBatterySlot(b);
                    if (PowerHelper.GetAvailableEnergyFromItem(bat.childItem, false) < 20) Power(b);
                }
                W.QuietDay();
            }
            R.Exact("forge: the old printer refuses the CNC module (stuck modules), so the engine leaves its bay alone", false, Arms.Forgeable(old));
            R.Exact("forge: metal + CNC fitted into the new printer", "printer_module_cnc,printer_module_metal",
                string.Join(",", MachineHelper.GetModules(fresh).Where(m => m != null).Select(m => m.identifier).OrderBy(x => x, StringComparer.Ordinal)));
            var guns = W.E.GetAllOwnedItems(false, true).Where(i => i != null && i.identifier == "heavy_handmade_pistol").ToList();
            R.Exact("forge: heavy pistols finished in 16 nights (keep_heavy 2)", 2, guns.Count, 1, string.Join(" ", log));
            R.Info("forge: the guns' FP/STY/REL/RCL/ERG/ACC, condition, contraband", string.Join("; ", guns.Select(g => Stats(g) + " " + GunFrameHelper.GetCondition(g) + " c" + ContrabandHelper.GetContrabandLevelInInt(g))));
            R.Exact("forge: both frames took a Very High ingot (Q 30)", "heavy_handmade_pistol_q30=2", string.Join(",", ((Newtonsoft.Json.Linq.JObject)Arms.Status()["forge"]["finished_by_gun_q"]).Properties().Select(x => x.Name + "=" + x.Value)));
            var left = W.E.GetAllOwnedItems(false, true).Where(i => i != null && i.identifier == "metal_ingot").Select(i => IngotPurityHelper.GetPurity(i).Replace("INGOT_PURITY_", "")).OrderBy(x => x, StringComparer.Ordinal);
            R.Info("forge: ingots left (the part run takes the Low one first)", string.Join(",", left));
        }
    }
}