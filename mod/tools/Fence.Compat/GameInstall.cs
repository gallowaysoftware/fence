using System;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using System.Security.Cryptography;
using System.Text;
using System.Text.RegularExpressions;
using Fence.Compat;
using Mono.Cecil;
using Mono.Cecil.Cil;

namespace Fence.CompatTool
{
    // Facts about one installed build, all read offline from files Steam and Unity leave behind.
    public sealed class GameInstall
    {
        public const string AppId = "4348910";

        public string GameDir { get; }
        public string DataDir => Path.Combine(GameDir, "Probably Stolen_Data");
        public string ManagedDir => Path.Combine(DataDir, "Managed");
        public string SteamApps => Path.GetFullPath(Path.Combine(GameDir, "..", ".."));
        public string AcfPath => Path.Combine(SteamApps, "appmanifest_" + AppId + ".acf");
        public string PersistentDir => Path.Combine(SteamApps, "compatdata", AppId, "pfx", "drive_c", "users", "steamuser",
            "AppData", "LocalLow", "Questing Goose Studio", "Probably Stolen");

        public GameInstall(string gameDir)
        {
            GameDir = Path.GetFullPath(gameDir);
        }

        public static string DefaultGameDir()
        {
            var env = Environment.GetEnvironmentVariable("GAME_DIR");
            if (!string.IsNullOrEmpty(env)) return env;
            return Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.UserProfile),
                ".local", "share", "Steam", "steamapps", "common", "Probably Stolen");
        }

        public static string Sha256(string path)
        {
            using var stream = File.OpenRead(path);
            return Convert.ToHexString(SHA256.HashData(stream)).ToLowerInvariant();
        }

        public (string buildId, List<DepotInfo> depots) ReadAcf()
        {
            if (!File.Exists(AcfPath)) return (null, new List<DepotInfo>());
            string text = File.ReadAllText(AcfPath);
            string buildId = Regex.Match(text, "\"buildid\"\\s+\"(\\d+)\"").Groups[1].Value;
            var depots = new List<DepotInfo>();
            var block = Regex.Match(text, "\"InstalledDepots\"\\s*\\{((?:[^{}]|\\{[^{}]*\\})*)\\}");
            if (block.Success)
            {
                foreach (Match m in Regex.Matches(block.Groups[1].Value, "\"(\\d+)\"\\s*\\{[^{}]*?\"manifest\"\\s+\"(\\d+)\""))
                    depots.Add(new DepotInfo { Depot = m.Groups[1].Value, Manifest = m.Groups[2].Value });
            }
            return (buildId.Length > 0 ? buildId : null, depots);
        }

        static readonly Regex UnityVersion = new Regex(@"\d{4}\.\d+\.\d+[abfpx]\d+", RegexOptions.Compiled);
        static readonly Regex AppVersionShape = new Regex(@"^\d+\.\d+[0-9A-Za-z._-]*$", RegexOptions.Compiled);

        public string ReadUnityVersion()
        {
            var bytes = ReadGlobalGameManagers();
            if (bytes == null) return null;
            var head = Encoding.ASCII.GetString(bytes, 0, Math.Min(bytes.Length, 256));
            var m = UnityVersion.Match(head);
            return m.Success ? m.Value : null;
        }

        // PlayerSettings.bundleVersion is a length-prefixed, 4-byte-aligned string in
        // globalgamemanagers. No type tree to walk, so: every aligned length-prefixed string shaped
        // like a version, minus the Unity version. Exactly one candidate is required.
        public List<string> BundleVersionCandidates()
        {
            var bytes = ReadGlobalGameManagers();
            var found = new List<string>();
            if (bytes == null) return found;
            string unity = ReadUnityVersion();
            for (int i = 0; i + 8 <= bytes.Length; i += 4)
            {
                int n = BitConverter.ToInt32(bytes, i);
                if (n < 2 || n > 32 || i + 4 + n > bytes.Length) continue;
                string s = Encoding.ASCII.GetString(bytes, i + 4, n);
                if (s != unity && AppVersionShape.IsMatch(s) && !found.Contains(s)) found.Add(s);
            }
            return found;
        }

        byte[] ReadGlobalGameManagers()
        {
            string p = Path.Combine(DataDir, "globalgamemanagers");
            return File.Exists(p) ? File.ReadAllBytes(p) : null;
        }

        // The loader writes "audit: game <Application.version>, ..." on every start.
        public string AuditLogVersion()
        {
            string p = Path.Combine(PersistentDir, "mod-audit.log");
            if (!File.Exists(p)) return null;
            string last = null;
            foreach (var line in File.ReadLines(p))
            {
                var m = Regex.Match(line, @"\[ModLoader\] audit: game ([^,\s]+),");
                if (m.Success) last = m.Groups[1].Value;
            }
            return last;
        }

        // "VERSION 100-Rev13": the studio's own build label, separate from Application.version.
        public static string BuildLabel(ModuleDefinition module)
        {
            var labels = new SortedSet<string>(StringComparer.Ordinal);
            foreach (var type in module.GetTypes())
            foreach (var method in type.Methods.Where(m => m.HasBody))
            foreach (var ins in method.Body.Instructions)
            {
                if (ins.OpCode.Code != Code.Ldstr) continue;
                var m = Regex.Match((string)ins.Operand, @"VERSION (\S+-Rev\d+)");
                if (m.Success) labels.Add(m.Groups[1].Value);
            }
            return labels.Count == 0 ? null : string.Join(" ", labels);
        }

        public static ScannerInfo ReadScanner(ModuleDefinition module)
        {
            var info = new ScannerInfo();
            var scanner = module.GetType("ModScanner");
            var version = scanner?.Fields.FirstOrDefault(f => f.Name == "VERSION" && f.HasConstant);
            if (version != null) info.Version = Convert.ToInt32(version.Constant);
            info.RefuseUnscannable = StaticBool(scanner, "REFUSE_UNSCANNABLE_CAPABILITIES");
            info.Retiring = StaticStrings(module.GetType("ModCapability"), "Retiring");
            return info;
        }

        static bool? StaticBool(TypeDefinition type, string field)
        {
            var cctor = type?.Methods.FirstOrDefault(m => m.IsConstructor && m.IsStatic && m.HasBody);
            if (cctor == null) return null;
            var ins = cctor.Body.Instructions;
            for (int i = 1; i < ins.Count; i++)
            {
                if (ins[i].OpCode.Code == Code.Stsfld && ((FieldReference)ins[i].Operand).Name == field)
                {
                    if (ins[i - 1].OpCode.Code == Code.Ldc_I4_0) return false;
                    if (ins[i - 1].OpCode.Code == Code.Ldc_I4_1) return true;
                    return null;
                }
            }
            return null;
        }

        static List<string> StaticStrings(TypeDefinition type, string field)
        {
            var result = new List<string>();
            var cctor = type?.Methods.FirstOrDefault(m => m.IsConstructor && m.IsStatic && m.HasBody);
            if (cctor == null) return result;
            var pending = new List<string>();
            foreach (var ins in cctor.Body.Instructions)
            {
                if (ins.OpCode.Code == Code.Ldstr) pending.Add((string)ins.Operand);
                else if (ins.OpCode.Code == Code.Stsfld)
                {
                    if (((FieldReference)ins.Operand).Name == field) return pending;
                    pending = new List<string>();
                }
            }
            return result;
        }
    }
}
