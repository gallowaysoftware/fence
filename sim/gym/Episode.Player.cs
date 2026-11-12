using System;
using System.Collections.Generic;
using System.Linq;
using Fence.Aug;
using Fence.Barter;
using Fence.Mod;
using Fence.Protocol;
using Fence.Recording;
using Fence.Sim;
using Newtonsoft.Json.Linq;

namespace Fence.Gym
{
    // The executor's half of the episode (`-human player`): the morning plan, the day's accounting, the
    // gun, and the permit official's donations. Inert for every other human role.
    internal sealed partial class Episode
    {
        bool IsPlayer => Human is PlayerRole;
        public readonly PlayerLedger Ledger = new PlayerLedger();
        // The directives every day starts from (-directives FILE, else the knobs), under the planner's.
        public Directives BaseDirectives;
        // Supplies the planner's directives for a day (step mode, a bridge's summary), or null.
        public Func<Episode, Directives> Planner;
        // Step mode's file writers: after each day, and once at the end of the run.
        public Action<Episode, JObject> DayEnded;
        public Action<Episode> RunEnded;
        public string StepNote;

        void PlayerMorning()
        {
            if (!IsPlayer) return;
            var ps = PlayerStore.Instance;
            Ledger.BeginDay(this);
            DemandTrace.Open(Options.Seed);
            Directives plan = null;
            try
            {
                plan = Planner?.Invoke(this);
            }
            catch (ArgumentException e)
            {
                GameError("directives", e);
            }
            Directives.Current = Directives.Merge(BaseDirectives ?? new Directives(), plan);
            // Without the table the board would silently go unjoined and every appraisal result would be wrong.
            if (Appraisal.Guides == null && Directives.Current.Raw.Value<string>("appraisal") == "join")
                throw new InvalidOperationException("appraisal: join needs the guides' value tables in local/appraisal_guides.json (or FENCE_APPRAISAL_GUIDES); see docs/re/APPRAISAL.md");
            SupplyProbe.Morning(Day);
            PlayerDay.Current = ExecutorMorning.Plan(this, Ledger.Days);
            ExecutorMorning.Chores(this);
        }

        public void ProductionError(Exception e) => GameError("production", e);

        void PlayerTrade(string side, int amount)
        {
            var p = PlayerDay.Current;
            if (!IsPlayer || p == null) return;
            if (side == TradeBatch.Sold) { p.Sold += amount; p.SoldItems++; }
            else { p.Spent += amount; p.Bought++; }
            Ledger.CustomerTrade(side == TradeBatch.Sold, amount);
        }

        void PlayerEndOfDay()
        {
            if (!IsPlayer) return;
            var r = Ledger.EndDay(this);
            if (r != null) DayEnded?.Invoke(this, r);
        }

        void PlayerRunEnd()
        {
            if (!IsPlayer) return;
            var r = Ledger.RunEnd(this);
            if (r != null) DayEnded?.Invoke(this, r);
            RunEnded?.Invoke(this);
        }

        // ---- the gun (docs/re/AUGS.md) -------------------------------------------------------

        object augAsked;
        string augAskedSignature;

        // Before anything else at the counter: an aug (perfect detection) or a suspected aug (tells) with the gun
        // available gets an aug_check decision; "shoot" is executed. True when the customer is gone. Perfect
        // detection asks once per customer, as before M3; tells asks again whenever what is visible changes.
        bool PlayerCounterHooks()
        {
            if (!IsPlayer) return false;
            var c = GameBarter.CurrentClient();
            bool perfect = AugDetection.IsPerfect(AugDetection.Current);
            if (c == null || (perfect && ReferenceEquals(augAsked, c))) return false;
            AugSituation s = null;
            AugQuestion q = null;
            Guard("aug read", () => q = AugCheck.Build(s = AugSituation(), AugDetection.Current));
            if (!perfect) AugEval.See(s);
            if (q == null) return false;
            if (!perfect && ReferenceEquals(augAsked, c) && q.Signature == augAskedSignature) return false;
            augAsked = c;
            augAskedSignature = q.Signature;
            string id = "a-sim-s" + Options.Seed + "-" + ClientSeq + (perfect ? "" : "-" + q.Signature.GetHashCode().ToString("x8"));
            var request = AugCheck.Request(q, id, Options.DeadlineMs);
            var choice = Mod.Decide(request, out _, out _);
            lastDecision = id;
            lastAdvised = choice.IsDefer ? Choice.Defer : choice.Option;
            // A deferring bridge (observe mode): the executor's own aug rule decides.
            string pick = choice.IsDefer ? AugRule.Pick(request, out _) : choice.Option;
            if (pick != AugOptionIds.Shoot) return false;
            AugEval.Shot();
            return Shoot(id);
        }

        // ---- what the player sees of an aug that the headless game does not draw ----

        // StoreClientMono.PlayIdleForCurrentClient on arrival: a disguised aug's idle is fixed at creation, an
        // undisguised one rolls 25 % still, then 5 % twitch; humans bob. Headless there is no StoreClientMono, so
        // the gym stands in for the renderer, on its own stream so the game's random sequence is untouched.
        System.Random idleRng;
        string clientIdle;
        // The dialogue history's last line and length before this customer, to tell their lines from earlier ones.
        string linesAnchor;
        int linesAnchorCount;
        public readonly AugEvaluation AugEval = new AugEvaluation();

