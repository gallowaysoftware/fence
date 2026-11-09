using Newtonsoft.Json.Linq;

namespace Fence.Recording
{
    public static class MoneyKinds
    {
        public const string RentPaid = "rent_paid";
        public const string RentMissed = "rent_missed";
        public const string ServicePaid = "service_paid";
        public const string WildUpgrade = "wild_upgrade";
        public const string PermitBought = "permit_bought";
        public const string PermitBribe = "permit_bribe";
        public const string StoreBought = "store_bought";
        public const string MortgageTaken = "mortgage_taken";
        public const string MortgagePayment = "mortgage_payment";
        public const string MortgagePaidOff = "mortgage_paid_off";
        public const string GeorgioLoan = "georgio_loan";
        public const string GeorgioPayback = "georgio_payback";
        public const string VendingSale = "vending_sale";
        public const string FountainSale = "fountain_sale";
        public const string Burglary = "burglary";
        public const string InspectorBribe = "inspector_bribe";
        public const string ItemSold = "item_sold";
        public const string ItemBought = "item_bought";
        public const string DayEnded = "day_ended";
    }

    public static class RunEvents
    {
        // A `money` event: what happened and the exact amounts the game passed to RunStats.
        public static JObject Money(string kind, JObject amounts = null)
        {
            var o = new JObject { ["kind"] = kind };
            if (amounts != null)
                foreach (var p in amounts.Properties())
                    o[p.Name] = p.Value;
            return o;
        }
    }

    // WildNetworkUI calls RunStats.OnWildUpgradeBought(cash) and then
    // EmporiumAnalytics.TrackWildUpgradePurchased(id); the pair is one upgrade_buy. Favor covers
    // the rest of the cost.
    public sealed class UpgradePurchase
    {
        int? cash;

        public void CashSpent(int amount) => cash = amount;

        public JObject Bought(string id, string name, int? cost)
        {
            var o = new JObject { ["upgrade_id"] = id };
            if (name != null) o["name"] = name;
            if (cost.HasValue) o["cost"] = cost.Value;
            if (cash.HasValue)
            {
                o["cash_spent"] = cash.Value;
                if (cost.HasValue) o["favor_spent"] = System.Math.Max(0, cost.Value - cash.Value);
            }
            cash = null;
            return o;
        }
    }
}
