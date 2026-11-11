using System;
using System.Collections.Generic;
using System.Linq;
using Newtonsoft.Json.Linq;

namespace Fence.Gym
{
    // Wild's phone suppliers (PhoneDialogList, StoreClientListSupplier, ShareholderHelper). `beer: "phone"` buys the
    // BREWER introduction and phones the Red Imp rep whenever his line is free: a free case first, then a case of
    // eight beers per call at 20 % off (two once 400 has been spent with him), 4-day cooldown. `pharma: "phone"`
    // does the same with Dale (PHARMA: six free pink injectors, then 5 a call at 20 % off, 10 once 500 is spent,
    // 7-day cooldown) and `chem: "phone"` with Ashish (CHEMIST: eight free dream dust, then 8 a call, 16 once 400
    // is spent, 4-day cooldown; dust is contraband 2, so paid orders wait for `buy.contraband`). `<key>_shareholder: true` adds the shareholder upgrade:
    // another 15 % off every delivery and a weekly dividend (a case, 5 injectors or 8 dust) laid on the front
    // counter at opening. Free deliveries and dividends arrive already ours on the front counter, where the night
    // clears them (EmporiumEntry.ClearFrontInv): they are stowed like a player picks them up. Couriers sell at the
    // counter like any wholesale seller.
    internal static class Suppliers
    {
        sealed class Line
        {
            public string Key, Intro, Shareholder, Free, Paid, Goods, Name;
            public long Number;
            public int Small, Big, BigAfter, Cells, UnitValue;
            public bool Contraband;
            public Func<StorePhoneClient> Client;
        }

        static readonly Line[] Lines =
        {
            new Line { Key = "beer", Name = "red_imp", Intro = "BREWER", Shareholder = "BREWER_SHAREHOLDER", Number = StorePhoneClient.RED_IMP_REP_NUMBER, Free = "courrierRedImpFree", Paid = "courrierRedImp", Goods = "beerCase", Small = 1, Big = 2, BigAfter = 400, Cells = 20, UnitValue = 145, Client = StorePhoneClient.GetBrewerPhoneClient },
            new Line { Key = "pharma", Name = "pharma", Intro = "PHARMA", Shareholder = "PHARMA_SHAREHOLDER", Number = StorePhoneClient.PHARMA_NUMBER, Free = "courrierPharmaFree", Paid = "courrierPharma", Goods = "pink_injector", Small = 5, Big = 10, BigAfter = 500, Cells = 4, UnitValue = 50, Client = StorePhoneClient.GetPharmaPhoneClient },
            new Line { Key = "chem", Name = "chem", Intro = "CHEMIST", Shareholder = "CHEMIST_SHAREHOLDER", Number = StorePhoneClient.CHEM_NUMBER, Free = "courrierChemFree", Paid = "courrierChem", Goods = "dream_dust", Small = 8, Big = 16, BigAfter = 400, Cells = 1, UnitValue = 18, Contraband = true, Client = StorePhoneClient.GetChemPhoneClient },
        };

        public static int Orders;
        static readonly SortedDictionary<string, int[]> sold = new SortedDictionary<string, int[]>(StringComparer.Ordinal), bought = new SortedDictionary<string, int[]>(StringComparer.Ordinal);
        static readonly HashSet<int> stowed = new HashSet<int>();

        public static void Reset()
        {
            Orders = 0;
            stowed.Clear();
            sold.Clear();
            bought.Clear();
        }

        static bool LineOn(Line l) => Directives.Current.Raw.Value<string>(l.Key) == "phone";
        static bool HolderOn(Line l) => LineOn(l) && (Directives.Current.Raw.Value<bool?>(l.Key + "_shareholder") ?? false);

        public static bool On => Lines.Any(LineOn);
        // `miner_shareholder: true` (with the furnace's `ore_phone`): MINER_SHAREHOLDER, 6 ore a week and 15 % more
        // off Caulson's deliveries; the furnace stows and smelts the dividend.
        static bool MinerHolder => (Directives.Current.Raw.Value<bool?>("miner_shareholder") ?? false) && (NetworkUpgrade.GetUpgradeById("MINER")?.IsUnlocked() ?? false);
        public static bool BeerOn => LineOn(Lines[0]);

