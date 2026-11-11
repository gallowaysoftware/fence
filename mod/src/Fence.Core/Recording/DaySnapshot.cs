using System;
using System.Collections.Generic;
using System.Linq;
using Fence.Barter;
using Newtonsoft.Json.Linq;

namespace Fence.Recording
{
    // Plain copies of the shop's day-level state (docs/re/DAY.md §1), filled by Fence.Mod.GameDay or a
    // test fake. Null = could not be read, omitted.
    public sealed class StoreDay
    {
        public int? Cash, Day, WildFavor, NetWorth, Attractiveness, TrashPenalty, ProjectorBonus, QueueLength, CommissaryTokens, RetailMarkup;
        // PlayerStore.difficultyId: cozy, normal, hard, challenge (shown on the new-game screen).
        public string Difficulty;
        public bool StampIntroduced;
        public bool? BusinessPermit, NewspaperUnlocked;
    }

    public sealed class RentDay
    {
        public int? Rent, Arrears, DaysLeft;
        public bool? LandlordGrace;
    }

    public sealed class PropertyDay
    {
        public bool Owned;
        public bool PayingMortgage;
        public int? MortgageWeekly, MortgageWeeksLeft, BuyoutPrice;
        public bool? BuyoutAffordable;
        // The loan shark's offer, shown only when CanBeOfferedMortgage.
        public bool MortgageOffered;
        public int? OfferDownPayment, OfferWeekly, OfferWeeks;
    }

    public sealed class PerkDay
    {
        public string Id;
        public int Threshold;
        public bool Unlocked;
    }

    public sealed class RepDay
    {
        public string FactionId;
        public double Value;
        public string Status;
        public List<PerkDay> Perks = new List<PerkDay>();
    }

    public sealed class OwnedItem
    {
        public ItemInfo Item;
        // stock, showcase, back_counter, front_counter, hidden, trash, docs, vending_machine,
        // vending_fountain, afterhour, other; or "inside:<uid>" for an item nested in another owned item.
        public string Where;
        // The root container of the outermost item (equals Where unless nested).
        public string Root;
        // ContrabandHelper.IsSmugglerBay: inspections do not search its contents.
        public bool SmugglerBay;
        // The grid inside a container (a bay, bag or machine); null for an item that holds nothing.
        public Capacity Capacity;
    }

    // Grid cells inside a container: every cell the shape allows, and those no item covers.
    public sealed class Capacity
    {
        public int Cells, Free;
    }

    public sealed class Prediction
    {
        public string ClientId;
        public string ClientName;
        public int Chance;
        public string ItemId;
    }

    public sealed class ClienteleDay
    {
        public int? NormalCustomers;
        public List<string> DisplayBuyers;
        // Null when they could not be read without disturbing the game (docs/re/DAY.md).
        public List<Prediction> Predictions;
        // "computed" now, or "game": the game's own last computation (at shutter open or the clientele panel).
        public string PredictionsSource;
        public int? PredictionsDay;
        public int? DiscerningChance, DiscerningCooldown;
    }

    public sealed class CrimeDay
    {
        public string Id;
        public int Amount;
        // crimeAmountTotal: never decays, HIDDEN.
        public int? Total;
    }

    public sealed class SecurityDay
    {
        // The player can open the record (owns intel_sec).
        public bool Visible;
        public List<CrimeDay> Crimes = new List<CrimeDay>();
        public int? Evidence, TraffickingValue, TotalCrime;
        public string EvidenceDisplay, ProjectedSentence, CaseOfficer, RecommendedAction, Operation, Note;
    }

    public sealed class UpgradeDay
    {
        public string Id, Name;
        public int? Cost;
        public int State, Cooldown;
        public bool Unlocked, HasPrerequisite, Hidden;
    }

    public sealed class ServiceDay
    {
        public string Id, Name;
        public int Cost, Cooldown;
        public bool Unlocked, Using, Mandatory;
    }

