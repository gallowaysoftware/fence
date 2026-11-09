using System;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using System.Reflection;
using Fence.Barter;
using Fence.Bridge;
using Fence.Compat;
using Fence.Protocol;
using Newtonsoft.Json.Linq;
using Xunit;

namespace Fence.Tests
{
    public class ActGateTests
    {
        [Theory]
        [InlineData("act", true, false, false, true)]
        [InlineData("advise", true, false, false, false)]
        [InlineData("observe", true, false, false, false)]
        [InlineData(null, true, false, false, false)]
        [InlineData("act", false, false, false, false)]
        [InlineData("act", true, true, false, false)]
        [InlineData("act", true, false, true, false)]
        public void OpensOnlyWithActModeTheSettingAndNoBrake(string mode, bool setting, bool paused, bool faulted, bool open)
        {
            var sw = new ActSwitch();
            if (paused) sw.Pause();
            if (faulted) sw.Fault("boom");
            Assert.Equal(open, ActGate.TryOpen(() => mode, () => setting, sw) != null);
            Assert.Equal(open, ActGate.Allows(mode, setting, sw));
        }

        [Fact]
        public void NoSwitchNoGate() => Assert.Null(ActGate.TryOpen(() => Modes.Act, () => true, null));

        [Fact]
        public void TheOnlyWayToAGateIsTryOpen() =>
            Assert.Empty(typeof(ActGate).GetConstructors(BindingFlags.Public | BindingFlags.Instance));

        [Fact]
        public void StillOpenFollowsEveryCondition()
        {
            string mode = Modes.Act;
            bool setting = true;
            var sw = new ActSwitch();
            var gate = ActGate.TryOpen(() => mode, () => setting, sw)!;
            Assert.True(gate.StillOpen);
            setting = false;
            Assert.False(gate.StillOpen);
            setting = true;
            mode = Modes.Advise;
            Assert.False(gate.StillOpen);
            mode = Modes.Act;
            sw.Pause();
            Assert.False(gate.StillOpen);
            sw.Resume();
            Assert.True(gate.StillOpen);
            sw.Fault("x");
            sw.Resume();
            Assert.False(gate.StillOpen);
            Assert.True(sw.Faulted);
        }
    }

    public class ActCommandTests
    {
        [Fact]
        public void EveryOfferableOptionParsesBack()
        {
            var ids = new List<string> { OptionIds.SellAll, OptionIds.BuyAll, OptionIds.AcceptWholesale, OptionIds.Dismiss, OptionIds.Blackmail, OptionIds.Threaten };
            foreach (BargainKind k in new[] { BargainKind.Discount, BargainKind.Markup, BargainKind.BuyingMarkup, BargainKind.BuyingDiscount })
                ids.AddRange(BarterRules.PricePercents.Select(p => OptionIds.Bargain(k, p)));
            foreach (var id in ids)
            {
                var c = ActCommands.Parse(id);
                Assert.NotNull(c);
                Assert.Equal(id, c.Kind == ActKind.Bargain ? OptionIds.Bargain(c.Bargain, c.Percent) : c.OptionId);
            }
            Assert.Equal(ActKind.Bargain, ActCommands.Parse("demand_discount_75")!.Kind);
            Assert.Equal(75, ActCommands.Parse("demand_discount_75")!.Percent);
            Assert.Equal(BargainKind.Threaten, ActCommands.Parse("threaten")!.Bargain);
        }

        [Theory]
        [InlineData(null)]
        [InlineData("")]
        [InlineData("defer")]
        [InlineData("offer_discount_15")]
        [InlineData("stage_item")]
        [InlineData("expose_feature")]
        public void AnythingElseIsNotAnAction(string id) => Assert.Null(ActCommands.Parse(id));

