using System;
using System.Collections.Generic;
using Fence.Recording;
using HarmonyLib;

namespace Fence.Mod
{
    // The gun (compat/hooks.json, features advise.aug / record.aug). Observation only: the shooting and
    // kill methods appear here as patch targets, never as calls; the mod never raises or fires the gun.
    static partial class FencePatches
    {
        internal static AugAdvisor Augs;

        static readonly List<Spec> AugSpecs = new List<Spec>
        {
            new Spec
            {
                HookId = "aug.shooting_enter",
                Target = () => AccessTools.Method(typeof(StoreUIManager), "EnterShootingMode", new Type[0]),
                Prefix = () => AccessTools.Method(typeof(FencePatches), nameof(UnholsterPrefix)),
            },
            new Spec
            {
                HookId = "aug.shooting_exit",
                Target = () => AccessTools.Method(typeof(StoreUIManager), "ExitShootingMode", new Type[0]),
                Prefix = () => AccessTools.Method(typeof(FencePatches), nameof(HolsterPrefix)),
            },
            new Spec
            {
                HookId = "aug.shoot",
                Target = () => AccessTools.Method(typeof(StoreUIManager), "Shoot", new Type[0]),
                Prefix = () => AccessTools.Method(typeof(FencePatches), nameof(ShootPrefix)),
            },
            new Spec
            {
                HookId = "aug.good_kill",
                Target = () => AccessTools.Method(typeof(AugHelper), "OnGoodKill", new Type[0]),
                Prefix = () => AccessTools.Method(typeof(FencePatches), nameof(AugResultPrefix)),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(GoodKillPostfix)),
            },
            new Spec
            {
                HookId = "aug.wrongful_kill",
                Target = () => AccessTools.Method(typeof(AugHelper), "CleanupKill", new Type[0]),
                Prefix = () => AccessTools.Method(typeof(FencePatches), nameof(WrongfulKillPrefix)),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(WrongfulKillPostfix)),
            },
            new Spec
            {
                HookId = "aug.left_alive",
                Target = () => AccessTools.Method(typeof(AugHelper), "OnAugLeftAlive", new Type[0]),
                Prefix = () => AccessTools.Method(typeof(FencePatches), nameof(AugResultPrefix)),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(LeftAlivePostfix)),
            },
        };

        static void Aug(Action<AugAdvisor> a)
        {
            var g = Augs;
            if (g == null) return;
            try
            {
                a(g);
            }
            catch (Exception)
            {
                // Observation only: losing one record is better than breaking the shootout.
            }
        }

        static void UnholsterPrefix()
        {
            Aug(a => a.Unholstered());
            Rec("unholster", r => r.Gun(HumanActions.Unholster, null));
        }

        static void HolsterPrefix()
        {
            bool shot = false;
            Aug(a =>
            {
                shot = GameAug.IsShot();
                a.Holstered(shot);
            });
            if (!shot) Rec("holster", r => r.Gun(HumanActions.Holster, null));
        }

        static void ShootPrefix()
        {
            Newtonsoft.Json.Linq.JObject tells = null;
            Aug(a =>
            {
                a.BeforeShot();
                tells = a.ShotContext();
            });
            Rec("shoot", r => r.Gun(HumanActions.Shoot, tells));
        }

        static void AugResultPrefix() => Aug(a => a.BeforeResult());

        static void GoodKillPostfix() => Aug(a => a.GoodKill());

        static void WrongfulKillPrefix(out string __state)
        {
            string name = null;
            Aug(a =>
            {
                a.BeforeResult();
                name = GameAug.ShotClientName();
            });
            __state = name;
        }

        static void WrongfulKillPostfix(string __state) => Aug(a => a.WrongfulKill(__state));

        static void LeftAlivePostfix() => Aug(a => a.LeftAlive());
    }
}
