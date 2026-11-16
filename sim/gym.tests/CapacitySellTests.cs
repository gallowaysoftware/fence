using Xunit;

namespace Fence.Gym.Tests
{
    // R10: selling the shop's storage strangles the stock grid the engine kits leave at 0-8 free cells,
    // so the sell policy (PlayerRole.KeepWhy -> KeepCapacity) may not offer a stock container while the
    // grid would lose its free-cell margin on the sale. The margin is `min_free_cells` (default 36), the
    // same threshold Liquidating uses.
    public sealed class CapacitySellTests
    {
        [Theory]
        // A storage bay in the stock grid is 100 cells, a large bay 170; the margin is min_free_cells (36).
        // The policy holds the container while the free cells left after the sale are under the margin
        // (FreeCells - its own cells < 36). A 100-cell bay in hand means the grid free cells never reach
        // 136, so it is never offered; a large bay may sell only with enough of the grid left.
        [InlineData(100, 0, true)]      // live day 6: a bay sold into a nearly full grid
        [InlineData(100, 8, true)]      // engine kits leave 0-8 free cells
        [InlineData(100, 35, true)]     // one cell short of the margin after the sale
        [InlineData(100, 36, true)]     // at the margin, not over it: still short
        [InlineData(100, 135, true)]    // the grid keeps its margin with nothing else to spare
        [InlineData(100, 136, false)]   // plenty free
        [InlineData(170, 0, true)]
        [InlineData(170, 35, true)]     // 205 - 170 = 35: under the margin
        [InlineData(170, 205, true)]    // 205 - 170 = 35: under the margin
        [InlineData(170, 207, false)]   // 207 - 170 = 37: over the margin
        [InlineData(170, 210, false)]   // 210 - 170 = 40: plenty free
        public void TheSellPolicyKeepsTheBayWhileTheGridIsShort(int cells, int free, bool hold)
        {
            // The rule's exact condition: free cells left after the sale >= min_free_cells (36).
            Assert.Equal(hold, free - cells < 36);
        }
    }
}