        [Fact]
        public void RequiredHooksAreInTheManifestUnderAnActFeature()
        {
            var hooks = HookManifest.Parse(File.ReadAllText(Paths.HooksJson)).Hooks;
            foreach (var id in new[] { "sell_all", "buy_all", "accept_wholesale", "dismiss", "blackmail", "threaten", "offer_discount_10", "add_markup_25", "offer_markup_50", "demand_discount_75" })
            foreach (var hook in ActCommands.RequiredHooks(ActCommands.Parse(id)))
                Assert.True(hooks.Any(h => h.Id == hook), id + " needs " + hook + ", absent from hooks.json");
            var actHooks = hooks.Where(h => h.Features.Any(f => f.StartsWith("act."))).Select(h => h.Id).ToHashSet();
            foreach (var h in new[] { "barter.sell_all", "barter.buy_all", "barter.wholesale", "barter.confirm_over_budget", "barter.bargain_threaten", "act.client_has_budget", "act.bargain_refresh_ui" })
                Assert.Contains(h, actHooks);
            Assert.Contains("act.barter.wholesale", hooks.Single(h => h.Id == "barter.wholesale").Features);
            Assert.Contains("act.barter.threaten", hooks.Single(h => h.Id == "barter.bargain_threaten").Features);
        }

        // A missing act member blocks only the options that need it; advice never depends on act hooks.
        [Fact]
        public void AMissingActMemberBlocksOnlyItsOption()
        {
            var hooks = HookManifest.Parse(File.ReadAllText(Paths.HooksJson)).Hooks;
            Func<ActCommand, bool> withoutWholesale = c => !ActCommands.RequiredHooks(c).Contains("barter.wholesale");
            Assert.False(withoutWholesale(ActCommands.Parse("accept_wholesale")!));
            Assert.True(withoutWholesale(ActCommands.Parse("buy_all")!));
            foreach (var h in hooks.Where(h => h.Features.All(f => f.StartsWith("act."))))
                Assert.DoesNotContain("advise.barter", h.Features);
        }
    }

    public class ActPlannerTests
    {
        static (FakeBarterGame game, BarterSnapshot snap, Advice pending) Setup(string option = "sell_all")
        {
            var g = new FakeBarterGame();
            var snap = BarterContext.Read(g, 3);
            var pending = new Advice { DecisionId = "d1", Signature = snap.Signature, OptionId = option, ReceivedMs = 1000, Act = ActState.Pending };
            return (g, snap, pending);
        }

        static ActVerdict Eval(FakeBarterGame g, Advice p, long now, Func<ActCommand, bool> exec = null, ActLimiter lim = null, object key = null) =>
            ActPlanner.Evaluate(p, key ?? g.ClientState.Key, BarterContext.Read(g, 3), now, exec ?? (_ => true), lim ?? new ActLimiter());

        [Fact]
        public void SameStateExecutes()
        {
            var (g, _, p) = Setup();
            Assert.Equal(ActVerdictKind.Execute, Eval(g, p, 1200).Kind);
        }

        [Fact]
        public void AHumanClickChangesTheSignatureAndWins()
        {
            var (g, _, p) = Setup();
            g.Flags.MarkupOffered = true;
            var v = Eval(g, p, 1200);
            Assert.Equal(ActVerdictKind.Stale, v.Kind);
            Assert.Equal("state changed", v.Reason);
        }

        [Fact]
        public void APriceChangeIsStale()
        {
            var (g, _, p) = Setup();
            g.Items[0].Price = 341;
            Assert.Equal(ActVerdictKind.Stale, Eval(g, p, 1200).Kind);
        }

        [Fact]
        public void AnotherCustomerIsStale()
        {
            var (g, _, p) = Setup();
            Assert.Equal("customer changed", Eval(g, p, 1200, key: new object()).Reason);
        }

        [Fact]
        public void DialogueWaitsThenGoesStale()
        {
            var (g, _, p) = Setup();
            g.UiState.DialoguePlaying = true;
            Assert.Equal(ActVerdictKind.Wait, Eval(g, p, 1000 + ActPlanner.ReadyGraceMs - 1).Kind);
            Assert.Equal(ActVerdictKind.Stale, Eval(g, p, 1000 + ActPlanner.ReadyGraceMs).Kind);
            g.UiState.DialoguePlaying = false;
            Assert.Equal(ActVerdictKind.Execute, Eval(g, p, 1000 + ActPlanner.ReadyGraceMs).Kind);
        }

