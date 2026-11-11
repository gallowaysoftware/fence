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
            GameAssets.EnsureExtracted(GameDir, Var("gamedata"));
            string guides = Environment.GetEnvironmentVariable("FENCE_APPRAISAL_GUIDES");
            if (string.IsNullOrEmpty(guides)) guides = Path.Combine(MainCheckout(Repo), "local", "appraisal_guides.json");
            if (File.Exists(guides)) Appraisal.Guides = Newtonsoft.Json.Linq.JObject.Parse(File.ReadAllText(guides));
            done = true;
        }

        // A git worktree's local/ is empty; its .git file names the main checkout ("gitdir: <main>/.git/worktrees/<name>").
        static string MainCheckout(string repo)
        {
            string dotGit = Path.Combine(repo, ".git");
            if (!File.Exists(dotGit)) return repo;
            string line = File.ReadAllText(dotGit).Trim();
            int at = line.IndexOf("/.git/worktrees/", StringComparison.Ordinal);
            return line.StartsWith("gitdir: ", StringComparison.Ordinal) && at > 0 ? line.Substring(8, at - 8) : repo;
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
