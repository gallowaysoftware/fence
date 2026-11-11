using System;
using System.Collections.Generic;
using System.Linq;
using System.Reflection;

namespace Fence.Gym
{
    // Wild's Network without its window: what WildNetworkUI.Open does besides drawing, the purchase of
    // OnUseClicked, and OnWindowClosed (which resumes Joe's introduction or dismisses a reminder).
    internal static class WildNetwork
    {
        public static bool Reachable() => WildRead.Reachable();

        public static string Why() => WildRead.Why();

        public static void Open()
        {
            NetworkUpgrade.InvalidateDefinitions();
            NetworkUpgrade.AddMissingAfterLoad();
            PlayerStore.Instance.daySinceLastBrowseUpgrade = 0;
        }

        public static UpgradeView View() => WildRead.View();

        // WildNetworkUI.OnUseClicked: favor first, then cash.
        public static bool Buy(string id, out int cost, out int cashSpent)
        {
            var ps = PlayerStore.Instance;
            cost = cashSpent = 0;
            if (id == null || !ps.networkUpgrade.TryGetValue(id, out var u) || !u.IsReady() || u.IsUnlocked()) return false;
            cost = u.GetCost();
            if (cost > ps.playerCash + ps.wildFavor) return false;
            if (ps.wildFavor >= cost) ps.wildFavor -= cost;
            else
            {
                cashSpent = cost - ps.wildFavor;
                ps.wildFavor = 0;
                ps.playerCash -= cashSpent;
            }
            RunStats.OnWildUpgradeBought(cashSpent);
            u.Unlock();
            return true;
        }

        static readonly MethodInfo Closed = typeof(WildNetworkUI).GetMethod("OnWindowClosed", BindingFlags.Static | BindingFlags.NonPublic);

        public static void Close() => Closed.Invoke(null, null);
    }
}
