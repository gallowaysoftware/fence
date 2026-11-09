using System;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using System.Reflection;
using IODir = System.IO.Directory;
using MethodBody = Mono.Cecil.Cil.MethodBody;
using MethodImplAttributes = Mono.Cecil.MethodImplAttributes;
using FieldAttributes = Mono.Cecil.FieldAttributes;
using MethodAttributes = Mono.Cecil.MethodAttributes;
using Random = System.Random;
using System.Runtime.Loader;
using System.Security.Cryptography;
using Mono.Cecil;
using Mono.Cecil.Cil;

namespace Fence.Sim
{
    // Builds a headless copy of the game's managed assemblies and loads it into this process.
    //
    // Unity's managed layer is a thin shell over native internal calls, which do not exist outside
    // the player. Every body-less method in a Unity/third-party assembly is rewritten to return
    // default, so engine calls become no-ops; a handful of Assembly-CSharp members that need real
    // engine data (sprites, localization) get small replacement bodies; UnityEngine.Random is
    // backed by a seedable System.Random. The patched copies live under var/gamedata/.headless and
    // are derived game binaries: never commit them.
    public enum Variant
    {
        // sim/extract: observation only; coroutines and UI managers stay inert.
        Extract,
        // sim/gym: coroutines run on Scheduler, the counter/dialogue managers' logic is live, hooks report.
        Gym,
    }

    public static class Headless
    {
        public static string Managed;
        public static string OutDir;

        static readonly string[] Keep = { "mscorlib", "netstandard", "System", "Mono.Security" };

        public static Variant Current { get; private set; }

        public static void Build(string gameManaged, string outDir, Variant variant = Variant.Extract)
        {
            Managed = gameManaged;
            OutDir = outDir;
            Current = variant;
            string stamp = Path.Combine(outDir, ".stamp");
            // The patch code itself is part of the key: a rebuilt Fence.Headless rebuilds the copies.
            string want = Sha256(Path.Combine(gameManaged, "Assembly-CSharp.dll")) + "|" + Version + "|" + variant + "|" + Sha256(typeof(Headless).Assembly.Location);
            if (File.Exists(stamp) && File.ReadAllText(stamp) == want) return;

            IODir.CreateDirectory(outDir);
            var resolver = new DefaultAssemblyResolver();
            resolver.AddSearchDirectory(gameManaged);
            foreach (string src in IODir.GetFiles(gameManaged, "*.dll"))
            {
                string name = Path.GetFileNameWithoutExtension(src);
                if (Keep.Contains(name) || name.StartsWith("System.")) continue;
                using var asm = AssemblyDefinition.ReadAssembly(src, new ReaderParameters { AssemblyResolver = resolver });
                bool game = name == "Assembly-CSharp";
                if (!game) StubExterns(asm.MainModule);
                if (UiOnly(name)) StubAll(asm.MainModule, t => true);
                if (name.StartsWith("UnityEngine.") && name.EndsWith("Module")) StubAll(asm.MainModule, IsEngineObject);
                if (name == "UnityEngine.CoreModule")
                {
                    SeedRandom(asm.MainModule);
                    if (variant == Variant.Gym) Playable.PatchCoroutines(asm.MainModule);
                }
                // Persistence and telemetry must never run: a headless game must not touch real saves.
                StubAll(asm.MainModule, t => t.Name.StartsWith("ES3") || t.Name == "SaveFiles" || t.Name == "EmporiumAnalytics" || t.Name.StartsWith("Steam"));
                if (game) PatchGame(asm.MainModule, variant);
                asm.Write(Path.Combine(outDir, name + ".dll"));
            }
            File.WriteAllText(stamp, want);
        }

        // Bump when the patch set changes so stale headless copies are rebuilt.
        const string Version = "25";

        public static string Sha256(string path)
        {
            using var s = File.OpenRead(path);
            return Convert.ToHexString(SHA256.HashData(s)).ToLowerInvariant();
        }

