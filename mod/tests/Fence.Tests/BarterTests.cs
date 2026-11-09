using System;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using Fence.Barter;
using Fence.Protocol;
using Newtonsoft.Json.Linq;
using Xunit;

namespace Fence.Tests
{
    // A game state mirroring protocol/fixtures/decide.request.json.
    sealed class FakeBarterGame : IBarterGame
    {
        public StoreInfo StoreInfo = new StoreInfo { Cash = 2450, Day = 69, RetailMarkup = 15, RentDue = 600, RentDaysLeft = 3 };
        public List<FactionRep> Reps = new List<FactionRep>
        {
            new FactionRep { FactionId = "FACTION_LOWER_LEVEL", Value = 12.0, Status = "Known" },
            new FactionRep { FactionId = "FACTION_UPPER_LEVEL", Value = -4.5, Status = "Neutral" },
            new FactionRep { FactionId = "FACTION_SECURITY", Value = 3.0, Status = "Neutral" },
            new FactionRep { FactionId = "FACTION_BLACK_MARKET", Value = 20.25, Status = "Known" },
            new FactionRep { FactionId = "FACTION_REVOLUTION", Value = 0.0, Status = "Neutral" },
        };
        public ClientInfo ClientState = new ClientInfo
        {
            Key = new object(),
            Name = "Retired farmer",
            Identifier = "retired_farmer",
            FactionId = "FACTION_LOWER_LEVEL",
            Intent = ClientIntent.Buy,
            RawIntent = "BUY",
            UsesBudget = true,
            Budget = 900,
            InterestTags = new List<string> { "ELECTRONICS", "MUSIC" },
            AttractedItemId = "cassette2",
            Dismissable = true,
            BargainAvailable = true,
            MainDialogueStarted = true,
        };
        public List<ItemInfo> Items = new List<ItemInfo>
        {
            new ItemInfo
            {
                Uid = 2231, Id = "cassette2", Name = "Cassette", Price = 340, EstimatedValue = 300, BoughtAt = 180,
                Owned = true, OnBack = true, ClientWants = true, StolenHeat = 22, AppraisalValue = 95,
                Features = new List<FeatureInfo>
                {
                    new FeatureInfo { Visible = true, Shown = "Worn", Real = "Worn", Group = FeatureGroup.Condition },
                    new FeatureInfo { Visible = false, Shown = "Counterfeit", Real = "Counterfeit", Group = FeatureGroup.Condition },
                },
            },
        };
        public BargainFlags Flags = new BargainFlags();
        public UiState UiState = new UiState { ClientPresent = true, ClientArrived = true, BargainIntroduced = true };
        public int RepBonus = 2;

        public StoreInfo Store() => StoreInfo;
        public IList<FactionRep> Reputations() => Reps;
        public ClientInfo Client() => ClientState;
        public IList<ItemInfo> CounterItems() => Items;
        public BargainFlags Bargain() => Flags;
        public UiState Ui() => UiState;
        public IList<MarketEvent> MarketEvents() => new List<MarketEvent> { new MarketEvent { ItemType = "ELECTRONICS", ValuePct = 10 } };
        public int? DaysToInspection() => 3;

        // BargainUIManager.GetBargainSuccessChance / GetBargainLeaveChance with rep bonus 2, no event, no deal maker.
        public Odds Odds(BargainKind kind, int pct)
        {
            switch (kind)
            {
                case BargainKind.Markup:
                case BargainKind.BuyingDiscount:
                    int a = Math.Clamp((int)Math.Round(100 - pct * 2.0 + RepBonus), 0, 100);
                    int b = (int)Math.Round(100 - pct * 1.2 + RepBonus);
                    return new Odds { Success = a, Leave = Math.Clamp(100 - Math.Max(a, b), 0, 100) };
                case BargainKind.Blackmail:
                    return new Odds { Success = 60 };
                default:
                    return new Odds { Success = 100 };
            }
        }
    }

    public class BarterContextTests
    {
        static JObject Fixture() => JObject.Parse(File.ReadAllText(Path.Combine(Paths.Fixtures, "decide.request.json")));

