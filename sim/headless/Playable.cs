using System;
using System.Collections.Generic;
using System.Linq;
using Mono.Cecil;
using Mono.Cecil.Cil;
using MethodBody = Mono.Cecil.Cil.MethodBody;

namespace Fence.Sim
{
    // The gym variant's extra patches. The extract build leaves every UI manager inert; a playable
    // game needs the logic half of a few of them (dialogue flow, bargain rolls, the tablet's trade
    // buttons, shutter, arrival), coroutines that actually run, and hooks where the live mod has
    // Harmony patches. Their drawing stays stubbed.
    static class Playable
    {
        static readonly HashSet<string> AllBut = new HashSet<string> { "BargainUIManager" };

        // Methods of presentation behaviours whose bodies stay real. For AllBut types: methods that stay stubbed.
        static readonly Dictionary<string, HashSet<string>> Methods = new Dictionary<string, HashSet<string>>
        {
            ["DialogUIManager"] = new HashSet<string>
            {
                "StartDialogue", "HasPendingChainConsequence", "CheckOwned", "OnCurrentTextDisplayed", "ScaleByDialogSpeed",
                "GetTypewriterDelay", "IsLineStillDisplaying", "IsCurrentDialogEnd", "ContinueConversation", "ResumeIfWaitingForAdvance",
                "EndConversation", "IsChoice", "SelectChoice", "CurrentClientLeave", "FlushScheduledEndActions", "get_HasPausedConversation",
                "PauseConversation", "ClearPausedConversation", "ResumeConversation", "DisplayLine", "InterruptDialog", "RemoveCurrentDialog",
                "RemoveCustomerDialogue", "SetCurrentDialog", "GetCurrentDialog", "StopAllTrackedDialogueCoroutines", "PlayQuickDialogue",
                "ScheduleFlow", "ScheduleAutoAdvance", "ClearPendingAdvance", "OnAdvancePressed", "FinishConversationNow",
                "UpdateDialogHistory", "RemoveTag", "DisplayClientText", "DisplaySecText", "DisplayerPlayerText",
            },
            // Stubbed: window building, button states, previews.
            ["BargainUIManager"] = new HashSet<string>
            {
                "Awake", "Start", "CreateWindow", "get_IsOpen", "Update", "RefreshUI", "UpdateGroupVisibility", "BuildGroups", "BuildGroup",
                "UpdateRepGainText", "OpenUI", "CloseUI", "ToggleUI", "DockToTablet", "SetRepPreview", "ClearRepPreview",
            },
            ["NegociationUIManager"] = new HashSet<string>
            {
                "Client", "IsNegociatingItemOwned", "OnBackCounter", "ResolveMode", "SectionTitle", "GetSellableItem", "GetOwnedBackInvItems",
                "GetNonOwnedBackInvItems", "GetSellableBackInvItems", "GetNotBuyingReason", "GetRefusalLabel", "GetFinalOfferValue",
                "SumNegociatedValue", "GetMultiSellValue", "IsClientHaveEnoughToBuy", "IsTradeLocked", "OnSellAllClick", "OnBuyAllClick",
                "OnWholesaleClick", "OnOfferAcceptClick", "BuyMultipleItem", "BuyItem", "SellMultiple", "PlayAcceptDealDialogueOnce",
                "TrySellItem", "SellItem", "OnConfirmPanelYes", "IsDonationClient", "SumDonationCreditValue", "UpdateUI",
            },
            ["StoreUIManager"] = new HashSet<string> { "OnNextClientArrived", "StartBlackscreenSequence", "BlackScreenSequence" },
            ["StoreShutterButton"] = new HashSet<string> { "OpenShutter", "OnShutterDoorOpenMorning", "HandlePermit", "EndAndClose" },
            ["MapUIManager"] = new HashSet<string>
            {
                "get_Current", "set_Current", "get_IsOutside", "get_isOutside", "get_LastVisitedTonightId", "set_LastVisitedTonightId",
                "get_IsInTransit", "OpenUI", "<OpenUI>b__40_0", "Travel", "Return", "ReturnNow", "OnGoingOutside", "OnReturning",
                "ScavengeButtonClick", "LeaveScavenging", "LeaveCommissary", "LeaveUpgradeMerchant", "CheckItemLeftOnGround",
                "TransferBackInvToCounter", "TransferCounterToBackInv",
            },
        };

