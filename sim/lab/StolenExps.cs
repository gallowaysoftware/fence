using System;
using System.Collections.Generic;
using System.Linq;
using Fence.Gym;
using Fence.Sim;

namespace Fence.Lab
{
    // docs/re/STOLEN.md: the Fence start, how stolen goods arrive and what they cost, who brings them, and what
    // Security does about them (reporting a thief, the crackdown's inspectors).
    public static class StolenExps
    {
        const string SEC = "FACTION_SECURITY", BM = "FACTION_BLACK_MARKET", LL = "FACTION_LOWER_LEVEL";

        // The Fence start's kit and reputation offsets, on normal and hard (start offsets scale with the difficulty).
        // One world a process: a second W.New keeps the first world's shop.
        public static void Start(int seed) => StartOn(seed, "hard", -37.5, 22.5);
        public static void StartNormal(int seed) => StartOn(seed, "normal", -30, 30);

        static void StartOn(int seed, string diff, double sec, double bm)
        {
            {
                W.New(seed, diff, true, "fence");
                R.Near($"Fence start on {diff}: Security {sec} (-30 x loss mult)", sec, W.Rep(SEC), 0.01);
                R.Near($"Fence start on {diff}: Black Market {bm} (+30 x gain mult)", bm, W.Rep(BM), 0.01);
                var kit = W.E.GetAllOwnedItems(false, true).Where(i => i != null).GroupBy(i => i.identifier).OrderBy(g => g.Key, StringComparer.Ordinal)
                    .Select(g => g.Key + (g.Count() > 1 ? "x" + g.Count() : ""));
                R.Info($"Fence start on {diff}: owned items, cash, rent, exchange unlocked",
                    $"{string.Join(",", kit)}; cash {W.Cash}; rent {W.PS.GetRentValue()}; exchange {W.PS.exchangeManager.isExchangeUnlocked}");
            }
        }

        // A seller's stolen item carries a buying feature of -10/-15/-20/-25 % by heat (<=25/<=50/<=75/more) while it is
        // on the counter; the shown estimate includes it, and buying it removes it (the clean estimate returns).
        public static void Discount(int seed)
        {
            W.New(seed);
            W.Cash = 100000;
            foreach (var heat in new[] { 25, 50, 75, 100 })
            {
                var c = W.Seat("thief");
                var clean = W.Make("surgery_tool");
                long cleanEst = clean.GetCurrentValue(false, true, true, false, 0L);
                var it = W.Make("surgery_tool");
                W.PS.TryAddDirectSellingItemToTable(it, false, true, false, heat);
                Scheduler.Settle();
                long shown = it.GetCurrentValue(false, true, true, false, 0L), price = it.GetNegociatedValue();
                int want = heat <= 25 ? 10 : heat <= 50 ? 15 : heat <= 75 ? 20 : 25;
                R.Near($"stolen discount at heat {heat}: estimate on the counter = clean x (1 - {want}%)", cleanEst * (1 - want / 100.0), shown, 1.01, 1,
                    $"clean {cleanEst}, shown {shown}, price {price}, heat {StolenHelper.GetFoundChance(it)}");
                W.PS.BuyItem(it);
                Scheduler.Settle();
                W.Unseat();
                R.Near($"stolen discount at heat {heat}: bought, the estimate is the clean one again", cleanEst, it.GetCurrentValue(false, true, true, false, 0L), 1.01, 1,
                    "still stolen: " + StolenHelper.IsStolenItem(it));
                W.ClearInventory(W.E.invElement);
                W.ClearInventory(W.E.frontInvinvElement);
                W.ClearInventory(W.E.backInvinvElement);
            }
        }

        static readonly string[] Thieves = { "thief", "pettyThief", "foodThief" };

