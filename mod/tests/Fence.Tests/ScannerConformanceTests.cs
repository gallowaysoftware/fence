using System;
using System.Collections;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using System.Reflection;
using System.Runtime.Loader;
using Mono.Cecil;
using Mono.Cecil.Cil;
using Xunit;
using Xunit.Abstractions;

namespace Fence.Tests
{
    // Runs the game's own ModScanner (and the loader's manifest checks) against the mod folder
    // exactly as install.sh lays it out. Our early warning when an update tightens mod rules:
    // it fails on anything the loader would refuse today AND on the "pending" findings that the
    // December 10, 2026 update turns into refusals.
    public class ScannerConformanceTests
    {
        readonly ITestOutputHelper output;

        public ScannerConformanceTests(ITestOutputHelper output)
        {
            this.output = output;
        }

        sealed class GameContext : AssemblyLoadContext
        {
            readonly string managed;

            public GameContext(string managed) : base("probably-stolen", isCollectible: false)
            {
                this.managed = managed;
            }

            protected override Assembly Load(AssemblyName name)
            {
                // The BCL comes from the test host; Unity's Mono copies of it do not load on CoreCLR.
                if (name.Name == "mscorlib" || name.Name == "netstandard" || name.Name == "System" || name.Name!.StartsWith("System.")) return null;
                string p = Path.Combine(managed, name.Name + ".dll");
                return File.Exists(p) ? LoadFromAssemblyPath(p) : null;
            }
        }

        static readonly Lazy<Assembly> Game = new Lazy<Assembly>(() =>
        {
            var ctx = new GameContext(Paths.GameManaged);
            var asm = ctx.LoadFromAssemblyPath(Paths.AssemblyCSharp);
            // In the game ModLoader.Awake fills this from the Managed folder; it needs Unity there.
            var names = new HashSet<string>(Directory.GetFiles(Paths.GameManaged, "*.dll").Select(Path.GetFileNameWithoutExtension), StringComparer.OrdinalIgnoreCase);
            asm.GetType("ModLoader")!.GetField("gameAssemblyNames", BindingFlags.NonPublic | BindingFlags.Static)!.SetValue(null, names);
            return asm;
        });

        static string InstalledLayout()
        {
            string dir = Path.Combine(Path.GetTempPath(), "fence-scan-" + Guid.NewGuid().ToString("N"), "fence");
            Directory.CreateDirectory(dir);
            File.Copy(Paths.ManifestXml, Path.Combine(dir, "manifest.xml"));
            foreach (var dll in Paths.ShippedDlls) File.Copy(Path.Combine(Paths.ModBin, dll), Path.Combine(dir, dll));
            return dir;
        }

        static object Static(Type t, string method, params object[] args) =>
            t.GetMethod(method, BindingFlags.Public | BindingFlags.Static)!.Invoke(null, args);

        static IEnumerable<string> Findings(object result, string field) =>
            ((IEnumerable)result.GetType().GetField(field)!.GetValue(result)!).Cast<object>().Select(f => f.ToString());

        // Negative control: proves the harness really runs the game's rules rather than passing vacuously.
        [Fact]
        public void GameScannerFlagsFilesAndThreadsInAProbe()
        {
            if (!Paths.GameInstalled) return;
            string dir = Path.Combine(Path.GetTempPath(), "fence-probe-" + Guid.NewGuid().ToString("N"));
            Directory.CreateDirectory(dir);
            try
            {
                string dll = Path.Combine(dir, "Probe.dll");
                var module = ModuleDefinition.CreateModule("Probe", ModuleKind.Dll);
                var type = new TypeDefinition("Probe", "P", Mono.Cecil.TypeAttributes.Public | Mono.Cecil.TypeAttributes.Class, module.TypeSystem.Object);
                module.Types.Add(type);
                var m = new MethodDefinition("Run", Mono.Cecil.MethodAttributes.Public | Mono.Cecil.MethodAttributes.Static, module.TypeSystem.Void);
                type.Methods.Add(m);
                var il = m.Body.GetILProcessor();
                il.Emit(OpCodes.Ldstr, "x");
                il.Emit(OpCodes.Call, module.ImportReference(typeof(File).GetMethod("Exists", new[] { typeof(string) })));
                il.Emit(OpCodes.Pop);
                il.Emit(OpCodes.Ldnull);
                il.Emit(OpCodes.Newobj, module.ImportReference(typeof(System.Threading.Thread).GetConstructor(new[] { typeof(System.Threading.ThreadStart) })));
                il.Emit(OpCodes.Pop);
                il.Emit(OpCodes.Ret);
                module.Write(dll);

                var game = Game.Value;
                var result = game.GetType("ModScanner")!.GetMethod("Scan")!.Invoke(null, new object[] { new List<string> { dll }, new List<string>(), null, true })!;
                var detected = ((IEnumerable<string>)result.GetType().GetField("Capabilities")!.GetValue(result)!).ToList();
                var pending = Findings(result, "Pending").ToList();
                Assert.Contains("files", detected);
                Assert.Contains(pending, f => f.Contains("threads"));
            }
            finally
            {
                Directory.Delete(dir, true);
            }
        }