        void AugArrive(StoreClient c)
        {
            if (!IsPlayer) return;
            AugEval.Close();
            idleRng ??= new System.Random(unchecked(Options.Seed * 7919 + 2591));
            clientIdle = c == null ? null : IdleOf(c);
            var h = PlayerStore.Instance.dialogHistory;
            linesAnchorCount = h?.Count ?? 0;
            linesAnchor = linesAnchorCount > 0 ? h.Last() : null;
            augAskedSignature = null;
        }

        string IdleOf(StoreClient c)
        {
            switch (c.augIdleTell)
            {
                case StoreClient.AugIdleTell.Freeze: return AugIdle.Still;
                case StoreClient.AugIdleTell.Seizure: return AugIdle.Twitch;
                case StoreClient.AugIdleTell.Normal: return AugIdle.Normal;
            }
            if (!c.isAug) return AugIdle.Normal;
            if (idleRng.Next(100) < 25) return AugIdle.Still;
            return idleRng.Next(100) < 5 ? AugIdle.Twitch : AugIdle.Normal;
        }

        // This customer's lines so far (PlayerStore.dialogHistory, capped at 100 by the game).
        List<string> ClientLines()
        {
            var all = PlayerStore.Instance.dialogHistory?.ToList() ?? new List<string>();
            if (all.Count < 100) return all.Skip(Math.Min(linesAnchorCount, all.Count)).ToList();
            int i = linesAnchor == null ? -1 : all.LastIndexOf(linesAnchor);
            return all.Skip(i + 1).ToList();
        }

        AugSituation AugSituation()
        {
            var ps = PlayerStore.Instance;
            var c = GameBarter.CurrentClient();
            var s = new AugSituation
            {
                ClientSeq = ClientSeq,
                Client = Mod.Game.Client(),
                Ui = Mod.Game.Ui(),
                Flags = Mod.Game.Bargain() ?? new BargainFlags(),
                Store = Mod.Game.Store(),
                AugsActive = ps.isAugIntroduced && !ps.isAugEnded,
                ShootingUnlocked = ps.isShootingUnlocked,
                ShootingMode = StoreUIManager.Instance.isShootingMode,
                BeingArrested = ps.isClientBeingArrested,
                HandlingShootout = ps.isHandlingShootout,
                Presence = ps.augPresence,
                Difficulty = ps.difficultyId,
            };
            if (c != null)
            {
                s.Arrestable = c.arrestable;
                s.ForceShootable = c.forceShootable;
                s.NonShootable = c.nonShootable;
                s.ShotCountsAsCapture = c.shotCountsAsCapture;
            }
            if (!s.AugsActive) return s;
            if (s.Client?.Aug != null)
            {
                s.Client.Aug.Idle ??= clientIdle;
                if (string.IsNullOrEmpty(s.Client.Aug.Sprite) && c != null && augPortraits.Contains(c)) s.Client.Aug.Sprite = HeadlessAugPortrait;
            }
            if (!AugDetection.IsPerfect(AugDetection.Current))
            {
                GameBarter.EnsureAugLines();
                if (Options.AugLines) s.Lines = ClientLines();
            }
            s.IntroDay = PlayerStore.GetAugIntroducedDay();
            s.AugMenace = StartingPerk.IsAugMenace();
            var rules = Difficulty.Rules;
            if (rules != null)
            {
                s.PresenceRise = rules.augLeftAlivePresenceRise;
                s.KillPresenceDrop = rules.augKillPresenceDrop;
                s.GameOverAtFull = rules.augGameOverAtFullPresence;
            }
            s.ScannerOwned = EmporiumEntry.Instance.GetAllOwnedItems(false, false).Any(i => i != null && i.identifier == "aug_scanner");
            s.MarkupOdds = Mod.Game.Odds(BargainKind.Markup, AugCheck.ProbePercent);
            s.DiscountOdds = Mod.Game.Odds(BargainKind.BuyingDiscount, AugCheck.ProbePercent);
            return s;
        }

