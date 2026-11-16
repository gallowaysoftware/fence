using System.Collections.Generic;
using System.Linq;

namespace Fence.Gym
{
    // The stock grid's fit check (item 21, R4). The grid is a `width` x `height` cell matrix
    // (inventoryShape: 0 = usable cell, 1 = frame) holding the shop's items, each occupying its
    // item shape (`modifiedShape`, an oriented GridShape placed at its cell; mostly axis-aligned
    // rectangles). The game decides an item fits by its slot search (TryInventorySlot): the shape
    // fully inside the usable cells, not intersecting a resident, its slot allowed one unit. A
    // free-cell count cannot see any of this: a 2 x 2 item does not fit two separate cells, and
    // a lot of wide items needs contiguous runs, not just the sum of their cells.
    internal static class CounterFit
    {
        // One item to place. `occupied` is the shape's footprint (1 = cell it takes,
        // GridShapeBuilder's byte layout), `size` the cells in it (GetItemSize's count).
        public readonly struct Shape
        {
            public readonly byte[] Occupied;
            public readonly int W, H, Size;

            public Shape(byte[] occupied, int w, int h, int size)
            {
                Occupied = occupied;
                W = w;
                H = h;
                Size = size;
            }

            public static Shape Of(GridShape s)
            {
                if (s == null) return new Shape(new byte[] { 1 }, 1, 1, 1);
                int w = s.width, h = s.height;
                var occ = new byte[w * h];
                int size = 0;
                for (int y = 0; y < h; y++)
                    for (int x = 0; x < w; x++)
                        if (s.GetLocal(x, y) == 1)
                        {
                            occ[y * w + x] = 1;
                            size++;
                        }
                return new Shape(occ, w, h, size);
            }
        }

        // The grid's free cells as a matrix (false = taken: frame or a resident's cell) and how
        // many — the old free-cell count, with the residency read per cell the way the game's
        // HasFreeCell does.
        static (bool[] free, int count) FreeMap(GameGridInventory grid)
        {
            var shape = grid.inventoryShape;
            int cap = shape.width * shape.height;
            var free = new bool[cap];
            int count = 0;
            for (int y = 0; y < shape.height; y++)
                for (int x = 0; x < shape.width; x++)
                {
                    if (shape.Get(x, y) != 0) continue;
                    free[y * shape.width + x] = true;
                    count++;
                    foreach (var it in grid.childItems)
                        if (it != null && it.modifiedShape != null && it.modifiedShape.Intersects(x, y))
                        {
                            free[y * shape.width + x] = false;
                            count--;
                            break;
                        }
                }
            return (free, count);
        }

        // Whether all the shapes fit the grid at once: each finds a cell the game's slot search
        // (TryInventorySlot) would accept — its shape inside the free cells (in that order, in
        // place, no rotating or moving residents) — and placing it leaves at least `margin` free
        // cells behind. `margin` is the plan's `min_free_cells` (PlayerDay.Liquidating's default):
        // D11's threshold, kept as the floor the placement leaves the shop with rather than one
        // the purchase was measured against.
        public static bool Fits(GameGridInventory grid, IEnumerable<Shape> shapes, int margin = 0)
        {
            var shape = grid.inventoryShape;
            var (free, freeCount) = FreeMap(grid);
            if (freeCount < margin) return false;
            foreach (var sh in shapes)
            {
                (int px, int py)? put = null;
                for (int y = 0; y <= shape.height - sh.H; y++)
                    for (int x = 0; x <= shape.width - sh.W; x++)
                    {
                        bool ok = true;
                        for (int cy = 0; cy < sh.H && ok; cy++)
                            for (int cx = 0; cx < sh.W; cx++)
                                if (sh.Occupied[cy * sh.W + cx] == 1 && !free[(y + cy) * shape.width + (x + cx)])
                                {
                                    ok = false;
                                    break;
                                }
                        if (ok)
                        {
                            put = (x, y);
                            break;
                        }
                    }
                if (put == null) return false;
                for (int cy = 0; cy < sh.H; cy++)
                    for (int cx = 0; cx < sh.W; cx++)
                        if (sh.Occupied[cy * sh.W + cx] == 1) free[(put.Value.py + cy) * shape.width + (put.Value.px + cx)] = false;
                freeCount -= sh.Size;
                if (freeCount < margin) return false;
            }
            return true;
        }
    }
}