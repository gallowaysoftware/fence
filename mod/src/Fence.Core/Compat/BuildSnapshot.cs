using System;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using Newtonsoft.Json;

namespace Fence.Compat
{
    // compat/versions/<buildid>.json, written by Fence.Compat and embedded into Fence.Core so the
    // running mod can recognise a verified build by Assembly-CSharp's MVID (it cannot hash the
    // DLL itself without the `files` capability).
    public sealed class BuildSnapshot
    {
        [JsonProperty("steam_build_id")] public string SteamBuildId;
        [JsonProperty("steam_depots")] public List<DepotInfo> SteamDepots = new List<DepotInfo>();
        [JsonProperty("app_version")] public string AppVersion;
        [JsonProperty("app_version_sources")] public Dictionary<string, string> AppVersionSources = new Dictionary<string, string>();
        [JsonProperty("build_label")] public string BuildLabel;
        [JsonProperty("unity")] public string Unity;
        [JsonProperty("assembly_csharp")] public AssemblyInfo AssemblyCSharp;
        [JsonProperty("referenced_dlls")] public List<AssemblyInfo> ReferencedDlls = new List<AssemblyInfo>();
        [JsonProperty("scanner")] public ScannerInfo Scanner;
        [JsonProperty("hooks_sha256")] public string HooksSha256;
        [JsonProperty("hooks_ok")] public bool HooksOk;
        [JsonProperty("required_ok")] public bool RequiredOk;
        [JsonProperty("hooks")] public List<HookResultInfo> Hooks = new List<HookResultInfo>();
    }

    public sealed class DepotInfo
    {
        [JsonProperty("depot")] public string Depot;
        [JsonProperty("manifest")] public string Manifest;
    }

    public sealed class AssemblyInfo
    {
        [JsonProperty("name")] public string Name;
        [JsonProperty("sha256")] public string Sha256;
        [JsonProperty("mvid", NullValueHandling = NullValueHandling.Ignore)] public string Mvid;
        [JsonProperty("version", NullValueHandling = NullValueHandling.Ignore)] public string Version;
    }

    public sealed class ScannerInfo
    {
        [JsonProperty("version")] public int? Version;
        [JsonProperty("refuse_unscannable")] public bool? RefuseUnscannable;
        [JsonProperty("retiring")] public List<string> Retiring = new List<string>();
    }

    public sealed class HookResultInfo
    {
        [JsonProperty("id")] public string Id;
        [JsonProperty("status")] public string Status;
        [JsonProperty("detail", NullValueHandling = NullValueHandling.Ignore)] public string Detail;
    }

    public static class KnownBuilds
    {
        const string Prefix = "Fence.versions.";

        public static List<BuildSnapshot> LoadEmbedded()
        {
            var asm = typeof(KnownBuilds).Assembly;
            var list = new List<BuildSnapshot>();
            foreach (var name in asm.GetManifestResourceNames().Where(n => n.StartsWith(Prefix, StringComparison.Ordinal)).OrderBy(n => n, StringComparer.Ordinal))
            {
                using (var stream = asm.GetManifestResourceStream(name))
                using (var reader = new StreamReader(stream))
                {
                    var snap = JsonConvert.DeserializeObject<BuildSnapshot>(reader.ReadToEnd());
                    if (snap != null) list.Add(snap);
                }
            }
            return list;
        }

        public static BuildSnapshot ByMvid(IEnumerable<BuildSnapshot> builds, Guid mvid)
        {
            string key = mvid.ToString("D");
            return builds.FirstOrDefault(b => string.Equals(b.AssemblyCSharp?.Mvid, key, StringComparison.OrdinalIgnoreCase));
        }
    }
}