        // StoreUIManager.Shoot and the routines after it, without the animations: the shot, Security's
        // arrival (GenericClientMono's walk-in is a tween headless) and its verdict dialogue, which the
        // customer loop then plays through (good kill, capture, or manslaughter).
        bool Shoot(string decisionId)
        {
            var ui = StoreUIManager.Instance;
            var ps = PlayerStore.Instance;
            var dlg = DialogUIManager.Instance;
            var target = ps.currentClientInstance;
            string name = target?.GetClientBlueprint()?.displayName;
            int cash0 = Cash, presence0 = ps.augPresence;
            var rep0 = Mod.Game.Reputations();
            var crime0 = ps.secData?.GetTotalCrimeValue() ?? 0;
            Mod.Human(HumanActions.Unholster, Advice(), null, null, "player:aug");
            augResult = null;
            Guard("shoot", () =>
            {
                dlg.ClearPausedConversation();
                dlg.InterruptDialog();
                ui.isShot = true;
                ui.shotClientInstance = target;
                ps.RemoveNotOwnedItemFromTable();
            });
            Mod.Human(HumanActions.Shoot, Advice(), null, null, "player:aug");
            Guard("shootout", () =>
            {
                dlg.InterruptDialog();
                ps.isHandlingShootout = true;
                ps.ClientFlagUp();
                ps.ClientFlagUp();
            });
            // StoreUIManager.OnGenericArrived("SEC") (the presentation manager is stubbed headless).
            bool started = false;
            Guard("security arrives", () =>
            {
                dlg.isSecMonologuePlaying = true;
                started = dlg.StartDialogue(StoreClientDialogList.SecShootoutDialog(AugHelper.GetShootResult()), forcedStart: true);
            });
            Scheduler.Settle();
            // Play Security's lines through; the last one's end action pays the kill or books the crime.
            for (int i = 0; started && i < 40 && !Over(); i++)
            {
                var cur = dlg.GetCurrentDialog();
                if (cur == null) break;
                if (cur.choices != null && cur.choices.Count > 0) Guard("choice", () => dlg.SelectChoice(0));
                else Guard("advance", () => dlg.ContinueConversation());
                Scheduler.Pump(1.0);
                Scheduler.Settle();
            }
            // StoreUIManager.OnArrestDialoguePlayed / StartLeaveSec: the body is carried out (still "shot",
            // so it is not an escape), then the shootout state clears.
            Guard("leave sec", () =>
            {
                if (ReferenceEquals(ps.currentClientInstance, target)) ps.DismissCurrentClient(false, false);
            });
            ui.isShot = false;
            ui.shotClientInstance = null;
            ps.isHandlingShootout = false;
            dlg.isSecMonologuePlaying = false;
            Scheduler.Settle();
            string ev = augResult ?? (ps.augPresence < presence0 || Cash > cash0 ? AugEvents.GoodKill : (ps.secData?.GetTotalCrimeValue() ?? 0) > crime0 ? AugEvents.WrongfulKill : AugEvents.WantedCapture);
            JObject extra = ev == AugEvents.WrongfulKill ? new JObject { ["crime"] = new JObject { ["id"] = "MANSLAUGHTER", ["amount"] = AugCheck.Manslaughter(Day) } } : null;
            var data = AugEvents.Data(ev, ClientSeq, name, extra);
            data["cash_delta"] = Cash - cash0;
            data["rep_delta"] = Outcomes.RepDelta(rep0, Mod.Game.Reputations());
            Mod.Emit(RecordKinds.Aug, data, new JObject { ["is_aug"] = ev == AugEvents.GoodKill, ["presence_before"] = presence0, ["presence_after"] = ps.augPresence });
            Mod.Outcome(decisionId, Outcomes.WithActor(AugEvents.Outcome(data), Actors.Human));
            Ledger.Aug(ev, data);
            return !Present();
        }

        string augResult;

        // Hooks on AugHelper (sim/headless Playable.Hooked): the verdicts and the escapes.
        void RegisterAugHooks()
        {
            Hooks.On("AugHelper::OnGoodKill", (self, a) => augResult = AugEvents.GoodKill);
            Hooks.On("AugHelper::CleanupKill", (self, a) => augResult = AugEvents.WrongfulKill);
            Hooks.On("AugHelper::OnAugLeftAlive", (self, a) => AugLeftAlive());
            Hooks.On("AugHelper::SetRandomAugFaction", (self, a) => portraitFor = a[0]);
            Hooks.On("AugHelper::MakeDisguisedAug", (self, a) => portraitFor = a[0]);
            Hooks.On("AugHelper::GetRandomAugSprite", (self, a) =>
            {
                if (portraitFor != null) augPortraits.Add(portraitFor);
            });
        }

        // Customers the game gave an aug portrait (AugHelper.GetRandomAugSprite); headless the sprite name stays empty.
        object portraitFor;
        readonly HashSet<object> augPortraits = new HashSet<object>(ReferenceEqualityComparer.Instance);
        const string HeadlessAugPortrait = "augHeadless";

        void AugLeftAlive()
        {
            Ledger.Aug(AugEvents.LeftAlive);
            if (!IsPlayer) return;
            var data = AugEvents.Data(AugEvents.LeftAlive, ClientSeq, GameBarter.CurrentClient()?.displayName,
                new JObject { ["cause"] = walkedOut ? "walked_out" : playerDismissing ? "dismissed" : "served" });
            Mod.Emit(RecordKinds.Aug, data, new JObject { ["is_aug"] = true, ["presence_after"] = PlayerStore.Instance.augPresence });
            if (augAsked != null && ReferenceEquals(augAsked, GameBarter.CurrentClient()) && lastDecision != null && lastDecision.StartsWith("a-"))
                Mod.Outcome(lastDecision, Outcomes.WithActor(AugEvents.Outcome(data), Actors.Human));
        }

        // ---- Dr. Jackson (after-hours shop) ----------------------------------------------------

