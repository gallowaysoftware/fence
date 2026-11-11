using System;
using System.Collections.Generic;
using Fence.Inspect;
using HarmonyLib;

namespace Fence.Mod
{
    // Inspect advice patches (compat/hooks.json, feature advise.inspect). Observation only: the confront
    // and estimate methods appear here as patch targets, never as calls.
    static partial class FencePatches
    {
        internal static InspectAdvisor Inspector;

        static readonly List<Spec> InspectSpecs = new List<Spec>
        {
            new Spec
            {
                HookId = "inspect.open",
                Target = () => AccessTools.Method(typeof(InspectionUIManager), "OnInspectItem", new[] { typeof(GameItem) }),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(InspectOpenPostfix)),
            },
            new Spec
            {
                HookId = "human.inspect_verdict",
                Target = () => AccessTools.Method(typeof(InspectionUIManager), "OnConfront", new Type[0]),
                Prefix = () => AccessTools.Method(typeof(FencePatches), nameof(VerdictPrefix)),
            },
            new Spec
            {
                HookId = "inspect.confront_result",
                Target = () => AccessTools.Method(typeof(EmporiumAnalytics), "TrackConfront", new[] { typeof(string), typeof(string), typeof(string) }),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(ConfrontResultPostfix)),
            },
            new Spec
            {
                HookId = "inspect.estimate",
                Target = () => AccessTools.Method(typeof(AppraisalServiceHelper), "OnEstimate", new[] { typeof(GameItem), typeof(int) }),
                Prefix = () => AccessTools.Method(typeof(FencePatches), nameof(InspectEstimatePrefix)),
            },
            new Spec
            {
                HookId = "inspect.appraisal_too_high",
                Target = () => AccessTools.Method(typeof(AppraisalServiceHelper), "GetTooHighDialog", new Type[0]),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(TooHighPostfix)),
            },
            new Spec
            {
                HookId = "inspect.appraisal_too_low",
                Target = () => AccessTools.Method(typeof(AppraisalServiceHelper), "GetTooLowDialog", new Type[0]),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(TooLowPostfix)),
            },
            new Spec
            {
                HookId = "inspect.appraisal_accepted",
                Target = () => AccessTools.Method(typeof(AppraisalServiceHelper), "GetAcceptEstimateDialog", new[] { typeof(GameItem), typeof(int) }),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(AcceptedPostfix)),
            },
            new Spec
            {
                HookId = "inspect.appraisal_walkout",
                Target = () => AccessTools.Method(typeof(AppraisalServiceHelper), "GetPatienceRunOutDialog", new Type[0]),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(WalkoutPostfix)),
            },
        };

        static void Inspect(Action<InspectAdvisor> a)
        {
            var i = Inspector;
            if (i == null) return;
            try
            {
                a(i);
            }
            catch (Exception)
            {
                // Observation only: losing one inspect answer is better than breaking the inspection UI.
            }
        }

        static void InspectOpenPostfix(GameItem gameItem) => Inspect(i => i.OnInspected(gameItem));

        static void VerdictPrefix()
        {
            string issue = null;
            Inspect(i => issue = i.BeforeConfront());
            if (issue != null) Rec("inspect_verdict", r => r.InspectVerdict(GameInspect.Inspecting(), issue));
        }

        static void ConfrontResultPostfix(string itemId, string confrontType, string outcome) => Inspect(i => i.Confronted(itemId, confrontType, outcome));

        static void InspectEstimatePrefix(GameItem gameItem, int value) => Inspect(i => i.Estimating(gameItem, value));

        static void TooHighPostfix() => Inspect(i => i.AppraisalAnswer(AppraisalFeedback.TooHigh, null));

        static void TooLowPostfix() => Inspect(i => i.AppraisalAnswer(AppraisalFeedback.TooLow, null));

        static void AcceptedPostfix(GameItem gameItem) => Inspect(i => i.AppraisalAnswer(AppraisalFeedback.Accepted, gameItem));

        static void WalkoutPostfix() => Inspect(i => i.AppraisalAnswer(AppraisalFeedback.WalkedOut, null));
    }
}
