using System;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using Fence.Bridge;
using Fence.Protocol;
using Mono.Cecil;
using Mono.Cecil.Cil;
using Xunit;

namespace Fence.Tests
{
    public class DecideRetryTests
    {
        static BridgeSession Advise()
        {
            var s = new BridgeSession();
            s.RequestHello();
            s.TryBeginHello(0);
            s.HelloSucceeded(new HelloResponse { SessionId = "s1", Protocol = ProtocolVersion.Current, Mode = Modes.Advise });
            return s;
        }

        static DecideRequest Req() => new DecideRequest
        {
            DecisionId = "d1", Kind = "barter", DeadlineMs = 2000,
            Options = new List<DecisionOption> { new DecisionOption { Id = "dismiss" } },
        };

        [Fact]
        public void UnknownSessionRehellosAndRetriesOnce()
        {
            var s = Advise();
            var t = s.BeginDecision(Req(), 0);
            var o = s.CompleteDecision(t, null, DecideFailure.UnknownSession, 10);
            Assert.True(o.RetryAfterHello);
            Assert.Equal(SessionState.NeedHello, s.State);

            Assert.False(s.BeginRetry(t, 20).Ask);
            Assert.True(s.TryBeginHello(30));
            s.HelloSucceeded(new HelloResponse { SessionId = "s2", Protocol = ProtocolVersion.Current, Mode = Modes.Advise });

            var retry = s.BeginRetry(t, 40);
            Assert.True(retry.Ask);
            Assert.Equal("s2", retry.Request.SessionId);
            Assert.Equal(t.StartedMs, retry.StartedMs);
            var again = s.CompleteDecision(retry, null, DecideFailure.UnknownSession, 50);
            Assert.False(again.RetryAfterHello);
            Assert.True(again.Choice.IsDefer);
        }

        [Fact]
        public void RetryRespectsTheOriginalDeadline()
        {
            var s = Advise();
            var t = s.BeginDecision(Req(), 0);
            s.CompleteDecision(t, null, DecideFailure.UnknownSession, 10);
            s.TryBeginHello(20);
            s.HelloSucceeded(new HelloResponse { SessionId = "s2", Protocol = ProtocolVersion.Current, Mode = Modes.Advise });
            Assert.False(s.BeginRetry(t, 2000 + 250).Ask);
        }

        [Fact]
        public void WantsDecisionsOnlyWhenAsking()
        {
            Assert.True(Advise().WantsDecisions(0));
            Assert.False(new BridgeSession().WantsDecisions(0));
        }
    }

    // P2: game actions are pressed only by Fence.Mod.BarterExecutor (counter) and Fence.Mod.LiveHands (the
    // autopilot's other presses), and only through an ActGate. Act-only names may otherwise appear only as
    // Harmony patch-target strings inside FencePatches. The gym executor's shared files (namespace Fence.Gym)
    // move items into machines themselves; the mod may not call those engines yet (M2).
    public class ActSafetyTests
    {
        const string Executor = "Fence.Mod.BarterExecutor";
        const string Hands = "Fence.Mod.LiveHands";
        const string Driver = "Fence.Mod.AutopilotDriver";
        const string Shared = "Fence.Gym";

