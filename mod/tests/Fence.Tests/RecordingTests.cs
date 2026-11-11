using System;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using Fence.Barter;
using Fence.Bridge;
using Fence.Protocol;
using Fence.Recording;
using Mono.Cecil;
using Mono.Cecil.Cil;
using Newtonsoft.Json.Linq;
using Xunit;

namespace Fence.Tests
{
    public class RecordingQueueTests
    {
        static BridgeSession Active(BridgeOptions o = null)
        {
            var s = new BridgeSession(o);
            s.RequestHello();
            s.TryBeginHello(0);
            s.HelloSucceeded(new HelloResponse { SessionId = "s1", Protocol = ProtocolVersion.Current, Mode = Modes.Observe });
            return s;
        }

        static List<string> Drain(BridgeSession s)
        {
            var kinds = new List<string>();
            while (s.TryDequeueEvent(0, out var e))
            {
                kinds.Add(e.Kind);
                s.EventSucceeded();
            }
            return kinds;
        }

        [Theory]
        [InlineData("day_snapshot", EventPriority.High)]
        [InlineData("human_action", EventPriority.High)]
        [InlineData("customer_observed", EventPriority.High)]
        [InlineData("money", EventPriority.High)]
        [InlineData("run_start", EventPriority.High)]
        [InlineData("run_end", EventPriority.High)]
        [InlineData("inspection", EventPriority.High)]
        [InlineData("shutter_opened", EventPriority.Normal)]
        [InlineData("client_arrived", EventPriority.Normal)]
        [InlineData("paper_opened", EventPriority.Low)]
        [InlineData("afterhour_event_started", EventPriority.Low)]
        public void Priorities(string kind, EventPriority want) => Assert.Equal(want, RecordKinds.Priority(kind));

        // Under pressure: low first, then normal; recording kinds never while anything else is left.
        [Fact]
        public void FullQueueDropsLowValueKindsFirst()
        {
            var s = Active(new BridgeOptions { MaxQueuedEvents = 4, MaxQueuedEventsHard = 6 });
            s.Enqueue("paper_opened", 1, null, 0);
            s.Enqueue("shutter_opened", 1, null, 0);
            s.Enqueue("day_snapshot", 1, null, 0);
            s.Enqueue("human_action", 1, null, 0);
            s.Enqueue("human_action", 1, null, 0); // evicts paper_opened
            s.Enqueue("money", 1, null, 0);        // evicts shutter_opened
            Assert.False(s.Enqueue("customer_generated", 1, null, 0)); // a low event never displaces a high one
            Assert.Equal(new[] { "day_snapshot", "human_action", "human_action", "money" }, Drain(s));
            Assert.Equal(3, s.DroppedEvents);
        }

        [Fact]
        public void HighEventsGrowToTheHardCapThenDropOldest()
        {
            var s = Active(new BridgeOptions { MaxQueuedEvents = 2, MaxQueuedEventsHard = 3 });
            for (int i = 0; i < 4; i++) s.Enqueue("human_action", i, null, 0);
            Assert.Equal(3, s.QueuedEvents);
            Assert.Equal(1, s.DroppedEvents);
            Assert.True(s.TryDequeueEvent(0, out var e));
            Assert.Equal(1, e.GameDay);
        }

        [Fact]
        public void TransportFailuresKeepTheEventWithItsSeq()
        {
            var s = Active(new BridgeOptions { BreakerThreshold = 100 });
            s.Enqueue("day_snapshot", 1, new JObject(), new JObject { ["days_to_inspection"] = 3 }, 0);
            for (int i = 0; i < 10; i++)
            {
                Assert.True(s.TryDequeueEvent(i, out var e));
                Assert.Equal(1, e.Seq);
                Assert.Equal(3, (int)e.Hidden["days_to_inspection"]);
                s.EventFailed(i, transport: true);
            }
            Assert.Equal(0, s.DroppedEvents);
            Assert.Equal(1, s.QueuedEvents);
        }

        [Fact]
        public void HttpErrorsAreRetriedThenDropped()
        {
            var s = Active(new BridgeOptions { BreakerThreshold = 100, EventHttpAttempts = 3 });
            s.Enqueue("human_action", 1, null, 0);
            for (int i = 0; i < 3; i++)
            {
                Assert.True(s.TryDequeueEvent(0, out _));
                s.EventFailed(0, transport: false);
            }
            Assert.Equal(0, s.QueuedEvents);
            Assert.Equal(1, s.DroppedEvents);
        }

