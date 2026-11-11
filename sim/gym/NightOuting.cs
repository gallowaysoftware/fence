using System.Collections.Generic;
using System.Linq;

namespace Fence.Gym
{
    // Bringing an outing's loot home (docs/STREAM.md rows 21 and 23). The bag's contents go into stock, the most
    // valuable first; loot that does not fit is worth room only when cheap stock can make it, and whatever is
    // still on the ground is left there when leaving (MapUIManager.Return's "leave items on the ground?" prompt,
    // answered yes).
    internal static class NightOuting
    {
        // Stock left behind for one item of loot is worth at most this share of it.
        const double JunkShare = 0.5;
        const int MaxJunk = 6;

        // StoreDoor.OnClick's gates, which a player going out meets before the map opens; null when it opens.
        public static string DoorShut()
        {
            var ps = PlayerStore.Instance;
            if (ps.devMode) return null;
            if (HealthData.IsSeriouslyWounded()) return "seriously_wounded";
            if (StoreStation.Instance.storeEventManager.IsEventActive("crackdown")) return "crackdown";
            if (StartingPerkContent.IsAgoraphobiaResting()) return "agoraphobia_rest";
            if (!ps.isAfterhoursIntroduced) return "afterhours_not_introduced";
            if (ps.storeState != PlayerStore.StoreState.CLOSED) return "store_not_closed";
            if (ps.visitLeftTonight <= 0) return "no_visit_left";
            return null;
        }

        public static List<GameItem> LootOrder(IEnumerable<GameItem> bag) =>
            bag.Where(i => i != null).OrderByDescending(PlayerRole.Est).ThenBy(i => i.uniqueId).ToList();

        // The stock a player would leave on the ground to fit `loot`, cheapest first (to be tried one at a time):
        // plain goods, nothing an engine, the plan or a tool needs, together worth under half the loot and
        // covering the cells it lacks. Empty when the loot is not worth room.
        public static List<GameItem> RoomFor(GameItem loot, IEnumerable<GameItem> stock, int freeCells)
        {
            int lootValue = PlayerRole.Est(loot), need = System.Math.Max(1, PlayerRole.Cells(loot) - freeCells);
            var junk = new List<GameItem>();
            // Without the executor's day (the gym's other human roles) nothing is judged junk.
            if (lootValue <= 0 || PlayerDay.Current == null) return junk;
            long spent = 0;
            int cells = 0;
            foreach (var i in stock.Where(Leavable).OrderBy(PlayerRole.Est).ThenBy(i => i.uniqueId))
            {
                if (cells >= need || junk.Count >= MaxJunk) break;
                int v = PlayerRole.Est(i);
                if (spent + v >= lootValue * JunkShare) break;
                junk.Add(i);
                spent += v;
                cells += PlayerRole.Cells(i);
            }
            return cells >= need ? junk : new List<GameItem>();
        }

        // Keep(i, null) reads a buyer for hot goods, so those are ruled out first.
        static bool Leavable(GameItem i) =>
            GeneralHelper.IsItemOwned(i) && !StolenHelper.IsStolenItem(i) && ContrabandHelper.GetContrabandLevelInInt(i) == 0
            && !PlayerRole.Keep(i, null) && !Production.Reserved(i) && !Production.IsInput(i.identifier)
            && !i.IsGameItemType("MACHINE") && !i.IsGameItemType("MODULE") && !i.IsTag("CONTAINER_TAG") && !i.IsTag("LIQUID_CONTAINER_TAG");
    }
}
