using System.Collections.Generic;
using System.IO;
using System.Linq;
using Fence.Barter;
using Fence.Decisions;
using Fence.Protocol;
using Newtonsoft.Json.Linq;
using Xunit;

namespace Fence.Tests
{
    public class StageTests
    {
        // The shop behind protocol/fixtures/decide.stage.request.json: the retired farmer, nothing staged.
        static List<ShopItem> Shop() => new List<ShopItem>
        {
            Shelf(new ItemInfo
            {
                Uid = 2231, Id = "cassette2", Name = "Cassette", Price = 345, EstimatedValue = 300, BoughtAt = 180, StolenHeat = 22, AppraisalValue = 95,
                Features = new List<FeatureInfo>
                {
                    new FeatureInfo { Visible = true, Shown = "Worn", Real = "Worn", Group = FeatureGroup.Condition },
                    new FeatureInfo { Visible = false, Shown = "Counterfeit", Real = "Counterfeit", Group = FeatureGroup.Condition },
                },
            }, StageWhere.Stock, "MUSIC"),
            Shelf(new ItemInfo { Uid = 2260, Id = "synth", Name = "Synth", Price = 1150, EstimatedValue = 1000, BoughtAt = 600 }, StageWhere.Stock, "MUSIC"),
            Shelf(new ItemInfo { Uid = 2255, Id = "walkman", Name = "Walkman", Price = 230, EstimatedValue = 200, BoughtAt = 260 }, StageWhere.SmugglerBay, "ELECTRONICS"),
            Shelf(new ItemInfo { Uid = 2240, Id = "radio", Name = "Radio", Price = 598, EstimatedValue = 520, BoughtAt = 300 }, StageWhere.Showcase, "ELECTRONICS", "MUSIC"),
            Shelf(new ItemInfo { Uid = 2270, Id = "pistol", Name = "Pistol", Price = 400, EstimatedValue = 350, BoughtAt = 100, ClientWants = false }, StageWhere.Stock),
        };

        static ShopItem Shelf(ItemInfo i, string where, params string[] matches)
        {
            i.Owned = true;
            if (i.Id != "pistol") i.ClientWants = true;
            return new ShopItem { Item = i, Where = where, Matches = matches.ToList() };
        }

        static FakeBarterGame NothingStaged() => new FakeBarterGame { Items = new List<ItemInfo>() };

        static BarterSnapshot Stage(FakeBarterGame g, IEnumerable<ShopItem> shop, int clientSeq = 41)
        {
            var barter = BarterContext.Read(g, clientSeq);
            Assert.True(barter.Ready);
            Assert.True(StageContext.IsMoment(barter.Client, barter.Items, barter.Ui, out var left, out var suitable));
            var cands = StageContext.Rank(barter.Client, shop, left, out int total);
            return StageContext.Build(barter, cands, total, left, suitable);
        }

        [Fact]
        public void MatchesTheFixture()
        {
            var snap = Stage(NothingStaged(), Shop());
            var fixture = JObject.Parse(File.ReadAllText(Path.Combine(Paths.Fixtures, "decide.stage.request.json")));
            Assert.True(JToken.DeepEquals(fixture["context"], snap.Context), "context:\n" + snap.Context);
            Assert.True(JToken.DeepEquals(fixture["hidden"], snap.Hidden), "hidden:\n" + snap.Hidden);
            Assert.Equal(fixture["options"]!.Select(o => ((string)o["id"], (string)o["label"])), snap.Options.Select(o => (o.Id, o.Label)));
            Assert.Equal((string)fixture["state_signature"], snap.Signature);

            var wire = JObject.Parse(ProtocolJson.Serialize(BarterContext.Request(snap, "d-0002", 20000)));
            Assert.Equal("stage", (string)wire["kind"]);
            Assert.Null(wire["context"]!["hidden"]);
            Assert.DoesNotContain("Counterfeit", wire["context"]!.ToString());
        }

