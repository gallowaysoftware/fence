using System.Collections.Generic;
using System.IO;
using System.Linq;
using Fence.Compat;
using Fence.CompatTool;
using Mono.Cecil;
using Xunit;

namespace Fence.Tests
{
    sealed class FakeResolver : IMemberResolver
    {
        public readonly Dictionary<string, Dictionary<string, List<MemberSignature>>> Types = new Dictionary<string, Dictionary<string, List<MemberSignature>>>();
        public readonly HashSet<string> Unsupported = new HashSet<string>();

        public FakeResolver Add(string type, string member, MemberSignature sig)
        {
            if (!Types.TryGetValue(type, out var members)) Types[type] = members = new Dictionary<string, List<MemberSignature>>();
            if (!members.TryGetValue(member, out var list)) members[member] = list = new List<MemberSignature>();
            list.Add(sig);
            return this;
        }

        public TypeLookup Lookup(string type, string member)
        {
            if (Unsupported.Contains(type)) return TypeLookup.Unsupported();
            if (!Types.TryGetValue(type, out var members)) return TypeLookup.MissingType();
            return new TypeLookup { TypeFound = true, Members = members.TryGetValue(member, out var l) ? l : new List<MemberSignature>() };
        }
    }

    public class CompatCheckerTests
    {
        static HookSpec Method(string id, params string[] ps) => new HookSpec
        {
            Id = id, Type = "StoreClientManager", Member = "HandleSkipClient", Kind = MemberKinds.Method,
            Params = ps.ToList(), Returns = "System.Void", Features = new List<string> { "act.barter" },
        };

        static HookSpec Event(string returns = "System.Action") => new HookSpec
        {
            Id = "ev", Type = "ModHook", Member = "OnShutterOpenedLate", Kind = MemberKinds.Event,
            Returns = returns, Features = new List<string> { "event.shutter_opened" },
        };

        static MemberSignature Sig(string kind, string returns, params string[] ps) =>
            new MemberSignature { Kind = kind, Returns = returns, Params = ps.ToList() };

        static HookStatus Status(HookSpec hook, IMemberResolver r) => CompatChecker.CheckOne(hook, r).Status;

        [Fact]
        public void MatchingMethod() =>
            Assert.Equal(HookStatus.Ok, Status(Method("m", "System.Int32"), new FakeResolver().Add("StoreClientManager", "HandleSkipClient", Sig("method", "System.Void", "System.Int32"))));

        [Fact]
        public void MatchesTheRightOverload()
        {
            var r = new FakeResolver()
                .Add("StoreClientManager", "HandleSkipClient", Sig("method", "System.Void"))
                .Add("StoreClientManager", "HandleSkipClient", Sig("method", "System.Void", "System.Int32"));
            Assert.Equal(HookStatus.Ok, Status(Method("m", "System.Int32"), r));
        }

        [Fact]
        public void MissingType() => Assert.Equal(HookStatus.MissingType, Status(Method("m"), new FakeResolver()));

        [Fact]
        public void MissingMember()
        {
            var r = new FakeResolver().Add("StoreClientManager", "Other", Sig("method", "System.Void"));
            Assert.Equal(HookStatus.MissingMember, Status(Method("m"), r));
        }

        [Fact]
        public void ChangedParams()
        {
            var r = new FakeResolver().Add("StoreClientManager", "HandleSkipClient", Sig("method", "System.Void", "System.Int64"));
            var result = CompatChecker.CheckOne(Method("m", "System.Int32"), r);
            Assert.Equal(HookStatus.ParamsChanged, result.Status);
            Assert.Contains("System.Int64", result.Detail);
        }

        [Fact]
        public void ChangedReturnType()
        {
            var r = new FakeResolver().Add("StoreClientManager", "HandleSkipClient", Sig("method", "System.Boolean"));
            Assert.Equal(HookStatus.ReturnChanged, Status(Method("m"), r));
        }

        [Fact]
        public void ChangedEventType()
        {
            var r = new FakeResolver().Add("ModHook", "OnShutterOpenedLate", Sig("event", "System.Action<System.String>"));
            Assert.Equal(HookStatus.ReturnChanged, Status(Event(), r));
        }

        [Fact]
        public void ChangedKind()
        {
            var r = new FakeResolver().Add("ModHook", "OnShutterOpenedLate", Sig("field", "System.Action"));
            Assert.Equal(HookStatus.KindChanged, Status(Event(), r));
        }