        [Fact]
        public void EventsBeforeTheBridgeAnswersAreKept()
        {
            var s = new BridgeSession(new BridgeOptions { BreakerThreshold = 1, BreakerCooldownMs = 1000 });
            s.RequestHello();
            Assert.True(s.TryBeginHello(0));
            s.HelloFailed(0);
            Assert.True(s.Enqueue("day_snapshot", 1, null, 5));
            Assert.True(s.Enqueue("human_action", 1, null, 6));
            Assert.Equal(0, s.DroppedEvents);
            Assert.True(s.TryBeginHello(1000));
            s.HelloSucceeded(new HelloResponse { SessionId = "s2", Protocol = 1, Mode = "observe" });
            Assert.Null(s.UnreachableSinceMs);
            Assert.True(s.TryDequeueEvent(1000, out var e));
            Assert.Equal(1, e.Seq);
            Assert.Equal("day_snapshot", e.Kind);
        }

        public static IEnumerable<object[]> StatusCases()
        {
            // name, setup, now, recording?, text contains
            yield return new object[] { "fresh", (Action<BridgeSession>)(_ => { }), 0L, true, "recording" };
            yield return new object[] { "active", (Action<BridgeSession>)(s => Hello(s)), 0L, true, "recording" };
            yield return new object[] { "briefly down", (Action<BridgeSession>)(s => { s.TryBeginHello(0); s.HelloFailed(1000); }), 10999L, true, "recording" };
            yield return new object[] { "down > 10 s", (Action<BridgeSession>)(s => { s.TryBeginHello(0); s.HelloFailed(1000); }), 11001L, false, "unreachable" };
            yield return new object[] { "back", (Action<BridgeSession>)(s => { s.TryBeginHello(0); s.HelloFailed(1000); s.TryBeginHello(40000); Hello(s); }), 50000L, true, "recording" };
            yield return new object[] { "vanilla", (Action<BridgeSession>)(s => { s.TryBeginHello(0); s.HelloRejected("409"); }), 0L, false, "refused" };
            yield return new object[] { "off", (Action<BridgeSession>)(s => s.Enabled = false), 0L, false, "off in settings" };
            yield return new object[] { "dropped", (Action<BridgeSession>)(s => { Hello(s); s.Enqueue("a", 1, null, 0); s.TryDequeueEvent(0, out _); s.SessionUnknown(); s.TryBeginHello(0); Hello(s); s.TryDequeueEvent(0, out _); s.SessionUnknown(); }), 0L, false, "lost" };
        }

        static void Hello(BridgeSession s)
        {
            s.TryBeginHello(0);
            s.HelloSucceeded(new HelloResponse { SessionId = "s", Protocol = 1, Mode = "observe" });
        }

        [Theory]
        [MemberData(nameof(StatusCases))]
        public void RecordingStatusLine(string name, Action<BridgeSession> setup, long now, bool recording, string text)
        {
            var s = new BridgeSession(new BridgeOptions { BreakerThreshold = 1, BreakerCooldownMs = 30000 });
            s.RequestHello();
            setup(s);
            var v = RecordingStatus.Of(s, now);
            Assert.True(recording == v.Recording, name + ": " + v.Text);
            Assert.Contains(text, v.Text);
            if (!recording) Assert.StartsWith("NOT RECORDING", v.Text);
        }
    }

    sealed class FakeDay : IDayGame
    {
        public const int Sentinel = 987654;
        public const string SentinelText = "SENTINEL_HIDDEN";
        public readonly HashSet<string> Throwing = new HashSet<string>();
        public bool SecurityVisible;

        T Maybe<T>(string section, Func<T> f)
        {
            if (Throwing.Contains(section)) throw new MissingFieldException(section);
            return f();
        }

        static ItemInfo Item(int uid, string id) => new ItemInfo
        {
            Uid = uid, Id = id, Name = id, Owned = true, EstimatedValue = 300, BoughtAt = 100, StolenHeat = 40, AppraisalValue = Sentinel,
            Features = { new FeatureInfo { Visible = true, Shown = "Worn (-20%)", Real = SentinelText, CurrentMod = -20, ActualMod = -60 } },
        };