        // DrJacksonLocation.Arrive lays his stock on the front counter; the player moves what he wants to the
        // back counter and buys it as from any seller.
        void ShopAtJackson()
        {
            var e = EmporiumEntry.Instance;
            var offered = e.frontInvinvElement.childItems.Where(i => i != null && !GeneralHelper.IsItemOwned(i)).ToList();
            if (offered.Count == 0)
            {
                // DrJacksonLocation.Arrive's logic half (its stage objects do not exist headless, and its
                // exception is swallowed by MapUIManager.Travel): the location, the wide counter, his stock.
                PlayerStore.Instance.playerLocation = PlayerStore.PlayerLocation.INVENTOR;
                Guard("jackson counter", () => e.ExpandFrontCounterGrid());
                Guard("jackson stock", () =>
                {
                    if (AfterhourEvents.IsActive("special_stock")) StoreClientList.PlaceInventorSpecialStock();
                    else StoreClientList.PlaceInventorInventory();
                });
                Scheduler.Settle();
                offered = e.frontInvinvElement.childItems.Where(i => i != null && !GeneralHelper.IsItemOwned(i)).ToList();
            }
            Ledger.Jackson(offered.Select(i => i.identifier + ":" + PlayerRole.Est(i)).ToList());
            // DrJacksonLocation.Arrive ends in MapUIManager.ActivateClientOutsideStore: Dr. Jackson is the counter's
            // client for the visit, without which OnBuyAllClick buys nothing. Its logic half only (no sprites headless).
            var ps = PlayerStore.Instance;
            bool asClient = ps.currentClientInstance == null;
            if (asClient)
            {
                var inventor = StoreClientList.CreateInventor();
                inventor.spriteName ??= "";
                inventor.possibleSprites ??= new List<string>();
                ps.currentClientInstance = StoreClientInstance.CreateClientInstance(inventor);
                ps.isClientArrived = true;
            }
            var moved = PlayerCounter.JacksonStage(offered, Cash, i => Move(e.backInvinvElement, i));
            Scheduler.Settle();
            Ledger.JacksonVisit();
            if (moved.Count > 0)
            {
                int before = Cash;
                var prices = moved.Select(i => (int)i.GetNegociatedValue()).ToList();
                Guard("jackson buy", () => NegociationUIManager.Instance.OnBuyAllClick());
                Scheduler.Settle();
                if (before != Cash)
                    for (int k = 0; k < moved.Count; k++) Ledger.JacksonBought(true, moved[k].identifier, prices[k]);
                Defaults.Take("player:jackson_buy", moved.Count + " items, " + (before - Cash) + " cash");
                // PlayerStore.MoveDealItemToInventory puts goods bought outside on the ground (afterhourInventory);
                // DrJacksonLocation.Leave carries them home, a step its headless throw skips.
                // What fits neither stays on his back counter, which Leave carries home.
                foreach (var i in In(e.afterhourInventory).Concat(In(e.backInvinvElement)).ToList())
                    if (!Move(e.invElement, i) && !Move(e.showcaseElement, i)) Move(e.backInvinvElement, i);
            }
            // MapUIManager.DeactivateClientOutsideStore.
            if (asClient)
            {
                ps.currentClientInstance = null;
                NegociationUIManager.Instance.currentNegociatedItem = null;
                ps.isClientArrived = false;
            }
        }

        // ---- the permit official's donations (StoreClientList.CreateBusinessPermit) ------------

        // From day 45 the official accepts goods marked as donations until the week's bribe is covered,
        // which spares the week's OPERATING_WITHOUT_LICENSE. He turns buyer on his last line; the player
        // stages goods worth the bribe and "sells" them (the Donate button) instead of answering.
        // `inspect: "injectors"` (gym): with the magnifier owned, a seller's Immunivax injectors are inspected on the
        // counter and one showing a fault is confronted on that fault (InspectionUIManager.OnInjectorConfront's
        // dialogue): exposed at its true value less 15 %, it is then honest stock. Only what the inspection panel
        // shows is read: the indicator and body sprites, the printed company, model and serial (docs/re/INSPECT.md).
        int ConfrontInjectors(IList<GameItem> theirs)
        {
            if (Directives.Current.Raw.Value<string>("inspect") != "injectors" || NegociationUIManager.IsTradeLocked()) return 0;
            if (!EmporiumEntry.Instance.GetAllOwnedItems(false, true).Any(i => i != null && i.identifier == "magnifier")) return 0;
            int n = 0;
            foreach (var i in theirs.Where(i => i.IsTag("INS_INJECTOR_TAG") && !i.IsTag("ALREADY_CONFRONTED_TAG")).OrderBy(i => i.uniqueId).ToList())
            {
                var f = i.FindItemFeatureByCategory("CATEGORY_GENUINE_INJECTOR");
                if (f == null || f.GetCurrentItemCondition()?.identifier != "genuineInjector") continue;
                string issue = InjectorFault(i);
                if (issue == null) continue;
                Guard("confront", () => DialogUIManager.Instance.StartDialogue(InsInjectorHelper.CreateConfrontDialogue(issue, i)));
                Scheduler.Settle();
                Defaults.Take("player:confront_injector", issue);
                n++;
            }
            return n;
        }

        // The reference injector the guide shows: a freshly made one is genuine by construction.
        static (string company, string model)? reference;