    public sealed class EventDay
    {
        public string Id, Name, Type;
        public int StartDay, Duration;
        public bool DurationVisible;
        // Future events: days until it starts.
        public int? InDays;
        public List<MarketEvent> Modifiers = new List<MarketEvent>();
    }

    public sealed class AfterhourEventDay
    {
        public string Id, Name;
        public List<string> Effects = new List<string>();
        public int DaysLeft;
    }

    public sealed class AfterhourDay
    {
        public string Id, Name;
        public bool Available, ClosedByEvent;
        public List<AfterhourEventDay> Events = new List<AfterhourEventDay>();
    }

    public sealed class RequestDay
    {
        public string Id, Title, Requester, FactionId, RequiredItem;
        public int Count, CreditReward, RepReward, Duration;
    }

    public sealed class HiddenDay
    {
        public int? DaysToInspection, AugPresence, Luck;
        public bool? InspectionCountdownActive;
    }

    // Every day-level read. Each may throw; the builder then drops only that section.
    public interface IDayGame
    {
        StoreDay Store();
        RentDay Rent();
        PropertyDay Property();
        IList<RepDay> Reputation();
        IList<OwnedItem> Inventory();
        ClienteleDay Clientele();
        SecurityDay Security();
        IList<UpgradeDay> Upgrades();
        IList<ServiceDay> Services();
        // Active today and visible in the newspaper.
        IList<EventDay> ActiveEvents();
        // What the calendar shows on a day (known events and innate markers).
        IList<EventDay> EventsOnDay(int day);
        IList<EventDay> UnknownFutureEvents();
        IList<AfterhourDay> Afterhours();
        IList<RequestDay> Requests();
        IList<string> NightLog();
        // The run's starting perk ids (PlayerStore.startingPerks).
        IList<string> StartingPerks();
        HiddenDay Hidden();
    }

    public static class SnapshotPhases
    {
        public const string Morning = "morning";
        public const string Open = "open";
        public const string Close = "close";
        public const string Sleep = "sleep";
        public const string RunStart = "run_start";
        public const string Load = "load";
    }

    public sealed class SnapshotResult
    {
        public JObject Data;
        public JObject Hidden;
        // Sections that threw (the caller logs once per section).
        public List<(string section, Exception error)> Failed = new List<(string, Exception)>();
    }

    // day_snapshot (docs/re/DAY.md §1, "Proposed day_snapshot.data"). Visible values go to `data`,
    // internal ones to `hidden`, never the other way.
    public static class DaySnapshot
    {
        public static readonly string[] Sections =
        {
            "store", "rent", "property", "reputation", "inventory", "clientele", "security", "upgrades", "services",
            "market_events", "afterhours", "requests", "night_log", "perks", "hidden",
        };

        public const int CalendarDays = 14;

