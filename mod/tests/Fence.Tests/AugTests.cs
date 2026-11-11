using System.Collections.Generic;
using System.IO;
using System.Linq;
using Fence.Aug;
using Fence.Barter;
using Fence.Recording;
using Newtonsoft.Json.Linq;
using Xunit;

namespace Fence.Tests
{
    // Tests that change AugDetection.Current run alone: every context builder reads it.
    [CollectionDefinition("aug-detection", DisableParallelization = true)]
    public class AugDetectionCollection
    {
    }

    public class BubbleTests
    {
        [Theory]
        [InlineData("DEFAULT", "NONE", "plain", "lower_levels")]
        [InlineData("DEFAULT", "FACTION_UPPER_LEVEL", "upper", "upper_levels")]
        [InlineData("DEFAULT", "FACTION_TOURIST", "upper", "tourist")]
        [InlineData("DEFAULT", "FACTION_BLACK_MARKET", "black_market", "black_market")]
        [InlineData("", "FACTION_REVOLUTION", "revolution", "revolution")]
        [InlineData("sec", "NONE", "security", "security")]
        [InlineData("pink", "NONE", "pink", null)]
        [InlineData("DEFAULT", "FACTION_CARTEL", "plain", "cartel")]
        public void BubbleAndVisibleFaction(string color, string faction, string bubble, string visible)
        {
            Assert.Equal(bubble, Bubbles.Of(color, faction));
            Assert.Equal(visible, Factions.Visible(new ClientInfo { BubbleColor = color, FactionId = faction }));
        }

        [Fact]
        public void AnUndisguisedAugIsNeverNamedNone()
        {
            var g = new FakeBarterGame();
            g.ClientState.FactionId = "NONE";
            g.ClientState.BubbleColor = "DEFAULT";
            var snap = BarterContext.Read(g, 1);
            Assert.Equal("lower_levels", (string)snap.Context["client"]!["faction"]);
        }

        [Theory]
        [InlineData("augLLMale2", "lower-level clothes; unusual pupils, an uncanny expression or a visible chip")]
        [InlineData("maleMiner3", "miner's work gear")]
        [InlineData("femaleUpper1", "upper-level clothes")]
        [InlineData("unknownThing", null)]
        public void PortraitsInWords(string sprite, string words) => Assert.Equal(words, AugTells.PortraitWords(sprite));
    }

    public class AugCheckTests
    {
        // The undisguised aug behind protocol/fixtures/decide.aug_check.request.json.
        internal static AugSituation Fixture()
        {
            var g = new FakeBarterGame { RepBonus = 0 };
            var c = g.ClientState;
            c.Name = "Upper Level Citizen";
            c.Identifier = "aug_med";
            c.FactionId = "NONE";
            c.BubbleColor = "DEFAULT";
            c.Budget = 210;
            c.InterestTags = new List<string> { "MEDICAL" };
            c.AttractedItemId = null;
            c.IsAug = true;
            c.Aug = new AugInfo { Sprite = "maleUpper2", Idle = AugIdle.Normal, AugOnlyLines = true, NameClaim = "upper_levels", Tier = 3, IdleTell = "default" };
            g.UiState.AugIntroduced = true;
            g.StoreInfo.Day = 40;
            return new AugSituation
            {
                ClientSeq = 7,
                Client = c,
                Ui = g.UiState,
                Flags = g.Flags,
                Store = g.StoreInfo,
                AugsActive = true,
                ShootingUnlocked = true,
                IntroDay = 12,
                Difficulty = "normal",
                PresenceRise = 10,
                KillPresenceDrop = 25,
                GameOverAtFull = false,
                Lines = new List<string> { "I require medical supplies.", "This unit— I have 210 credits." },
                MarkupOdds = g.Odds(BargainKind.Markup, 10),
                DiscountOdds = g.Odds(BargainKind.BuyingDiscount, 10),
                Presence = 35,
            };
        }

        static JObject FixtureJson() => JObject.Parse(File.ReadAllText(Path.Combine(Paths.Fixtures, "decide.aug_check.request.json")));

        [Fact]
        public void MatchesTheFixture()
        {
            var q = AugCheck.Build(Fixture(), AugDetection.Perfect);
            var f = FixtureJson();
            Assert.True(JToken.DeepEquals(f["context"], q.Context), "context:\n" + q.Context);
            Assert.True(JToken.DeepEquals(f["hidden"], q.Hidden), "hidden:\n" + q.Hidden);
            Assert.Equal(f["options"]!.Select(o => ((string)o["id"], (string)o["label"])), q.Options.Select(o => (o.Id, o.Label)));
            Assert.Equal((string)f["state_signature"], q.Signature);
        }

        [Fact]
        public void PerfectModeAsksOnlyAboutAugs()
        {
            var s = Fixture();
            s.Client.IsAug = false;
            Assert.Null(AugCheck.Build(s, AugDetection.Perfect));
        }

