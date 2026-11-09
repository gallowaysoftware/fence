using System;
using System.Collections.Generic;
using Newtonsoft.Json.Linq;

namespace Fence.Mod
{
    sealed class EventBinder
    {
        readonly Action<string, JObject> sink;
        readonly List<Action> unbinds;

        public string Kind { get; }

        public EventBinder(string kind, Action<string, JObject> sink, List<Action> unbinds)
        {
            Kind = kind;
            this.sink = sink;
            this.unbinds = unbinds;
        }

        public void Fire() => sink(Kind, null);
        public void Fire(string key, string value) => sink(Kind, new JObject { [key] = value });
        public void Fire(string k1, string v1, string k2, string v2) => sink(Kind, new JObject { [k1] = v1, [k2] = v2 });

        public void OnUnbind(Action unbind) => unbinds.Add(unbind);
    }

    // One method per ModHook event in compat/hooks.json, keyed by hook id. Each is invoked in its
    // own try/catch, so an event an update removed costs only that event (Fence.Tests checks this
    // table against hooks.json).
    static class HookBindings
    {
        public static readonly Dictionary<string, Action<EventBinder>> Events = new Dictionary<string, Action<EventBinder>>
        {
            ["ModHook.OnGameLoadedLate"] = GameLoadedLate,
            ["ModHook.OnShutterOpenedLate"] = ShutterOpenedLate,
            ["ModHook.OnShutterClosedLate"] = ShutterClosedLate,
            ["ModHook.OnGoingSleepLate"] = GoingSleepLate,
            ["ModHook.OnWakingUpLate"] = WakingUpLate,
            ["ModHook.OnLeavingStoreLate"] = LeavingStoreLate,
            ["ModHook.OnReturningStoreLate"] = ReturningStoreLate,
            ["ModHook.OnHandlingNightlyServicesLate"] = HandlingNightlyServicesLate,
            ["ModHook.OnGenerateCustomerVeryLate"] = GenerateCustomerVeryLate,
            ["ModHook.OnNewGameStarted"] = NewGameStarted,
            ["ModHook.OnDifficultyApplied"] = DifficultyApplied,
            ["ModHook.OnNetworkUpgradeUnlocked"] = NetworkUpgradeUnlocked,
            ["ModHook.OnAfterhourArriveLate"] = AfterhourArriveLate,
            ["ModHook.OnAfterhourLeaveLate"] = AfterhourLeaveLate,
            ["ModHook.OnAfterhourEventStarted"] = AfterhourEventStarted,
            ["ModHook.OnAfterhourEventEnded"] = AfterhourEventEnded,
            ["ModHook.OnPaperOpened"] = PaperOpened,
        };

        static void GameLoadedLate(EventBinder b)
        {
            Action h = b.Fire;
            ModHook.OnGameLoadedLate += h;
            b.OnUnbind(() => ModHook.OnGameLoadedLate -= h);
        }

        static void ShutterOpenedLate(EventBinder b)
        {
            Action h = b.Fire;
            ModHook.OnShutterOpenedLate += h;
            b.OnUnbind(() => ModHook.OnShutterOpenedLate -= h);
        }

        static void ShutterClosedLate(EventBinder b)
        {
            Action h = b.Fire;
            ModHook.OnShutterClosedLate += h;
            b.OnUnbind(() => ModHook.OnShutterClosedLate -= h);
        }

        static void GoingSleepLate(EventBinder b)
        {
            Action h = b.Fire;
            ModHook.OnGoingSleepLate += h;
            b.OnUnbind(() => ModHook.OnGoingSleepLate -= h);
        }

        static void WakingUpLate(EventBinder b)
        {
            Action h = b.Fire;
            ModHook.OnWakingUpLate += h;
            b.OnUnbind(() => ModHook.OnWakingUpLate -= h);
        }

        static void LeavingStoreLate(EventBinder b)
        {
            Action h = b.Fire;
            ModHook.OnLeavingStoreLate += h;
            b.OnUnbind(() => ModHook.OnLeavingStoreLate -= h);
        }

        static void ReturningStoreLate(EventBinder b)
        {
            Action h = b.Fire;
            ModHook.OnReturningStoreLate += h;
            b.OnUnbind(() => ModHook.OnReturningStoreLate -= h);
        }

        static void HandlingNightlyServicesLate(EventBinder b)
        {
            Action h = b.Fire;
            ModHook.OnHandlingNightlyServicesLate += h;
            b.OnUnbind(() => ModHook.OnHandlingNightlyServicesLate -= h);
        }

        static void GenerateCustomerVeryLate(EventBinder b)
        {
            Action<StoreClientManager> h = _ => b.Fire();
            ModHook.OnGenerateCustomerVeryLate += h;
            b.OnUnbind(() => ModHook.OnGenerateCustomerVeryLate -= h);
        }

        static void NewGameStarted(EventBinder b)
        {
            Action<string> h = id => b.Fire("scenario_id", id);
            ModHook.OnNewGameStarted += h;
            b.OnUnbind(() => ModHook.OnNewGameStarted -= h);
        }

        static void DifficultyApplied(EventBinder b)
        {
            Action<string> h = id => b.Fire("difficulty_id", id);
            ModHook.OnDifficultyApplied += h;
            b.OnUnbind(() => ModHook.OnDifficultyApplied -= h);
        }

        static void NetworkUpgradeUnlocked(EventBinder b)
        {
            Action<string> h = id => b.Fire("upgrade_id", id);
            ModHook.OnNetworkUpgradeUnlocked += h;
            b.OnUnbind(() => ModHook.OnNetworkUpgradeUnlocked -= h);
        }

        static void AfterhourArriveLate(EventBinder b)
        {
            Action<string> h = id => b.Fire("location_id", id);
            ModHook.OnAfterhourArriveLate += h;
            b.OnUnbind(() => ModHook.OnAfterhourArriveLate -= h);
        }

        static void AfterhourLeaveLate(EventBinder b)
        {
            Action<string> h = id => b.Fire("location_id", id);
            ModHook.OnAfterhourLeaveLate += h;
            b.OnUnbind(() => ModHook.OnAfterhourLeaveLate -= h);
        }

        static void AfterhourEventStarted(EventBinder b)
        {
            Action<string, string> h = (loc, ev) => b.Fire("location_id", loc, "event_id", ev);
            ModHook.OnAfterhourEventStarted += h;
            b.OnUnbind(() => ModHook.OnAfterhourEventStarted -= h);
        }

        static void AfterhourEventEnded(EventBinder b)
        {
            Action<string, string> h = (loc, ev) => b.Fire("location_id", loc, "event_id", ev);
            ModHook.OnAfterhourEventEnded += h;
            b.OnUnbind(() => ModHook.OnAfterhourEventEnded -= h);
        }

        // Only the book id: PaperUI is a live UI object, not state.
        static void PaperOpened(EventBinder b)
        {
            Action<string, PaperUI> h = (id, _) => b.Fire("book_id", id);
            ModHook.OnPaperOpened += h;
            b.OnUnbind(() => ModHook.OnPaperOpened -= h);
        }
    }
}
