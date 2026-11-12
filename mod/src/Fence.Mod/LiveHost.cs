using System;
using Fence.Barter;
using Fence.Gym;
using Fence.Recording;
using Newtonsoft.Json.Linq;

namespace Fence.Mod
{
    // The live half of IExecutorHost (the gym's is Episode.Host.cs): the executor's engines record and fail
    // through it, a supplier's phone order becomes a call the driver makes through the phone's own flow, and a
    // machine window's button is the driver's gated press (LiveMachines lists the ones pressed live).
    sealed class LiveHost : IExecutorHost
    {
        readonly GameBarter game;
        readonly Recorder.Emit emit;
        readonly Action<string> fault;
        readonly Action<Fence.Autopilot.PhoneCall> call;
        readonly Func<GameItem, string, Action> button;

        public LiveHost(GameBarter game, Recorder.Emit emit, Action<string> fault, Action<Fence.Autopilot.PhoneCall> call, Func<GameItem, string, Action> button)
        {
            this.button = button;
            this.game = game;
            this.emit = emit;
            this.fault = fault;
            this.call = call;
        }

        public int Day => GameAccess.DayCounter();

        public int Uid(GameItem item) => game.Uid(item);

        public void Human(string action, AdviceInForce advice, int? itemUid = null, JObject extra = null, string defaultKey = null)
        {
            var data = HumanActions.Build(action, advice, itemUid, extra);
            data["actor"] = Actors.Autopilot;
            if (defaultKey != null) data["rule"] = defaultKey;
            emit(RecordKinds.HumanAction, data, null);
        }

        public void ProductionError(Exception e) => fault("production: " + e.GetType().Name + ": " + e.Message);

        public void Error(string where, Exception e) => fault(where + ": " + e.GetType().Name + ": " + e.Message);

        public void PhoneOrder(StorePhoneClient client, long number, int choice, string courier, int days) =>
            call(new Fence.Autopilot.PhoneCall { Number = number, Choice = choice, Who = client.GetLocalizedDisplayName(), Why = choice < 0 ? "the introduction call (" + courier + " in " + days + " days)" : "order " + (choice + 1) + " (" + courier + " in " + days + " days)" });

        public Action MachineButton(GameItem machine, string label) => button(machine, label);
    }
}