        // The autopilot's presses outside the counter (docs/STREAM.md section 3).
        static readonly (string type, string member)[] AutopilotOnly =
        {
            ("StoreShutterButton", "OnClick"),
            ("StoreBell", "RingBell"),
            ("DialogUIManager", "SelectChoice"),
            ("DialogUIManager", "OnAdvancePressed"),
            ("StoreBedroll", "OnClick"),
            ("EndOfDayUIManager", "InitUI"),
            ("EndOfDayUIManager", "OnEndDayButtonClick"),
            ("AdvCalendarUIManager", "CloseUI"),
            ("ModHelper", "MoveItemToBackCounter"),
            ("ModHelper", "MoveItemToDisplayCase"),
            ("ModHelper", "MoveItemToInventory"),
            // M2: the phone, the door and the map, the return prompt, Wild's window, the appraisal panel.
            ("ModHelper", "MoveItemToAfterhours"),
            ("ModHelper", "ReturnFromAfterhour"),
            ("ModHelper", "CloseWildNetwork"),
            ("PhoneUIManager", "OpenUI"),
            ("PhoneUIManager", "AutoCall"),
            ("PhoneUIManager", "CloseUI"),
            ("DialoguePhoneUIManager", "SelectChoice"),
            ("DialoguePhoneUIManager", "OnAdvancePressed"),
            ("StoreDoor", "OnClick"),
            ("MapUIManager", "OnGoOutsideConfirm"),
            ("MapUIManager", "Travel"),
            ("MapUIManager", "ScavengeButtonClick"),
            ("GenericConfirmationUIManager", "ConfirmButtonPressed"),
            ("InspectionUIManager", "OnInspectItem"),
            ("InspectionUIManager", "ChangeAppraisalServicePrice"),
            ("InspectionUIManager", "OnAppraisalServiceEstimated"),
            // M3: the gun, on the executor's verdict from the visible tells.
            ("Shotgun", "OnClick"),
            ("StoreUIManager", "Shoot"),
        };

        static readonly (string type, string member)[] ActOnly =
        {
            ("NegociationUIManager", "OnBuyAllClick"),
            ("NegociationUIManager", "OnSellAllClick"),
            ("NegociationUIManager", "OnWholesaleClick"),
            ("NegociationUIManager", "OnConfirmPanelYes"),
            ("BargainUIManager", "OfferDiscount"),
            ("BargainUIManager", "OfferMarkup"),
            ("BargainUIManager", "OfferBuyingMarkup"),
            ("BargainUIManager", "DemandBuyingDiscount"),
            ("BargainUIManager", "Blackmail"),
            ("BargainUIManager", "ThreatenAug"),
            ("BargainUIManager", "RefreshUI"),
            ("PlayerStore", "TryPlayerDismissCurrentClient"),
        };

        // Out of scope for P2a (staging is unverified, exposing features without the tool is cheating).
        // Inspect and aug advice are advise-only: the mod never confronts, talks, ticks a toggle or estimates,
        // and never touches the gun.
        static readonly (string type, string member)[] Never =
        {
            ("InspectionUIManager", "OnConfront"),
            ("InspectionUIManager", "OnCigaretteConfront"),
            ("InspectionUIManager", "OnStampConfront"),
            ("InspectionUIManager", "OnInjectorConfront"),
            ("InspectionUIManager", "OnCigaretteTalk"),
            ("InspectionUIManager", "OnStampTalk"),
            ("InspectionUIManager", "FinishEditingAppraisalField"),
            ("InspectionUIManager", "OnValueChanged"),
            ("AppraisalServiceHelper", "OnEstimate"),
            ("InsCigaretteHelper", "CreateConfrontDialogue"),
            ("InsCigaretteHelper", "CreateTalkDialogue"),
            ("InsStampHelper", "CreateConfrontDialogue"),
            ("InsStampHelper", "CreateTalkDialogue"),
            ("InsInjectorHelper", "CreateConfrontDialogue"),
            ("UnityEngine.UI.Toggle", "set_isOn"),
            ("TMPro.TMP_InputField", "set_text"),
            ("ItemFeature", "ExposeFeature"),
            ("PlayerStore", "DismissCurrentClient"),
            ("PlayerStore", "SellItem"),
            ("PlayerStore", "BuyItem"),
            // aug_check advice never touches the gun; the autopilot raises it (Shotgun.OnClick) and fires (Shoot) only
            // through the hands, and never lowers it, kills, scans or reports.
            ("Shotgun", "Unlock"),
            ("StoreUIManager", "EnterShootingMode"),
            ("StoreUIManager", "ExitShootingMode"),
            ("StoreUIManager", "StartShootoutSequence"),
            ("AugHelper", "OnGoodKill"),
            ("AugHelper", "CleanupKill"),
            ("AugHelper", "OnAugLeftAlive"),
            ("AugHelper", "OnShootoutLeave"),
            ("PlayerStore", "OnWantedClientShot"),
            ("PlayerStore", "OnCustomerReported"),
            ("PowerHelper", "DrawPowerSource"),
        };

