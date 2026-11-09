using System.Collections.Generic;
using Fence.Protocol;

namespace Fence.Barter
{
    public enum AdviceStatus
    {
        None,
        Thinking,
        Ready,
        Stale,
    }

    public enum ActState
    {
        // Advice only: advise mode, or acting is off/paused for this answer.
        None,
        Pending,
        Executed,
        Stale,
        Blocked,
    }

    public sealed class Advice
    {
        public string DecisionId;
        // The decide kind: "barter" or StageContext.Kind.
        public string Kind;
        public string Signature;
        public bool Deferred;
        public string OptionId;
        public string Label;
        public string Rationale;
        public long ReceivedMs;
        // The options the decision offered and the model's per-option notes (may be null).
        public List<DecisionOption> Options;
        public Dictionary<string, string> Notes;
        public ActState Act;
        // Why an act answer was dropped or not executed.
        public string ActNote;
    }

    public sealed class AdviceView
    {
        public AdviceStatus Status;
        public Advice Advice;
        // The bridge answered defer for the current state.
        public bool Deferred;
    }

    // Per-customer advise loop, minus Unity and HTTP: ask once per distinct state signature after
    // it has been stable for StableMs (dragging items must not spam the bridge), keep at most one
    // decide in flight, and let the newest signature win once the in-flight one returns.
    public sealed class AdviceTracker
    {
        public const long StableMs = 750;

        readonly Dictionary<string, Advice> answers = new Dictionary<string, Advice>();
        readonly HashSet<string> asked = new HashSet<string>();
        readonly Dictionary<string, string> answeredByDecision = new Dictionary<string, string>();
        string observed;
        long observedSince;
        string inFlightSig;
        Advice lastShown;

        public object ClientKey { get; private set; }
        public int ClientSeq { get; private set; }
        // Outcomes are attributed to the newest decision sent for this customer.
        public string LastDecisionId { get; private set; }
        public bool InFlight => inFlightSig != null;
        public string CurrentSignature => observed;

        public void NewClient(object key)
        {
            ClientKey = key;
            ClientSeq++;
            answers.Clear();
            asked.Clear();
            answeredByDecision.Clear();
            observed = null;
            inFlightSig = null;
            lastShown = null;
            LastDecisionId = null;
        }

        // Called every poll with the current signature (null while no decision is ready). Returns
        // the signature to ask about now, or null.
        public string Observe(string signature, long nowMs, bool canAsk)
        {
            if (signature != observed)
            {
                observed = signature;
                observedSince = nowMs;
            }
            if (signature == null || !canAsk || inFlightSig != null || asked.Contains(signature)) return null;
            if (nowMs - observedSince < StableMs) return null;
            return signature;
        }

        public void Sent(string signature, string decisionId)
        {
            asked.Add(signature);
            inFlightSig = signature;
            LastDecisionId = decisionId;
        }

        // A late answer for a customer who already left is dropped by the caller (ClientKey check).
        public void Answered(Advice advice)
        {
            if (advice.Signature == inFlightSig) inFlightSig = null;
            answers[advice.Signature] = advice;
            if (advice.DecisionId != null) answeredByDecision[advice.DecisionId] = advice.Deferred ? Protocol.Choice.Defer : advice.OptionId;
            if (!advice.Deferred) lastShown = advice;
        }

        // The bridge's answer to a decision of this customer: an option id, "defer", or null if none yet.
        public string AnswerTo(string decisionId) =>
            decisionId != null && answeredByDecision.TryGetValue(decisionId, out var option) ? option : null;

        // The answer for a state signature, or null if none yet.
        public Advice AnswerFor(string signature) => signature != null && answers.TryGetValue(signature, out var a) ? a : null;

        // The answer waiting to be executed, if any (at most one: the newest).
        public Advice PendingAct => lastShown != null && lastShown.Act == ActState.Pending ? lastShown : null;

        // "Ask again": forget the answer for the current signature so the next stable poll asks anew.
        public bool AskAgain()
        {
            if (observed == null || inFlightSig != null) return false;
            asked.Remove(observed);
            answers.Remove(observed);
            return true;
        }

        public AdviceView View(bool canAsk)
        {
            if (observed != null && answers.TryGetValue(observed, out var a))
                return a.Deferred ? new AdviceView { Status = AdviceStatus.None, Deferred = true } : new AdviceView { Status = AdviceStatus.Ready, Advice = a };
            if (observed != null && canAsk && (inFlightSig == observed || !asked.Contains(observed)))
                return new AdviceView { Status = AdviceStatus.Thinking };
            if (lastShown != null) return new AdviceView { Status = AdviceStatus.Stale, Advice = lastShown };
            return new AdviceView { Status = AdviceStatus.None };
        }
    }
}