        [Fact]
        public void ContextAndHiddenMatchTheFixture()
        {
            var snap = BarterContext.Read(new FakeBarterGame(), 41);
            Assert.True(snap.Ready);
            var fixture = Fixture();
            Assert.True(JToken.DeepEquals(fixture["context"], snap.Context), "context:\n" + snap.Context);
            Assert.True(JToken.DeepEquals(fixture["hidden"], snap.Hidden), "hidden:\n" + snap.Hidden);
        }

        // The fixture lists an abbreviated option set; every option it shows must come out identical
        // and in the same order, and the rest are the steps it leaves out.
        [Fact]
        public void OptionsMatchTheFixtureLabels()
        {
            var snap = BarterContext.Read(new FakeBarterGame(), 41);
            var fixture = Fixture()["options"]!.Select(o => (id: (string)o["id"], label: (string)o["label"])).ToList();
            var built = snap.Options.Select(o => (id: o.Id, label: o.Label)).ToList();
            foreach (var f in fixture) Assert.Contains(f, built);
            var order = built.Where(b => fixture.Any(f => f.id == b.id)).Select(b => b.id);
            Assert.Equal(fixture.Select(f => f.id), order);
            var extra = built.Select(b => b.id).Except(fixture.Select(f => f.id)).OrderBy(x => x).ToList();
            Assert.Equal(new[] { "add_markup_50", "add_markup_75", "blackmail", "offer_discount_25", "offer_discount_50", "offer_discount_75" }, extra);
        }

        [Fact]
        public void NothingHiddenAppearsInContext()
        {
            var snap = BarterContext.Read(new FakeBarterGame(), 41);
            string context = snap.Context.ToString(Newtonsoft.Json.Formatting.None);
            var contextKeys = snap.Context.Descendants().OfType<JProperty>().Select(p => p.Name).ToHashSet();
            foreach (var key in new[] { "identifier", "is_aug", "stealth_wanted", "true_value", "real_features", "days_to_inspection", "hidden" })
                Assert.DoesNotContain(key, contextKeys);
            Assert.DoesNotContain("retired_farmer", context);
            Assert.DoesNotContain("Counterfeit", context);
            Assert.DoesNotContain("\"true_value\"", context);

            var request = BarterContext.Request(snap, "d-1", 20000);
            var wire = JObject.Parse(ProtocolJson.Serialize(request));
            Assert.Null(wire["context"]!["hidden"]);
            Assert.Equal("barter", (string)wire["kind"]);
            Assert.Equal(69, (int)wire["game_day"]);
        }

        [Fact]
        public void SignatureIgnoresOrderButNotPricesOrFlags()
        {
            var g = new FakeBarterGame();
            g.Items.Add(new ItemInfo { Uid = 7, Id = "x", Name = "X", Price = 10, Owned = true, OnBack = true });
            string a = BarterContext.Read(g, 1).Signature;
            g.Items.Reverse();
            Assert.Equal(a, BarterContext.Read(g, 1).Signature);
            g.Items[0].Price = 11;
            string b = BarterContext.Read(g, 1).Signature;
            Assert.NotEqual(a, b);
            g.Flags.MarkupOffered = true;
            Assert.NotEqual(b, BarterContext.Read(g, 1).Signature);
            Assert.NotEqual(b, BarterContext.Read(g, 2).Signature);
        }

        [Theory]
        [InlineData("Worn", "Worn")]
        [InlineData("-Worn (-20%)", "Worn (-20%)")]
        [InlineData("<color=#39a78e>-Worn (-20%) ✓</color>", "Worn (-20%) ✓")]
        [InlineData("<color=#808080><s>-Mint (+30%)</s> </color> \n -Counterfeit (-80%)", "Counterfeit (-80%) (exposed; claimed Mint (+30%))")]
        [InlineData(null, "")]
        public void FeatureTextIsPlain(string raw, string expected) => Assert.Equal(expected, FeatureText.Plain(raw));

        [Fact]
        public void TrueValueRestagesRealModifiers()
        {
            var item = new ItemInfo
            {
                EstimatedValue = 120,
                Features = { new FeatureInfo { Group = FeatureGroup.Condition, CurrentMod = 20, ActualMod = -50 } },
            };
            Assert.Equal(50, BarterContext.TrueValue(item));
            item.AppraisalValue = 77;
            Assert.Equal(77, BarterContext.TrueValue(item));
        }

