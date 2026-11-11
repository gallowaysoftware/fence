using System;
using System.Collections;
using System.Runtime.CompilerServices;
using UnityEngine;

namespace Fence.Mod
{
    // Every touch of a game API that could vanish in an update lives behind a NoInlining method,
    // so a missing member surfaces as an exception at the call site we wrap in try/catch, not as
    // a load failure of a larger class.
    static class GameAccess
    {
        public const string ModId = "fence";

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static object NewModLog(ModManifest manifest) => new ModLog(manifest);

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static void ModLogWrite(object log, int level, string message)
        {
            var l = (ModLog)log;
            if (level == 0) l.Log(message);
            else if (level == 1) l.Warning(message);
            else l.Error(message);
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static string ManifestVersion(ModManifest manifest) => manifest.ModVersion;

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static Func<bool> DefineBool(string key, bool fallback, string section, string label, string tooltip)
        {
            var entry = ModHelper.GetConfig(ModId).Define(key, fallback, section, () => label, () => tooltip, false);
            return () => entry.Value;
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static Func<string> DefineString(string key, string fallback, string section, string label, string tooltip)
        {
            var entry = ModHelper.GetConfig(ModId).Define(key, fallback, section, () => label, null, () => tooltip, false);
            return () => entry.Value;
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static void AddKeyBind(string key, string section, string label, Func<KeyCode> get, Action<KeyCode> set, string tooltip) =>
            ModHelper.AddGameOptionKeyBind(ModId, section, key, () => label, get, set, () => tooltip);

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static bool KeyDown(KeyCode key) => key != KeyCode.None && Input.GetKeyDown(key);

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static Coroutine StartModCoroutine(IEnumerator routine) => ModHelper.StartCoroutine(ModId, routine);

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static void StopModCoroutines() => ModHelper.StopAllCoroutines(ModId);

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static int DayCounter()
        {
            var station = StoreStation.Instance;
            return station != null ? station.dayCounter : 0;
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static HarmonyLib.Harmony NewHarmony(string id) => new HarmonyLib.Harmony(id);

        // Returns the unsubscribe action.
        [MethodImpl(MethodImplOptions.NoInlining)]
        public static Action SubscribeTooltipLate(Action<RichTextBuilder, GameItem> handler)
        {
            ModHook.OnCreateTooltipLate += handler;
            return () => ModHook.OnCreateTooltipLate -= handler;
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static Action SubscribeWakingUpEarly(Action handler)
        {
            ModHook.OnWakingUpEarly += handler;
            return () => ModHook.OnWakingUpEarly -= handler;
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static Action SubscribeGoingSleepEarly(Action handler)
        {
            ModHook.OnGoingSleepEarly += handler;
            return () => ModHook.OnGoingSleepEarly -= handler;
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static Action SubscribeGameLoadedInit(Action handler)
        {
            ModHook.OnGameLoadedInit += handler;
            return () => ModHook.OnGameLoadedInit -= handler;
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static Guid GameAssemblyMvid() => typeof(ModHook).Assembly.ManifestModule.ModuleVersionId;
    }

    sealed class FenceLog
    {
        object modLog;

        public void Attach(ModManifest manifest)
        {
            try
            {
                modLog = GameAccess.NewModLog(manifest);
            }
            catch (Exception e)
            {
                Debug.LogWarning("[fence] ModLog unavailable, using Debug.Log: " + e.Message);
            }
        }

        public void Info(string m) => Write(0, m);
        public void Warn(string m) => Write(1, m);
        public void Error(string m) => Write(2, m);

        void Write(int level, string message)
        {
            if (modLog != null)
            {
                try
                {
                    GameAccess.ModLogWrite(modLog, level, message);
                    return;
                }
                catch (Exception)
                {
                    modLog = null;
                }
            }
            string line = "[fence] " + message;
            if (level == 0) Debug.Log(line);
            else if (level == 1) Debug.LogWarning(line);
            else Debug.LogError(line);
        }
    }

    // A component of our own: StartCoroutine on `this` is free under the scanner, and the loader
    // destroys it when it stops the mod. Used only if ModHelper.StartCoroutine is unavailable.
    sealed class FenceHost : MonoBehaviour
    {
        public Coroutine Run(IEnumerator routine) => StartCoroutine(routine);
    }
}
