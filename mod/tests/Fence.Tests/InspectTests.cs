using System;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using Fence.Barter;
using Fence.Inspect;
using Fence.Protocol;
using Newtonsoft.Json.Linq;
using Xunit;

namespace Fence.Tests
{
    // A GameItem as the inspection views see it: tags plus the authenticity feature's conditions.
    public sealed class FakeInspectItem : IInspectItem
    {
        public readonly HashSet<string> Tags = new HashSet<string>();
        public readonly Dictionary<string, int> Ints = new Dictionary<string, int>();
        public readonly Dictionary<string, string> Strings = new Dictionary<string, string>();
        public string Shown, Real;

        public FakeInspectItem(params string[] tags)
        {
            foreach (var t in tags) Tags.Add(t);
        }

        public FakeInspectItem With(string tag, int v)
        {
            Ints[tag] = v;
            return this;
        }

        public FakeInspectItem With(string tag, string v)
        {
            Strings[tag] = v;
            return this;
        }

        public bool IsTag(string tag) => Tags.Contains(tag);
        public int Int(string tag) => Ints.TryGetValue(tag, out var v) ? v : 0;
        public string Str(string tag) => Strings.TryGetValue(tag, out var v) ? v : "";
        public string ShownCondition(string category) => Shown;
        public string RealCondition(string category) => Real;
    }

    public class InspectTests
    {
        // The injector behind protocol/fixtures/decide.inspect.request.json: a recalled serial, claimed genuine.
        static FakeInspectItem RecalledInjector() => new FakeInspectItem("INS_INJECTOR_TAG", "INSPECTABLE_TAG", "ISSUE_SERIAL")
            .With("INJECTOR_SERIAL", "2514471").With("INJECTOR_COMPANY", "Carmicheal Pharmaceutical").With("INJECTOR_MODEL", "Immunivax")
            .With("INJECTOR_INDICATOR", 0).With("INJECTOR_BODY", 0).With("INJECTOR_COLOR", -1)
            .Shown_("genuineInjector", "unusableInjector");

        static InspectSituation Counter(int uid = 4120, string id = "large_purple_injector", string name = "Immunivax", long? price = 160) => new InspectSituation
        {
            ClientSeq = 3,
            Uid = uid,
            ItemId = id,
            Name = name,
            Price = price,
            Client = new ClientInfo { Name = "Clinic Runner", FactionId = "FACTION_LOWER_LEVEL", Intent = ClientIntent.Sell },
            Reputations = new List<FactionRep>
            {
                new FactionRep { FactionId = "FACTION_LOWER_LEVEL", Value = 12.5, Status = "Known" },
                new FactionRep { FactionId = "FACTION_UPPER_LEVEL", Value = -4, Status = "Neutral" },
            },
            ConfrontPanelOpen = true,
            GameDay = 71,
        };

        [Fact]
        public void MatchesTheFixture()
        {
            var q = InspectContext.Build(RecalledInjector(), Counter());
            Assert.NotNull(q);
            var fixture = JObject.Parse(File.ReadAllText(Path.Combine(Paths.Fixtures, "decide.inspect.request.json")));
            Assert.True(JToken.DeepEquals(fixture["context"], q.Context), "context:\n" + q.Context);
            Assert.True(JToken.DeepEquals(fixture["hidden"], q.Hidden), "hidden:\n" + q.Hidden);
            Assert.Equal(fixture["options"]!.Select(o => ((string)o["id"], (string)o["label"])), q.Options.Select(o => (o.Id, o.Label)));
            Assert.Equal((string)fixture["state_signature"], q.Signature);

            var wire = JObject.Parse(ProtocolJson.Serialize(InspectContext.Request(q, "i-4b1c9e02-3", 20000)));
            Assert.Equal("inspect", (string)wire["kind"]);
            Assert.Equal(71, (int)wire["game_day"]);
        }

        // The information boundary: no truth flag, issue tag, condition id or raw sprite index in context.
        [Fact]
        public void ContextCarriesNoTruth()
        {
            foreach (var (item, s) in new[] { (RecalledInjector(), Counter()), (Ribwich(), Appraisal()), (Stamp(170, 0), Counter(id: "food_stamp")) })
            {
                var q = InspectContext.Build(item, s);
                string ctx = q.Context.ToString();
                foreach (var bad in new[] { "ISSUE", "unusable", "counterfeitInjector", "genuineInjector", "_TAG", "true_value", "realCondition", "1080" })
                    Assert.DoesNotContain(bad, ctx);
                foreach (var v in q.Context["visual"]!.Children<JProperty>())
                    Assert.False(int.TryParse((string)v.Value, out _), v.Name + " is a raw index: " + v.Value);
            }
        }