        [Fact]
        public void MissingReadsAreOmittedNotGuessed()
        {
            var g = new FakeBarterGame { StoreInfo = new StoreInfo { Cash = 5 } };
            g.Items[0].EstimatedValue = null;
            g.Items[0].StolenHeat = null;
            var snap = BarterContext.Read(g, 1);
            Assert.Equal(new[] { "cash" }, ((JObject)snap.Context["store"]!).Properties().Select(p => p.Name));
            var item = (JObject)snap.Context["counter"]!["yours_staged"]![0]!;
            Assert.Null(item["estimated_value"]);
            Assert.Equal(JTokenType.Null, item["stolen_heat"]!.Type);
        }

        [Fact]
        public void SellClientBudgetIsNullAndTheirItemsHaveNoBoughtAt()
        {
            var g = new FakeBarterGame();
            g.ClientState.Intent = ClientIntent.Sell;
            g.Items = new List<ItemInfo> { new ItemInfo { Uid = 1, Id = "a", Name = "A", Price = 50, BoughtAt = 30, Owned = false, OnBack = true } };
            var snap = BarterContext.Read(g, 1);
            Assert.Equal(JTokenType.Null, snap.Context["client"]!["budget"]!.Type);
            Assert.Empty((JArray)snap.Context["client"]!["interests"]!);
            Assert.Null(snap.Context["counter"]!["theirs"]![0]!["bought_at"]);
            Assert.Equal("sell", (string)snap.Context["client"]!["intent"]);
        }
    }

    public class BarterRulesTests
    {
        static ClientInfo Client(ClientIntent intent = ClientIntent.Buy) => new ClientInfo
        {
            Intent = intent, Dismissable = true, BargainAvailable = true, MainDialogueStarted = true, UsesBudget = true, Budget = 500,
        };

        static UiState Ready() => new UiState { ClientPresent = true, ClientArrived = true, BargainIntroduced = true };

        static ItemInfo Own(int uid, long price, bool wants = true) => new ItemInfo { Uid = uid, Price = price, Owned = true, OnBack = true, ClientWants = wants };
        static ItemInfo Theirs(int uid, long price, bool back = true) => new ItemInfo { Uid = uid, Price = price, Owned = false, OnBack = back };

        static Odds FlatOdds(BargainKind k, int p) => new Odds { Success = 50, Leave = 10 };

        static List<string> Ids(ClientInfo c, IList<ItemInfo> items, BargainFlags f = null, UiState ui = null, int cash = 1000) =>
            BarterRules.Options(new StoreInfo { Cash = cash }, c, items, f ?? new BargainFlags(), ui ?? Ready(), FlatOdds).Select(o => o.Id).ToList();

        [Theory]
        [InlineData(ClientIntent.Buy, true, false, false, null)]
        [InlineData(ClientIntent.Other, true, false, false, "intent BARTER")]
        [InlineData(ClientIntent.Dialogue, true, false, false, "intent BARTER")]
        [InlineData(ClientIntent.Sell, false, false, false, "not dismissable")]
        [InlineData(ClientIntent.SellAndBuy, true, true, false, "security")]
        [InlineData(ClientIntent.Buy, true, false, true, "special client")]
        public void Coverage(ClientIntent intent, bool dismissable, bool security, bool wanted, string gap)
        {
            var c = Client(intent);
            c.RawIntent = "BARTER";
            c.Dismissable = dismissable;
            c.IsSecurity = security;
            c.IsWanted = wanted;
            Assert.Equal(gap, BarterRules.CoverageGap(c));
        }

        public static IEnumerable<object[]> NotReady()
        {
            yield return new object[] { new UiState { ClientPresent = true, ClientArrived = false } };
            yield return new object[] { new UiState { ClientPresent = true, ClientArrived = true, DialoguePlaying = true } };
            yield return new object[] { new UiState { ClientPresent = true, ClientArrived = true, ClientMoving = true } };
            yield return new object[] { new UiState { ClientPresent = true, ClientArrived = true, AboutToStartMainDialog = true } };
            yield return new object[] { new UiState { ClientPresent = true, ClientArrived = true, TradeLocked = true } };
        }