        public StoreDay Store() => Maybe("store", () => new StoreDay { Cash = 500, Day = 70, StampIntroduced = false, CommissaryTokens = 4 });
        public RentDay Rent() => Maybe("rent", () => new RentDay { Rent = 600, Arrears = 50, DaysLeft = 0 });
        public PropertyDay Property() => Maybe("property", () => new PropertyDay { BuyoutPrice = 12000, MortgageOffered = true, OfferDownPayment = 2500, OfferWeekly = 300, OfferWeeks = 40 });
        public IList<RepDay> Reputation() => Maybe("reputation", () => (IList<RepDay>)new List<RepDay>
        {
            new RepDay { FactionId = "FACTION_LOWER_LEVEL", Value = 25, Status = "Known", Perks = { new PerkDay { Id = "LLREFERAL", Threshold = 20, Unlocked = true }, new PerkDay { Id = "LLRAREGOOD", Threshold = 40 }, new PerkDay { Id = "LLPOORREVIEWS", Threshold = -20 } } },
            new RepDay { FactionId = "FACTION_TOURIST", Value = 5 },
        });
        public IList<OwnedItem> Inventory() => Maybe("inventory", () => (IList<OwnedItem>)new List<OwnedItem>
        {
            new OwnedItem { Item = Item(1, "smuggler_bay"), Where = "stock", Root = "stock", SmugglerBay = true, Capacity = new Capacity { Cells = 12, Free = 9 } },
            new OwnedItem { Item = Item(2, "pills"), Where = "inside:1", Root = "stock" },
            new OwnedItem { Item = Item(3, "sign_food"), Where = "showcase", Root = "showcase" },
        });
        public ClienteleDay Clientele() => Maybe("clientele", () => new ClienteleDay
        {
            NormalCustomers = 2, DisplayBuyers = new List<string> { "LOWTECH" }, PredictionsSource = "computed",
            Predictions = new List<Prediction> { new Prediction { ClientId = "sentinelClientId", ClientName = "Food Seller", Chance = 75, ItemId = "sign_food" } },
        });
        public SecurityDay Security() => Maybe("security", () => new SecurityDay
        {
            Visible = SecurityVisible, Evidence = 40, TotalCrime = 200, EvidenceDisplay = "40/100",
            Crimes = { new CrimeDay { Id = "FENCING", Amount = 200, Total = Sentinel } },
        });
        public IList<UpgradeDay> Upgrades() => Maybe("upgrades", () => (IList<UpgradeDay>)new List<UpgradeDay>
        {
            new UpgradeDay { Id = "A", Name = "Owned", State = 2, Unlocked = true },
            new UpgradeDay { Id = "B", Name = "Buyable", Cost = 300, State = 1, HasPrerequisite = true },
            new UpgradeDay { Id = "C", Name = "Locked", Cost = 900, State = 1 },
            new UpgradeDay { Id = "E", Name = "Cooling", Cost = 400, State = 1, HasPrerequisite = true, Cooldown = 5 },
            new UpgradeDay { Id = "F", Name = "Not offered", Cost = 100, HasPrerequisite = true },
            new UpgradeDay { Id = "D", Name = SentinelText, Cost = 1, HasPrerequisite = true, Hidden = true },
        });
        public IList<ServiceDay> Services() => Maybe("services", () => (IList<ServiceDay>)new List<ServiceDay> { new ServiceDay { Id = "POWER_SERVICE", Cost = 60, Unlocked = true, Using = true } });
        public IList<EventDay> ActiveEvents() => Maybe("market_events", () => (IList<EventDay>)new List<EventDay> { new EventDay { Id = "e1", Name = "Shortage", Type = "normale", Duration = 4, Modifiers = { new MarketEvent { ItemType = "LOWTECH", ValuePct = 10 } } } });
        public IList<EventDay> EventsOnDay(int day) => day == 71 ? new List<EventDay> { new EventDay { Id = "rent_day", Name = "Rent", Type = "innate" } } : new List<EventDay>();
        public IList<EventDay> UnknownFutureEvents() => new List<EventDay> { new EventDay { Id = SentinelText, Name = "Raid", InDays = 3 } };
        public IList<AfterhourDay> Afterhours() => Maybe("afterhours", () => (IList<AfterhourDay>)new List<AfterhourDay> { new AfterhourDay { Id = "dumping_ground", Available = true } });
        public IList<RequestDay> Requests() => Maybe("requests", () => (IList<RequestDay>)new List<RequestDay>());
        public IList<string> NightLog() => new List<string> { "<color=red>Burglary</color> took 2 items" };
        public IList<string> StartingPerks() => Maybe("perks", () => (IList<string>)new List<string> { "aug_menace" });
        public HiddenDay Hidden() => Maybe("hidden", () => new HiddenDay { DaysToInspection = Sentinel, AugPresence = Sentinel, Luck = Sentinel, InspectionCountdownActive = true });
    }

