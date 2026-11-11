using System.Collections.Generic;

namespace Fence.Gym
{
    // The gym's stock-breadth trace (sim/gym/DemandTrace.cs) writes files, which a mod may not; the shared
    // executor calls it, so the mod compiles this silent stand-in instead.
    internal static class DemandTrace
    {
        public static void Buyer(StoreClient c, IList<GameItem> owned, IList<GameItem> picked, long room)
        {
        }

        public static void Seller(StoreClient c, IList<GameItem> theirs)
        {
        }
    }
}