        [Theory]
        [MemberData(nameof(NotReady))]
        public void NotReadyWhileTheGameIsBusy(UiState ui) =>
            Assert.False(BarterRules.DecisionReady(ui, Client(), new List<ItemInfo>()));

        [Fact]
        public void ReadinessNeedsMainDialogueAndSellerItems()
        {
            Assert.True(BarterRules.DecisionReady(Ready(), Client(), new List<ItemInfo>()));
            var c = Client();
            c.MainDialogueStarted = false;
            Assert.False(BarterRules.DecisionReady(Ready(), c, new List<ItemInfo>()));
            Assert.False(BarterRules.DecisionReady(Ready(), Client(ClientIntent.Sell), new List<ItemInfo> { Own(1, 5) }));
            Assert.True(BarterRules.DecisionReady(Ready(), Client(ClientIntent.Sell), new List<ItemInfo> { Theirs(1, 5, back: false) }));
            var done = Client();
            done.BusinessComplete = true;
            Assert.False(BarterRules.DecisionReady(Ready(), done, new List<ItemInfo>()));
        }

        [Fact]
        public void BuyingClientGetsSellSideMoves()
        {
            var ids = Ids(Client(), new List<ItemInfo> { Own(1, 100), Own(2, 50, wants: false) });
            Assert.Equal(new[]
            {
                "sell_all", "offer_discount_10", "offer_discount_25", "offer_discount_50", "offer_discount_75",
                "add_markup_10", "add_markup_25", "add_markup_50", "add_markup_75", "blackmail", "dismiss",
            }, ids);
        }

        [Fact]
        public void SellingClientGetsBuySideMoves()
        {
            var c = Client(ClientIntent.Sell);
            c.OffersWholesale = true;
            c.WholesaleDiscount = -15;
            c.WholesalePrice = 170;
            var ids = Ids(c, new List<ItemInfo> { Theirs(1, 100), Theirs(2, 100, back: false) });
            Assert.Equal(new[]
            {
                "buy_all", "accept_wholesale", "offer_markup_10", "offer_markup_25", "offer_markup_50", "offer_markup_75",
                "demand_discount_10", "demand_discount_25", "demand_discount_50", "demand_discount_75", "blackmail", "dismiss",
            }, ids);
        }

        // One row per UI-only gate (docs/re/BARTER.md, "Buttons → methods").
        public static IEnumerable<object[]> Gates()
        {
            BargainFlags F(Action<BargainFlags> set) { var f = new BargainFlags(); set(f); return f; }
            UiState U(Action<UiState> set) { var u = Ready(); set(u); return u; }
            var buy = Client();
            var sell = Client(ClientIntent.Sell);
            ClientInfo C(ClientIntent i, Action<ClientInfo> set) { var c = Client(i); set(c); return c; }

            yield return new object[] { "sell side used: no discount/markup", buy, F(f => f.MarkupOffered = true), Ready(), "offer_discount_10", false };
            yield return new object[] { "sell side used blocks markup", buy, F(f => f.DiscountOffered = true), Ready(), "add_markup_10", false };
            yield return new object[] { "coerced: no discount", buy, F(f => f.BlackmailOffered = true), Ready(), "offer_discount_10", false };
            yield return new object[] { "coerced: markup still allowed", buy, F(f => f.ThreatenOffered = true), Ready(), "add_markup_10", true };
            yield return new object[] { "buy side used: no offer markup", sell, F(f => f.BuyingDiscountOffered = true), Ready(), "offer_markup_10", false };
            yield return new object[] { "coerced: no offer markup", sell, F(f => f.BlackmailOffered = true), Ready(), "offer_markup_10", false };
            yield return new object[] { "coerced: demand discount allowed", sell, F(f => f.BlackmailOffered = true), Ready(), "demand_discount_25", true };
            yield return new object[] { "one blackmail", buy, F(f => f.BlackmailOffered = true), Ready(), "blackmail", false };
            yield return new object[] { "no blackmail on security", C(ClientIntent.Buy, c => c.IsSecurity = true), new BargainFlags(), Ready(), "blackmail", false };
            yield return new object[] { "threaten needs augs introduced", buy, new BargainFlags(), Ready(), "threaten", false };
            yield return new object[] { "threaten once augs exist", buy, new BargainFlags(), U(u => u.AugIntroduced = true), "threaten", true };
            yield return new object[] { "one threaten", buy, F(f => f.ThreatenOffered = true), U(u => u.AugIntroduced = true), "threaten", false };
            yield return new object[] { "bargain not introduced", buy, new BargainFlags(), U(u => u.BargainIntroduced = false), "add_markup_10", false };
            yield return new object[] { "awful negotiator perk", buy, new BargainFlags(), U(u => u.AwfulNegotiator = true), "add_markup_10", false };
            yield return new object[] { "outside", buy, new BargainFlags(), U(u => u.Outside = true), "blackmail", false };
            yield return new object[] { "dialogue playing", buy, new BargainFlags(), U(u => u.DialoguePlaying = true), "add_markup_10", false };
            yield return new object[] { "client moving", buy, new BargainFlags(), U(u => u.ClientMoving = true), "add_markup_10", false };
            yield return new object[] { "trade locked", buy, new BargainFlags(), U(u => u.TradeLocked = true), "sell_all", false };
            yield return new object[] { "client has no bargaining", C(ClientIntent.Buy, c => c.BargainAvailable = false), new BargainFlags(), Ready(), "add_markup_10", false };
            yield return new object[] { "bell locked: no dismiss", buy, new BargainFlags(), U(u => u.LockNextBell = 1), "dismiss", false };
            yield return new object[] { "business complete: no sale", C(ClientIntent.Buy, c => c.BusinessComplete = true), new BargainFlags(), Ready(), "sell_all", false };
        }

