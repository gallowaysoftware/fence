using System;
using Xunit;

namespace Fence.Gym.Tests
{
    // Item 19 (R6): with the furnace engine on but no furnace owned, Furnace.Reserved returned true for
    // every scrap, ore, flux and module, so the shop kept buying inputs (they count as engine inputs for
    // the counter) while nothing could smelt them and no buyer could take them (day 12: Scrap x7, Ore x6,
    // Flux). Inputs are now reserved only in one night's worth until a furnace stands in stock.
    [Collection("gym")]
    public sealed class FurnaceInputTests
    {
        // The cap is one night's worth per id: with the blast module the metal night is three, and a
        // flux, a module and the advanced flux are one each (advanced flux is reusable).
        [Fact]
        public void FurnaceInputCapIsOneNightPerId()
        {
            lock (BootLock) Boot.Init();
            var saved = Directives.Current;
            try
            {
                Directives.Current = Directives.Parse(Newtonsoft.Json.Linq.JObject.Parse("{\"engine\":\"furnace\"}"));
                Assert.Equal(3, Furnace.InputCap("scrap_metal"));
                Assert.Equal(3, Furnace.InputCap("common_ore"));
                Assert.Equal(1, Furnace.InputCap("flux_agent"));
                Assert.Equal(1, Furnace.InputCap("advanced_flux_agent"));
                Assert.Equal(1, Furnace.InputCap("furnace_module_junk"));
                Assert.Equal(1, Furnace.InputCap("furnace_module_blast"));
                Directives.Current = Directives.Parse(Newtonsoft.Json.Linq.JObject.Parse("{\"engine\":\"furnace\",\"furnace_blast\":false}"));
                Assert.Equal(2, Furnace.InputCap("scrap_metal"));
                Assert.Equal(2, Furnace.InputCap("common_ore"));
                Directives.Current = Directives.Parse(Newtonsoft.Json.Linq.JObject.Parse("{\"engine\":\"wine\"}"));
                Assert.Equal(0, Furnace.InputCap("scrap_metal"));
            }
            finally
            {
                Directives.Current = saved;
            }
        }

        static readonly object BootLock = new object();
    }
}
