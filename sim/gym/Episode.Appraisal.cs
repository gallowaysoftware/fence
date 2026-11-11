using System.Collections.Generic;
using System.Linq;
using Fence.Mod;
using Fence.Recording;
using Fence.Sim;
using Newtonsoft.Json.Linq;

namespace Fence.Gym
{
    // An appraisal client at the counter (`appraisal: join`, Appraisal.cs). The magnifier opens the
    // inspection view on each of the client's items and its appraisal panel takes the estimate
    // (InspectionUIManager.OnAppraisalServiceEstimated); headless, the view is stubbed, so the executor reads the
    // cues the view would draw and submits through the same AppraisalServiceHelper.OnEstimate.
    internal sealed partial class Episode
    {
        // True when an estimate was submitted (the client's reply then plays through the dialogue loop).
        bool PlayerAppraise()
        {
            if (!IsPlayer || !Appraisal.On) return false;
            var e = EmporiumEntry.Instance;
            var a = Appraisal.Next(GameBarter.CurrentClient(), e.frontInvinvElement.childItems);
            if (a == null) return false;
            var item = a.Item;
            int value = a.Value, estimate = a.Estimate;
            int truth = AppraisalHelper.GetCorrectBaseValue(item);
            int cash0 = Cash, uid = Mod.Game.Uid(item);
            int rank = AppraisalRankHelper.GetRank();
            Mod.Human("appraise", Advice(), uid, new JObject { ["estimate"] = estimate, ["cues"] = a.Cues }, "player:appraise");
            Dialogue reply = null;
            Guard("appraise", () =>
            {
                AppraisalServiceHelper.OnEstimate(item, estimate);
                reply = DialogUIManager.Instance.GetCurrentDialog();
            });
            bool accepted = item.parentInventory == null || !ReferenceEquals(item.parentInventory, e.frontInvinvElement);
            int fee = Cash - cash0;
            var data = new JObject { ["client_seq"] = ClientSeq, ["type"] = a.Cues["type"], ["estimate"] = estimate, ["read_value"] = value, ["try"] = a.Try + 1, ["rank"] = rank };
            if (accepted)
            {
                int grade = Grade(truth, estimate);
                Appraisal.Items++;
                Appraisal.Fees += fee;
                Appraisal.Grades[grade - 1]++;
                if ((bool?)a.Cues["fine"] == true) Appraisal.FineCueItems++;
                data["fee"] = fee;
                data["grade"] = grade;
                // The slip names the true value: visible once the estimate is in.
                data["slip_true_value"] = truth;
                DisposeSlips();
            }
            else
            {
                bool walked = !Present();
                int feedback = Appraisal.Refused(a, reply);
                if (walked) Appraisal.Walkouts++;
                data["refused"] = feedback < 0 ? "too_low" : "too_high";
            }
            if (value != truth) Appraisal.Mismatches++;
            Mod.Emit("appraisal", data, new JObject { ["true_value"] = truth });
            Scheduler.Settle();
            return true;
        }

        static int Grade(int truth, int estimate)
        {
            if (truth <= 0) return 5;
            int lo = RNG.CalculatePercentage(truth, 95), hi = RNG.CalculatePercentage(truth, 105);
            if (estimate >= lo && estimate <= hi) return 1;
            if (estimate < lo) return estimate <= RNG.CalculatePercentage(truth, 75) ? 5 : estimate <= RNG.CalculatePercentage(truth, 85) ? 4 : estimate <= RNG.CalculatePercentage(truth, 90) ? 3 : 2;
            return estimate >= RNG.CalculatePercentage(truth, 125) ? 5 : estimate >= RNG.CalculatePercentage(truth, 115) ? 4 : estimate >= RNG.CalculatePercentage(truth, 110) ? 3 : 2;
        }

        // The board hands its kit (certificate, magnifier, guides) over on the counter, as the tutorial does the
        // labeler, and the counter is wiped every night: into the dossier, which takes documents, the magnifier and
        // the labeler, else back to stock with `left`.
        void StowKit(List<GameItem> left)
        {
            var dossier = Appraisal.Dossier();
            foreach (var k in Appraisal.KitOnCounter())
            {
                bool ok = false;
                if (dossier != null) Guard("stow kit", () => ok = GraphUtils.TryAcceptAll(dossier, k, -1) == 0);
                if (ok) Mod.Human(HumanActions.Unstage, null, Mod.Game.Uid(k), new JObject { ["owned"] = true, ["to"] = "dossier" }, "player:appraisal_kit");
                else left.Add(k);
            }
        }

        // A slip the bin cannot take goes with the counter at night.
        void DisposeSlips()
        {
            var bin = EmporiumEntry.Instance.trashcan?.contentWindow?.child as GameInventory;
            foreach (var slip in Appraisal.Slips())
                if (bin != null) Guard("bin slip", () => GraphUtils.TryAcceptAll(bin, slip, -1));
        }
    }
}