        // Black Market sellers a day by BM rep (StoreClientManager.GetBlackMarketClientChance, BM_RELIABLE's weekly thief
        // and 20 % petty thief): the queue generated at the opening, rep pinned each morning.
        public static void ThiefRate(int seed)
        {
            foreach (var rep in new[] { -10.0, 0.0, 22.5, 25.0, 40.0, 50.0, 60.0 })
            {
                W.New(seed, "hard");
                W.Cash = 100000;
                int days = Math.Max(60, Program.N / 100), bmClients = 0;
                var n = Thieves.ToDictionary(x => x, x => 0);
                for (int d = 0; d < days && !W.Over; d++)
                {
                    W.SetRep(BM, rep);
                    W.SetRep(SEC, 0);
                    W.SetRep(LL, 0);
                    W.Cash = Math.Max(W.Cash, 100000);
                    var q = W.Open();
                    bmClients += q.Count(c => c.clientFaction == BM && c.clientIntent == StoreClient.ClientIntent.SELL);
                    foreach (var c in q) if (n.ContainsKey(c.identifier)) n[c.identifier]++;
                    W.CloseAndSleep();
                }
                R.Info($"BM rep {rep}: BM sellers / thief / petty thief / food thief per 7 days", $"{7.0 * bmClients / days:0.00} / {string.Join(" / ", Thieves.Select(t => (7.0 * n[t] / days).ToString("0.00")))}", days);
            }
        }

        // Reporting a seller of stolen goods (not wanted): 50 + 4 % of the stolen goods' price, Security + the highest
        // contraband level on the counter (0 for plain stolen goods); the seller's faction loses its escalating report penalty
        // (StoreReputation.ReportedClient: -8, then 2 more each report, floor -20, before the BM x2 and difficulty multipliers).
        public static void Report(int seed)
        {
            W.New(seed, "hard");
            W.Cash = 1000;
            foreach (var id in Thieves)
            {
                W.ResetReps();
                var c = W.Seat(id);
                foreach (var item in new[] { "surgery_tool", "metal_ingot" }) W.PS.TryAddDirectSellingItemToTable(W.Make(item), false, true, false, 100);
                Scheduler.Settle();
                long value = W.E.GetAllNonOwnedItem().Where(StolenHelper.IsStolenItem).Sum(i => Math.Round(i.GetNegociatedValue() * 0.04, MidpointRounding.ToEven) is double r ? (long)r : 0);
                int cash0 = W.Cash;
                double sec0 = W.Rep(SEC), bm0 = W.Rep(BM);
                W.PS.OnCustomerReported();
                var a = W.PS.onSecAlarmLeftAction;
                W.PS.onSecAlarmLeftAction = null;
                a?.Invoke();
                Scheduler.Settle();
                R.Near($"report a {id} selling stolen goods: cash = 50 + 4 % of their price", 50 + value, W.Cash - cash0, 1.01, 1,
                    $"Security {W.Rep(SEC) - sec0:+0.##;-0.##;0}, BM {W.Rep(BM) - bm0:+0.##;-0.##;0}, degrades {c.reportDegradesFaction}, penalty {c.reputationPenalty}");
                W.Unseat();
                W.ClearInventory(W.E.frontInvinvElement);
                W.ClearInventory(W.E.backInvinvElement);
            }
        }

        // A crackdown (Security operation, two days) sends a normal inspector each day it is active
        // (StoreEvent.InitNormalEvent sets "crackdownaddClientAction"; StoreClientManager.HandleClientFromEvent).
        public static void Crackdown(int seed)
        {
            int days = 0, withInspector = 0, control = 0, controlDays = 0;
            for (int t = 0; t < Math.Max(20, Program.N / 200); t++)
            {
                W.New(seed + t, "hard");
                W.Cash = 100000;
                for (int d = 0; d < 3; d++) W.QuietDay();
                var q0 = W.Open();
                controlDays++;
                if (q0.Any(c => c.clientIntent == StoreClient.ClientIntent.INSPECTION)) control++;
                W.CloseAndSleep();
                StoreStation.Instance.storeEventManager.ActivateEvent(OperationListSec.Crackdown(), false, true);
                var q = W.Open();
                days++;
                if (q.Any(c => c.clientIntent == StoreClient.ClientIntent.INSPECTION && c.eventSourceId == "crackdown")) withInspector++;
                W.CloseAndSleep();
            }
            R.Rate("crackdown active: a crackdown inspector is queued at the opening", 1.0, withInspector, days, $"control days with any inspector: {control}/{controlDays}");
        }
    }
}