        // Signature unchanged but the UI gate closed (lock bell): the fresh option set no longer has dismiss.
        [Fact]
        public void AClosedUiGateDropsTheOption()
        {
            var (g, _, p) = Setup("dismiss");
            Assert.Equal(ActVerdictKind.Execute, Eval(g, p, 1200).Kind);
            g.UiState.LockNextBell = 1;
            var v = Eval(g, p, 1200);
            Assert.Equal(ActVerdictKind.Stale, v.Kind);
            Assert.Equal("option no longer offered", v.Reason);
        }

        [Fact]
        public void BargainPanelGatesAreReappliedAtExecution()
        {
            var (g, _, p) = Setup("add_markup_10");
            Assert.Equal(ActVerdictKind.Execute, Eval(g, p, 1200).Kind);
            g.UiState.AwfulNegotiator = true;
            Assert.Equal(ActVerdictKind.Stale, Eval(g, p, 1200).Kind);
        }

        [Fact]
        public void UncoveredCustomerIsStale()
        {
            var (g, _, p) = Setup();
            g.ClientState.IsWanted = true;
            Assert.Equal(ActVerdictKind.Stale, Eval(g, p, 1200).Kind);
        }

        [Fact]
        public void MissingMemberOrLimitsBlockButKeepTheAdvice()
        {
            var (g, _, p) = Setup();
            Assert.Equal(ActVerdictKind.Blocked, Eval(g, p, 1200, _ => false).Kind);
            var lim = new ActLimiter();
            for (int i = 0; i < ActLimiter.PerCustomer; i++) lim.Record(i * 10000);
            var v = Eval(g, p, 100000, lim: lim);
            Assert.Equal(ActVerdictKind.Blocked, v.Kind);
            Assert.Contains("this customer", v.Reason);
        }
    }

    public class ActLimiterTests
    {
        [Fact]
        public void CapsPerCustomerPerMinuteAndSettles()
        {
            var l = new ActLimiter();
            Assert.Null(l.Refusal(0));
            l.Record(0);
            Assert.True(l.Settling(ActLimiter.SettleMs - 1));
            Assert.Contains("settling", l.Refusal(ActLimiter.SettleMs - 1));
            Assert.Null(l.Refusal(ActLimiter.SettleMs));
            for (int i = 1; i < ActLimiter.PerCustomer; i++) l.Record(i * 2000);
            Assert.NotNull(l.Refusal(20000));
            l.NewClient();
            Assert.Null(l.Refusal(20000));


            var m = new ActLimiter();
            long t = 0;
            for (int i = 0; i < ActLimiter.PerMinute; i++, t += 2000)
            {
                if (m.CustomerCount == ActLimiter.PerCustomer) m.NewClient();
                Assert.Null(m.Refusal(t));
                m.Record(t);
            }
            m.NewClient();
            Assert.Contains("minute", m.Refusal(t));
            Assert.Null(m.Refusal(60000));
        }
    }

    public class AttributionTests
    {
        [Fact]
        public void ModOnlyWhileExecutingThenConsequencesFollowTheLastMove()
        {
            var a = new Attribution();
            Assert.Equal(("d2", Actors.Human), a.Consequence("d2"));
            a.BeginMod("d1");
            Assert.True(a.Executing);
            Assert.Equal(("d1", Actors.Mod), a.Direct("d2"));
            a.EndMod();
            Assert.Equal(("d1", Actors.Mod), a.Consequence("d3"));
            Assert.Equal(("d3", Actors.Human), a.Direct("d3"));
            Assert.Equal(("d3", Actors.Human), a.Consequence("d4"));
            a.NewClient();
            Assert.Equal(("d5", Actors.Human), a.Consequence("d5"));
        }

