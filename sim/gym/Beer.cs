using System;
using System.Linq;
using Newtonsoft.Json.Linq;

namespace Fence.Gym
{
    // `beer: "phone"` (gym): Wild's BREWER introduction (200) puts the Red Imp rep on the phone (PhoneDialogList
    // RedImpRepIntro / RedImpRepRegular): a free case first, then a case of eight beers per call at 20 % off
    // (two cases once 400 has been spent with him), 4-day cooldown. `beer_shareholder: true` adds
    // BREWER_SHAREHOLDER (1 450): another 15 % off every delivery and a free case each week
    // (ShareholderHelper dividends). The courier sells at the counter like any wholesale seller.
    internal static class Beer
    {
        public static int Orders;

        public static void Reset() => Orders = 0;

        public static bool On => Directives.Current.Raw.Value<string>("beer") == "phone";
        public static bool Shareholder => On && (Directives.Current.Raw.Value<bool?>("beer_shareholder") ?? false);
        // Cash kept over the rent and floor before buying either upgrade.
        public static int Reserve => Directives.Current.Raw.Value<int?>("beer_reserve") ?? 500;

        static bool Up(string id) => NetworkUpgrade.GetUpgradeById(id)?.IsUnlocked() ?? false;

        public static void Morning(IExecutorHost ep)
        {
            if (!On || !Up("BREWER")) return;
            var c = StorePhoneClient.GetBrewerPhoneClient();
            string courier;
            int days = 1, choice = -1;
            if (c.phoneState == StorePhoneClient.PhoneState.Introduction)
                courier = "courrierRedImpFree";
            else if (c.phoneState == StorePhoneClient.PhoneState.Regular)
            {
                bool big = c.amountSpent >= 400;
                int rent = PlayerDay.Current?.RentDue ?? PlayerStore.Instance.GetRentValue();
                if (PlayerStore.Instance.playerCash < rent + (big ? 300 : 150) || PlayerStock.FreeCells() < 12) return;
                choice = big ? 1 : 0;
                courier = "courrierRedImp";
                if (big) days = 2;
            }
            else return;
            ep.PhoneOrder(c, StorePhoneClient.RED_IMP_REP_NUMBER, choice, courier, days);
            Orders++;
            ep.Human("phone_order", null, null, new JObject { ["supplier"] = "red_imp", ["courier_id"] = courier, ["in_days"] = days }, "player:beer");
        }
    }
}
