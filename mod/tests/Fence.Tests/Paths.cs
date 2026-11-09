using System.IO;
using System.Linq;
using System.Reflection;

namespace Fence.Tests
{
    static class Paths
    {
        static string Meta(string key) =>
            typeof(Paths).Assembly.GetCustomAttributes<AssemblyMetadataAttribute>().Single(a => a.Key == key).Value;

        public static string Repo => Path.GetFullPath(Meta("RepoRoot"));
        public static string GameManaged => Meta("GameManaged");
        public static string ModBin => Path.GetFullPath(Meta("ModBinDir"));
        public static string Fixtures => Path.Combine(Repo, "protocol", "fixtures");
        public static string HooksJson => Path.Combine(Repo, "compat", "hooks.json");
        public static string ManifestXml => Path.Combine(Repo, "mod", "src", "Fence.Mod", "manifest.xml");
        public static string AssemblyCSharp => Path.Combine(GameManaged, "Assembly-CSharp.dll");
        public static bool GameInstalled => File.Exists(AssemblyCSharp);

        // What mod/install.sh copies into Mods/fence: exactly the DLLs the manifest must list.
        public static readonly string[] ShippedDlls = { "Fence.Core.dll", "Fence.Mod.dll" };
    }
}