        [Fact]
        public void TradesCarryTheirActorAndDecision()
        {
            var b = new TradeBatch();
            b.Begin(TradeBatch.Sold, new List<FactionRep>(), "d7", Actors.Mod);
            b.Add(1, 50);
            Assert.Equal("d7", b.DecisionId);
            var r = b.Flush(new List<FactionRep>());
            Assert.Equal("mod", (string)r["actor"]);
            Assert.Equal("human", (string)Outcomes.WithActor(Outcomes.Left("dismissed"), Actors.Human)["actor"]);
        }
    }

    public class AskAgainTests
    {
        [Fact]
        public void AskAgainReasksTheCurrentSignatureAndPendingIsTheNewest()
        {
            var t = new AdviceTracker();
            t.NewClient(new object());
            t.Observe("a", 0, true);
            t.Sent(t.Observe("a", 800, true), "d1");
            Assert.False(t.AskAgain());
            t.Answered(new Advice { Signature = "a", DecisionId = "d1", OptionId = "sell_all", Act = ActState.Pending });
            Assert.Equal("d1", t.PendingAct!.DecisionId);
            Assert.Null(t.Observe("a", 900, true));
            Assert.True(t.AskAgain());
            Assert.Equal("a", t.Observe("a", 950, true));
            t.Sent("a", "d2");
            t.Answered(new Advice { Signature = "a", DecisionId = "d2", OptionId = "dismiss" });
            Assert.Null(t.PendingAct);
        }
    }

    public class OverlayTests
    {
        static ClientInfo Client() => new FakeBarterGame().ClientState;

        static readonly List<DecisionOption> Opts = new List<DecisionOption>
        {
            new DecisionOption { Id = "sell_all", Label = "Sell 1 item for 340" },
            new DecisionOption { Id = "offer_discount_10", Label = "Offer 10% discount (always accepted)" },
            new DecisionOption { Id = "add_markup_10", Label = "Add 10% markup (accept 82%)" },
            new DecisionOption { Id = "dismiss", Label = "Send the customer away" },
        };

        static Advice A(ActState act = ActState.None) => new Advice
        {
            DecisionId = "d1", Signature = "s", OptionId = "sell_all", Label = "Sell 1 item for 340", Rationale = "plain sale <b>wins</b>",
            Options = Opts, Notes = new Dictionary<string, string> { ["add_markup_10"] = "expected 333", ["offer_discount_10"] = "306", ["ghost"] = "x" }, Act = act,
        };

        static OverlayInput In(AdviceStatus status, Advice a, ActDisplay act = ActDisplay.Advise) => new OverlayInput
        {
            Mode = act == ActDisplay.Advise ? Modes.Advise : Modes.Act, Act = act, Bridge = BridgeStatus.Connected, Client = Client(), Ready = true,
            View = new AdviceView { Status = status, Advice = a }, Today = new List<DayEntry>(),
        };

        [Theory]
        [InlineData(AdviceStatus.Ready, ActState.None, ActDisplay.Advise, "advice", "fence suggests: Sell 1 item for 340")]
        [InlineData(AdviceStatus.Ready, ActState.Pending, ActDisplay.Active, "acting…", "fence will: Sell 1 item for 340")]
        [InlineData(AdviceStatus.Ready, ActState.Executed, ActDisplay.Active, "done", "fence did: Sell 1 item for 340")]
        [InlineData(AdviceStatus.Stale, ActState.Executed, ActDisplay.Active, "done — waiting for the counter", "fence did: Sell 1 item for 340")]
        [InlineData(AdviceStatus.Stale, ActState.None, ActDisplay.Advise, "stale — state changed", "was: Sell 1 item for 340")]
        [InlineData(AdviceStatus.Ready, ActState.Blocked, ActDisplay.Paused, "advice", "fence suggests: Sell 1 item for 340 (not acting: paused)")]
        public void StatusAndMove(AdviceStatus status, ActState act, ActDisplay display, string expectStatus, string expectMove)
        {
            var a = A(act);
            if (act == ActState.Blocked) a.ActNote = "paused";
            var v = OverlayText.Build(In(status, a, display));
            Assert.Equal(expectStatus, v.Status);
            Assert.Equal(expectMove, v.Move);
        }

