using System;
using System.Diagnostics;
using System.IO;
using System.Linq;
using System.Net.Http;
using System.Threading;
using Fence.Mod;
using Fence.Recording;
using Newtonsoft.Json;
using Newtonsoft.Json.Linq;

namespace Fence.Gym
{
    // Where the executor's daily plan comes from (Episode.Planner):
    // - `-step-dir DIR`: a file exchange with an agent playing the game one day at a time. Each morning the
    //   gym writes DIR/day-<N>/snapshot.json, waits for DIR/day-<N>/directives.json (or the timeout, then
    //   plays the base directives), plays the day and writes DIR/day-<N>/result.json; at the end
    //   DIR/final.json. A file is written to a temp name and renamed, so a reader never sees half of it.
    // - `-bridge URL`: the bridge's /v1/summary `directives` (docs/PROTOCOL.md), read each morning.
    // - otherwise none: the base directives (-directives FILE, knobs) alone.
    internal static class StepMode
    {
        public static Func<Episode, Directives> Planner(GymOptions o, IBridge bridge)
        {
            if (!string.IsNullOrEmpty(o.StepDir))
            {
                System.IO.Directory.CreateDirectory(o.StepDir);
                return ep => Step(ep, o);
            }
            if (bridge is HttpBridge http) return ep => http.SummaryDirectives(ep.Mod.SessionId);
            return null;
        }

        static void WriteAtomic(string path, JToken body)
        {
            string tmp = path + ".tmp";
            File.WriteAllText(tmp, body.ToString(Formatting.Indented) + "\n");
            File.Move(tmp, path, overwrite: true);
        }

        static string DayDir(GymOptions o, int day) => Path.Combine(o.StepDir, "day-" + day.ToString("000"));

        static Directives Step(Episode ep, GymOptions o)
        {
            int day = ep.Day;
            string dir = DayDir(o, day);
            System.IO.Directory.CreateDirectory(dir);
            if (ep.DayEnded == null) ep.DayEnded = (e, result) => WriteAtomic(Path.Combine(DayDir(o, result.Value<int>("day")), "result.json"), Result(e, result));
            if (ep.RunEnded == null) ep.RunEnded = e => WriteAtomic(Path.Combine(o.StepDir, "final.json"), Final(e, o));
            WriteAtomic(Path.Combine(dir, "snapshot.json"), Snapshot(ep, o));
            string want = Path.Combine(dir, "directives.json");
            var sw = Stopwatch.StartNew();
            while (!File.Exists(want) && sw.Elapsed.TotalSeconds < o.StepTimeoutSec) Thread.Sleep(150);
            if (!File.Exists(want))
            {
                ep.StepNote = "no directives.json within " + o.StepTimeoutSec + " s: played the base directives";
                return null;
            }
            for (int attempt = 0; ; attempt++)
            {
                try
                {
                    var d = Directives.Load(want);
                    ep.StepNote = null;
                    return d;
                }
                catch (Exception e) when (e is IOException || e is JsonException)
                {
                    // A writer that did not rename atomically: let it finish.
                    if (attempt >= 20)
                    {
                        ep.StepNote = "directives.json unreadable (" + e.Message + "): played the base directives";
                        return null;
                    }
                    Thread.Sleep(150);
                }
                catch (ArgumentException e)
                {
                    ep.StepNote = "directives.json rejected (" + e.Message + "): played the base directives";
                    return null;
                }
            }
        }

