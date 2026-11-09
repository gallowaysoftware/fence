using System.Collections.Generic;
using Fence.Decisions;
using Fence.Recording;
using Fence.Protocol;
using Newtonsoft.Json.Linq;

namespace Fence.Bridge
{
    public enum SessionState
    {
        Idle,
        NeedHello,
        HelloInFlight,
        Active,
        Vanilla,
    }

    public enum DecideFailure
    {
        None,
        Timeout,
        Transport,
        UnknownSession,
        BadResponse,
    }

    public sealed class BridgeOptions
    {
        public int BreakerThreshold = 3;
        public long BreakerCooldownMs = 30000;
        // Recording buffers while the bridge is down (docs/PROTOCOL.md, Recording events). Past this,
        // the oldest low-value event goes first; high-value events may grow the queue up to the hard cap.
        public int MaxQueuedEvents = 4096;
        public int MaxQueuedEventsHard = 16384;
        // HTTP errors other than 404 (a transport failure is retried until the bridge answers).
        public int EventHttpAttempts = 3;
        public DeadlinePolicy Deadlines = new DeadlinePolicy();
    }

    public sealed class DecisionTicket
    {
        public DecideRequest Request;
        public bool Ask;
        public int EffectiveDeadlineMs;
        public long StartedMs;
        public string Mode;
        public string SkipReason;
        public bool Retried;
    }

    public sealed class DecisionOutcome
    {
        public Choice Choice;
        // True only in act mode with a validated non-defer choice; advise mode shows, never executes.
        public bool Execute;
        public string Reason;
        public DecideResponse Response;
        // 404: the bridge restarted. Say hello again and resend once (docs/PROTOCOL.md, Errors).
        public bool RetryAfterHello;
    }

    // Everything the mod decides about talking to the bridge, minus the I/O: the Unity side only
    // asks this object what to send next and reports what happened.
    public sealed class BridgeSession
    {
        sealed class PendingEvent
        {
            public long Seq;
            public string Kind;
            public int GameDay;
            public JObject Data;
            public JObject Hidden;
            public EventPriority Priority;
            public bool Retried;
            public int HttpFailures;
        }

        readonly BridgeOptions options;
        readonly LinkedList<PendingEvent> queue = new LinkedList<PendingEvent>();
        long nextSeq = 1;
        PendingEvent inFlight;

        public CircuitBreaker Breaker { get; }
        public SessionState State { get; private set; } = SessionState.Idle;
        public string SessionId { get; private set; }
        public string Mode { get; private set; } = Modes.Observe;
        public string VanillaReason { get; private set; }
        public bool Enabled { get; set; } = true;
        public long DroppedEvents { get; private set; }
        public int QueuedEvents => queue.Count + (inFlight != null ? 1 : 0);
        // Since when the bridge has not answered; null while it does.
        public long? UnreachableSinceMs { get; private set; }

        public BridgeSession(BridgeOptions options = null)
        {
            this.options = options ?? new BridgeOptions();
            Breaker = new CircuitBreaker(this.options.BreakerThreshold, this.options.BreakerCooldownMs);
        }

        public void RequestHello()
        {
            if (State == SessionState.Idle) State = SessionState.NeedHello;
        }

        public bool TryBeginHello(long nowMs)
        {
            if (!Enabled || State != SessionState.NeedHello || !Breaker.TryAcquire(nowMs)) return false;
            State = SessionState.HelloInFlight;
            return true;
        }

        public void HelloSucceeded(HelloResponse response)
        {
            Breaker.RecordSuccess();
            UnreachableSinceMs = null;
            if (response == null || string.IsNullOrEmpty(response.SessionId))
            {
                GoVanilla("hello response without session_id");
                return;
            }
            if (response.Protocol != ProtocolVersion.Current)
            {
                GoVanilla("bridge speaks protocol " + response.Protocol + ", mod speaks " + ProtocolVersion.Current);
                return;
            }
            SessionId = response.SessionId;
            Mode = response.Mode == Modes.Advise || response.Mode == Modes.Act ? response.Mode : Modes.Observe;
            State = SessionState.Active;
            nextSeq = 1;
            foreach (var e in queue) e.Seq = nextSeq++;
        }

