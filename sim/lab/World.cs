using System;
using System.Collections.Generic;
using System.Linq;
using System.Reflection;
using Fence.Gym;
using Fence.Sim;

namespace Fence.Lab
{
    // A controlled game state on top of the gym's headless game: a real new game, then direct edits
    // (cash, rep, day, items, flags) and direct calls into the game's own methods. Every experiment
    // runs in a fresh process, so the game's statics never leak between experiments.
    internal static class W
    {
        public static Episode Ep;
        public static int Seed;
        public static PlayerStore PS => PlayerStore.Instance;
        public static EmporiumEntry E => EmporiumEntry.Instance;

        public static void New(int seed, string difficulty = "normal", bool skipIntro = true, string start = "none")
        {
            Seed = seed;
            var o = new GymOptions { Seed = seed, Difficulty = difficulty, SkipIntro = skipIntro, Start = start, Days = 100000 };
            Ep = new Episode(o, new LocalBridge(seed), null);
            Ep.Step(); // new game + first morning
        }

        public static int Day
        {
            get => StoreStation.Instance.dayCounter;
            set => StoreStation.Instance.dayCounter = value;
        }

        public static int Cash
        {
            get => PS.playerCash;
            set => PS.playerCash = value;
        }

        public static StoreReputation RepOf(string f) => StoreReputation.GetStoreReputation(f);
        public static void SetRep(string f, double v) => RepOf(f).SetReputation(v);
        public static double Rep(string f) => RepOf(f).GetReputationExact();

        public static void ResetReps()
        {
            foreach (var r in PS.storeReputations) r.SetReputation(0);
        }

        public static GameItem Make(string id) => DirectoryMaster.Create<GameItem>(id);

        // A new owned item placed in an inventory (stock by default). Footprints are the real ones, so a
        // full stock grows: the lab measures mechanisms, not space (the gym measures space).
        public static GameItem Own(string id, GameGridInventory where = null)
        {
            var it = Make(id);
            GeneralHelper.SetItemOwned(it, true);
            var inv = where ?? E.invElement;
            for (int grow = 0; GraphUtils.TryAcceptAll(inv, it, -1) != 0; grow++)
            {
                if (inv != E.invElement || grow == 4) throw new InvalidOperationException("could not place " + id);
                inv.SetShape(inv.inventoryShape.width, inv.inventoryShape.height * 2);
            }
            return it;
        }

        public static bool Place(GameGridInventory to, GameItem it) => GraphUtils.TryAcceptAll(to, it, -1) == 0;

        public static void ClearInventory(GameGridInventory inv)
        {
            foreach (var it in inv.childItems.ToList()) inv.Expel(it);
        }

        public static StoreClient NewClient(string id)
        {
            var c = StoreClientListDict.storeClientDict[id]();
            c.spriteName ??= "";
            c.possibleSprites ??= new List<string>();
            return c;
        }

        // Puts a client at the counter without its dialogue: the bargain panel tracks it from here.
        public static StoreClient Seat(string id, Action<StoreClient> edit = null)
        {
            var c = NewClient(id);
            edit?.Invoke(c);
            PS.currentClientInstance = StoreClientInstance.CreateClientInstance(c);
            PS.isClientArrived = true;
            return c;
        }

        public static void Unseat()
        {
            try { DialogUIManager.Instance.EndConversation(); } catch { }
            PS.currentClientInstance = null;
            PS.isClientArrived = false;
            Scheduler.Reset();
        }

        public static T Get<T>(object o, string field)
        {
            for (var t = o.GetType(); t != null; t = t.BaseType)
            {
                var f = t.GetField(field, BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic);
                if (f != null) return (T)f.GetValue(o);
            }
            throw new MissingFieldException(o.GetType().Name, field);
        }

        public static void Set(object o, string field, object v)
        {
            for (var t = o.GetType(); t != null; t = t.BaseType)
            {
                var f = t.GetField(field, BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic);
                if (f != null) { f.SetValue(o, v); return; }
            }
            throw new MissingFieldException(o.GetType().Name, field);
        }

        public static object Call(object o, string method, params object[] args)
        {
            var t = o as Type ?? o.GetType();
            var m = t.GetMethods(BindingFlags.Instance | BindingFlags.Static | BindingFlags.Public | BindingFlags.NonPublic)
                .First(x => x.Name == method && x.GetParameters().Length == args.Length);
            try { return m.Invoke(o is Type ? null : o, args); }
            catch (TargetInvocationException e) when (e.InnerException != null)
            {
                System.Runtime.ExceptionServices.ExceptionDispatchInfo.Capture(e.InnerException).Throw();
                throw;
            }
        }

        // ---- days ---------------------------------------------------------------------------

        // Opens the shutter (the day's queue is generated here) and returns the queued client ids.
        public static List<StoreClient> Open()
        {
            StoreShutterButton.Instance.OpenShutter();
            Scheduler.Settle();
            return PS.storeClientManager.clientStack.ToList();
        }

        // Closes without serving anyone, then sleeps through the game's own end-of-day callback.
        public static void CloseAndSleep(bool clearQueue = true)
        {
            if (clearQueue) PS.storeClientManager.clientStack.Clear();
            if (PS.currentClientInstance != null) PS.DismissCurrentClient(false, true, true);
            StoreShutterButton.Instance.EndAndClose();
            Scheduler.Settle();
            Sleep();
        }

        // Headless, every item's render handler is an unattached fake that compares equal to null, so
        // GameItemElement.IsDestroyed() is true and ChemicalHelper.RunNightlyChemicalSimulation skips every
        // container. Marking the handlers alive restores the night chemistry (tablets, heat, acid/base).
        public static bool NightChemistry = true;

        public static void MarkHandlersAlive()
        {
            foreach (var it in E.GetAllItems())
                if (it is GameItemElement el && !ReferenceEquals(el.handler, null)) Game.MarkAlive(el.handler);
        }

        public static void Sleep()
        {
            if (NightChemistry) MarkHandlersAlive();
            foreach (var s in PS.storeService.Where(s => s.unlocked && s.isMandatory)) s.isUsing = true;
            // No nightly burglary: insurance on (experiments must not lose the items they watch).
            var ins = StoreService.GetInsuranceService();
            if (NoBurglary && ins != null) { ins.unlocked = true; ins.isUsing = true; PS.playerCash = Math.Max(PS.playerCash, ins.cost + 1000); }
            Call(typeof(Episode), "SleepCallback");
            Scheduler.Settle();
            PS.nightLogs?.Clear();
        }

        // A quiet day: open, drop the queue, close, sleep. Returns the queue that was generated.
        public static List<StoreClient> QuietDay()
        {
            var q = Open();
            CloseAndSleep();
            return q;
        }

        public static bool NoBurglary = true;

        public static bool Over => Episode.GameOverReason() != null;
    }
}
