using System;
using System.IO;
using System.Linq;
using System.Reflection;
using Xunit;

namespace Fence.Gym.Tests
{
    // The water engine's durable kit (filters, pitcher, strips and their box, tablets and their box,
    // scanners, purifiers, tanks) is equipment, not stock: while the water engine exists — `engine: water`
    // (or `wine+water`), even with selling water off — or a purifier is owned, it is held by the sell
    // policy (KeepWhy "water_kit"). Raw-water vessels are not kit: a jug or a ration is input the
    // surplus rule handles, sellable stock again otherwise.
    [Collection("gym")]
    public sealed class WaterKitTests
    {
        internal static readonly object BootLock = new object();

        static readonly string[] KitIds =
        {
            "water_filter", "water_pitcher", "water_test_container", "water_test_strip", "water_tablet_container",
            "water_tablet", "aquascan", "water_purifier", "portable_water_purifier", "water_tank",
        };

        // ---- ProtectsWaterTool: pure over the engine name -----------------------------------------------------

        static void WithDirectives(string json, Action body)
        {
            lock (BootLock) Boot.Init();
            var saved = Directives.Current;
            try
            {
                Directives.Current = Directives.Parse(Newtonsoft.Json.Linq.JObject.Parse(json));
                body();
            }
            finally
            {
                Directives.Current = saved;
            }
        }

        [Theory]
        [InlineData("furnace+water", true)]
        [InlineData("water", true)]
        [InlineData("wine+water", true)]
        [InlineData("wine+furnace", false)]
        [InlineData("furnace", false)]
        [InlineData("none", false)]
        public void ProtectsEveryKitIdWhileTheWaterEngineExists(string engine, bool protect)
        {
            WithDirectives("{\"engine\":\"" + engine + "\"}", () =>
            {
                foreach (var id in KitIds)
                    Assert.True(protect == Production.ProtectsWaterTool(id), engine + " " + id);
            });
        }

        [Theory]
        [InlineData("water_ration", false)]
        [InlineData("water_jug", false)]
        [InlineData("bottled_water", false)]
        [InlineData("small_bottled_water", false)]
        [InlineData("large_bottled_water", false)]
        [InlineData("energy_credit", false)]
        [InlineData("scrap_metal", false)]
        [InlineData("water_guide", false)]
        [InlineData(null, false)]
        public void ProtectsNoKitIdOrNothingElse(string id, bool protect)
        {
            WithDirectives("{\"engine\":\"furnace+water\"}", () =>
            {
                Assert.True(protect == Production.ProtectsWaterTool(id), id?.ToString() ?? "null");
            });
        }

        // A null engine name (no plan at all): no water engine; a purifier is still kit.
        [Fact]
        public void ANullEngineProtectsNothingWithoutAPurifier()
        {
            lock (BootLock) Boot.Init();
            var saved = Directives.Current;
            try
            {
                Directives.Current = new Directives(); // Engine null
                foreach (var id in KitIds)
                    Assert.True(!Production.ProtectsWaterTool(id), id);
            }
            finally
            {
                Directives.Current = saved;
            }
        }

        // ---- KeepWhy: the sell policy holds the kit, not the vessels ------------------------------------------

        static string Dir(string name)
        {
            lock (BootLock) Boot.Init();
            string d = Boot.Var("sim", "test", name);
            if (System.IO.Directory.Exists(d)) System.IO.Directory.Delete(d, true);
            System.IO.Directory.CreateDirectory(d);
            return d;
        }

        // Boots one episode (new game + first morning) so the stock reads work, with the given directives.
        static void BootEpisode(string d, string directives)
        {
            lock (BootLock) Boot.Init();
            File.WriteAllText(d + "directives.json", directives);
            var ep = new Episode(new GymOptions { Seed = 1, Human = "player", SkipIntro = true, DirectivesFile = d + "directives.json" }, new LocalBridge(1), null);
            ep.BaseDirectives = Directives.Load(d + "directives.json");
            ep.Step();
        }

        // The game types are not referenced by the test assembly: items come from the game's own directory
        // factory (the gym's ExecutorHost.Faucet path, DirectoryMaster.Item), reached through reflection once
        // Boot has installed the headless game.
        static System.Reflection.MethodInfo Make;