        [Fact]
        public void NoQuestionWithoutTheGun()
        {
            var s = Fixture();
            s.ShootingUnlocked = false;
            Assert.Null(AugCheck.Build(s, AugDetection.Perfect));
            s = Fixture();
            s.Flags.ThreatenOffered = true;
            Assert.Null(AugCheck.Build(s, AugDetection.Perfect));
            s = Fixture();
            s.Client.IsSecurity = true;
            Assert.Null(AugCheck.Build(s, AugDetection.Perfect));
            s = Fixture();
            s.AugsActive = false;
            Assert.Null(AugCheck.Build(s, AugDetection.Perfect));
        }

        [Fact]
        public void RaisedGunStillAsksAndChangesTheSignature()
        {
            var a = AugCheck.Build(Fixture(), AugDetection.Perfect);
            var s = Fixture();
            s.ShootingMode = true;
            s.Arrestable = false;
            var b = AugCheck.Build(s, AugDetection.Perfect);
            Assert.NotNull(b);
            Assert.NotEqual(a.Signature, b.Signature);
            Assert.True((bool)b.Context["gun_raised"]);
        }

        // Invented stand-ins for the game's aug script (the real lines come from its string table at run time).
        internal static void InstallLines() => AugLines.Install(key =>
            key == "aug_dialog_tier1_0_a" ? "I require " + AugLines.Slot + " supplies."
            : key == "aug_dialog_budget_sus_0" ? "This unit— I have " + AugLines.Slot + " credits."
            : null);

        [Fact]
        public void AugLinesMatchTheScriptByItsText()
        {
            InstallLines();
            Assert.True(AugLines.IsAugLine("I require medical supplies."));
            Assert.True(AugLines.IsAugLine("Upper Level Citizen: I require <b>medical</b> supplies."));
            Assert.True(AugLines.IsAugLine("This unit— I have 210 credits."));
            Assert.False(AugLines.IsAugLine("I require nothing at all."));
            Assert.False(AugLines.IsAugLine("Hello. Medical supplies, please."));
            Assert.False(AugLines.AnyShown(new List<string>()));
        }

        [Fact]
        public void TellsModeDescribesInsteadOfStating()
        {
            InstallLines();
            var q = AugCheck.Build(Fixture(), AugDetection.Tells);
            var client = (JObject)q.Context["client"];
            Assert.Null(client["aug"]);
            var tells = client["aug_tells"]!.Select(t => (string)t).ToList();
            Assert.Contains(tells, t => t.StartsWith("speaks in aug-only lines"));
            Assert.Contains(tells, t => t.StartsWith("named \"Upper Level Citizen\" but talks in a plain bubble"));
            Assert.Equal("upper-level clothes", (string)client["portrait"]);
            Assert.Equal("bobs normally", (string)client["idle"]);
            Assert.Equal(2, client["lines"]!.Count());
            Assert.True((bool)q.Hidden["client"]!["is_aug"]);
            string context = q.Context.ToString();
            Assert.DoesNotContain("aug_med", context);
            Assert.DoesNotContain("aug_tier", context);
        }

        [Fact]
        public void TellsModeAsksOnSuspicionAndNotOnAPlainHuman()
        {
            var s = Fixture();
            s.Client.IsAug = false;
            s.Client.Aug = new AugInfo { Sprite = "maleLower1", Idle = AugIdle.Normal };
            s.Client.InterestTags = new List<string> { "FOOD" };
            s.Client.FactionId = "FACTION_LOWER_LEVEL";
            Assert.Null(AugCheck.Build(s, AugDetection.Tells));
            s.Client.InterestTags = new List<string> { "MATERIAL", "ENERGY_CELL" };
            var q = AugCheck.Build(s, AugDetection.Tells);
            Assert.NotNull(q);
            Assert.Null(q.Context["client"]!["aug_tells"]);
            s.Client.Aug.Scanned = true;
            Assert.Null(AugCheck.Build(s, AugDetection.Tells));
        }

        [Fact]
        public void ScanOptionOnlyWhileUnscanned()
        {
            var s = Fixture();
            s.ScannerOwned = true;
            Assert.Contains(AugCheck.Build(s, AugDetection.Perfect).Options, o => o.Id == AugOptionIds.Scan);
            s.Client.Aug.Scanned = true;
            var q = AugCheck.Build(s, AugDetection.Perfect);
            Assert.DoesNotContain(q.Options, o => o.Id == AugOptionIds.Scan);
            Assert.Equal("AUG DETECTED", (string)q.Context["client"]!["scan"]);
        }

        [Theory]
        [InlineData(12, 12, false, 100)]
        [InlineData(40, 12, false, 128)]
        [InlineData(400, 12, false, 250)]
        [InlineData(40, 1, true, 174)]
        public void KillReward(int day, int intro, bool menace, int reward) => Assert.Equal(reward, AugCheck.KillReward(day, intro, menace));

        [Theory]
        [InlineData(24, 250)]
        [InlineData(25, 500)]
        [InlineData(50, 750)]
        public void Manslaughter(int day, int crime) => Assert.Equal(crime, AugCheck.Manslaughter(day));