    public class DaySnapshotTests
    {
        static SnapshotResult Build(FakeDay g, string phase = SnapshotPhases.Morning, Func<string, bool> on = null) =>
            DaySnapshot.Build(g, phase, "run-1", on ?? (_ => true));

        // The information boundary: every hidden sentinel lands in `hidden`, none in `data`.
        [Theory]
        [InlineData(false)]
        [InlineData(true)]
        public void HiddenValuesNeverReachData(bool securityVisible)
        {
            var r = Build(new FakeDay { SecurityVisible = securityVisible });
            string data = r.Data.ToString(Newtonsoft.Json.Formatting.None), hidden = r.Hidden.ToString(Newtonsoft.Json.Formatting.None);
            Assert.DoesNotContain(FakeDay.Sentinel.ToString(), data);
            Assert.DoesNotContain(FakeDay.SentinelText, data);
            Assert.DoesNotContain("sentinelClientId", data);
            Assert.Contains(FakeDay.Sentinel.ToString(), hidden);
            Assert.Contains(FakeDay.SentinelText, hidden);
            Assert.Contains("sentinelClientId", hidden);
            Assert.Null(r.Data["hidden"]);
            Assert.Equal(FakeDay.Sentinel, (int)r.Hidden["days_to_inspection"]);
        }

        [Fact]
        public void SecurityTheRecordDoesNotShowIsHidden()
        {
            var hiddenRecord = Build(new FakeDay { SecurityVisible = false });
            Assert.Equal(new JObject { ["visible"] = false }, hiddenRecord.Data["security"]);
            Assert.Equal(200, (int)hiddenRecord.Hidden["security"]["crimes"][0]["amount"]);

            var shown = Build(new FakeDay { SecurityVisible = true });
            Assert.Equal("40/100", (string)shown.Data["security"]["evidence_level"]);
            Assert.Null(shown.Hidden["security"]);
        }

        [Fact]
        public void ShapeOfTheMorning()
        {
            var d = Build(new FakeDay()).Data;
            Assert.Equal("morning", (string)d["phase"]);
            Assert.Equal("run-1", (string)d["run_id"]);
            Assert.Equal(0, (int)d["store"]["weekday"]);
            Assert.Null(d["store"]["commissary_tokens"]); // stamps not introduced
            Assert.Equal(650, (int)d["rent"]["due"]);
            Assert.Equal(0, (int)d["rent"]["days_left_morning"]);
            Assert.Equal(2500, (int)d["property"]["mortgage_offer"]["down_payment"]);
            Assert.Null(d["reputation"]["tourist"]);
            Assert.Equal("LLRAREGOOD", (string)d["reputation"]["lower_levels"]["next_perk"]["id"]);
            Assert.Equal("LLPOORREVIEWS", (string)d["reputation"]["lower_levels"]["next_negative_perk"]["id"]);
            Assert.Equal(new JArray("LLREFERAL"), d["reputation"]["lower_levels"]["unlocked_perks"]);
            Assert.Equal(new[] { "stock", "inside:1", "showcase" }, d["inventory"].Select(i => (string)i["where"]));
            Assert.True((bool)d["inventory"][0]["smuggler_bay"]);
            Assert.Equal("stock", (string)d["inventory"][1]["root"]);
            Assert.Equal(40, (int)d["inventory"][1]["stolen_heat"]);
            Assert.Null(d["inventory"][0]["price"]);
            Assert.Equal("sign_food", (string)d["clientele"]["display_predictions"][0]["item"]);
            Assert.Equal("Food Seller", (string)d["clientele"]["display_predictions"][0]["client"]);
            Assert.Equal("lowtech", (string)d["clientele"]["display_buyers"][0]);
            Assert.Equal(new[] { "A" }, d["upgrades"]["owned"].Select(u => (string)u["id"]));
            Assert.Equal(new[] { "B" }, d["upgrades"]["available"].Select(u => (string)u["id"]));
            Assert.Equal(5, (int)d["upgrades"]["cooling"].Single()["days"]);
            Assert.Equal(71, (int)d["market_events"]["calendar"][0]["day"]);
            Assert.Equal("Burglary took 2 items", (string)d["night_log"][0]);
            Assert.Null(d["unavailable"]);
        }