        static void StubExterns(ModuleDefinition m)
        {
            foreach (var t in AllTypes(m))
            {
                foreach (var md in t.Methods)
                {
                    if (md.HasBody || md.IsAbstract || md.IsRuntime) continue;
                    if (t.BaseType != null && (t.BaseType.FullName == "System.MulticastDelegate")) continue;
                    md.IsInternalCall = false;
                    md.PInvokeInfo = null; // the setter re-sets IsPInvokeImpl
                    md.IsPInvokeImpl = false;
                    md.ImplAttributes = MethodImplAttributes.IL | MethodImplAttributes.Managed;
                    md.IsPreserveSig = false;
                    ReturnDefault(md);
                }
            }
        }

        // Presentation-only assemblies: nothing in them feeds the economy.
        static bool UiOnly(string name) =>
            name == "UnityEngine.UI" || name == "Unity.TextMeshPro" || name.StartsWith("DOTween") ||
            name.StartsWith("Febucci.") || name == "DemiLib" || name == "Unity.Localization";

        // Scene objects (components, game objects) have no native peer here. UnityEngine.Object
        // itself keeps its managed equality, so a fake still compares equal to null.
        static bool IsEngineObject(TypeDefinition t)
        {
            if (t.FullName == "UnityEngine.Object") return false;
            for (var b = t.BaseType; b != null; )
            {
                if (b.FullName == "UnityEngine.Object") return true;
                TypeDefinition d;
                try { d = b.Resolve(); } catch { return false; }
                if (d == null) return false;
                b = d.BaseType;
            }
            return false;
        }

        public static readonly string[] PresentationNames =
        {
            "AudioManager", "EmporiumAudioPlayer", "StoreSoundPlayer", "LetterboxManager", "WalkerManager", "FontLoader",
            "IconProvider", "SpriteDict", "DebugPanel", "OverlayHandler", "WindowsHandler", "StorePoster", "InputActionManager",
            "LogWriter", "ItemMouseDragHandler", "ItemMultiSelectHandler", "ItemSelectHandler", "StoreClientMono",
            "EmporiumRTSController", "DebugOwnershipToggleMode", "PaperEditor", "InteractableUI", "Interactable",
            "AchievementTracker", "StoreScale", "StoreSecButton", "StoreShutterButton", "DebugHandler", "DialogueHandler",
            "QuickChooseHandler", "RecipeSelectHandler", "SelectableElementHandler", "StoreVendingFountain",
            "StoreVendingMachine", "SteamManager", "ItemQuickTransferAltHandler1", "EmporiumOption",
        };

        public static bool IsPresentationName(string n) =>
            n.EndsWith("UIManager") || n.EndsWith("UIController") || (n.StartsWith("Item") && n.EndsWith("Handler")) || n.StartsWith("Store") || PresentationNames.Contains(n);

        static bool IsPresentationBehaviour(TypeDefinition t)
        {
            if (!IsPresentationName(t.Name)) return false;
            for (var b = t.BaseType; b != null; )
            {
                if (b.FullName == "UnityEngine.MonoBehaviour") return true;
                TypeDefinition d;
                try { d = b.Resolve(); } catch { return false; }
                if (d == null) return false;
                b = d.BaseType;
            }
            return false;
        }

        static void StubAll(ModuleDefinition m, Func<TypeDefinition, bool> which)
        {
            foreach (var t in AllTypes(m))
            {
                if (!which(t) || t.BaseType?.FullName == "System.MulticastDelegate") continue;
                foreach (var md in t.Methods)
                    if (md.HasBody && !md.IsConstructor) ReturnDefault(md);
            }
        }

        internal static IEnumerable<TypeDefinition> AllTypes(ModuleDefinition m)
        {
            var stack = new Stack<TypeDefinition>(m.Types);
            while (stack.Count > 0)
            {
                var t = stack.Pop();
                yield return t;
                foreach (var n in t.NestedTypes) stack.Push(n);
            }
        }

