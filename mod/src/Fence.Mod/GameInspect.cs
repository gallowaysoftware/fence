using System;
using System.Collections.Generic;
using System.Runtime.CompilerServices;
using Fence.Inspect;

namespace Fence.Mod
{
    // IInspectItem over a live GameItem. Reads only; members are listed in compat/hooks.json under
    // advise.inspect, each call NoInlining so a removed member fails inside the caller's try/catch.
    sealed class GameInspectItem : IInspectItem
    {
        readonly GameItem item;

        public GameInspectItem(GameItem item) => this.item = item;

        [MethodImpl(MethodImplOptions.NoInlining)]
        public bool IsTag(string tag) => item.IsTag(tag);

        [MethodImpl(MethodImplOptions.NoInlining)]
        public int Int(string tag) => item.GetTagReadonly(tag).GetInt();

        [MethodImpl(MethodImplOptions.NoInlining)]
        public string Str(string tag) => item.GetTagReadonly(tag).GetString();

        [MethodImpl(MethodImplOptions.NoInlining)]
        public string ShownCondition(string category)
        {
            var f = category != null ? item.FindItemFeatureByCategory(category) : null;
            return f?.GetCurrentItemCondition()?.identifier;
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        public string RealCondition(string category)
        {
            var f = category != null ? item.FindItemFeatureByCategory(category) : null;
            return f?.realCondition?.identifier;
        }
    }

    static class GameInspect
    {
        [MethodImpl(MethodImplOptions.NoInlining)]
        public static bool ConfrontPanelOpen()
        {
            var ui = InspectionUIManager.Instance;
            return ui != null && ui.confrontPanel != null && ui.confrontPanel.activeSelf;
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static bool AppraisalService(StoreClient c) => c != null && c.clientIntent == StoreClient.ClientIntent.APPRAISAL_SERVICE;

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static bool Owned(GameItem item) => GeneralHelper.IsItemOwned(item);

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static string Identifier(GameItem item) => item.identifier;

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static string Name(GameItem item) => item.GetDisplayName(false);

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static long Price(GameItem item) => item.GetNegociatedValue();

        // HIDDEN: the appraisal truth, for labels only.
        [MethodImpl(MethodImplOptions.NoInlining)]
        public static int TrueValue(GameItem item) => AppraisalHelper.GetCorrectBaseValue(item);

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static AppraisalTerms Terms(GameItem item)
        {
            var t = new AppraisalTerms
            {
                TheirFigure = item.GetTagReadonly("APPRAISAL_CLIENT_VALUE").GetInt(),
                Rank = PlayerStore.Instance != null ? PlayerStore.Instance.appraiserLevel : 0,
                BaseFee = AppraisalRankHelper.GetBaseFee(),
            };
            for (int grade = 1; grade <= 5; grade++) t.FeePct[grade] = AppraisalRankHelper.GetFeePercent(grade);
            return t;
        }

        // The confront panel's toggles, for the verdict the player submits.
        [MethodImpl(MethodImplOptions.NoInlining)]
        public static List<bool> Toggles()
        {
            var ui = InspectionUIManager.Instance;
            if (ui == null) return null;
            return new List<bool> { ui.toggle1.isOn, ui.toggle2.isOn, ui.toggle3.isOn, ui.toggle4.isOn, ui.toggle5.isOn };
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static GameItem Inspecting() => InspectionUIManager.Instance != null ? InspectionUIManager.Instance.currentInspectingItem : null;
    }
}
