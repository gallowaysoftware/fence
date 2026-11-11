using System.Collections.Generic;

namespace Fence.Gym
{
    // What the executor reads of Wild's Network, shared with the live autopilot (WildNetwork.cs is the gym's
    // windowless half).
    internal static class WildRead
    {
        // The game opens the window from Joe's visits and from the phone; at night the gym models the
        // phone call, which needs Wild's line in its regular state.
        public static bool Reachable()
        {
            var ps = PlayerStore.Instance;
            if (!ps.isNetworkIntroduced) return false;
            try
            {
                return StorePhoneClient.GetWildPhoneClient().phoneState == StorePhoneClient.PhoneState.Regular;
            }
            catch (KeyNotFoundException)
            {
                return false;
            }
        }

        public static string Why()
        {
            var ps = PlayerStore.Instance;
            if (!ps.isNetworkIntroduced) return "network not introduced";
            try { return "phone " + StorePhoneClient.GetWildPhoneClient().phoneState; }
            catch (KeyNotFoundException) { return "no phone client"; }
        }

        // The slots WildNetworkUI.BuildTabs lists, with the use button's state (ShowInfo).
        public static UpgradeView View()
        {
            var ps = PlayerStore.Instance;
            var v = new UpgradeView { Cash = ps.playerCash, Favor = ps.wildFavor, RentDue = ps.GetRentValue() };
            foreach (var d in NetworkUpgrade.GetDefinitionsInOrder())
            {
                if (d.hidden || NetworkUpgrade.IsHiddenForRun(d.id) || !ps.networkUpgrade.TryGetValue(d.id, out var u)) continue;
                int cost = u.GetCost();
                bool unlocked = u.IsUnlocked();
                v.Upgrades.Add(new UpgradeOption
                {
                    Id = u.id, Cost = cost, State = u.state, Unlocked = unlocked, Repeatable = u.isRepeatable,
                    Buyable = u.IsReady() && !unlocked && cost <= ps.playerCash + ps.wildFavor,
                    Ready = u.IsReady(), Cooldown = u.cooldownCurrent,
                });
            }
            return v;
        }

    }
}
