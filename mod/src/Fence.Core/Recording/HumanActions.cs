using System;
using System.Collections.Generic;
using System.Linq;
using Fence.Barter;
using Newtonsoft.Json.Linq;

namespace Fence.Recording
{
    // Who moved: the weighted-area and display-case callbacks fire for every add/remove, so a move
    // counts as the player's only outside the game's own item shuffles (GameScope, docs/re/DAY.md §3),
    // outside BarterExecutor (Executor) and not while a save loads or a new run is set up.
    public sealed class HumanFilter
    {
        int gameDepth;
        int executorDepth;

        public bool Loading { get; private set; } = true;
        public bool Executing => executorDepth > 0;
        public bool InGameMove => gameDepth > 0;
        public bool IsHuman => !Loading && gameDepth == 0 && executorDepth == 0;
        // Bargains, trades and dismisses are button presses: only the loading and executor checks apply.
        public bool IsHumanClick => !Loading && executorDepth == 0;

        public void EnterGame() => gameDepth++;

        public void ExitGame()
        {
            if (gameDepth > 0) gameDepth--;
        }

        public void EnterExecutor() => executorDepth++;

        public void ExitExecutor()
        {
            if (executorDepth > 0) executorDepth--;
        }

        public void LoadStarted()
        {
            Loading = true;
            gameDepth = 0;
        }

        public void LoadFinished()
        {
            Loading = false;
            gameDepth = 0;
        }
    }

    // The customer and advice a move happened under.
    public sealed class AdviceInForce
    {
        // 0 = no customer at the counter.
        public int ClientSeq;
        public string DecisionId;
        // The bridge's answer to DecisionId: an option id or "defer"; null while still thinking.
        public string Advised;
    }

    public static class HumanActions
    {
        public const string Stage = "stage";
        public const string Unstage = "unstage";
        public const string FrontAdd = "front_add";
        public const string FrontRemove = "front_remove";
        public const string Showcase = "showcase";
        public const string Unshowcase = "unshowcase";
        public const string OpenTablet = "open_tablet";
        public const string CloseTablet = "close_tablet";
        public const string ExposeFeature = "expose_feature";
        public const string InspectItem = "inspect_item";
        public const string Confront = "confront";
        // The issue the player submits on the confront panel ("genuine" for a talk), before the answer.
        public const string InspectVerdict = "inspect_verdict";
        public const string ConfirmOverBudget = "confirm_over_budget";
        public const string AcceptOffer = "accept_offer";
        public const string RingBell = "ring_bell";
        public const string DialogueChoice = "dialogue_choice";
        public const string PhoneChoice = "phone_choice";
        public const string Report = "report_customer";
        public const string Shoot = "shoot";
        public const string Unholster = "unholster";
        public const string Holster = "holster";
        public const string AppraisalEstimate = "appraisal_estimate";
        public const string ServicesSet = "services_set";
        public const string Sleep = "sleep";
        public const string UpgradeBuy = "upgrade_buy";
        public const string PhoneOrder = "phone_order";
        public const string GoOut = "go_out";
        public const string Return = "return";
        public const string Scavenge = "scavenge";
        public const string CommissaryBuy = "commissary_buy";
        public const string CommissaryRedeem = "commissary_redeem";
        public const string FillRequest = "fill_request";
        public const string Poster = "poster";
        public const string ContinuePlay = "continue_play";

        public static JObject Build(string action, AdviceInForce advice, int? itemUid = null, JObject extra = null)
        {
            var o = new JObject();
            if (advice != null && advice.ClientSeq > 0) o["client_seq"] = advice.ClientSeq;
            o["action"] = action;
            if (itemUid.HasValue) o["item_uid"] = itemUid.Value;
            if (advice != null && advice.ClientSeq > 0 && !string.IsNullOrEmpty(advice.DecisionId))
            {
                o["decision_id"] = advice.DecisionId;
                if (!string.IsNullOrEmpty(advice.Advised)) o["advised"] = advice.Advised;
            }
            if (extra != null)
                foreach (var p in extra.Properties())
                    if (o[p.Name] == null) o[p.Name] = p.Value;
            return o;
        }

        // The choice the player clicked and everything that was on offer, as the bubbles showed it.
        public static JObject Choice(int index, IList<ChoiceInfo> choices, IList<string> history)
        {
            var o = new JObject { ["index"] = index };
            var picked = choices != null && index >= 0 && index < choices.Count ? choices[index] : null;
            if (picked != null)
            {
                o["text"] = FeatureText.Plain(picked.Text);
                if (!string.IsNullOrEmpty(picked.Label)) o["label"] = picked.Label;
                o["enabled"] = picked.Enabled;
            }
            o["choices"] = new JArray((choices ?? new List<ChoiceInfo>()).Select(c => (object)new JObject
            {
                ["text"] = FeatureText.Plain(c.Text),
                ["enabled"] = c.Enabled,
            }).ToArray());
            if (history != null && history.Count > 0)
                o["history"] = new JArray(history.Select(FeatureText.Plain).Where(s => s.Length > 0).Cast<object>().ToArray());
            return o;
        }
    }

    public sealed class ChoiceInfo
    {
        public string Label;
        public string Text;
        public bool Enabled;
    }

    // services_set: the final selection at sleep against the one before the night.
    public static class ServiceSelection
    {
        public static JObject Diff(IList<ServiceDay> before, IList<ServiceDay> now)
        {
            var current = (now ?? new List<ServiceDay>()).Where(s => s != null && s.Unlocked).ToList();
            var selected = current.Where(s => s.Using).Select(s => s.Id).ToList();
            var o = new JObject
            {
                ["selected"] = new JArray(selected.Cast<object>().ToArray()),
                ["costs"] = new JObject(current.Select(s => new JProperty(s.Id, s.Cost)).ToArray()),
                ["total_cost"] = current.Where(s => s.Using).Sum(s => s.Cost),
            };
            if (before != null)
            {
                var was = new HashSet<string>(before.Where(s => s != null && s.Using).Select(s => s.Id), StringComparer.Ordinal);
                o["turned_on"] = new JArray(selected.Where(id => !was.Contains(id)).Cast<object>().ToArray());
                o["turned_off"] = new JArray(was.Where(id => !selected.Contains(id)).OrderBy(id => id, StringComparer.Ordinal).Cast<object>().ToArray());
            }
            return o;
        }
    }
}
