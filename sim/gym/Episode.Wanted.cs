using System;
using Fence.Mod;
using Fence.Recording;
using Fence.Sim;
using Newtonsoft.Json.Linq;

namespace Fence.Gym
{
    internal sealed partial class Episode
    {
        object reportAsked;

        // The Security button on a poster face (StoreSecButton.OnClick → PlayerStore.OnCustomerReported), then what the
        // presentation layer does headless-less: Security arrives (StoreUIManager.OnGenericArrived "SEC": the arrest
        // lines, whose end action dismisses the client), leaves (OnGenericLeft: the bounty), and the door reopens.
        // True when the customer is gone. A storyline intercept (tryInterceptReport) leaves them at the counter.
        bool PlayerReportWanted()
        {
            if (!IsPlayer) return false;
            var c = GameBarter.CurrentClient();
            if (c == null || ReferenceEquals(reportAsked, c)) return false;
            reportAsked = c;
            if (!Wanted.Report(c)) return false;
            var ps = PlayerStore.Instance;
            var dlg = DialogUIManager.Instance;
            var ui = StoreUIManager.Instance;
            var target = ps.currentClientInstance;
            int cash0 = Cash;
            Mod.Human(HumanActions.Report, Advice(), null, new JObject { ["poster"] = c.wantedPosterId, ["bounty"] = c.reportReward }, "player:report_wanted");
            Guard("report", () => ps.OnCustomerReported());
            if (!c.reported) return !Present();
            Scheduler.Settle();
            bool started = false;
            Guard("security arrives", () =>
            {
                dlg.InterruptDialog();
                dlg.HideChoiceBubble();
                started = dlg.StartDialogue(c.customOnArrestDialogue ?? StoreClientDialogList.ClientArrestDialogCriminal(), forcedStart: true);
            });
            Scheduler.Settle();
            for (int i = 0; started && i < 40 && !Over() && ReferenceEquals(ps.currentClientInstance, target); i++)
            {
                var cur = dlg.GetCurrentDialog();
                if (cur == null) break;
                if (cur.choices != null && cur.choices.Count > 0) Guard("choice", () => dlg.SelectChoice(0));
                else Guard("advance", () => dlg.ContinueConversation());
                Scheduler.Pump(1.0);
                Scheduler.Settle();
            }
            Guard("leave sec", () =>
            {
                if (ReferenceEquals(ps.currentClientInstance, target)) ps.DismissCurrentClient(false, false);
                ui.isShot = false;
                ui.shotClientInstance = null;
                ps.isHandlingShootout = false;
                dlg.isSecMonologuePlaying = false;
                ps.isClientBeingArrested = false;
                ps.ClientFlagDown();
                ps.ClientFlagDown();
            });
            Guard("door", () => ui.StoreGlass.OpenGlassDoor());
            Guard("bounty", () =>
            {
                var paid = ps.onSecAlarmLeftAction;
                ps.onSecAlarmLeftAction = null;
                paid?.Invoke();
            });
            Scheduler.Settle();
            Wanted.Reported++;
            Wanted.Bounties += Cash - cash0;
            Ledger.Flow("wanted_bounties", Cash - cash0);
            return !Present();
        }
    }
}