        static string InjectorFault(GameItem i)
        {
            int indicator = i.GetTagReadonly("INJECTOR_INDICATOR").GetInt(), body = i.GetTagReadonly("INJECTOR_BODY").GetInt();
            if (indicator == 1 || indicator == 3) return "ISSUE_INDICATOR";
            if (body >= 1 && body <= 3) return "ISSUE_BODY";
            if (body == -1) return "ISSUE_COLOR";
            if (reference == null)
            {
                var g = DirectoryMaster.Item("large_purple_injector");
                reference = (g.GetTagReadonly("INJECTOR_COMPANY").GetString(), g.GetTagReadonly("INJECTOR_MODEL").GetString());
            }
            if (i.GetTagReadonly("INJECTOR_COMPANY").GetString() != reference.Value.company || i.GetTagReadonly("INJECTOR_MODEL").GetString() != reference.Value.model) return "ISSUE_LABEL";
            string serial = i.GetTagReadonly("INJECTOR_SERIAL").GetString() ?? "";
            if (serial.Length != 7 || !serial.All(char.IsDigit)) return "ISSUE_SERIAL";
            int prefix = int.Parse(serial.Substring(0, 3));
            if (prefix < 125 || prefix > 455 || InsInjectorHelper.RECALLED_PREFIX.Contains(prefix)) return "ISSUE_SERIAL";
            return null;
        }

        int bribeTries;

        int PullExposable(StoreClient c)
        {
            var e = EmporiumEntry.Instance;
            int n = 0;
            foreach (var i in PlayerCounter.Exposable(c))
                if (Move(e.invElement, i)) { n++; Mod.Human(HumanActions.Unstage, null, Mod.Game.Uid(i), new JObject { ["owned"] = true }, "unstage_exposable"); }
            if (n > 0) Scheduler.Settle();
            return n;
        }

        bool TryBribe()
        {
            if (!IsPlayer) return false;
            var c = GameBarter.CurrentClient();
            var ps = PlayerStore.Instance;
            // Two rounds: the first can fall a few credits short.
            if (bribeTries >= 2 || !PlayerCounter.BribeDue(c, Cash)) return false;
            bribeTries++;
            var e = EmporiumEntry.Instance;
            var picks = PlayerCounter.BribePicks(c, Cash, Defaults, out int need, out long sum);
            if (picks == null) return false;
            int moved = 0;
            foreach (var i in picks)
                if (Move(e.backInvinvElement, i))
                {
                    moved++;
                    Mod.Human(HumanActions.Stage, Advice(), Mod.Game.Uid(i), new JObject { ["owned"] = true, ["donation"] = true }, "player:bribe");
                }
            Scheduler.Settle();
            moved -= PullExposable(c);
            if (moved == 0) return false;
            int before = ps.storeClientData.businessPermitBribeAmount;
            Guard("donate", () =>
            {
                NegociationUIManager.Instance.OnSellAllClick();
                if (!NegociationUIManager.Instance.IsClientHaveEnoughToBuy()) NegociationUIManager.Instance.OnConfirmPanelYes();
            });
            Scheduler.Settle();
            Defaults.Take("player:bribe", need + " owed, " + sum + " staged, " + ps.storeClientData.businessPermitBribeAmount + " left");
            Ledger.Bribes += before - Math.Max(0, ps.storeClientData.businessPermitBribeAmount);
            return true;
        }
    }

    // Per-day and per-run accounting the step mode's result files and the summary report.
    // The visible-tell detector against the hidden truth, one verdict per customer while augs are active (gym
    // evaluation only: nothing here reaches a decision). A customer counts as flagged when any zero-false-positive
    // tell showed during the visit; the weak tells (an outfit that does not match the bubble, the shopping
    // pattern) are counted for comparison and never shoot.
    internal sealed class AugEvaluation
    {
        sealed class Visit
        {
            public object Key;
            public bool Aug, Disguised, Flagged, GunAvailable, Shot;
            public int Tier, Day;
            public string Name, Sprite, Idle;
            public readonly SortedSet<string> Tells = new SortedSet<string>(StringComparer.Ordinal);
        }

        Visit v;
        int tp, fp, fn, tn, augsShootable, flaggedShootable;
        readonly SortedDictionary<string, int[]> byTell = new SortedDictionary<string, int[]>(StringComparer.Ordinal);
        readonly SortedDictionary<string, int[]> missesBy = new SortedDictionary<string, int[]>(StringComparer.Ordinal);
        readonly JArray misses = new JArray(), falseAlarms = new JArray();

        static readonly Dictionary<string, string> CoreFamilies = new Dictionary<string, string>
        {
            ["Lower"] = "lower_levels", ["Upper"] = "upper_levels", ["BM"] = "black_market", ["Rev"] = "revolution",
        };

