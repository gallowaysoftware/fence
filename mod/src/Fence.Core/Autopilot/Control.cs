using System;

namespace Fence.Autopilot
{
    public enum AutopilotState
    {
        // The setting is off.
        Off,
        Running,
        // The human paused it (overlay, key bind, or a move of their own).
        Paused,
        // Stopped by an error or an action that did not take.
        Faulted,
        // Stopped by a situation it has no handler for; the human plays it.
        Handover,
    }

    // The brake. Starts paused: the human presses Resume once the game is where they want it. Every stop
    // is resumed by hand; nothing restarts the autopilot on its own.
    public sealed class AutopilotSwitch
    {
        public AutopilotState State { get; private set; } = AutopilotState.Paused;
        public string Reason { get; private set; } = "press Resume to start";
        public bool Running => State == AutopilotState.Running;
        // Something went wrong or needs a human, as opposed to a pause the human chose.
        public bool Alarmed => State == AutopilotState.Faulted || State == AutopilotState.Handover;

        public void SetEnabled(bool on)
        {
            if (!on) Set(AutopilotState.Off, "off in settings");
            else if (State == AutopilotState.Off) Set(AutopilotState.Paused, "press Resume to start");
        }

        public bool Resume()
        {
            if (State == AutopilotState.Off || State == AutopilotState.Running) return false;
            Set(AutopilotState.Running, null);
            return true;
        }

        public bool Pause(string why)
        {
            if (State != AutopilotState.Running) return false;
            Set(AutopilotState.Paused, why);
            return true;
        }

        public bool Fault(string why) => Stop(AutopilotState.Faulted, why);

        public bool Handover(string why) => Stop(AutopilotState.Handover, why);

        bool Stop(AutopilotState to, string why)
        {
            if (State != AutopilotState.Running) return false;
            Set(to, why ?? "unknown");
            return true;
        }

        void Set(AutopilotState s, string why)
        {
            State = s;
            Reason = why;
        }
    }

    public static class PaceKinds
    {
        public const string Move = "move";
        public const string Decision = "decision";
        public const string Dialogue = "dialogue";
        public const string Hold = "hold";
    }

    // Time between actions: pace_ms scaled by the kind of action, so a viewer can follow (docs/STREAM.md
    // "Pacing"). Moves go at half pace; a dialogue choice waits for its text to be read; a hold before a
    // big decision lets the panel or TTS land it.
    public sealed class Pace
    {
        public const int DefaultMs = 1200;
        public const int MinMs = 100;
        public const int MaxMs = 20000;
        public const int DefaultHoldMs = 4000;
        // Reading speed for dialogue pacing, characters per pace unit.
        const int CharsPerPace = 60;

        public int BaseMs = DefaultMs;
        public int HoldMs = DefaultHoldMs;

        public static int Parse(string s, int fallback)
        {
            if (string.IsNullOrWhiteSpace(s) || !int.TryParse(s.Trim(), out int v)) return fallback;
            return Math.Max(MinMs, Math.Min(MaxMs, v));
        }

        public int DelayMs(string kind, int chars = 0)
        {
            switch (kind)
            {
                case PaceKinds.Move: return BaseMs / 2;
                case PaceKinds.Dialogue:
                    double units = Math.Max(1.0, Math.Min(4.0, chars / (double)CharsPerPace));
                    return (int)(BaseMs * units);
                case PaceKinds.Hold: return Math.Max(BaseMs, Math.Min(MaxMs, HoldMs));
                default: return BaseMs;
            }
        }

        public static string KindOf(StepKind step)
        {
            switch (step)
            {
                case StepKind.Move: return PaceKinds.Move;
                case StepKind.Advance:
                case StepKind.Choose: return PaceKinds.Dialogue;
                case StepKind.ClosePanel:
                case StepKind.AugCheck: return PaceKinds.Hold;
                default: return PaceKinds.Decision;
            }
        }
    }

    // A stall is a fault: no action and no change in the game for this long while the planner waits.
    public sealed class Watchdog
    {
        public const long LimitMs = 60000;

        string signature;
        long sinceMs;

        public void Reset(long nowMs) => sinceMs = nowMs;

        // How long the view has held still; also the planner's quiet time.
        public long Quiet(string viewSignature, long nowMs)
        {
            if (viewSignature != signature)
            {
                signature = viewSignature;
                sinceMs = nowMs;
            }
            return nowMs - sinceMs;
        }

        public bool Stalled(long quietMs) => quietMs >= LimitMs;
    }

    // What an action must have changed, checked on the following ticks: a miss is a fault, never a retry.
    public sealed class Expectation
    {
        public string What;
        public Func<LiveView, bool> Met;
        public long DeadlineMs;

        public static Expectation Within(string what, long nowMs, long ms, Func<LiveView, bool> met) =>
            new Expectation { What = what, Met = met, DeadlineMs = nowMs + ms };

        public bool Missed(long nowMs) => nowMs > DeadlineMs;
    }
}