        static FakeInspectItem Stamp(int amount, int seal, int logo = 0, int lettering = 0, int chip = 0) =>
            new FakeInspectItem("FOOD_STAMP_TAG").With("STAMP_AMOUNT", amount).With("STAMP_SEAL", seal).With("STAMP_LOGO", logo)
                .With("STAMP_LETTERING", lettering).With("STAMP_CHIP", chip).Shown_("genuineStamp", "counterfeitStamp");

        static FakeInspectItem Cigarettes(int seal, int lettering, int logo, int box, string claim = "genuineCigarette") =>
            new FakeInspectItem("CIGARETTE_TAG").With("CIGARETTE_SEAL", seal).With("CIGARETTE_LETTERING", lettering).With("CIGARETTE_LOGO", logo)
                .With("CIGARETTE_BOX", box).Shown_(claim, "counterfeitCigarette");

        static FakeInspectItem Ribwich() => new FakeInspectItem("RIBWICH_TAG", "APPRAISAL_ITEM")
            .With("RIBWICH_MEAT_TAG", 1).With("RIBWICH_BUN_TAG", 0).With("RIBWICH_PICKLES_TAG", 0).With("RIBWICH_WRAP_TAG", 0)
            .With("RIBWICH_DAVRE_LOGO_TAG", 0).With("RIBWICH_RIBWICH_LOGO_TAG", -1).With("RIBWICH_FACILITY_NUMBER_TAG", 4)
            .With("RIBWICH_PRODUCTION_MONTH_TAG", 6).With("RIBWICH_PRODUCTION_YEAR_TAG", 612).With("RIBWICH_PRODUCTION_TAG", 0).With("RIBWICH_FACILITY_TAG", 0);

        static InspectSituation Appraisal(AppraisalTerms terms = null)
        {
            var s = Counter(uid: 900, id: "ribwich", name: "Ribwich", price: null);
            s.ConfrontPanelOpen = false;
            s.AppraisalService = true;
            s.Client = new ClientInfo { Name = "Collector", FactionId = "FACTION_UPPER_LEVEL", Intent = ClientIntent.Other, RawIntent = "APPRAISAL_SERVICE" };
            s.Appraisal = terms ?? new AppraisalTerms
            {
                TheirFigure = 300, Rank = 1, BaseFee = 40,
                FeePct = new Dictionary<int, int> { [1] = 18, [2] = 15, [3] = 12, [4] = 10, [5] = 0 },
            };
            s.TrueValue = 1080;
            return s;
        }

