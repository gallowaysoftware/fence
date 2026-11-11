using System;
using System.Collections.Generic;
using System.Linq;
using Newtonsoft.Json.Linq;

namespace Fence.Gym
{
    // The executor's morning (docs/STREAM.md refactor 1), shared by the gym's Episode and the live autopilot:
    // the day's plan under the directives in force, then the engines' chores. `days` is the ledger the plan
    // reads (one object per day played: sold_value, net_worth_start, net_worth_end).
    internal static class ExecutorMorning
    {
        public static PlayerDay Plan(IExecutorHost host, IList<JObject> days)
        {
            var ps = PlayerStore.Instance;
            var d = Directives.Current;
            int today = host.Day, cash = ps.playerCash;
            int svc = ps.storeService.Where(s => s.unlocked && s.isUsing).Sum(s => s.cost);
            // Under a mortgage the loan shark's weekly payment takes the rent's place.
            int rent = ps.isPayingMortgage ? ps.mortgageWeeklyPayment : ps.GetRentValue(), left = ps.dayUntilRent;
            var bribeHold = PlayerCounter.BribeReserve(today);
            var day = new PlayerDay { Day = today, CashAtMorning = cash, RentDue = rent, RentDaysLeft = left, Reserved = i => Production.Reserved(i) || bribeHold.Contains(i.uniqueId) };
            // Rent is paid to the landlord on rent day; keep it (and the nights of services until then)
            // from two days out, and sell down toward it from three.
            // A forward floor from five days out: the rent less what the shop typically sells before it.
            var recent = days.Skip(Math.Max(0, days.Count - 7)).ToList();
            int perDay = recent.Count == 0 ? 0 : (int)(recent.Sum(x => (int?)x["sold_value"] ?? 0) / recent.Count * 0.6);
            day.Rent2Floor = left <= 2 ? rent + svc * (left + 1) : left <= 5 ? Math.Max(0, rent + svc * (left + 1) - perDay * left) : 0;
            day.Floor = d.CashFloor ?? day.Rent2Floor;
            day.CashTarget = d.SellDownToCash ?? (left <= 3 ? rent + svc * (left + 1) : 0);
            // Under the mortgage a missed payment repossesses goods (and ends the run when there are not
            // enough): one payment always stays in cash, and the sell-down starts a day earlier.
            if (ps.isPayingMortgage && d.CashFloor == null)
            {
                // `mortgage_floor_days: N`: the payment is held in cash only from N days before it.
                // `mortgage_floor_stock: f` (gym): repossessable stock counts toward that payment at f of its value (a
                // missed payment takes goods worth it, PlayerStore.Repo), so trade buying goes on.
                int held = d.Raw.Value<double?>("mortgage_floor_stock") is double fs ? (int)Math.Min(rent, fs * PlayerStock.RepoValue()) : 0;
                if (!(d.Raw.Value<int?>("mortgage_floor_days") is int fd && left > fd)) day.Floor = Math.Max(day.Floor, rent - held);
                // Sell down from five days out, with a margin, while the payment is in doubt.
                int sdDays = d.Raw.Value<int?>("mortgage_selldown_days") ?? 5, sdMargin = d.Raw.Value<int?>("mortgage_selldown_margin") ?? 150;
                if (d.SellDownToCash == null && left <= sdDays) day.CashTarget = Math.Max(day.CashTarget, rent + svc * (left + 1) + sdMargin);
            }
            // The win: the landlord offers the buyout on rent day once the cash covers the price. When cash
            // and most of the stock would cover it, stop buying and sell down toward it for that visit.
            // `mortgage_save_from_day: N`: from day N, stop buying and sell down toward the loan shark's
            // threshold (twice the down payment), so his offer comes on a rent morning.
            if (d.Raw.Value<int?>("mortgage_save_from_day") is int saveFrom && today >= saveFrom && !ps.isPayingMortgage && !ps.IsPropertyPaid)
            {
                int need = 2 * ps.GetMortgageDownPayment() + 100;
                day.Floor = Math.Max(day.Floor, Math.Min(cash, need));
                day.CashTarget = Math.Max(day.CashTarget, need);
            }
            bool hold = d.Raw.Value<string>("buyout") == "hold_cash_for_buyout";
            if (hold && !ps.IsPropertyPaid && !ps.isPayingMortgage) day.Floor = Math.Max(day.Floor, Math.Min(cash, ps.GetStoreBuyPrice(false) + 100));
            // `buyout_selldown_days` (gym, default 3): how far before a rent visit the buyout sell-down starts.
            if (!ps.IsPropertyPaid && !ps.isPayingMortgage && (left <= (d.Raw.Value<int?>("buyout_selldown_days") ?? 3) || hold))
            {
                int price = ps.GetStoreBuyPrice(false);
                long sellable = PlayerStock.Value();
                if (cash + sellable * 0.7 >= price + 100)
                {
                    day.CashTarget = Math.Max(day.CashTarget, price + 100);
                    day.Floor = Math.Max(day.Floor, Math.Min(cash, price + 100));
                    day.SavingForBuyout = true;
                }
            }
            // `buyout_save: [...]` from `buyout_save_from` (gym): while saving for the cash buyout, cut the spending
            // named (upgrades, inputs, vending, insurance); the mortgage or the buyout ends it.
            if (d.Raw["buyout_save"] is JArray cuts && today >= (d.Raw.Value<int?>("buyout_save_from") ?? 1) && !ps.IsPropertyPaid && !ps.isPayingMortgage)
                day.BuyoutCuts = new HashSet<string>(cuts.Select(x => (string)x), StringComparer.Ordinal);
            // Known price rises today and over the next three days (the calendar a player reads): their item
            // types are worth stocking up on at their estimate.
            try
            {
                var em = StoreStation.Instance.storeEventManager;
                for (int on = today; on <= today + 3; on++)
                    foreach (var ev in em.GetActiveEventOnDay(on, true, true))
                        foreach (var n in ev.negociationDatas ?? new List<NegociationData>())
                            if (n != null && n.value >= 20 && n.itemType != null) day.RisingTypes.Add(n.itemType);
            }
            catch (Exception e)
            {
                host.Error("events", e);
            }
            // Georgio asks for a loan of 25 or 50 % of the cash on hand (around day 43) and repays four times as
            // much: `georgio_peak_days: [40, 43]` stops buying and sells down over those days to lend more.
            if (d.Raw["georgio_peak_days"] is JArray gp && gp.Count == 2 && today >= (int)gp[0] && today <= (int)gp[1] && ps.loanOfferedToGeorgio == 0 && !ps.isPayingMortgage)
            {
                day.Floor = Math.Max(day.Floor, cash);
                day.CashTarget = Math.Max(day.CashTarget, cash + (int)(PlayerStock.Value() * 0.5));
            }
            // `georgio_collect: early`: Georgio repays (4x) on a rent morning when the cash is under the rent. On the
            // eve of a rent day after his loan, spend the cash down below the rent (stock at up to its estimate,
            // no floor) with Landlord Grace armed as the backstop.
            if (d.Raw.Value<string>("georgio_collect") == "early" && ps.loanOfferedToGeorgio > 0 && today > ps.dayWhenLoanOfferedGeorgio + 5
                && left == 1 && NetworkUpgrade.IsLandlordGraceUnlocked() && !ps.isPayingMortgage)
            {
                day.SpendDownTo = ps.rentValue - 1;
                day.Floor = 0;
                day.CashTarget = 0;
            }
            // `spend_down_to: X`: end the day under X cash, buying stock and inputs at up to their estimate.
            if (d.Raw.Value<int?>("spend_down_to") is int sdx && cash > sdx)
            {
                day.SpendDownTo = sdx;
                day.Floor = 0;
                day.CashTarget = 0;
            }
            if (days.Count >= 28)
                day.NetGainWeek4 = ((int)days[days.Count - 1]["net_worth_end"] - (int)days[days.Count - 28]["net_worth_start"]) / 4;
            // The slower of the last four and eight weeks: a young shop's stock build-up is not income.
            if (days.Count >= 56)
            {
                int g4 = ((int)days[days.Count - 1]["net_worth_end"] - (int)days[days.Count - 28]["net_worth_start"]) / 4;
                int g8 = ((int)days[days.Count - 1]["net_worth_end"] - (int)days[days.Count - 56]["net_worth_start"]) / 8;
                day.NetGainWeek = Math.Min(g4, g8);
            }
            return day;
        }

        // The engines' morning chores: back-office moves into machines, before the shutter opens.
        public static void Chores(IExecutorHost host)
        {
            Production.Morning(host);
            try { Beer.Morning(host); }
            catch (Exception e) { host.Error("beer", e); }
            try { Vending.Morning(host); }
            catch (Exception e) { host.Error("vending", e); }
            try { Labels.Morning(host); }
            catch (Exception e) { host.Error("labels", e); }
            try { Conceal.Morning(host); }
            catch (Exception e) { host.Error("conceal", e); }
            try { Storage.Morning(); }
            catch (Exception e) { host.Error("storage", e); }
        }
    }
}