        internal static void ReturnDefault(MethodDefinition md)
        {
            md.Body = new MethodBody(md) { InitLocals = true };
            var il = md.Body.GetILProcessor();
            foreach (var p in md.Parameters)
            {
                if (!p.ParameterType.IsByReference || !p.IsOut) continue;
                var elem = ((ByReferenceType)p.ParameterType).ElementType;
                il.Emit(OpCodes.Ldarg, p);
                il.Emit(OpCodes.Initobj, elem);
            }
            var rt = md.ReturnType;
            if (FakeableEngineObject(rt))
            {
                var m = md.Module;
                il.Emit(OpCodes.Ldtoken, m.ImportReference(rt));
                il.Emit(OpCodes.Call, m.ImportReference(typeof(Type).GetMethod("GetTypeFromHandle")));
                il.Emit(OpCodes.Call, m.ImportReference(typeof(System.Runtime.CompilerServices.RuntimeHelpers).GetMethod("GetUninitializedObject")));
                il.Emit(OpCodes.Castclass, m.ImportReference(rt));
            }
            else if (rt.MetadataType != MetadataType.Void)
            {
                var v = new VariableDefinition(rt);
                md.Body.Variables.Add(v);
                il.Emit(OpCodes.Ldloc, v);
            }
            il.Emit(OpCodes.Ret);
        }

        // A stubbed `x.gameObject` returns an unattached GameObject rather than null, so chains like
        // `x.gameObject.name = …` stay no-ops; the fake still compares equal to null.
        static bool FakeableEngineObject(TypeReference rt)
        {
            if (rt.IsGenericParameter || rt.IsArray || rt.IsByReference || rt.IsValueType || rt.FullName == "UnityEngine.Object") return false;
            TypeDefinition d;
            try { d = rt.Resolve(); } catch { return false; }
            if (d == null || d.IsAbstract || d.IsInterface || d.HasGenericParameters) return false;
            return IsEngineObject(d);
        }

        // UnityEngine.Random is native; it becomes HeadlessRandom (seedable, state save/restore).
        static void SeedRandom(ModuleDefinition m)
        {
            var random = m.GetType("UnityEngine.Random");
            MethodReference H(string name) => m.ImportReference(typeof(HeadlessRandom).GetMethod(name));
            foreach (var md in random.Methods.Where(x => x.Name == "Range" && x.Parameters.Count == 2))
            {
                bool ints = md.Parameters[0].ParameterType.MetadataType == MetadataType.Int32;
                Replace(md, il =>
                {
                    il.Emit(OpCodes.Ldarg_0);
                    il.Emit(OpCodes.Ldarg_1);
                    il.Emit(OpCodes.Call, H(ints ? nameof(HeadlessRandom.RangeInt) : nameof(HeadlessRandom.RangeFloat)));
                    il.Emit(OpCodes.Ret);
                });
            }
            var getValue = random.Methods.FirstOrDefault(x => x.Name == "get_value");
            if (getValue != null) Replace(getValue, il => { il.Emit(OpCodes.Call, H(nameof(HeadlessRandom.Value))); il.Emit(OpCodes.Ret); });

            var state = random.NestedTypes.Single(t => t.Name == "State");
            var s0 = state.Fields.Single(f => f.Name == "s0");
            s0.IsPrivate = false;
            s0.IsAssembly = true;
            var getState = random.Methods.Single(x => x.Name == "get_state");
            Replace(getState, il =>
            {
                var v = new VariableDefinition(state);
                getState.Body.Variables.Add(v);
                il.Emit(OpCodes.Ldloca_S, v);
                il.Emit(OpCodes.Initobj, state);
                il.Emit(OpCodes.Ldloca_S, v);
                il.Emit(OpCodes.Call, H(nameof(HeadlessRandom.SaveState)));
                il.Emit(OpCodes.Stfld, s0);
                il.Emit(OpCodes.Ldloc, v);
                il.Emit(OpCodes.Ret);
            });
            var setState = random.Methods.Single(x => x.Name == "set_state");
            Replace(setState, il =>
            {
                il.Emit(OpCodes.Ldarga_S, setState.Parameters[0]);
                il.Emit(OpCodes.Ldfld, s0);
                il.Emit(OpCodes.Call, H(nameof(HeadlessRandom.RestoreState)));
                il.Emit(OpCodes.Ret);
            });
        }

        internal static MethodDefinition Method(ModuleDefinition m, string type, string name, int argc = -1)
        {
            var t = m.GetType(type) ?? throw new InvalidOperationException("missing type " + type);
            return t.Methods.FirstOrDefault(x => x.Name == name && (argc < 0 || x.Parameters.Count == argc))
                ?? throw new InvalidOperationException("missing " + type + "." + name);
        }