        public static IEnumerable<object[]> CueCases() => new[]
        {
            new object[] { Cigarettes(0, 0, 0, 0), "seal=matches reference;lettering=matches reference;logo=matches reference;box_colour=matches colour card" },
            new object[] { Cigarettes(-1, 3, 4, 2), "seal=missing;lettering=misprinted;logo=wrong logo;box_colour=off-shade" },
            new object[] { Cigarettes(1, -1, -1, 0), "seal=broken;lettering=missing;logo=missing;box_colour=matches colour card" },
            new object[] { Stamp(120, -1), "amount=120 credits;seal=missing;lettering=matches reference;logo=matches reference;chip=matches reference" },
            new object[] { Stamp(170, 0, logo: -1, chip: 2), "amount=170 credits;seal=present;logo=missing;chip=wrong chip" },
            new object[] { RecalledInjector(), "serial=2514471;company=Carmicheal Pharmaceutical;model=Immunivax;indicator=normal;body=intact;body_colour=standard" },
            new object[] { RecalledInjector().With("INJECTOR_INDICATOR", 1).With("INJECTOR_BODY", 2), "indicator=expired warning;body=cracked" },
            new object[] { RecalledInjector().With("INJECTOR_INDICATOR", 3).With("INJECTOR_BODY", -1).With("INJECTOR_COLOR", 1), "indicator=fault warning;body=intact;body_colour=off-colour" },
            new object[] { Ribwich(), "production_date=6/ 612;facility=#4;meat=double;top_bun=hidden under double meat;pickles=hidden under double meat;wrap=sealed;ribwich_logo=absent;barcode_stamp=present" },
            new object[] { Ribwich().With("RIBWICH_MEAT_TAG", 0).With("RIBWICH_BUN_TAG", 1).With("RIBWICH_PICKLES_TAG", 1).With("RIBWICH_WRAP_TAG", 2).With("RIBWICH_DAVRE_LOGO_TAG", 4),
                "top_bun=deluxe;pickles=extra;wrap=opened;davre_logo=wrong logo" },
            new object[] { Ribwich().With("RIBWICH_PRODUCTION_TAG", -1).With("RIBWICH_FACILITY_TAG", -1), "barcode_stamp=missing;facility_stamp=missing;production_date=;facility=" },
            new object[] { Hand("GOLD_PLATED", "NEURAL", "SYMBOL", "NONE", "ARMORED", "NONE"),
                "material=gold plated;interface_marking=Neural;marking_style=symbol;condition=pristine;armor_plating=fitted;blade=none" },
            new object[] { Hand("ALUMINUM_ALLOY", "UNSPECIFIED", "FULL", "DAMAGE", "NONE", "BLADED"),
                "material=aluminium alloy;interface_marking=none;marking_style=;condition=damaged;blade=fitted" },
            new object[] { Plasma("POOR2", "IGNITION_LAB", "WEAR_HEAVY", "CRACK", "UNCOVERED", "REINFORCED"),
                "colour=poor;seal_brand=Ignition Lab;casing_wear=heavily worn;crack=cracked;screw=uncovered;reinforcement=reinforced" },
            new object[] { Plasma("", "NONE", "", "NONE", "NONE", "NONE"), "colour=;seal_brand=none;casing_wear=none;crack=none;screw=covered;reinforcement=no" },
        };

        static FakeInspectItem Hand(string material, string iface, string marking, string condition, string armored, string bladed) =>
            new FakeInspectItem("HAND_TAG", "APPRAISAL_ITEM").With("HAND_MATERIAL", material).With("HAND_INTERFACE", iface).With("HAND_INTERFACE_MARKING", marking)
                .With("HAND_CONDITION", condition).With("HAND_ADDON_ARMORED", armored).With("HAND_ADDON_BLADED", bladed);

        static FakeInspectItem Plasma(string colour, string seal, string wear, string crack, string screw, string reinforced) =>
            new FakeInspectItem("PLASMA_CELL_TAG", "APPRAISAL_ITEM").With("PLASMA_COLOR", colour).With("PLASMA_SEAL", seal).With("PLASMA_CASING_CONDITION_WEAR", wear)
                .With("PLASMA_CASING_CONDITION_CRACK", crack).With("PLASMA_CASING_CONDITION_UNCOVERED", screw).With("PLASMA_CASING_CONDITION_REINFORCED", reinforced);

        // "cue=word;..." where an empty word means the cue is not shown at all.
        [Theory]
        [MemberData(nameof(CueCases))]
        public void CueDictionary(FakeInspectItem item, string expected)
        {
            var cues = CueWords.Of(ItemKinds.Of(item), item);
            foreach (var pair in expected.Split(';'))
            {
                var kv = pair.Split(new[] { '=' }, 2);
                bool text = cues.Text.TryGetValue(kv[0], out var t);
                bool visual = cues.Visual.TryGetValue(kv[0], out var v);
                if (kv[1].Length == 0) Assert.False(text || visual, kv[0] + " should not be shown");
                else Assert.Equal(kv[1], text ? t : visual ? v : null);
            }
        }

        static JObject Vocabulary() => JObject.Parse(File.ReadAllText(Path.Combine(Paths.Repo, "internal", "policy", "testdata", "inspect_cues.json")));