        // The morning view: the same day_snapshot the live mod sends, yesterday's result, and the risks to the
        // run the player can read off the screen (rent, the permit, the record, aug escapes so far).
        static JObject Snapshot(Episode ep, GymOptions o)
        {
            var snap = DaySnapshot.Build(ep.Mod.DayReads, SnapshotPhases.Morning, GameDay.RunId(), null);
            var ps = PlayerStore.Instance;
            int fee = PlayerStore.GetBusinessPermitPrice();
            int day = ep.Day;
            var risks = new JObject
            {
                ["rent"] = new JObject
                {
                    ["due"] = ps.GetRentValue(), ["days_left"] = ps.dayUntilRent, ["cash"] = ep.Cash, ["covered"] = ep.Cash >= ps.GetRentValue(),
                    ["landlord_grace_owned"] = NetworkUpgrade.IsLandlordGraceUnlocked(),
                    ["landlord_grace_cooldown_days"] = ps.networkUpgrade.TryGetValue("LANDLORD_GRACE", out var lg) ? lg.cooldownCurrent : 0,
                    ["note"] = "an unpaid rent ends the run unless Landlord Grace is owned (it is used up; re-buy it from Wild)",
                },
                ["permit"] = new JObject
                {
                    ["owned"] = ps.hasBusinessPermit, ["fee"] = fee, ["bribe_owed"] = ps.storeClientData.businessPermitBribeAmount,
                    ["bribe_failing"] = PlayerRole.BribeFailing,
                    ["official_visits"] = "from day 38 every day with day % 7 == 3 until the permit is bought; refusing commits OPERATING_WITHOUT_LICENSE (50-170); from day 45 he accepts donated goods worth the week's bribe instead",
                    ["next_visit_day"] = ps.hasBusinessPermit ? (int?)null : NextVisit(day),
                },
                ["record"] = new JObject
                {
                    ["crime"] = ps.secData?.GetTotalCrimeValue() ?? 0, ["evidence"] = ps.secData?.evidenceLevel ?? 0,
                    ["crimes"] = Crimes(ps),
                    ["evidence_days_to_100"] = EvidenceDays(ps),
                    ["note"] = "evidence 100 brings a sentencing officer: arrest at 300+ crime",
                },
                ["augs"] = new JObject
                {
                    ["introduced"] = ps.isAugIntroduced, ["ended"] = ps.isAugEnded,
                    ["good_kills"] = ep.Ledger.GoodKills, ["left_alive"] = ep.Ledger.LeftAlive,
                    // Reconstructible from the night log's escape lines and the kill verdicts (docs/re/AUGS.md §4.1).
                    ["presence_reconstructed"] = ps.augPresence,
                    ["game_over_at_100"] = Difficulty.Rules?.augGameOverAtFullPresence ?? false,
                },
                ["buyout"] = new JObject { ["price"] = ps.GetStoreBuyPrice(false), ["affordable"] = ps.CanBuyOutStore(), ["offered_on"] = "the landlord's rent-day visit, when cash covers the price" },
                ["mortgage"] = ps.isPayingMortgage
                    ? new JObject
                    {
                        ["paying"] = true, ["weekly"] = ps.mortgageWeeklyPayment, ["weeks_left"] = ps.mortgageWeekLeft, ["days_to_payment"] = ps.dayUntilRent,
                        ["repossessable_stock"] = PlayerStock.RepoValue(),
                        // Cash and repossessable stock together short of a payment: the next collection ends the run.
                        ["game_over_if_unpaid"] = ep.Cash + PlayerStock.RepoValue() < ps.mortgageWeeklyPayment,
                        ["note"] = "the last payment wins the run (buyout_mortgage); a missed one repossesses goods worth it, or ends the run without enough goods" }
                    : new JObject
                    {
                        ["paying"] = false, ["down_payment"] = ps.GetMortgageDownPayment(), ["weekly"] = ps.GetMortgageWeeklyPaymentPreview(), ["weeks"] = PlayerStore.GetMortgageWeeks(),
                        ["offered_at_cash"] = 2 * ps.GetMortgageDownPayment(), ["offerable_now"] = ps.CanBeOfferedMortgage(),
                        ["intro_seen"] = ps.storeClientManager.uniqueClientTracker.ContainsKey("loan_shark"),
                        ["declined_for_good"] = ps.IsMortgageOfferDeclined,
                        ["cash_needed_by_rent_morning"] = Math.Max(0, 2 * ps.GetMortgageDownPayment() - ep.Cash),
                        ["note"] = "the loan shark comes on a rent day once cash >= 2x the down payment; directive mortgage: take|decline|auto|accept_if_rent",
                    },
                ["wild"] = Wild(ps),
                ["georgio"] = ps.loanOfferedToGeorgio == 0 ? null : new JObject { ["lent"] = ps.loanOfferedToGeorgio, ["lent_on_day"] = ps.dayWhenLoanOfferedGeorgio, ["payback_from_day"] = ps.dayWhenLoanOfferedGeorgio + 6, ["note"] = "he pays back on a rent day once you are short of the rent (StoreClientManager.CheckRentDay)" },
            };
            return new JObject
            {
                ["day"] = day,
                ["seed"] = o.Seed,
                ["difficulty"] = o.Difficulty,
                ["start"] = o.Start,
                ["days_requested"] = o.Days,
                ["respond_with"] = Path.Combine(DayDir(o, day), "directives.json"),
                ["timeout_seconds"] = o.StepTimeoutSec,
                ["snapshot"] = snap.Data,
                ["yesterday"] = ep.Ledger.Days.LastOrDefault(),
                ["risks"] = risks,
                // Item types (the tooltip's category line) for every owned item, by uid.
                ["item_types"] = ItemTypes(),
                ["base_directives"] = ep.BaseDirectives?.ToJson(),
            };
        }

