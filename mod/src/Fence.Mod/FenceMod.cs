using System;
using System.Collections;
using System.Collections.Generic;
using System.Linq;
using Fence.Barter;
using Fence.Bridge;
using Fence.Compat;
using Fence.Protocol;
using Fence.Recording;
using Newtonsoft.Json.Linq;
using UnityEngine;

namespace Fence.Mod
{
    // Forwards ModHook events to the bridge (P0), advises on ordinary barter through the overlay (P1)
    // and, only when the bridge is in act mode AND the act_enabled setting is on, executes the
    // advised barter move (P2, BarterExecutor).
    public sealed class FenceMod : IMod
    {
        const string DefaultBridgeUrl = "http://127.0.0.1:7341";
        const string GameLoadedHook = "ModHook.OnGameLoadedLate";

        readonly FenceLog log = new FenceLog();
        readonly List<Action> unbinds = new List<Action>();
        ModManifest manifest;
        HookManifest hooks;
        CompatReport compat;
        BridgeSession session;
        BridgeClient client;
        Func<bool> enabled = () => true;
        Func<string> bridgeUrl = () => DefaultBridgeUrl;
        Func<bool> actEnabled = () => false;
        bool coroutinesViaModHelper = true;
        GameObject hostObject;
        FenceHost host;
        bool gameDayUsable;
        BarterAdvisor advisor;
        Recorder recorder;
        HashSet<string> disabledFeatures = new HashSet<string>();
        HarmonyLib.Harmony harmony;
        Action unbindTooltip;

        public void Init(ModManifest manifest)
        {
            this.manifest = manifest;
            log.Attach(manifest);
            try
            {
                hooks = HookManifest.LoadEmbedded();
                compat = CompatChecker.Check(hooks, new RuntimeResolver());
            }
            catch (Exception e)
            {
                log.Error("compat check failed: " + e);
                hooks = hooks ?? new HookManifest();
                compat = new CompatReport();
            }
            foreach (var r in compat.Failures) log.Warn("compat: " + r.Hook.Display + " " + r.Status + (r.Detail != null ? " (" + r.Detail + ")" : ""));

            if (compat.IsUsable("ModHelper.GetConfig") && compat.IsUsable("ModConfig.Define(bool)") && compat.IsUsable("ModConfig.Define(string)") && compat.IsUsable("ConfigEntry.Value"))
            {
                try
                {
                    enabled = GameAccess.DefineBool("enabled", true, "Bridge", "Talk to the fence bridge",
                        "Off: the mod stays loaded but sends nothing.");
                    bridgeUrl = GameAccess.DefineString("bridge_url", DefaultBridgeUrl, "Bridge", "Bridge URL",
                        "Where the fence bridge listens. Plain http on localhost.");
                    actEnabled = GameAccess.DefineBool("act_enabled", false, "Act", "Let fence play the counter",
                        "On, and the bridge in act mode: fence sells, buys, haggles and dismisses ordinary customers itself. "
                        + "Off: it only advises. Back up your saves first.");
                }
                catch (Exception e)
                {
                    foreach (var id in new[] { "ModHelper.GetConfig", "ModConfig.Define(bool)", "ModConfig.Define(string)", "ConfigEntry.Value" })
                        compat.MarkBindFailed(id, e.GetType().Name);
                    log.Warn("config unavailable, using defaults: " + e.Message);
                }
            }
            coroutinesViaModHelper = compat.IsUsable("ModHelper.StartCoroutine");
            gameDayUsable = compat.IsUsable("StoreStation.Instance") && compat.IsUsable("StoreStation.dayCounter");
        }

        public void OnEnable()
        {
            session = new BridgeSession();
            client = new BridgeClient(session, log, () => bridgeUrl(), BuildHello, StartCoroutine);
            BindEvents();
            recorder = new Recorder(log, Record, f => !disabledFeatures.Contains(f));
            StartAdvisor();
            recorder.Subscribe(compat.IsUsable);
            log.Info("enabled; bridge " + bridgeUrl() + ", " + HookBindings.Events.Count + " event hooks, compat " + (compat.Ok ? "ok" : "degraded"));
        }