        [Fact]
        public void GameScannerAcceptsTheModWithNetworkOnly()
        {
            if (!Paths.GameInstalled) return;
            var game = Game.Value;
            var loader = game.GetType("ModLoader")!;
            var scanner = game.GetType("ModScanner")!;
            var capability = game.GetType("ModCapability")!;
            string folder = InstalledLayout();
            try
            {
                var args = new object[] { folder, null };
                var manifest = loader.GetMethod("ParseManifestForTools")!.Invoke(null, args);
                Assert.True(manifest != null, "loader rejects manifest.xml: " + args[1]);

                var unlisted = (IList)Static(loader, "UnlistedAssemblyFiles", folder, manifest);
                Assert.Empty(unlisted.Cast<object>());

                var listed = ((List<string>)Static(loader, "ListedAssemblyPaths", folder, manifest)).ToList();
                foreach (var p in listed)
                    Assert.False((bool)Static(loader, "IsGameAssemblyName", AssemblyName.GetAssemblyName(p).Name), p + " shadows a game assembly");

                var declared = (List<string>)manifest!.GetType().GetField("Capabilities")!.GetValue(manifest)!;
                var refusedDeclared = declared.Where(c => (bool)Static(capability, "IsRefused", c)).ToList();
                Assert.Empty(refusedDeclared);

                var allowed = ((List<string>)manifest.GetType().GetField("Assemblies")!.GetValue(manifest)!).Select(Path.GetFileNameWithoutExtension).ToList();
                var result = scanner.GetMethod("Scan")!.Invoke(null, new object[] { listed, allowed, folder, true })!;

                string error = (string)result.GetType().GetField("Error")!.GetValue(result);
                var hard = Findings(result, "HardFlags").ToList();
                var pending = Findings(result, "Pending").ToList();
                var findings = Findings(result, "Findings").ToList();
                var detected = ((IEnumerable<string>)result.GetType().GetField("Capabilities")!.GetValue(result)!).ToList();
                int methods = (int)result.GetType().GetField("MethodsScanned")!.GetValue(result)!;
                var undeclared = (List<string>)Static(capability, "Undeclared", detected, declared);
                var deprecated = (List<string>)result.GetType().GetProperty("DeprecatedCapabilities")!.GetValue(result)!;

                output.WriteLine("scanner v" + scanner.GetField("VERSION")!.GetValue(null) + ", methods scanned " + methods);
                output.WriteLine("declared " + string.Join(",", declared) + "; detected " + string.Join(",", detected));
                foreach (var f in findings) output.WriteLine("  finding " + f);

                Assert.Null(error);
                Assert.True(hard.Count == 0, "hard flags:\n" + string.Join("\n", hard));
                Assert.True(pending.Count == 0, "findings refused from the December 10, 2026 update:\n" + string.Join("\n", pending));
                Assert.Empty(undeclared);
                Assert.Empty(deprecated);
                Assert.Contains("network", detected);
                Assert.True(methods > 0);
            }
            finally
            {
                Directory.Delete(Path.GetDirectoryName(folder)!, true);
            }
        }
    }
}
