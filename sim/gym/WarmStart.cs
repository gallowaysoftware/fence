using System;
using System.Collections.Generic;
using System.Globalization;
using System.Linq;
using Newtonsoft.Json.Linq;

namespace Fence.Gym
{
    // Starting perks as the new-game screen allows them (NewGamePerksUI.CanStart/LockedBy): known ids,
    // at most 3 points, at most 4 + Σ maxSlot perks, no incompatible pair. Start type and difficulty do
    // not restrict them. The screen blocks perks on a player's first run; the gym is always a first run
    // (saves are stubbed), so that gate is not applied.
    internal static class StartingPerks
    {
        public const int MaxPoints = 3;
        public const int BaseSlots = 4;

        public static List<StartingPerk> Parse(string spec)
        {
            var ids = (spec ?? "").Split('+', StringSplitOptions.RemoveEmptyEntries | StringSplitOptions.TrimEntries);
            if (ids.Length == 0) return new List<StartingPerk>();
            if (StartingPerkList.Perks == null) StartingPerkList.InitStartingPerk();
            var all = StartingPerkList.Perks.ToDictionary(p => p.id, StringComparer.Ordinal);
            var picked = new List<StartingPerk>();
            foreach (var id in ids)
            {
                if (!all.TryGetValue(id, out var p)) throw new ArgumentException("unknown perk " + id + " (known: " + string.Join(" ", all.Keys.OrderBy(x => x, StringComparer.Ordinal)) + ")");
                if (picked.Any(x => x.id == id)) throw new ArgumentException("perk " + id + " given twice");
                var clash = picked.FirstOrDefault(x => (x.incompatiblePerks?.Contains(id) ?? false) || (p.incompatiblePerks?.Contains(x.id) ?? false));
                if (clash != null) throw new ArgumentException("perk " + id + " is incompatible with " + clash.id);
                picked.Add(p);
            }
            int points = picked.Sum(p => p.cost), slots = BaseSlots + picked.Sum(p => p.maxSlot);
            if (points > MaxPoints) throw new ArgumentException($"perks cost {points} points, the game allows {MaxPoints}");
            if (picked.Count > slots) throw new ArgumentException($"{picked.Count} perks, the game allows {slots}");
            return picked;
        }
    }

    // A late-game situation laid over a new game, through the game's own state: Wild's upgrades by
    // NetworkUpgrade.Unlock (each upgrade's unlock action runs, e.g. +150 base attractiveness per
    // renovation), reputation by StoreReputation.SetReputation (which re-evaluates the rep perks), and
    // the fields the game's own day, rent and permit logic read. Not carried over: stock, machines,
    // crime record, story progress.
    internal sealed class WarmStart
    {
        public int? Day, Cash, Rent, RentDays, Attractiveness, Favor;
        public bool? Permit;
        public readonly List<(string faction, double value)> Reps = new List<(string, double)>();
        public readonly List<string> Upgrades = new List<string>();
        public string Perks;

        static readonly Dictionary<string, string> Factions = new Dictionary<string, string>(StringComparer.Ordinal)
        {
            ["lower_levels"] = "FACTION_LOWER_LEVEL", ["upper_levels"] = "FACTION_UPPER_LEVEL", ["security"] = "FACTION_SECURITY",
            ["black_market"] = "FACTION_BLACK_MARKET", ["revolution"] = "FACTION_REVOLUTION",
        };