        public void OnDisable()
        {
            client?.Stop();
            advisor?.Stop();
            FencePatches.Advisor = null;
            FencePatches.Recorder = null;
            recorder?.Unsubscribe();
            try
            {
                unbindTooltip?.Invoke();
            }
            catch (Exception)
            {
            }
            unbindTooltip = null;
            if (harmony != null)
            {
                try
                {
                    FencePatches.Remove(harmony);
                }
                catch (Exception e)
                {
                    log.Warn("could not remove patches: " + e.Message);
                }
                harmony = null;
            }
            foreach (var unbind in unbinds)
            {
                try
                {
                    unbind();
                }
                catch (Exception)
                {
                    // The loader removes our ModHook handlers itself when it stops a mod.
                }
            }
            unbinds.Clear();
            if (coroutinesViaModHelper)
            {
                try
                {
                    GameAccess.StopModCoroutines();
                }
                catch (Exception)
                {
                }
            }
            if (hostObject != null) UnityEngine.Object.Destroy(hostObject);
            hostObject = null;
            host = null;
            log.Info("disabled");
        }

        void BindEvents()
        {
            foreach (var hook in hooks.Hooks.Where(h => h.Kind == MemberKinds.Event && h.EventKind != null))
            {
                if (!HookBindings.Events.TryGetValue(hook.Id, out var bind))
                {
                    compat.MarkBindFailed(hook.Id, "no binding in this mod build");
                    continue;
                }
                if (!compat.IsUsable(hook.Id)) continue;
                try
                {
                    bind(new EventBinder(hook.EventKind, OnGameEvent, unbinds));
                }
                catch (Exception e)
                {
                    compat.MarkBindFailed(hook.Id, e.GetType().Name + ": " + e.Message);
                    log.Warn("could not subscribe " + hook.Display + ": " + e.Message);
                }
            }
        }

        const string AdviseFeature = "advise.barter";
        const string StageFeature = "advise.stage";
        const string DisplayFeature = "advise.display";
        const string TooltipHook = "display.tooltip";
        const string OverlayFeature = "display.overlay";

        void StartAdvisor()
        {
            advisor = new BarterAdvisor(log, session, client, StartCoroutine, OnGameEvent, Record, () => actEnabled(), compat.IsUsable, recorder);
            FencePatches.Advisor = advisor;
            FencePatches.Recorder = recorder;
            try
            {
                harmony = GameAccess.NewHarmony(FencePatches.HarmonyId);
                FencePatches.Apply(harmony, compat, log);
            }
            catch (Exception e)
            {
                foreach (var id in FencePatches.HookIds()) compat.MarkBindFailed(id, "Harmony unavailable: " + e.GetType().Name);
                log.Warn("Harmony unavailable, no outcomes: " + e.Message);
            }
            if (compat.IsUsable(TooltipHook))
            {
                try
                {
                    unbindTooltip = GameAccess.SubscribeTooltipLate((b, item) => advisor.AppendTooltip(b, item));
                }
                catch (Exception e)
                {
                    compat.MarkBindFailed(TooltipHook, e.GetType().Name + ": " + e.Message);
                    log.Warn("no in-game advice display: " + e.Message);
                }
            }
            var disabled = compat.DisabledFeatures();
            disabledFeatures = disabled;
            foreach (var f in disabled.Where(f => f.StartsWith("record.")).OrderBy(f => f, StringComparer.Ordinal)) log.Warn("recording without " + f + " (a member it needs is missing, see compat warnings)");
            advisor.AdviseEnabled = !disabled.Contains(AdviseFeature);
            advisor.StageEnabled = advisor.AdviseEnabled && !disabled.Contains(StageFeature);
            if (advisor.AdviseEnabled && !advisor.StageEnabled) log.Warn("staging advice disabled: a member it reads is missing (see compat warnings)");
            advisor.DisplayEnabled = !disabled.Contains(DisplayFeature) && unbindTooltip != null;
            advisor.OverlayEnabled = !disabled.Contains(OverlayFeature);
            if (!advisor.OverlayEnabled) log.Warn("no overlay window: a CustomUI member it needs is missing (see compat warnings)");
            if (!advisor.AdviseEnabled) log.Warn("barter advice disabled: a member it reads is missing (see compat warnings)");
        }

