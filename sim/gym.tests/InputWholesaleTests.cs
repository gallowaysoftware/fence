using System;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using Newtonsoft.Json.Linq;
using Xunit;

namespace Fence.Gym.Tests
{
    // Item 4: staged inputs bought singly when the seller offers the whole lot at wholesale (~85 % of the
    // estimate) is more expensive than the lot. The inputs rule (PlayerPolicy.Buy) now takes the lot only
    // when the wholesale price passes the same check the generic path uses (wholesale_max_pct, default 85
    // on normal): above it the staged singles at up to 1.3 x estimate are cheaper, so the singles win.
    [Collection("gym")]
    public sealed class InputWholesaleTests
    {
        static readonly object BootLock = new object();

        static string Dir(string name)
        {
            lock (BootLock) Boot.Init();
            string d = Boot.Var("sim", "test", name);
            if (Directory.Exists(d)) Directory.Delete(d, true);
            Directory.CreateDirectory(d);
            return d + Path.DirectorySeparatorChar;
        }

        // Boots one episode (new game + first morning) so the stock reads work, the furnace engine on.
        // The first back-counter pick of the run may be staged; it is cleared (via reflection: the game
        // types are not referenced by the test assembly) so only the request's own items are on the counter.
        static void Setup(string d)
        {
            lock (BootLock) Boot.Init();
            // PlayerDay.Current carries the boot's real-date rent floor (Rent2Floor), which flips the
            // buy rows' AffordInput by calendar day; the request pins its own cash, so pin the day too.
            File.WriteAllText(d + "directives.json", "{\"engine\":\"furnace\"}");
            var ep = new Episode(new GymOptions { Seed = 1, Human = "player", SkipIntro = true, DirectivesFile = d + "directives.json" }, new LocalBridge(1), null);
            ep.BaseDirectives = Directives.Load(d + "directives.json");
            ep.Step();
            PlayerDay.Current = new PlayerDay(); // after Step: the boot leaves the real-date rent floor in it
            var entryType = AppDomain.CurrentDomain.GetAssemblies().Select(a => { try { return a.GetType("EmporiumEntry"); } catch { return null; } }).FirstOrDefault(t => t != null);
            var back = entryType.GetProperty("Instance", System.Reflection.BindingFlags.Static | System.Reflection.BindingFlags.Public).GetValue(null);
            var inv = back.GetType().GetProperty("backInvinvElement", System.Reflection.BindingFlags.Instance | System.Reflection.BindingFlags.Public).GetValue(back);
            inv.GetType().GetMethod("ExpelAll").Invoke(inv, null);
            if (PlayerStock.BackUids().Any())
                throw new InvalidOperationException("back counter not clear after ExpelAll: " + string.Join(",", PlayerStock.BackUids()));
            // The stock grid the fit checks read must be empty too: whatever the boot left there varies
            // with the world's start state, and the request's lot is the only thing that should fit.
            var stock = back.GetType().GetProperty("invElement", System.Reflection.BindingFlags.Instance | System.Reflection.BindingFlags.Public).GetValue(back);
            stock.GetType().GetMethod("ExpelAll").Invoke(stock, null);
        }

        // A seller of six scraps (estimate 12 apiece, 72 for the lot) on the back counter, with the
        // seller's wholesale offer (when any) and the cash on hand.
        static Fence.Protocol.DecideRequest Req(long? wholesale, long cash)
        {
            var c = new JObject
            {
                ["client"] = new JObject
                {
                    ["intent"] = "sell",
                    ["faction"] = "lower_levels",
                    ["wholesale"] = wholesale is long w ? new JObject { ["price"] = w } : null,
                },
                ["counter"] = new JObject
                {
                    ["theirs"] = new JArray(Enumerable.Repeat(new JObject { ["id"] = "scrap_metal", ["uid"] = 1, ["price"] = 12, ["estimated_value"] = 12 }, 6).ToArray()),
                    ["yours_staged"] = new JArray(),
                },
                ["store"] = new JObject { ["cash"] = cash },
                ["reputation"] = new JObject { ["lower_levels"] = new JObject { ["value"] = 0 } },
            };
            return new Fence.Protocol.DecideRequest
            {
                Kind = "barter", Context = c,
                Options = new List<Fence.Protocol.DecisionOption> { new() { Id = "accept_wholesale" }, new() { Id = "buy_all" }, new() { Id = "dismiss" } },
            };
        }

        [Theory]
        [InlineData(60L, "accept_wholesale")]   // 60 <= 72 x 0.85: the lot at a wholesale price is cheaper than the singles.
        [InlineData(61L, "accept_wholesale")]   // 61 <= 61.2: still at a wholesale price.
        [InlineData(62L, "buy_all")]            // 62 > 61.2: over the wholesale cap the lot fails the check; the
                                                // staged singles (60 <= 72 x 1.3) are cheaper than the lot at 62.
        [InlineData(null, "buy_all")]           // No wholesale offer: the singles.
        public void InputsTakeTheWholesaleLotOnlyAtAWholesalePrice(long? wholesale, string want)
        {
            Setup(Dir("inputs_wholesale"));
            Assert.Equal(want, PlayerPolicy.Pick(Req(wholesale, 1000), out _));
        }
    }
}
