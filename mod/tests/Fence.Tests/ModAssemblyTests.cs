using System;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using System.Xml.Linq;
using Fence.Compat;
using Mono.Cecil;
using Mono.Cecil.Cil;
using Xunit;

namespace Fence.Tests
{
    // Static checks on the DLLs we actually ship (built Release/Debug output of Fence.Mod).
    public class ModAssemblyTests
    {
        static HookManifest Hooks() => HookManifest.Parse(File.ReadAllText(Paths.HooksJson));

        static IEnumerable<ModuleDefinition> ShippedModules()
        {
            foreach (var dll in Paths.ShippedDlls)
                yield return ModuleDefinition.ReadModule(Path.Combine(Paths.ModBin, dll));
        }

        static bool IsGame(IMetadataScope scope) => scope?.Name == "Assembly-CSharp" || scope?.Name == "Assembly-CSharp.dll";

        static string TypeKey(TypeReference t) => t.GetElementType().FullName.Replace('/', '+');

        static IEnumerable<(string type, string kind, string member)> GameMemberRefs(ModuleDefinition module)
        {
            foreach (var type in module.GetTypes())
            foreach (var method in type.Methods.Where(m => m.HasBody))
            foreach (var ins in method.Body.Instructions)
            {
                switch (ins.Operand)
                {
                    case MethodReference mr when IsGame(mr.DeclaringType.GetElementType().Scope):
                        string name = mr.Name, kind = MemberKinds.Method;
                        if (name.StartsWith("get_") || name.StartsWith("set_")) { kind = MemberKinds.Property; name = name.Substring(4); }
                        else if (name.StartsWith("add_")) { kind = MemberKinds.Event; name = name.Substring(4); }
                        else if (name.StartsWith("remove_")) { kind = MemberKinds.Event; name = name.Substring(7); }
                        yield return (TypeKey(mr.DeclaringType), kind, name);
                        break;
                    case FieldReference fr when IsGame(fr.DeclaringType.GetElementType().Scope):
                        yield return (TypeKey(fr.DeclaringType), MemberKinds.Field, fr.Name);
                        break;
                }
            }
        }

        [Fact]
        public void EveryGameMemberWeTouchIsInHooksJson()
        {
            var hooks = Hooks().Hooks;
            var unlisted = ShippedModules().SelectMany(GameMemberRefs).Distinct()
                .Where(r => !hooks.Any(h => h.Type == r.type && h.Member == r.member && h.Kind == r.kind))
                .Select(r => r.type + "::" + r.member + " (" + r.kind + ")").ToList();
            Assert.True(unlisted.Count == 0, "add to compat/hooks.json: " + string.Join(", ", unlisted));
        }

        [Fact]
        public void EveryGameTypeWeNameIsCoveredByHooksJson()
        {
            var hooks = Hooks().Hooks;
            // A nested type ("RenderHandler+ColorPalette") also covers its declaring type.
            var mentioned = string.Join(" ", hooks.Select(h => h.Type + " " + h.Returns + " " + string.Join(" ", h.Params ?? new List<string>())));
            var missing = ShippedModules().SelectMany(m => m.GetTypeReferences())
                .Where(t => IsGame(t.Scope))
                .Select(t => t.FullName.Replace('/', '+'))
                .Distinct()
                .Where(n => !hooks.Any(h => h.Type == n) && !System.Text.RegularExpressions.Regex.IsMatch(mentioned, @"(^|[\s<,])" + System.Text.RegularExpressions.Regex.Escape(n) + @"($|[\s>,\[&+])"))
                .ToList();
            Assert.True(missing.Count == 0, "game types used but absent from hooks.json: " + string.Join(", ", missing));
        }