        public void See(AugSituation s)
        {
            var c = s?.Client;
            if (c == null || !s.AugsActive || c.IsSecurity) return;
            if (v == null || !ReferenceEquals(v.Key, c.Key))
            {
                Close();
                v = new Visit
                {
                    Key = c.Key, Aug = c.IsAug, Disguised = c.Aug?.Disguised ?? false, Tier = c.Aug?.Tier ?? 0, Day = s.Store?.Day ?? 0,
                    Name = c.Name, Sprite = c.Aug?.Sprite, Idle = c.Aug?.Idle,
                };
            }
            AugCheck.SeeLines(s);
            var a = c.Aug;
            if (a != null)
            {
                if (a.Idle == AugIdle.Still) v.Tells.Add("still");
                if (a.Idle == AugIdle.Twitch) v.Tells.Add("twitch");
                if (AugTells.IsAugPortrait(a.Sprite)) v.Tells.Add("aug_portrait");
                if (a.AugOnlyLines) v.Tells.Add("aug_lines");
                if (AugTells.NameBubbleMismatch(c)) v.Tells.Add("name_bubble");
                if (OutfitMismatch(c)) v.Tells.Add("weak:outfit_mismatch");
            }
            if (BarterRules.IsBuying(c.Intent) && c.InterestTags.Any(t => t == "MEDICAL" || t == "MATERIAL")) v.Tells.Add("weak:wants_medical_or_material");
            v.Flagged |= AugTells.Definitive(c);
            v.GunAvailable |= AugCheck.GunAvailable(s);
        }

        public void Shot()
        {
            if (v != null) v.Shot = true;
        }

        // A core-faction portrait under a bubble that implies another faction.
        static bool OutfitMismatch(ClientInfo c)
        {
            string sprite = c.Aug?.Sprite;
            if (string.IsNullOrEmpty(sprite) || AugTells.IsAugPortrait(sprite)) return false;
            string f = System.Text.RegularExpressions.Regex.Replace(sprite, "[0-9]+$", "");
            f = f.StartsWith("female", StringComparison.Ordinal) ? f.Substring(6) : f.StartsWith("male", StringComparison.Ordinal) ? f.Substring(4) : f;
            if (!CoreFamilies.TryGetValue(f, out var family)) return false;
            string implied = Bubbles.ImpliedFaction(Bubbles.Of(c.BubbleColor, c.FactionId));
            return implied != null && implied != family;
        }

        public void Close()
        {
            if (v == null) return;
            var x = v;
            v = null;
            if (x.Aug && x.Flagged) tp++;
            else if (x.Aug) fn++;
            else if (x.Flagged) fp++;
            else tn++;
            if (x.Aug && x.GunAvailable)
            {
                augsShootable++;
                if (x.Flagged) flaggedShootable++;
            }
            foreach (var t in x.Tells)
            {
                if (!byTell.TryGetValue(t, out var n)) byTell[t] = n = new int[2];
                n[x.Aug ? 0 : 1]++;
            }
            if (x.Aug && !x.Flagged)
            {
                string kind = (x.Disguised ? "disguised" : "undisguised") + "_t" + x.Tier;
                if (!missesBy.TryGetValue(kind, out var n)) missesBy[kind] = n = new int[1];
                n[0]++;
            }
            var row = new JObject
            {
                ["day"] = x.Day, ["name"] = x.Name, ["disguised"] = x.Disguised, ["tier"] = x.Tier, ["sprite"] = x.Sprite, ["idle"] = x.Idle,
                ["tells"] = new JArray(x.Tells.Cast<object>().ToArray()), ["gun_available"] = x.GunAvailable, ["shot"] = x.Shot,
            };
            if (x.Aug && !x.Flagged && misses.Count < 100) misses.Add(row);
            if (!x.Aug && x.Flagged && falseAlarms.Count < 100) falseAlarms.Add(row);
        }

        public JObject Json()
        {
            Close();
            var tells = new JObject();
            foreach (var kv in byTell) tells[kv.Key] = new JObject { ["augs"] = kv.Value[0], ["humans"] = kv.Value[1] };
            var by = new JObject();
            foreach (var kv in missesBy) by[kv.Key] = kv.Value[0];
            return new JObject
            {
                ["detector"] = AugRule.Visible,
                ["true_positive"] = tp, ["false_positive"] = fp, ["false_negative"] = fn, ["true_negative"] = tn,
                ["augs_shootable"] = augsShootable, ["augs_shootable_flagged"] = flaggedShootable,
                ["tells"] = tells, ["misses_by_kind"] = by, ["misses"] = misses, ["false_alarms"] = falseAlarms,
            };
        }
    }

    internal sealed class PlayerLedger
    {
        public int Bribes, GoodKills, WrongfulKills, LeftAlive;
        public List<JObject> Days = new List<JObject>();
        JObject today, customer;
        JArray customers = new JArray();

        // One line per customer for result.json: who, what the counter chose, what changed hands.
        public void BeginCustomer(int seq, JObject who)
        {
            customer = who ?? new JObject();
            customer["seq"] = seq;
            customer["sold"] = 0;
            customer["bought"] = 0;
        }

        public void CustomerTrade(bool sold, int amount)
        {
            if (customer == null) return;
            string k = sold ? "sold" : "bought";
            customer[k] = (int)customer[k] + amount;
        }

        public void Flow(string key, int amount)
        {
            if (today != null) money[key] = (money.Value<int?>(key) ?? 0) + amount;
        }

