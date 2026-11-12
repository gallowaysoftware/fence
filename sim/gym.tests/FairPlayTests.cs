using System;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using System.Text.RegularExpressions;
using Xunit;

namespace Fence.Gym.Tests
{
    // The player sees only what a player could (docs/research/FAIR-PLAY.md). The game's hidden-truth accessors may be
    // called from the measurement layer (Known.cs: instruments the player owns, its own records), the summary counters
    // (Quality.cs), and the listed environment and log lines; anywhere else in sim/gym a call is a decision reading
    // the truth, and fails here.
    public sealed class FairPlayTests
    {
        static readonly Regex Hidden = new Regex(
            @"\b(IsFeatureMatch|realCondition|GetActualValueModifier|IsRealFeatureCheaper|CanClientExposeAnyFeature(Deep)?|CanClientExposeThisFeature|canClientExposeFeature"
            + @"|GetCorrectBaseValue|isAug|IsAug|isStealthWanted|dayUntilInspection|clientStack|futurEvents|GetFoundChance|IsOnlyIssueRecalled"
            + @"|GetWaterPurity|GetContaminantPercentage|GetHealthPercent|GetTraitRank|augPresence|clientImpatience|GetActualDisplay|GetBaseRealDisplay)\b"
            + @"|IngotPurityHelper\.GetPurity|ChemicalProductHelper\.GetPurity|WINE_SCORE_INT|CURRENT_PART_|_ISSUE""",
            RegexOptions.Compiled);

        static readonly HashSet<string> MeasurementLayer = new HashSet<string>(StringComparer.Ordinal) { "Known.cs", "Quality.cs" };

        // file, accessor, uses allowed, why it is not a decision.
        static readonly (string file, string token, int count, string why)[] Allowed =
        {
            ("Episode.cs", "clientStack", 3, "the gym's day loop serves the queue as the game does"),
            ("Episode.Appraisal.cs", "GetCorrectBaseValue", 1, "the slip's true value and the grade, logged after the estimate"),
            ("Episode.Player.cs", "isAug", 1, "the gym plays the aug's idle animation, as the game's StoreClientMono does"),
            ("Episode.Player.cs", "augPresence", 5, "decide.hidden and aug outcome records, never a prompt"),
            ("Episode.Player.cs", "IsAug", 1, "aug_eval scores the executor's verdicts against the truth"),
            ("StepMode.cs", "augPresence", 1, "reconstructible from the night log (docs/re/AUGS.md §4.1)"),
            ("Hydro.cs", "IsFeatureMatch", 1, "read right after the executor's own strip (or with the AquaScan): the scanner shows Match"),
            ("Hydro.cs", "GetContaminantPercentage", 1, "FENCE_BAY_DEBUG line"),
            ("Cellar.cs", "GetWaterPurity", 1, "FENCE_REED_DEBUG line"),
            ("Arms.cs", "IngotPurityHelper.GetPurity", 1, "only our own furnace's ingots, whose purity follows from what was loaded"),
            ("PlayerRole.cs", "GetWaterPurity", 1, "FENCE_WATER_DEBUG line"),
            ("Wine.cs", "WINE_SCORE_INT", 1, "the started_by_score summary counter"),
            ("SupplyProbe.cs", "WINE_SCORE_INT", 1, "a gym probe writing stock, not a player move"),
        };

        static string GymDir()
        {
            for (var d = new DirectoryInfo(AppContext.BaseDirectory); d != null; d = d.Parent)
                if (File.Exists(Path.Combine(d.FullName, "sim", "gym", "Known.cs"))) return Path.Combine(d.FullName, "sim", "gym");
            throw new DirectoryNotFoundException("sim/gym not found above " + AppContext.BaseDirectory);
        }

        [Fact]
        public void DecisionCodeReadsNoHiddenTruth()
        {
            var uses = new List<(string file, int line, string token, string text)>();
            foreach (var path in Directory.GetFiles(GymDir(), "*.cs").OrderBy(p => p, StringComparer.Ordinal))
            {
                string file = Path.GetFileName(path);
                if (MeasurementLayer.Contains(file)) continue;
                var lines = File.ReadAllLines(path);
                for (int n = 0; n < lines.Length; n++)
                {
                    string code = lines[n];
                    int cut = code.IndexOf("//", StringComparison.Ordinal);
                    if (cut >= 0 && !code.Substring(0, cut).Contains('"')) code = code.Substring(0, cut);
                    foreach (Match m in Hidden.Matches(code)) uses.Add((file, n + 1, m.Value, lines[n].Trim()));
                }
            }
            var errors = new List<string>();
            foreach (var g in uses.GroupBy(u => (u.file, u.token)))
            {
                var allow = Allowed.FirstOrDefault(a => a.file == g.Key.file && a.token == g.Key.token);
                if (g.Count() > allow.count)
                    errors.AddRange(g.Select(u => $"{u.file}:{u.line}: {u.token} ({(allow.file == null ? "not allowed" : $"allowed {allow.count}: {allow.why}")}): {u.text}"));
            }
            Assert.True(errors.Count == 0, "hidden-truth reads outside the measurement layer (docs/research/FAIR-PLAY.md):\n" + string.Join("\n", errors));
        }

        [Fact]
        public void TheGuardSeesAPlantedRead()
        {
            Assert.Matches(Hidden, "if (c.CanClientExposeAnyFeatureDeep(i)) return \"exposable\";");
            Assert.Matches(Hidden, "x = WaterHelper.GetWaterPurity(c);");
            Assert.Matches(Hidden, "i.IsTag(\"STAMP_SEAL_ISSUE\")");
            Assert.DoesNotMatch(Hidden, "if (Known.MayExpose(c, i)) return \"exposable\";");
        }
    }
}
