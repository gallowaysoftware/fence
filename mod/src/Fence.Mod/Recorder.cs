using System;
using System.Collections.Generic;
using System.Linq;
using System.Runtime.CompilerServices;
using Fence.Barter;
using Fence.Recording;
using Newtonsoft.Json.Linq;

namespace Fence.Mod
{
    // Phase D: records everything a player can observe and every move they make (docs/PROTOCOL.md,
    // "Recording events"; docs/re/DAY.md). Called from Harmony patches (FencePatches.Record) and ModHook
    // events; never throws into the game: every entry point swallows, counts and logs once per hook.
    // Independent of the bridge mode: events queue in BridgeSession whatever the bridge is doing.
    sealed class Recorder
    {
        public delegate void Emit(string kind, JObject data, JObject hidden);

        readonly FenceLog log;
        readonly Emit emit;
        readonly Func<string, bool> featureOn;
        readonly GameDay day = new GameDay();
        readonly GameBarter items = new GameBarter();
        readonly UpgradePurchase upgrade = new UpgradePurchase();
        readonly Dictionary<string, int> errors = new Dictionary<string, int>();
        readonly List<Action> unbinds = new List<Action>();
        IList<ServiceDay> servicesBeforeNight;
        ExposeAttempt expose;
        string phoneChoiceText;
        int commissaryBefore;

        public readonly HumanFilter Filter = new HumanFilter();
        // The customer and advice in force; set by BarterAdvisor.
        public Func<AdviceInForce> Advice = () => null;

        public Recorder(FenceLog log, Emit emit, Func<string, bool> featureOn)
        {
            this.log = log;
            this.emit = emit;
            this.featureOn = featureOn;
        }

        public int ErrorCount => errors.Values.Sum();

        public GameDay Day => day;

        void Count(string hook, Exception e)
        {
            errors.TryGetValue(hook, out int n);
            errors[hook] = n + 1;
            if (n == 0) log.Warn("recording " + hook + " failed (counting, logged once): " + e.GetType().Name + ": " + e.Message);
        }

        // Every hook funnels through here: a recording bug costs one event, never the game method.
        public void Guard(string hook, Action a)
        {
            try
            {
                a();
            }
            catch (Exception e)
            {
                Count(hook, e);
            }
        }

        // ---- ModHook events ----

        public void Subscribe(Func<string, bool> hookUsable)
        {
            Bind(hookUsable, "snap.waking_up_early", () => GameAccess.SubscribeWakingUpEarly(() => Guard("morning", () => Snapshot(SnapshotPhases.Morning))));
            Bind(hookUsable, "snap.going_sleep_early", () => GameAccess.SubscribeGoingSleepEarly(() => Guard("sleep", GoingToSleep)));
            Bind(hookUsable, "record.loading", () => GameAccess.SubscribeGameLoadedInit(() => Guard("loading", Filter.LoadStarted)));
        }

        void Bind(Func<string, bool> usable, string hookId, Func<Action> subscribe)
        {
            if (!usable(hookId)) return;
            try
            {
                unbinds.Add(subscribe());
            }
            catch (Exception e)
            {
                log.Warn("could not subscribe " + hookId + ": " + e.Message);
            }
        }

        public void Unsubscribe()
        {
            foreach (var u in unbinds)
            {
                try
                {
                    u();
                }
                catch (Exception)
                {
                }
            }
            unbinds.Clear();
        }

        // Forwarded P0 events that also mark recording moments.
        public void OnGameEvent(string kind)
        {
            Guard("event:" + kind, () =>
            {
                switch (kind)
                {
                    case "game_loaded":
                        Filter.LoadFinished();
                        Snapshot(SnapshotPhases.Load);
                        break;
                    case "new_game_started":
                        Filter.LoadStarted();
                        break;
                    case "shutter_opened":
                        // Fallback if OnGameLoadedLate is gone: a day in progress is never a load.
                        if (Filter.Loading) Filter.LoadFinished();
                        Snapshot(SnapshotPhases.Open);
                        break;
                    case "shutter_closed":
                        Snapshot(SnapshotPhases.Close);
                        break;
                }
            });
        }