        [Fact]
        public void NightLogOnlyInTheMorning()
        {
            Assert.NotNull(Build(new FakeDay(), SnapshotPhases.Morning).Data["night_log"]);
            foreach (var phase in new[] { SnapshotPhases.Open, SnapshotPhases.Close, SnapshotPhases.Sleep, SnapshotPhases.Load, SnapshotPhases.RunStart })
                Assert.Null(Build(new FakeDay(), phase).Data["night_log"]);
        }

        [Fact]
        public void AFailingOrDisabledSectionCostsOnlyItself()
        {
            var g = new FakeDay();
            g.Throwing.Add("security");
            var r = Build(g, on: s => s != "upgrades");
            Assert.Equal(new JArray("security", "upgrades"), r.Data["unavailable"]);
            Assert.Null(r.Data["security"]);
            Assert.Null(r.Data["upgrades"]);
            Assert.NotNull(r.Data["inventory"]);
            Assert.Single(r.Failed);
            Assert.Equal("security", r.Failed[0].section);
        }

        [Theory]
        [InlineData(25.0, "LLRAREGOOD", "LLPOORREVIEWS")]
        [InlineData(19.99, "LLREFERAL", "LLPOORREVIEWS")]
        [InlineData(-30.0, "LLREFERAL", "LLDISTRUSTED")]
        [InlineData(-100.0, "LLREFERAL", null)]
        [InlineData(100.0, null, "LLPOORREVIEWS")]
        public void NextPerks(double value, string next, string nextNegative)
        {
            var perks = new List<PerkDay>
            {
                new PerkDay { Id = "LLREFERAL", Threshold = 20 }, new PerkDay { Id = "LLRAREGOOD", Threshold = 40 },
                new PerkDay { Id = "LLPOORREVIEWS", Threshold = -20 }, new PerkDay { Id = "LLDISTRUSTED", Threshold = -40 },
                new PerkDay { Id = "LLLYNCH", Threshold = -100 },
            };
            var (up, down) = DaySnapshot.NextPerks(value, perks);
            Assert.Equal(next, up?.Id);
            Assert.Equal(nextNegative, down?.Id);
        }

        [Theory]
        [InlineData(1, 1)]
        [InlineData(7, 1)]
        [InlineData(8, 8)]
        [InlineData(69, 64)]
        [InlineData(70, 64)]
        public void CalendarStartsAtTheWeek(int day, int start) => Assert.Equal(start, DaySnapshot.CalendarStart(day));

        [Fact]
        public void FixtureKeepsHiddenOutOfData()
        {
            var ev = ProtocolJson.Deserialize<EventRequest>(File.ReadAllText(Path.Combine(Paths.Fixtures, "event.day_snapshot.json")));
            foreach (var key in new[] { "days_to_inspection", "aug_presence", "luck", "true_value", "real_features", "unknown_events", "crime_totals", "display_prediction_clients" })
            {
                Assert.DoesNotContain(ev.Data.Descendants().OfType<JProperty>(), p => p.Name == key);
                Assert.Contains(ev.Hidden.Descendants().OfType<JProperty>(), p => p.Name == key);
            }
        }
    }

    public class CustomerRecordingTests
    {
        static ClientInfo Client(ClientIntent i, string raw = null) => new ClientInfo { Intent = i, RawIntent = raw, Name = "N", Identifier = "secret_id", Dismissable = true, IsAug = true };

        [Theory]
        [InlineData(ClientIntent.Buy, null, "trade", true)]
        [InlineData(ClientIntent.SellAndBuy, "SELLNBUY", "trade", true)]
        [InlineData(ClientIntent.Other, "BARTER", "barter", true)]
        [InlineData(ClientIntent.Other, "APPRAISAL_SERVICE", "appraisal", true)]
        [InlineData(ClientIntent.Other, "INFORMATION_DEALER", "info_dealer", true)]
        [InlineData(ClientIntent.Other, "GUNSMITH", "gunsmith", true)]
        [InlineData(ClientIntent.Other, "INSPECTION", "inspection", false)]
        [InlineData(ClientIntent.Other, "RENT", "landlord", false)]
        [InlineData(ClientIntent.Other, "LOAN_SHARK", "loan_shark", false)]
        [InlineData(ClientIntent.Dialogue, "DIALOGUE", "dialogue", false)]
        [InlineData(ClientIntent.Other, "EXPEDITION", "expedition", false)]
        public void Kinds(ClientIntent intent, string raw, string kind, bool counter)
        {
            Assert.Equal(kind, CustomerKinds.Of(Client(intent, raw)));
            Assert.Equal(counter, CustomerKinds.UsesCounter(kind));
        }

