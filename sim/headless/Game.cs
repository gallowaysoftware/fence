using System;
using System.Collections.Generic;
using System.Linq;
using System.Reflection;
using System.Runtime.CompilerServices;

namespace Fence.Sim
{
    // The game's registries brought up without a scene.
    public sealed class Game
    {
        public readonly Assembly Asm = typeof(GameItem).Assembly;
        public readonly List<string> Errors = new List<string>();
        public readonly SortedDictionary<string, string> ItemDirectoryOf = new SortedDictionary<string, string>(StringComparer.Ordinal);

        // Playable: scene singletons compare non-null like live objects do, and their editor-wired
        // references are filled one level deeper, so the logic the gym un-stubs can walk them.
        public static bool Playable { get; private set; }

        public Game(bool playable = false)
        {
            Playable = playable;
            FakeScene();
            foreach (var t in Asm.GetTypes().Where(t => !t.IsAbstract && IsDirectory(t)).OrderBy(t => t.FullName, StringComparer.Ordinal))
            {
                try
                {
                    var dir = RuntimeHelpers.GetUninitializedObject(t);
                    var prop = t.GetProperty("factoryDictionary", BindingFlags.Public | BindingFlags.Instance);
                    var dictType = prop.PropertyType;
                    prop.SetValue(dir, Activator.CreateInstance(dictType));
                    var add = typeof(DirectoryMaster).GetMethod("AddDirectory").MakeGenericMethod(DirectoryEntryType(t));
                    if (!(bool)add.Invoke(null, new[] { dir })) continue;
                    t.GetMethod("InitDirectory", BindingFlags.NonPublic | BindingFlags.Instance).Invoke(dir, null);
                    if (dir is ItemDirectory items)
                        foreach (var id in items.factoryDictionary.Keys) ItemDirectoryOf[id] = t.Name;
                }
                catch (Exception e)
                {
                    Errors.Add("directory " + t.Name + ": " + Root(e));
                }
            }
        }

        static object FakeNode() => typeof(TreeNodeRender).GetMethod("__Fake").Invoke(null, null);

        // Singletons the scene would have created. Constructors run (field initializers give the
        // real defaults); render nodes are fakes.
        public static T Fake<T>() where T : class
        {
            var o = (T)Activator.CreateInstance(typeof(T), nonPublic: true);
            FillFakes(o);
            return o;
        }

        // Scene references a component would have had wired in the editor become unattached fakes.
        static void FillFakes(object o) => FillFakes(o, Playable ? 2 : 0);

        static void FillFakes(object o, int deeper)
        {
            for (var t = o.GetType(); t != null && t.Assembly == typeof(GameItem).Assembly; t = t.BaseType)
                foreach (var f in t.GetFields(BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic | BindingFlags.DeclaredOnly))
                {
                    if (f.GetValue(o) != null || f.IsInitOnly) continue;
                    if (f.FieldType == typeof(TreeNodeRender)) f.SetValue(o, FakeNode());
                    else if (typeof(UnityEngine.Object).IsAssignableFrom(f.FieldType) && !f.FieldType.IsAbstract && !f.FieldType.ContainsGenericParameters)
                    {
                        var fake = RuntimeHelpers.GetUninitializedObject(f.FieldType);
                        f.SetValue(o, fake);
                        if (deeper > 0) FillFakes(fake, deeper - 1);
                    }
                    else if (deeper > 0 && Playable && f.FieldType.IsGenericType && f.FieldType.GetGenericTypeDefinition() == typeof(List<>) && !f.FieldType.ContainsGenericParameters)
                        f.SetValue(o, Activator.CreateInstance(f.FieldType));
                }
        }

        static readonly FieldInfo CachedPtr = typeof(UnityEngine.Object).GetField("m_CachedPtr", BindingFlags.Instance | BindingFlags.NonPublic);

        public static void MarkAlive(object o) => CachedPtr.SetValue(o, (IntPtr)1);