        public static bool KeepsReal(TypeDefinition t, MethodDefinition md)
        {
            // Player settings the game logic reads (MoveBoughtItemsToInventory, AutoOpenNegotiationUI, …).
            if (t.Name == "EmporiumOption" && md.IsGetter) return true;
            if (!Methods.TryGetValue(t.Name, out var set)) return false;
            return AllBut.Contains(t.Name) ? !set.Contains(md.Name) : set.Contains(md.Name);
        }

        // Prefix hooks: where the live mod observes through Harmony.
        static readonly string[] Hooked =
        {
            "PlayerStore::OnItemBought", "StoreClient::OnItemBought", "PlayerStore::DismissCurrentClient",
            "PlayerStore::TryPlayerDismissCurrentClient", "PlayerStore::ExecuteGameOver", "PlayerStore::AddedItemToWeightedArea",
            "PlayerStore::RemovedItemToWeightedArea", "PlayerStore::AddedItemToDisplayCase", "PlayerStore::RemovedItemToDisplayCase",
            "PlayerStore::QueueFuturClient", "DialogUIManager::SelectChoice", "DialogUIManager::StartDialogue",
            "ContrabandHelper::StartInspection", "PlayerStore::OnShutterOpened", "PlayerStore::EndDay", "PlayerStore::EndNight",
            "PlayerStore::BeginDay", "AugHelper::OnGoodKill", "AugHelper::CleanupKill", "AugHelper::OnAugLeftAlive",
            // SpriteDict is presentation (stubbed): which pool an aug's portrait came from is seen here instead.
            "AugHelper::SetRandomAugFaction", "AugHelper::MakeDisguisedAug", "AugHelper::GetRandomAugSprite",
        };