        void Snapshot(string phase)
        {
            var r = DaySnapshot.Build(day, phase, GameDay.RunId(), section => featureOn("record.snapshot." + section));
            foreach (var (section, error) in r.Failed) Count("day_snapshot." + section, error);
            emit(RecordKinds.DaySnapshot, r.Data, r.Hidden.HasValues ? r.Hidden : null);
            if (phase == SnapshotPhases.Close || phase == SnapshotPhases.Load || phase == SnapshotPhases.Morning) servicesBeforeNight = TryServices();
        }

        IList<ServiceDay> TryServices()
        {
            if (!featureOn("record.night.services")) return null;
            try
            {
                return day.Services();
            }
            catch (Exception)
            {
                return null;
            }
        }

        // OnGoingSleepEarly: after the night's outings, before services are charged.
        void GoingToSleep()
        {
            var now = TryServices();
            if (now != null) Human(HumanActions.ServicesSet, null, ServiceSelection.Diff(servicesBeforeNight, now));
            Snapshot(SnapshotPhases.Sleep);
        }

        // ---- run ----

        public void RunStarted()
        {
            Filter.LoadFinished();
            emit(RecordKinds.RunStart, GameDay.Run(), null);
            Snapshot(SnapshotPhases.RunStart);
        }

        public void GameOver(string ending)
        {
            var o = new JObject { ["ending"] = ending };
            try
            {
                var s = day.Store();
                o["day"] = s.Day;
                o["cash"] = s.Cash;
                o["net_worth"] = s.NetWorth;
            }
            catch (Exception)
            {
            }
            o["run_id"] = GameDay.RunId();
            emit(RecordKinds.RunEnd, o, null);
        }

        // ---- human actions ----

        void Human(string action, int? itemUid = null, JObject extra = null)
        {
            AdviceInForce advice = null;
            try
            {
                advice = Advice?.Invoke();
            }
            catch (Exception)
            {
            }
            emit(RecordKinds.HumanAction, HumanActions.Build(action, advice, itemUid, extra), null);
        }

        int Uid(GameItem item) => items.Uid(item);

        public void CounterMove(string action, GameItem item, bool requireOwned)
        {
            if (!Filter.IsHuman || item == null) return;
            bool owned = GameRecord.Owned(item);
            if (requireOwned && !owned) return;
            Human(action, Uid(item), new JObject { ["item_id"] = GameRecord.Id(item), ["owned"] = owned });
        }

        public void ShowcaseMove(string action, GameItem item)
        {
            if (!Filter.IsHuman || item == null) return;
            Human(action, Uid(item), new JObject { ["item_id"] = GameRecord.Id(item) });
        }

        public void Tablet(string action)
        {
            if (Filter.IsHuman) Human(action);
        }

        // Trade buttons, bargains, dismiss: presses, so only BarterExecutor and loading are filtered.
        public void Click(string action, JObject extra = null)
        {
            if (Filter.IsHumanClick) Human(action, null, extra);
        }

        sealed class ExposeAttempt
        {
            public string FeatureId, Category, Tool;
            public int ItemUid;
            public bool WasExposed, BadMatch, Notebook;
        }

        public object BeforeExpose(ItemFeature feature, bool notebook)
        {
            if (!Filter.IsHumanClick || feature == null) return null;
            expose = new ExposeAttempt
            {
                FeatureId = feature.identifier,
                Category = feature.category,
                ItemUid = feature.parentItemUniqueId,
                WasExposed = feature.IsExposed(),
                Notebook = notebook,
                Tool = notebook ? "notebook" : GameRecord.SelectedTool(),
            };
            return expose;
        }

        public void ExposeBadMatch()
        {
            if (expose != null) expose.BadMatch = true;
        }

        public void AfterExpose(object state, ItemFeature feature)
        {
            var a = state as ExposeAttempt;
            expose = null;
            if (a == null || feature == null) return;
            string result = !a.WasExposed && feature.IsExposed() ? "exposed"
                : a.BadMatch ? "bad_match"
                : !a.Notebook && a.Tool != null && feature.GetExposeDialog() != null ? "exposing_dialog"
                : null;
            // No selection, a locked confront or an already exposed feature: the click did nothing.
            if (result == null) return;
            Human(HumanActions.ExposeFeature, a.ItemUid, new JObject
            {
                ["feature_id"] = a.FeatureId,
                ["category"] = a.Category,
                ["tool"] = a.Tool,
                ["result"] = result,
            });
        }

