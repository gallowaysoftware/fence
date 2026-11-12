using System.Collections.Generic;
using System.Linq;
using System.Runtime.CompilerServices;
using Fence.Autopilot;

namespace Fence.Mod
{
    // The machine windows' buttons the shared engines press (IExecutorHost.MachineButton), found the way a player finds
    // them: in the window, by their label. Reads only; LiveHands presses. Only the buttons listed here are pressed live.
    static class LiveMachines
    {
        // The blender's and the feed dispenser's start (Lab, Rats), the chem finisher's output (Lab), the bottle printer's
        // three sizes (Water). The RAS's blend (Fish) is IsTankBlend.
        static readonly string[] Keys =
        {
            "mech_blender_btn_start", "mech_chem_finisher_btn_output", "mech_bottle_printer_btn_print_normal", "mech_bottle_printer_btn_print_large",
        };

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static bool Pressable(string label)
        {
            if (string.IsNullOrEmpty(label)) return false;
            if (label == LocHelper.GetLocalizedMechanicOrDefault("mech_bottle_printer_btn_print_small", "Print Small Bottle")) return true;
            return Keys.Any(k => label == LocHelper.GetLocalizedMechanic(k));
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static bool IsTankBlend(GameItem machine, string label) =>
            machine != null && machine.IsTag("RAS_TAG") && label == LocHelper.GetLocalizedMechanic("mech_ras_btn_blend");

        // The RAS hides its blend button while no dispenser is slotted (MachineRAS.RefreshBlendButton).
        [MethodImpl(MethodImplOptions.NoInlining)]
        public static bool TankCanBlend(GameItem ras)
        {
            var disp = MachineRAS.GetDispenserSlot(ras)?.childItem;
            return disp != null && disp.IsTag("FEED_DISPENSER_TAG");
        }

        // The window's button with this label that a click could select (shown, enabled), or null.
        [MethodImpl(MethodImplOptions.NoInlining)]
        public static MachineryButton Button(GameItem machine, string label)
        {
            if (machine?.contentWindow == null || string.IsNullOrEmpty(label)) return null;
            return UiTree.Find(machine.contentWindow, Children, o => o is MachineryButton b && b.text == label && !b.isHidden && !b.isDisabled ? b : null);
        }

        // The public widget tree: the window's row, its panels, their content, an intake's content, a stack's elements.
        // A GridPixelElement's children are private (the RAS's blend button sits in one).
        [MethodImpl(MethodImplOptions.NoInlining)]
        static IEnumerable<object> Children(object o)
        {
            switch (o)
            {
                case PixelWindow w: return new object[] { w.childElement };
                case MachineryPanelRow r: return r.Panels;
                case MachineryPanel p: return new object[] { p.content };
                case MachineryIntake i: return new object[] { i.content };
                case MachineryStack s: return s.elements;
                default: return null;
            }
        }
    }
}