        static ModuleDefinition ModDll() => ModuleDefinition.ReadModule(Path.Combine(Paths.ModBin, "Fence.Mod.dll"));

        static IEnumerable<(TypeDefinition type, MethodDefinition method, Instruction ins)> Instructions(ModuleDefinition m) =>
            from t in m.GetTypes()
            from meth in t.Methods.Where(x => x.HasBody)
            from ins in meth.Body.Instructions
            select (t, meth, ins);

        static TypeDefinition Outermost(TypeDefinition t)
        {
            while (t.DeclaringType != null) t = t.DeclaringType;
            return t;
        }

        static List<string> Refs(ModuleDefinition m, (string type, string member)[] members, Func<TypeDefinition, bool> allowed)
        {
            var hits = new List<string>();
            foreach (var (type, method, ins) in Instructions(m))
            {
                if (!(ins.Operand is MemberReference mr) || mr.DeclaringType == null) continue;
                string declaring = mr.DeclaringType.GetElementType().FullName;
                if (members.Any(a => a.type == declaring && a.member == mr.Name) && !allowed(Outermost(type)))
                    hits.Add(type.FullName + "::" + method.Name + " " + ins.OpCode + " " + declaring + "::" + mr.Name);
            }
            return hits;
        }

        [Fact]
        public void ActOnlyMembersAreCalledOnlyByTheExecutor()
        {
            var m = ModDll();
            var hits = Refs(m, ActOnly, t => t.FullName == Executor);
            Assert.True(hits.Count == 0, "act-only members referenced outside BarterExecutor:\n" + string.Join("\n", hits));
            Assert.NotEmpty(Refs(m, ActOnly, _ => false));
        }

        [Fact]
        public void OutOfScopeMembersAreNeverCalled()
        {
            var hits = Refs(ModDll(), Never, t => t.Namespace == Shared);
            Assert.True(hits.Count == 0, "out-of-scope game actions referenced:\n" + string.Join("\n", hits));
        }

        [Fact]
        public void AutopilotPressesOnlyThroughTheHands()
        {
            var m = ModDll();
            var hits = Refs(m, AutopilotOnly, t => t.FullName == Hands);
            Assert.True(hits.Count == 0, "autopilot presses outside LiveHands:\n" + string.Join("\n", hits));
            Assert.Equal(AutopilotOnly.Length, AutopilotOnly.Count(a => Refs(m, new[] { a }, _ => false).Count > 0));
        }

        // Every press of the hands takes an ActGate first and re-checks it; only the driver calls them, and
        // the driver opens the autopilot's gate.
        [Fact]
        public void TheHandsAreReachableOnlyThroughAnOpenGate()
        {
            var m = ModDll();
            var hands = m.GetType(Hands);
            Assert.True(hands.NestedTypes.Count == 0, "no closures in the hands: every game call must sit behind the gate check");
            var entries = hands.Methods.Where(x => x.IsPublic && !x.IsConstructor).ToList();
            Assert.NotEmpty(entries);
            foreach (var e in entries)
            {
                Assert.Equal("Fence.Barter.ActGate", e.Parameters[0].ParameterType.FullName);
                Assert.Contains(e.Body.Instructions, i => i.Operand is MethodReference r && r.DeclaringType.Name == "ActGate" && r.Name == "get_StillOpen");
            }
            var callers = Instructions(m).Where(x => x.ins.Operand is MethodReference r && r.DeclaringType.FullName == Hands && !r.Name.StartsWith(".ctor"))
                .Select(x => Outermost(x.type).FullName).Where(t => t != Hands).Distinct().ToList();
            Assert.Equal(new[] { Driver }, callers);
            Assert.Contains(Instructions(m), x => Outermost(x.type).FullName == Driver && x.ins.Operand is MethodReference r && r.DeclaringType.Name == "ActGate" && r.Name == "TryOpenAutopilot");
        }