        static void FakeScene()
        {
            UISettings.current = Fake<UISettings>();
            RenderHandler.current = Fake<RenderHandler>();
            typeof(NewGameData).GetProperty("Instance").SetValue(null, Fake<NewGameData>());
            foreach (var t in typeof(GameItem).Assembly.GetTypes().Where(t => typeof(UnityEngine.MonoBehaviour).IsAssignableFrom(t) && Headless.IsPresentationName(t.Name) && !t.IsAbstract && !t.ContainsGenericParameters))
            {
                var o = Construct(t);
                FillFakes(o);
                if (Playable) MarkAlive(o);
                SetSingleton(t, o);
            }
            const BindingFlags I = BindingFlags.Instance | BindingFlags.NonPublic | BindingFlags.Public;
            foreach (var t in new[] { typeof(EventTriggerManager), typeof(ItemBehaviourManager), typeof(ModHook), typeof(TableMaster), typeof(TableGroupMaster), typeof(GameMaster) })
            {
                var o = Construct(t);
                if (Playable && o is UnityEngine.Object) MarkAlive(o);
                t.GetMethod("Awake", I)?.Invoke(o, null);
            }
            typeof(GraphHandler).GetProperty("windowGraphNodeRoots").SetValue(null, new List<GraphNodeWindow>());
        }

        static object Construct(Type t)
        {
            try { return Activator.CreateInstance(t, nonPublic: true); }
            catch { return RuntimeHelpers.GetUninitializedObject(t); }
        }

        static void SetSingleton(Type t, object o)
        {
            const BindingFlags S = BindingFlags.Static | BindingFlags.Public | BindingFlags.NonPublic;
            foreach (var f in t.GetFields(S).Where(f => f.FieldType == t && !f.IsInitOnly && !f.IsLiteral)) f.SetValue(null, o);
            foreach (var name in new[] { "Instance", "current", "instance", "Current" })
            {
                var p = t.GetProperty(name, S);
                if (p != null && p.PropertyType == t && p.SetMethod != null) { p.SetValue(null, o); return; }
                var f = t.GetField(name, S);
                if (f != null && f.FieldType == t && !f.IsInitOnly) { f.SetValue(null, o); return; }
                if (p != null && p.PropertyType == t)
                {
                    var backing = t.GetField("<" + name + ">k__BackingField", S);
                    if (backing != null) { backing.SetValue(null, o); return; }
                }
            }
        }

        static bool IsDirectory(Type t)
        {
            for (var b = t.BaseType; b != null; b = b.BaseType)
                if (b.IsGenericType && b.GetGenericTypeDefinition().Name == "Directory`1") return true;
            return false;
        }

        static Type DirectoryEntryType(Type t)
        {
            for (var b = t.BaseType; b != null; b = b.BaseType)
                if (b.IsGenericType && b.GetGenericTypeDefinition().Name == "Directory`1") return b.GetGenericArguments()[0];
            throw new InvalidOperationException(t.Name);
        }

        public static string Root(Exception e)
        {
            while (e is TargetInvocationException && e.InnerException != null) e = e.InnerException;
            var frame = e.StackTrace?.Split('\n').FirstOrDefault(l => !l.Contains("UnityEngine."))?.Trim();
            return e.GetType().Name + ": " + e.Message + (frame != null ? " @ " + frame : "");
        }

        // EmporiumEntry is the scene root: Start builds the inventories and starts a new game.
        public static void StartEmporium()
        {
            var e = RuntimeHelpers.GetUninitializedObject(typeof(EmporiumEntry));
            if (Playable) MarkAlive(e);
            const BindingFlags I = BindingFlags.Instance | BindingFlags.NonPublic | BindingFlags.Public;
            typeof(EmporiumEntry).GetMethod("Awake", I).Invoke(e, null);
            typeof(EmporiumEntry).GetMethod("Start", I).Invoke(e, null);
        }