        [Theory]
        [MemberData(nameof(Gates))]
        public void Gate(string why, ClientInfo c, BargainFlags f, UiState ui, string option, bool offered)
        {
            var items = BarterRules.IsBuying(c.Intent) ? new List<ItemInfo> { Own(1, 100) } : new List<ItemInfo> { Theirs(1, 100) };
            Assert.True(offered == Ids(c, items, f, ui).Contains(option), why);
        }

        [Fact]
        public void BuyAllNeedsCashAndAPlainMultiBuy()
        {
            var c = Client(ClientIntent.Sell);
            var two = new List<ItemInfo> { Theirs(1, 300), Theirs(2, 300) };
            Assert.Contains("buy_all", Ids(c, two, cash: 600));
            Assert.DoesNotContain("buy_all", Ids(c, two, cash: 599));
            c.MultiBuyDisabled = true;
            Assert.DoesNotContain("buy_all", Ids(c, two));
            Assert.Contains("buy_all", Ids(c, new List<ItemInfo> { Theirs(1, 300) }));
            c.MultiBuyDisabled = false;
            c.RegularBuyDisabled = true;
            Assert.DoesNotContain("buy_all", Ids(c, two));
            Assert.DoesNotContain("buy_all", Ids(Client(ClientIntent.Sell), new List<ItemInfo> { Theirs(1, 300, back: false) }));
        }

        [Fact]
        public void WholesaleNeedsTheCashTheMethodDoesNotCheck()
        {
            var c = Client(ClientIntent.Sell);
            c.OffersWholesale = true;
            c.WholesaleDiscount = -15;
            c.WholesalePrice = 850;
            var items = new List<ItemInfo> { Theirs(1, 1000, back: false) };
            Assert.Contains("accept_wholesale", Ids(c, items, cash: 850));
            Assert.DoesNotContain("accept_wholesale", Ids(c, items, cash: 849));
            var label = BarterRules.Options(new StoreInfo { Cash = 900 }, c, items, new BargainFlags(), Ready(), FlatOdds).Single(o => o.Id == "accept_wholesale").Label;
            Assert.Equal("Accept wholesale: all 1 item for 850 (-15%)", label);
        }

        [Fact]
        public void SellOverBudgetStatesTheCappedAmount()
        {
            var opts = BarterRules.Options(new StoreInfo { Cash = 0 }, Client(), new List<ItemInfo> { Own(1, 400), Own(2, 300) }, new BargainFlags(), Ready(), FlatOdds);
            Assert.Equal("Sell 2 items for 700 (over their budget: they pay 500)", opts.Single(o => o.Id == "sell_all").Label);
        }

