using System.Collections.Generic;
using System.Linq;
using Fence.Recording;
using Newtonsoft.Json.Linq;

namespace Fence.Gym
{
    // Bringing an outing's loot home (docs/STREAM.md rows 21 and 23). The player carries it in the two pockets
    // (one slot each) plus the backpack slot, whose window shows the owned backpack's grid; the return brings
    // those and the ground expels whatever was left (EmporiumEntry.TransferAfterhourToInv).
    internal static class NightOuting
    {
        // Stock left behind for one item of loot is worth at most this share of it.
        const double JunkShare = 0.5;
        const int MaxJunk = 6;

        // The afterhours box (EmporiumEntry.SetupAfterhourInv): two one-slot pockets and the backpack slot, whose
        // window is 3 x 3 and takes only BACKPACK_TAG items (the owned backpack's grid is what actually holds).
        public const int Pockets = 2;

        // The backpacks the player can own, their grids (ContainerItemDirectory): the scavenged satchel (ScavHelper's
        // 3 % find, satchel then kids_backpack, once each) and the shop's backpacks.
        static readonly (string id, int rows, int cols)[] BackpackGrids =
        {
            ("satchel", 6, 3), ("kids_backpack", 5, 5), ("kids_backpack_large", 5, 6),
            ("backpack_small", 6, 6), ("backpack_medium", 6, 9), ("backpack_medium_military", 6, 9),
            ("backpack_large", 8, 9), ("backpack_large_military", 7, 11), ("fanny_pack", 3, 3),
        };

        public static int BackpackCells(string identifier)
        {
            foreach (var b in BackpackGrids)
                if (identifier == b.id) return b.rows * b.cols;
            // A backpack of an unknown shape (a mod's): the slot's window caps its carry at 3 x 3.
            return 3 * 3;
        }

        public static bool IsBackpack(GameItem i) => i != null && i.IsTag("BACKPACK_TAG");

        // A bag the player uses for outings (the player's equipment, not stock): the shop's backpacks and the
        // scavenger's finds in this table (the satchel, the kids' packs), by identifier (a mod's bag is unknown
        // here and stays stock, as IsBackpack alone would keep it). The keep rule (PlayerRole.KeepWhy "bag")
        // applies to this, not to IsBackpack: a buyer's window may show a bag the shop does not own.
        public static bool IsBag(GameItem i) => i != null && BackpackGrids.Any(b => b.id == (i.identifier ?? ""));

        // `bag_spare` (gym, default 2): the most-valued owned bags are the outing's bag and its spares, kept
        // out of the counter; any owned beyond that is sellable stock again (a bag is the cheapest of the tools
        // in a player's eye, but it is the only bag the night's pick lists, so selling the best is worse).
        public static bool KeepBag(GameItem i)
        {
            int keep = (Directives.Current.Raw.Value<int?>("bag_spare") ?? 2) + 1;
            var bags = EmporiumEntry.Instance.GetAllOwnedItems(false, true).Where(x => x != null && (IsBag(x) || IsBackpack(x))).ToList();
            return bags.Count <= keep || bags.OrderByDescending(x => x.GetNegociatedValue()).ThenBy(x => x.uniqueId).Take(keep).Contains(i);
        }

        // The backpacks the player owns (the bag choice's list): the shop's stock plus the afterhours box, which
        // holds one found there before this night (ScavHelper marks the find by it landing in the box).
        public static IEnumerable<GameItem> OwnedBackpacks()
        {
            foreach (var i in EmporiumEntry.Instance.GetAllOwnedItems(false, true))
                if (IsBackpack(i)) yield return i;
        }

        // The grid the player would choose at the afterhours bag choice (ResponseList.CreateAfterhoursResponse,
        // which lists the owned BACKPACK_TAG items): the best-value one; null for the plastic bag.
        public static GameItem OwnedBackpack(IEnumerable<GameItem> owned)
        {
            GameItem best = null;
            foreach (var i in owned)
                if (IsBackpack(i) && (best == null || PlayerRole.Est(i) > PlayerRole.Est(best) || (PlayerRole.Est(i) == PlayerRole.Est(best) && i.uniqueId < best.uniqueId)))
                    best = i;
            return best;
        }

        // What the player carries home (the real UI's packing, approximated by slot count: a shape-packed grid is
        // worth at most its cell count, and the loot here is mostly one-cell finds): the two pockets first, then
        // the backpack's grid; a backpack found in the loot takes a pocket, its grid still counting.
        public static (List<GameItem> carried, List<GameItem> left) Pack(IEnumerable<GameItem> bag, GameItem backpack)
        {
            var rest = LootOrder(bag).ToList();
            var carried = new List<GameItem>();
            int pockets = Pockets;
            int cells = 0;
            if (backpack != null)
            {
                cells = BackpackCells(backpack.identifier);
                // A scavenged backpack rides in a pocket (the slot takes BACKPACK_TAG items) and still opens its grid.
                var found = rest.Find(IsBackpack);
                if (found != null && pockets > 0)
                {
                    rest.Remove(found);
                    pockets--;
                    carried.Add(found);
                }
            }
            // The pockets fill first, then the backpack's grid; the rest stays in the bag to be expelled.
            int packed = 0;
            foreach (var i in rest)
            {
                if (pockets <= 0 && cells <= 0) break;
                if (pockets > 0) pockets--;
                else cells--;
                carried.Add(i);
                packed++;
            }
            // packed, not carried.Count: a scavenged backpack rides in carried but was already removed from rest.
            return (carried, rest.Skip(packed).ToList());
        }

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

        // The game's own gate on the search button (ScavHelper.CanScavenge): the night's searching ends the
        // moment a fresh wound shows, whether the searches run out or not.
        public static bool CanScavenge() => ScavHelper.CanScavenge();

        public static JObject ScavengeRefused() =>
            new JObject
            {
                ["reason"] = "fresh_wound",
                ["wound_state"] = PlayerStore.Instance.healthData.woundState,
                ["searches_left"] = ScavHelper.GetScavTimeLeft(),
            };

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