        static void PatchGame(ModuleDefinition m, Variant variant)
        {
            FakeRenderNodes(m);
            foreach (var name in new[] { "HandleLocale", "PreLoadLocalizationTable", "SetAspectRatio", "UpdateScaleFactor" })
                foreach (var md in m.GetType("EmporiumEntry").Methods.Where(x => x.Name == name && x.HasBody)) ReturnDefault(md);
            var ps = m.GetType("PlayerStore");
            var over = new FieldDefinition("__gameOver", FieldAttributes.Public | FieldAttributes.Static, m.TypeSystem.String);
            ps.Fields.Add(over);
            var exec = ps.Methods.Single(x => x.Name == "ExecuteGameOver");
            var first = exec.Body.Instructions[0];
            var ilx = exec.Body.GetILProcessor();
            ilx.InsertBefore(first, ilx.Create(OpCodes.Ldarg_1));
            ilx.InsertBefore(first, ilx.Create(OpCodes.Stsfld, over));
            foreach (var name in new[] { "SaveGame", "InitialSave", "WriteSlotFile" })
                foreach (var md in m.GetType("PlayerStore").Methods.Where(x => x.Name == name && x.HasBody)) ReturnDefault(md);

            // The game's own render layer: anything implementing PixelElement only draws.
            var drawing = new[] { "Validate", "Toggle", "Refresh", "Render", "Resize", "ToFront", "ToBack", "FixBounds", "Update" };
            foreach (var t in AllTypes(m).Where(t => t.Interfaces.Any(i => i.InterfaceType.Name == "PixelElement") || t.Name == "GameItemElement"))
                foreach (var md in t.Methods.Where(x => x.HasBody && !x.IsConstructor && !x.IsStatic && drawing.Any(d => x.Name.StartsWith(d))))
                    ReturnDefault(md);

            // A singleton accessor reads its own static field, so the extractor's instance is what callers see.
            foreach (var t in AllTypes(m).Where(IsPresentationBehaviour))
                foreach (var md in t.Methods.Where(x => x.HasBody && !x.IsConstructor))
                {
                    var own = md.IsStatic && md.IsGetter && md.ReturnType.FullName == t.FullName
                        ? t.Fields.FirstOrDefault(f => f.IsStatic && f.FieldType.FullName == t.FullName) : null;
                    if (own != null) Replace(md, il => { il.Emit(OpCodes.Ldsfld, own); il.Emit(OpCodes.Ret); });
                    else if (variant == Variant.Gym && Playable.KeepsReal(t, md)) continue;
                    else if (!(md.IsStatic && md.IsSetter)) ReturnDefault(md);
                }
            var fluent = new[] { "With", "Set", "Add" };
            foreach (var t in AllTypes(m).Where(t => t.Name.StartsWith("Machinery") || t.Name == "PixelWindow" || t.Name.StartsWith("GameSlotInventory") || t.Name == "GameGridInventory"))
                foreach (var md in t.Methods.Where(x => x.HasBody && !x.IsConstructor && !x.IsStatic))
                {
                    bool draws = new[] { "Hide", "Show", "SetMachineryStyle", "HideBorder", "ShowBorder", "WithInfoLink", "ApplyStyle" }.Any(d => md.Name.StartsWith(d));
                    if (!draws && !t.Name.StartsWith("Machinery")) continue;
                    if (t.Name.StartsWith("Machinery") && !draws && !fluent.Any(f => md.Name.StartsWith(f))) continue;
                    if (md.ReturnType.FullName == t.FullName) Replace(md, il => { il.Emit(OpCodes.Ldarg_0); il.Emit(OpCodes.Ret); });
                    else if (md.ReturnType.MetadataType == MetadataType.Void) ReturnDefault(md);
                }

            var shape = Method(m, "GameItem", "BuildShapeFromSprite", 2);
            var shapeCtor = m.GetType("GridShapeBuilder").Methods.Single(x => x.IsConstructor && x.Parameters.Count == 2
                && x.Parameters.All(p => p.ParameterType.MetadataType == MetadataType.Int32));
            Replace(shape, il =>
            {
                il.Emit(OpCodes.Ldc_I4_1);
                il.Emit(OpCodes.Ldc_I4_1);
                il.Emit(OpCodes.Newobj, shapeCtor);
                il.Emit(OpCodes.Ret);
            });

            // Localization returns the key: ids are what the extractor wants anyway.
            Replace(Method(m, "LocHelper", "Get", 3), il => { il.Emit(OpCodes.Ldarg_1); il.Emit(OpCodes.Ret); });
            Replace(Method(m, "LocHelper", "HasEntry", 2), il => { il.Emit(OpCodes.Ldc_I4_0); il.Emit(OpCodes.Ret); });

            // Sprites are unattached objects too: rect and friends read back as zero.
            var sprite = m.ImportReference(m.GetType("RenderHandler").Methods.First(x => x.Name == "LoadFromAtlas").ReturnType);
            var uninit = m.ImportReference(typeof(System.Runtime.CompilerServices.RuntimeHelpers).GetMethod("GetUninitializedObject"));
            var fromHandle = m.ImportReference(typeof(Type).GetMethod("GetTypeFromHandle"));
            foreach (var md in m.GetType("RenderHandler").Methods.Where(x => x.IsStatic && x.HasBody && x.ReturnType.FullName == "UnityEngine.Sprite"))
                Replace(md, il =>
                {
                    il.Emit(OpCodes.Ldtoken, sprite);
                    il.Emit(OpCodes.Call, fromHandle);
                    il.Emit(OpCodes.Call, uninit);
                    il.Emit(OpCodes.Castclass, sprite);
                    il.Emit(OpCodes.Ret);
                });

            if (variant == Variant.Gym) Playable.PatchGame(m);
        }