        public static SnapshotResult Build(IDayGame g, string phase, string runId, Func<string, bool> sectionEnabled)
        {
            var r = new SnapshotResult { Data = new JObject { ["phase"] = phase }, Hidden = new JObject() };
            if (!string.IsNullOrEmpty(runId)) r.Data["run_id"] = runId;
            var unavailable = new JArray();
            StoreDay store = null;
            List<OwnedItem> inventory = null;
            IList<RepDay> reps = null;

            void Section(string name, Action fill)
            {
                if (sectionEnabled != null && !sectionEnabled(name))
                {
                    unavailable.Add(name);
                    return;
                }
                try
                {
                    fill();
                }
                catch (Exception e)
                {
                    unavailable.Add(name);
                    r.Failed.Add((name, e));
                }
            }

            Section("store", () =>
            {
                store = g.Store();
                r.Data["store"] = Store(store);
            });
            Section("rent", () => r.Data["rent"] = Rent(g.Rent(), store?.Day));
            Section("property", () => r.Data["property"] = Property(g.Property()));
            Section("reputation", () =>
            {
                reps = g.Reputation();
                r.Data["reputation"] = Reputation(reps);
            });
            Section("inventory", () =>
            {
                inventory = (g.Inventory() ?? new List<OwnedItem>()).Where(i => i?.Item != null).ToList();
                r.Data["inventory"] = new JArray(inventory.Select(i => (object)InventoryItem(i)).ToArray());
                r.Hidden["items"] = new JArray(inventory.Select(i => (object)BarterContext.HiddenItem(i.Item)).ToArray());
            });
            Section("clientele", () => Clientele(g.Clientele(), inventory, r));
            Section("security", () => Security(g.Security(), r));
            Section("upgrades", () => r.Data["upgrades"] = Upgrades(g.Upgrades()));
            Section("services", () => r.Data["services"] = Services(g.Services()));
            Section("market_events", () => Market(g, store?.Day, r));
            Section("afterhours", () => r.Data["afterhours"] = Afterhours(g.Afterhours()));
            Section("requests", () => r.Data["requests"] = Requests(g.Requests()));
            // The start-of-day panel clears the night log right after the morning hook.
            if (phase == SnapshotPhases.Morning)
                Section("night_log", () => r.Data["night_log"] = new JArray((g.NightLog() ?? new List<string>()).Select(FeatureText.Plain).Where(s => s.Length > 0).Cast<object>().ToArray()));
            Section("perks", () => r.Data["perks"] = Perks(g.StartingPerks(), reps ?? g.Reputation()));
            Section("hidden", () => HiddenValues(g.Hidden(), r.Hidden));

            if (unavailable.Count > 0) r.Data["unavailable"] = unavailable;
            return r;
        }

        static JObject InventoryItem(OwnedItem i)
        {
            var o = BarterContext.VisibleItem(i.Item, false, i.Where);
            if (i.Root != null && i.Root != i.Where) o["root"] = i.Root;
            if (i.SmugglerBay) o["smuggler_bay"] = true;
            if (i.Capacity != null) o["capacity"] = new JObject { ["cells"] = i.Capacity.Cells, ["free"] = i.Capacity.Free };
            return o;
        }

        static JObject Store(StoreDay s)
        {
            var o = new JObject();
            Put(o, "cash", s.Cash);
            Put(o, "day", s.Day);
            if (s.Day.HasValue) o["weekday"] = s.Day.Value % 7;
            Put(o, "wild_favor", s.WildFavor);
            Put(o, "net_worth", s.NetWorth);
            if (s.Attractiveness.HasValue)
            {
                var a = new JObject { ["total"] = s.Attractiveness.Value };
                Put(a, "trash_penalty", s.TrashPenalty);
                Put(a, "projector_bonus", s.ProjectorBonus);
                o["attractiveness"] = a;
            }
            Put(o, "queue_length", s.QueueLength);
            if (s.StampIntroduced) Put(o, "commissary_tokens", s.CommissaryTokens);
            Put(o, "retail_markup", s.RetailMarkup);
            if (!string.IsNullOrEmpty(s.Difficulty)) o["difficulty"] = s.Difficulty;
            o["aug_detection"] = Fence.Aug.AugDetection.Current;
            Put(o, "business_permit", s.BusinessPermit);
            Put(o, "newspaper_unlocked", s.NewspaperUnlocked);
            return o;
        }

        // GetRentDayCounter's formula: what the morning reminder prints.
        public static int RentDaysLeftMorning(int day) => (7 - day % 7) % 7;

        static JObject Rent(RentDay r, int? day)
        {
            var o = new JObject();
            Put(o, "rent", r.Rent);
            Put(o, "arrears", r.Arrears);
            if (r.Rent.HasValue) o["due"] = r.Rent.Value + (r.Arrears ?? 0);
            Put(o, "days_left", r.DaysLeft);
            if (day.HasValue) o["days_left_morning"] = RentDaysLeftMorning(day.Value);
            Put(o, "landlord_grace", r.LandlordGrace);
            return o;
        }

