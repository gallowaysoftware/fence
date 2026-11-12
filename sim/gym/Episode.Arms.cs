using System.Linq;
using Fence.Barter;
using Fence.Mod;
using Fence.Recording;
using Fence.Sim;
using Newtonsoft.Json.Linq;

namespace Fence.Gym
{
    // A gun-order client at the counter (`arms`, Arms.cs): the tablet's order is filled from stock and accepted
    // (NegociationUIManager.OnOfferAcceptClick, GUNSMITH), or the client is sent away.
    internal sealed partial class Episode
    {
        object gunOrderClient;
        int serviceWaits;

        // Headless every item reads as destroyed to GameItemElement.IsDestroyed (its render node is an unattached fake),
        // so GunOrderData.GetPlacedServiceGun would never find the client's gun: its node is marked alive, as the lab's
        // night chemistry does (sim/lab/World.cs).
        static GameItem ServiceGun(GunOrderData o)
        {
            var e = EmporiumEntry.Instance;
            foreach (var g in e.frontInvinvElement.childItems.Concat(e.backInvinvElement.childItems))
                if (g is GameItemElement el && GunHelper.IsServiceGun(g) && !ReferenceEquals(el.handler, null)) Game.MarkAlive(el.handler);
            return o.GetPlacedServiceGun();
        }

        bool PlayerGunOrder()
        {
            if (!IsPlayer || !Arms.On) return false;
            var c = GameBarter.CurrentClient();
            if (c == null || c.clientIntent != StoreClient.ClientIntent.GUNSMITH || c.gunOrderData == null || c.isBusinessComplete) return false;
            if (ReferenceEquals(gunOrderClient, c)) return false;
            var o = c.gunOrderData;
            // The client's gun lands on the counter when the greeting ends.
            if (o.isServiceOrder && ServiceGun(o) == null && ++serviceWaits <= 12)
            {
                Scheduler.Pump(1.0);
                return true;
            }
            serviceWaits = 0;
            gunOrderClient = c;
            var info = new JObject { ["tier"] = o.tier, ["service"] = o.isServiceOrder, ["best"] = o.isBestOrder };
            Arms.Plan plan = null;
            Guard("gun order plan", () => plan = Arms.Prepare(c));
            if (plan?.Gun == null)
            {
                Arms.Refused(plan?.Why ?? "orders off");
                info["refused"] = plan?.Why ?? "orders off";
                Mod.Emit("gun_order", info);
                Dismiss(Actors.Human, "player:gun_order_refused");
                return true;
            }
            var e = EmporiumEntry.Instance;
            if (!plan.Service && !Move(e.backInvinvElement, plan.Gun))
            {
                Arms.Refused("could not stage the gun");
                Dismiss(Actors.Human, "player:gun_order_refused");
                return true;
            }
            int cash0 = Cash, uid = Mod.Game.Uid(plan.Gun);
            info["gun"] = plan.Gun.identifier;
            info["reward"] = plan.Reward;
            info["given"] = plan.Given;
            Mod.Human("gun_order", Advice(), uid, (JObject)info.DeepClone(), "player:gun_order");
            Guard("gun order accept", () => NegociationUIManager.Instance.OnOfferAcceptClick());
            int paid = Cash - cash0;
            if (paid > 0)
            {
                Arms.Delivered(c, plan, paid);
                Ledger.Flow("gun_orders", paid);
            }
            else Arms.Refused("accept did nothing");
            info["paid"] = paid;
            Mod.Emit("gun_order", info);
            Scheduler.Settle();
            return true;
        }
    }
}
