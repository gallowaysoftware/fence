using System.Collections.Generic;
using System.Linq;
using Fence.Barter;
using Fence.Protocol;
using Newtonsoft.Json.Linq;

namespace Fence.Recording
{
    // What kind of customer, from what the player can see (the tablet's section title / intent).
    public static class CustomerKinds
    {
        public const string Trade = "trade";

        public static string Of(ClientInfo c)
        {
            if (c == null) return "none";
            switch (c.Intent)
            {
                case ClientIntent.Buy:
                case ClientIntent.Sell:
                case ClientIntent.SellAndBuy:
                    return Trade;
                case ClientIntent.Dialogue:
                    return "dialogue";
            }
            switch (c.RawIntent)
            {
                case "BARTER": return "barter";
                case "APPRAISAL_SERVICE": return "appraisal";
                case "INFORMATION_DEALER": return "info_dealer";
                case "GUNSMITH": return "gunsmith";
                case "WHOLESALE": return "wholesale";
                case "INSPECTION": return "inspection";
                case "RENT": return "landlord";
                case "LOAN_SHARK": return "loan_shark";
                default: return string.IsNullOrEmpty(c.RawIntent) ? "other" : c.RawIntent.ToLowerInvariant();
            }
        }

        // Kinds the player handles at the counter; the others (inspection, landlord, story dialogue)
        // are observed on arrival because their counter never becomes ready.
        public static bool UsesCounter(string kind) =>
            kind == Trade || kind == "barter" || kind == "appraisal" || kind == "info_dealer" || kind == "gunsmith" || kind == "wholesale";
    }

    // One customer_observed per customer: when the counter first becomes ready, at arrival for kinds
    // that never get there, and as a fallback when the customer leaves without either.
    public sealed class CustomerObserver
    {
        int seq = -1;
        bool emitted;
        bool usesCounter;

        public bool Pending => seq >= 0 && !emitted;

        // Returns true when the observation is due now (at arrival).
        public bool Arrived(int clientSeq, string kind)
        {
            seq = clientSeq;
            emitted = false;
            usesCounter = CustomerKinds.UsesCounter(kind);
            return Take(!usesCounter);
        }

        public bool Ready(int clientSeq) => clientSeq == seq && Take(true);

        public bool Leaving(int clientSeq) => clientSeq == seq && Take(true);

        bool Take(bool due)
        {
            if (!due || emitted || seq < 0) return false;
            emitted = true;
            return true;
        }
    }

    public static class CustomerObservation
    {
        // `context` is the v1 barter context (or as much of it as applies); `extras` the kind's own
        // visible fields (service cost, gun order reward, inspector).
        public static (JObject data, JObject hidden) Build(int clientSeq, ClientInfo c, string kind, string when, string coverageGap,
            JObject context, IList<DecisionOption> options, JObject extras, JObject hidden)
        {
            var data = new JObject
            {
                ["client_seq"] = clientSeq,
                ["covered"] = coverageGap == null,
                ["reason"] = coverageGap != null ? (JToken)coverageGap : JValue.CreateNull(),
                ["kind"] = kind,
                ["at"] = when,
                ["dismissable"] = c != null && c.Dismissable,
                ["context"] = context ?? MinimalContext(c),
                ["options"] = new JArray((coverageGap == null && options != null ? options : new List<DecisionOption>())
                    .Select(o => (object)new JObject { ["id"] = o.Id, ["label"] = o.Label }).ToArray()),
            };
            if (extras != null && extras.HasValues) data["extras"] = extras;
            return (data, hidden ?? HiddenClient(c));
        }

        public static JObject MinimalContext(ClientInfo c)
        {
            var client = new JObject();
            if (c != null)
            {
                client["name"] = c.Name;
                client["faction"] = Factions.Visible(c);
                client["intent"] = c.Intent == ClientIntent.Other ? (c.RawIntent ?? "other").ToLowerInvariant() : CustomerKinds.Of(c) == CustomerKinds.Trade ? Intent(c.Intent) : "dialogue";
                Fence.Aug.AugDetection.Describe(client, c);
            }
            return new JObject { ["client"] = client };
        }

        static string Intent(ClientIntent i) => i == ClientIntent.Buy ? "buy" : i == ClientIntent.Sell ? "sell" : "sell_and_buy";

        public static JObject HiddenClient(ClientInfo c) => c == null ? new JObject() : new JObject
        {
            ["client"] = BarterContext.HiddenClient(c),
        };
    }
}
