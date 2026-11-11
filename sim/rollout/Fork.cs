using System;
using System.IO;
using System.Linq;
using System.Runtime.InteropServices;

namespace Fence.Rollout
{
    // fork(2) for branching a running game: the child gets a copy of the whole headless game at this
    // instant, so a branch costs only the days it plays, not a replay from day 1. Works because the game
    // is single-threaded, the runtime has no helper threads that matter (Fence.Rollout.csproj) and W^X is
    // off (Program.Main). The child never runs finalizers and must leave with _exit, not return.
    internal static class Libc
    {
        [DllImport("libc", SetLastError = true)] public static extern int fork();
        [DllImport("libc", SetLastError = true)] public static extern int waitpid(int pid, out int status, int options);
        [DllImport("libc")] public static extern void _exit(int status);
        [DllImport("libc", SetLastError = true)] public static extern int setpriority(int which, int who, int prio);
        [DllImport("libc", SetLastError = true)] public static extern int getpriority(int which, int who);

        public static bool Exited(int status, out int code)
        {
            code = (status >> 8) & 0xff;
            return (status & 0x7f) == 0;
        }
    }

    // The machine is shared: at most `Par` branches at once, and when the real game is running, fewer and nicer.
    internal static class Throttle
    {
        public static bool GameRunning()
        {
            try
            {
                foreach (var d in System.IO.Directory.EnumerateDirectories("/proc"))
                {
                    if (!int.TryParse(Path.GetFileName(d), out _)) continue;
                    string cmd;
                    try { cmd = File.ReadAllText(Path.Combine(d, "cmdline")); }
                    catch (Exception) { continue; }
                    // argv[0] only: a shell whose command line mentions the game is not the game.
                    string argv0 = cmd.Split('\0').FirstOrDefault() ?? "";
                    if (argv0.EndsWith("Probably Stolen.exe", StringComparison.Ordinal)) return true;
                }
            }
            catch (Exception) { }
            return false;
        }

        public static int Par(int want)
        {
            if (!GameRunning()) return want;
            if (Libc.getpriority(0, 0) < 19) Libc.setpriority(0, 0, 19);
            return Math.Min(want, 4);
        }
    }
}