        public static void PatchGame(ModuleDefinition m)
        {
            var enter = m.ImportReference(typeof(Hooks).GetMethod(nameof(Hooks.Enter)));
            foreach (var id in Hooked)
            {
                var parts = id.Split("::");
                foreach (var md in m.GetType(parts[0]).Methods.Where(x => x.Name == parts[1] && x.HasBody && !x.HasGenericParameters))
                    InjectHook(m, md, id, enter);
            }
            foreach (var md in m.GetType("RunStats").Methods.Where(x => x.IsStatic && x.IsPublic && x.HasBody && x.Name.StartsWith("On") && !x.HasGenericParameters))
                InjectHook(m, md, "RunStats::" + md.Name, enter);

            // The typewriter's "line shown" event never fires headless: each line counts as shown on the next pump.
            var dialog = m.GetType("DialogUIManager");
            var shown = dialog.Methods.Single(x => x.Name == "OnCurrentTextDisplayed");
            var later = m.ImportReference(typeof(Scheduler).GetMethod(nameof(Scheduler.Later)));
            var actionCtor = m.ImportReference(typeof(Action).GetConstructors().Single());
            foreach (var name in new[] { "DisplayClientText", "DisplaySecText", "DisplayerPlayerText" })
                AppendBeforeReturn(dialog.Methods.Single(x => x.Name == name), il => new[]
                {
                    il.Create(OpCodes.Ldarg_0),
                    il.Create(OpCodes.Ldftn, shown),
                    il.Create(OpCodes.Newobj, actionCtor),
                    il.Create(OpCodes.Call, later),
                });

            // Wild's Network is a prefab window headless (CustomUIManager.networkUIPrefab is null, so Open
            // bails out). The host gets the open instead and plays the browse and the close (sim/gym WildNetwork).
            var openWild = Headless.Method(m, "WildNetworkUI", "Open", 1);
            Headless.Replace(openWild, il =>
            {
                il.Emit(OpCodes.Ldstr, "WildNetworkUI::Open");
                il.Emit(OpCodes.Ldnull);
                il.Emit(OpCodes.Ldc_I4_1);
                il.Emit(OpCodes.Newarr, m.TypeSystem.Object);
                il.Emit(OpCodes.Dup);
                il.Emit(OpCodes.Ldc_I4_0);
                il.Emit(OpCodes.Ldarg_0);
                il.Emit(OpCodes.Box, m.TypeSystem.Boolean);
                il.Emit(OpCodes.Stelem_Ref);
                il.Emit(OpCodes.Call, enter);
                il.Emit(OpCodes.Ret);
            });

            // Canvas layout and machinery labels only.
            foreach (var md in m.GetType("EmporiumEntry").Methods.Where(x => x.Name == "Validate" && x.HasBody)) Headless.ReturnDefault(md);
            Headless.ReturnDefault(Headless.Method(m, "ToolboxHelper", "RefreshSlots"));
            // Hides the store's windows on going out (null headless). Its throw, swallowed by the travel's
            // blackscreen callback, left every outing unarrived, so scavenging attempts never reset.
            Headless.ReturnDefault(Headless.Method(m, "EmporiumEntry", "OnLeaveStore"));

            // OnConfirmPanelYes returns early without the tablet's frame; this is its logic without the panel.
            var nego = m.GetType("NegociationUIManager");
            var yes = nego.Methods.Single(x => x.Name == "OnConfirmPanelYes");
            var ps = m.GetType("PlayerStore");
            var getPs = ps.Methods.Single(x => x.Name == "get_Instance");
            var current = ps.Fields.Single(f => f.Name == "currentClientInstance");
            Headless.Replace(yes, il =>
            {
                var ret = il.Create(OpCodes.Ret);
                il.Emit(OpCodes.Call, getPs);
                il.Emit(OpCodes.Ldfld, current);
                il.Emit(OpCodes.Brfalse, ret);
                il.Emit(OpCodes.Call, nego.Methods.Single(x => x.Name == "IsTradeLocked"));
                il.Emit(OpCodes.Brtrue, ret);
                il.Emit(OpCodes.Ldarg_0);
                il.Emit(OpCodes.Call, nego.Methods.Single(x => x.Name == "Client"));
                il.Emit(OpCodes.Call, nego.Methods.Single(x => x.Name == "GetSellableBackInvItems"));
                il.Emit(OpCodes.Ldc_I4_1);
                il.Emit(OpCodes.Call, nego.Methods.Single(x => x.Name == "SellMultiple"));
                il.Append(ret);
            });
        }

