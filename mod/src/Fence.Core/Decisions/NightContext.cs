using System.Collections.Generic;
using Fence.Protocol;
using Newtonsoft.Json.Linq;

namespace Fence.Decisions
{
    // The production engines' wants for the night's jackson_wants table (Production.Wants in the gym
    // and the live mod). The interface keeps the builder away from the gym's internal Production
    // class; the live mod supplies the real counts.
    public interface IProductionWants
    {
        int Wants(string id);
    }

    // The player-visible half of the live game at the end of the day, for the night's decision (the
    // outing: Dr. Jackson's, the dumping ground, staying in, or calling Wild's Network). The mod may
    // only answer with one of the options it enumerated.
    public sealed class NightView
    {
        public int Day;
        public int Cash;
        public int RentDue;
        public int RentDaysLeft;
        public int Wound;
        // The afterhours locations the player can go to tonight (id, name).
        public List<(string Id, string Name)> Locations = new List<(string Id, string Name)>();
        // The services the player would have on tonight (id, cost, using).
        public List<(string Id, int Cost, bool Using)> Services = new List<(string Id, int Cost, bool Using)>();
        // What Dr. Jackson's stocks that the engines want tonight (identifier, count).
        public List<(string Id, int Count)> Wants = new List<(string Id, int Count)>();
        // The items on the back counter (the afterhours bag's contents when it is wired).
        public int BagItems;
        // The plan's notes (the directives' name and thesis, when the bridge has one).
        public string PlanNotes;
    }

    public static class NightContext
    {
        public const string Kind = "night";

        public const string GoJackson = "go_jackson";
        public const string Scavenge = "scavenge";
        public const string StayIn = "stay_in";
        public const string CallWild = "call_wild";

        // The well-known ids Dr. Jackson's stocks that the engines buy from him: the furnace and its
        // modules (fuel and blast), the hydroponic bay, the water purifier, the infinite wine yeast,
        // the current converter.
        static readonly string[] JacksonStock =
        {
            "furnace", "furnace_module_junk", "furnace_module_blast",
            "hydroponic", "water_purifier", "wine_yeast_infinite", "current_converter",
        };

        // Builds the request for the bridge's night decision. The deadline comes from the session's
        // mode (observe/advise: 20 s, act: 10 s); on defer, timeout or error the mod plays the role's
        // own script for the night.
        public static DecideRequest Request(NightView view, string decisionId, string mode, IProductionWants wants)
        {
            return new DecideRequest
            {
                DecisionId = decisionId,
                Kind = Kind,
                GameDay = view.Day,
                DeadlineMs = DecisionDeadlines.ForMode(mode),
                Context = Context(view, wants),
                Options = Options(view),
            };
        }

        public static List<DecisionOption> Options(NightView v)
        {
            var list = new List<DecisionOption>
            {
                new DecisionOption { Id = StayIn, Label = "stay in for the night" },
            };
            bool jackson = false, dump = false;
            foreach (var (id, _) in v.Locations)
            {
                if (id == "dr_jackson") jackson = true;
                else if (id == "dumping_ground") dump = true;
            }
            if (jackson) list.Add(new DecisionOption { Id = GoJackson, Label = "go to Dr. Jackson's" });
            if (dump) list.Add(new DecisionOption { Id = Scavenge, Label = "scavenge the dumping ground" });
            return list;
        }

        public static JObject Context(NightView v, IProductionWants wants)
        {
            var locs = new JArray();
            foreach (var (id, name) in v.Locations)
                locs.Add(new JObject { ["id"] = id, ["name"] = name });
            var services = new JArray();
            foreach (var (id, cost, usingSvc) in v.Services)
                services.Add(new JObject { ["id"] = id, ["cost"] = cost, ["using"] = usingSvc });
            var jacksonWants = new JObject();
            if (wants != null)
                foreach (var id in JacksonStock)
                {
                    int count;
                    try
                    {
                        count = wants.Wants(id);
                    }
                    catch
                    {
                        // The directives may be unset in a headless context: leave the id out.
                        continue;
                    }
                    if (count > 0) jacksonWants[id] = count;
                }
            return new JObject
            {
                ["day"] = v.Day,
                ["cash"] = v.Cash,
                ["rent_due"] = v.RentDue,
                ["rent_days_left"] = v.RentDaysLeft,
                ["wound"] = v.Wound,
                ["locations"] = locs,
                ["services"] = services,
                ["jackson_wants"] = jacksonWants,
                ["bag_items"] = v.BagItems,
                ["plan_notes"] = v.PlanNotes,
            };
        }
    }
}