        // (kind, steps) → which step emits. Every customer exactly once.
        [Theory]
        [InlineData("trade", "arrive,poll,ready,ready,leave", "ready")]
        [InlineData("trade", "arrive,leave", "leave")]
        [InlineData("inspection", "arrive,ready,leave", "arrive")]
        [InlineData("dialogue", "arrive,leave", "arrive")]
        public void ObservedOncePerCustomer(string kind, string steps, string emitsAt)
        {
            var o = new CustomerObserver();
            var emitted = new List<string>();
            foreach (var step in steps.Split(','))
            {
                bool due = step switch
                {
                    "arrive" => o.Arrived(5, kind),
                    "ready" => o.Ready(5),
                    "leave" => o.Leaving(5),
                    _ => false,
                };
                if (due) emitted.Add(step);
            }
            Assert.Equal(new[] { emitsAt }, emitted);
            Assert.False(o.Pending);
        }

        [Fact]
        public void AStaleCustomerSeqNeverEmits()
        {
            var o = new CustomerObserver();
            o.Arrived(5, "trade");
            Assert.False(o.Ready(4));
            o.Arrived(6, "trade");
            Assert.False(o.Leaving(5));
            Assert.True(o.Ready(6));
        }

        [Fact]
        public void UncoveredCustomersHaveNoOptionsAndKeepTheirIdentityHidden()
        {
            var c = Client(ClientIntent.Other, "INSPECTION");
            var (data, hidden) = CustomerObservation.Build(9, c, "inspection", "arrival", "intent INSPECTION", null,
                new List<DecisionOption> { new DecisionOption { Id = "dismiss" } }, new JObject { ["lazy_inspector"] = false }, null);
            Assert.False((bool)data["covered"]);
            Assert.Equal("intent INSPECTION", (string)data["reason"]);
            Assert.Empty((JArray)data["options"]);
            Assert.Equal("N", (string)data["context"]["client"]["name"]);
            Assert.False((bool)data["extras"]["lazy_inspector"]);
            Assert.DoesNotContain("secret_id", data.ToString());
            Assert.Equal("secret_id", (string)hidden["client"]["identifier"]);
            Assert.True((bool)hidden["client"]["is_aug"]);
        }
    }

    public class HumanActionTests
    {
        // (loading, game scope, executor) → a counter move counts / a button press counts.
        [Theory]
        [InlineData(false, false, false, true, true)]
        [InlineData(true, false, false, false, false)]
        [InlineData(false, true, false, false, true)]
        [InlineData(false, false, true, false, false)]
        [InlineData(false, true, true, false, false)]
        public void WhoMoved(bool loading, bool game, bool executor, bool human, bool click)
        {
            var f = new HumanFilter();
            f.LoadFinished();
            if (loading) f.LoadStarted();
            if (game) f.EnterGame();
            if (executor) f.EnterExecutor();
            Assert.Equal(human, f.IsHuman);
            Assert.Equal(click, f.IsHumanClick);
        }

        [Fact]
        public void ScopesNestAndNeverGoNegative()
        {
            var f = new HumanFilter();
            Assert.True(f.Loading);
            f.LoadFinished();
            f.ExitGame();
            f.ExitExecutor();
            Assert.True(f.IsHuman);
            f.EnterGame();
            f.EnterGame();
            f.ExitGame();
            Assert.False(f.IsHuman);
            f.ExitGame();
            Assert.True(f.IsHuman);
            f.EnterGame();
            f.LoadStarted();
            f.LoadFinished();
            Assert.True(f.IsHuman);
        }

        [Fact]
        public void CarriesTheAdviceInForce()
        {
            var a = HumanActions.Build("sell_all", new AdviceInForce { ClientSeq = 41, DecisionId = "d-1", Advised = "add_markup_10" }, 2231, new JObject { ["action"] = "ignored", ["owned"] = true });
            Assert.Equal(41, (int)a["client_seq"]);
            Assert.Equal("sell_all", (string)a["action"]);
            Assert.Equal(2231, (int)a["item_uid"]);
            Assert.Equal("d-1", (string)a["decision_id"]);
            Assert.Equal("add_markup_10", (string)a["advised"]);
            Assert.True((bool)a["owned"]);
        }

