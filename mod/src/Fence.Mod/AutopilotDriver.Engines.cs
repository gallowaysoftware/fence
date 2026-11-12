using System;
using Fence.Autopilot;
using Fence.Barter;
using Fence.Gym;
using Fence.Recording;
using Newtonsoft.Json.Linq;

namespace Fence.Mod
{
    // The engines that were gym-only (docs/STREAM-OPS.md section 12): the machine windows' buttons the shared engines
    // press (the lab's blender and finisher, the RAS's blend, the bottle printer, the rats' feeder), the lab's
    // thermostat, and the Armourer's gun orders. A press that fails throws inside the engine's chore, which stops the
    // autopilot (a fault, never a retry); a gun order that cannot be delivered puts the gun back and hands over.
    sealed partial class AutopilotDriver
    {
        // Whether the members of autopilot.machines and autopilot.gun_orders resolved: without them the engines see no
        // buttons (as before the port) and order clients are sent away.
        public bool MachinesUsable = true;
        public bool GunOrdersUsable = true;

        readonly Thermostat thermostat = new Thermostat();
        // Inside a quiet chore (the gun order's trial fits, the thermostat) moves and presses are counted, not recorded
        // one by one.
        bool choreQuiet;
        int quietChores;

        // ---- machine buttons (IExecutorHost.MachineButton) ----

        Action MachineButton(GameItem machine, string label)
        {
            if (machine == null || !MachinesUsable) return null;
            if (LiveMachines.IsTankBlend(machine, label))
                return LiveMachines.TankCanBlend(machine) ? Press(machine, label, null) : null;
            if (!LiveMachines.Pressable(label)) return null;
            var button = LiveMachines.Button(machine, label);
            return button == null ? null : Press(machine, label, button);
        }

        Action Press(GameItem machine, string label, MachineryButton button) => () =>
        {
            var gate = actGate ?? throw new InvalidOperationException("a machine button pressed outside an autopilot action");
            bool pressed = button == null ? hands.BlendTank(gate, machine) : hands.PressMachineButton(gate, button);
            if (!pressed) throw new InvalidOperationException(label + " on " + LiveGame.Name(machine) + " was refused (paused, hidden or disabled)");
            if (choreQuiet) quietChores++;
            else Record("chore", Planned(machine), "pressed " + label, "chore", null);
        };

        // ---- the lab's thermostat ----

        // Lab.Second once a second of wall time while the autopilot runs, whatever else it is doing: the plate is
        // switched between low and off on its readout until the batch's window is held (Lab.Thermostat). Paused, the
        // plate stays as it was (docs/STREAM-OPS.md section 12, "If you pause").
        void TickThermostat(LiveView v, long now)
        {
            if (!v.InStore || v.Loading || v.GameOver || Directives.Current == null || !Lab.On || !thermostat.Due(now)) return;
            var gate = ActGate.TryOpenAutopilot(() => Enabled && sw.Running);
            if (gate == null) return;
            int before = quietChores;
            actGate = gate;
            executing = true;
            choreQuiet = true;
            recorder?.Filter.EnterExecutor();
            try
            {
                Lab.Second();
            }
            catch (Exception e)
            {
                Stop(AutopilotState.Faulted, "lab thermostat: " + e.GetType().Name + ": " + e.Message);
            }
            finally
            {
                recorder?.Filter.ExitExecutor();
                choreQuiet = false;
                executing = false;
                actGate = null;
            }
            int n = thermostat.Switched(quietChores - before, now);
            if (n > 0) Record("thermostat", null, "the heating plate switched " + n + " time" + (n == 1 ? "" : "s") + " on its readout to hold the batch's window", "player:lab", null, v);
        }

        // ---- gun orders (Arms.Prepare; the gym's Episode.PlayerGunOrder) ----