        // Sales and purchases by line of business (cash_flow): wine and water sold, engine inputs bought.
        public void ItemTrade(bool sold, GameItem item, int amount)
        {
            if (today == null || item == null) return;
            string k = sold
                ? (item.identifier == "wine_bottle" ? "wine_sales" : item.GetGameItemType().Contains("WATER") ? "water_sales" : "trade_sales")
                : (Production.IsInput(item.identifier) ? "input_costs" : "trade_purchases");
            money[k] = (money.Value<int?>(k) ?? 0) + amount;
            Furnace.Trade(sold, item, amount);
            Modules.Trade(sold, item, amount);
            DemandTrace.Trade(sold, item, amount);
            Suppliers.Trade(sold, item, amount);
            Rats.Trade(sold, item, amount);
            Fish.Trade(sold, item, amount);
            Crops.Trade(sold, item, amount);
            Lab.Trade(sold, item, amount);
            SupplyProbe.Trade(sold, item, amount);
            Printer.Trade(sold, item, amount);
            Arms.Trade(sold, item, amount);
            if (sold) Production.Sold(item, amount, GameBarter.CurrentClient());
            if (!sold && GameBarter.CurrentClient()?.identifier == "inventor") JacksonBought(false, item.identifier, amount);
        }

        // What was bought from Dr. Jackson, at his after-hours shop or from his visits to the store: per item id,
        // count, cash and the first day.
        readonly SortedDictionary<string, int[]> jacksonShop = new SortedDictionary<string, int[]>(StringComparer.Ordinal);
        readonly SortedDictionary<string, int[]> jacksonStore = new SortedDictionary<string, int[]>(StringComparer.Ordinal);
        int jacksonVisits;

        public void JacksonVisit() => jacksonVisits++;

        public void JacksonBought(bool shop, string id, int amount)
        {
            var d = shop ? jacksonShop : jacksonStore;
            if (!d.TryGetValue(id, out var v)) d[id] = v = new[] { 0, 0, today?.Value<int?>("day") ?? 0 };
            v[0]++;
            v[1] += amount;
            if (today == null) return;
            var list = today["jackson_bought"] as JArray ?? new JArray();
            list.Add(new JObject { ["id"] = id, ["cost"] = amount, ["at"] = shop ? "shop" : "store" });
            today["jackson_bought"] = list;
        }

        static JObject JacksonById(SortedDictionary<string, int[]> d)
        {
            var o = new JObject();
            foreach (var kv in d) o[kv.Key] = new JObject { ["n"] = kv.Value[0], ["cash"] = kv.Value[1], ["first_day"] = kv.Value[2] };
            return o;
        }

        public void Wild(string id, int cost, int cashSpent, string via)
        {
            if (today == null) return;
            var list = today["wild_purchases"] as JArray ?? new JArray();
            list.Add(new JObject { ["upgrade"] = id, ["cost"] = cost, ["cash_spent"] = cashSpent, ["favor_spent"] = cost - cashSpent, ["via"] = via });
            today["wild_purchases"] = list;
        }

        public void Jackson(List<string> offered)
        {
            if (today != null) today["jackson_offer"] = new JArray(offered);
        }

        public void CustomerWhy(string why)
        {
            DemandTrace.Why(why);
            if (customer != null && why != null) customer["why"] = why.Length > 160 ? why.Substring(0, 160) : why;
        }

        public void EndCustomer(string advised, bool walkedOut)
        {
            DemandTrace.Flush();
            if (customer == null) return;
            if (advised != null) customer["choice"] = advised;
            if (walkedOut) customer["walked_out"] = true;
            customers.Add(customer);
            customer = null;
        }
        int cash0, nw0;
        Dictionary<string, double> rep0;

        static Dictionary<string, double> Reps() => PlayerStore.Instance.storeReputations.Where(r => r != null).ToDictionary(r => r.factionId, r => Math.Round(r.GetReputation() * 1.0, 1));

        public void BeginDay(Episode ep)
        {
            cash0 = ep.Cash;
            nw0 = PlayerStore.Instance.GetTotalEstimatedValue();
            rep0 = Reps();
            today = new JObject { ["day"] = ep.Day, ["cash_start"] = cash0, ["net_worth_start"] = nw0 };
        }

        // Where the cash went today, from the game's RunStats funnels (the live mod records the same).
        JObject money = new JObject();

        public void Money(string method, object[] a)
        {
            if (today == null) return;
            int Arg(int i) => a != null && a.Length > i && a[i] is int v ? v : 0;
            string k = method switch
            {
                "OnServicePaid" => "services",
                "OnRentPaid" => "rent",
                "OnMortgagePayment" => "mortgage",
                "OnPermitBought" => "permit",
                "OnPermitBribe" => "permit_bribe_goods",
                "OnWildUpgradeBought" => "wild_upgrades",
                "OnBurglary" => "burglary_goods",
                "OnInspectorBribe" => "inspector_bribe",
                "OnGeorgioLoan" => "georgio_loan",
                "OnGeorgioPayback" => "georgio_payback",
                "OnVendingSale" => "vending_sales",
                "OnFountainSale" => "fountain_sales",
                _ => null,
            };
            if (k == null) return;
            int amount = method == "OnServicePaid" ? Arg(1) : Arg(0);
            money[k] = (money.Value<int?>(k) ?? 0) + amount;
        }