        // "day=108,cash=6180,rent=1347,rent_days=4,rep=lower_levels:88.3+upper_levels:36.5,attract=700,permit=1,favor=0,upgrades=A+B,perks=a+b"
        public static WarmStart Parse(string spec)
        {
            if (string.IsNullOrWhiteSpace(spec)) return null;
            var w = new WarmStart();
            var inv = CultureInfo.InvariantCulture;
            foreach (var part in spec.Split(',', StringSplitOptions.RemoveEmptyEntries | StringSplitOptions.TrimEntries))
            {
                int eq = part.IndexOf('=');
                if (eq <= 0) throw new ArgumentException("-warm: " + part + " needs key=value");
                string k = part.Substring(0, eq), v = part.Substring(eq + 1);
                switch (k)
                {
                    case "day": w.Day = int.Parse(v, inv); break;
                    case "cash": w.Cash = int.Parse(v, inv); break;
                    case "rent": w.Rent = int.Parse(v, inv); break;
                    case "rent_days": w.RentDays = int.Parse(v, inv); break;
                    case "attract": w.Attractiveness = int.Parse(v, inv); break;
                    case "favor": w.Favor = int.Parse(v, inv); break;
                    case "permit": w.Permit = v == "1" || v == "true" || v == "yes"; break;
                    case "upgrades": w.Upgrades.AddRange(v.Split('+', StringSplitOptions.RemoveEmptyEntries | StringSplitOptions.TrimEntries)); break;
                    case "perks": w.Perks = v; break;
                    case "rep":
                        foreach (var r in v.Split('+', StringSplitOptions.RemoveEmptyEntries | StringSplitOptions.TrimEntries))
                        {
                            var fv = r.Split(':');
                            if (fv.Length != 2) throw new ArgumentException("-warm rep: " + r + " needs faction:value");
                            string f = Factions.TryGetValue(fv[0], out var id) ? id : fv[0];
                            w.Reps.Add((f, double.Parse(fv[1], inv)));
                        }
                        break;
                    default:
                        throw new ArgumentException("-warm: unknown key " + k + " (day cash rent rent_days rep attract permit favor upgrades perks)");
                }
            }
            return w;
        }

        public void Apply()
        {
            var ps = PlayerStore.Instance;
            if (Day.HasValue) StoreStation.Instance.dayCounter = Day.Value;
            if (Upgrades.Count > 0)
            {
                NetworkUpgrade.AddMissingAfterLoad();
                foreach (var id in Upgrades)
                {
                    if (!ps.networkUpgrade.TryGetValue(id, out var u)) throw new ArgumentException("-warm: unknown upgrade " + id);
                    u.Unlock();
                }
            }
            foreach (var (f, v) in Reps)
            {
                var r = StoreReputation.GetStoreReputation(f) ?? throw new ArgumentException("-warm: unknown faction " + f);
                r.SetReputation(v);
            }
            if (Attractiveness.HasValue) ps.baseStoreAttractiveness = Attractiveness.Value;
            if (Cash.HasValue) ps.playerCash = Cash.Value;
            if (Favor.HasValue) ps.wildFavor = Favor.Value;
            if (Rent.HasValue) ps.rentValue = Rent.Value;
            if (RentDays.HasValue) ps.dayUntilRent = RentDays.Value;
            if (Permit.HasValue) ps.hasBusinessPermit = Permit.Value;
        }

        // What the run actually starts from, read back from the game.
        public static JObject Read()
        {
            var ps = PlayerStore.Instance;
            var rep = new JObject();
            foreach (var r in ps.storeReputations) if (r != null) rep[r.factionId] = Math.Round(r.GetReputationExact(), 2);
            return new JObject
            {
                ["day"] = StoreStation.Instance.dayCounter, ["cash"] = ps.playerCash, ["rent"] = ps.GetRentValue(), ["rent_days"] = ps.dayUntilRent,
                ["attract"] = ps.baseStoreAttractiveness, ["permit"] = ps.hasBusinessPermit, ["favor"] = ps.wildFavor, ["rep"] = rep,
                ["upgrades"] = new JArray(ps.networkUpgrade.Values.Where(u => u.state == 2).Select(u => u.id).OrderBy(x => x, StringComparer.Ordinal)),
                ["perks"] = new JArray((ps.startingPerks ?? new List<StartingPerk>()).Select(p => p.id)),
            };
        }
    }
}
