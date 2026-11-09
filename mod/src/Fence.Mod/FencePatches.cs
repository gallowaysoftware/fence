using System;
using System.Collections.Generic;
using System.Linq;
using System.Reflection;
using Fence.Barter;
using Fence.Compat;
using HarmonyLib;

namespace Fence.Mod
{
    // Observation-only Harmony patches, applied one by one by hand (no [HarmonyPatch] attributes, so
    // the loader's PatchAll applies nothing): a target an update removed costs only its own hook,
    // reported in hello.compat. Every handler swallows its exceptions; a throwing patch would break
    // the game method it wraps. P1 never calls a game action: the bargain/dismiss targets below are
    // patched to watch the human, and their names exist here only as patch-target strings.
    static partial class FencePatches
    {
        // The loader unpatches "mod.<ID>" when it pauses or kill-switches a mod (ModLoader), so
        // sharing that id means our patches go with it even if OnDisable never runs.
        public const string HarmonyId = "mod.fence";

        internal static BarterAdvisor Advisor;

        sealed class Spec
        {
            public string HookId;
            public Func<MethodBase> Target;
            public Func<MethodInfo> Prefix;
            public Func<MethodInfo> Postfix;
            // Runs even when the original throws: closes a HumanFilter game scope.
            public Func<MethodInfo> Finalizer;
        }

