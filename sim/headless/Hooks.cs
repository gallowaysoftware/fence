using System;
using System.Collections.Generic;

namespace Fence.Sim
{
    // Prefix hooks the playable build injects into chosen game methods (Playable.Hooked): the
    // host's stand-in for the live mod's Harmony patches. `id` is "Type::Method"; `self` is null for
    // static methods. Handlers observe only and must not throw into the game.
    public static class Hooks
    {
        public delegate void Handler(object self, object[] args);

        static readonly Dictionary<string, Handler> handlers = new Dictionary<string, Handler>(StringComparer.Ordinal);
        public static Action<string, Exception> OnError;

        public static void On(string id, Handler h)
        {
            handlers.TryGetValue(id, out var prev);
            handlers[id] = prev + h;
        }

        public static void Clear() => handlers.Clear();

        public static void Enter(string id, object self, object[] args)
        {
            if (!handlers.TryGetValue(id, out var h) || h == null) return;
            try
            {
                h(self, args);
            }
            catch (Exception e)
            {
                OnError?.Invoke(id, e);
            }
        }
    }
}
