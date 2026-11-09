using System;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using System.Text.Json;
using Fence.Barter;
using Fence.Decisions;
using Fence.Mod;
using Fence.Protocol;
using Fence.Recording;
using Newtonsoft.Json.Linq;

namespace Fence.Gym
{
    // The live mod's half of the protocol, played by the gym: the same hello, event kinds, decide
    // requests (built by Fence.Core from Fence.Mod.GameBarter reads) and outcome shapes, sent to a
    // bridge and/or the gym's own trajectory.
    internal sealed class ModSide
    {
        // Marks sim data wherever a mod version is recorded (hello, fencedata sessions).
        public const string VersionSuffix = "+sim";

        readonly IBridge bridge;
        readonly Trajectory trajectory;
        readonly string decisionPrefix;
        public readonly GameBarter Game = new GameBarter();
        public readonly GameDay DayReads = new GameDay();
        long seq;
        int decisions;

        public string SessionId { get; private set; }
        public string Mode { get; private set; } = Modes.Act;
        public string PolicyName { get; set; }
        public int EventsSent { get; private set; }
        public int DecisionsAsked { get; private set; }
        public int DecisionFailures { get; private set; }
        public SummaryResponse LastSummary { get; private set; }
        public Action<string, Exception> SnapshotError;
        readonly HashSet<string> failedSections = new HashSet<string>(StringComparer.Ordinal);

        public ModSide(IBridge bridge, Trajectory trajectory, int seed)
        {
            this.bridge = bridge;
            this.trajectory = trajectory;
            decisionPrefix = "d-sim-s" + seed + "-";
        }

        public static int CurrentDay() => StoreStation.Instance != null ? StoreStation.Instance.dayCounter : 0;

        public void Hello(JObject sim)
        {
            var (version, mvid) = GameVersion();
            var hello = new HelloRequest
            {
                ModVersion = typeof(BarterContext).Assembly.GetName().Version.ToString(3) + VersionSuffix,
                Game = new GameInfo { Version = version, Unity = "2021.3.45f2", AssemblySha256 = Sim.Headless.Sha256(Path.Combine(Boot.GameDir, "Probably Stolen_Data", "Managed", "Assembly-CSharp.dll")), AssemblyMvid = mvid },
                Compat = new HelloCompat { Ok = true },
            };
            var resp = bridge.Hello(hello);
            SessionId = resp.SessionId;
            Mode = resp.Mode;
            PollSummary();
            trajectory?.Hello(SessionId, Mode, PolicyName, hello.ModVersion, version);
            // The gym's run parameters, as an event until hello has a field for them.
            Emit("sim_run", sim);
        }

        // compat/versions/*.json names the build the installed Assembly-CSharp belongs to.
        static (string version, string mvid) GameVersion()
        {
            string sha = Sim.Headless.Sha256(Path.Combine(Boot.GameDir, "Probably Stolen_Data", "Managed", "Assembly-CSharp.dll"));
            var dir = Path.Combine(Boot.Repo, "compat", "versions");
            if (System.IO.Directory.Exists(dir))
                foreach (var f in System.IO.Directory.GetFiles(dir, "*.json").OrderBy(x => x, StringComparer.Ordinal))
                {
                    try
                    {
                        using var doc = JsonDocument.Parse(File.ReadAllBytes(f));
                        var a = doc.RootElement.GetProperty("assembly_csharp");
                        if (a.GetProperty("sha256").GetString() == sha)
                            return (doc.RootElement.GetProperty("app_version").GetString(), a.GetProperty("mvid").GetString());
                    }
                    catch (Exception) { }
                }
            return ("unknown", null);
        }

        public void Emit(string kind, JObject data, JObject hidden = null)
        {
            var e = new EventRequest { SessionId = SessionId, Seq = ++seq, Kind = kind, GameDay = CurrentDay(), Data = data ?? new JObject(), Hidden = hidden != null && hidden.HasValues ? hidden : null };
            trajectory?.Event(e);
            bridge.Event(e);
            EventsSent++;
        }

        public void Snapshot(string phase)
        {
            var r = DaySnapshot.Build(DayReads, phase, GameDay.RunId(), null);
            foreach (var (section, error) in r.Failed)
                if (failedSections.Add(section)) SnapshotError?.Invoke(section, error);
            Emit(RecordKinds.DaySnapshot, r.Data, r.Hidden);
        }