        // MonoBehaviour's coroutine API onto Scheduler.
        public static void PatchCoroutines(ModuleDefinition m)
        {
            var mb = m.GetType("UnityEngine.MonoBehaviour");
            MethodReference S(string name) => m.ImportReference(typeof(Scheduler).GetMethod(name));
            foreach (var md in mb.Methods.Where(x => (x.Name == "StartCoroutine" || x.Name == "StartCoroutine_Auto") && x.Parameters.Count == 1
                                                      && x.Parameters[0].ParameterType.FullName == "System.Collections.IEnumerator"))
                Headless.Replace(md, il =>
                {
                    il.Emit(OpCodes.Ldarg_0);
                    il.Emit(OpCodes.Ldarg_1);
                    il.Emit(OpCodes.Call, S(nameof(Scheduler.Start)));
                    il.Emit(OpCodes.Castclass, md.ReturnType);
                    il.Emit(OpCodes.Ret);
                });
            foreach (var md in mb.Methods.Where(x => x.Name == "StopCoroutine" && x.Parameters.Count == 1 && x.Parameters[0].ParameterType.FullName != "System.String"))
                Headless.Replace(md, il =>
                {
                    il.Emit(OpCodes.Ldarg_0);
                    il.Emit(OpCodes.Ldarg_1);
                    il.Emit(OpCodes.Call, S(nameof(Scheduler.Stop)));
                    il.Emit(OpCodes.Ret);
                });
            var time = m.GetType("UnityEngine.Time");
            var clock = new Dictionary<string, string>
            {
                ["get_deltaTime"] = nameof(Scheduler.DeltaTime), ["get_unscaledDeltaTime"] = nameof(Scheduler.DeltaTime),
                ["get_smoothDeltaTime"] = nameof(Scheduler.DeltaTime), ["get_fixedDeltaTime"] = nameof(Scheduler.DeltaTime),
                ["get_time"] = nameof(Scheduler.TimeSingle), ["get_unscaledTime"] = nameof(Scheduler.TimeSingle),
                ["get_realtimeSinceStartup"] = nameof(Scheduler.TimeSingle), ["get_timeSinceLevelLoad"] = nameof(Scheduler.TimeSingle),
                ["get_fixedTime"] = nameof(Scheduler.TimeSingle), ["get_timeAsDouble"] = nameof(Scheduler.TimeDouble),
                ["get_unscaledTimeAsDouble"] = nameof(Scheduler.TimeDouble), ["get_realtimeSinceStartupAsDouble"] = nameof(Scheduler.TimeDouble),
                ["get_frameCount"] = nameof(Scheduler.FrameCount), ["get_timeScale"] = nameof(Scheduler.One),
            };
            foreach (var md in time.Methods.Where(x => clock.ContainsKey(x.Name) && x.Parameters.Count == 0))
            {
                md.IsInternalCall = false;
                md.ImplAttributes = MethodImplAttributes.IL | MethodImplAttributes.Managed;
                string target = clock[md.Name];
                Headless.Replace(md, il =>
                {
                    il.Emit(OpCodes.Call, S(target));
                    il.Emit(OpCodes.Ret);
                });
            }

            var stopAll = mb.Methods.Single(x => x.Name == "StopAllCoroutines");
            stopAll.IsInternalCall = false;
            stopAll.ImplAttributes = MethodImplAttributes.IL | MethodImplAttributes.Managed;
            Headless.Replace(stopAll, il =>
            {
                il.Emit(OpCodes.Ldarg_0);
                il.Emit(OpCodes.Call, S(nameof(Scheduler.StopAll)));
                il.Emit(OpCodes.Ret);
            });
        }

        static void InjectHook(ModuleDefinition m, MethodDefinition md, string id, MethodReference enter)
        {
            var il = md.Body.GetILProcessor();
            var first = md.Body.Instructions[0];
            var seq = new List<Instruction>
            {
                il.Create(OpCodes.Ldstr, id),
                md.IsStatic ? il.Create(OpCodes.Ldnull) : il.Create(OpCodes.Ldarg_0),
                il.Create(OpCodes.Ldc_I4, md.Parameters.Count),
                il.Create(OpCodes.Newarr, m.TypeSystem.Object),
            };
            for (int i = 0; i < md.Parameters.Count; i++)
            {
                var p = md.Parameters[i];
                if (p.ParameterType.IsByReference || p.ParameterType.IsGenericParameter) continue;
                seq.Add(il.Create(OpCodes.Dup));
                seq.Add(il.Create(OpCodes.Ldc_I4, i));
                seq.Add(il.Create(OpCodes.Ldarg, p));
                if (p.ParameterType.IsValueType) seq.Add(il.Create(OpCodes.Box, m.ImportReference(p.ParameterType)));
                seq.Add(il.Create(OpCodes.Stelem_Ref));
            }
            seq.Add(il.Create(OpCodes.Call, enter));
            foreach (var x in seq) il.InsertBefore(first, x);
            // Exception handlers starting at the old first instruction keep starting there, after the hook.
        }

        // Every `ret` becomes a branch to one epilogue: the appended code, then ret.
        static void AppendBeforeReturn(MethodDefinition md, Func<ILProcessor, Instruction[]> epilogue)
        {
            var il = md.Body.GetILProcessor();
            var code = epilogue(il);
            var ret = il.Create(OpCodes.Ret);
            foreach (var x in md.Body.Instructions.Where(x => x.OpCode == OpCodes.Ret).ToList())
            {
                x.OpCode = OpCodes.Br;
                x.Operand = code[0];
            }
            foreach (var x in code) il.Append(x);
            il.Append(ret);
        }
    }
}