        static JObject Property(PropertyDay p)
        {
            var o = new JObject { ["owned"] = p.Owned };
            o["mortgage"] = p.PayingMortgage
                ? new JObject { ["weekly"] = p.MortgageWeekly, ["weeks_left"] = p.MortgageWeeksLeft }
                : (JToken)JValue.CreateNull();
            Put(o, "buyout_price", p.BuyoutPrice);
            Put(o, "buyout_affordable", p.BuyoutAffordable);
            o["mortgage_offer"] = p.MortgageOffered && !p.Owned && !p.PayingMortgage
                ? new JObject { ["down_payment"] = p.OfferDownPayment, ["weekly"] = p.OfferWeekly, ["weeks"] = p.OfferWeeks }
                : (JToken)JValue.CreateNull();
            return o;
        }

        public static JObject Reputation(IList<RepDay> reps)
        {
            var o = new JObject();
            foreach (var r in reps ?? new List<RepDay>())
            {
                string name = Factions.Name(r?.FactionId);
                if (name == null || !Factions.WithRepBar.Contains(name)) continue;
                var (next, nextNegative) = NextPerks(r.Value, r.Perks);
                o[name] = new JObject
                {
                    ["value"] = Math.Round(r.Value, 2),
                    ["status"] = r.Status,
                    ["next_perk"] = next != null ? new JObject { ["id"] = next.Id, ["threshold"] = next.Threshold } : (JToken)JValue.CreateNull(),
                    ["next_negative_perk"] = nextNegative != null ? new JObject { ["id"] = nextNegative.Id, ["threshold"] = nextNegative.Threshold } : (JToken)JValue.CreateNull(),
                    ["unlocked_perks"] = new JArray((r.Perks ?? new List<PerkDay>()).Where(p => p.Unlocked).Select(p => (object)p.Id).ToArray()),
                };
            }
            return o;
        }

        // Starting perks, and every reputation perk unlocked now (any faction): what is in force today.
        public static JObject Perks(IList<string> starting, IList<RepDay> reps)
        {
            var unlocked = (reps ?? new List<RepDay>())
                .Where(r => r != null && Factions.WithRepBar.Contains(Factions.Name(r.FactionId) ?? ""))
                .SelectMany(r => r.Perks ?? new List<PerkDay>())
                .Where(p => p != null && p.Unlocked && !string.IsNullOrEmpty(p.Id))
                .Select(p => p.Id).Distinct().OrderBy(id => id, StringComparer.Ordinal);
            return new JObject
            {
                ["starting"] = new JArray((starting ?? new List<string>()).Where(id => !string.IsNullOrEmpty(id)).Cast<object>().ToArray()),
                ["reputation"] = new JArray(unlocked.Cast<object>().ToArray()),
            };
        }

        // The next perk up (the lowest non-negative threshold above the value) and the next one down
        // (the highest negative threshold below it): what to push for and what to stay clear of.
        public static (PerkDay next, PerkDay nextNegative) NextPerks(double value, IList<PerkDay> perks)
        {
            if (perks == null) return (null, null);
            var up = perks.Where(p => p != null && p.Threshold >= 0 && p.Threshold > value).OrderBy(p => p.Threshold).FirstOrDefault();
            var down = perks.Where(p => p != null && p.Threshold < 0 && p.Threshold < value).OrderByDescending(p => p.Threshold).FirstOrDefault();
            return (up, down);
        }

