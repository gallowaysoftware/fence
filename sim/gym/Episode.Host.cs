using System;
using System.Collections.Generic;
using System.Linq;
using Fence.Recording;
using Newtonsoft.Json.Linq;

namespace Fence.Gym
{
    // The headless half of IExecutorHost. Reflection lives here, never in the executor's shared files.
    internal sealed partial class Episode : IExecutorHost
    {
        int IExecutorHost.Uid(GameItem item) => Mod.Game.Uid(item);

        void IExecutorHost.Human(string action, AdviceInForce advice, int? itemUid, JObject extra, string defaultKey) =>
            Mod.Human(action, advice, itemUid, extra, defaultKey);

        void IExecutorHost.Error(string where, Exception e) => GameError(where, e);

        // The call's effects as its dialogue applies them (PhoneDialogList's intro end action, or the order choice's).
        void IExecutorHost.PhoneOrder(StorePhoneClient client, long number, int choice, string courier, int days)
        {
            if (choice < 0) client.phoneState = StorePhoneClient.PhoneState.Regular;
            else client.stockType = choice;
            client.UseService();
            PlayerStore.Instance.QueueFuturClient(courier, days);
        }

        Action IExecutorHost.MachineButton(GameItem machine, string label)
        {
            var button = Buttons(machine).FirstOrDefault(b => b.text == label);
            if (button == null) return null;
            return () => (Callback?.GetValue(button) as Action)?.Invoke();
        }

        // The machine window's buttons, found the way a click finds them: by walking the window's elements.
        static List<MachineryButton> Buttons(GameItem machine)
        {
            var found = new List<MachineryButton>();
            var seen = new HashSet<object>(ReferenceEqualityComparer.Instance);
            void Walk(object o, int depth)
            {
                if (o == null || depth > 8 || o is string || o.GetType().IsPrimitive || !seen.Add(o)) return;
                if (o is MachineryButton b) { found.Add(b); return; }
                if (o is GameItem || o is GameInventory || o is UnityEngine.Object || o is Delegate) return;
                try
                {
                    if (o is System.Collections.IEnumerable en) { foreach (var x in en) Walk(x, depth + 1); return; }
                    for (var t = o.GetType(); t != null && t != typeof(object); t = t.BaseType)
                        foreach (var f in t.GetFields(System.Reflection.BindingFlags.Instance | System.Reflection.BindingFlags.Public | System.Reflection.BindingFlags.NonPublic | System.Reflection.BindingFlags.DeclaredOnly))
                            if (!f.FieldType.IsValueType) Walk(f.GetValue(o), depth + 1);
                }
                catch (Exception) { }
            }
            Walk(machine.contentWindow, 0);
            return found;
        }

        static readonly System.Reflection.FieldInfo Callback = typeof(MachineryButton).GetField("callback", System.Reflection.BindingFlags.Instance | System.Reflection.BindingFlags.NonPublic);
    }
}