        // Every value each tag can take, per kind (docs/re/INSPECT.md): the generators' ranges plus the
        // not-drawn and unexpected values.
        static IEnumerable<FakeInspectItem> Sweep(string kind)
        {
            var rnd = new Random(7);
            int[] sprite = { -1, 0, 1, 2, 3, 4, 5, 6, 7, 8 };
            T Pick<T>(T[] a) => a[rnd.Next(a.Length)];
            for (int n = 0; n < 3000; n++)
            {
                switch (kind)
                {
                    case ItemKinds.Cigarettes:
                        yield return Cigarettes(Pick(sprite), Pick(sprite), Pick(sprite), Pick(sprite));
                        break;
                    case ItemKinds.FoodStamp:
                        yield return Stamp(Pick(new[] { 40, 50, 80, 90, 120, 130, 160, 170, 200, 210 }), Pick(new[] { -1, 0, 1 }), Pick(sprite), Pick(sprite), Pick(sprite));
                        break;
                    case ItemKinds.Injector:
                        yield return RecalledInjector().With("INJECTOR_INDICATOR", Pick(new[] { -1, 0, 1, 2, 3 })).With("INJECTOR_BODY", Pick(new[] { -1, 0, 1, 2, 3 }))
                            .With("INJECTOR_COLOR", Pick(new[] { -1, 0, 1 }));
                        break;
                    case ItemKinds.Ribwich:
                        yield return Ribwich().With("RIBWICH_MEAT_TAG", Pick(new[] { 0, 1 })).With("RIBWICH_BUN_TAG", Pick(new[] { 0, 1 })).With("RIBWICH_PICKLES_TAG", Pick(new[] { 0, 1 }))
                            .With("RIBWICH_WRAP_TAG", Pick(new[] { 0, 1, 2 })).With("RIBWICH_DAVRE_LOGO_TAG", Pick(sprite)).With("RIBWICH_RIBWICH_LOGO_TAG", Pick(new[] { -1, 0, 1, 2, 3 }))
                            .With("RIBWICH_PRODUCTION_TAG", Pick(new[] { -1, 0 })).With("RIBWICH_FACILITY_TAG", Pick(new[] { -1, 0 }));
                        break;
                    case ItemKinds.Hand:
                        yield return Hand(Pick(new[] { "GOLD_PLATED", "CARBON_COMPOSITE", "ALUMINUM_ALLOY" }), Pick(new[] { "EMG", "EMG_PLUS", "NEURAL", "UNSPECIFIED" }),
                            Pick(new[] { "FULL", "SYMBOL" }), Pick(new[] { "NONE", "WEAR", "DAMAGE" }), Pick(new[] { "NONE", "ARMORED" }), Pick(new[] { "NONE", "BLADED" }));
                        break;
                    case ItemKinds.Plasma:
                        yield return Plasma(Pick(new[] { "STANDARD", "HIGH_QUALITY", "POOR1", "POOR2" }), Pick(new[] { "NONE", "LL", "ZHONGHUA", "REPUBLIC", "IGNITION_LAB" }),
                            Pick(new[] { "NONE", "", "WEAR", "WEAR_HEAVY" }), Pick(new[] { "NONE", "CRACK" }), Pick(new[] { "NONE", "UNCOVERED" }), Pick(new[] { "NONE", "REINFORCED" }));
                        break;
                }
            }
        }

        // The Go rule checker reads the same word list (internal/policy TestInspectCueVocabulary): the
        // dictionary may only say words it knows, and every listed word must be reachable.
        [Fact]
        public void CueWordsMatchTheSharedVocabulary()
        {
            var vocab = Vocabulary();
            var kinds = new[] { ItemKinds.Cigarettes, ItemKinds.FoodStamp, ItemKinds.Injector, ItemKinds.Ribwich, ItemKinds.Hand, ItemKinds.Plasma };
            Assert.Equal(kinds.OrderBy(k => k), vocab.Properties().Select(p => p.Name).OrderBy(k => k));
            foreach (var kind in kinds)
            {
                var seen = new Dictionary<string, HashSet<string>>();
                var texts = new HashSet<string>();
                foreach (var item in Sweep(kind))
                {
                    Assert.Equal(kind, ItemKinds.Of(item));
                    var cues = CueWords.Of(kind, item);
                    foreach (var t in cues.Text.Keys) texts.Add(t);
                    foreach (var kv in cues.Visual)
                    {
                        var allowed = vocab[kind]!["visual"]![kv.Key] as JArray;
                        Assert.True(allowed != null, kind + "." + kv.Key + " is not in the shared vocabulary");
                        Assert.True(allowed.Any(w => (string)w == kv.Value), kind + "." + kv.Key + " = \"" + kv.Value + "\" is not in the shared vocabulary");
                        if (!seen.TryGetValue(kv.Key, out var set)) seen[kv.Key] = set = new HashSet<string>();
                        set.Add(kv.Value);
                    }
                }
                foreach (var cue in ((JObject)vocab[kind]!["visual"]!).Properties())
                    foreach (var w in cue.Value.Select(x => (string)x))
                        Assert.True(seen.TryGetValue(cue.Name, out var s) && s.Contains(w), kind + "." + cue.Name + " = \"" + w + "\" is never produced");
                Assert.Equal(vocab[kind]!["text"]!.Select(x => (string)x).OrderBy(x => x), texts.OrderBy(x => x));
            }
        }