        // Feasibility check: buy a scavenger's stock, then sell one item to a merchant, through the
        // game's own funnels (PlayerStore.BuyItem / SellItem) with no UI.
        public static void ProbeTrade()
        {
            var ps = PlayerStore.Instance;
            var e = EmporiumEntry.Instance;
            string Rep(string f) => ps.storeReputations.Find(r => r.factionId == f)?.GetReputationExact().ToString("0.00");
            var scav = StoreClientListDict.storeClientDict["scavGeneral"]();
            scav.spriteName ??= "";
            ps.currentClientInstance = StoreClientInstance.CreateClientInstance(scav);
            for (var d = scav.mainDialogue; d != null; d = d.nextDialogue) { d.startAction?.Invoke(); d.endAction?.Invoke(); }
            var stock = e.frontInvinvElement.childItems.Where(i => !GeneralHelper.IsItemOwned(i)).ToList();
            Console.WriteLine($"scav offers {stock.Count}: " + string.Join(", ", stock.Select(i => i.identifier + "=" + i.GetNegociatedValue())));
            int cash0 = ps.playerCash;
            foreach (var i in stock) ps.BuyItem(i);
            Console.WriteLine($"bought: cash {cash0} -> {ps.playerCash}, LL rep {Rep("FACTION_LOWER_LEVEL")}, owned {stock.Count(GeneralHelper.IsItemOwned)}");
            ps.currentClientInstance = null;

            var merchant = StoreClientListDict.storeClientDict["merchant"]();
            merchant.spriteName ??= "";
            ps.currentClientInstance = StoreClientInstance.CreateClientInstance(merchant);
            var item = stock.OrderByDescending(i => i.GetNegociatedValue()).First();
            GraphUtils.TryAcceptAll(e.backInvinvElement, item);
            int budget0 = merchant.GetBudget();
            cash0 = ps.playerCash;
            Console.WriteLine($"merchant budget {budget0}, buys {merchant.IsClientBuyingThisItem(item, false)}, price on back counter {item.GetNegociatedValue()}");
            ps.SellItem(item);
            Console.WriteLine($"sold {item.identifier}: cash {cash0} -> {ps.playerCash}, budget -> {merchant.GetBudget()}, LL rep {Rep("FACTION_LOWER_LEVEL")}, owned {GeneralHelper.IsItemOwned(item)}");
        }

        public static string Inner(Exception e)
        {
            while (e is TargetInvocationException && e.InnerException != null) e = e.InnerException;
            return e.ToString();
        }

        public GameItem CreateItem(string id)
        {
            return DirectoryMaster.Create<GameItem>(id);
        }

        public int Probe()
        {
            foreach (var e in Errors) Console.WriteLine(e);
            int ok = 0, bad = 0;
            var reasons = new Dictionary<string, int>();
            foreach (var id in ItemDirectoryOf.Keys)
            {
                try
                {
                    var it = CreateItem(id);
                    if (it == null) { bad++; continue; }
                    ok++;
                }
                catch (Exception e)
                {
                    bad++;
                    string r = Root(e);
                    if (!reasons.ContainsKey(r) && Environment.GetEnvironmentVariable("FENCE_TRACE") != null) Console.WriteLine(id + "\n" + Inner(e));
                    reasons[r] = reasons.GetValueOrDefault(r) + 1;
                }
            }
            Console.WriteLine($"items ok={ok} bad={bad} of {ItemDirectoryOf.Count}");
            try { StartEmporium(); Console.WriteLine("Emporium ok"); }
            catch (Exception e) { Console.WriteLine("Emporium: " + Inner(e)); return 1; }
            try { ProbeTrade(); }
            catch (Exception e) { Console.WriteLine("trade: " + Inner(e)); }
            foreach (var kv in reasons.OrderByDescending(k => k.Value).Take(25)) Console.WriteLine($"{kv.Value,4} {kv.Key}");
            return 0;
        }
    }
}