        [Fact]
        public void NeverOffersWhatTheCustomerMustNotGet()
        {
            var shop = new List<ShopItem>
            {
                Shelf(new ItemInfo { Uid = 1, Id = "watch", Name = "Watch", Price = 100, StolenHeat = 40 }, StageWhere.SmugglerBay),
                Shelf(new ItemInfo { Uid = 2, Id = "pills", Name = "Pills", Price = 100, ContrabandLevel = 2 }, StageWhere.Stock),
                Shelf(new ItemInfo { Uid = 3, Id = "rifle", Name = "Rifle", Price = 100, SeriousContraband = true }, StageWhere.Stock),
                Shelf(new ItemInfo { Uid = 4, Id = "lamp", Name = "Lamp", Price = 100 }, StageWhere.Stock),
                Shelf(new ItemInfo { Uid = 5, Id = "free", Name = "Free", Price = 0 }, StageWhere.Stock),
            };
            var cases = new (ClientInfo client, int[] uids)[]
            {
                (new ClientInfo { Intent = ClientIntent.Buy }, new[] { 1, 2, 3, 4 }),
                (new ClientInfo { Intent = ClientIntent.Buy, IsSecurity = true }, new[] { 4 }),
                (new ClientInfo { Intent = ClientIntent.Buy, NoContraband = true }, new[] { 1, 4 }),
            };
            foreach (var (client, uids) in cases)
            {
                var got = StageContext.Rank(client, shop, null, out int total).Select(c => c.Shop.Item.Uid).OrderBy(u => u).ToArray();
                Assert.Equal(uids, got);
                Assert.Equal(uids.Length, total);
            }
            shop[3].Item.ClientWants = false;
            Assert.DoesNotContain(StageContext.Rank(new ClientInfo { Intent = ClientIntent.Buy }, shop, null, out _), c => c.Shop.Item.Uid == 4);
        }

        [Fact]
        public void RanksByFitAndCaps()
        {
            var client = new ClientInfo { Intent = ClientIntent.Buy, AttractedItemId = "came_for" };
            var shop = new List<ShopItem>
            {
                Shelf(new ItemInfo { Uid = 10, Id = "big", Name = "Big", Price = 900, BoughtAt = 10 }, StageWhere.Stock, "FOOD"),
                Shelf(new ItemInfo { Uid = 11, Id = "hot", Name = "Hot", Price = 300, BoughtAt = 10, StolenHeat = 30 }, StageWhere.Stock, "FOOD"),
                Shelf(new ItemInfo { Uid = 12, Id = "plain", Name = "Plain", Price = 300, BoughtAt = 10 }, StageWhere.Stock),
                Shelf(new ItemInfo { Uid = 13, Id = "thin", Name = "Thin", Price = 100, BoughtAt = 90 }, StageWhere.Stock, "FOOD"),
                Shelf(new ItemInfo { Uid = 14, Id = "fat", Name = "Fat", Price = 200, BoughtAt = 10 }, StageWhere.Stock, "FOOD"),
                Shelf(new ItemInfo { Uid = 15, Id = "came_for", Name = "Came for", Price = 950, BoughtAt = 900, StolenHeat = 80 }, StageWhere.Stock),
            };
            var order = StageContext.Rank(client, shop, 500, out _).Select(c => c.Shop.Item.Uid);
            Assert.Equal(new[] { 15, 14, 13, 12, 11, 10 }, order);

            var many = Enumerable.Range(1, 20).Select(i => Shelf(new ItemInfo { Uid = i, Id = "x" + i, Name = "X", Price = i }, StageWhere.Stock)).ToList();
            var capped = StageContext.Rank(client, many, null, out int total);
            Assert.Equal(StageContext.MaxCandidates, capped.Count);
            Assert.Equal(20, total);
            var barter = BarterContext.Read(NothingStaged(), 1);
            var snap = StageContext.Build(barter, capped, total, null, false);
            Assert.Equal(8, (int)snap.Context["candidates_omitted"]);
            Assert.Equal(JTokenType.Null, snap.Context["budget_left"]!.Type);
        }

        [Theory]
        [InlineData("buy, nothing staged", ClientIntent.Buy, false, false, true, 900, false)]
        [InlineData("buy, something they want staged", ClientIntent.Buy, true, false, true, 560, true)]
        [InlineData("buy, staged item they do not want", ClientIntent.Buy, true, false, true, 900, false)]
        [InlineData("sell", ClientIntent.Sell, false, true, false, null, false)]
        [InlineData("sell and buy, their items still on the counter", ClientIntent.SellAndBuy, false, true, false, null, false)]
        [InlineData("sell and buy, their items gone", ClientIntent.SellAndBuy, false, false, true, 900, false)]
        public void Moment(string name, ClientIntent intent, bool staged, bool theirs, bool expect, int? left, bool suitable)
        {
            var g = NothingStaged();
            g.ClientState.Intent = intent;
            if (staged) g.Items.Add(new ItemInfo { Uid = 1, Id = "x", Price = 340, Owned = true, OnBack = true, ClientWants = name.Contains("they want") });
            if (theirs) g.Items.Add(new ItemInfo { Uid = 2, Id = "y", Price = 50, OnBack = true });
            Assert.Equal(expect, StageContext.IsMoment(g.ClientState, g.Items, g.UiState, out var budgetLeft, out var s));
            if (expect)
            {
                Assert.Equal(left, budgetLeft);
                Assert.Equal(suitable, s);
            }
        }

