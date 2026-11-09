using System;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using Fence.Compat;
using Mono.Cecil;
using Newtonsoft.Json;

namespace Fence.CompatTool
{
    public static class Program
    {
        // DLLs the mod compiles against or the loader judges it with; hashed into every snapshot.
        static readonly string[] ReferencedDlls =
        {
            "Assembly-CSharp", "Newtonsoft.Json", "UnityEngine", "UnityEngine.CoreModule",
            "UnityEngine.UnityWebRequestModule", "netstandard", "mscorlib", "0Harmony", "DseCecil",
        };

        const string Usage = @"usage: Fence.Compat [--game-dir DIR] [--repo DIR] [--hooks FILE] [--check-only]

Checks every hook in compat/hooks.json (or --hooks FILE) against the installed game.
Without --check-only it writes compat/versions/<buildid>.json and compat/surface/<buildid>.txt.
Exit 0: every required hook resolves. 1: a required hook is missing. 2: usage or install error.";

        public static int Main(string[] args)
        {
            string gameDir = GameInstall.DefaultGameDir();
            string repo = null, hooksPath = null;
            bool checkOnly = false;
            for (int i = 0; i < args.Length; i++)
            {
                switch (args[i])
                {
                    case "--game-dir" when i + 1 < args.Length: gameDir = args[++i]; break;
                    case "--repo" when i + 1 < args.Length: repo = args[++i]; break;
                    case "--hooks" when i + 1 < args.Length: hooksPath = args[++i]; break;
                    case "--check-only": checkOnly = true; break;
                    default:
                        Console.Error.WriteLine(Usage);
                        return 2;
                }
            }

            repo ??= FindRepo(Directory.GetCurrentDirectory()) ?? FindRepo(AppContext.BaseDirectory);
            if (repo == null)
            {
                Console.Error.WriteLine("cannot find the repo (compat/hooks.json); pass --repo");
                return 2;
            }
            hooksPath ??= Path.Combine(repo, "compat", "hooks.json");

            var game = new GameInstall(gameDir);
            string asmPath = Path.Combine(game.ManagedDir, "Assembly-CSharp.dll");
            if (!File.Exists(asmPath))
            {
                Console.Error.WriteLine("no " + asmPath + " (IL2CPP build, or wrong --game-dir?)");
                return 2;
            }

            var manifest = HookManifest.Parse(File.ReadAllText(hooksPath));
            var problems = manifest.Validate();
            if (problems.Count > 0)
            {
                foreach (var p in problems) Console.Error.WriteLine(hooksPath + ": " + p);
                return 2;
            }

            var resolverDirs = new DefaultAssemblyResolver();
            resolverDirs.AddSearchDirectory(game.ManagedDir);
            using var module = ModuleDefinition.ReadModule(asmPath, new ReaderParameters { AssemblyResolver = resolverDirs });
            var resolver = new CecilResolver(module);
            var report = CompatChecker.Check(manifest, resolver);

            PrintReport(hooksPath, report);
            if (checkOnly) return report.RequiredOk ? 0 : 1;

            var (buildId, depots) = game.ReadAcf();
            if (buildId == null)
            {
                Console.Error.WriteLine("no buildid in " + game.AcfPath + "; snapshot not written");
                return 2;
            }

            var snapshot = BuildSnapshot(game, module, buildId, depots, hooksPath, report);
            string versionsDir = Path.Combine(repo, "compat", "versions");
            string surfaceDir = Path.Combine(repo, "compat", "surface");
            Directory.CreateDirectory(versionsDir);
            Directory.CreateDirectory(surfaceDir);
            string snapPath = Path.Combine(versionsDir, buildId + ".json");
            File.WriteAllText(snapPath, JsonConvert.SerializeObject(snapshot, Formatting.Indented) + "\n");

            var types = manifest.Hooks.Select(h => h.Type).Concat(ReadTypeList(Path.Combine(repo, "compat", "surface-types.txt")));
            string surfacePath = Path.Combine(surfaceDir, buildId + ".txt");
            File.WriteAllText(surfacePath, SurfaceDump.Render(resolver, types));

            Console.WriteLine();
            Console.WriteLine("build " + buildId + "  app " + snapshot.AppVersion + "  label " + snapshot.BuildLabel + "  unity " + snapshot.Unity);
            Console.WriteLine("depots " + string.Join(", ", depots.Select(d => d.Depot + "/" + d.Manifest)));
            Console.WriteLine("Assembly-CSharp sha256 " + snapshot.AssemblyCSharp.Sha256 + " mvid " + snapshot.AssemblyCSharp.Mvid);
            Console.WriteLine("scanner v" + snapshot.Scanner.Version + " refuse_unscannable=" + snapshot.Scanner.RefuseUnscannable);
            if (snapshot.AppVersionSources.Values.Distinct().Count() > 1)
                Console.WriteLine("WARNING: app version sources disagree: " + JsonConvert.SerializeObject(snapshot.AppVersionSources));
            Console.WriteLine("wrote " + snapPath);
            Console.WriteLine("wrote " + surfacePath);
            return report.RequiredOk ? 0 : 1;
        }