        [Fact]
        public void ResolverThatCannotSeeATypeIsUncheckedNotMissing()
        {
            var r = new FakeResolver();
            r.Unsupported.Add("ModHook");
            var result = CompatChecker.CheckOne(Event(), r);
            Assert.Equal(HookStatus.Unchecked, result.Status);
            Assert.False(result.Failed);
        }

        [Fact]
        public void ResolverExceptionCountsAsMissingType() =>
            Assert.Equal(HookStatus.MissingType, Status(Event(), new ThrowingResolver()));

        sealed class ThrowingResolver : IMemberResolver
        {
            public TypeLookup Lookup(string type, string member) => throw new System.TypeLoadException("gone");
        }

        [Fact]
        public void HelloCompatListsMissingMembersAndTheirFeatures()
        {
            var manifest = new HookManifest { Hooks = { Method("skip"), Event() } };
            var r = new FakeResolver().Add("ModHook", "OnShutterOpenedLate", Sig("event", "System.Action"));
            var report = CompatChecker.Check(manifest, r);
            var hello = report.ToHello();
            Assert.False(hello.Ok);
            Assert.Equal(new[] { "StoreClientManager::HandleSkipClient()" }, hello.Missing);
            Assert.Equal(new[] { "act.barter" }, hello.DisabledFeatures);
            Assert.True(report.RequiredOk);
        }

        [Fact]
        public void BindFailureAfterAResolvedCheckIsReported()
        {
            var manifest = new HookManifest { Hooks = { Event() } };
            var report = CompatChecker.Check(manifest, new FakeResolver().Add("ModHook", "OnShutterOpenedLate", Sig("event", "System.Action")));
            Assert.True(report.Ok);
            report.MarkBindFailed("ev", "MissingMethodException");
            Assert.False(report.Ok);
            Assert.Equal(new[] { "event.shutter_opened" }, report.ToHello().DisabledFeatures);
        }

        [Fact]
        public void RequiredFailureIsFlagged()
        {
            var hook = Method("m");
            hook.Required = true;
            var report = CompatChecker.Check(new HookManifest { Hooks = { hook } }, new FakeResolver());
            Assert.False(report.RequiredOk);
        }
    }

    public class HooksManifestTests
    {
        static HookManifest Load() => HookManifest.Parse(File.ReadAllText(Paths.HooksJson));

        [Fact]
        public void IsValidWithUniqueIds()
        {
            var m = Load();
            Assert.NotEmpty(m.Hooks);
            Assert.Empty(m.Validate());
            Assert.Equal(m.Hooks.Count, m.Hooks.Select(h => h.Id).Distinct().Count());
        }

        [Fact]
        public void EmbeddedCopyMatchesTheFile()
        {
            var embedded = HookManifest.LoadEmbedded();
            Assert.Equal(Load().Hooks.Select(h => h.Id), embedded.Hooks.Select(h => h.Id));
        }

        [Fact]
        public void EveryForwardedEventHasOneKind()
        {
            foreach (var h in Load().Hooks.Where(h => h.Kind == MemberKinds.Event && h.EventKind != null))
            {
                Assert.Single(h.Features, f => f.StartsWith(HookSpec.EventFeaturePrefix));
                Assert.Matches("^[a-z_]+$", h.EventKind);
            }
            var kinds = Load().Hooks.Where(h => h.EventKind != null).Select(h => h.EventKind).ToList();
            Assert.Equal(kinds.Count, kinds.Distinct().Count());
            Assert.Contains("shutter_opened", kinds);
        }

        [Fact]
        public void ProposedManifestParses()
        {
            string p = Path.Combine(Paths.Repo, "compat", "hooks.proposed.json");
            if (!File.Exists(p)) return;
            Assert.Empty(HookManifest.Parse(File.ReadAllText(p)).Validate());
        }

        [Fact]
        public void InstalledGameSatisfiesHooksJson()
        {
            if (!Paths.GameInstalled) return;
            var resolver = new DefaultAssemblyResolver();
            resolver.AddSearchDirectory(Paths.GameManaged);
            using var module = ModuleDefinition.ReadModule(Paths.AssemblyCSharp, new ReaderParameters { AssemblyResolver = resolver });
            var report = CompatChecker.Check(Load(), new CecilResolver(module));
            Assert.True(report.Ok, string.Join("\n", report.Failures.Select(f => f.Hook.Id + " " + f.Status + " " + f.Detail)));
        }
    }
}