        [Fact]
        public void NoQuestionWhenTheGameOffersNoChoice()
        {
            var owned = Counter();
            owned.Owned = true;
            Assert.Null(InspectContext.Build(RecalledInjector(), owned));
            var closed = Counter();
            closed.ConfrontPanelOpen = false;
            Assert.Null(InspectContext.Build(RecalledInjector(), closed));
            Assert.Null(InspectContext.Build(RecalledInjector().Shown_("expiredInjector", "expiredInjector"), Counter()));
            var seller = Appraisal();
            seller.AppraisalService = false;
            Assert.Null(InspectContext.Build(Ribwich(), seller));
            Assert.Null(InspectContext.Build(new FakeInspectItem("BERRY_TAG", "INSPECTABLE_TAG"), Counter()));
        }

        [Fact]
        public void ClaimedCounterfeitOffersTheTalk()
        {
            var q = InspectContext.Build(Cigarettes(0, 0, 0, 0, "counterfeitCigarette"), Counter(id: "packet_red_cigarette", name: "Red cigarettes", price: 50));
            Assert.Equal("counterfeit", (string)q.Context["claim"]);
            Assert.Equal(new[] { InspectOptionIds.Genuine, InspectOptionIds.Counterfeit }, q.Options.Select(o => o.Id));
            Assert.Contains("+2 rep", q.Options[0].Label);
            Assert.Equal("Agree it is counterfeit: price stays 50", q.Options[1].Label);
        }

        [Fact]
        public void PenaltyFallsOnLowerLevelsForFactionsWithoutABar()
        {
            var s = Counter();
            s.Client = new ClientInfo { Name = "Tourist", FactionId = "FACTION_TOURIST", Intent = ClientIntent.Sell };
            var q = InspectContext.Build(RecalledInjector(), s);
            Assert.Equal("lower_levels", (string)q.Context["reputation"]!["faction"]);
            Assert.Equal(12.5, (double)q.Context["reputation"]!["value"]);
            Assert.Contains("-2 rep with lower_levels", q.Options[0].Label);
        }

        [Fact]
        public void AppraisalAsksForAnEstimate()
        {
            var q = InspectContext.Build(Ribwich(), Appraisal());
            Assert.Equal("appraisal", q.Mode);
            var o = Assert.Single(q.Options);
            Assert.Equal(InspectOptionIds.Estimate, o.Id);
            var p = o.Params[InspectOptionIds.EstimateParam];
            Assert.Equal((ParamSpec.Int, 1L, 5000L), (p.Type, p.Min.Value, p.Max.Value));
            var a = (JObject)q.Context["appraisal"];
            Assert.Equal(300, (int)a["their_figure"]);
            Assert.Equal("appraiser", (string)a["rank"]);
            Assert.Equal(0, (int)a["fee_pct"]!["awful"]);
            Assert.Equal(1080, (int)q.Hidden["true_value"]);
            Assert.Null(q.Context["claim"]);
            Assert.Null(q.Context["reputation"]);
        }

        [Fact]
        public void EachAppraisalAnswerIsANewQuestion()
        {
            var t = new InspectTracker();
            t.NewClient("c");
            var s = Appraisal();
            var q1 = InspectContext.Build(Ribwich(), s);
            Assert.True(t.Show(q1));
            t.Sent(q1, "i-1");
            Assert.True(t.Thinking);
            Assert.False(t.Show(InspectContext.Build(Ribwich(), s)));

            t.Estimated(900, 1200);
            Assert.Equal(1200, t.Feedback(null, AppraisalFeedback.TooHigh, out int uid));
            Assert.Equal(900, uid);
            Assert.Null(t.Feedback(null, AppraisalFeedback.TooHigh, out _));
            s.Appraisal = t.Terms(900, Appraisal().Appraisal);
            Assert.Equal((1, 1200, "too_high"), (s.Appraisal.Refusals, s.Appraisal.LastEstimate.Value, s.Appraisal.LastFeedback));
            var q2 = InspectContext.Build(Ribwich(), s);
            Assert.NotEqual(q1.Signature, q2.Signature);
            Assert.Equal("too_high", (string)q2.Context["appraisal"]!["last_feedback"]);
            Assert.True(t.Show(q2));
        }