        void GunOrder(ActGate gate, LiveView v, long now)
        {
            memory.GunOrderDone = true;
            var c = LiveGame.Client();
            var o = c?.gunOrderData;
            if (o == null || c.clientIntent != StoreClient.ClientIntent.GUNSMITH || c.isBusinessComplete)
            {
                Record("gun_order_skip", null, "the order is gone", "player:gun_order", null, v);
                return;
            }
            string order = "tier " + o.tier + (o.isServiceOrder ? ", service" : "") + (o.isBestOrder ? ", best" : "");
            Arms.Plan plan;
            int fits = quietChores, parts = Arms.PartsUsed;
            choreQuiet = true;
            try
            {
                plan = Arms.Prepare(c);
            }
            catch (Exception e)
            {
                Stop(AutopilotState.Faulted, "gun order plan: " + e.GetType().Name + ": " + e.Message + " (mods may be left on a gun)");
                return;
            }
            finally
            {
                choreQuiet = false;
            }
            fits = quietChores - fits;
            parts = Arms.PartsUsed - parts;
            string tried = (fits > 0 ? "; " + fits + " fits tried" : "") + (parts > 0 ? "; " + parts + " gun part" + (parts == 1 ? "" : "s") + " spent on repairs" : "");
            if (plan?.Gun == null)
            {
                string why = plan?.Why ?? "orders off";
                Arms.Refused(why);
                Record("gun_order_refused", null, why + tried, "player:gun_order_refused", null, v, detail: order);
                if (v.ClientDismissable) Dismiss(v, "the order: " + why, "player:gun_order_refused", now);
                else Stop(AutopilotState.Handover, (v.ClientName ?? "the order client") + ": the order is refused (" + why + ") and the client cannot be sent away");
                return;
            }
            var gun = plan.Gun;
            var m = Planned(gun);
            if (!plan.Service && !(hands.Move(gate, gun, LiveHands.Back) && LiveGame.IsIn(gun, LiveHands.Back)))
            {
                Arms.Refused("could not stage the gun");
                Record("gun_order_refused", m, "the gun would not go on the back counter", "player:gun_order_refused", null, v, detail: order);
                if (v.ClientDismissable) Dismiss(v, "the order: the gun could not be staged", "player:gun_order_refused", now);
                else Stop(AutopilotState.Handover, (v.ClientName ?? "the order client") + ": the gun could not be staged");
                return;
            }
            if (!plan.Service && !ReferenceEquals(LiveGame.OrderGun(), gun))
            {
                GunBack(gate, gun, v);
                Stop(AutopilotState.Handover, "another gun on the back counter is the one the order would take; the order is yours");
                return;
            }
            int cash0 = LiveGame.Cash();
            if (!hands.AcceptGunOrder(gate, gun))
            {
                if (!plan.Service) GunBack(gate, gun, v);
                Stop(AutopilotState.Handover, "the order tablet does not accept " + m.Name + " (" + order + "); the order is yours");
                return;
            }
            int paid = LiveGame.Cash() - cash0;
            if (paid <= 0)
            {
                Arms.Refused("accept did nothing");
                if (!plan.Service) GunBack(gate, gun, v);
                Stop(AutopilotState.Handover, "the order's Accept paid nothing (" + order + "); the order is yours");
                return;
            }
            Arms.Delivered(c, plan, paid);
            var extra = new JObject { ["kind"] = "gun_order", ["tier"] = o.tier, ["service"] = o.isServiceOrder, ["best"] = o.isBestOrder, ["reward"] = plan.Reward, ["given"] = plan.Given, ["paid"] = paid };
            host.Human(HumanActions.AcceptOffer, advisor.InForce(), m.Uid, extra, "player:gun_order");
            Record("gun_order", m, (plan.Service ? "their gun serviced" : m.Name + " delivered") + " for " + paid + " (given " + plan.Given + " at estimates)" + tried, "player:gun_order", null, v, detail: order);
            object client = v.ClientKey;
            expecting = Expectation.Within("the order is delivered", now, ExpectMs, x => x.DialogueActive || !x.GunOrder || !x.ClientPresent || !ReferenceEquals(x.ClientKey, client));
        }

        // An order that cannot be delivered leaves the gun where it was: in stock (the night clears the counter).
        void GunBack(ActGate gate, GameItem gun, LiveView v)
        {
            if (gun == null || !LiveGame.IsIn(gun, LiveHands.Back)) return;
            if (hands.Move(gate, gun, LiveHands.Stock) && LiveGame.IsIn(gun, LiveHands.Stock)) Record(MoveKinds.Unstage, Planned(gun), "the order is not delivered: back to stock", "player:gun_order", null, v);
            else log.Warn("autopilot: " + LiveGame.Name(gun) + " stays on the back counter");
        }
    }
}
