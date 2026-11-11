using System.IO;
using Newtonsoft.Json.Linq;

namespace Fence.Gym
{
    // Gym-only: the executor's shared files never touch the file system (mod/src/Fence.Autopilot).
    internal sealed partial class Directives
    {
        public static Directives Load(string path) => Parse(JObject.Parse(File.ReadAllText(path)));
    }
}
