using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.IO;
using System.Linq;
using System.Reflection;
using System.Runtime.CompilerServices;
using System.Threading.Tasks;
using Fence.Gym;

namespace Fence.Lab
{
    // fence-lab: controlled experiments on the real game, headless.
    //   fence-lab list                 experiment names
    //   fence-lab <name> [-seed N] [-n N]
    //   fence-lab all [-parallel P]    every experiment, one child process each; results in var/lab/results.jsonl
    internal static class Program
    {
        internal static int N = 2000;

        static int Main(string[] args)
        {
            Boot.Init();
            string cmd = args.Length > 0 ? args[0] : "list";
            int seed = Arg(args, "-seed", 1);
            N = Arg(args, "-n", N);
            R.Out = ArgS(args, "-out", null);
            var exps = Experiments();
            if (cmd == "list")
            {
                foreach (var k in exps.Keys) Console.WriteLine(k);
                return 0;
            }
            if (cmd == "all") return All(exps.Keys.ToList(), Arg(args, "-parallel", Math.Max(1, Environment.ProcessorCount / 2)), args);
            if (!exps.TryGetValue(cmd, out var run))
            {
                Console.Error.WriteLine("unknown experiment " + cmd);
                return 2;
            }
            R.Exp = cmd;
            return RunExp(run, seed);
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        static int RunExp(MethodInfo run, int seed)
        {
            var sw = Stopwatch.StartNew();
            run.Invoke(null, new object[] { seed });
            Console.Error.WriteLine($"{R.Exp}: {sw.Elapsed.TotalSeconds:0.0} s");
            return 0;
        }

        // Every public static void X(int seed) on an experiment class is an experiment named x.
        static SortedDictionary<string, MethodInfo> Experiments()
        {
            var d = new SortedDictionary<string, MethodInfo>(StringComparer.Ordinal);
            foreach (var t in typeof(Program).Assembly.GetTypes().Where(t => t.Name.EndsWith("Exps")))
                foreach (var m in t.GetMethods(BindingFlags.Public | BindingFlags.Static))
                    if (m.ReturnType == typeof(void) && m.GetParameters().Length == 1 && m.GetParameters()[0].ParameterType == typeof(int))
                        d[t.Name.Replace("Exps", "").ToLowerInvariant() + "." + m.Name.ToLowerInvariant()] = m;
            return d;
        }

        static int All(List<string> names, int parallel, string[] args)
        {
            string dir = Boot.Var("lab");
            System.IO.Directory.CreateDirectory(dir);
            string results = Path.Combine(dir, "results.jsonl");
            File.WriteAllText(results, "");
            string self = Environment.ProcessPath, dll = typeof(Program).Assembly.Location;
            string filter = ArgS(args, "-only", null);
            names = names.Where(n => !n.StartsWith("debug.")).ToList();
            if (filter != null) names = names.Where(n => n.StartsWith(filter)).ToList();
            var failed = new List<string>();
            Parallel.ForEach(names, new ParallelOptions { MaxDegreeOfParallelism = parallel }, name =>
            {
                var psi = new ProcessStartInfo(self) { RedirectStandardOutput = true, RedirectStandardError = true };
                if (Path.GetFileNameWithoutExtension(self) == "dotnet") psi.ArgumentList.Add(dll);
                foreach (var a in new[] { name, "-out", results }) psi.ArgumentList.Add(a);
                if (args.Contains("-n")) { psi.ArgumentList.Add("-n"); psi.ArgumentList.Add(ArgS(args, "-n", "")); }
                using var p = Process.Start(psi);
                string so = p.StandardOutput.ReadToEnd(), se = p.StandardError.ReadToEnd();
                p.WaitForExit();
                lock (failed)
                {
                    Console.Error.Write(se.Length > 1500 ? se.Substring(se.Length - 1500) : se);
                    if (p.ExitCode != 0) failed.Add(name);
                }
            });
            Console.WriteLine($"{names.Count} experiments, {failed.Count} failed: {string.Join(", ", failed)} -> {results}");
            return failed.Count == 0 ? 0 : 1;
        }

        static int Arg(string[] a, string k, int def) { int i = Array.IndexOf(a, k); return i >= 0 && i + 1 < a.Length ? int.Parse(a[i + 1]) : def; }
        static string ArgS(string[] a, string k, string def) { int i = Array.IndexOf(a, k); return i >= 0 && i + 1 < a.Length ? a[i + 1] : def; }
    }
}