        [Theory]
        [InlineData(null, false, "s")]   // their item on the back: the tablet shows buying
        [InlineData(1, true, "b")]       // negotiating one of ours the client wants
        public void SellAndBuyModeFollowsTheTablet(int? negotiatedOwn, bool wants, string expected)
        {
            var c = Client(ClientIntent.SellAndBuy);
            var items = new List<ItemInfo> { Theirs(9, 100) };
            if (negotiatedOwn.HasValue) items.Add(new ItemInfo { Uid = 1, Owned = true, OnBack = true, ClientWants = wants, Negotiated = true });
            var mode = BarterRules.ResolveMode(c, items);
            Assert.Equal(expected, mode == TabletMode.ClientBuys ? "b" : "s");
            var ids = Ids(c, items);
            Assert.Equal(expected == "b", ids.Contains("sell_all"));
            Assert.Equal(expected == "s", ids.Contains("buy_all"));
            Assert.Contains("add_markup_10", ids);
            Assert.Contains("demand_discount_10", ids);
        }

        [Theory]
        [InlineData(BargainKind.Markup, 10, 82, 10, "Add 10% markup (accept 82%, refuse 8%, walk out 10% (-3 rep))")]
        [InlineData(BargainKind.Markup, 10, 82, 0, "Add 10% markup (accept 82%, else refused and they stay)")]
        [InlineData(BargainKind.BuyingDiscount, 50, 2, 38, "Demand 50% discount (accept 2%, refuse 60%, walk out 38% (-3 rep))")]
        [InlineData(BargainKind.BuyingMarkup, 25, 100, 0, "Offer 25% markup (always accepted)")]
        [InlineData(BargainKind.Discount, 10, 100, 0, "Offer 10% discount (always accepted; raises their budget 20%)")]
        [InlineData(BargainKind.Blackmail, 20, 63, 0, "Blackmail for 20% (accept 63% if they are a criminal, else -5 rep; if innocent: -7 rep, 50% they walk out -10 rep)")]
        public void LabelsCarryTheGameOdds(BargainKind kind, int pct, int success, int leave, string expected) =>
            Assert.Equal(expected, BarterRules.BargainLabel(kind, pct, new Odds { Success = success, Leave = leave }, Client()));
    }

    public class AdviceTrackerTests
    {
        static AdviceTracker Fresh()
        {
            var t = new AdviceTracker();
            t.NewClient(new object());
            return t;
        }

        [Fact]
        public void WaitsForAStableSignature()
        {
            var t = Fresh();
            Assert.Null(t.Observe("a", 0, true));
            Assert.Null(t.Observe("a", 700, true));
            Assert.Null(t.Observe("b", 800, true));
            Assert.Null(t.Observe("b", 1500, true));
            Assert.Equal("b", t.Observe("b", 1550, true));
        }

        [Fact]
        public void AsksOncePerSignatureAndNeverWhileBlocked()
        {
            var t = Fresh();
            t.Observe("a", 0, true);
            Assert.Null(t.Observe("a", 1000, false));
            Assert.Equal("a", t.Observe("a", 1000, true));
            t.Sent("a", "d1");
            t.Answered(new Advice { Signature = "a", DecisionId = "d1", OptionId = "sell_all" });
            Assert.Null(t.Observe("a", 5000, true));
            Assert.Null(t.Observe(null, 5000, true));
        }

        [Fact]
        public void OneInFlightAndTheNewestSignatureWins()
        {
            var t = Fresh();
            t.Observe("a", 0, true);
            Assert.Equal("a", t.Observe("a", 800, true));
            t.Sent("a", "d1");
            t.Observe("b", 900, true);
            t.Observe("c", 1000, true);
            Assert.Null(t.Observe("c", 3000, true));
            Assert.Equal(AdviceStatus.Thinking, t.View(true).Status);
            t.Answered(new Advice { Signature = "a", DecisionId = "d1", OptionId = "dismiss", Label = "Send the customer away" });
            Assert.Equal(AdviceStatus.Thinking, t.View(true).Status);
            Assert.Equal(AdviceStatus.Stale, t.View(false).Status);
            Assert.Equal("c", t.Observe("c", 3100, true));
            t.Sent("c", "d2");
            Assert.Equal("d2", t.LastDecisionId);
            t.Answered(new Advice { Signature = "c", DecisionId = "d2", OptionId = "sell_all" });
            var v = t.View(true);
            Assert.Equal(AdviceStatus.Ready, v.Status);
            Assert.Equal("sell_all", v.Advice.OptionId);
        }