        [Fact]
        public void StaleActionSaysDropped()
        {
            var a = A(ActState.Stale);
            a.ActNote = "state changed";
            var v = OverlayText.Build(In(AdviceStatus.Ready, a, ActDisplay.Active));
            Assert.Equal("stale — dropped", v.Status);
            Assert.Equal("dropped (state changed): Sell 1 item for 340", v.Move);
        }

        [Fact]
        public void OptionsInOurOrderWithNotesAndTheChoiceMarked()
        {
            var v = OverlayText.Build(In(AdviceStatus.Ready, A()));
            Assert.Equal(
                "▶ Sell 1 item for 340\n  Offer 10% discount (always accepted) — 306\n  Add 10% markup (accept 82%) — expected 333\n  Send the customer away",
                v.Options);
            Assert.Equal("plain sale ‹b›wins‹/b›", v.Rationale);
        }

        [Theory]
        [InlineData(ActDisplay.Advise, "advise", null)]
        [InlineData(ActDisplay.ConfigOff, "advise (act off in settings)", null)]
        [InlineData(ActDisplay.Active, "act", "Pause acting")]
        [InlineData(ActDisplay.Paused, "act paused", "Resume acting")]
        [InlineData(ActDisplay.Faulted, "act off (error)", "Acting off (error)")]
        public void HeaderAndPauseButton(ActDisplay act, string mode, string pause)
        {
            var s = In(AdviceStatus.None, null, act);
            s.Summary = new SummaryResponse { Model = "qwen38fn-mlx", Day = 69, Today = new DayStats() };
            var v = OverlayText.Build(s);
            Assert.StartsWith(mode + " · connected · qwen38fn-mlx", v.Header);
            Assert.Equal(pause, v.PauseLabel);
            Assert.Equal(act == ActDisplay.Active || act == ActDisplay.Paused, v.PauseInteractable);
        }

        [Fact]
        public void CustomerStates()
        {
            var s = In(AdviceStatus.None, null);
            Assert.Equal("Retired farmer · lower_levels · buying · budget 900", OverlayText.Build(s).Customer);
            Assert.Equal("waiting for the counter", OverlayText.Build(s).Status);
            s.View = new AdviceView { Status = AdviceStatus.None, Deferred = true };
            Assert.Equal("deferred to you", OverlayText.Build(s).Status);
            s.View = new AdviceView { Status = AdviceStatus.Thinking };
            Assert.Equal("thinking…", OverlayText.Build(s).Status);
            s.CoverageGap = "security";
            var v = OverlayText.Build(s);
            Assert.Equal("not covered: security", v.Status);
            Assert.Equal("", v.Move);
            s.Client = null;
            Assert.Equal("No customer at the counter", OverlayText.Build(s).Customer);
        }

        [Fact]
        public void DaySummaryNoDataBridgeDownAndLessons()
        {
            var s = In(AdviceStatus.None, null);
            var fixture = ProtocolJson.Deserialize<SummaryResponse>(File.ReadAllText(Path.Combine(Paths.Fixtures, "summary.response.json")));
            s.Summary = fixture;
            var v = OverlayText.Build(s);
            Assert.Equal("Day 69: 7 decisions · 3 sales · 2 purchases · cash +415 · 1 walk-out\nrep black_market -1.5, lower_levels +0.8", v.Summary);
            Assert.StartsWith("Lessons (day 68):\nMarkups above 10%", v.Lessons);
            Assert.Contains("\nKeep 600 for rent", v.Lessons);

            s.Summary = new SummaryResponse { Day = 0, Today = new DayStats { RepDelta = new Dictionary<string, double>() }, Lessons = "" };
            v = OverlayText.Build(s);
            Assert.Equal("Day summary: no data yet", v.Summary);
            Assert.Equal("", v.Lessons);

            s.SummaryFailed = true;
            Assert.Equal("bridge down", OverlayText.Build(s).Summary);
        }

