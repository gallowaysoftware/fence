using System;
using System.Collections.Generic;

namespace Fence.Sim
{
    // What UnityEngine.Random is rewritten to call. The generator is .NET's seeded System.Random
    // algorithm (Knuth's subtractive generator), reproduced so its state can be saved and restored:
    // the output for a seed is identical to `new System.Random(seed)`, which keeps extraction
    // byte-identical, and `Random.state` round-trips like it does in the player.
    public static class HeadlessRandom
    {
        sealed class Knuth
        {
            const int MBig = int.MaxValue;
            const int MSeed = 161803398;
            public readonly int[] SeedArray = new int[56];
            public int Inext, Inextp;

            public Knuth(int seed)
            {
                int subtraction = seed == int.MinValue ? int.MaxValue : Math.Abs(seed);
                int mj = MSeed - subtraction;
                SeedArray[55] = mj;
                int mk = 1;
                for (int i = 1; i < 55; i++)
                {
                    int ii = 21 * i % 55;
                    SeedArray[ii] = mk;
                    mk = mj - mk;
                    if (mk < 0) mk += MBig;
                    mj = SeedArray[ii];
                }
                for (int k = 1; k < 5; k++)
                    for (int i = 1; i < 56; i++)
                    {
                        SeedArray[i] -= SeedArray[1 + (i + 30) % 55];
                        if (SeedArray[i] < 0) SeedArray[i] += MBig;
                    }
                Inext = 0;
                Inextp = 21;
            }

            Knuth() { }

            public Knuth Clone()
            {
                var k = new Knuth { Inext = Inext, Inextp = Inextp };
                Array.Copy(SeedArray, k.SeedArray, 56);
                return k;
            }

            int InternalSample()
            {
                int locINext = Inext + 1;
                if (locINext >= 56) locINext = 1;
                int locINextp = Inextp + 1;
                if (locINextp >= 56) locINextp = 1;
                int ret = SeedArray[locINext] - SeedArray[locINextp];
                if (ret == MBig) ret--;
                if (ret < 0) ret += MBig;
                SeedArray[locINext] = ret;
                Inext = locINext;
                Inextp = locINextp;
                return ret;
            }

            public double Sample() => InternalSample() * (1.0 / MBig);

            double LargeRangeSample()
            {
                int result = InternalSample();
                if (InternalSample() % 2 == 0) result = -result;
                double d = result;
                d += int.MaxValue - 1;
                d /= 2u * int.MaxValue - 1;
                return d;
            }

            public int Next(int min, int max)
            {
                long range = (long)max - min;
                return range <= int.MaxValue ? (int)(Sample() * range) + min : (int)((long)(LargeRangeSample() * range) + min);
            }
        }

        static Knuth rng = new Knuth(0);

        // Random.state handles: the player's State is four ints, too small for this generator, so a
        // state is a ticket into a bounded table of saved generators.
        const int Tickets = 256;
        static readonly Dictionary<int, Knuth> saved = new Dictionary<int, Knuth>();
        static int nextTicket = 1;

        public static long Draws { get; private set; }

        public static void Seed(int seed)
        {
            rng = new Knuth(seed);
            saved.Clear();
            nextTicket = 1;
            Draws = 0;
        }

        // Unity: max is exclusive, and max <= min returns min.
        public static int RangeInt(int min, int max)
        {
            Draws++;
            return max <= min ? min : rng.Next(min, max);
        }

        public static float RangeFloat(float min, float max)
        {
            Draws++;
            return (float)rng.Sample() * (max - min) + min;
        }

        public static float Value()
        {
            Draws++;
            return (float)rng.Sample();
        }

        public static int SaveState()
        {
            int ticket = nextTicket++;
            saved[ticket] = rng.Clone();
            saved.Remove(ticket - Tickets);
            return ticket;
        }

        public static void RestoreState(int ticket)
        {
            if (saved.TryGetValue(ticket, out var k)) rng = k.Clone();
        }
    }
}