        static readonly List<Spec> Specs = new List<Spec>
        {
            new Spec
            {
                HookId = "barter.session_start",
                Target = () => AccessTools.Method(typeof(StoreUIManager), "OnNextClientArrived", new Type[0]),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(ClientArrivedPostfix)),
            },
            new Spec
            {
                HookId = "barter.outcome_bought",
                Target = () => AccessTools.Method(typeof(PlayerStore), "OnItemBought", new[] { typeof(GameItem), typeof(int) }),
                Prefix = () => AccessTools.Method(typeof(FencePatches), nameof(ShopBoughtPrefix)),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(ShopBoughtPostfix)),
            },
            new Spec
            {
                HookId = "barter.outcome_sold",
                Target = () => AccessTools.Method(typeof(StoreClient), "OnItemBought", new[] { typeof(GameItem), typeof(int) }),
                Prefix = () => AccessTools.Method(typeof(FencePatches), nameof(ShopSoldPrefix)),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(ShopSoldPostfix)),
            },
            new Spec
            {
                HookId = "barter.outcome_left",
                Target = () => AccessTools.Method(typeof(PlayerStore), "DismissCurrentClient", new[] { typeof(bool), typeof(bool), typeof(bool) }),
                Prefix = () => AccessTools.Method(typeof(FencePatches), nameof(DismissPrefix)),
            },
            new Spec
            {
                HookId = "barter.dismiss",
                Target = () => AccessTools.Method(typeof(PlayerStore), "TryPlayerDismissCurrentClient", new[] { typeof(bool) }),
                Prefix = () => AccessTools.Method(typeof(FencePatches), nameof(PlayerDismissPrefix)),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(PlayerDismissPostfix)),
            },
            new Spec
            {
                HookId = "barter.bargain_offer_discount",
                Target = () => AccessTools.Method(typeof(BargainUIManager), "OfferDiscount", new[] { typeof(int) }),
                Prefix = () => AccessTools.Method(typeof(FencePatches), nameof(BargainPrefix)),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(DiscountPostfix)),
            },
            new Spec
            {
                HookId = "barter.bargain_markup",
                Target = () => AccessTools.Method(typeof(BargainUIManager), "OfferMarkup", new[] { typeof(int) }),
                Prefix = () => AccessTools.Method(typeof(FencePatches), nameof(BargainPrefix)),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(MarkupPostfix)),
            },
            new Spec
            {
                HookId = "barter.bargain_offer_markup",
                Target = () => AccessTools.Method(typeof(BargainUIManager), "OfferBuyingMarkup", new[] { typeof(int) }),
                Prefix = () => AccessTools.Method(typeof(FencePatches), nameof(BargainPrefix)),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(BuyingMarkupPostfix)),
            },
            new Spec
            {
                HookId = "barter.bargain_demand_discount",
                Target = () => AccessTools.Method(typeof(BargainUIManager), "DemandBuyingDiscount", new[] { typeof(int) }),
                Prefix = () => AccessTools.Method(typeof(FencePatches), nameof(BargainPrefix)),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(BuyingDiscountPostfix)),
            },
            new Spec
            {
                HookId = "barter.bargain_blackmail",
                Target = () => AccessTools.Method(typeof(BargainUIManager), "Blackmail", new Type[0]),
                Prefix = () => AccessTools.Method(typeof(FencePatches), nameof(BargainPrefix)),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(BlackmailPostfix)),
            },
            new Spec
            {
                HookId = "barter.bargain_threaten",
                Target = () => AccessTools.Method(typeof(BargainUIManager), "ThreatenAug", new Type[0]),
                Prefix = () => AccessTools.Method(typeof(FencePatches), nameof(BargainPrefix)),
                Postfix = () => AccessTools.Method(typeof(FencePatches), nameof(ThreatenPostfix)),
            },
        };

        public static IEnumerable<string> HookIds()
        {
            foreach (var s in Specs.Concat(RecordSpecs)) yield return s.HookId;
        }

        public static void Apply(Harmony harmony, CompatReport compat, FenceLog log)
        {
            foreach (var spec in Specs.Concat(RecordSpecs))
            {
                if (!compat.IsUsable(spec.HookId)) continue;
                try
                {
                    var target = spec.Target();
                    if (target == null) throw new MissingMethodException(spec.HookId);
                    harmony.Patch(target,
                        prefix: spec.Prefix != null ? new HarmonyMethod(spec.Prefix()) : null,
                        postfix: spec.Postfix != null ? new HarmonyMethod(spec.Postfix()) : null,
                        finalizer: spec.Finalizer != null ? new HarmonyMethod(spec.Finalizer()) : null);
                }
                catch (Exception e)
                {
                    compat.MarkBindFailed(spec.HookId, e.GetType().Name + ": " + e.Message);
                    log.Warn("could not patch " + spec.HookId + ": " + e.Message);
                }
            }
        }

        public static void Remove(Harmony harmony)
        {
            harmony.UnpatchAll(HarmonyId);
        }

        static void Guard(Action a)
        {
            try
            {
                if (Advisor != null) a();
            }
            catch (Exception)
            {
                // Observation only: losing one outcome is better than breaking the game method.
            }
        }

        static void ClientArrivedPostfix() => Guard(() => Advisor.OnClientArrived());

        static void ShopBoughtPrefix() => Guard(() => Advisor.BeforeTrade(TradeBatch.Bought, null));

        static void ShopBoughtPostfix(GameItem gameItem, int cost) => Guard(() => Advisor.AfterTrade(TradeBatch.Bought, null, gameItem, cost));

        static void ShopSoldPrefix(StoreClient __instance) => Guard(() => Advisor.BeforeTrade(TradeBatch.Sold, __instance));

        static void ShopSoldPostfix(StoreClient __instance, GameItem item, int cost) => Guard(() => Advisor.AfterTrade(TradeBatch.Sold, __instance, item, cost));

        static void DismissPrefix() => Guard(() => Advisor.BeforeDismiss());

        static void PlayerDismissPrefix()
        {
            Guard(() => Advisor.PlayerDismissing(true));
            Rec("dismiss", r => r.Click(OptionIds.Dismiss));
        }

        static void PlayerDismissPostfix() => Guard(() => Advisor.PlayerDismissing(false));

        static void BargainPrefix(out object __state)
        {
            object probe = null;
            Guard(() => probe = Advisor.BeforeBargain());
            __state = probe;
        }

        static void After(BargainKind kind, int percent, object state)
        {
            Guard(() => Advisor.AfterBargain(kind, percent, state as BarterAdvisor.BargainProbe));
            Rec("bargain", r => r.Click(OptionIds.Bargain(kind, kind == BargainKind.Blackmail || kind == BargainKind.Threaten ? BarterRules.CoercionPercent : Math.Abs(percent))));
        }

        static void DiscountPostfix(object __state, int percent) => After(BargainKind.Discount, percent, __state);

        static void MarkupPostfix(object __state, int percent) => After(BargainKind.Markup, percent, __state);

        static void BuyingMarkupPostfix(object __state, int percent) => After(BargainKind.BuyingMarkup, percent, __state);

        static void BuyingDiscountPostfix(object __state, int percent) => After(BargainKind.BuyingDiscount, percent, __state);

        static void BlackmailPostfix(object __state) => After(BargainKind.Blackmail, BarterRules.CoercionPercent, __state);

        static void ThreatenPostfix(object __state) => After(BargainKind.Threaten, BarterRules.CoercionPercent, __state);
    }
}
