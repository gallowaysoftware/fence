using System;
using System.Collections.Generic;
using System.Runtime.CompilerServices;
using Fence.Barter;
using UnityEngine;

namespace Fence.Mod
{
    // The persistent "fence" window, built with the game's CustomUIManager on the store scene's front
    // layer. Scene layers die with the scene, so Update rebuilds whenever the window is gone and the
    // layer exists (after every game load); widgets are then updated by id, only when their text changed.
    sealed class FenceOverlay
    {
        public const string WindowId = "fence.overlay";
        const float Width = 440f, Height = 760f, CollapsedHeight = 120f;
        const long RebuildEveryMs = 2000;

        static readonly string[] Body =
        {
            "h_customer", "customer", "status", "move", "inspect", "rationale", "options", "outcome", "h_today", "summary", "today", "lessons",
        };

        static readonly string[] PlanBody = { "h_plan", "plan_window", "plan_toggle", "plan" };

        readonly FenceLog log;
        readonly Action askAgain;
        readonly Action togglePause;
        readonly Dictionary<string, string> shown = new Dictionary<string, string>();
        object window;
        bool collapsed;
        bool planHidden;
        long lastBuildMs = long.MinValue / 2;
        bool broken;

        public FenceOverlay(FenceLog log, Action askAgain, Action togglePause)
        {
            this.log = log;
            this.askAgain = askAgain;
            this.togglePause = togglePause;
        }

        public bool Enabled { get; set; }

        public void Update(OverlayView view, long nowMs)
        {
            if (!Enabled || broken || view == null) return;
            try
            {
                if (window == null || !OverlayUi.IsAlive(window))
                {
                    window = null;
                    if (nowMs - lastBuildMs < RebuildEveryMs || !OverlayUi.SceneReady()) return;
                    lastBuildMs = nowMs;
                    shown.Clear();
                    window = OverlayUi.Build(WindowId, Width, Height, () => Safe(askAgain), () => Safe(ToggleCollapsed), () => Safe(togglePause), () => Safe(TogglePlan));
                    if (window == null) return;
                    ApplyCollapsed();
                }
                Set("header", collapsed ? view.Bar : view.Header);
                // Always visible, collapsed or not: a gap in the data must be noticed.
                if (Changed("recording", view.NotRecording + view.Recording))
                {
                    OverlayUi.SetText(window, view.NotRecording ? "not_recording" : "recording", view.Recording);
                    OverlayUi.SetVisible(window, "not_recording", view.NotRecording);
                    OverlayUi.SetVisible(window, "recording", !view.NotRecording);
                }
                // The autopilot's state, collapsed or not: a stop that needs the human must be seen.
                string auto = view.Autopilot ?? "";
                if (Changed("autopilot", auto + view.AutopilotAlert))
                {
                    if (auto.Length > 0) OverlayUi.SetText(window, view.AutopilotAlert ? "autopilot_alert" : "autopilot", auto);
                    OverlayUi.SetVisible(window, "autopilot_alert", auto.Length > 0 && view.AutopilotAlert);
                    OverlayUi.SetVisible(window, "autopilot", auto.Length > 0 && !view.AutopilotAlert);
                }
                bool hasPlan = !string.IsNullOrEmpty(view.Plan) && !collapsed;
                bool hasWindow = hasPlan && !string.IsNullOrEmpty(view.PlanWindow);
                if (Changed("plan_vis", hasPlan + "|" + hasWindow + "|" + planHidden))
                {
                    OverlayUi.SetVisible(window, "h_plan", hasPlan);
                    OverlayUi.SetVisible(window, "plan_toggle", hasPlan);
                    OverlayUi.SetVisible(window, "plan_window", hasWindow);
                    OverlayUi.SetVisible(window, "plan", hasPlan && !planHidden);
                    OverlayUi.SetText(window, "plan_toggle", planHidden ? "Show plan" : "Hide plan");
                }
                if (hasPlan)
                {
                    SetPlain("h_plan", view.PlanHeader);
                    SetPlain("plan_window", view.PlanWindow);
                    SetPlain("plan", view.Plan);
                }
                Set("customer", view.Customer);
                Set("status", "Status: " + view.Status);
                Set("move", view.Move);
                Set("inspect", view.Inspect);
                Set("rationale", view.Rationale);
                Set("options", view.Options);
                Set("outcome", view.Outcome);
                Set("summary", view.Summary);
                Set("today", view.Today);
                Set("lessons", view.Lessons);
                string pause = view.PauseLabel ?? "";
                if (Changed("pause", pause + view.PauseInteractable))
                {
                    OverlayUi.SetVisible(window, "pause", view.PauseLabel != null);
                    if (view.PauseLabel != null)
                    {
                        OverlayUi.SetText(window, "pause", view.PauseLabel);
                        OverlayUi.SetInteractable(window, "pause", view.PauseInteractable);
                    }
                }
            }
            catch (Exception e)
            {
                broken = true;
                log.Warn("overlay off until restart: " + e.GetType().Name + ": " + e.Message);
            }
        }

