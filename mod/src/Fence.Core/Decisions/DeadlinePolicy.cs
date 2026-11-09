using System;
using Fence.Protocol;

namespace Fence.Decisions
{
    // The game may be paused while a decision is pending, so the mod never waits longer than
    // MaxMs no matter what a request asks for.
    public sealed class DeadlinePolicy
    {
        public int DefaultMs { get; }
        public int MinMs { get; }
        public int MaxMs { get; }
        public int GraceMs { get; }

        public DeadlinePolicy(int defaultMs = 20000, int minMs = 500, int maxMs = 60000, int graceMs = 250)
        {
            if (minMs <= 0 || maxMs < minMs || defaultMs < minMs || defaultMs > maxMs || graceMs < 0)
                throw new ArgumentException("inconsistent deadline bounds");
            DefaultMs = defaultMs;
            MinMs = minMs;
            MaxMs = maxMs;
            GraceMs = graceMs;
        }

        public int Effective(int requestedMs)
        {
            if (requestedMs <= 0) return DefaultMs;
            if (requestedMs < MinMs) return MinMs;
            return requestedMs > MaxMs ? MaxMs : requestedMs;
        }

        // The bridge is told Effective(); the mod gives up GraceMs later to absorb transport latency.
        public bool IsExpired(long startedMs, long nowMs, int effectiveMs) => nowMs - startedMs >= (long)effectiveMs + GraceMs;
    }

    public static class DecisionDeadlines
    {
        // Advise: the human acts in parallel and a late answer still helps. Act: the mod executes the
        // answer, so a slow one is more likely to be stale.
        public const int AdviseMs = 20000;
        public const int ActMs = 10000;

        public static int ForMode(string mode) => mode == Modes.Act ? ActMs : AdviseMs;
    }
}