        [Fact]
        public void LeftAliveCause()
        {
            var t = new AugTracker();
            t.NewClient(new object(), 1);
            Assert.Equal("served", t.LeftCause(false));
            t.Dismissing = true;
            Assert.Equal("dismissed", t.LeftCause(false));
            t.Holstered = true;
            Assert.Equal("holstered", t.LeftCause(false));
            t.Reported = true;
            Assert.Equal("reported", t.LeftCause(false));
            Assert.Equal("threatened", t.LeftCause(true));
        }

        [Fact]
        public void TrackerAsksOncePerSignature()
        {
            var t = new AugTracker();
            t.NewClient(new object(), 7);
            var q = AugCheck.Build(Fixture(), AugDetection.Perfect);
            Assert.True(t.Show(q));
            t.Sent(q, "a-1");
            Assert.False(t.Show(AugCheck.Build(Fixture(), AugDetection.Perfect)));
            Assert.True(t.Thinking);
            t.Answered(q.Signature, new Advice { OptionId = "shoot", Rationale = "confirmed" });
            Assert.Equal("aug: shoot — confirmed", AugText.Line(t.Current, t.CurrentAnswer, t.Thinking));
            Assert.Equal("a-1", t.LastDecisionId);
            Assert.Equal("shoot", t.LastAdvised);
        }

        [Fact]
        public void EventsAndOutcomesShareAShape()
        {
            var data = AugEvents.Data(AugEvents.WrongfulKill, 4, "Scav", new JObject { ["crime"] = new JObject { ["id"] = "MANSLAUGHTER", ["amount"] = 500 } });
            var outcome = AugEvents.Outcome(data);
            Assert.Equal("aug", (string)outcome["kind"]);
            Assert.Equal("wrongful_kill", (string)outcome["result"]);
            Assert.Equal(500, (int)outcome["crime"]!["amount"]);
            Assert.Null(outcome["client_seq"]);
            Assert.Equal("shot a human: manslaughter", AugEvents.Describe(outcome));
        }
    }

    [Collection("aug-detection")]
    public class AugDetectionModeTests
    {
        [Fact]
        public void BarterAndCustomerContextsFollowTheMode()
        {
            var g = new FakeBarterGame();
            g.ClientState.IsAug = true;
            g.ClientState.Aug = new AugInfo { Idle = AugIdle.Twitch, Tier = 2, IdleTell = "seizure", Disguised = true };
            try
            {
                AugDetection.Current = AugDetection.Perfect;
                var perfect = BarterContext.Read(g, 1);
                Assert.True((bool)perfect.Context["client"]!["aug"]);
                Assert.Equal(2, (int)perfect.Hidden["client"]!["aug_tier"]);
                Assert.True((bool)CustomerObservation.MinimalContext(g.ClientState)["client"]!["aug"]);

                AugDetection.Current = AugDetection.Tells;
                var tells = BarterContext.Read(g, 1);
                Assert.Null(tells.Context["client"]!["aug"]);
                Assert.Contains("twitching", (string)tells.Context["client"]!["aug_tells"]![0]);
                Assert.DoesNotContain("seizure", tells.Context.ToString());
                Assert.Null(CustomerObservation.MinimalContext(g.ClientState)["client"]!["aug"]);
            }
            finally
            {
                AugDetection.Current = AugDetection.Perfect;
            }
        }

        [Fact]
        public void SnapshotStatesTheMode()
        {
            try
            {
                AugDetection.Current = AugDetection.Tells;
                var r = DaySnapshot.Build(new FakeDay(), SnapshotPhases.Morning, "run-1", _ => true);
                Assert.Equal("tells", (string)r.Data["store"]!["aug_detection"]);
            }
            finally
            {
                AugDetection.Current = AugDetection.Perfect;
            }
        }
    }

    public class SnapshotPerksAndCapacityTests
    {
        [Fact]
        public void PerksInForce()
        {
            var r = DaySnapshot.Build(new FakeDay(), SnapshotPhases.Morning, "run-1", _ => true);
            Assert.True(JToken.DeepEquals(JObject.Parse("{\"starting\":[\"aug_menace\"],\"reputation\":[\"LLREFERAL\"]}"), r.Data["perks"]), r.Data["perks"]!.ToString());
        }

        [Fact]
        public void PerksSurviveAFailedReputationSection()
        {
            var g = new FakeDay();
            g.Throwing.Add("perks");
            var r = DaySnapshot.Build(g, SnapshotPhases.Morning, "run-1", _ => true);
            Assert.Null(r.Data["perks"]);
            Assert.Contains("perks", r.Data["unavailable"]!.Select(x => (string)x));
        }

        [Fact]
        public void ContainersCarryTheirCapacity()
        {
            var r = DaySnapshot.Build(new FakeDay(), SnapshotPhases.Morning, "run-1", _ => true);
            var bay = r.Data["inventory"]!.First(i => (string)i["id"] == "smuggler_bay");
            Assert.Equal(12, (int)bay["capacity"]!["cells"]);
            Assert.Equal(9, (int)bay["capacity"]!["free"]);
            Assert.Null(r.Data["inventory"]!.First(i => (string)i["id"] == "pills")["capacity"]);
        }
    }
}