        [Fact]
        public void DeferShowsNothingAndANewClientForgetsEverything()
        {
            var t = Fresh();
            t.Observe("a", 0, true);
            t.Sent(t.Observe("a", 800, true), "d1");
            t.Answered(new Advice { Signature = "a", DecisionId = "d1", Deferred = true });
            Assert.Equal(AdviceStatus.None, t.View(true).Status);
            int seq = t.ClientSeq;
            t.NewClient(new object());
            Assert.Equal(seq + 1, t.ClientSeq);
            Assert.Null(t.LastDecisionId);
            Assert.Equal(AdviceStatus.None, t.View(false).Status);
        }
    }

    public class OutcomeTests
    {
        static List<FactionRep> Reps(double ll) => new List<FactionRep>
        {
            new FactionRep { FactionId = "FACTION_LOWER_LEVEL", Value = ll },
            new FactionRep { FactionId = "FACTION_SECURITY", Value = 1 },
        };

        [Fact]
        public void MarkupWalkOut()
        {
            var after = new BargainFlags { MarkupOffered = true };
            var r = Outcomes.Bargain(BargainKind.Markup, 25, new BargainFlags(), after, false, ClientIntent.Buy, ClientIntent.Dialogue,
                Outcomes.RepDelta(Reps(10), Reps(7)));
            Assert.True(JToken.DeepEquals(JObject.Parse(
                "{\"kind\":\"bargain\",\"option\":\"add_markup_25\",\"accepted\":false,\"walked_out\":true,\"rep_delta\":{\"lower_levels\":-3.0}}"), r));
        }

        [Fact]
        public void AlwaysAcceptedKindsAndUnknownRolls()
        {
            var r = Outcomes.Bargain(BargainKind.Discount, 10, new BargainFlags(), new BargainFlags { DiscountOffered = true }, null, ClientIntent.Buy, ClientIntent.Buy, new JObject());
            Assert.True((bool)r["accepted"]);
            var b = Outcomes.Bargain(BargainKind.Blackmail, 20, new BargainFlags(), new BargainFlags { BlackmailOffered = true }, null, ClientIntent.Sell, ClientIntent.Sell, new JObject());
            Assert.Equal("blackmail", (string)b["option"]);
            Assert.Null(b["accepted"]);
        }

        [Fact]
        public void RefusedCallIsNoOutcome() =>
            Assert.Null(Outcomes.Bargain(BargainKind.Markup, 10, new BargainFlags { MarkupOffered = true }, new BargainFlags { MarkupOffered = true }, true, ClientIntent.Buy, ClientIntent.Buy, null));

        [Fact]
        public void TradeBatchSumsSignedCash()
        {
            var b = new TradeBatch();
            Assert.True(b.NeedsSnapshot(TradeBatch.Bought));
            b.Begin(TradeBatch.Bought, Reps(0));
            b.Add(5, 100);
            Assert.False(b.NeedsSnapshot(TradeBatch.Bought));
            Assert.True(b.NeedsSnapshot(TradeBatch.Sold));
            b.Add(6, 40);
            var r = b.Flush(Reps(0.35));
            Assert.True(JToken.DeepEquals(JObject.Parse(
                "{\"kind\":\"trade\",\"side\":\"bought\",\"items\":[5,6],\"cash_delta\":-140,\"rep_delta\":{\"lower_levels\":0.35},\"actor\":\"human\"}"), r));
            Assert.True(b.IsEmpty);
            Assert.Null(b.Flush(null));

            b.Begin(TradeBatch.Sold, Reps(0));
            b.Add(7, 90);
            Assert.Equal(90, (long)b.Flush(Reps(0))!["cash_delta"]!);
        }

        [Theory]
        [InlineData(true, true, true, "walked_out")]
        [InlineData(false, true, true, "shutter")]
        [InlineData(false, false, true, "dismissed")]
        [InlineData(false, false, false, "business_done")]
        public void LeftCause(bool walked, bool shutter, bool player, string cause) =>
            Assert.Equal(cause, Outcomes.LeftCause(walked, shutter, player));
    }
}
