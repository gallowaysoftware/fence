using System;
using System.Collections.Generic;

namespace Fence.Autopilot
{
    // A machine window's widget tree searched depth first through the children the caller exposes (the mod: the
    // public members of PixelWindow, MachineryPanelRow, MachineryPanel, MachineryIntake and MachineryStack). Cycles
    // and runaway depth end the search instead of the frame.
    public static class UiTree
    {
        public const int MaxDepth = 12;

        public static T Find<T>(object root, Func<object, IEnumerable<object>> children, Func<object, T> match) where T : class
        {
            var seen = new List<object>();
            return Walk(root, 0);

            T Walk(object node, int depth)
            {
                if (node == null || depth > MaxDepth || seen.Exists(x => ReferenceEquals(x, node))) return null;
                seen.Add(node);
                var hit = match(node);
                if (hit != null) return hit;
                var kids = children(node);
                if (kids == null) return null;
                foreach (var k in kids)
                {
                    var found = Walk(k, depth + 1);
                    if (found != null) return found;
                }
                return null;
            }
        }
    }

    // The chemistry lab's thermostat (Lab.Second): the player at the plate's readout, once a second of wall time
    // while the autopilot runs. Toggles are summed into one feed line per ReportMs instead of one each.
    public sealed class Thermostat
    {
        public const long PeriodMs = 1000;
        public const long ReportMs = 30000;

        long lastMs = -1;
        long reportedMs = long.MinValue / 2;
        int switches;

        public bool Due(long nowMs)
        {
            if (lastMs >= 0 && nowMs - lastMs < PeriodMs) return false;
            lastMs = nowMs;
            return true;
        }

        // The switches a tick made; the count to report when a line is due (the first at once, then at most one per
        // ReportMs), else 0.
        public int Switched(int n, long nowMs)
        {
            switches += Math.Max(0, n);
            if (switches == 0 || nowMs - reportedMs < ReportMs) return 0;
            int r = switches;
            switches = 0;
            reportedMs = nowMs;
            return r;
        }
    }
}