        [Fact]
        public void TodayListAndOutcomeLines()
        {
            var day = new DayLog();
            day.SetDay(5);
            day.Arrive("Scav");
            day.Note(OverlayText.Describe(JObject.Parse("{\"kind\":\"bargain\",\"option\":\"add_markup_25\",\"accepted\":false,\"walked_out\":true,\"rep_delta\":{\"lower_levels\":-3.0},\"actor\":\"mod\"}")));
            day.Note(OverlayText.Describe(Outcomes.Left("walked_out")));
            day.Arrive("Farmer");
            day.Note(OverlayText.Describe(JObject.Parse("{\"kind\":\"trade\",\"side\":\"sold\",\"items\":[1,2],\"cash_delta\":680,\"rep_delta\":{},\"actor\":\"human\"}")));
            var s = In(AdviceStatus.None, null);
            s.Today = day.Entries;
            var v = OverlayText.Build(s);
            Assert.Equal("Scav → add_markup_25 refused, walked out (lower_levels -3 rep) (fence); walked out\nFarmer → sold 2 items for 680", v.Today);
            Assert.Equal("Result: sold 2 items for 680", v.Outcome);
            day.SetDay(5);
            Assert.Equal(2, day.Entries.Count);
            day.SetDay(6);
            Assert.Empty(day.Entries);
        }

        [Theory]
        [InlineData("{\"kind\":\"trade\",\"side\":\"bought\",\"items\":[9],\"cash_delta\":-120}", "bought 1 item for 120")]
        [InlineData("{\"kind\":\"bargain\",\"option\":\"offer_discount_10\",\"accepted\":true,\"walked_out\":false,\"rep_delta\":{}}", "offer_discount_10 accepted")]
        [InlineData("{\"kind\":\"bargain\",\"option\":\"blackmail\",\"walked_out\":false,\"rep_delta\":{}}", "blackmail")]
        [InlineData("{\"kind\":\"left\",\"cause\":\"dismissed\",\"actor\":\"mod\"}", "dismissed (fence)")]
        [InlineData("{\"kind\":\"left\",\"cause\":\"business_done\"}", "left, done")]
        [InlineData("{\"kind\":\"left\",\"cause\":\"shutter\"}", "left at closing")]
        [InlineData("{\"kind\":\"other\"}", null)]
        public void DescribesOutcomes(string json, string text) => Assert.Equal(text, OverlayText.Describe(JObject.Parse(json)));

        [Fact]
        public void BridgeStatusFromTheSession()
        {
            var s = new BridgeSession();
            Assert.Equal(BridgeStatus.Connecting, OverlayText.BridgeOf(s, 0));
            s.RequestHello();
            s.TryBeginHello(0);
            s.HelloSucceeded(new HelloResponse { SessionId = "s1", Protocol = ProtocolVersion.Current, Mode = Modes.Act });
            Assert.Equal(BridgeStatus.Connected, OverlayText.BridgeOf(s, 0));
            s.SessionForgotten();
            Assert.Equal(SessionState.NeedHello, s.State);
            s.GoVanilla("x");
            Assert.Equal(BridgeStatus.Vanilla, OverlayText.BridgeOf(s, 0));
            s.Enabled = false;
            Assert.Equal(BridgeStatus.Off, OverlayText.BridgeOf(s, 0));
            var down = new BridgeSession();
            down.RequestHello();
            for (int i = 0; i < 3; i++)
            {
                down.TryBeginHello(i);
                down.HelloFailed(i);
            }
            Assert.Equal(BridgeStatus.Down, OverlayText.BridgeOf(down, 10));
        }

        [Fact]
        public void SummaryPollerAsksOnDemandAtMostEveryFiveSeconds()
        {
            var p = new SummaryPoller();
            Assert.True(p.TryBegin(0));
            p.Want();
            Assert.False(p.TryBegin(10));
            p.Done(true);
            Assert.False(p.TryBegin(SummaryPoller.MinIntervalMs - 1));
            Assert.True(p.TryBegin(SummaryPoller.MinIntervalMs));
            p.Done(false);
            Assert.True(p.LastFailed);
            Assert.False(p.TryBegin(SummaryPoller.MinIntervalMs * 3));
            p.Want();
            Assert.True(p.TryBegin(SummaryPoller.MinIntervalMs * 3));
        }
    }
}
