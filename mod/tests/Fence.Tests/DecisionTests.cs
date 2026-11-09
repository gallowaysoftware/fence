using System.Collections.Generic;
using Fence.Bridge;
using Fence.Decisions;
using Fence.Protocol;
using Newtonsoft.Json.Linq;
using Xunit;

namespace Fence.Tests
{
    public class CircuitBreakerTests
    {
        [Fact]
        public void OpensAfterThresholdAndProbesAfterCooldown()
        {
            var b = new CircuitBreaker(3, 1000);
            Assert.True(b.TryAcquire(0));
            b.RecordFailure(0);
            b.RecordFailure(0);
            Assert.Equal(BreakerState.Closed, b.State);
            b.RecordFailure(10);
            Assert.Equal(BreakerState.Open, b.State);
            Assert.True(b.IsOpen(500));
            Assert.False(b.TryAcquire(500));

            Assert.True(b.TryAcquire(1010));
            Assert.Equal(BreakerState.HalfOpen, b.State);
            Assert.False(b.TryAcquire(1010));
            b.RecordSuccess();
            Assert.Equal(BreakerState.Closed, b.State);
            Assert.Equal(0, b.ConsecutiveFailures);
        }

        [Fact]
        public void FailedProbeReopensForAFullCooldown()
        {
            var b = new CircuitBreaker(1, 1000);
            b.RecordFailure(0);
            Assert.True(b.TryAcquire(1000));
            b.RecordFailure(1000);
            Assert.Equal(BreakerState.Open, b.State);
            Assert.False(b.TryAcquire(1999));
            Assert.True(b.TryAcquire(2000));
        }

        [Fact]
        public void SuccessResetsTheCount()
        {
            var b = new CircuitBreaker(2, 1000);
            b.RecordFailure(0);
            b.RecordSuccess();
            b.RecordFailure(0);
            Assert.Equal(BreakerState.Closed, b.State);
        }
    }

    public class DeadlinePolicyTests
    {
        [Theory]
        [InlineData(0, 20000)]
        [InlineData(-5, 20000)]
        [InlineData(100, 500)]
        [InlineData(15000, 15000)]
        [InlineData(999999, 60000)]
        public void ClampsRequestedDeadline(int requested, int expected) =>
            Assert.Equal(expected, new DeadlinePolicy().Effective(requested));

        [Fact]
        public void ExpiresAfterDeadlinePlusGrace()
        {
            var p = new DeadlinePolicy(graceMs: 250);
            Assert.False(p.IsExpired(1000, 1000 + 2000 + 249, 2000));
            Assert.True(p.IsExpired(1000, 1000 + 2000 + 250, 2000));
        }
    }

    public class ChoiceValidatorTests
    {
        static DecideRequest Request() => new DecideRequest
        {
            DecisionId = "d1",
            Options = new List<DecisionOption>
            {
                new DecisionOption { Id = "sell_all" },
                new DecisionOption
                {
                    Id = "counter",
                    Params = new Dictionary<string, ParamSpec> { ["price"] = new ParamSpec { Type = "int", Min = 1, Max = 400 } },
                },
            },
        };

        static DecideResponse Response(string option, JObject p = null, string id = "d1") =>
            new DecideResponse { DecisionId = id, Choice = new Choice { Option = option, Params = p } };

        public static IEnumerable<object[]> Cases() => new[]
        {
            new object[] { Response("sell_all"), "sell_all" },
            new object[] { Response("counter", new JObject { ["price"] = 140 }), "counter" },
            new object[] { Response("defer"), "defer" },
            new object[] { Response("burn_store"), "defer" },
            new object[] { Response("sell_all", id: "d2"), "defer" },
            new object[] { Response("counter", new JObject { ["price"] = 401 }), "defer" },
            new object[] { Response("counter", new JObject { ["price"] = 0 }), "defer" },
            new object[] { Response("counter", new JObject { ["price"] = "140" }), "defer" },
            new object[] { Response("counter", new JObject { ["price"] = 14.5 }), "defer" },
            new object[] { Response("counter"), "defer" },
            new object[] { Response("counter", new JObject { ["price"] = 5, ["tip"] = 1 }), "defer" },
            new object[] { Response("sell_all", new JObject { ["price"] = 5 }), "defer" },
            new object[] { new DecideResponse { DecisionId = "d1" }, "defer" },
        };

        [Theory]
        [MemberData(nameof(Cases))]
        public void OnlyEnumeratedOptionsWithinBounds(DecideResponse response, string expected)
        {
            var choice = ChoiceValidator.Validate(Request(), response, out _);
            Assert.Equal(expected, choice.Option);
        }

        [Fact]
        public void NullResponseDefers() => Assert.True(ChoiceValidator.Validate(Request(), null, out _).IsDefer);
    }