        static object Item(string id)
        {
            if (Make == null)
            {
                var t = AppDomain.CurrentDomain.GetAssemblies().Select(a => { try { return a.GetType("DirectoryMaster"); } catch { return null; } }).FirstOrDefault(x => x != null);
                if (t != null)
                {
                    var ms = t.GetMethods(BindingFlags.Public | BindingFlags.Static | BindingFlags.NonPublic);
                    Make = ms.FirstOrDefault(m => m.Name == "Item" && !m.IsGenericMethodDefinition
                        && m.GetParameters().Length == 2 && m.GetParameters()[0].ParameterType == typeof(string) && m.GetParameters()[1].ParameterType == typeof(bool));
                    if (Make == null)
                        throw new InvalidOperationException("DirectoryMaster.Item not found: " + string.Join("; ", ms.Select(m => m.Name + (m.IsGenericMethodDefinition ? "<T>" : "") + "(" + string.Join(",", m.GetParameters().Select(p => p.ParameterType.Name)) + ")").Take(40)));
                }
                else throw new InvalidOperationException("DirectoryMaster type not found");
            }
            return Make.Invoke(null, new object[] { id, true });
        }

        static System.Collections.Generic.IEnumerable<Type> SafeTypes(System.Reflection.Assembly a)
        {
            try { return a.GetTypes(); }
            catch (ReflectionTypeLoadException e) { return e.Types.Where(x => x != null); }
            catch { return Array.Empty<Type>(); }
        }

        // KeepWhy's signature names the game types (unreferenced here), so call it through reflection.
        static string Keep(object item)
        {
            var m = typeof(PlayerRole).GetMethod("KeepWhy", BindingFlags.Public | BindingFlags.Static);
            return (string)m.Invoke(null, new[] { item, null });
        }

        // The water route (engine water, selling off, no tap): the kit stays with the water engine even
        // though water is never sold, and the vessels are not kit.
        [Theory]
        [InlineData("water_filter", "water_kit")]
        [InlineData("water_pitcher", "water_kit")]
        [InlineData("water_test_container", "water_kit")]
        [InlineData("water_test_strip", "water_kit")]
        [InlineData("water_tablet_container", "water_kit")]
        [InlineData("water_tablet", "water_kit")]
        [InlineData("aquascan", "water_kit")]
        [InlineData("water_purifier", "water_kit")]
        [InlineData("portable_water_purifier", "water_kit")]
        [InlineData("water_tank", "water_kit")]
        [InlineData("water_ration", null)]
        [InlineData("water_jug", null)]
        [InlineData("bottled_water", null)]
        public void TheWaterEngineKeepsItsKit(string id, string why)
        {
            string d = Dir("water_kit");
            BootEpisode(d, "{\"engine\":\"water\"}");
            Assert.True((Keep(Item(id)) ?? "") == (why ?? ""), id + " kept as " + Keep(Item(id)));
        }

        // `wine+water`: the winemaker's kit and the water's kit are both held, the vessels still stock.
        [Theory]
        [InlineData("water_test_container", "water_kit")]
        [InlineData("water_filter", "water_kit")]
        [InlineData("water_jug", null)]
        public void TheWineWaterEngineKeepsTheWaterKit(string id, string why)
        {
            string d = Dir("water_kit_wine");
            BootEpisode(d, "{\"engine\":[\"wine\",\"water\"]}");
            Assert.True((Keep(Item(id)) ?? "") == (why ?? ""), id + " kept as " + Keep(Item(id)));
        }

        // The wine engine alone (no purifier owned, none bought by day one): nothing water is kit.
        [Theory]
        [InlineData("water_filter", null)]
        [InlineData("water_pitcher", null)]
        [InlineData("water_test_container", null)]
        [InlineData("water_purifier", null)]
        [InlineData("water_tank", null)]
        [InlineData("water_jug", null)]
        public void TheWineEngineAloneKeepsNoWaterKit(string id, string why)
        {
            string d = Dir("water_kit_wine_only");
            BootEpisode(d, "{\"engine\":\"wine\",\"upgrades_buy\":[]}");
            Assert.True((Keep(Item(id)) ?? "") == (why ?? ""), id + " kept as " + Keep(Item(id)));
        }
    }
}