        // The upgrades the executor buys, in order: each introduction, then its shareholder share.
        public static IEnumerable<string> Upgrades()
        {
            foreach (var l in Lines.Where(LineOn))
            {
                yield return l.Intro;
                if (HolderOn(l)) yield return l.Shareholder;
            }
            if (MinerHolder) yield return "MINER_SHAREHOLDER";
        }

        public static bool Wants(string upgradeId) => Lines.Any(l => (LineOn(l) && l.Intro == upgradeId) || (HolderOn(l) && l.Shareholder == upgradeId)) || (MinerHolder && upgradeId == "MINER_SHAREHOLDER");

        // Cash kept over the rent and floor before buying a supplier's upgrade.
        public static int Reserve(string upgradeId)
        {
            var l = Lines.FirstOrDefault(x => x.Intro == upgradeId || x.Shareholder == upgradeId);
            string key = l?.Key ?? (upgradeId == "MINER_SHAREHOLDER" ? "miner" : null);
            return key == null ? 500 : Directives.Current.Raw.Value<int?>(key + "_reserve") ?? 500;
        }

        // A supplier's free delivery or dividend, ours on the front counter.
        public static bool IsGift(GameItem i) => i != null && Lines.Any(l => LineOn(l) && i.identifier == l.Goods);

        static bool Up(string id) => NetworkUpgrade.GetUpgradeById(id)?.IsUnlocked() ?? false;

        public static void Morning(IExecutorHost ep)
        {
            foreach (var l in Lines)
                if (LineOn(l) && Up(l.Intro)) Phone(ep, l);
        }

        static void Phone(IExecutorHost ep, Line l)
        {
            var c = l.Client();
            string courier;
            int days = 1, choice = -1, n = 0;
            if (c.phoneState == StorePhoneClient.PhoneState.Introduction)
                courier = l.Free;
            else if (c.phoneState == StorePhoneClient.PhoneState.Regular)
            {
                // A contraband lot the plan would not buy only brings the courier for nothing.
                if (l.Contraband && !(Directives.Current.Raw["buy"]?.Value<bool?>("contraband") ?? false)) return;
                bool big = c.amountSpent >= l.BigAfter;
                n = big ? l.Big : l.Small;
                int rent = PlayerDay.Current?.RentDue ?? PlayerStore.Instance.GetRentValue();
                // The courier is paid on arrival: the lot at 80 % (65 % as a shareholder) of its value.
                if (PlayerStore.Instance.playerCash < rent + n * l.UnitValue * 8 / 10 + 50 || PlayerStock.FreeCells() < n * l.Cells + 12) return;
                choice = big ? 1 : 0;
                courier = l.Paid;
                if (big) days = 2;
            }
            else return;
            ep.PhoneOrder(c, l.Number, choice, courier, days);
            Orders++;
            ep.Human("phone_order", null, null, new JObject { ["supplier"] = l.Name, ["courier_id"] = courier, ["in_days"] = days }, "player:" + l.Key);
        }

        public static void Stowed(GameItem i) => stowed.Add(i.uniqueId);

        // Gym bookkeeping: each supplier's goods bought and sold (count, cash).
        public static void Trade(bool isSale, GameItem item, int amount)
        {
            if (item == null || !On) return;
            var l = Lines.FirstOrDefault(x => LineOn(x) && (x.Goods == item.identifier || (x.Goods == "beerCase" && item.identifier == "red_beer")));
            if (l == null) return;
            var d = isSale ? sold : bought;
            string k = item.identifier;
            if (!d.TryGetValue(k, out var v)) d[k] = v = new int[2];
            v[0]++;
            v[1] += amount;
        }

        public static JObject Status()
        {
            if (!On) return null;
            static JObject By(SortedDictionary<string, int[]> d) => new JObject(d.Select(kv => new JProperty(kv.Key, new JArray(kv.Value[0], kv.Value[1]))));
            return new JObject { ["orders"] = Orders, ["gifts_stowed"] = stowed.Count, ["bought"] = By(bought), ["sold"] = By(sold) };
        }
    }
}