        internal static void Replace(MethodDefinition md, Action<ILProcessor> emit)
        {
            md.Body = new MethodBody(md) { InitLocals = true };
            emit(md.Body.GetILProcessor());
        }

        // Every render node is an unattached component object: Unity setters on it hit the stubbed
        // externs and do nothing, and UI code that checks `this == null` bails out early.
        static void FakeRenderNodes(ModuleDefinition m)
        {
            var node = m.GetType("TreeNodeRender");
            var uninit = m.ImportReference(typeof(System.Runtime.CompilerServices.RuntimeHelpers).GetMethod("GetUninitializedObject"));
            var fromHandle = m.ImportReference(typeof(Type).GetMethod("GetTypeFromHandle"));
            var fake = new MethodDefinition("__Fake", MethodAttributes.Public | MethodAttributes.Static | MethodAttributes.HideBySig, node);
            node.Methods.Add(fake);
            var il = fake.Body.GetILProcessor();
            void New(TypeReference t)
            {
                il.Emit(OpCodes.Ldtoken, t);
                il.Emit(OpCodes.Call, fromHandle);
                il.Emit(OpCodes.Call, uninit);
                il.Emit(OpCodes.Castclass, t);
            }
            New(node);
            foreach (var prop in new[] { "rectTransform", "image", "mask", "text" })
            {
                var setter = node.Properties.Single(p => p.Name == prop).SetMethod;
                il.Emit(OpCodes.Dup);
                New(setter.Parameters[0].ParameterType);
                il.Emit(OpCodes.Call, setter);
            }
            il.Emit(OpCodes.Ret);

            foreach (var md in node.Methods.Where(x => x.IsStatic && x != fake && x.Name.StartsWith("Instantiate") && x.ReturnType.FullName == "TreeNodeRender").ToList())
                Replace(md, p => { p.Emit(OpCodes.Call, fake); p.Emit(OpCodes.Ret); });
        }

        public static void Install()
        {
            AssemblyLoadContext.Default.Resolving += (ctx, name) =>
            {
                string p = Path.Combine(OutDir, name.Name + ".dll");
                if (File.Exists(p)) return ctx.LoadFromAssemblyPath(p);
                p = Path.Combine(Managed, name.Name + ".dll");
                return File.Exists(p) ? ctx.LoadFromAssemblyPath(p) : null;
            };
        }

        public static void Seed(int seed)
        {
            HeadlessRandom.Seed(seed);
            global::RNG.random = new Random(seed ^ 0x5eed);
        }
    }
}
