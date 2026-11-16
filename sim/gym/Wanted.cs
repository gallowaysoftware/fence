using System;

namespace Fence.Gym
{
    // The wanted posters (docs/re/CLIENTS.md, "The wanted list"): a poster face at the counter can be reported for its
    // bounty (+5 Security before the difficulty factor; the poster's faction changes by its fixed rule), and a reported
    // poster stops its return visits. `wanted`: "report" (every poster but Lou's), "report_all" (Lou too: her
    // report starts the chain that ends in a 1 000 demand on hard, a game over if underpaid), or "dismiss" (default "report": hard, 60 seeds, −4.7 ± 3.8 days on appraiser).
    // `wanted_rep_floor` (default -60): a report that would cost a faction rep is skipped once that faction is below it.
    internal static class Wanted
    {
        public const string Lou = "wanted5";

        public static int Reported, Bounties, Skipped;

        static Directives D => Directives.Current;

        public static string Mode => D.Raw.Value<string>("wanted") ?? "report";

        public static bool Report(StoreClient c)
        {
            string mode = Mode;
            if (mode == "dismiss" || !Known.OnPosterWall(c) || !c.CanReport()) return false;
            if (c.wantedPosterId == Lou && mode != "report_all") return Skip();
            if (c.reportDegradesFaction && !c.reportImprovesFaction && c.reputationPenalty != 0
                && StoreReputation.GetStoreReputation(c.clientFaction) is StoreReputation r && r.GetReputationExact() < (D.Raw.Value<int?>("wanted_rep_floor") ?? -60))
                return Skip();
            return true;
        }

        static bool Skip()
        {
            Skipped++;
            return false;
        }

        public static Newtonsoft.Json.Linq.JObject Status() => Reported + Skipped == 0 ? null
            : new Newtonsoft.Json.Linq.JObject { ["reported"] = Reported, ["bounties"] = Bounties, ["skipped"] = Skipped };
    }
}