        public void InspectItem(GameItem item)
        {
            if (Filter.IsHumanClick && item != null) Human(HumanActions.InspectItem, Uid(item), new JObject { ["item_id"] = GameRecord.Id(item) });
        }

        public void Confront(string itemId, string confrontType, string outcome)
        {
            if (Filter.IsHumanClick) Human(HumanActions.Confront, null, new JObject { ["item_id"] = itemId, ["confront_type"] = confrontType, ["outcome"] = outcome });
        }

        public void DialogueChoice(int index, Dialogue dialog, bool phone)
        {
            if (Filter.Loading || dialog == null) return;
            var choices = GameRecord.Choices(dialog);
            var o = HumanActions.Choice(index, choices, phone ? null : GameRecord.DialogHistory(6));
            if (phone) phoneChoiceText = (string)o["text"];
            Human(phone ? HumanActions.PhoneChoice : HumanActions.DialogueChoice, null, o);
        }

        public void CourierQueued(string clientId, int inDays)
        {
            if (!GameRecord.PhoneDialogOpen()) return;
            Human(HumanActions.PhoneOrder, null, new JObject { ["courier_id"] = clientId, ["in_days"] = inDays, ["choice_text"] = phoneChoiceText });
        }

        public void Report() => Click(HumanActions.Report);

        public string CurrentKind() => CustomerKinds.Of(items.Client());

        public void Scavenged(List<GameItem> found)
        {
            var list = new JArray();
            if (found != null)
                foreach (var item in found)
                    if (item != null) list.Add(new JObject { ["uid"] = Uid(item), ["id"] = GameRecord.Id(item) });
            RunStat("scavenged", new JObject { ["items"] = list });
        }

        public void Shot(StoreClient client) => Click(HumanActions.Shoot, new JObject { ["name"] = GameRecord.ClientName(client) });

        public void AppraisalEstimate(GameItem item, int value)
        {
            if (Filter.IsHumanClick) Human(HumanActions.AppraisalEstimate, item != null ? Uid(item) : (int?)null, new JObject { ["value"] = value });
        }

        public void AppraisalSent(GameItem item, int sent, int actual) =>
            emit(RecordKinds.RunStat, new JObject { ["stat"] = "appraisal_sent", ["item_uid"] = item != null ? Uid(item) : 0, ["sent_value"] = sent },
                new JObject { ["actual_value"] = actual });

        public void Poster(bool mute)
        {
            if (!mute && Filter.IsHumanClick) Human(HumanActions.Poster, null, new JObject { ["pulled"] = GameRecord.PosterPulled() });
        }

        public void RunStat(string stat, JObject extra = null)
        {
            var o = new JObject { ["stat"] = stat };
            if (extra != null)
                foreach (var p in extra.Properties())
                    o[p.Name] = p.Value;
            emit(RecordKinds.RunStat, o, null);
        }

        // ---- night ----

        public void Sleep() => Click(HumanActions.Sleep);

        public void UpgradeCash(int cashSpent)
        {
            upgrade.CashSpent(cashSpent);
            Money(MoneyKinds.WildUpgrade, new JObject { ["cash_spent"] = cashSpent });
        }

        public void UpgradeBought(string upgradeId)
        {
            var (name, cost) = GameRecord.Upgrade(upgradeId);
            Click(HumanActions.UpgradeBuy, upgrade.Bought(upgradeId, name, cost));
        }

        public void Travel(AfterhourLocation location)
        {
            if (location != null) Click(HumanActions.GoOut, new JObject { ["location"] = GameRecord.LocationId(location) });
        }

        public void Return() => Click(HumanActions.Return, new JObject { ["location"] = GameRecord.CurrentLocation() });

        public void Scavenge() => Click(HumanActions.Scavenge, GameRecord.ScavengeOdds());

        public void CommissaryBefore(CommissaryData data) => commissaryBefore = data != null ? data.redeemedCredit : 0;

        public void CommissaryBought(string id, CommissaryData data)
        {
            if (data == null || data.redeemedCredit == commissaryBefore) return;
            Click(HumanActions.CommissaryBuy, new JObject { ["item_id"] = id, ["price"] = commissaryBefore - data.redeemedCredit, ["tokens_left"] = data.redeemedCredit });
        }

