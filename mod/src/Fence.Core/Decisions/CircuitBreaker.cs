using System;

namespace Fence.Decisions
{
    public enum BreakerState
    {
        Closed,
        Open,
        HalfOpen,
    }

    // After FailureThreshold consecutive failures the breaker opens for CooldownMs; while open nothing
    // is sent. When the cooldown elapses exactly one probe is let through: success closes it, failure
    // re-opens it for another cooldown.
    public sealed class CircuitBreaker
    {
        public int FailureThreshold { get; }
        public long CooldownMs { get; }
        public BreakerState State { get; private set; } = BreakerState.Closed;
        public int ConsecutiveFailures { get; private set; }
        public long OpenUntilMs { get; private set; }

        public CircuitBreaker(int failureThreshold, long cooldownMs)
        {
            if (failureThreshold < 1) throw new ArgumentOutOfRangeException(nameof(failureThreshold));
            if (cooldownMs < 0) throw new ArgumentOutOfRangeException(nameof(cooldownMs));
            FailureThreshold = failureThreshold;
            CooldownMs = cooldownMs;
        }

        // True while requests must not even be attempted; does not consume the half-open probe.
        public bool IsOpen(long nowMs) => State == BreakerState.Open && nowMs < OpenUntilMs
                                          || State == BreakerState.HalfOpen;

        public bool TryAcquire(long nowMs)
        {
            switch (State)
            {
                case BreakerState.Closed:
                    return true;
                case BreakerState.Open when nowMs >= OpenUntilMs:
                    State = BreakerState.HalfOpen;
                    return true;
                default:
                    return false;
            }
        }

        public void RecordSuccess()
        {
            State = BreakerState.Closed;
            ConsecutiveFailures = 0;
        }

        public void RecordFailure(long nowMs)
        {
            ConsecutiveFailures++;
            if (State == BreakerState.HalfOpen || ConsecutiveFailures >= FailureThreshold)
            {
                State = BreakerState.Open;
                OpenUntilMs = nowMs + CooldownMs;
            }
        }
    }
}
