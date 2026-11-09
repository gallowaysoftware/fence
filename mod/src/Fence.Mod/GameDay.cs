using System;
using System.Collections.Generic;
using System.Runtime.CompilerServices;
using Fence.Barter;
using Fence.Recording;
using Newtonsoft.Json.Linq;

namespace Fence.Mod
{
    // IDayGame over the live game: reads only, every member in compat/hooks.json under record.snapshot.*.
    // NoInlining per section, so a member an update removed fails that section only (DaySnapshot
    // catches it). Never calls anything that rolls the game's RNG or caches a roll (docs/re/DAY.md).
    sealed class GameDay : IDayGame
    {
        static readonly string[] RolledSigns = { "sign_food", "sign_material", "sign_medical", "sign_household", "sign_firearm" };

        readonly GameBarter items = new GameBarter();

        // The game's own last GetDisplayCasePrediction (queue generation at shutter open, the clientele panel).
        public List<Prediction> LastGamePredictions;
        public int? LastGamePredictionsDay;
        // Set while we compute predictions ourselves, so the capture patch ignores our call.
        public bool Computing;

        [MethodImpl(MethodImplOptions.NoInlining)]
        public StoreDay Store()
        {
            var ps = PlayerStore.Instance;
            if (ps == null) throw new InvalidOperationException("no PlayerStore");
            var s = new StoreDay
            {
                Cash = ps.playerCash,
                WildFavor = ps.wildFavor,
                NetWorth = ps.GetTotalEstimatedValue(),
                Attractiveness = ps.GetCurrentStoreAttractiveness(),
                TrashPenalty = ps.GetTrashAttPenalty(),
                ProjectorBonus = ps.projectorAttractivenessBonus,
                StampIntroduced = ps.isStampIntroduced,
                CommissaryTokens = ps.commissaryData != null ? ps.commissaryData.redeemedCredit : (int?)null,
                RetailMarkup = ps.retailMarkup,
                BusinessPermit = ps.hasBusinessPermit,
                NewspaperUnlocked = ps.isNewspaperUnlocked,
            };
            var station = StoreStation.Instance;
            if (station != null) s.Day = station.dayCounter;
            if (ps.storeClientManager != null) s.QueueLength = ps.GetClientInQueueCount();
            return s;
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        public RentDay Rent()
        {
            var ps = PlayerStore.Instance;
            return new RentDay
            {
                Rent = ps.rentValue,
                Arrears = ps.temporaryRent,
                DaysLeft = ps.dayUntilRent,
                LandlordGrace = NetworkUpgrade.IsLandlordGraceUnlocked(),
            };
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        public PropertyDay Property()
        {
            var ps = PlayerStore.Instance;
            var p = new PropertyDay
            {
                Owned = ps.IsPropertyPaid,
                PayingMortgage = ps.isPayingMortgage,
                MortgageWeekly = ps.mortgageWeeklyPayment,
                MortgageWeeksLeft = ps.mortgageWeekLeft,
                BuyoutPrice = ps.GetStoreBuyPrice(false),
                BuyoutAffordable = ps.CanBuyOutStore(),
                MortgageOffered = ps.CanBeOfferedMortgage(),
            };
            if (p.MortgageOffered)
            {
                p.OfferDownPayment = ps.GetMortgageDownPayment();
                p.OfferWeekly = ps.GetMortgageWeeklyPaymentPreview();
                p.OfferWeeks = PlayerStore.GetMortgageWeeks();
            }
            return p;
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        public IList<RepDay> Reputation()
        {
            var list = new List<RepDay>();
            var ps = PlayerStore.Instance;
            if (ps.storeReputations == null) return list;
            foreach (var r in ps.storeReputations)
            {
                if (r == null) continue;
                var rep = new RepDay { FactionId = r.factionId, Value = r.GetReputationExact(), Status = StoreReputation.GetReputationStatus(r.GetReputation()) };
                if (r.perks != null)
                    foreach (var p in r.perks)
                        if (p != null) rep.Perks.Add(new PerkDay { Id = p.perkId, Threshold = p.perkThreshold, Unlocked = p.isUnlocked });
                list.Add(rep);
            }
            return list;
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        public IList<OwnedItem> Inventory()
        {
            var e = EmporiumEntry.Instance;
            var roots = new List<KeyValuePair<GameInventory, string>>
            {
                Root(e.invElement, "stock"),
                Root(e.showcaseElement, "showcase"),
                Root(e.backInvinvElement, "back_counter"),
                Root(e.backInvinvElementCounter, "back_counter"),
                Root(e.frontInvinvElement, "front_counter"),
                Root(e.hiddenElement, "hidden"),
                Root(e.trashInvElement, "trash"),
                Root(e.docInvElement, "docs"),
                Root(e.vendingMachineElement, "vending_machine"),
                Root(e.vendingFountainElement, "vending_fountain"),
                Root(e.afterhourInventory, "afterhour"),
            };
            var list = new List<OwnedItem>();
            foreach (var item in e.GetAllOwnedItems(false, true))
            {
                if (item == null) continue;
                var (root, inside) = Place(item, roots);
                list.Add(new OwnedItem
                {
                    Item = items.Item(item, true, false, false, false),
                    Where = inside.HasValue ? "inside:" + inside.Value : root,
                    Root = root,
                    SmugglerBay = ContrabandHelper.IsSmugglerBay(item),
                });
            }
            return list;
        }

        static KeyValuePair<GameInventory, string> Root(GameInventory inv, string where) => new KeyValuePair<GameInventory, string>(inv, where);

        // The root container of the outermost item, and the item this one is nested in.
        (string where, int? inside) Place(GameItem item, List<KeyValuePair<GameInventory, string>> roots)
        {
            int? inside = null;
            var current = item;
            for (int depth = 0; depth < 16 && current != null; depth++)
            {
                var inv = current.parentInventory;
                if (inv == null) return ("other", inside);
                foreach (var r in roots)
                    if (r.Key != null && ReferenceEquals(r.Key, inv)) return (r.Value, inside);
                var parent = inv.GetParentItem();
                if (parent == null) return ("other", inside);
                if (!inside.HasValue) inside = items.Uid(parent);
                current = parent;
            }
            return ("other", inside);
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        public ClienteleDay Clientele()
        {
            var ps = PlayerStore.Instance;
            var c = new ClienteleDay
            {
                DisplayBuyers = ShowcaseHelper.GetDisplayCaseType(),
                DiscerningChance = ShowcaseHelper.GetDiscerningShopperChance(),
                DiscerningCooldown = ShowcaseHelper.GetDiscerningShopperCooldownOpenings(),
            };
            // GetNormalCustomerCount rolls Roll(100) (always true): the result is fixed, the draw is not.
            if (ps.storeClientManager != null) c.NormalCustomers = WithRandomRestored(() => ps.storeClientManager.GetNormalCustomerCount());
            if (PredictionIsPure())
            {
                Computing = true;
                try
                {
                    c.Predictions = WithRandomRestored(() => Predictions(ShowcaseHelper.GetDisplayCasePrediction()));
                }
                finally
                {
                    Computing = false;
                }
                c.PredictionsSource = "computed";
                c.PredictionsDay = StoreStation.Instance != null ? StoreStation.Instance.dayCounter : (int?)null;
            }
            else if (LastGamePredictions != null)
            {
                c.Predictions = LastGamePredictions;
                c.PredictionsSource = "game";
                c.PredictionsDay = LastGamePredictionsDay;
            }
            return c;
        }

        // GetDisplayCasePrediction rolls (and caches until the next queue) a supplier for each sign in the
        // window that has none yet; with every such pick cached it only reads.
        [MethodImpl(MethodImplOptions.NoInlining)]
        static bool PredictionIsPure()
        {
            var picks = ModReflect.GetStatic(typeof(ShowcaseHelper), "pendingSupplierPicks") as Dictionary<string, string>;
            if (picks == null) return false;
            var showcase = EmporiumEntry.Instance.showcaseElement;
            foreach (var item in EmporiumEntry.Instance.GetAllOwnedItems(false, false))
            {
                if (item == null || !ReferenceEquals(item.parentInventory, showcase)) continue;
                if (Array.IndexOf(RolledSigns, item.identifier) >= 0 && !picks.ContainsKey(item.identifier)) return false;
            }
            return true;
        }

        // StoreClientListDict builds a client once to learn its name (then caches it); the build may roll.
        [MethodImpl(MethodImplOptions.NoInlining)]
        public static List<Prediction> Predictions(List<(string, int, string)> raw)
        {
            var list = new List<Prediction>();
            if (raw == null) return list;
            foreach (var (client, chance, item) in raw)
                list.Add(new Prediction { ClientId = client, Chance = chance, ItemId = item, ClientName = WithRandomRestored(() => StoreClientListDict.GetClientDisplayName(client)) });
            return list;
        }

        static T WithRandomRestored<T>(Func<T> f)
        {
            var state = UnityEngine.Random.state;
            try
            {
                return f();
            }
            finally
            {
                UnityEngine.Random.state = state;
            }
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        public SecurityDay Security()
        {
            var sd = PlayerStore.Instance.secData;
            var s = new SecurityDay { Visible = OwnsRecord() };
            if (sd == null) return s;
            if (sd.crimeTypes != null && sd.crimeAmount != null)
                for (int i = 0; i < sd.crimeTypes.Count && i < sd.crimeAmount.Count; i++)
                    s.Crimes.Add(new CrimeDay
                    {
                        Id = sd.crimeTypes[i],
                        Amount = sd.crimeAmount[i],
                        Total = sd.crimeAmountTotal != null && i < sd.crimeAmountTotal.Count ? sd.crimeAmountTotal[i] : (int?)null,
                    });
            int total = sd.GetTotalCrimeValue();
            s.TotalCrime = total;
            s.Evidence = sd.evidenceLevel;
            s.EvidenceDisplay = sd.GetEvidenceLevelDisplay();
            s.ProjectedSentence = sd.GetProjectedSentence();
            s.CaseOfficer = sd.GetCaseOfficer(total);
            s.RecommendedAction = sd.GetAction();
            s.Operation = sd.GetOperation();
            s.Note = sd.note;
            s.TraffickingValue = sd.GetTotalTraffickingCrimes();
            return s;
        }

        // RecordUIManager renders the record when the player activates the intel_sec document (dossier at
        // start with skip-intro, else handed over by the mentor once there is a crime).
        static bool OwnsRecord()
        {
            foreach (var item in EmporiumEntry.Instance.GetAllOwnedItems(false, true))
                if (item != null && item.identifier == "intel_sec") return true;
            return false;
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        public IList<UpgradeDay> Upgrades()
        {
            var list = new List<UpgradeDay>();
            var all = PlayerStore.Instance.networkUpgrade;
            if (all == null) return list;
            foreach (var kv in all)
            {
                var u = kv.Value;
                if (u == null) continue;
                list.Add(new UpgradeDay
                {
                    Id = u.id,
                    Name = u.GetLocalizedTitle(),
                    Cost = u.GetCost(),
                    State = u.state,
                    Unlocked = u.IsUnlocked(),
                    HasPrerequisite = u.IsHavePrerequisite(),
                    Hidden = u.hidden || NetworkUpgrade.IsHiddenForRun(u.id),
                });
            }
            return list;
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        public IList<ServiceDay> Services()
        {
            var list = new List<ServiceDay>();
            var all = PlayerStore.Instance.storeService;
            if (all == null) return list;
            foreach (var s in all)
                if (s != null)
                    list.Add(new ServiceDay { Id = s.id, Name = s.displayName, Cost = s.cost, Unlocked = s.unlocked, Using = s.isUsing, Mandatory = s.isMandatory, Cooldown = s.currentCooldown });
            return list;
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        public IList<EventDay> ActiveEvents()
        {
            var list = new List<EventDay>();
            foreach (var ev in StoreStation.Instance.storeEventManager.GetActiveEvents())
                if (ev != null && !ev.isHiddenFromNewsPaper) list.Add(Event(ev, null));
            return list;
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        public IList<EventDay> EventsOnDay(int day)
        {
            var list = new List<EventDay>();
            foreach (var ev in StoreStation.Instance.storeEventManager.GetActiveEventOnDay(day, true, true))
                if (ev != null) list.Add(Event(ev, null));
            return list;
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        public IList<EventDay> UnknownFutureEvents()
        {
            var list = new List<EventDay>();
            var m = StoreStation.Instance.storeEventManager;
            if (m.futurEvents == null) return list;
            for (int i = 0; i < m.futurEvents.Count; i++)
            {
                var ev = m.futurEvents[i];
                if (ev == null || ev.IsKnown) continue;
                list.Add(Event(ev, m.futurEventsDay != null && i < m.futurEventsDay.Count ? m.futurEventsDay[i] : (int?)null));
            }
            return list;
        }

        static EventDay Event(StoreEvent ev, int? inDays)
        {
            var e = new EventDay
            {
                Id = ev.identifier,
                Name = ev.GetDisplayName(),
                Type = ev.eventType.ToString().ToLowerInvariant(),
                StartDay = ev.startDay,
                Duration = ev.totalDuration,
                DurationVisible = ev.isDurationVisible,
                InDays = inDays,
            };
            if (ev.negociationDatas != null)
                foreach (var d in ev.negociationDatas)
                    if (d != null) e.Modifiers.Add(new MarketEvent { ItemType = d.itemType, ValuePct = d.value });
            return e;
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        public IList<AfterhourDay> Afterhours()
        {
            var list = new List<AfterhourDay>();
            foreach (var loc in AfterhourLocation.GetDiscovered())
            {
                if (loc == null) continue;
                var a = new AfterhourDay { Id = loc.Id, Name = loc.GetTitle(), Available = loc.IsAvailable(), ClosedByEvent = loc.IsClosedByEvent() };
                foreach (var ev in AfterhourEvents.GetActive(loc.Id))
                    if (ev != null) a.Events.Add(new AfterhourEventDay { Id = ev.id, Name = ev.GetTitle(), Effects = ev.GetEffectTexts(), DaysLeft = AfterhourEvents.GetDaysLeft(ev.id) });
                list.Add(a);
            }
            return list;
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        public IList<RequestDay> Requests()
        {
            var list = new List<RequestDay>();
            var all = PlayerStore.Instance.storeRequests;
            if (all == null) return list;
            foreach (var r in all)
                if (r != null) list.Add(Request(r));
            return list;
        }

        public static RequestDay Request(StoreRequest r) => new RequestDay
        {
            Id = r.id,
            Title = r.title,
            Requester = r.requesterName,
            FactionId = r.factionId,
            RequiredItem = r.requiredItem,
            Count = r.requiredItemCount,
            CreditReward = r.creditReward,
            RepReward = r.reputationReward,
            Duration = r.duration,
        };

        [MethodImpl(MethodImplOptions.NoInlining)]
        public IList<string> NightLog() => PlayerStore.Instance.nightLogs ?? new List<string>();

        [MethodImpl(MethodImplOptions.NoInlining)]
        public HiddenDay Hidden()
        {
            var ps = PlayerStore.Instance;
            var h = new HiddenDay { AugPresence = ps.augPresence };
            if (ps.storeClientManager != null)
            {
                h.DaysToInspection = ps.storeClientManager.dayUntilInspection;
                h.InspectionCountdownActive = ps.storeClientManager.IsInspectionCountdownActive();
            }
            if (ps.healthData != null) h.Luck = ps.healthData.luck;
            return h;
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static string RunId() => PlayerStore.Instance != null ? PlayerStore.Instance.runID : null;

        // run_start: the run's fixed setup (docs/re/DAY.md §5), all on the new-game and status screens.
        [MethodImpl(MethodImplOptions.NoInlining)]
        public static JObject Run()
        {
            var ps = PlayerStore.Instance;
            var perks = new JArray();
            if (ps.startingPerks != null)
                foreach (var p in ps.startingPerks)
                    if (p != null) perks.Add(new JObject { ["id"] = p.id, ["name"] = p.GetLocalizedDisplayName() });
            var o = new JObject
            {
                ["run_id"] = ps.runID,
                ["run_number"] = ps.runNumber,
                ["start_type"] = ps.startType.ToString().ToLowerInvariant(),
                ["scenario"] = ps.startScenarioId,
                ["difficulty"] = ps.difficultyId,
                ["endless"] = ps.endlessMode,
                ["skip_intro"] = ps.skipIntro,
                ["retail_markup"] = ps.retailMarkup,
                ["perks"] = perks,
            };
            var rules = Difficulty.Rules;
            if (rules != null)
                o["rules"] = new JObject
                {
                    ["retail_markup"] = rules.retailMarkup,
                    ["rent_increase_multiplier"] = Math.Round(rules.rentIncreaseMultiplier, 4),
                    ["property_base_price"] = rules.propertyBasePrice,
                    ["property_increase_per_day"] = rules.propertyPriceIncreasePerDay,
                    ["property_increase_cap"] = rules.propertyPriceIncreaseCap,
                    ["vendor_permit_fee"] = rules.vendorPermitFee,
                    ["mortgage_weeks"] = rules.mortgageWeeks,
                    ["mortgage_interest_rate"] = rules.mortgageInterestRate,
                    ["mortgage_down_payment_share"] = Math.Round(rules.mortgageDownPaymentShare, 4),
                    ["upgrade_price_multiplier"] = Math.Round(rules.upgradePriceMultiplier, 4),
                    ["aug_game_over_at_full_presence"] = rules.augGameOverAtFullPresence,
                };
            return o;
        }
    }
}