        [Fact]
        public void MovesAttachToTheInspectDecision()
        {
            var t = new InspectTracker();
            t.NewClient("c");
            Assert.Null(t.InForce(3));
            var q = InspectContext.Build(RecalledInjector(), Counter());
            t.Show(q);
            t.Sent(q, "i-7");
            var f = t.InForce(3);
            Assert.Equal(("i-7", (string)null), (f.DecisionId, f.Advised));
            t.Answered(q.Signature, new Advice { OptionId = "confront_serial" });
            Assert.Equal("confront_serial", t.InForce(3).Advised);
            Assert.Equal("i-7", t.DecisionFor("large_purple_injector", out _));
            Assert.Null(t.DecisionFor("food_stamp", out _));
            t.Verdict("large_purple_injector", "serial");
            Assert.Equal("serial", t.VerdictFor("large_purple_injector"));
            t.NewClient("d");
            Assert.Null(t.InForce(4));
            Assert.Null(t.VerdictFor("large_purple_injector"));
        }

        [Theory]
        [InlineData(new[] { false, false, true, false, false }, "serial")]
        [InlineData(new[] { true, false, false, true, false }, "body")]
        [InlineData(new[] { false, false, false, false, false }, null)]
        public void LastCheckedToggleIsTheVerdict(bool[] toggles, string want) =>
            Assert.Equal(want, ConfrontIssues.Chosen(ItemKinds.Injector, toggles));

        [Fact]
        public void OverlayLine()
        {
            var q = InspectContext.Build(RecalledInjector(), Counter());
            Assert.Equal("inspect: thinking…", InspectText.Line(q, null, true));
            Assert.Equal("inspect: deferred to you", InspectText.Line(q, new Advice { Deferred = true }, false));
            Assert.Equal("inspect: confront serial — Looks unusable: serial 2514471: prefix 251 is recalled.",
                InspectText.Line(q, new Advice { OptionId = "confront_serial", Rationale = "Looks unusable: serial 2514471: prefix 251 is recalled." }, false));
            var a = InspectContext.Build(Ribwich(), Appraisal());
            Assert.StartsWith("inspect: estimate 1134", InspectText.Line(a, new Advice { OptionId = "estimate", Params = new JObject { ["value"] = 1134 }, Rationale = "x" }, false));
            Assert.Equal("", InspectText.Line(null, null, false));
        }

        [Fact]
        public void OutcomesDescribeTheResult()
        {
            var c = InspectOutcomes.Confront("serial", "confront", "success");
            Outcomes.WithActor(c, Actors.Human);
            Assert.Equal("confronted serial: right", OverlayText.Describe(c));
            Assert.Equal("told them it is genuine: wrong, -2 rep", OverlayText.Describe(InspectOutcomes.Confront("genuine", "talk", "fail")));
            var a = InspectOutcomes.Appraisal(1134, AppraisalFeedback.Accepted, 1080);
            Assert.Equal(1080, (int)a["true_value"]);
            Assert.Null(InspectOutcomes.Appraisal(1500, AppraisalFeedback.TooHigh, 1080)["true_value"]);
            Assert.Equal("estimate 1500: too high", OverlayText.Describe(InspectOutcomes.Appraisal(1500, AppraisalFeedback.TooHigh, null)));
        }

        [Fact]
        public void OverlayShowsTheInspectLine()
        {
            var v = OverlayText.Build(new OverlayInput { Client = new ClientInfo { Name = "x" }, Inspect = "inspect: confront serial — <b>x</b>" });
            Assert.Equal("inspect: confront serial — ‹b›x‹/b›", v.Inspect);
            Assert.Contains(v.Inspect, v.Bar);
            Assert.Equal("", OverlayText.Build(new OverlayInput { Inspect = "inspect: x" }).Inspect);
        }
    }

    static class FakeInspectItemExtensions
    {
        public static FakeInspectItem Shown_(this FakeInspectItem i, string shown, string real)
        {
            i.Shown = shown;
            i.Real = real;
            return i;
        }
    }
}
