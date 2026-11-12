using System;
using System.Linq;
using Fence.Aug;
using Fence.Autopilot;
using Fence.Barter;
using Fence.Gym;

namespace Fence.Mod
{
    // M3 (docs/STREAM.md section 5): the gun. The executor's aug verdict reads only what a player sees (AugCheck in
    // tells mode, whatever the advice setting says, then the shared AugRule); a sure tell raises the gun and fires,
    // and Security's verdict is read through. Anything else on the way hands over.
    sealed partial class AutopilotDriver
    {
        public const string GunShoot = "shoot";
        public const string GunPause = "pause";
        const long GunUpMs = 8000;
        const long ShotLandsMs = 15000;

        // The autopilot_gun setting and whether the gun's members resolved (autopilot.gun).
        public Func<string> GunSetting = () => GunShoot;
        public bool GunUsable = true;

        object linesClient;
        string linesBefore;
        bool augReadBroken;

        // The customer shot and what the run looked like before it, for Security's verdict.
        sealed class ShotWatch
        {
            public object Client;
            public string Name;
            public int Cash;
            public int Crime;
        }

        ShotWatch shot;

        // The customer at the counter on the visible tells; the planner asks for the gun only when this says aug and
        // the gun can be raised. The lines are the ones after the history's last line when this customer was called
        // (before their walk-in, so before their first line).
        void ReadAugVerdict(LiveView v, BarterSnapshot snap)
        {
            if (!v.ClientPresent || v.Outside || v.ClientKey == null)
            {
                linesClient = null;
                return;
            }
            if (!ReferenceEquals(v.ClientKey, linesClient))
            {
                linesClient = v.ClientKey;
                linesBefore = null;
                try
                {
                    linesBefore = GameAug.Lines(null).LastOrDefault();
                }
                catch (Exception)
                {
                }
            }
            if (augReadBroken || snap?.Client == null || !v.ClientArrived || v.ClientMoving) return;
            try
            {
                GameBarter.EnsureAugLines();
                var q = AugCheck.Build(GameAug.Situation(game, snap.Client, memory.ClientSeq, linesBefore), AugDetection.Tells);
                if (q == null) return;
                var verdict = AugRule.Judge(q.Context);
                if (verdict.Aug) v.AugTells = string.Join("; ", verdict.Tells);
            }
            catch (Exception e)
            {
                augReadBroken = true;
                log.Warn("autopilot: cannot read the aug tells (augs are served from now on): " + e.GetType().Name + ": " + e.Message);
            }
        }

        // After the shot: once the shootout is over and the customer gone, Security has said what they were. A
        // manslaughter (the crime record grew) hands over.
        void CheckShot(LiveView v)
        {
            if (shot == null || v.ShootingMode || v.Shootout || v.BlockedBy != null) return;
            if (v.ClientPresent && ReferenceEquals(v.ClientKey, shot.Client)) return;
            var s = shot;
            shot = null;
            int crime = LiveGame.Crime(), cash = LiveGame.Cash();
            if (crime > s.Crime)
            {
                Stop(AutopilotState.Handover, "Security says " + (s.Name ?? "the customer") + " was human: a manslaughter (crime +" + (crime - s.Crime) + ")");
                return;
            }
            Record("shot_result", null, "Security's verdict on " + (s.Name ?? "the customer") + ": " + (cash > s.Cash ? "an aug, +" + (cash - s.Cash) + " cash" : "no reward, no crime"), "player:aug", null, v);
        }

        bool ExecuteM3(Step step, LiveView v, ActGate gate, long now)
        {
            switch (step.Kind)
            {
                case StepKind.AugCheck:
                    AugCheckStep(gate, v, step.Reason, now);
                    return true;
                case StepKind.Fire:
                {
                    var target = memory.ClientKey;
                    int crime = LiveGame.Crime(), cash = LiveGame.Cash();
                    if (!hands.Fire(gate, target))
                    {
                        Stop(AutopilotState.Handover, "possible aug: the gun is up but the shot cannot be fired; lower it or fire by hand");
                        return true;
                    }
                    memory.Fired = true;
                    shot = new ShotWatch { Client = target, Name = v.ClientName, Cash = cash, Crime = crime };
                    Record("shoot", null, "fired at " + (v.ClientName ?? "the customer"), "player:aug", null, v);
                    expecting = Expectation.Within("the shot lands", now, ShotLandsMs, x => x.Shootout || !x.ShootingMode);
                    return true;
                }
                case StepKind.GunOrder:
                    GunOrder(gate, v, now);
                    return true;
                default:
                    return false;
            }
        }

        void AugCheckStep(ActGate gate, LiveView v, string tells, long now)
        {
            memory.GunDecided = true;
            string why = "aug tells: " + tells;
            if (Directives.Current?.Augs == "serve")
            {
                Record("aug_serve", null, why + "; the plan serves augs", "player:aug", null, v);
                return;
            }
            if (!GunUsable || !string.Equals((Safe(GunSetting) ?? GunShoot).Trim(), GunShoot, StringComparison.OrdinalIgnoreCase))
            {
                Record("aug_handover", null, why, "player:aug", null, v);
                Stop(AutopilotState.Handover, "possible aug (" + tells + ")");
                return;
            }
            if (!hands.RaiseGun(gate))
            {
                Stop(AutopilotState.Handover, "possible aug (" + tells + "): the gun cannot be raised");
                return;
            }
            memory.GunRaised = true;
            object client = v.ClientKey;
            Record("raise_gun", null, why + ". Shoot", "player:aug", null, v);
            expecting = Expectation.Within("the gun comes up", now, GunUpMs, x => x.ShootingMode && (!x.ClientPresent || ReferenceEquals(x.ClientKey, client)));
        }
    }
}