        static BuildSnapshot BuildSnapshot(GameInstall game, ModuleDefinition module, string buildId, List<DepotInfo> depots, string hooksPath, CompatReport report)
        {
            var snap = new BuildSnapshot
            {
                SteamBuildId = buildId,
                SteamDepots = depots,
                Unity = game.ReadUnityVersion(),
                BuildLabel = GameInstall.BuildLabel(module),
                Scanner = GameInstall.ReadScanner(module),
                HooksSha256 = GameInstall.Sha256(hooksPath),
                HooksOk = report.Ok,
                RequiredOk = report.RequiredOk,
                AssemblyCSharp = new AssemblyInfo
                {
                    Name = "Assembly-CSharp",
                    Sha256 = GameInstall.Sha256(Path.Combine(game.ManagedDir, "Assembly-CSharp.dll")),
                    Mvid = module.Mvid.ToString("D"),
                },
            };

            var candidates = game.BundleVersionCandidates();
            if (candidates.Count == 1) snap.AppVersionSources["globalgamemanagers"] = candidates[0];
            else if (candidates.Count > 1) snap.AppVersionSources["globalgamemanagers"] = "ambiguous: " + string.Join(" ", candidates);
            string audit = game.AuditLogVersion();
            if (audit != null) snap.AppVersionSources["mod-audit.log"] = audit;
            snap.AppVersion = candidates.Count == 1 ? candidates[0] : audit;

            foreach (var name in ReferencedDlls)
            {
                string p = Path.Combine(game.ManagedDir, name + ".dll");
                if (!File.Exists(p)) continue;
                var info = new AssemblyInfo { Name = name, Sha256 = GameInstall.Sha256(p) };
                try
                {
                    info.Version = System.Reflection.AssemblyName.GetAssemblyName(p).Version?.ToString();
                }
                catch (Exception)
                {
                }
                snap.ReferencedDlls.Add(info);
            }

            snap.Hooks = report.Results.Select(r => new HookResultInfo { Id = r.Hook.Id, Status = r.Status.ToString(), Detail = r.Detail }).ToList();
            return snap;
        }

        static void PrintReport(string hooksPath, CompatReport report)
        {
            int ok = report.Results.Count(r => !r.Failed);
            Console.WriteLine(Path.GetFileName(hooksPath) + ": " + ok + "/" + report.Results.Count + " hooks resolve");
            foreach (var r in report.Failures)
                Console.WriteLine((r.Hook.Required ? "  REQUIRED " : "  missing  ") + r.Hook.Id + "  " + r.Hook.Display + "  " + r.Status + (r.Detail != null ? ": " + r.Detail : ""));
            var disabled = report.DisabledFeatures();
            if (disabled.Count > 0) Console.WriteLine("  disabled features: " + string.Join(", ", disabled.OrderBy(s => s, StringComparer.Ordinal)));
        }

        static IEnumerable<string> ReadTypeList(string path) =>
            File.Exists(path)
                ? File.ReadAllLines(path).Select(l => l.Trim()).Where(l => l.Length > 0 && !l.StartsWith("#"))
                : Enumerable.Empty<string>();

        static string FindRepo(string start)
        {
            for (var d = new DirectoryInfo(start); d != null; d = d.Parent)
                if (File.Exists(Path.Combine(d.FullName, "compat", "hooks.json"))) return d.FullName;
            return null;
        }
    }
}