    public class BridgeSessionTests
    {
        static BridgeSession Active(string mode = Modes.Observe, BridgeOptions o = null)
        {
            var s = new BridgeSession(o);
            s.RequestHello();
            Assert.True(s.TryBeginHello(0));
            s.HelloSucceeded(new HelloResponse { SessionId = "s1", Protocol = ProtocolVersion.Current, Mode = mode });
            Assert.Equal(SessionState.Active, s.State);
            return s;
        }

        [Fact]
        public void EventsQueuedBeforeHelloAreNumberedFromOne()
        {
            var s = new BridgeSession();
            s.Enqueue("new_game_started", 0, null, 0);
            s.Enqueue("game_loaded", 1, null, 0);
            Assert.False(s.TryDequeueEvent(0, out _));
            s.RequestHello();
            Assert.True(s.TryBeginHello(0));
            s.HelloSucceeded(new HelloResponse { SessionId = "s1", Protocol = 1, Mode = "observe" });
            Assert.True(s.TryDequeueEvent(0, out var e1));
            Assert.Equal(1, e1.Seq);
            Assert.Equal("s1", e1.SessionId);
            Assert.False(s.TryDequeueEvent(0, out _));
            s.EventSucceeded();
            Assert.True(s.TryDequeueEvent(0, out var e2));
            Assert.Equal(2, e2.Seq);
        }

        [Fact]
        public void QueueIsBoundedAndDropsLeaveSeqGaps()
        {
            var s = Active(o: new BridgeOptions { MaxQueuedEvents = 2 });
            s.Enqueue("a", 1, null, 0);
            s.Enqueue("b", 1, null, 0);
            s.Enqueue("c", 1, null, 0);
            Assert.Equal(2, s.QueuedEvents);
            Assert.Equal(1, s.DroppedEvents);
            Assert.True(s.TryDequeueEvent(0, out var e));
            Assert.Equal("b", e.Kind);
            Assert.Equal(2, e.Seq);
        }

        [Fact]
        public void OpenBreakerBuffersEventsAndFlushesInOrder()
        {
            var s = Active(o: new BridgeOptions { BreakerThreshold = 1, BreakerCooldownMs = 1000 });
            s.Enqueue("a", 1, null, 0);
            Assert.True(s.TryDequeueEvent(0, out var first));
            s.EventFailed(0);
            Assert.True(s.Enqueue("b", 1, null, 10));
            Assert.False(s.TryDequeueEvent(10, out _));
            Assert.Equal(2, s.QueuedEvents);
            Assert.Equal(0, s.DroppedEvents);

            Assert.True(s.TryDequeueEvent(1000, out var probe));
            Assert.Equal("a", probe.Kind);
            Assert.Equal(first.Seq, probe.Seq);
            s.EventSucceeded();
            Assert.Equal(BreakerState.Closed, s.Breaker.State);
            Assert.True(s.TryDequeueEvent(1000, out var next));
            Assert.Equal("b", next.Kind);
            Assert.Equal(2, next.Seq);
        }

        [Fact]
        public void HelloRetriesOnlyAfterCooldown()
        {
            var s = new BridgeSession(new BridgeOptions { BreakerThreshold = 2, BreakerCooldownMs = 5000 });
            s.RequestHello();
            Assert.True(s.TryBeginHello(0));
            s.HelloFailed(0);
            Assert.True(s.TryBeginHello(10));
            s.HelloFailed(10);
            Assert.False(s.TryBeginHello(4000));
            Assert.True(s.TryBeginHello(5010));
        }

        [Fact]
        public void ConflictStaysVanilla()
        {
            var s = new BridgeSession();
            s.Enqueue("a", 1, null, 0);
            s.RequestHello();
            Assert.True(s.TryBeginHello(0));
            s.HelloRejected("409");
            Assert.Equal(SessionState.Vanilla, s.State);
            Assert.False(s.Enqueue("b", 1, null, 0));
            Assert.False(s.TryBeginHello(999999));
            Assert.Equal(0, s.QueuedEvents);
        }

        [Fact]
        public void WrongProtocolInHelloResponseStaysVanilla()
        {
            var s = new BridgeSession();
            s.RequestHello();
            s.TryBeginHello(0);
            s.HelloSucceeded(new HelloResponse { SessionId = "s", Protocol = 2, Mode = "act" });
            Assert.Equal(SessionState.Vanilla, s.State);
        }

        [Fact]
        public void UnknownSessionTriggersNewHelloAndResetsSeq()
        {
            var s = Active();
            s.Enqueue("a", 1, null, 0);
            s.Enqueue("b", 1, null, 0);
            Assert.True(s.TryDequeueEvent(0, out _));
            s.SessionUnknown();
            Assert.Equal(SessionState.NeedHello, s.State);
            Assert.True(s.TryBeginHello(0));
            s.HelloSucceeded(new HelloResponse { SessionId = "s2", Protocol = 1, Mode = "observe" });
            Assert.True(s.TryDequeueEvent(0, out var retried));
            Assert.Equal("a", retried.Kind);
            Assert.Equal(1, retried.Seq);
            Assert.Equal("s2", retried.SessionId);
            s.EventSucceeded();
            Assert.True(s.TryDequeueEvent(0, out var next));
            Assert.Equal("b", next.Kind);
            Assert.Equal(2, next.Seq);
        }

