using Fence.Bridge;

namespace Fence.Recording
{
    // Event kinds of phase D (docs/PROTOCOL.md, "Recording events").
    public static class RecordKinds
    {
        public const string DaySnapshot = "day_snapshot";
        public const string CustomerObserved = "customer_observed";
        public const string HumanAction = "human_action";
        public const string Money = "money";
        public const string RunStart = "run_start";
        public const string RunEnd = "run_end";
        public const string Inspection = "inspection";
        public const string RunStat = "run_stat";
        // Shots, kills, captures and augs that left alive (Fence.Aug.AugEvents).
        public const string Aug = "aug";
        // Fence.Autopilot.AutopilotEvents: every autopilot action, and its stops.
        public const string Autopilot = "autopilot";
        public const string AutopilotFault = "autopilot_fault";

        // What a full queue gives up first: forwarded ModHook events are context, the recording
        // kinds are the training data.
        public static EventPriority Priority(string kind)
        {
            switch (kind)
            {
                case DaySnapshot:
                case CustomerObserved:
                case HumanAction:
                case Money:
                case RunStart:
                case RunEnd:
                case Inspection:
                case RunStat:
                case Aug:
                case Autopilot:
                case AutopilotFault:
                    return EventPriority.High;
                case "client_arrived":
                case "shutter_opened":
                case "shutter_closed":
                case "going_sleep":
                case "waking_up":
                case "game_loaded":
                case "new_game_started":
                case "leaving_store":
                case "returning_store":
                    return EventPriority.Normal;
                default:
                    return EventPriority.Low;
            }
        }
    }

    public enum EventPriority
    {
        Low,
        Normal,
        High,
    }

    public sealed class RecordingView
    {
        public bool Recording;
        public string Text;
    }

    // The overlay's recording line. Anything that means a gap in the data says NOT RECORDING: the
    // setting is off, the bridge refused us, an event was lost, or the bridge has not answered for
    // longer than UnreachableGraceMs (events then only wait in the in-memory buffer).
    public static class RecordingStatus
    {
        public const long UnreachableGraceMs = 10000;

        public static RecordingView Of(BridgeSession s, long nowMs)
        {
            if (s == null || !s.Enabled) return Not("off in settings");
            if (s.State == SessionState.Vanilla) return Not("bridge refused: " + (s.VanillaReason ?? "vanilla"));
            if (s.DroppedEvents > 0) return Not(s.DroppedEvents + " events lost this session");
            if (s.UnreachableSinceMs.HasValue && nowMs - s.UnreachableSinceMs.Value > UnreachableGraceMs)
                return Not("bridge unreachable " + ((nowMs - s.UnreachableSinceMs.Value) / 1000) + " s, " + s.QueuedEvents + " buffered");
            return new RecordingView { Recording = true, Text = s.QueuedEvents > 20 ? "recording (" + s.QueuedEvents + " buffered)" : "recording" };
        }

        static RecordingView Not(string why) => new RecordingView { Recording = false, Text = "NOT RECORDING — " + why };
    }
}