        public void CommissaryRedeemed()
        {
            int after = GameRecord.CommissaryTokens();
            Click(HumanActions.CommissaryRedeem, new JObject { ["tokens_before"] = commissaryBefore, ["tokens_after"] = after });
        }

        public void FillRequest(StoreRequest request)
        {
            if (request == null) return;
            var r = GameDay.Request(request);
            Click(HumanActions.FillRequest, new JObject
            {
                ["request_id"] = r.Id,
                ["title"] = r.Title,
                ["faction"] = Factions.Name(r.FactionId),
                ["required_item"] = r.RequiredItem,
                ["count"] = r.Count,
                ["credit_reward"] = r.CreditReward,
                ["rep_reward"] = r.RepReward,
            });
        }

        // ---- money and inspections ----

        public void Money(string kind, JObject amounts = null) => emit(RecordKinds.Money, RunEvents.Money(kind, amounts), null);

        public void ItemMoney(string kind, GameItem item, int cost, int? boughtFor, StoreClient client)
        {
            var o = new JObject { ["cost"] = cost };
            if (item != null)
            {
                o["item_uid"] = Uid(item);
                o["item_id"] = GameRecord.Id(item);
            }
            if (boughtFor.HasValue) o["bought_for"] = boughtFor.Value;
            if (client != null) o["client"] = GameRecord.ClientName(client);
            Money(kind, o);
        }

        // CreateInspectionClient(personality): the name the player sees says which one it is.
        static readonly string[] InspectorKinds = { "standard", "shady", "lazy" };

        public void InspectorArrived(int personality, bool fromShowcase)
        {
            var o = new JObject
            {
                ["phase"] = "arrived",
                ["personality"] = personality,
                ["inspector"] = personality >= 0 && personality < InspectorKinds.Length ? InspectorKinds[personality] : "unknown",
                ["from_showcase"] = fromShowcase,
                ["bribable"] = personality == 1,
            };
            if (personality == 1) o["bribe_amount"] = GameRecord.BribeAmount();
            emit(RecordKinds.Inspection, o, null);
        }

        public void InspectionBribe(int amount) => emit(RecordKinds.Inspection, new JObject { ["phase"] = "bribe", ["amount"] = amount }, null);

        // What the search will see: the player's stolen and contraband items by place, and the poster.
        public void InspectionSearch(int personality, bool fromShowcase)
        {
            var exposed = new JObject();
            if (featureOn("record.snapshot.inventory"))
            {
                foreach (var g in day.Inventory().Where(i => i.Item.StolenHeat.HasValue || i.Item.ContrabandLevel > 0).GroupBy(i => i.Root ?? i.Where))
                    exposed[g.Key] = new JObject
                    {
                        ["stolen"] = g.Count(i => i.Item.StolenHeat.HasValue),
                        ["contraband"] = g.Count(i => i.Item.ContrabandLevel > 0),
                        ["items"] = new JArray(g.Select(i => (object)i.Item.Uid).ToArray()),
                    };
            }
            emit(RecordKinds.Inspection, new JObject
            {
                ["phase"] = "search",
                ["personality"] = personality,
                ["from_showcase"] = fromShowcase,
                ["poster_pulled"] = GameRecord.PosterPulled(),
                ["exposed"] = exposed,
            }, null);
        }

        public void Inspected(string outcomeType, int lostCount, int lostValue, bool usedGrace) =>
            emit(RecordKinds.Inspection, new JObject
            {
                ["phase"] = "result",
                ["outcome"] = outcomeType,
                ["lost_count"] = lostCount,
                ["lost_value"] = lostValue,
                ["used_grace"] = usedGrace,
            }, null);

        public void InspectionStats(bool passed, int seizedValue) =>
            emit(RecordKinds.Inspection, new JObject { ["phase"] = "stats", ["passed"] = passed, ["seized_value"] = seizedValue }, null);

        // ---- predictions the game computed itself ----

        public void PredictionsComputed(List<(string, int, string)> result)
        {
            if (day.Computing || result == null) return;
            day.LastGamePredictions = GameDay.Predictions(result);
            day.LastGamePredictionsDay = GameRecord.DayCounter();
        }