        public void HelloRejected(string reason) => GoVanilla(reason);

        public void HelloFailed(long nowMs)
        {
            Breaker.RecordFailure(nowMs);
            Unreachable(nowMs);
            if (State == SessionState.HelloInFlight) State = SessionState.NeedHello;
        }

        public void GoVanilla(string reason)
        {
            State = SessionState.Vanilla;
            VanillaReason = reason;
            SessionId = null;
            DroppedEvents += queue.Count + (inFlight != null ? 1 : 0);
            queue.Clear();
            inFlight = null;
        }

        // Fire-and-forget: returns false when the event was dropped instead of queued. Queuing does not
        // start a hello; the caller does that on OnGameLoadedLate (docs/PROTOCOL.md). An unreachable
        // bridge does not drop: events wait in the bounded queue and flush in order when it returns.
        public bool Enqueue(string kind, int gameDay, JObject data, long nowMs) => Enqueue(kind, gameDay, data, null, nowMs);

        public bool Enqueue(string kind, int gameDay, JObject data, JObject hidden, long nowMs)
        {
            if (!Enabled || State == SessionState.Vanilla)
            {
                DroppedEvents++;
                return false;
            }
            var priority = RecordKinds.Priority(kind);
            if (queue.Count >= options.MaxQueuedEvents && !Evict(priority))
            {
                DroppedEvents++;
                return false;
            }
            queue.AddLast(new PendingEvent
            {
                Seq = State == SessionState.Active ? nextSeq++ : 0,
                Kind = kind,
                GameDay = gameDay,
                Data = data ?? new JObject(),
                Hidden = hidden,
                Priority = priority,
            });
            return true;
        }

        // Makes room for an incoming event: the oldest event of the lowest priority below High goes
        // first; High events are dropped (oldest first) only past the hard cap. False: drop the incoming one.
        bool Evict(EventPriority incoming)
        {
            for (var p = EventPriority.Low; p < EventPriority.High; p++)
            {
                if (p > incoming) break;
                for (var node = queue.First; node != null; node = node.Next)
                {
                    if (node.Value.Priority != p) continue;
                    queue.Remove(node);
                    DroppedEvents++;
                    return true;
                }
            }
            if (incoming != EventPriority.High) return false;
            if (queue.Count < options.MaxQueuedEventsHard) return true;
            queue.RemoveFirst();
            DroppedEvents++;
            return true;
        }

        void Unreachable(long nowMs)
        {
            if (!UnreachableSinceMs.HasValue) UnreachableSinceMs = nowMs;
        }

        public bool TryDequeueEvent(long nowMs, out EventRequest request)
        {
            request = null;
            if (!Enabled || State != SessionState.Active || inFlight != null || queue.Count == 0) return false;
            if (!Breaker.TryAcquire(nowMs)) return false;
            var e = queue.First.Value;
            queue.RemoveFirst();
            inFlight = e;
            request = new EventRequest
            {
                SessionId = SessionId,
                Seq = e.Seq,
                Kind = e.Kind,
                GameDay = e.GameDay,
                Data = e.Data,
                Hidden = e.Hidden,
            };
            return true;
        }

        public void EventSucceeded()
        {
            inFlight = null;
            Breaker.RecordSuccess();
            UnreachableSinceMs = null;
        }

        // Transport failures (no answer) keep the event at the head of the queue with its seq; the
        // bridge drops a resent duplicate by (session, seq). An HTTP error is retried a few times.
        public void EventFailed(long nowMs, bool transport = true)
        {
            var e = inFlight;
            inFlight = null;
            Breaker.RecordFailure(nowMs);
            if (transport) Unreachable(nowMs);
            if (e == null) return;
            if (!transport && ++e.HttpFailures >= options.EventHttpAttempts)
            {
                DroppedEvents++;
                return;
            }
            queue.AddFirst(e);
        }