        static JObject ItemTypes()
        {
            var o = new JObject();
            foreach (var i in EmporiumEntry.Instance.GetAllOwnedItems(false, true))
                if (i != null && i.uniqueId != 0) o[i.uniqueId.ToString()] = new JArray(i.GetGameItemType().Select(t => (object)t.ToLowerInvariant()).ToArray());
            return o;
        }

        static JObject Wild(PlayerStore ps)
        {
            var o = new JObject { ["favor"] = ps.wildFavor };
            var avail = new JArray();
            try
            {
                foreach (var u in WildNetwork.View().Upgrades.Where(u => !u.Unlocked && u.State == 1))
                    avail.Add(new JObject { ["id"] = u.Id, ["cost"] = u.Cost, ["buyable_now"] = u.Buyable, ["ready"] = u.Ready, ["cooldown_days"] = u.Cooldown });
                o["note"] = "favor pays first; an upgrade on cooldown (e.g. Landlord Grace for 15 days after use) or missing a prerequisite is not ready";
            }
            catch (Exception) { }
            o["available"] = avail;
            return o;
        }

        static JObject Crimes(PlayerStore ps)
        {
            var o = new JObject();
            var sd = ps.secData;
            for (int i = 0; i < (sd?.crimeTypes?.Count ?? 0); i++)
                if (sd.crimeAmount[i] > 0) o[sd.crimeTypes[i]] = sd.crimeAmount[i];
            return o;
        }

        // SecData.OnNewDay: evidence grows by the total crime's band (100-299: +2 a day, 300-849: +6,
        // 850-1199: +9, more above); at 100 a sentencing officer comes. A rough projection, not a promise.
        static int? EvidenceDays(PlayerStore ps)
        {
            int crime = ps.secData?.GetTotalCrimeValue() ?? 0, ev = ps.secData?.evidenceLevel ?? 0;
            int rate = crime >= 1200 ? 12 : crime >= 850 ? 9 : crime >= 300 ? 6 : crime >= 100 ? 2 : 0;
            return rate == 0 ? (int?)null : Math.Max(0, (100 - ev + rate - 1) / rate);
        }

        static int NextVisit(int day)
        {
            int d = Math.Max(day, 38);
            while (d % 7 != 3) d++;
            return d;
        }

        static JObject Result(Episode ep, JObject day)
        {
            var r = (JObject)day.DeepClone();
            r["directives_used"] = Directives.Current.ToJson();
            if (ep.StepNote != null) r["note"] = ep.StepNote;
            r["game_over"] = Episode.GameOverReason();
            r["night_log_tomorrow"] = "in the next snapshot (snapshot.night_log)";
            return r;
        }

        static JObject Final(Episode ep, GymOptions o) => new JObject
        {
            ["day"] = ep.Day,
            ["game_over"] = ep.GameOver,
            ["cash"] = ep.Cash,
            ["net_worth"] = PlayerStore.Instance.GetTotalEstimatedValue(),
            ["days_played"] = ep.Stats.CashByDay.Count,
            ["player"] = ep.Ledger.Summary(),
        };
    }
}
