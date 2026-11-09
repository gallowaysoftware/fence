using System;
using System.Collections.Generic;
using System.Linq;
using Newtonsoft.Json.Linq;

namespace Fence.Barter
{
    public static class LeftCauses
    {
        public const string Dismissed = "dismissed";
        public const string BusinessDone = "business_done";
        public const string WalkedOut = "walked_out";
        public const string Shutter = "shutter";
    }

    // outcome.result for kind "barter" (docs/PROTOCOL.md, POST /v1/outcome).
    public static class Outcomes
    {
        public static JObject RepDelta(IList<FactionRep> before, IList<FactionRep> after)
        {
            var delta = new JObject();
            if (before == null || after == null) return delta;
            foreach (var a in after)
            {
                var b = before.FirstOrDefault(x => x.FactionId == a.FactionId);
                double d = Math.Round(a.Value - (b?.Value ?? 0.0), 3);
                string name = Factions.Name(a.FactionId);
                if (name != null && d != 0.0) delta[name] = d;
            }
            return delta;
        }

        // Null when the call changed nothing (the game's Can* guard refused it).
        public static JObject Bargain(BargainKind kind, int percent, BargainFlags before, BargainFlags after, bool? applied,
            ClientIntent intentBefore, ClientIntent intentAfter, JObject repDelta)
        {
            if (before == null || after == null || before.Offered(kind) || !after.Offered(kind)) return null;
            bool walkedOut = intentBefore != ClientIntent.Dialogue && intentAfter == ClientIntent.Dialogue;
            // Offer Discount and Offer Markup never roll; the others report their private *Applied flag,
            // and `accepted` is omitted when that could not be read.
            bool? accepted = kind == BargainKind.Discount || kind == BargainKind.BuyingMarkup ? true : applied;
            int pct = kind == BargainKind.Blackmail || kind == BargainKind.Threaten ? BarterRules.CoercionPercent : Math.Abs(percent);
            var result = new JObject { ["kind"] = "bargain", ["option"] = OptionIds.Bargain(kind, pct) };
            if (accepted.HasValue) result["accepted"] = accepted.Value;
            result["walked_out"] = walkedOut;
            result["rep_delta"] = repDelta ?? new JObject();
            return result;
        }

        public static JObject Left(string cause) => new JObject { ["kind"] = "left", ["cause"] = cause };

        // result.actor: "mod" when fence executed the move, "human" otherwise.
        public static JObject WithActor(JObject result, string actor)
        {
            if (result != null) result["actor"] = actor;
            return result;
        }

        public static string LeftCause(bool walkedOut, bool shutterClosed, bool playerDismissing)
        {
            if (walkedOut) return LeftCauses.WalkedOut;
            if (shutterClosed) return LeftCauses.Shutter;
            if (playerDismissing) return LeftCauses.Dismissed;
            return LeftCauses.BusinessDone;
        }
    }

    // Purchases and sales arrive one item at a time (wholesale and Sell N loop over items); they are
    // reported as one `trade` per side once the burst is over.
    public sealed class TradeBatch
    {
        public const string Sold = "sold";
        public const string Bought = "bought";

        readonly List<int> items = new List<int>();
        long cashDelta;

        public string Side { get; private set; }
        public IList<FactionRep> RepBefore { get; private set; }
        public string DecisionId { get; private set; }
        public string Actor { get; private set; }
        public bool IsEmpty => items.Count == 0;

        // True when this item starts a new batch, so the caller snapshots reputation first.
        public bool NeedsSnapshot(string side) => IsEmpty || side != Side;

        public void Begin(string side, IList<FactionRep> repBefore, string decisionId = null, string actor = Actors.Human)
        {
            Side = side;
            RepBefore = repBefore;
            DecisionId = decisionId;
            Actor = actor;
            items.Clear();
            cashDelta = 0;
        }

        public void Add(int uid, int amount)
        {
            items.Add(uid);
            cashDelta += Side == Sold ? amount : -amount;
        }

        public JObject Flush(IList<FactionRep> repAfter)
        {
            if (IsEmpty) return null;
            var result = new JObject
            {
                ["kind"] = "trade",
                ["side"] = Side,
                ["items"] = new JArray(items.Cast<object>().ToArray()),
                ["cash_delta"] = cashDelta,
                ["rep_delta"] = Outcomes.RepDelta(RepBefore, repAfter),
                ["actor"] = Actor ?? Actors.Human,
            };
            items.Clear();
            cashDelta = 0;
            RepBefore = null;
            return result;
        }
    }
}