        [Fact]
        public void EveryEventHookIsSubscribed()
        {
            var adds = new HashSet<string>(ShippedModules().SelectMany(GameMemberRefs)
                .Where(r => r.type == "ModHook" && r.kind == MemberKinds.Event).Select(r => r.member));
            var unbound = Hooks().Hooks.Where(h => h.Type == "ModHook" && h.Kind == MemberKinds.Event && !adds.Contains(h.Member)).Select(h => h.Id).ToList();
            Assert.True(unbound.Count == 0, "hooks.json events with no subscription in Fence.Mod: " + string.Join(", ", unbound));

            var bindingIds = ShippedModules().Single(m => m.Name == "Fence.Mod.dll").GetType("Fence.Mod.HookBindings")
                .Methods.Single(m => m.IsConstructor && m.IsStatic).Body.Instructions
                .Where(i => i.OpCode.Code == Code.Ldstr).Select(i => (string)i.Operand).ToHashSet();
            // Forwarded events go through HookBindings; the others (the tooltip) are bound by the advisor.
            foreach (var h in Hooks().Hooks.Where(h => h.Type == "ModHook" && h.Kind == MemberKinds.Event && h.EventKind != null))
                Assert.Contains(h.Id, bindingIds);
        }

        [Fact]
        public void RuntimeResolverCoversEveryHookType()
        {
            var lookup = ShippedModules().Single(m => m.Name == "Fence.Mod.dll").GetType("Fence.Mod.RuntimeResolver")
                .Methods.Single(m => m.Name == "Lookup");
            var cases = lookup.Body.Instructions.Where(i => i.OpCode.Code == Code.Ldstr).Select(i => (string)i.Operand).ToHashSet();
            var uncovered = Hooks().Hooks.Select(h => h.Type).Distinct().Where(t => !cases.Contains(t)).ToList();
            Assert.True(uncovered.Count == 0, "RuntimeResolver.Lookup has no case for: " + string.Join(", ", uncovered));
        }

        [Fact]
        public void ReferencesOnlyGameAssembliesOrEachOther()
        {
            if (!Paths.GameInstalled) return;
            var game = new HashSet<string>(Directory.GetFiles(Paths.GameManaged, "*.dll").Select(Path.GetFileNameWithoutExtension), StringComparer.OrdinalIgnoreCase);
            var ours = new HashSet<string>(Paths.ShippedDlls.Select(Path.GetFileNameWithoutExtension), StringComparer.OrdinalIgnoreCase);
            foreach (var m in ShippedModules())
            {
                Assert.DoesNotContain(m.Assembly.Name.Name, game);
                foreach (var r in m.AssemblyReferences)
                    Assert.True(game.Contains(r.Name) || ours.Contains(r.Name), m.Name + " references " + r.Name);
            }
        }
    }

    public class ManifestTests
    {
        static XElement Root() => XDocument.Load(Paths.ManifestXml).Root;

        static List<string> List(string parent, string child) =>
            Root().Element(parent)?.Elements(child).Select(e => e.Value.Trim()).ToList() ?? new List<string>();

        [Fact]
        public void IdAndCapabilities()
        {
            Assert.Equal("fence", Root().Element("ID")!.Value.Trim());
            Assert.Equal(new[] { "network" }, List("Capabilities", "Capability"));
            Assert.Empty(List("Prerequisites", "Mod"));
        }

        [Fact]
        public void ListsExactlyTheShippedDlls()
        {
            Assert.Equal(Paths.ShippedDlls.OrderBy(x => x), List("Assemblies", "Assembly").OrderBy(x => x));
            var built = Directory.GetFiles(Paths.ModBin, "*.dll").Select(Path.GetFileName).OrderBy(x => x);
            Assert.Equal(Paths.ShippedDlls.OrderBy(x => x), built);
        }

        [Fact]
        public void ModVersionMatchesTheAssembly()
        {
            var v = ModuleDefinition.ReadModule(Path.Combine(Paths.ModBin, "Fence.Mod.dll")).Assembly.Name.Version;
            Assert.Equal(v.ToString(3), Root().Element("ModVersion")!.Value.Trim());
        }

        [Fact]
        public void TargetsEveryVerifiedGameVersion()
        {
            var versions = List("GameVersions", "Version");
            foreach (var snap in KnownBuilds.LoadEmbedded())
                Assert.Contains(snap.AppVersion, versions);
        }
    }
}
