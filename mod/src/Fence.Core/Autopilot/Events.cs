using System.Collections.Generic;
using System.Linq;
using Newtonsoft.Json.Linq;

namespace Fence.Autopilot
{
    // One autopilot action as the decision log records it (docs/PROTOCOL.md, "Autopilot events").
    public sealed class AutopilotAction
    {
        public int Day;
        public string Phase;
        public string Action;
        public int? TargetUid;
        public string TargetName;
        public string Detail;
        public string Why;
        public string Rule;
        public int? PlanDay;
        public int ClientSeq;
        public string Client;
        // Names of hidden values the rule read (never the values): the panel marks the entry.
        public IList<string> HiddenUsed;
    }

    public static class AutopilotEvents
    {
        public const string Action = "autopilot";
        public const string Fault = "autopilot_fault";

        public static JObject Build(AutopilotAction a)
        {
            var o = new JObject { ["day"] = a.Day, ["phase"] = a.Phase, ["action"] = a.Action };
            if (a.TargetUid.HasValue || a.TargetName != null)
            {
                var t = new JObject();
                if (a.TargetUid.HasValue) t["uid"] = a.TargetUid.Value;
                if (a.TargetName != null) t["name"] = a.TargetName;
                o["target"] = t;
            }
            if (!string.IsNullOrEmpty(a.Detail)) o["detail"] = a.Detail;
            o["why"] = a.Why ?? "";
            if (!string.IsNullOrEmpty(a.Rule)) o["rule"] = a.Rule;
            if (a.PlanDay.HasValue) o["plan_day"] = a.PlanDay.Value;
            if (a.ClientSeq > 0) o["client_seq"] = a.ClientSeq;
            if (!string.IsNullOrEmpty(a.Client)) o["client"] = a.Client;
            o["hidden_used"] = new JArray((a.HiddenUsed ?? new string[0]).Distinct().OrderBy(x => x, System.StringComparer.Ordinal).Cast<object>().ToArray());
            return o;
        }

        // The autopilot stopped and needs a human: `state` is faulted or handover.
        public static JObject BuildFault(int day, string phase, AutopilotState state, string reason, string action, string client, string clientKind)
        {
            var o = new JObject
            {
                ["day"] = day,
                ["phase"] = phase,
                ["state"] = state == AutopilotState.Handover ? "handover" : "faulted",
                ["reason"] = reason ?? "",
                ["message"] = Banner(state, reason),
            };
            if (!string.IsNullOrEmpty(action)) o["action"] = action;
            if (!string.IsNullOrEmpty(client)) o["client"] = client;
            if (!string.IsNullOrEmpty(clientKind)) o["client_kind"] = clientKind;
            return o;
        }

        // The overlay's line for a stop; the same words go to the bridge so the panel can say them.
        public static string Banner(AutopilotState state, string reason)
        {
            switch (state)
            {
                case AutopilotState.Handover: return "AUTOPILOT PAUSED — your turn: " + reason + ". Play it by hand, then press Resume.";
                case AutopilotState.Faulted: return "AUTOPILOT STOPPED — " + reason + ". Check the game, then press Resume.";
                case AutopilotState.Paused: return "Autopilot paused" + (string.IsNullOrEmpty(reason) ? "" : ": " + reason);
                case AutopilotState.Off: return "";
                default: return "Autopilot running";
            }
        }

        public static string PhaseOf(LiveView v)
        {
            if (v == null || !v.InStore) return "none";
            if (v.Outside) return "outing";
            switch (v.Store)
            {
                case StorePhase.Morning: return "morning";
                case StorePhase.Open: return v.ClientPresent ? "customer" : "open";
                case StorePhase.Closed: return "night";
                default: return "unknown";
            }
        }
    }
}