        // The engines' chores (the shared methods that take an IExecutorHost, and the between-customers stow) run
        // only from the driver, inside one of its gated actions.
        [Fact]
        public void EngineChoresRunOnlyFromTheDriver()
        {
            var calls = Instructions(ModDll())
                .Where(x => Outermost(x.type).Namespace != Shared && x.ins.Operand is MethodReference r && r.DeclaringType.Namespace == Shared
                            && (r.Parameters.Any(p => p.ParameterType.Name == "IExecutorHost") || (r.DeclaringType.Name == "Production" && r.Name == "Stow")))
                .ToList();
            Assert.NotEmpty(calls);
            var hits = calls.Where(x => Outermost(x.type).FullName != Driver)
                .Select(x => x.type.FullName + "::" + x.method.Name + " -> " + ((MethodReference)x.ins.Operand).FullName).ToList();
            Assert.True(hits.Count == 0, "engine chores called outside the driver:\n" + string.Join("\n", hits));
        }

        // Every item move a shared engine makes goes through Fence.Gym.Chore (live: the gated hands); the raw
        // move appears only there (its headless branch) and in the hands.
        [Fact]
        public void GameMovesGoThroughChoreOrTheHands()
        {
            var hits = Refs(ModDll(), new[] { ("GraphUtils", "TryAcceptAll") }, t => t.FullName == Hands || t.FullName == "Fence.Gym.Chore");
            Assert.True(hits.Count == 0, "raw item moves outside Chore and the hands:\n" + string.Join("\n", hits));
        }

        [Fact]
        public void ActOnlyNamesAppearOnlyAsPatchTargets()
        {
            var names = ActOnly.Concat(Never).Concat(AutopilotOnly).Select(a => a.member).ToHashSet();
            var hits = Instructions(ModDll())
                .Where(x => x.ins.OpCode.Code == Code.Ldstr && names.Contains((string)x.ins.Operand))
                .Where(x => Outermost(x.type).FullName != "Fence.Mod.FencePatches")
                .Select(x => x.type.FullName + "::" + x.method.Name + " \"" + x.ins.Operand + "\"").ToList();
            Assert.True(hits.Count == 0, "act-only names outside FencePatches:\n" + string.Join("\n", hits));
        }

        // The executor's only way in takes an ActGate and re-checks it; its only caller opens the gate first.
        [Fact]
        public void TheExecutorIsReachableOnlyThroughAnOpenGate()
        {
            var m = ModDll();
            var exec = m.GetType(Executor);
            var entries = exec.Methods.Where(x => !x.IsPrivate && !x.IsConstructor).ToList();
            Assert.Equal(new[] { "Execute" }, entries.Select(x => x.Name));
            var execute = entries[0];
            Assert.Equal("Fence.Barter.ActGate", execute.Parameters[0].ParameterType.FullName);
            Assert.Contains(execute.Body.Instructions, i => i.Operand is MethodReference r && r.DeclaringType.Name == "ActGate" && r.Name == "get_StillOpen");
            Assert.True(exec.NestedTypes.Count == 0, "no closures in the executor: every game call must sit behind the gate check");

            var callers = Instructions(m).Where(x => x.ins.Operand is MethodReference r && r.DeclaringType.FullName == Executor && r.Name == "Execute")
                .Select(x => x.method).Distinct().ToList();
            Assert.Single(callers);
            Assert.Contains(callers[0].Body.Instructions, i => i.Operand is MethodReference r && r.DeclaringType.Name == "ActGate" && r.Name == "TryOpen");
        }

        [Fact]
        public void PatchesAreAppliedByHandWithOurOwnId()
        {
            var m = ModDll();
            Assert.DoesNotContain(m.GetTypes().SelectMany(t => t.CustomAttributes), a => a.AttributeType.FullName == "HarmonyLib.HarmonyPatch");
            var unpatch = Instructions(m).Where(x => x.ins.Operand is MethodReference r && r.DeclaringType.FullName == "HarmonyLib.Harmony" && r.Name.StartsWith("Unpatch")).ToList();
            Assert.NotEmpty(unpatch);
            foreach (var (_, method, ins) in unpatch)
            {
                var arg = ins.Previous;
                Assert.True(arg.OpCode.Code == Code.Ldstr && (string)arg.Operand == "mod.fence", method.Name + " unpatches without the loader's mod.fence id");
            }
        }
    }
}