        public void Close()
        {
            try
            {
                if (window != null) OverlayUi.Close(window);
            }
            catch (Exception)
            {
            }
            window = null;
        }

        void Set(string id, string text)
        {
            text = text ?? "";
            if (!Changed(id, text)) return;
            OverlayUi.SetText(window, id, text);
            if (!collapsed && id != "header") OverlayUi.SetVisible(window, id, text.Length > 0);
        }

        void SetPlain(string id, string text)
        {
            if (Changed(id, text ?? "")) OverlayUi.SetText(window, id, text ?? "");
        }

        void TogglePlan()
        {
            planHidden = !planHidden;
            shown.Remove("plan_vis");
        }

        bool Changed(string id, string value)
        {
            if (shown.TryGetValue(id, out var old) && old == value) return false;
            shown[id] = value;
            return true;
        }

        void ToggleCollapsed()
        {
            collapsed = !collapsed;
            shown.Clear();
            ApplyCollapsed();
        }

        void ApplyCollapsed()
        {
            foreach (var id in Body) OverlayUi.SetVisible(window, id, !collapsed);
            foreach (var id in PlanBody) OverlayUi.SetVisible(window, id, false);
            shown.Remove("plan_vis");
            OverlayUi.SetText(window, "hide", collapsed ? "Show" : "Hide");
            OverlayUi.SetSize(window, Width, collapsed ? CollapsedHeight : Height);
        }

        // Button callbacks run inside the game's UI event loop; never throw into it.
        void Safe(Action a)
        {
            try
            {
                a?.Invoke();
            }
            catch (Exception e)
            {
                log.Warn("overlay button failed: " + e.Message);
            }
        }
    }

    // Every CustomUI call, NoInlining so a member an update removed fails here (display.overlay in
    // compat/hooks.json), inside FenceOverlay's try/catch. The window is passed as object so a missing
    // CustomUIWindow type cannot break the loading of FenceOverlay.
    static class OverlayUi
    {
        const string Layer = "front";

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static bool SceneReady() => CustomUIManager.Instance != null && CustomUIManager.HasLayer(Layer);

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static object Build(string id, float width, float height, Action askAgain, Action hide, Action pause, Action togglePlan)
        {
            var b = CustomUIManager.Window(id, "fence", Layer);
            if (b == null) return null;
            // Left edge, upper half: clear of the counter and the tablet in the middle and right.
            b.SetSize(width, height)
                .SetPosition(new Vector2(-700f, 160f))
                .SetDraggable(true)
                .AddWarning("", "not_recording")
                .AddLabel("", "recording")
                .AddWarning("", "autopilot_alert")
                .AddLabel("", "autopilot")
                .AddLabel("", "header")
                .BeginRow()
                    .AddButton("Ask again", askAgain, "ask")
                    .AddButton("Pause acting", pause, "pause")
                    .AddButton("Hide", hide, "hide")
                .End()
                .AddHeader("Today's plan", "h_plan")
                .AddLabel("", "plan_window")
                .AddButton("Hide plan", togglePlan, "plan_toggle")
                .BeginScroll(150f)
                    .AddLabel("", "plan")
                .End()
                .AddHeader("This customer", "h_customer")
                .AddLabel("", "customer")
                .AddLabel("", "status")
                .AddLabel("", "move")
                .AddLabel("", "inspect")
                .AddLabel("", "rationale")
                .AddLabel("", "options")
                .AddLabel("", "outcome")
                .AddHeader("Today", "h_today")
                .BeginScroll(170f)
                    .AddLabel("", "summary")
                    .AddLabel("", "today")
                    .AddLabel("", "lessons")
                .End();
            return b.Show();
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static bool IsAlive(object window) => ((CustomUIWindow)window).IsAlive;

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static void SetText(object window, string id, string text) => ((CustomUIWindow)window).Get(id)?.SetText(text);

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static void SetVisible(object window, string id, bool visible) => ((CustomUIWindow)window).Get(id)?.SetVisible(visible);

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static void SetInteractable(object window, string id, bool on) => ((CustomUIWindow)window).Get(id)?.SetInteractable(on);

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static void SetSize(object window, float width, float height) => ((CustomUIWindow)window).SetSize(width, height);

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static void Close(object window) => ((CustomUIWindow)window).Close();
    }
}