        [Fact]
        public void NightActionsHaveNoCustomerOrAdvice()
        {
            var a = HumanActions.Build("sleep", new AdviceInForce { ClientSeq = 0, DecisionId = "d-9", Advised = "dismiss" });
            Assert.Equal(new JObject { ["action"] = "sleep" }, a);
        }

        [Fact]
        public void TrackerReportsTheAnswerToTheNewestDecision()
        {
            var t = new AdviceTracker();
            t.NewClient(new object());
            t.Sent("sig", "d-1");
            Assert.Null(t.AnswerTo("d-1"));
            t.Answered(new Advice { DecisionId = "d-1", Signature = "sig", Deferred = true });
            Assert.Equal("defer", t.AnswerTo("d-1"));
            t.Sent("sig2", "d-2");
            t.Answered(new Advice { DecisionId = "d-2", Signature = "sig2", OptionId = "sell_all" });
            Assert.Equal("sell_all", t.AnswerTo(t.LastDecisionId));
            t.NewClient(new object());
            Assert.Null(t.AnswerTo("d-2"));
        }

        [Fact]
        public void DialogueChoiceRecordsWhatThePlayerRead()
        {
            var o = HumanActions.Choice(1, new List<ChoiceInfo>
            {
                new ChoiceInfo { Label = "pay rent choice", Text = "<b>Pay</b> 600", Enabled = true },
                new ChoiceInfo { Label = "", Text = "Don't pay", Enabled = true },
            }, new List<string> { "<color=#eb4034>Landlord:</color> Rent's due." });
            Assert.Equal(1, (int)o["index"]);
            Assert.Equal("Don't pay", (string)o["text"]);
            Assert.Null(o["label"]);
            Assert.Equal("Pay 600", (string)o["choices"][0]["text"]);
            Assert.Equal("Landlord: Rent's due.", (string)o["history"][0]);
        }

        [Fact]
        public void ServicesSetDiffsAgainstTheNightBefore()
        {
            var before = new List<ServiceDay> { new ServiceDay { Id = "POWER", Using = true, Unlocked = true }, new ServiceDay { Id = "JANITOR", Using = true, Unlocked = true } };
            var now = new List<ServiceDay>
            {
                new ServiceDay { Id = "POWER", Using = true, Unlocked = true, Cost = 60 },
                new ServiceDay { Id = "JANITOR", Using = false, Unlocked = true, Cost = 30 },
                new ServiceDay { Id = "INFORMANT", Using = true, Unlocked = true, Cost = 40 },
                new ServiceDay { Id = "LOCKED", Using = false, Unlocked = false, Cost = 99 },
            };
            var d = ServiceSelection.Diff(before, now);
            Assert.Equal(new JArray("POWER", "INFORMANT"), d["selected"]);
            Assert.Equal(new JArray("INFORMANT"), d["turned_on"]);
            Assert.Equal(new JArray("JANITOR"), d["turned_off"]);
            Assert.Equal(100, (int)d["total_cost"]);
            Assert.Null(d["costs"]["LOCKED"]);
            Assert.Null(ServiceSelection.Diff(null, now)["turned_on"]);
        }

        [Theory]
        [InlineData(300, 1000, 700)]
        [InlineData(0, 1000, 1000)]
        [InlineData(1200, 1000, 0)]
        public void UpgradePairsCashWithItsId(int cash, int cost, int favor)
        {
            var u = new UpgradePurchase();
            u.CashSpent(cash);
            var o = u.Bought("SEC_GRACE", "Grace", cost);
            Assert.Equal("SEC_GRACE", (string)o["upgrade_id"]);
            Assert.Equal(cash, (int)o["cash_spent"]);
            Assert.Equal(favor, (int)o["favor_spent"]);
            Assert.Null(u.Bought("X", null, 5)["cash_spent"]);
        }
    }

    public class PlanOverlayTests
    {
        const string Plan = "GOALS: reach 40 BM rep\nCASH\nKeep 600 for rent.\n## WINDOW\n- cassette + sign_food in the case\n- nothing stolen\n**STOCK:** hold the pills\nRISK: inspection soon";

