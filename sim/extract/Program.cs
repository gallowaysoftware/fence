using System;
using System.IO;
using System.Runtime.CompilerServices;

using Fence.Sim;

namespace Fence.Extract
{
    static class Program
    {
        static int Main(string[] args)
        {
            string gameDir = Environment.GetEnvironmentVariable("GAME_DIR");
            if (string.IsNullOrEmpty(gameDir))
                gameDir = Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.UserProfile), ".local/share/Steam/steamapps/common/Probably Stolen");
            string repo = FindRepo();
            string outDir = Path.Combine(repo, "var", "gamedata");
            string mode = args.Length > 0 ? args[0] : "extract";

            if (mode == "stats") return Stats.Print(outDir);
            if (mode != "extract" && mode != "probe" && mode != "bench")
            {
                Console.Error.WriteLine("usage: extract [extract|stats|probe|bench]");
                return 2;
            }

            string managed = Path.Combine(gameDir, "Probably Stolen_Data", "Managed");
            Headless.Build(managed, Path.Combine(outDir, ".headless"));
            Headless.Install();
            var build = BuildInfo.Read(gameDir, managed);
            return Run(mode, outDir, build);
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        static int Run(string mode, string outDir, BuildInfo build)
        {
            Headless.Seed(1);
            var game = new Game();
            if (mode == "probe") return game.Probe();
            if (mode == "bench") return Extractor.Bench(1000);
            return Extractor.All(game, outDir, build);
        }

        static string FindRepo()
        {
            string d = AppContext.BaseDirectory;
            while (d != null && !File.Exists(Path.Combine(d, "go.mod"))) d = Path.GetDirectoryName(d);
            return d ?? System.IO.Directory.GetCurrentDirectory();
        }
    }
}