        // 404: the bridge restarted and forgot us. Re-hello, then retry the event once (docs/PROTOCOL.md).
        public void SessionUnknown()
        {
            var e = inFlight;
            inFlight = null;
            if (e != null && !e.Retried)
            {
                e.Retried = true;
                queue.AddFirst(e);
            }
            else if (e != null)
            {
                DroppedEvents++;
            }
            ForgetSession();
        }

        // A 404 on a request that carries no queued event (GET /v1/summary).
        public void SessionForgotten() => ForgetSession();

        public DecisionTicket BeginDecision(DecideRequest request, long nowMs)
        {
            var ticket = new DecisionTicket
            {
                Request = request,
                Mode = Mode,
                StartedMs = nowMs,
                EffectiveDeadlineMs = options.Deadlines.Effective(request.DeadlineMs),
            };
            if (!Enabled) ticket.SkipReason = "disabled";
            else if (State != SessionState.Active) ticket.SkipReason = "no session (" + State + ")";
            else if (Mode == Modes.Observe) ticket.SkipReason = "observe mode";
            else if (!Breaker.TryAcquire(nowMs)) ticket.SkipReason = "circuit open";
            else ticket.Ask = true;

            if (ticket.Ask)
            {
                request.SessionId = SessionId;
                request.DeadlineMs = ticket.EffectiveDeadlineMs;
            }
            return ticket;
        }

        // True when a decide would actually be sent (not observe mode, no open breaker).
        public bool WantsDecisions(long nowMs) =>
            Enabled && State == SessionState.Active && Mode != Modes.Observe && !Breaker.IsOpen(nowMs);

        // The one resend after a 404 + fresh hello; keeps the original start and deadline.
        public DecisionTicket BeginRetry(DecisionTicket first, long nowMs)
        {
            var ticket = new DecisionTicket
            {
                Request = first.Request,
                Mode = Mode,
                StartedMs = first.StartedMs,
                EffectiveDeadlineMs = first.EffectiveDeadlineMs,
                Retried = true,
            };
            if (IsExpired(ticket, nowMs)) ticket.SkipReason = "deadline exceeded before retry";
            else if (!Enabled) ticket.SkipReason = "disabled";
            else if (State != SessionState.Active) ticket.SkipReason = "no session after re-hello (" + State + ")";
            else if (Mode == Modes.Observe) ticket.SkipReason = "observe mode";
            else if (!Breaker.TryAcquire(nowMs)) ticket.SkipReason = "circuit open";
            else ticket.Ask = true;
            if (ticket.Ask) ticket.Request.SessionId = SessionId;
            return ticket;
        }

        public bool IsExpired(DecisionTicket ticket, long nowMs) =>
            options.Deadlines.IsExpired(ticket.StartedMs, nowMs, ticket.EffectiveDeadlineMs);

        public DecisionOutcome CompleteDecision(DecisionTicket ticket, DecideResponse response, DecideFailure failure, long nowMs)
        {
            if (!ticket.Ask) return new DecisionOutcome { Choice = Choice.MakeDefer(), Reason = ticket.SkipReason };

            switch (failure)
            {
                case DecideFailure.None:
                    Breaker.RecordSuccess();
                    break;
                case DecideFailure.UnknownSession:
                    ForgetSession();
                    return new DecisionOutcome { Choice = Choice.MakeDefer(), Reason = "unknown session", RetryAfterHello = !ticket.Retried };
                default:
                    Breaker.RecordFailure(nowMs);
                    return new DecisionOutcome { Choice = Choice.MakeDefer(), Reason = failure.ToString().ToLowerInvariant() };
            }

            if (IsExpired(ticket, nowMs))
                return new DecisionOutcome { Choice = Choice.MakeDefer(), Reason = "deadline exceeded", Response = response };

            var choice = ChoiceValidator.Validate(ticket.Request, response, out var reason);
            return new DecisionOutcome
            {
                Choice = choice,
                Execute = !choice.IsDefer && ticket.Mode == Modes.Act,
                Reason = reason,
                Response = response,
            };
        }

        void ForgetSession()
        {
            Breaker.RecordSuccess();
            if (State == SessionState.Active)
            {
                SessionId = null;
                State = SessionState.NeedHello;
            }
        }
    }
}
