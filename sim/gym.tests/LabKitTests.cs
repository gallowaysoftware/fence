using System;
using Xunit;
using Newtonsoft.Json.Linq;

namespace Fence.Gym.Tests
{
    // The chemistry engine's durable kit survives the sell path while the lab runs
    // (Production.ProtectsLabTool): the scanner is the analyser itself.
    [Collection("gym")]
    public class LabKitTests
    {
        static void WithDirectives(string json, Action body)
        {
            lock (WaterKitTests.BootLock) Boot.Init();
            var saved = Directives.Current;
            try
            {
                Directives.Current = Directives.Parse(JObject.Parse(json));
                body();
            }
            finally
            {
                Directives.Current = saved;
            }
        }

        [Fact]
        public void ScannerIsProtectedWithoutTheWaterEngine()
        {
            WithDirectives("{\"engine\":\"wine+furnace\",\"lab\":{\"products\":[\"dream_dust\"]}}", () =>
            {
                Assert.True(Production.ProtectsLabTool("chem_scanner"), "scanner unprotected on wine+furnace");
                Assert.True(Production.ProtectsLabTool("chem_finisher"), "finisher unprotected");
                Assert.False(Production.ProtectsLabTool("water_filter"), "water filter is not lab kit");
            });
        }

        [Fact]
        public void LabKitIsStockWhenTheLabIsOff()
        {
            WithDirectives("{\"engine\":\"wine+furnace\"}", () =>
            {
                Assert.False(Production.ProtectsLabTool("chem_scanner"), "scanner protected with no lab");
                Assert.False(Production.ProtectsLabTool("blender"), "blender protected with no lab");
            });
        }
    }
}