        public void Aug(string ev, JObject detail = null)
        {
            if (today != null && detail != null)
            {
                var list = today["aug_events"] as JArray ?? new JArray();
                list.Add(new JObject { ["event"] = ev, ["cash_delta"] = detail["cash_delta"], ["rep_delta"] = detail["rep_delta"] });
                today["aug_events"] = list;
            }
            if (ev == AugEvents.GoodKill) GoodKills++;
            else if (ev == AugEvents.WrongfulKill) WrongfulKills++;
            else if (ev == AugEvents.LeftAlive) LeftAlive++;
            if (today != null) today["augs"] = new JObject { ["good_kills"] = GoodKills, ["wrongful_kills"] = WrongfulKills, ["left_alive"] = LeftAlive };
        }

        public JObject EndDay(Episode ep)
        {
            if (today == null) return null;
            var p = PlayerDay.Current;
            var ps = PlayerStore.Instance;
            today["cash_end"] = ep.Cash;
            today["cash_delta"] = ep.Cash - cash0;
            today["net_worth_end"] = ps.GetTotalEstimatedValue();
            today["net_worth_delta"] = ps.GetTotalEstimatedValue() - nw0;
            if (p != null)
            {
                today["sold_value"] = p.Sold;
                today["sold_items"] = p.SoldItems;
                today["spent"] = p.Spent;
                today["bought_items"] = p.Bought;
            }
            var rep = Reps();
            var delta = new JObject();
            foreach (var kv in rep)
                if (rep0 != null && rep0.TryGetValue(kv.Key, out var b) && Math.Abs(kv.Value - b) >= 0.05) delta[kv.Key] = Math.Round(kv.Value - b, 1);
            today["rep_delta"] = delta;
            today["rent"] = new JObject { ["due"] = ps.GetRentValue(), ["days_left"] = ps.dayUntilRent, ["grace_owned"] = NetworkUpgrade.IsLandlordGraceUnlocked() };
            today["permit"] = ps.hasBusinessPermit;
            today["permit_bribe_owed"] = ps.storeClientData.businessPermitBribeAmount;
            today["crime"] = ps.secData?.GetTotalCrimeValue() ?? 0;
            today["evidence"] = ps.secData?.evidenceLevel ?? 0;
            today["production"] = Production.Status();
            if (Furnace.Status() is JObject furnace) today["furnace"] = furnace;
            if (Rats.Status() is JObject rats) today["rats"] = rats;
            if (Arms.Status() is JObject arms) today["arms"] = arms;
            if (Fish.Status() is JObject fish) today["fish"] = fish;
            if (Lab.Status() is JObject lab) today["lab"] = lab;
            if (Conceal.Status() is JObject bay) today["smuggler_bay"] = bay;
            if (Vending.Status() is JObject vend) today["vending"] = vend;
            if (Fountain.Status() is JObject fount) today["fountain"] = fount;
            if (Appraisal.Status() is JObject appr) today["appraisal"] = appr;
            if (Labels.Status() is JObject labels) today["labels"] = labels;
            var crimes = new JObject();
            var sd = ps.secData;
            for (int i = 0; i < (sd?.crimeTypes?.Count ?? 0); i++)
                if (sd.crimeAmount[i] > 0) crimes[sd.crimeTypes[i]] = sd.crimeAmount[i];
            today["crimes"] = crimes;
            today["customers"] = customers;
            today["cash_flow"] = money;
            money = new JObject();
            customers = new JArray();
            DemandTrace.Day(ep.Day);
            Days.Add(today);
            var done = today;
            today = null;
            return done;
        }

        public JObject RunEnd(Episode ep) => today != null ? EndDay(ep) : null;

        public JObject Summary()
        {
            var s = new JObject
            {
                ["permit_bribes"] = Bribes, ["aug_good_kills"] = GoodKills, ["aug_wrongful_kills"] = WrongfulKills, ["aug_left_alive"] = LeftAlive,
                ["production"] = Production.Totals(),
            };
            if (Furnace.On) s["furnace"] = Furnace.Totals(Days.Count > 0 ? (int)Days[Days.Count - 1]["day"] : 0);
            if (Vending.Status() is JObject vend) s["vending"] = vend;
            if (Fountain.Status() is JObject fount) s["fountain"] = fount;
            if (Appraisal.Status() is JObject appr) s["appraisal"] = appr;
            if (Labels.Status() is JObject labels) s["labels"] = labels;
            if (Modules.Status() is JObject mods) s["modules"] = mods;
            if (Suppliers.BeerOn) s["beer_orders"] = Suppliers.Orders;
            if (Suppliers.Status() is JObject sup) s["suppliers"] = sup;
            if (Rats.Status() is JObject rats) s["rats"] = rats;
            if (Fish.Status() is JObject fish) s["fish"] = fish;
            if (Printer.Status() is JObject printer) s["printer"] = printer;
            if (Arms.Status() is JObject arms) s["arms"] = arms;
            if (SupplyProbe.Totals() is JObject probe) s["supply_probe"] = probe;
            s["jackson"] = new JObject
            {
                ["shop_visits"] = jacksonVisits, ["shop_spent"] = jacksonShop.Values.Sum(v => v[1]), ["store_spent"] = jacksonStore.Values.Sum(v => v[1]),
                ["shop"] = JacksonById(jacksonShop), ["store"] = JacksonById(jacksonStore),
            };
            if (Storage.On) s["storage"] = new JObject { ["stored"] = Storage.Stored, ["in_bays"] = Storage.Contents().Count() };
            return s;
        }
    }
}
