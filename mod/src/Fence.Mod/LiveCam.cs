using UnityEngine;

namespace Fence.Mod
{
    // The chemistry has to be seen to be believed: when a lab chore touches a machine, the
    // camera jumps to it and the machine's window opens, so the droppers, cylinders and grids
    // play on screen instead of happening off-camera. The counter camera is restored after.
    static class LiveCam
    {
        static Vector3 saved;
        static bool focused;

        // A machine item's world position (its TreeNodeRender). Items moved by ModHelper's own
        // calls animate like a player drag, so once the camera is there the move is visible.
        public static void Focus(GameItem item)
        {
            var cam = Camera.main;
            var el = item as GameItemElement;
            if (cam == null || el?.handler == null) return;
            if (!focused)
            {
                saved = cam.transform.position;
                focused = true;
            }
            var p = el.handler.rectTransform.position;
            cam.transform.position = new Vector3(p.x, p.y, saved.z);
            item.contentWindow?.Show();
        }

        public static void Restore()
        {
            if (!focused) return;
            focused = false;
            var cam = Camera.main;
            if (cam != null) cam.transform.position = saved;
        }

        // True when the item is a machine worth showing: has a world object and a content window.
        public static bool IsMachine(GameItem item) =>
            item is GameItemElement el && el.handler != null && item.contentWindow != null && item.IsTag("MACHINE");
    }
}
