using System;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using Fence.Protocol;
using Newtonsoft.Json.Linq;
using Xunit;

namespace Fence.Tests
{
    public class ProtocolFixtureTests
    {
        // Fixture file stem -> DTO. A fixture without a row here fails FixturesAreAllMapped.
        static readonly Dictionary<string, Type> Map = new Dictionary<string, Type>
        {
            ["health.response"] = typeof(HealthResponse),
            ["hello.request"] = typeof(HelloRequest),
            ["hello.response"] = typeof(HelloResponse),
            ["event.request"] = typeof(EventRequest),
            ["event.day_snapshot"] = typeof(EventRequest),
            ["event.customer_observed"] = typeof(EventRequest),
            ["event.human_action"] = typeof(EventRequest),
            ["event.autopilot"] = typeof(EventRequest),
            ["event.autopilot_fault"] = typeof(EventRequest),
            ["decide.request"] = typeof(DecideRequest),
            ["decide.response"] = typeof(DecideResponse),
            ["decide.stage.request"] = typeof(DecideRequest),
            ["decide.stage.response"] = typeof(DecideResponse),
            ["decide.inspect.request"] = typeof(DecideRequest),
            ["decide.inspect.response"] = typeof(DecideResponse),
            ["decide.aug_check.request"] = typeof(DecideRequest),
            ["decide.aug_check.response"] = typeof(DecideResponse),
            ["outcome.request"] = typeof(OutcomeRequest),
            ["summary.response"] = typeof(SummaryResponse),
            // The mod doesn't follow directives yet (the gym's executor does); round-trip them untyped.
            ["directives"] = typeof(Newtonsoft.Json.Linq.JObject),
        };

        static Type TypeFor(string file)
        {
            string stem = Path.GetFileNameWithoutExtension(file);
            // decide.response.defer -> decide.response
            var key = Map.Keys.Where(k => stem == k || stem.StartsWith(k + ".", StringComparison.Ordinal)).OrderByDescending(k => k.Length).FirstOrDefault();
            return key == null ? null : Map[key];
        }

        public static IEnumerable<object[]> Fixtures() =>
            Directory.GetFiles(Paths.Fixtures, "*.json").OrderBy(f => f, StringComparer.Ordinal).Select(f => new object[] { Path.GetFileName(f) });

        [Fact]
        public void FixturesAreAllMapped()
        {
            var unmapped = Directory.GetFiles(Paths.Fixtures, "*.json").Where(f => TypeFor(f) == null).Select(Path.GetFileName).ToList();
            Assert.True(unmapped.Count == 0, "fixtures without a DTO mapping: " + string.Join(", ", unmapped));
            Assert.NotEmpty(Directory.GetFiles(Paths.Fixtures, "*.json"));
        }

        [Theory]
        [MemberData(nameof(Fixtures))]
        public void RoundTripsSemantically(string fixture)
        {
            string json = File.ReadAllText(Path.Combine(Paths.Fixtures, fixture));
            var type = TypeFor(fixture);
            Assert.NotNull(type);

            object dto = typeof(ProtocolJson).GetMethod(nameof(ProtocolJson.Deserialize))!.MakeGenericMethod(type).Invoke(null, new object[] { json });
            Assert.NotNull(dto);
            string again = ProtocolJson.Serialize(dto);

            var expected = JToken.Parse(json);
            var actual = JToken.Parse(again);
            Assert.True(JToken.DeepEquals(expected, actual), fixture + " did not round-trip:\nexpected " + expected.ToString(Newtonsoft.Json.Formatting.None) + "\nactual   " + actual.ToString(Newtonsoft.Json.Formatting.None));
        }

        [Fact]
        public void DecideRequestKeepsHiddenOutOfContext()
        {
            var req = ProtocolJson.Deserialize<DecideRequest>(File.ReadAllText(Path.Combine(Paths.Fixtures, "decide.request.json")));
            Assert.NotNull(req.Hidden);
            Assert.NotNull(req.Context);
            var json = JObject.Parse(ProtocolJson.Serialize(req));
            Assert.NotNull(json["hidden"]);
            Assert.Null(json["context"]!["hidden"]);
        }
    }
}
