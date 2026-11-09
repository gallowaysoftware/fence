using System;
using System.IO;
using System.Reflection;
using System.Runtime.Loader;

namespace Fence.Gym
{
    // A reset that is as clean as a new process: the game keeps run state in statics that a scene
    // reload clears in the player but nothing clears headless, so each episode after the first gets
    // its own collectible AssemblyLoadContext holding fresh copies of the game, Fence.Headless,
    // Fence.Core and this assembly.
    internal sealed class EpisodeContext : AssemblyLoadContext
    {
        readonly string baseDir, gameDir;

        EpisodeContext(string baseDir, string gameDir) : base("episode", isCollectible: true)
        {
            this.baseDir = baseDir;
            this.gameDir = gameDir;
        }

        protected override Assembly Load(AssemblyName name)
        {
            foreach (var dir in new[] { gameDir, baseDir })
            {
                string p = Path.Combine(dir, name.Name + ".dll");
                if (File.Exists(p)) return LoadFromAssemblyPath(p);
            }
            return null;
        }

        // Runs Program.PlayJson in a fresh context and unloads it afterwards.
        public static string Play(string argsJson)
        {
            string baseDir = AppContext.BaseDirectory;
            var ctx = new EpisodeContext(baseDir, Path.Combine(Boot.Repo, "var", "gamedata", ".headless-gym"));
            try
            {
                var asm = ctx.LoadFromAssemblyPath(typeof(EpisodeContext).Assembly.Location);
                var play = asm.GetType("Fence.Gym.Program").GetMethod("PlayJson", BindingFlags.Static | BindingFlags.Public | BindingFlags.NonPublic);
                return (string)play.Invoke(null, new object[] { argsJson });
            }
            finally
            {
                ctx.Unload();
            }
        }
    }
}