        [Theory]
        [InlineData("WINDOW", "- cassette + sign_food in the case\n- nothing stolen")]
        [InlineData("GOALS", "reach 40 BM rep")]
        [InlineData("CASH", "Keep 600 for rent.")]
        [InlineData("STOCK", "hold the pills")]
        [InlineData("FACTIONS", "")]
        public void Sections(string heading, string body) => Assert.Equal(body, PlanText.Section(Plan, heading));

        static OverlayView View(string plan, bool beforeOpen) => OverlayText.Build(new OverlayInput
        {
            Mode = Modes.Advise,
            Summary = new SummaryResponse { Day = 69, Plan = plan, PlanDay = 69 },
            BeforeOpen = beforeOpen,
            Recording = new RecordingView { Recording = true, Text = "recording" },
        });

        [Fact]
        public void WindowIsProminentOnlyBeforeOpening()
        {
            Assert.StartsWith("WINDOW", View(Plan, true).PlanWindow);
            Assert.Contains("nothing stolen", View(Plan, true).PlanWindow);
            Assert.Equal("", View(Plan, false).PlanWindow);
            Assert.Contains("RISK", View(Plan, false).Plan);
            Assert.Equal("Today's plan (day 69)", View(Plan, false).PlanHeader);
        }

        [Fact]
        public void NoPlanHidesTheSection()
        {
            var v = View(null, true);
            Assert.Equal("", v.Plan);
            Assert.Equal("", v.PlanHeader);
            Assert.Equal("", v.PlanWindow);
        }

        [Fact]
        public void NotRecordingShowsEvenCollapsed()
        {
            var v = OverlayText.Build(new OverlayInput { Recording = new RecordingView { Recording = false, Text = "NOT RECORDING — bridge unreachable 12 s, 3 buffered" } });
            Assert.True(v.NotRecording);
            Assert.StartsWith("NOT RECORDING", v.Bar);
        }
    }

    // Moves BarterExecutor makes must not be recorded as the player's: the executor call is bracketed by
    // HumanFilter.EnterExecutor / ExitExecutor (the latter in a finally).
    public class RecordingSafetyTests
    {
        [Fact]
        public void ExecutorRunsInsideTheRecorderExecutorScope()
        {
            var m = ModuleDefinition.ReadModule(Path.Combine(Paths.ModBin, "Fence.Mod.dll"));
            var callers = m.GetTypes().SelectMany(t => t.Methods).Where(x => x.HasBody)
                .Where(x => x.Body.Instructions.Any(i => i.Operand is MethodReference r && r.DeclaringType.FullName == "Fence.Mod.BarterExecutor" && r.Name == "Execute")).ToList();
            var method = Assert.Single(callers);
            var ins = method.Body.Instructions.ToList();
            int Index(string type, string name) => ins.FindIndex(i => i.Operand is MethodReference r && r.DeclaringType.Name == type && r.Name == name);
            int enter = Index("HumanFilter", "EnterExecutor"), exec = Index("BarterExecutor", "Execute"), exit = Index("HumanFilter", "ExitExecutor");
            Assert.True(enter >= 0 && enter < exec && exec < exit, "EnterExecutor < Execute < ExitExecutor");
            Assert.Contains(method.Body.ExceptionHandlers, h => h.HandlerType == ExceptionHandlerType.Finally
                && ins.IndexOf(h.HandlerStart) <= exit && exit < (h.HandlerEnd != null ? ins.IndexOf(h.HandlerEnd) : ins.Count));
        }

        // Every recording patch handler goes through Rec/Guard: none touches the game unguarded.
        [Fact]
        public void RecordingHandlersAreGuarded()
        {
            var m = ModuleDefinition.ReadModule(Path.Combine(Paths.ModBin, "Fence.Mod.dll"));
            var patches = m.GetType("Fence.Mod.FencePatches");
            var handlers = patches.Methods.Where(x => x.IsStatic && (x.Name.EndsWith("Prefix") || x.Name.EndsWith("Postfix") || x.Name.StartsWith("GameScope"))).ToList();
            Assert.True(handlers.Count > 60);
            foreach (var h in handlers)
                Assert.True(h.Body.Instructions.Any(i => i.Operand is MethodReference r && (r.Name == "Rec" || r.Name == "Guard" || r.Name == "Inspect" || r.Name == "Aug" || r.Name == "Money" || r.Name == "After")),
                    h.Name + " calls neither Rec nor Guard");
        }
    }
}
