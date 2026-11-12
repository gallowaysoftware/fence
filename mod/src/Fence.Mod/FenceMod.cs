using System;
using System.Collections;
using System.Collections.Generic;
using System.Linq;
using Fence.Aug;
using Fence.Autopilot;
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
        Func<string> augDetection = () => AugDetection.Tells;
        Func<string> stageShortlist = () => StageShortlist.DefaultSize.ToString();
        Func<bool> autopilotOn = () => false;
        Func<string> autopilotPace = () => Pace.DefaultMs.ToString();
        Func<string> autopilotHold = () => Pace.DefaultHoldMs.ToString();
        Func<string> autopilotDirectives = () => AutopilotDriver.DefaultDirectives;
        Func<string> autopilotGun = () => AutopilotDriver.GunShoot;
        KeyCode autopilotKey = KeyCode.F10;
        AutopilotDriver autopilot;
        bool coroutinesViaModHelper = true;
        GameObject hostObject;
        FenceHost host;
        bool gameDayUsable;
        BarterAdvisor advisor;
        InspectAdvisor inspector;
        AugAdvisor augs;
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
                    augDetection = GameAccess.DefineString("aug_detection", AugDetection.Tells, "Augs", "Aug detection",
                        "tells: the model sees only the visible tells, in words. perfect: it is told which customers are augs (analysis only; never on stream). The autopilot always uses tells.");
                    stageShortlist = GameAccess.DefineString("stage_shortlist", StageShortlist.DefaultSize.ToString(), "Advice", "Staging shortlist size",
                        "How many items the overlay lists at once for a buying customer, from the mod's own ranking. 0 turns the list off.");
                    autopilotOn = GameAccess.DefineBool("autopilot", false, "Autopilot", "Let fence play the shop",
                        "On: after you press Resume autopilot in the overlay, fence plays the day by the gym executor's rules, one action at a time. "
                        + "Pause, the key bind or any move of your own stops it. Back up your saves first.");
                    autopilotPace = GameAccess.DefineString("autopilot_pace_ms", Pace.DefaultMs.ToString(), "Autopilot", "Autopilot pace (ms)",
                        "Time between actions; item moves take half of it, dialogue choices longer for long lines.");
                    autopilotHold = GameAccess.DefineString("autopilot_hold_ms", Pace.DefaultHoldMs.ToString(), "Autopilot", "Hold before big decisions (ms)",
                        "How long the morning report and the landlord's, loan shark's and permit official's questions stay on screen.");
                    autopilotDirectives = GameAccess.DefineString("autopilot_directives", AutopilotDriver.DefaultDirectives, "Autopilot", "Base directives (JSON)",
                        "The executor's directives (docs/PROTOCOL.md) under the strategist's plan for the day.");
                    autopilotGun = GameAccess.DefineString("autopilot_gun", AutopilotDriver.GunShoot, "Autopilot", "Autopilot on an aug",
                        "shoot: on a sure visible tell the autopilot raises the gun and fires. pause: it stops and hands the customer to you.");
                }
                catch (Exception e)
                {
                    foreach (var id in new[] { "ModHelper.GetConfig", "ModConfig.Define(bool)", "ModConfig.Define(string)", "ConfigEntry.Value" })
                        compat.MarkBindFailed(id, e.GetType().Name);
                    log.Warn("config unavailable, using defaults: " + e.Message);
                }
            }
            if (compat.IsUsable("ModHelper.AddGameOptionKeyBind"))
            {
                try
                {
                    GameAccess.AddKeyBind("autopilot_key", "Autopilot", "Pause / resume autopilot", () => autopilotKey, k => autopilotKey = k,
                        "Stops or restarts the autopilot, like the overlay's button.");
                }
                catch (Exception e)
                {
                    compat.MarkBindFailed("ModHelper.AddGameOptionKeyBind", e.GetType().Name);
                    log.Warn("no autopilot key bind: " + e.Message);
                }
            }
            coroutinesViaModHelper = compat.IsUsable("ModHelper.StartCoroutine");
            gameDayUsable = compat.IsUsable("StoreStation.Instance") && compat.IsUsable("StoreStation.dayCounter");
        }

        public void OnEnable()
        {
            AugDetection.Current = AugDetection.Parse(SafeSetting(augDetection));
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
            stopping = true;
            client?.Stop();
            advisor?.Stop();
            autopilot?.Stop();
            FencePatches.Advisor = null;
            FencePatches.Inspector = null;
            FencePatches.Augs = null;
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
        const string InspectFeature = "advise.inspect";
        const string AugFeature = "advise.aug";
        const string AugRecordFeature = "record.aug";
        const string DisplayFeature = "advise.display";
        const string TooltipHook = "display.tooltip";
        const string OverlayFeature = "display.overlay";

        void StartAdvisor()
        {
            advisor = new BarterAdvisor(log, session, client, StartCoroutine, OnGameEvent, Record, () => actEnabled(), compat.IsUsable, recorder);
            inspector = new InspectAdvisor(log, session, client, advisor.InForce, advisor.NoteOutcome);
            augs = new AugAdvisor(log, session, client, Record, advisor.NoteOutcome);
            advisor.Augs = augs;
            advisor.InspectLine = () =>
            {
                string a = augs.OverlayLine();
                return a.Length > 0 ? a : inspector.OverlayLine();
            };
            if (recorder != null)
            {
                recorder.InspectAdvice = inspector.InForce;
                recorder.InspectVerdictFor = inspector.VerdictFor;
                recorder.AugAdvice = augs.InForce;
            }
            FencePatches.Advisor = advisor;
            FencePatches.Recorder = recorder;
            FencePatches.Inspector = inspector;
            FencePatches.Augs = augs;
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
            advisor.ShortlistSize = () => StageShortlist.Parse(SafeSetting(stageShortlist));
            advisor.StageEnabled = advisor.AdviseEnabled && !disabled.Contains(StageFeature);
            if (advisor.AdviseEnabled && !advisor.StageEnabled) log.Warn("staging advice disabled: a member it reads is missing (see compat warnings)");
            inspector.Enabled = advisor.AdviseEnabled && !disabled.Contains(InspectFeature);
            augs.Enabled = advisor.AdviseEnabled && !disabled.Contains(AugFeature);
            if (advisor.AdviseEnabled && !augs.Enabled) log.Warn("aug advice disabled: a member it reads is missing (see compat warnings)");
            augs.RecordEnabled = !disabled.Contains(AugRecordFeature);
            if (advisor.AdviseEnabled && !inspector.Enabled) log.Warn("inspect advice disabled: a member it reads is missing (see compat warnings)");
            advisor.DisplayEnabled = !disabled.Contains(DisplayFeature) && unbindTooltip != null;
            advisor.OverlayEnabled = !disabled.Contains(OverlayFeature);
            if (!advisor.OverlayEnabled) log.Warn("no overlay window: a CustomUI member it needs is missing (see compat warnings)");
            if (!advisor.AdviseEnabled) log.Warn("barter advice disabled: a member it reads is missing (see compat warnings)");
            StartAutopilot(disabled);
        }

        // advise.aug: without the tells the autopilot could not see an aug and would serve every one.
        static readonly string[] AutopilotFeatures = { "autopilot.hands", "autopilot.executor", "act.barter", "advise.barter", "advise.aug" };
        const string GunFeature = "autopilot.gun";
        const string MachinesFeature = "autopilot.machines";
        const string GunOrdersFeature = "autopilot.gun_orders";

        void StartAutopilot(HashSet<string> disabled)
        {
            autopilot = new AutopilotDriver(log, StartCoroutine, Record, recorder, advisor, () => autopilotOn(), () => autopilotPace(), () => autopilotHold(), () => autopilotDirectives());
            autopilot.GunSetting = () => autopilotGun();
            autopilot.GunUsable = !disabled.Contains(GunFeature);
            if (!autopilot.GunUsable) log.Warn("autopilot: the gun's members are missing; on an aug it hands over (see compat warnings)");
            autopilot.MachinesUsable = !disabled.Contains(MachinesFeature);
            if (!autopilot.MachinesUsable) log.Warn("autopilot: the machine buttons' members are missing; the lab, the RAS blend and the bottle printer do nothing live (see compat warnings)");
            autopilot.GunOrdersUsable = !disabled.Contains(GunOrdersFeature);
            if (!autopilot.GunOrdersUsable) log.Warn("autopilot: the gun order's members are missing; order clients are sent away (see compat warnings)");
            var missing = AutopilotFeatures.Where(disabled.Contains).ToList();
            if (missing.Count > 0)
            {
                autopilot.Unavailable("game members missing (" + string.Join(", ", missing) + "), see compat warnings");
                log.Warn("autopilot unavailable: " + string.Join(", ", missing) + " disabled");
            }
            advisor.AutopilotStatus = autopilot.Status;
            advisor.AutopilotToggle = by => autopilot.Toggle(by);
            if (recorder != null)
            {
                recorder.ClaimHuman = autopilot.ClaimHuman;
                recorder.ExecutorActor = () => autopilot.Executing ? Actors.Autopilot : Actors.Mod;
            }
        }

        bool keyPolling;

        // GetKeyDown is true for one frame only, so the key is read every frame.
        IEnumerator KeyPoll()
        {
            try
            {
                while (autopilot != null && !stopping)
                {
                    try
                    {
                        if (autopilot.Enabled && GameAccess.KeyDown(autopilotKey)) autopilot.Toggle("key " + autopilotKey);
                    }
                    catch (Exception e)
                    {
                        log.Warn("autopilot key bind off until restart: " + e.Message);
                        yield break;
                    }
                    yield return null;
                }
            }
            finally
            {
                keyPolling = false;
            }
        }

        bool stopping;

        void OnGameEvent(string kind, JObject data)
        {
            if (!Send(kind, data, null)) return;
            recorder?.OnGameEvent(kind);
            advisor?.OnGameEvent(kind);
            advisor?.EnsurePolling();
            autopilot?.EnsurePolling();
            if (autopilot != null && !keyPolling && !stopping) keyPolling = StartCoroutine(KeyPoll());
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
            AugDetection.Current = AugDetection.Parse(SafeSetting(augDetection));
            session.Enqueue(kind, GameDay(), data, hidden, BridgeClient.NowMs());
            // Hello waits for a loaded game (docs/PROTOCOL.md) unless that hook is gone.
            if (kind == "game_loaded" || kind == RecordKinds.RunStart || !compat.IsUsable(GameLoadedHook)) session.RequestHello();
            client.EnsurePumping();
            return true;
        }

        static string SafeSetting(Func<string> setting)
        {
            try
            {
                return setting();
            }
            catch (Exception)
            {
                return null;
            }
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