        // customer_observed extras: visible fields of the non-trade kinds.
        public JObject ClientExtras(object storeClient)
        {
            if (!featureOn("record.customer") || !(storeClient is StoreClient)) return null;
            try
            {
                return GameRecord.Extras((StoreClient)storeClient);
            }
            catch (Exception e)
            {
                Count("extras", e);
                return null;
            }
        }
    }

    // Small game reads for the recorder; NoInlining like GameAccess.
    static class GameRecord
    {
        [MethodImpl(MethodImplOptions.NoInlining)]
        public static bool Owned(GameItem item) => GeneralHelper.IsItemOwned(item);

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static string Id(GameItem item) => item.identifier;

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static int DayCounter() => StoreStation.Instance != null ? StoreStation.Instance.dayCounter : 0;

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static string ClientName(StoreClient c) => c?.displayName;

        // ItemFeatureUI.OnFeatureUIClick: the selected object is a negotiation tool (or a notebook clue).
        [MethodImpl(MethodImplOptions.NoInlining)]
        public static string SelectedTool()
        {
            var sel = RTSController.previousSelectedObjects;
            if (sel == null || sel.Count == 0 || sel[0] == null) return null;
            if (sel[0].TryGetComponent<NegociationElement>(out var tool)) return tool.IsNegociationTool() ? tool.GetNegociationToolID() : null;
            return sel[0].TryGetComponent<NotebookElement>(out _) ? "notebook" : null;
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static List<ChoiceInfo> Choices(Dialogue dialog)
        {
            var list = new List<ChoiceInfo>();
            if (dialog.choices == null) return list;
            for (int i = 0; i < dialog.choices.Count; i++)
            {
                var c = dialog.choices[i];
                list.Add(new ChoiceInfo
                {
                    Label = c.Item1 != null ? c.Item1.name : null,
                    Text = c.Item2 != null ? c.Item2.plainText : null,
                    Enabled = dialog.IsChoiceEnabledAt(i),
                });
            }
            return list;
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static List<string> DialogHistory(int last)
        {
            var q = PlayerStore.Instance != null ? PlayerStore.Instance.dialogHistory : null;
            if (q == null) return null;
            var all = q.ToList();
            return all.Skip(Math.Max(0, all.Count - last)).ToList();
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static bool PhoneDialogOpen() => DialoguePhoneUIManager.Instance != null && DialoguePhoneUIManager.Instance.GetCurrentDialog() != null;

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static bool PosterPulled() => StorePoster.Instance != null && StorePoster.Instance.isPulled;

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static (string name, int? cost) Upgrade(string id)
        {
            var all = PlayerStore.Instance?.networkUpgrade;
            if (id == null || all == null || !all.TryGetValue(id, out var u) || u == null) return (null, null);
            return (u.GetLocalizedTitle(), u.GetCost());
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static string LocationId(AfterhourLocation l) => l.Id;

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static string CurrentLocation()
        {
            var map = MapUIManager.Instance;
            return map != null && map.Current != null ? map.Current.Id : null;
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static JObject ScavengeOdds() => new JObject
        {
            ["attempts_left"] = ScavHelper.GetScavTimeLeft(),
            ["minor_pct"] = Math.Round(ScavHelper.GetMinorWoundChance(), 2),
            ["major_pct"] = Math.Round(ScavHelper.GetMajorWoundChance(), 2),
        };

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static CommissaryData Commissary() => PlayerStore.Instance?.commissaryData;

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static int CommissaryTokens() => PlayerStore.Instance?.commissaryData != null ? PlayerStore.Instance.commissaryData.redeemedCredit : 0;

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static int BribeAmount() => ContrabandHelper.GetBribeAmount();

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static JObject Extras(StoreClient c)
        {
            var o = new JObject();
            switch (c.clientIntent)
            {
                case StoreClient.ClientIntent.INFORMATION_DEALER:
                    o["service_cost"] = c.serviceCost;
                    break;
                case StoreClient.ClientIntent.GUNSMITH:
                    if (c.gunOrderData != null) o["gun_reward"] = FeatureText.Plain(c.gunOrderData.GetTotalRewardDisplay());
                    break;
                case StoreClient.ClientIntent.INSPECTION:
                    o["lazy_inspector"] = c.isLazyInspector;
                    break;
            }
            return o;
        }
    }
}
