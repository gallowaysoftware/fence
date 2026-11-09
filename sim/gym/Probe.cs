using System;
using System.Linq;
using System.Runtime.CompilerServices;

namespace Fence.Gym
{
    // `fence-gym probe`: quick checks of the headless game's inventory plumbing.
    internal static class Probe
    {
        [MethodImpl(MethodImplOptions.NoInlining)]
        public static int Run()
        {
            using var bridge = new LocalBridge(1);
            var ep = new Episode(new GymOptions { Seed = 1, Days = 1 }, bridge, null);
            ep.Step();
            var e = EmporiumEntry.Instance;
            Console.WriteLine($"stock {e.invElement.childItems.Count} shape {e.invElement.inventoryShape?.GetType().Name} showcase {e.showcaseElement.childItems.Count}");
            foreach (var id in new[] { "scrap_metal", "alarm_module_transmitter", "marking_pen", "toolbox", "trashcan" })
            {
                GameItem it = null;
                try { it = DirectoryMaster.Item(id, true); } catch (Exception x) { Console.WriteLine(id + ": " + x.Message); continue; }
                if (it == null) { Console.WriteLine(id + ": null"); continue; }
                GeneralHelper.SetItemOwned(it, true);
                bool may = e.invElement.MayHaveValidInventorySlot(it);
                var slot = e.invElement.TryFindOneValidInventorySlot(it);
                int r = GraphUtils.TryAcceptAll(e.invElement, it, -1);
                Console.WriteLine($"{id}: may {may} slot {(slot == null ? "null" : slot.numTransfer.ToString())} rest {r} parent {(ReferenceEquals(it.parentInventory, e.invElement) ? "stock" : it.parentInventory?.GetType().Name ?? "none")} shape {it.modifiedShape?.GetType().Name}");
            }
            return 0;
        }
    }
}