        static void Clientele(ClienteleDay c, List<OwnedItem> inventory, SnapshotResult r)
        {
            var o = new JObject();
            Put(o, "normal_customers", c.NormalCustomers);
            o["display_buyers"] = new JArray((c.DisplayBuyers ?? new List<string>()).Select(TypeTags.Name).Cast<object>().ToArray());
            if (c.Predictions != null)
            {
                var showcase = (inventory ?? new List<OwnedItem>()).Where(i => i.Where == "showcase").Select(i => i.Item).ToList();
                o["display_predictions"] = new JArray(c.Predictions.Select(p => (object)new JObject
                {
                    ["client"] = p.ClientName,
                    ["chance"] = p.Chance,
                    ["item_id"] = p.ItemId,
                    ["item"] = showcase.FirstOrDefault(i => i.Id == p.ItemId)?.Name,
                }).ToArray());
                o["predictions_source"] = c.PredictionsSource;
                Put(o, "predictions_day", c.PredictionsDay);
                // Client identifiers are internal: the clientele panel shows names only.
                r.Hidden["display_prediction_clients"] = new JArray(c.Predictions.Select(p => (object)p.ClientId).ToArray());
            }
            if (c.DiscerningChance.HasValue || c.DiscerningCooldown.HasValue)
                o["discerning_shopper"] = new JObject { ["chance"] = c.DiscerningChance, ["cooldown"] = c.DiscerningCooldown };
            r.Data["clientele"] = o;
        }

        static void Security(SecurityDay s, SnapshotResult r)
        {
            var full = new JObject
            {
                ["visible"] = s.Visible,
                ["crimes"] = new JArray(s.Crimes.Where(c => c.Amount > 0).Select(c => (object)new JObject { ["id"] = c.Id, ["amount"] = c.Amount }).ToArray()),
            };
            Put(full, "total", s.TotalCrime);
            Put(full, "evidence", s.Evidence);
            Put(full, "evidence_level", s.EvidenceDisplay);
            Put(full, "projected_sentence", s.ProjectedSentence);
            Put(full, "case_officer", s.CaseOfficer);
            Put(full, "recommended_action", s.RecommendedAction);
            Put(full, "operation", s.Operation);
            Put(full, "note", s.Note);
            Put(full, "trafficking_value", s.TraffickingValue);
            if (s.Visible)
            {
                r.Data["security"] = full;
            }
            else
            {
                r.Data["security"] = new JObject { ["visible"] = false };
                r.Hidden["security"] = full;
            }
            var totals = s.Crimes.Where(c => c.Total.HasValue).Select(c => (object)new JObject { ["id"] = c.Id, ["total"] = c.Total.Value }).ToArray();
            if (totals.Length > 0) r.Hidden["crime_totals"] = new JArray(totals);
        }

        static JObject Upgrades(IList<UpgradeDay> ups)
        {
            var list = (ups ?? new List<UpgradeDay>()).Where(u => u != null && !u.Hidden).OrderBy(u => u.Id, StringComparer.Ordinal).ToList();
            return new JObject
            {
                // NetworkUpgrade.state: 0 not offered, 1 for sale, 2 owned (or armed, for a held grace).
                ["owned"] = new JArray(list.Where(u => u.State == 2).Select(u => (object)new JObject { ["id"] = u.Id, ["name"] = u.Name }).ToArray()),
                ["available"] = new JArray(list.Where(u => u.State == 1 && u.HasPrerequisite && u.Cooldown == 0)
                    .Select(u => (object)new JObject { ["id"] = u.Id, ["name"] = u.Name, ["cost"] = u.Cost }).ToArray()),
                ["cooling"] = new JArray(list.Where(u => u.State == 1 && u.HasPrerequisite && u.Cooldown > 0)
                    .Select(u => (object)new JObject { ["id"] = u.Id, ["name"] = u.Name, ["cost"] = u.Cost, ["days"] = u.Cooldown }).ToArray()),
            };
        }

        static JArray Services(IList<ServiceDay> services) =>
            new JArray((services ?? new List<ServiceDay>()).Where(s => s != null).Select(s => (object)new JObject
            {
                ["id"] = s.Id,
                ["name"] = s.Name,
                ["cost"] = s.Cost,
                ["unlocked"] = s.Unlocked,
                ["using"] = s.Using,
                ["mandatory"] = s.Mandatory,
                ["cooldown"] = s.Cooldown,
            }).ToArray());