        void OnGameEvent(string kind, JObject data)
        {
            if (!Send(kind, data, null)) return;
            recorder?.OnGameEvent(kind);
            advisor?.OnGameEvent(kind);
            advisor?.EnsurePolling();
        }

        void Record(string kind, JObject data, JObject hidden) => Send(kind, data, hidden);

        bool Send(string kind, JObject data, JObject hidden)
        {
            if (!enabled())
            {
                session.Enabled = false;
                return false;
            }
            session.Enabled = true;
            session.Enqueue(kind, GameDay(), data, hidden, BridgeClient.NowMs());
            // Hello waits for a loaded game (docs/PROTOCOL.md) unless that hook is gone.
            if (kind == "game_loaded" || kind == RecordKinds.RunStart || !compat.IsUsable(GameLoadedHook)) session.RequestHello();
            client.EnsurePumping();
            return true;
        }

        int GameDay()
        {
            if (!gameDayUsable) return 0;
            try
            {
                return GameAccess.DayCounter();
            }
            catch (Exception e)
            {
                gameDayUsable = false;
                compat.MarkBindFailed("StoreStation.dayCounter", e.GetType().Name);
                log.Warn("game day unavailable: " + e.Message);
                return 0;
            }
        }

        HelloRequest BuildHello()
        {
            string version = null;
            if (compat.IsUsable("ModManifest.ModVersion"))
            {
                try
                {
                    version = GameAccess.ManifestVersion(manifest);
                }
                catch (Exception)
                {
                }
            }
            return new HelloRequest
            {
                ModVersion = string.IsNullOrEmpty(version) ? "0.0.0" : version,
                Game = new GameInfo
                {
                    Version = Application.version,
                    Unity = Application.unityVersion,
                    AssemblySha256 = KnownAssemblySha256(),
                    AssemblyMvid = GameAssemblyMvid(),
                },
                Compat = compat.ToHello(),
            };
        }

        // The mod has no `files` capability, so it cannot hash Assembly-CSharp.dll; it recognises a
        // build Fence.Compat verified by MVID instead. Empty means "not a verified build".
        string KnownAssemblySha256()
        {
            try
            {
                var known = KnownBuilds.ByMvid(KnownBuilds.LoadEmbedded(), GameAccess.GameAssemblyMvid());
                if (known != null) return known.AssemblyCSharp.Sha256;
                log.Warn("this game build was never checked by Fence.Compat (run it and rebuild the mod)");
            }
            catch (Exception e)
            {
                log.Warn("could not identify the game build: " + e.Message);
            }
            return "";
        }

        // Identifies the build even when Fence.Compat has not recorded it yet.
        string GameAssemblyMvid()
        {
            try { return GameAccess.GameAssemblyMvid().ToString("D"); }
            catch (Exception) { return null; }
        }

        bool StartCoroutine(IEnumerator routine)
        {
            if (coroutinesViaModHelper)
            {
                try
                {
                    if (GameAccess.StartModCoroutine(routine) != null) return true;
                }
                catch (Exception e)
                {
                    coroutinesViaModHelper = false;
                    compat.MarkBindFailed("ModHelper.StartCoroutine", e.GetType().Name);
                    log.Warn("ModHelper.StartCoroutine unavailable, using own host: " + e.Message);
                }
            }
            if (host == null)
            {
                hostObject = new GameObject("fence.host") { hideFlags = HideFlags.HideInHierarchy };
                UnityEngine.Object.DontDestroyOnLoad(hostObject);
                host = hostObject.AddComponent<FenceHost>();
            }
            return host.Run(routine) != null;
        }
    }
}
