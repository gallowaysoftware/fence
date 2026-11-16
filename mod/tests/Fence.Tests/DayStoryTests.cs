using Fence.Barter;
using Newtonsoft.Json.Linq;
using Xunit;

namespace Fence.Tests
{
    public class DayStoryTests
    {
        static JObject Trade(string side, int items, long cash) =>
            new JObject { ["kind"] = "trade", ["side"] = side, ["items"] = new JArray(new int[items]), ["cash_delta"] = side == TradeBatch.Sold ? cash : -cash, ["actor"] = "mod" };

        static JObject Bargain(string option, bool? accepted, bool walked = false)
        {
            var o = new JObject { ["kind"] = "bargain", ["option"] = option, ["walked_out"] = walked, ["rep_delta"] = new JObject() };
            if (accepted.HasValue) o["accepted"] = accepted.Value;
            return o;
        }

        [Theory]
        [InlineData("add_markup_25", true, "paid a 25% markup")]
        [InlineData("add_markup_25", false, "refused a 25% markup")]
        [InlineData("add_markup_10", null, "was asked 10% more")]
        [InlineData("offer_discount_10", true, "got a 10% discount")]
        [InlineData("offer_markup_50", true, "was offered 50% more")]
        [InlineData("demand_discount_25", true, "came down 25%")]
        [InlineData("demand_discount_75", false, "would not come down 75%")]
        [InlineData("blackmail", true, "gave in to blackmail")]
        [InlineData("threaten", false, "stood up to a threat")]
        public void BargainClauses(string option, bool? accepted, string want) =>
            Assert.Equal(want, DayStory.Clause(Bargain(option, accepted)));

        [Fact]
        public void TradeAndLeftClauses()
        {
            Assert.Equal("bought 1 item for 90", DayStory.Clause(Trade(TradeBatch.Sold, 1, 90)));
            Assert.Equal("sold the shop 3 items for 240", DayStory.Clause(Trade(TradeBatch.Bought, 3, 240)));
            Assert.Equal("was sent away", DayStory.Clause(Outcomes.Left(LeftCauses.Dismissed)));
            Assert.Equal("was still waiting at closing", DayStory.Clause(Outcomes.Left(LeftCauses.Shutter)));
            Assert.Equal("refused a 25% markup and walked out", DayStory.Clause(Bargain("add_markup_25", false, walked: true)));
        }

        [Theory]
        [InlineData("open_shutter", null, "the window is set; open for business", "Opened the shop.")]
        [InlineData("chore", "Chem Finisher", "pressed Output", "Pressed Output on the Chem Finisher.")]
        [InlineData("chore", "Yeast", "pour", null)]
        [InlineData("dial", null, "Wild's Network: browse the network", "Called Wild's Network to browse the network.")]
        [InlineData("wild_buy", null, "LANDLORD_GRACE for 150 (150 in cash)", "Bought landlord grace on Wild's Network for 150.")]
        [InlineData("open_door", null, "going out to dumping_ground", "Went out to the dumping ground.")]
        [InlineData("return", null, "home from dr_jackson", "Home from Dr. Jackson's.")]
        [InlineData("services", null, "tonight: JANITORIAL_SERVICE; bill 3", "Services tonight: janitorial service.")]
        [InlineData("services", null, "tonight: none; bill 0", null)]
        [InlineData("shot_result", null, "Security's verdict on Thief: an aug, +157 cash", "Security's verdict on Thief: an aug, +157 cash.")]
        [InlineData("ring_bell", null, "2 customers waiting", null)]
        public void ShopBeats(string action, string target, string why, string want) =>
            Assert.Equal(want, DayStory.ShopBeat(action, target, why));

        [Fact]
        public void CustomerBeats()
        {
            Assert.Equal("was told \"All Good\"", DayStory.CustomerBeat("dialogue_choice", "[All Good]"));
            Assert.Equal("got their order: Revolver delivered for 300", DayStory.CustomerBeat("gun_order", "Revolver delivered for 300 (given 280 at estimates)"));
            Assert.Null(DayStory.CustomerBeat("counter", "sell all"));
        }

        [Fact]
        public void TellsTheDayInOrderAndFoldsBrowsers()
        {
            var day = new DayLog();
            day.SetDay(15);
            day.Beat(DayStory.ShopBeat("open_shutter", null, ""));
            day.Arrive("Retired farmer");
            day.Record(Trade(TradeBatch.Sold, 2, 680));
            day.Record(Outcomes.Left(LeftCauses.BusinessDone));
            day.Arrive("Scav");
            day.Record(Outcomes.Left(LeftCauses.BusinessDone));
            day.Arrive("Tourist");
            day.Arrive("Junkie");
            day.Record(Bargain("add_markup_25", false, walked: true));
            day.Record(Outcomes.Left(LeftCauses.WalkedOut));
            day.Arrive("Cartel Handler");
            day.Tell(DayStory.CustomerBeat("dialogue_choice", "[Yes]"));
            day.Record(Outcomes.Left(LeftCauses.BusinessDone));
            day.Beat(DayStory.ShopBeat("chore", "Chem Finisher", "pressed Output"));
            day.Beat(DayStory.ShopBeat("chore", "Chem Finisher", "pressed Output"));
            day.Arrive("Officer");

            Assert.Equal(
                "Opened the shop.\n" +
                "Retired farmer bought 2 items for 680.\n" +
                "2 customers browsed and left.\n" +
                "Junkie refused a 25% markup and walked out.\n" +
                "Cartel Handler was told \"Yes\".\n" +
                "Pressed Output on the Chem Finisher (×2).\n" +
                "Officer is at the counter.",
                DayStory.Tell(day.Entries, customerAtCounter: true));
            Assert.EndsWith("Pressed Output on the Chem Finisher (×2).\nOne customer browsed and left.", DayStory.Tell(day.Entries, customerAtCounter: false));
        }

        [Fact]
        public void OutcomesAfterABeatStayWithTheCustomer()
        {
            var day = new DayLog();
            day.Arrive("Scav");
            day.Beat("Security's verdict on Scav: an aug, +157 cash.");
            day.Note("estimate 120: too low");
            Assert.Equal(2, day.Entries.Count);
            Assert.Equal("Scav: estimate 120: too low.\nSecurity's verdict on Scav: an aug, +157 cash.", DayStory.Tell(day.Entries, customerAtCounter: false));
        }
    }
}
