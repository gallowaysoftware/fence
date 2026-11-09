using System;
using System.Collections;
using System.Collections.Generic;
using System.Reflection;
using System.Runtime.CompilerServices;

namespace Fence.Sim
{
    // The game's coroutines on a virtual clock. In the playable build MonoBehaviour.StartCoroutine
    // hands its IEnumerator here instead of to the engine, so the game's own delays (dialogue
    // lines, end actions, shutter animations, MetaHelper.WaitAndDo) run in the order and with the
    // relative timing the game gave them, but only when the host pumps. Nothing here reads the wall
    // clock: the same pumps give the same order.
    public static class Scheduler
    {
        public const double Frame = 1.0 / 60.0;

        sealed class Task
        {
            public long Id;
            public object Owner;
            public readonly Stack<IEnumerator> Stack = new Stack<IEnumerator>();
            public object Handle;
            public double Due;
            public object WaitingOn;
            public Func<bool> KeepWaiting;
            public bool Done;
        }

        static readonly List<Task> tasks = new List<Task>();
        static readonly Dictionary<object, Task> byHandle = new Dictionary<object, Task>(ReferenceEqualityComparer.Instance);
        static long nextId;

        public static double Now { get; private set; }
        public static long Steps { get; private set; }
        public static int Pending => tasks.Count;
        public static Action<Exception> OnError;

        public static void Reset()
        {
            tasks.Clear();
            byHandle.Clear();
            Now = 0;
            Steps = 0;
            nextId = 0;
        }

        static object NewHandle()
        {
            var t = typeof(UnityEngine.Coroutine);
            return RuntimeHelpers.GetUninitializedObject(t);
        }

        // MonoBehaviour.StartCoroutine(IEnumerator). The first step runs now, as in Unity. Returns the
        // Coroutine handle as object: the patch is imported before the engine assembly can load.
        public static object Start(object owner, IEnumerator routine)
        {
            if (routine == null) return null;
            var task = new Task { Id = nextId++, Owner = owner, Handle = NewHandle(), Due = Now };
            task.Stack.Push(routine);
            byHandle[task.Handle] = task;
            byHandle[routine] = task;
            Step(task);
            if (!task.Done) tasks.Add(task);
            return task.Handle;
        }

        public static void Stop(object owner, object handleOrRoutine)
        {
            if (handleOrRoutine == null || !byHandle.TryGetValue(handleOrRoutine, out var task)) return;
            Finish(task);
        }

        public static void StopAll(object owner)
        {
            foreach (var t in tasks.ToArray())
                if (ReferenceEquals(t.Owner, owner)) Finish(t);
        }

        // An action on the next pump, for host patches that stand in for an engine callback.
        public static void Later(Action action) => Start(null, Once(action, 0f));

        static IEnumerator Once(Action action, float delay)
        {
            if (delay > 0f) yield return new UnityEngine.WaitForSeconds(delay);
            else yield return null;
            action();
        }

        static void Finish(Task t)
        {
            t.Done = true;
            byHandle.Remove(t.Handle);
            foreach (var e in t.Stack) byHandle.Remove(e);
            t.Stack.Clear();
            tasks.Remove(t);
        }

        static readonly FieldInfo WaitSeconds = typeof(UnityEngine.WaitForSeconds).GetField("m_Seconds", BindingFlags.Instance | BindingFlags.NonPublic);

        // Advances one routine until it yields something that waits.
        static void Step(Task t)
        {
            Steps++;
            t.WaitingOn = null;
            t.KeepWaiting = null;
            while (t.Stack.Count > 0)
            {
                var e = t.Stack.Peek();
                bool more;
                try
                {
                    more = e.MoveNext();
                }
                catch (Exception ex)
                {
                    // Unity logs a throwing coroutine and drops it; the rest of the game goes on.
                    OnError?.Invoke(ex);
                    Finish(t);
                    return;
                }
                if (!more)
                {
                    t.Stack.Pop();
                    byHandle.Remove(e);
                    continue;
                }
                object y = e.Current;
                switch (y)
                {
                    case null:
                        t.Due = Now + Frame;
                        return;
                    case UnityEngine.WaitForSeconds w:
                        t.Due = Now + Math.Max(Frame, (float)WaitSeconds.GetValue(w));
                        return;
                    case UnityEngine.WaitForSecondsRealtime r:
                        t.Due = Now + Math.Max(Frame, r.waitTime);
                        return;
                    case UnityEngine.CustomYieldInstruction c:
                        t.Due = Now + Frame;
                        t.KeepWaiting = () => c.keepWaiting;
                        return;
                    case UnityEngine.Coroutine co:
                        t.Due = Now + Frame;
                        t.WaitingOn = co;
                        return;
                    case IEnumerator nested:
                        t.Stack.Push(nested);
                        byHandle[nested] = t;
                        continue;
                    default:
                        t.Due = Now + Frame;
                        return;
                }
            }
            Finish(t);
        }

        static Task Next(double horizon)
        {
            Task best = null;
            foreach (var t in tasks)
            {
                if (t.Due > horizon) continue;
                if (best == null || t.Due < best.Due || (t.Due == best.Due && t.Id < best.Id)) best = t;
            }
            return best;
        }

        // Runs every step due within `seconds` of virtual time, advancing the clock to each. A task
        // waiting on a condition re-checks it every frame. Returns the steps taken.
        public static int Pump(double seconds, int maxSteps = 200000)
        {
            double horizon = Now + seconds;
            int n = 0;
            while (n < maxSteps)
            {
                var t = Next(horizon);
                if (t == null) break;
                if (t.Due > Now) Now = t.Due;
                n++;
                if (t.WaitingOn != null && byHandle.ContainsKey(t.WaitingOn))
                {
                    t.Due = Now + Frame;
                    continue;
                }
                if (t.KeepWaiting != null)
                {
                    bool wait;
                    try { wait = t.KeepWaiting(); } catch { wait = false; }
                    if (wait)
                    {
                        t.Due = Now + Frame;
                        continue;
                    }
                }
                Step(t);
            }
            if (Now < horizon && Next(double.MaxValue) == null) Now = horizon;
            return n;
        }

        // Pumps until nothing is due within `quiet` seconds, or `limit` seconds have passed.
        public static void Settle(double quiet = 3.0, double limit = 60.0)
        {
            double end = Now + limit;
            while (Now < end)
            {
                var t = Next(double.MaxValue);
                if (t == null || t.Due > Now + quiet) return;
                double before = Now;
                Pump(Math.Min(quiet, end - Now));
                if (Now <= before) return;
            }
        }

        // UnityEngine.Time on the virtual clock: one frame per pumped frame step.
        public static float DeltaTime() => (float)Frame;
        public static float TimeSingle() => (float)Now;
        public static double TimeDouble() => Now;
        public static int FrameCount() => (int)(Now / Frame);
        public static float One() => 1f;

        public static IEnumerable<string> Describe()
        {
            foreach (var t in tasks)
                yield return $"#{t.Id} due {t.Due:0.###} {(t.Stack.Count > 0 ? t.Stack.Peek().GetType().FullName : "?")}";
        }
    }
}
