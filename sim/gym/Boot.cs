using System;
using System.IO;
using Fence.Sim;

namespace Fence.Gym
{
    // Builds (once per game build) and installs the playable headless game. Must run before any code
    // that touches a game type is JIT-compiled.
    internal static class Boot
    {
        public static string Repo { get; private set; }
        public static string GameDir { get; private set; }
        static bool done;

        public static void Init()
        {
            if (done) return;
            GameDir = Environment.GetEnvironmentVariable("GAME_DIR");
            if (string.IsNullOrEmpty(GameDir))
                GameDir = Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.UserProfile), ".local/share/Steam/steamapps/common/Probably Stolen");
            Repo = FindRepo();
            string managed = Path.Combine(GameDir, "Probably Stolen_Data", "Managed");
            Headless.Build(managed, Path.Combine(Repo, "var", "gamedata", ".headless-gym"), Variant.Gym);
            Headless.Install();
            Loot.EnsureExtracted(GameDir);
            done = true;
        }

        public static string FindRepo()
        {
            foreach (var start in new[] { AppContext.BaseDirectory, System.IO.Directory.GetCurrentDirectory() })
            {
                for (string d = start; d != null; d = Path.GetDirectoryName(d))
                    if (File.Exists(Path.Combine(d, "go.mod")) && System.IO.Directory.Exists(Path.Combine(d, "sim"))) return d;
            }
            return System.IO.Directory.GetCurrentDirectory();
        }

        public static string Var(params string[] parts) => Path.Combine(Repo, Path.Combine("var", Path.Combine(parts)));
    }
}