        public string NextDecisionId() => decisionPrefix + (++decisions);

        // Returns the validated choice (defer on any failure), as the mod's BridgeClient + ChoiceValidator do.
        public Choice Decide(DecideRequest request, out DecideResponse response, out string reason)
        {
            request.SessionId = SessionId;
            DecisionsAsked++;
            response = bridge.Decide(request, out long latency);
            if (response == null) DecisionFailures++;
            trajectory?.Decision(request, response != null ? JObject.Parse(ProtocolJson.Serialize(response)) : null, Mode, PolicyName, bridge is LocalBridge ? 0 : latency);
            return ChoiceValidator.Validate(request, response, out reason);
        }

        public void Outcome(string decisionId, JObject result)
        {
            if (decisionId == null || result == null) return;
            var o = new OutcomeRequest { SessionId = SessionId, DecisionId = decisionId, GameDay = CurrentDay(), Result = result };
            trajectory?.Outcome(o);
            bridge.Outcome(o);
        }

        // The overlay's summary poll (on arrival, waking up, after outcomes in the mod).
        public void PollSummary()
        {
            var s = bridge.Summary(SessionId);
            if (s == null) return;
            LastSummary = s;
            if (!string.IsNullOrEmpty(s.Policy)) PolicyName = s.Policy;
        }

        public void Human(string action, AdviceInForce advice, int? itemUid = null, JObject extra = null, string defaultKey = null)
        {
            var data = HumanActions.Build(action, advice, itemUid, extra);
            if (defaultKey != null) data["sim_default"] = defaultKey;
            Emit(RecordKinds.HumanAction, data);
        }

        // RunStats funnels the live mod records as `money` (FencePatches.Record).
        public static readonly Dictionary<string, (string kind, string[] names)> MoneyHooks = new Dictionary<string, (string, string[])>
        {
            ["OnServicePaid"] = (MoneyKinds.ServicePaid, new[] { "service", "cost" }),
            ["OnRentPaid"] = (MoneyKinds.RentPaid, new[] { "amount", "credits_left" }),
            ["OnRentMissed"] = (MoneyKinds.RentMissed, new string[0]),
            ["OnStoreBoughtWithCash"] = (MoneyKinds.StoreBought, new[] { "price" }),
            ["OnMortgageTaken"] = (MoneyKinds.MortgageTaken, new[] { "price", "down_payment", "weekly", "weeks" }),
            ["OnMortgagePayment"] = (MoneyKinds.MortgagePayment, new[] { "amount", "credits_left" }),
            ["OnMortgagePaidOff"] = (MoneyKinds.MortgagePaidOff, new string[0]),
            ["OnGeorgioLoan"] = (MoneyKinds.GeorgioLoan, new[] { "amount" }),
            ["OnGeorgioPayback"] = (MoneyKinds.GeorgioPayback, new[] { "amount" }),
            ["OnVendingSale"] = (MoneyKinds.VendingSale, new[] { "value" }),
            ["OnFountainSale"] = (MoneyKinds.FountainSale, new[] { "value" }),
            ["OnBurglary"] = (MoneyKinds.Burglary, new[] { "value" }),
            ["OnDayEnded"] = (MoneyKinds.DayEnded, new[] { "day", "net_worth", "credits" }),
            ["OnInspectorBribe"] = (MoneyKinds.InspectorBribe, new[] { "amount" }),
            ["OnPermitBought"] = (MoneyKinds.PermitBought, new[] { "price" }),
            ["OnPermitBribe"] = (MoneyKinds.PermitBribe, new[] { "value" }),
            ["OnWildUpgradeBought"] = (MoneyKinds.WildUpgrade, new[] { "cash_spent" }),
        };

        public void Money(string method, object[] args)
        {
            if (!MoneyHooks.TryGetValue(method, out var m)) return;
            JObject amounts = null;
            if (m.names.Length > 0)
            {
                amounts = new JObject();
                for (int i = 0; i < m.names.Length && i < args.Length; i++) amounts[m.names[i]] = args[i] == null ? JValue.CreateNull() : JToken.FromObject(args[i]);
            }
            Emit(RecordKinds.Money, RunEvents.Money(m.kind, amounts));
        }
    }
}