        [Fact]
        public void EventIsRetriedOnlyOnceAfterUnknownSession()
        {
            var s = Active();
            s.Enqueue("a", 1, null, 0);
            Assert.True(s.TryDequeueEvent(0, out _));
            s.SessionUnknown();
            s.TryBeginHello(0);
            s.HelloSucceeded(new HelloResponse { SessionId = "s2", Protocol = 1, Mode = "observe" });
            Assert.True(s.TryDequeueEvent(0, out _));
            s.SessionUnknown();
            Assert.Equal(0, s.QueuedEvents);
            Assert.Equal(1, s.DroppedEvents);
        }

        [Fact]
        public void UnknownModeFallsBackToObserve() =>
            Assert.Equal(Modes.Observe, ActiveWithMode("autopilot").Mode);

        static BridgeSession ActiveWithMode(string mode)
        {
            var s = new BridgeSession();
            s.RequestHello();
            s.TryBeginHello(0);
            s.HelloSucceeded(new HelloResponse { SessionId = "s", Protocol = 1, Mode = mode });
            return s;
        }

        static DecideRequest Barter(int deadline = 2000) => new DecideRequest
        {
            DecisionId = "d1",
            Kind = "barter",
            DeadlineMs = deadline,
            Options = new List<DecisionOption> { new DecisionOption { Id = "sell_all" }, new DecisionOption { Id = "dismiss" } },
        };

        static DecideResponse Answer(string option) => new DecideResponse { DecisionId = "d1", Choice = new Choice { Option = option }, Source = "llm" };

        [Fact]
        public void ObserveModeNeverAsks()
        {
            var s = Active(Modes.Observe);
            var t = s.BeginDecision(Barter(), 0);
            Assert.False(t.Ask);
            Assert.True(s.CompleteDecision(t, null, DecideFailure.None, 0).Choice.IsDefer);
        }

        [Fact]
        public void NoSessionNeverAsks() => Assert.False(new BridgeSession().BeginDecision(Barter(), 0).Ask);

        [Fact]
        public void ActModeExecutesValidChoice()
        {
            var s = Active(Modes.Act);
            var t = s.BeginDecision(Barter(), 0);
            Assert.True(t.Ask);
            Assert.Equal("s1", t.Request.SessionId);
            var o = s.CompleteDecision(t, Answer("sell_all"), DecideFailure.None, 100);
            Assert.Equal("sell_all", o.Choice.Option);
            Assert.True(o.Execute);
        }

        [Fact]
        public void AdviseModeNeverExecutes()
        {
            var s = Active(Modes.Advise);
            var t = s.BeginDecision(Barter(), 0);
            var o = s.CompleteDecision(t, Answer("sell_all"), DecideFailure.None, 100);
            Assert.Equal("sell_all", o.Choice.Option);
            Assert.False(o.Execute);
        }

        [Fact]
        public void LateAnswerDefers()
        {
            var s = Active(Modes.Act);
            var t = s.BeginDecision(Barter(2000), 0);
            var o = s.CompleteDecision(t, Answer("sell_all"), DecideFailure.None, 2000 + 250);
            Assert.True(o.Choice.IsDefer);
            Assert.False(o.Execute);
        }

        [Fact]
        public void DeadlineIsClampedBeforeSending()
        {
            var s = Active(Modes.Act);
            var t = s.BeginDecision(Barter(10 * 60 * 1000), 0);
            Assert.Equal(60000, t.Request.DeadlineMs);
        }

        [Fact]
        public void TimeoutsOpenTheBreakerAndLaterDecisionsDeferWithoutAsking()
        {
            var s = Active(Modes.Act, new BridgeOptions { BreakerThreshold = 2, BreakerCooldownMs = 30000 });
            for (int i = 0; i < 2; i++)
            {
                var t = s.BeginDecision(Barter(), i);
                Assert.True(t.Ask);
                Assert.True(s.CompleteDecision(t, null, DecideFailure.Timeout, i).Choice.IsDefer);
            }
            var skipped = s.BeginDecision(Barter(), 100);
            Assert.False(skipped.Ask);
            Assert.Equal("circuit open", skipped.SkipReason);
        }

        [Fact]
        public void IllegalAnswerDefers()
        {
            var s = Active(Modes.Act);
            var t = s.BeginDecision(Barter(), 0);
            var o = s.CompleteDecision(t, Answer("shoot"), DecideFailure.None, 10);
            Assert.True(o.Choice.IsDefer);
            Assert.False(o.Execute);
        }
    }
}
