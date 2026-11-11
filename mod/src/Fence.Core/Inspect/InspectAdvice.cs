using System.Collections.Generic;
using System.Globalization;
using Fence.Barter;
using Fence.Protocol;
using Fence.Recording;
using Newtonsoft.Json.Linq;

namespace Fence.Inspect
{
    public static class AppraisalFeedback
    {
        public const string TooHigh = "too_high";
        public const string TooLow = "too_low";
        public const string Accepted = "accepted";
        public const string WalkedOut = "walked_out";
    }

    // One customer's inspections, minus Unity and HTTP: ask once per question signature, remember the
    // answers, the appraisal exchange per item (what the customer said to each estimate) and the verdict
    // the player submitted, so later moves and results attach to the right decision.
    public sealed class InspectTracker
    {
        sealed class Asked
        {
            public string DecisionId;
            public InspectQuestion Question;
            public Advice Answer;
        }

        sealed class Exchange
        {
            public int Refusals;
            public int? LastEstimate;
            public string LastFeedback;
            public int? Pending;
        }

        readonly Dictionary<string, Asked> bySignature = new Dictionary<string, Asked>();
        readonly Dictionary<int, Exchange> appraisals = new Dictionary<int, Exchange>();
        Asked current;
        int? pendingUid;
        string verdictItemId;
        string verdictIssue;

        public object ClientKey { get; private set; }

        public void NewClient(object key)
        {
            ClientKey = key;
            bySignature.Clear();
            appraisals.Clear();
            current = null;
            pendingUid = null;
            verdictItemId = verdictIssue = null;
        }

        // Makes q the item on screen (null: one with nothing to decide); true when q has not been asked yet.
        public bool Show(InspectQuestion q)
        {
            if (q == null)
            {
                current = null;
                return false;
            }
            if (bySignature.TryGetValue(q.Signature, out var a))
            {
                current = a;
                return false;
            }
            current = new Asked { Question = q };
            return true;
        }

        public void Sent(InspectQuestion q, string decisionId)
        {
            if (current == null || current.Question != q) current = new Asked { Question = q };
            current.DecisionId = decisionId;
            bySignature[q.Signature] = current;
        }

        public void Answered(string signature, Advice advice)
        {
            if (bySignature.TryGetValue(signature, out var a)) a.Answer = advice;
        }

        public InspectQuestion Current => current?.Question;
        public Advice CurrentAnswer => current?.Answer;
        public bool Thinking => current != null && current.DecisionId != null && current.Answer == null;

        // The newest decision about item `itemId` (game identifier), for results that name the item.
        public string DecisionFor(string itemId, out string advised)
        {
            advised = null;
            if (current == null || current.DecisionId == null || itemId != null && current.Question.ItemId != itemId) return null;
            advised = current.Answer == null ? null : current.Answer.Deferred ? Choice.Defer : current.Answer.OptionId;
            return current.DecisionId;
        }

        public AdviceInForce InForce(int clientSeq)
        {
            string id = DecisionFor(null, out var advised);
            return id == null ? null : new AdviceInForce { ClientSeq = clientSeq, DecisionId = id, Advised = advised };
        }

        public AppraisalTerms Terms(int uid, AppraisalTerms fromGame)
        {
            var t = fromGame ?? new AppraisalTerms();
            if (appraisals.TryGetValue(uid, out var e))
            {
                t.Refusals = e.Refusals;
                t.LastEstimate = e.LastEstimate;
                t.LastFeedback = e.LastFeedback;
            }
            return t;
        }

        public void Estimated(int uid, int value)
        {
            if (!appraisals.TryGetValue(uid, out var e)) appraisals[uid] = e = new Exchange();
            e.Pending = value;
            pendingUid = uid;
        }

        // What the customer said to the estimate in flight (on item uid, or the last one estimated);
        // returns that estimate, or null when none is in flight.
        public int? Feedback(int? uid, string feedback, out int estimatedUid)
        {
            estimatedUid = uid ?? pendingUid ?? 0;
            if (!appraisals.TryGetValue(estimatedUid, out var e) || !e.Pending.HasValue) return null;
            int value = e.Pending.Value;
            e.Pending = null;
            e.LastEstimate = value;
            e.LastFeedback = feedback;
            if (feedback == AppraisalFeedback.TooHigh || feedback == AppraisalFeedback.TooLow) e.Refusals++;
            return value;
        }

        public void Verdict(string itemId, string issue)
        {
            verdictItemId = itemId;
            verdictIssue = issue;
        }

        public string VerdictFor(string itemId) => itemId != null && itemId == verdictItemId ? verdictIssue : null;
    }

    // The overlay's inspect line: "inspect: confront serial — Looks unusable: serial 2514471: prefix 251 is recalled."
    public static class InspectText
    {
        public static string Line(InspectQuestion q, Advice a, bool thinking)
        {
            if (q == null) return "";
            if (thinking) return "inspect: thinking…";
            if (a == null) return "";
            if (a.Deferred) return "inspect: deferred to you";
            string rationale = OverlayText.Clean(a.Rationale, 160);
            string head = "inspect: " + Verdict(q, a);
            return rationale.Length > 0 ? head + " — " + rationale : head;
        }

        static string Verdict(InspectQuestion q, Advice a)
        {
            string id = a.OptionId ?? "";
            if (id.StartsWith(InspectOptionIds.ConfrontPrefix, System.StringComparison.Ordinal))
            {
                string issue = id.Substring(InspectOptionIds.ConfrontPrefix.Length);
                foreach (var x in ConfrontIssues.Of(q.ItemKind))
                    if (x.Id == issue) return "confront " + x.Caption;
                return "confront " + issue;
            }
            switch (id)
            {
                case InspectOptionIds.Genuine: return q.Claim == "counterfeit" ? "looks genuine — tell them" : "looks genuine — no confront";
                case InspectOptionIds.Counterfeit: return "agree it is counterfeit";
                case InspectOptionIds.Estimate:
                    var v = a.Params?[InspectOptionIds.EstimateParam];
                    return v != null && v.Type == JTokenType.Integer ? "estimate " + ((long)v).ToString(CultureInfo.InvariantCulture) : "estimate";
            }
            return OverlayText.Clean(a.Label ?? id, 80);
        }
    }

    // POST /v1/outcome results for inspect decisions.
    public static class InspectOutcomes
    {
        public static JObject Confront(string issue, string confrontType, string outcome)
        {
            var o = new JObject { ["kind"] = "confront", ["confront_type"] = confrontType, ["outcome"] = outcome };
            if (issue != null) o["issue"] = issue;
            return o;
        }

        // trueValue: printed on the accepted slip, so visible once the estimate is settled.
        public static JObject Appraisal(int estimate, string feedback, int? trueValue)
        {
            var o = new JObject { ["kind"] = "appraisal", ["estimate"] = estimate, ["feedback"] = feedback };
            if (feedback == AppraisalFeedback.Accepted && trueValue.HasValue) o["true_value"] = trueValue.Value;
            return o;
        }

        public static string Describe(JObject result)
        {
            switch ((string)result?["kind"])
            {
                case "confront":
                    string what = (string)result["confront_type"] == "talk" ? "told them it is genuine" : "confronted " + ((string)result["issue"] ?? "");
                    return what.TrimEnd() + ": " + ((string)result["outcome"] == "success" ? "right" : "wrong, -2 rep");
                case "appraisal":
                    string fb = ((string)result["feedback"] ?? "").Replace('_', ' ');
                    return "estimate " + ((long?)result["estimate"] ?? 0).ToString(CultureInfo.InvariantCulture) + ": " + fb;
            }
            return null;
        }
    }
}