        // AdvCalendarUIManager.OpenUI: two weeks from the start of the current week (day % 7 == 1).
        public static int CalendarStart(int day)
        {
            int offset = day % 7;
            if (offset == 0) offset = 7;
            return day - offset + 1;
        }

        static void Market(IDayGame g, int? day, SnapshotResult r)
        {
            var o = new JObject { ["active"] = new JArray((g.ActiveEvents() ?? new List<EventDay>()).Select(e => (object)Event(e, true)).ToArray()) };
            if (day.HasValue)
            {
                var calendar = new JArray();
                int start = CalendarStart(day.Value);
                for (int d = start; d < start + CalendarDays; d++)
                {
                    var events = g.EventsOnDay(d);
                    if (events == null || events.Count == 0) continue;
                    calendar.Add(new JObject
                    {
                        ["day"] = d,
                        ["events"] = new JArray(events.Select(e => (object)new JObject { ["id"] = e.Id, ["name"] = e.Name, ["type"] = e.Type }).ToArray()),
                    });
                }
                o["calendar"] = calendar;
            }
            r.Data["market_events"] = o;
            var unknown = g.UnknownFutureEvents();
            if (unknown != null && unknown.Count > 0) r.Hidden["unknown_events"] = new JArray(unknown.Select(e => (object)Event(e, false)).ToArray());
        }

        static JObject Event(EventDay e, bool visible)
        {
            var o = new JObject { ["id"] = e.Id, ["name"] = e.Name, ["type"] = e.Type, ["start_day"] = e.StartDay };
            if (!visible || e.DurationVisible) o["duration"] = e.Duration;
            if (e.InDays.HasValue) o["in_days"] = e.InDays.Value;
            o["modifiers"] = new JArray(e.Modifiers.Select(m => (object)new JObject { ["item_type"] = TypeTags.Name(m.ItemType), ["value_pct"] = m.ValuePct }).ToArray());
            return o;
        }

        static JArray Afterhours(IList<AfterhourDay> locations) =>
            new JArray((locations ?? new List<AfterhourDay>()).Where(l => l != null).Select(l => (object)new JObject
            {
                ["location"] = l.Id,
                ["name"] = l.Name,
                ["available"] = l.Available,
                ["closed_by_event"] = l.ClosedByEvent,
                ["events"] = new JArray(l.Events.Select(e => (object)new JObject
                {
                    ["id"] = e.Id,
                    ["name"] = e.Name,
                    ["effects"] = new JArray(e.Effects.Select(FeatureText.Plain).Cast<object>().ToArray()),
                    ["days_left"] = e.DaysLeft,
                }).ToArray()),
            }).ToArray());

        static JArray Requests(IList<RequestDay> requests) =>
            new JArray((requests ?? new List<RequestDay>()).Where(q => q != null).Select(q => (object)new JObject
            {
                ["id"] = q.Id,
                ["title"] = q.Title,
                ["requester"] = q.Requester,
                ["faction"] = Factions.Name(q.FactionId),
                ["required_item"] = q.RequiredItem,
                ["count"] = q.Count,
                ["credit_reward"] = q.CreditReward,
                ["rep_reward"] = q.RepReward,
                ["days_left"] = q.Duration,
            }).ToArray());

        static void HiddenValues(HiddenDay h, JObject hidden)
        {
            Put(hidden, "days_to_inspection", h.DaysToInspection);
            Put(hidden, "inspection_countdown_active", h.InspectionCountdownActive);
            Put(hidden, "aug_presence", h.AugPresence);
            Put(hidden, "luck", h.Luck);
        }

        static void Put(JObject o, string key, int? v)
        {
            if (v.HasValue) o[key] = v.Value;
        }

        static void Put(JObject o, string key, bool? v)
        {
            if (v.HasValue) o[key] = v.Value;
        }

        static void Put(JObject o, string key, string v)
        {
            if (v != null) o[key] = FeatureText.Plain(v);
        }
    }
}
