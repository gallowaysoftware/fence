using System;
using Fence.Recording;
using Newtonsoft.Json.Linq;

namespace Fence.Gym
{
    // What the executor's engines need from whatever drives the game: the gym's Episode headless, the
    // mod's autopilot live (docs/STREAM.md). The executor's files compile without Episode, under the
    // mod's rules (mod/src/Fence.Autopilot).
    internal interface IExecutorHost
    {
        int Day { get; }

        int Uid(GameItem item);

        // A recorded player move, as ModSide.Human.
        void Human(string action, AdviceInForce advice, int? itemUid = null, JObject extra = null, string defaultKey = null);

        void ProductionError(Exception e);

        // A read or chore that threw: headless it is counted, live it stops the autopilot.
        void Error(string where, Exception e);

        // A supplier's order by phone (Furnace, Suppliers): `choice` is the order's index in the call's menu, -1 for the
        // introduction call. Headless, the call's effects as its dialogue applies them; live, the phone's own flow.
        void PhoneOrder(StorePhoneClient client, long number, int choice, string courier, int days);

        // Presses the machine window's button with this label (the action re-reads the button each press),
        // or null when the window has none.
        Action MachineButton(GameItem machine, string label);
    }
}

namespace Fence.Gym
{
    // What the engines' item moves and machine actions go through (docs/STREAM.md section 3, rows 30–36).
    internal interface IChoreHands
    {
        // GraphUtils.TryAcceptAll's contract: the units it could not place.
        int Accept(GraphNodeStorage to, GameItem item, int amount);

        T Act<T>(string what, GameItem target, Func<T> act);
    }

    // Headless (no hands set) these are the game calls themselves, so the gym is unchanged; the live autopilot sets
    // Hands, which gate, re-check and record each one.
    internal static class Chore
    {
        public static IChoreHands Hands;

        public static int Accept(GraphNodeStorage to, GameItem item, int amount = -1) =>
            Hands != null ? Hands.Accept(to, item, amount) : GraphUtils.TryAcceptAll(to, item, amount);

        public static T Act<T>(string what, GameItem target, Func<T> act) => Hands != null ? Hands.Act(what, target, act) : act();

        public static void Act(string what, GameItem target, Action act) => Act(what, target, () =>
        {
            act();
            return true;
        });

        // The faucet a container is held to: live the shop's own (a made one would be a stray item in the scene);
        // headless a made one, as before (the container's handler reads only the target's identifier).
        public static GameItem Faucet() => Hands != null && EmporiumEntry.Instance?.faucet != null ? EmporiumEntry.Instance.faucet : DirectoryMaster.Item("faucet");
    }
}