        [Fact]
        public void NoMomentWithoutBudgetOrWhenDone()
        {
            var g = NothingStaged();
            g.Items.Add(new ItemInfo { Uid = 1, Id = "x", Price = 900, Owned = true, OnBack = true, ClientWants = true });
            Assert.False(StageContext.IsMoment(g.ClientState, g.Items, g.UiState, out _, out _));
            g.Items.Clear();
            g.ClientState.BusinessComplete = true;
            Assert.False(StageContext.IsMoment(g.ClientState, g.Items, g.UiState, out _, out _));
            g.ClientState.BusinessComplete = false;
            g.ClientState.UsesBudget = false;
            Assert.True(StageContext.IsMoment(g.ClientState, g.Items, g.UiState, out var left, out _));
            Assert.Null(left);
        }

        [Fact]
        public void SignatureFollowsTheCounterAndTheBudget()
        {
            Assert.Equal("sc41:i:b0000:mb:g900", StageContext.Signature("c41:i:b0000:mb", 900));
            Assert.Equal("sc41:i:b0000:mb:g-", StageContext.Signature("c41:i:b0000:mb", null));
            var g = NothingStaged();
            string before = Stage(g, Shop()).Signature;
            g.Items.Add(new ItemInfo { Uid = 2240, Id = "radio", Price = 598, Owned = true, OnBack = true, ClientWants = true });
            var after = Stage(g, Shop());
            Assert.NotEqual(before, after.Signature);
            Assert.Equal(302L, (long)after.Context["budget_left"]);
            Assert.Equal("Nothing more to offer: sell what is staged", after.Options.Single(o => o.Id == StageOptionIds.NothingFits).Label);
        }

        [Theory]
        [InlineData(false, 0, null, false, true)]
        [InlineData(false, 3, "offer_1", false, true)]
        [InlineData(true, 0, null, false, false)]
        [InlineData(true, 3, null, false, true)]
        [InlineData(true, 3, "offer_1", false, true)]
        [InlineData(true, 3, "nothing_fits", false, false)]
        [InlineData(true, 3, null, true, false)]
        public void AsksStageOnlyWhenItHelps(bool suitableStaged, int candidates, string answer, bool deferred, bool expect)
        {
            var advice = answer == null && !deferred ? null : new Advice { OptionId = answer, Deferred = deferred };
            Assert.Equal(expect, StageContext.Ask(suitableStaged, candidates, advice));
        }

        [Fact]
        public void StageAnswersAreNeverExecuted()
        {
            foreach (var o in Stage(NothingStaged(), Shop()).Options.Where(o => o.Id != OptionIds.Dismiss))
                Assert.Null(ActCommands.Parse(o.Id));
        }

        [Theory]
        [InlineData(Modes.Advise, 20000)]
        [InlineData(Modes.Act, 10000)]
        [InlineData(Modes.Observe, 20000)]
        [InlineData(null, 20000)]
        public void DeadlineFollowsTheMode(string mode, int expected) => Assert.Equal(expected, DecisionDeadlines.ForMode(mode));

        [Theory]
        [InlineData("stage", "offer_2240", AdviceStatus.Ready, "offer: Radio #2240 (showcase)")]
        [InlineData("stage", "offer_2240", AdviceStatus.Stale, "was: offer: Radio #2240 (showcase)")]
        [InlineData("stage", "nothing_fits", AdviceStatus.Ready, "fence suggests: Nothing fits")]
        [InlineData("barter", "offer_2240", AdviceStatus.Ready, "fence suggests: Radio #2240 (showcase)")]
        public void OverlayShowsTheOffer(string kind, string option, AdviceStatus status, string move)
        {
            var a = new Advice { Kind = kind, OptionId = option, Label = option == "nothing_fits" ? "Nothing fits" : "Radio #2240 (showcase)", Signature = "s" };
            var v = OverlayText.Build(new OverlayInput
            {
                Mode = Modes.Advise, Bridge = BridgeStatus.Connected, Client = new FakeBarterGame().ClientState, Ready = true,
                View = new AdviceView { Status = status, Advice = a }, Today = new List<DayEntry>(),
            });
            Assert.Equal(move, v.Move);
        }

        [Fact]
        public void TrackerReturnsTheAnswerForASignature()
        {
            var t = new AdviceTracker();
            t.NewClient(new object());
            Assert.Null(t.AnswerFor("s1"));
            t.Sent("s1", "d1");
            t.Answered(new Advice { DecisionId = "d1", Signature = "s1", OptionId = StageOptionIds.NothingFits });
            Assert.Equal(StageOptionIds.NothingFits, t.AnswerFor("s1").OptionId);
            Assert.Null(t.AnswerFor(null));
        }
    }
}
