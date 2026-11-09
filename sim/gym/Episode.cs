using System;
using System.Collections.Generic;
using System.Linq;
using System.Reflection;
using System.Runtime.CompilerServices;
using Fence.Barter;
using Fence.Mod;
using Fence.Protocol;
using Fence.Recording;
using Fence.Sim;
using Newtonsoft.Json.Linq;

namespace Fence.Gym
{
    internal sealed class GymOptions
    {
        public int Seed = 1;
        public string Difficulty = "normal";
        public string Start = "none";
        public bool SkipIntro;
        public int Days = 30;
        public int DeadlineMs = 10000;
        public bool Verbose;
        public string Human = "default";
        // Test aids: check Fence.Core's option gates against the game at every decision; log phases.
        public bool Parity;
        public bool PhaseLog;
        // Writes every request sent (the wire view) to this file.
        public string WireLog;
    }

    internal enum Phase
    {
        NotStarted,
        Morning,
        Window,
        Open,
        Customer,
        Close,
        Night,
        Sleep,
        Over,
    }

    internal sealed class EpisodeStats
    {
        public int Customers, Covered, Decisions, Deferred, Executed, Sales, Purchases, Walkouts, GameErrors, Forced, Stalls;
        public List<int> CashByDay = new List<int>();
        public List<int> NetWorthByDay = new List<int>();
        public List<string> Errors = new List<string>();
        public int ParityChecked;
        public List<string> ParityProblems = new List<string>();
        public List<string> Phases = new List<string>();
    }

    // One run of the real game, stepped phase by phase: morning → window → open → each customer →
    // close → night → sleep. Everything is driven from here on one thread; the game's coroutines run
    // only inside Scheduler pumps, so a seed and a policy fix the whole trajectory.
    internal sealed class Episode
    {
        const int MaxCustomerSteps = 60;
        const int MaxClientsPerDay = 80;

        public readonly GymOptions Options;
        public readonly ModSide Mod;
        public readonly IHumanRole Human;
        public readonly Defaults Defaults;
        public readonly EpisodeStats Stats = new EpisodeStats();
        public Phase Phase { get; private set; } = Phase.NotStarted;
        public string GameOver { get; private set; }
        public int ClientSeq { get; private set; }
        public int LootTables { get; private set; }
        readonly IEnumerator<Phase> loop;

        // Per-customer state, as BarterAdvisor keeps it.
        readonly Attribution attribution = new Attribution();
        readonly TradeBatch trades = new TradeBatch();
        readonly BarterExecutor executor = new BarterExecutor();
        readonly ActGate gate;
        string lastDecision, lastAdvised;
        bool walkedOut, playerDismissing, leftReported, observed, shutterClosing;
        object clientKey;

        public Episode(GymOptions options, IBridge bridge, Trajectory trajectory, Func<Defaults, IHumanRole> human = null)
        {
            Options = options;
            Defaults = new Defaults();
            Human = human != null ? human(Defaults)
                : options.Human == "prudent" ? new PrudentHumanRole(Defaults)
                : options.Human == "default" ? new DefaultHumanRole(Defaults)
                : throw new ArgumentException("unknown -human " + options.Human + " (default|prudent)");
            Mod = new ModSide(bridge, trajectory, options.Seed) { PolicyName = bridge.Name.StartsWith("local") ? "gym-heuristic" : "bridge" };
            Mod.SnapshotError = (section, e) => GameError("snapshot " + section, e);
            gate = ActGate.TryOpen(() => Modes.Act, () => true, new ActSwitch());
            loop = Run().GetEnumerator();
        }

        public int Day => ModSide.CurrentDay();
        public int Cash => PlayerStore.Instance.playerCash;

        // Advances to the end of the next phase (or the next customer). False once the run is over.
        public bool Step()
        {
            if (Phase == Phase.Over) return false;
            if (!loop.MoveNext()) Phase = Phase.Over;
            if (Options.PhaseLog) Stats.Phases.Add(Day + ":" + Phase);
            return Phase != Phase.Over;
        }

        public void RunToEnd()
        {
            while (Step()) { }
        }

        // ---- setup -------------------------------------------------------------------------

        static readonly FieldInfo GameOverField = typeof(PlayerStore).GetField("__gameOver");

        public static string GameOverReason() => GameOverField.GetValue(null) as string;

        void NewGame()
        {
            Headless.Seed(Options.Seed);
            Scheduler.Reset();
            Hooks.Clear();
            GameOverField.SetValue(null, null);
            Scheduler.OnError = e => GameError("coroutine", e);
            Hooks.OnError = (id, e) => GameError("hook " + id, e);
            _ = new Game(playable: true);
            LootTables = Loot.Install();

            var ngd = NewGameData.Instance;
            Game.MarkAlive(ngd);
            ngd.difficultyId = Options.Difficulty;
            ngd.hardMode = Options.Difficulty == "hard" || Options.Difficulty == "challenge";
            ngd.startType = (NewGameData.StartType)Enum.Parse(typeof(NewGameData.StartType), Options.Start, ignoreCase: true);
            ngd.skipIntro = Options.SkipIntro;
            ngd.isInMainMenu = false;
            ngd.saveSlot = -1;

            var opt = EmporiumOption.Instance;
            opt.autoNextDialog = true;
            opt.autoCallNext = false;
            opt.instantCallNextCustomer = false;
            opt.moveBoughtItemsToInventory = true;
            opt.enableTypewriterEffect = false;
            opt.disablePopCultureClients = false;

            RegisterHooks();
            // BargainUIManager.Start's logic half (its window is not built headless).
            typeof(BargainUIManager).GetMethod("InitializeDialogueLines", BindingFlags.Static | BindingFlags.Public | BindingFlags.NonPublic).Invoke(null, null);
            Game.StartEmporium();
            PlayerStore.Instance.runID = "sim-s" + Options.Seed;
            Scheduler.Settle();
        }

        void RegisterHooks()
        {
            Hooks.On("PlayerStore::OnItemBought", (self, a) => Trade(TradeBatch.Bought, null, a[0] as GameItem, a.Length > 1 && a[1] is int c ? c : 0));
            Hooks.On("StoreClient::OnItemBought", (self, a) => Trade(TradeBatch.Sold, self, a[0] as GameItem, a.Length > 1 && a[1] is int c ? c : 0));
            Hooks.On("PlayerStore::DismissCurrentClient", (self, a) => BeforeDismiss());
            Hooks.On("PlayerStore::ExecuteGameOver", (self, a) => GameOver ??= a[0] as string ?? "unknown");
            foreach (var name in ModSide.MoneyHooks.Keys)
            {
                string n = name;
                Hooks.On("RunStats::" + n, (self, a) => Mod.Money(n, a));
            }
        }

        void GameError(string where, Exception e)
        {
            Stats.GameErrors++;
            if (Stats.Errors.Count < 50) Stats.Errors.Add("day " + Day + " " + where + ": " + Game.Root(e));
            if (Options.Verbose) Console.Error.WriteLine("game error " + where + ": " + Game.Inner(e));
        }

        bool Guard(string where, Action a)
        {
            try
            {
                a();
                return true;
            }
            catch (Exception e)
            {
                GameError(where, e);
                return false;
            }
        }

        bool Over()
        {
            GameOver ??= GameOverReason();
            return GameOver != null;
        }

        // ---- the run -----------------------------------------------------------------------

        IEnumerable<Phase> Run()
        {
            NewGame();
            Mod.Hello(new JObject
            {
                ["backend"] = "headless", ["seed"] = Options.Seed, ["difficulty"] = Options.Difficulty, ["start"] = Options.Start,
                ["skip_intro"] = Options.SkipIntro, ["human_role"] = Human.Name, ["days"] = Options.Days,
            });
            Mod.Emit(RecordKinds.RunStart, GameDay.Run());
            Mod.Snapshot(SnapshotPhases.RunStart);
            int firstDay = Day;

            while (!Over() && Day < firstDay + Options.Days)
            {
                Phase = Phase.Morning;
                if (Day != firstDay)
                {
                    Mod.Emit("waking_up", new JObject());
                    Mod.Snapshot(SnapshotPhases.Morning);
                    // StartOfDayUIManager.InitPanel shows the night log, then clears it.
                    PlayerStore.Instance.nightLogs?.Clear();
                }
                Mod.PollSummary();
                yield return Phase;

                Phase = Phase.Window;
                ArrangeWindow();
                yield return Phase;

                Phase = Phase.Open;
                if (Environment.GetEnvironmentVariable("FENCE_GYM_DEBUG") != null)
                    foreach (var it in EmporiumEntry.Instance.GetAllItems().Where(x => x == null || x.identifier == null))
                        Console.Error.WriteLine("debug: day " + Day + " item " + (it == null ? "null" : it.GetType().Name + " name=" + it.name + " uid=" + it.uniqueId + " parent=" + it.parentInventory?.GetType().Name + " owned=" + GeneralHelper.IsItemOwned(it)));
                Guard("open", () => StoreShutterButton.Instance.OpenShutter());
                Scheduler.Settle();
                Mod.Emit("shutter_opened", new JObject());
                Mod.Snapshot(SnapshotPhases.Open);
                yield return Phase;
                if (Over()) break;

                int served = 0;
                while (!Over() && served < MaxClientsPerDay && (PlayerStore.Instance.currentClientInstance != null || PlayerStore.Instance.storeClientManager.clientStack.Count > 0))
                {
                    Phase = Phase.Customer;
                    served++;
                    ServeNext();
                    yield return Phase;
                }
                if (Over()) break;

                Phase = Phase.Close;
                Close();
                yield return Phase;
                if (Over()) break;

                Phase = Phase.Night;
                Night();
                yield return Phase;
                if (Over()) break;

                Phase = Phase.Sleep;
                Sleep();
                Stats.CashByDay.Add(Cash);
                Stats.NetWorthByDay.Add(PlayerStore.Instance.GetTotalEstimatedValue());
                yield return Phase;
            }
            FlushTrades();
            if (GameOver != null)
            {
                Mod.Emit(RecordKinds.RunEnd, new JObject { ["ending"] = GameOver, ["day"] = Day, ["cash"] = Cash });
            }
            Phase = Phase.Over;
        }

        // ---- window ------------------------------------------------------------------------

        static IEnumerable<GameItem> In(GameInventory inv) => inv?.childItems?.Where(i => i != null && GeneralHelper.IsItemOwned(i)) ?? Enumerable.Empty<GameItem>();

        void ArrangeWindow()
        {
            var e = EmporiumEntry.Instance;
            var owned = In(e.invElement).Concat(In(e.showcaseElement)).ToList();
            const int capacity = 35; // the base 7x5 case; Move reports what actually fits
            var want = new HashSet<GameItem>(Human.Window(owned, capacity));
            foreach (var i in In(e.showcaseElement).ToList())
                if (!want.Contains(i) && Move(e.invElement, i)) Mod.Human(HumanActions.Unshowcase, null, Mod.Game.Uid(i), null, "window");
            foreach (var i in want)
                if (!ReferenceEquals(i.parentInventory, e.showcaseElement) && Move(e.showcaseElement, i)) Mod.Human(HumanActions.Showcase, null, Mod.Game.Uid(i), null, "window");
            Scheduler.Settle();
        }

        bool Move(GameGridInventory to, GameItem item)
        {
            bool ok = false;
            // TryAcceptAll returns the units it could not place.
            Guard("move", () => ok = GraphUtils.TryAcceptAll(to, item, -1) == 0 && ReferenceEquals(item.parentInventory, to));
            return ok;
        }

        // ---- customers ---------------------------------------------------------------------

        AdviceInForce Advice() => new AdviceInForce { ClientSeq = clientKey != null ? ClientSeq : 0, DecisionId = lastDecision, Advised = lastAdvised };

        void ServeNext()
        {
            var ps = PlayerStore.Instance;
            if (ps.currentClientInstance == null)
            {
                // Sprite names come from the stubbed SpriteDict; GetNextClient reads them.
                var next = ps.storeClientManager.clientStack.FirstOrDefault();
                if (next != null)
                {
                    next.spriteName ??= "";
                    next.possibleSprites ??= new List<string>();
                }
                if (!Guard("call", () => ps.TryCallNextClient()) || ps.currentClientInstance == null)
                {
                    // Nothing could be called: the queue is held back; drop it like the shutter would.
                    ps.storeClientManager.clientStack.Clear();
                    return;
                }
                Arrive();
            }
            Customer();
            if (ps.currentClientInstance != null && ReferenceEquals(GameBarter.CurrentClient(), clientKey))
            {
                Stats.Forced++;
                Defaults.Take("customer:forced_dismiss", GameBarter.CurrentClient()?.identifier);
                playerDismissing = true;
                Guard("force dismiss", () => ps.DismissCurrentClient(false, true, true));
                playerDismissing = false;
                Scheduler.Settle();
            }
            ps.isClientArrived = false;
            FlushTrades();
            Unstage();
        }

        // What the player does between customers: unsold items on the back counter go back to stock.
        void Unstage()
        {
            var e = EmporiumEntry.Instance;
            var left = In(e.backInvinvElement).ToList();
            if (left.Count == 0) return;
            Defaults.Take("unstage_leftovers", left.Count + " items");
            foreach (var i in left)
                if (Move(e.invElement, i)) Mod.Human(HumanActions.Unstage, null, Mod.Game.Uid(i), new JObject { ["owned"] = true }, "unstage_leftovers");
            Scheduler.Settle();
        }

        // StoreClientMono.OnArrived without the walk-in tween.
        void Arrive()
        {
            var ps = PlayerStore.Instance;
            var c = GameBarter.CurrentClient();
            ClientSeq++;
            clientKey = c;
            lastDecision = lastAdvised = null;
            walkedOut = playerDismissing = leftReported = observed = false;
            attribution.NewClient();
            Mod.Game.ForgetLocalIds();
            Stats.Customers++;
            ps.isClientArrived = true;
            Guard("arrive", () =>
            {
                ps.ClientFlagDown();
                StoreUIManager.Instance.OnNextClientArrived();
            });
            var info = Mod.Game.Client();
            if (info != null)
                Mod.Emit("client_arrived", new JObject { ["name"] = info.Name, ["faction"] = Factions.Name(info.FactionId), ["intent"] = IntentName(info) });
            Mod.PollSummary();
            Scheduler.Settle();
        }

        static string IntentName(ClientInfo c) => c.Intent switch
        {
            ClientIntent.Buy => "buy",
            ClientIntent.Sell => "sell",
            ClientIntent.SellAndBuy => "sell_and_buy",
            _ => (c.RawIntent ?? "other").ToLowerInvariant(),
        };

        bool Present() => PlayerStore.Instance.currentClientInstance != null && ReferenceEquals(GameBarter.CurrentClient(), clientKey);

        void Customer()
        {
            bool staged = false, sellerStaged = false;
            int stalls = 0;
            for (int step = 0; step < MaxCustomerSteps && Present() && !Over(); step++)
            {
                var dialog = DialogUIManager.Instance;
                var current = dialog.GetCurrentDialog();
                if (current != null && current.choices != null && current.choices.Count > 0)
                {
                    Choose(current);
                    continue;
                }
                if (current != null)
                {
                    // A line is still playing or waiting for its end action. A line nothing will
                    // advance (no flow pending) is clicked through, as a player would.
                    Scheduler.Pump(1.0);
                    if (Scheduler.Pending == 0 || ++stalls > 20)
                    {
                        Stats.Stalls++;
                        stalls = 0;
                        if (Options.Verbose)
                            Console.Error.WriteLine($"  stall: {GameBarter.CurrentClient()?.identifier} line '{current.plainText}' wait={ModReflect.Get(dialog, "isWaitingForManualAdvance")} auto={ModReflect.Get(dialog, "isAutoAdvancePending")} pending={Scheduler.Pending}");
                        if (ReferenceEquals(dialog.GetCurrentDialog(), current))
                            Guard("advance", () => dialog.ContinueConversation());
                        Scheduler.Settle();
                    }
                    continue;
                }

                BarterSnapshot snap = null;
                Guard("read", () => snap = BarterContext.Read(Mod.Game, ClientSeq));
                if (snap?.Client == null) break;
                ObserveOnce(snap, "ready");

                if (snap.CoverageGap != null || !snap.Ready)
                {
                    if (snap.CoverageGap == null && snap.Client.Intent == ClientIntent.Sell && !sellerStaged)
                    {
                        sellerStaged = true;
                        StageSeller(snap);
                        continue;
                    }
                    Uncovered(snap);
                    continue;
                }

                if (BarterRules.IsBuying(snap.Client.Intent) && !staged && !snap.Options.Any(o => o.Id == OptionIds.SellAll)
                    && (snap.Client.Intent == ClientIntent.Buy || !snap.Client.OffersWholesale))
                {
                    staged = true;
                    if (StageBuyer()) continue;
                }
                if (BarterRules.IsSelling(snap.Client.Intent) && !sellerStaged)
                {
                    sellerStaged = true;
                    if (StageSeller(snap)) continue;
                }

                Decide(snap);
            }
        }

        void ObserveOnce(BarterSnapshot snap, string at)
        {
            if (observed) return;
            observed = true;
            if (snap.CoverageGap == null) Stats.Covered++;
            var hidden = snap.Hidden ?? CustomerObservation.HiddenClient(snap.Client);
            var (data, hide) = CustomerObservation.Build(ClientSeq, snap.Client, CustomerKinds.Of(snap.Client), at, snap.CoverageGap,
                snap.Ready ? snap.Context : null, snap.Options, null, hidden);
            Mod.Emit(RecordKinds.CustomerObserved, data, hide);
        }

        // Customers outside the advice path: their dialogue already ran; send them on when allowed.
        void Uncovered(BarterSnapshot snap)
        {
            var ps = PlayerStore.Instance;
            if (ps.IsClientDimissable() && ps.lockNextBellFlag <= 0)
            {
                Defaults.Take("customer:dismiss_uncovered", (snap.Client.Identifier ?? "?") + " (" + (snap.CoverageGap ?? "not ready") + ")");
                Dismiss(Actors.Human, "uncovered");
            }
            else
            {
                Stats.Forced++;
                Defaults.Take("customer:forced_dismiss", snap.Client.Identifier + " (" + (snap.CoverageGap ?? "not ready") + ", not dismissable)");
                playerDismissing = true;
                Guard("force dismiss", () => ps.DismissCurrentClient(false, true, true));
                playerDismissing = false;
                Scheduler.Settle();
            }
        }

        void Dismiss(string actor, string defaultKey)
        {
            if (actor == Actors.Human) Mod.Human(OptionIds.Dismiss, Advice(), null, null, defaultKey);
            playerDismissing = true;
            Guard("dismiss", () => PlayerStore.Instance.TryPlayerDismissCurrentClient(false));
            playerDismissing = false;
            Scheduler.Settle();
        }

        bool StageBuyer()
        {
            var e = EmporiumEntry.Instance;
            var owned = In(e.invElement).Concat(In(e.showcaseElement)).ToList();
            var picks = Human.StageForBuyer(GameBarter.CurrentClient(), owned);
            int moved = 0;
            foreach (var i in picks)
                if (Move(e.backInvinvElement, i))
                {
                    moved++;
                    Mod.Human(HumanActions.Stage, Advice(), Mod.Game.Uid(i), new JObject { ["owned"] = true }, "stage_for_buyer");
                }
            Scheduler.Settle();
            return moved > 0;
        }

        bool StageSeller(BarterSnapshot snap)
        {
            var e = EmporiumEntry.Instance;
            var theirs = e.frontInvinvElement.childItems.Where(i => i != null && !GeneralHelper.IsItemOwned(i)).ToList();
            if (theirs.Count == 0) return false;
            var picks = Human.StageFromSeller(GameBarter.CurrentClient(), theirs, Cash);
            int moved = 0;
            foreach (var i in picks)
                if (Move(e.backInvinvElement, i))
                {
                    moved++;
                    Mod.Human(HumanActions.Stage, Advice(), Mod.Game.Uid(i), new JObject { ["owned"] = false }, "stage_from_seller");
                }
            Scheduler.Settle();
            return moved > 0;
        }

        void Choose(Dialogue d)
        {
            var view = new DialogueView
            {
                ClientId = GameBarter.CurrentClient()?.identifier,
                ClientName = GameBarter.CurrentClient()?.displayName,
                Line = d.plainText,
                Cash = Cash,
            };
            var owned = EmporiumEntry.Instance.GetAllOwnedItems(false, true);
            view.HoldsSeriousContraband = owned.Any(i => i != null && ContrabandHelper.IsSeriousContraband(i));
            view.HoldsStolen = owned.Any(i => i != null && StolenHelper.IsStolenItem(i));
            var infos = new List<ChoiceInfo>();
            for (int i = 0; i < d.choices.Count; i++)
            {
                var (token, reply, _) = d.choices[i];
                bool enabled = d.IsChoiceEnabledAt(i);
                view.Choices.Add(new ChoiceView { Index = i, Label = token?.name, Text = reply?.plainText, Enabled = enabled });
                infos.Add(new ChoiceInfo { Label = token?.name, Text = reply?.plainText, Enabled = enabled });
            }
            int pick = Human.Dialogue(view);
            var history = PlayerStore.Instance.dialogHistory?.Reverse().Take(6).Reverse().ToList();
            var extra = HumanActions.Choice(pick, infos, history);
            Mod.Human(HumanActions.DialogueChoice, Advice(), null, extra, "dialogue");
            bool before = ReferenceEquals(DialogUIManager.Instance.GetCurrentDialog(), d);
            Guard("choice", () => DialogUIManager.Instance.SelectChoice(pick));
            Scheduler.Settle();
            if (before && ReferenceEquals(DialogUIManager.Instance.GetCurrentDialog(), d))
            {
                // The pick was refused (disabled): take any enabled choice so the run cannot wedge.
                int any = view.Choices.FirstOrDefault(c => c.Enabled && c.Index != pick)?.Index ?? -1;
                Defaults.Take("dialogue:refused_pick", view.ClientId + " [" + pick + "]");
                if (any < 0) Guard("end dialogue", () => DialogUIManager.Instance.EndConversation());
                else Guard("choice", () => DialogUIManager.Instance.SelectChoice(any));
                Scheduler.Settle();
            }
        }

        // ---- decisions (BarterAdvisor's ask → execute path) ---------------------------------

        void Decide(BarterSnapshot snap)
        {
            string id = Mod.NextDecisionId();
            var request = BarterContext.Request(snap, id, Options.DeadlineMs);
            Stats.Decisions++;
            if (Options.Parity)
                Guard("parity", () =>
                {
                    Stats.ParityChecked++;
                    foreach (var p in Parity.Check(snap, request, Mod.Game))
                        if (Stats.ParityProblems.Count < 100) Stats.ParityProblems.Add("day " + Day + " c" + ClientSeq + " " + snap.Client.Identifier + ": " + p);
                });
            var choice = Mod.Decide(request, out var response, out string reason);
            lastDecision = id;
            string option = choice.IsDefer ? null : choice.Option;
            lastAdvised = option ?? Choice.Defer;
            string actor = Actors.Mod;
            if (option == null)
            {
                Stats.Deferred++;
                option = Human.CounterFallback(snap);
                actor = Actors.Human;
                if (option == null)
                {
                    Uncovered(snap);
                    return;
                }
            }
            var cmd = ActCommands.Parse(option);
            if (cmd == null)
            {
                Stats.Deferred++;
                Uncovered(snap);
                return;
            }
            if (actor == Actors.Human) Mod.Human(option, Advice(), null, null, "counter_fallback");
            Execute(cmd, id, actor);
        }

        void Execute(ActCommand cmd, string decisionId, string actor)
        {
            var before = cmd.Kind == ActKind.Bargain ? BeforeBargain() : null;
            if (actor == Actors.Mod)
            {
                attribution.BeginMod(decisionId);
                Stats.Executed++;
            }
            playerDismissing = cmd.Kind == ActKind.Dismiss;
            try
            {
                Guard("execute " + cmd.OptionId, () => executor.Execute(gate, cmd));
            }
            finally
            {
                playerDismissing = false;
                if (actor == Actors.Mod) attribution.EndMod();
            }
            if (before != null) AfterBargain(cmd, before, actor == Actors.Mod ? decisionId : lastDecision, actor);
            FlushTrades();
            Scheduler.Settle();
            FlushTrades();
        }

        sealed class BargainProbe
        {
            public BargainFlags Flags;
            public ClientIntent Intent;
            public IList<FactionRep> Reps;
        }

        BargainProbe BeforeBargain()
        {
            var c = GameBarter.CurrentClient();
            return new BargainProbe { Flags = Mod.Game.Bargain(), Intent = GameBarter.MapIntent(c.clientIntent), Reps = Mod.Game.Reputations() };
        }

        void AfterBargain(ActCommand cmd, BargainProbe before, string decisionId, string actor)
        {
            var c = GameBarter.CurrentClient();
            if (c == null || !ReferenceEquals(c, clientKey)) return;
            var flags = Mod.Game.Bargain();
            bool? applied = cmd.Bargain switch
            {
                BargainKind.Markup => (bool)ModReflect.Get(BargainUIManager.Instance, "markupApplied"),
                BargainKind.BuyingDiscount => (bool)ModReflect.Get(BargainUIManager.Instance, "buyingDiscountApplied"),
                BargainKind.Blackmail => (bool)ModReflect.Get(BargainUIManager.Instance, "blackmailApplied"),
                BargainKind.Threaten => (bool)ModReflect.Get(BargainUIManager.Instance, "threatenApplied"),
                _ => true,
            };
            var result = Outcomes.Bargain(cmd.Bargain, cmd.Percent, before.Flags, flags, applied, before.Intent, GameBarter.MapIntent(c.clientIntent),
                Outcomes.RepDelta(before.Reps, Mod.Game.Reputations()));
            if (result == null) return;
            if (result.Value<bool>("walked_out"))
            {
                walkedOut = true;
                Stats.Walkouts++;
            }
            Mod.Outcome(decisionId, Outcomes.WithActor(result, actor));
        }

        void Trade(string side, object storeClient, GameItem item, int amount)
        {
            if (item == null || clientKey == null || !ReferenceEquals(storeClient ?? GameBarter.CurrentClient(), clientKey)) return;
            if (trades.NeedsSnapshot(side))
            {
                FlushTrades();
                var who = attribution.Direct(lastDecision);
                trades.Begin(side, Mod.Game.Reputations(), who.decisionId, who.actor);
            }
            trades.Add(Mod.Game.Uid(item), amount);
            if (side == TradeBatch.Sold) Stats.Sales++;
            else Stats.Purchases++;
        }

        void FlushTrades()
        {
            if (trades.IsEmpty) return;
            var who = trades.DecisionId;
            var result = trades.Flush(Mod.Game.Reputations());
            if (result != null && who != null) Mod.Outcome(who, result);
        }

        void BeforeDismiss()
        {
            var c = GameBarter.CurrentClient();
            if (c == null || !ReferenceEquals(c, clientKey) || leftReported) return;
            FlushTrades();
            leftReported = true;
            if (!observed)
                Guard("observe", () => ObserveOnce(BarterContext.Read(Mod.Game, ClientSeq), "leaving"));
            string cause = Outcomes.LeftCause(walkedOut, shutterClosing, playerDismissing);
            var who = cause == LeftCauses.Dismissed ? attribution.Direct(lastDecision)
                : cause == LeftCauses.Shutter ? (lastDecision, Actors.Human)
                : attribution.Consequence(lastDecision);
            if (who.Item1 != null) Mod.Outcome(who.Item1, Outcomes.WithActor(Outcomes.Left(cause), who.Item2));
        }

        // ---- close, night, sleep -----------------------------------------------------------

        void Close()
        {
            shutterClosing = true;
            Guard("close", () => StoreShutterButton.Instance.EndAndClose());
            Scheduler.Settle();
            shutterClosing = false;
            FlushTrades();
            Mod.Emit("shutter_closed", new JObject());
            Mod.Snapshot(SnapshotPhases.Close);
        }

        List<ServiceDay> servicesAtClose;

        void Night()
        {
            servicesAtClose = Mod.DayReads.Services().ToList();
            var view = new NightView { Day = Day, Cash = Cash, RentDue = PlayerStore.Instance.GetRentValue() };
            foreach (var s in PlayerStore.Instance.storeService)
                view.Services.Add(new ServiceView { Id = s.id, Cost = s.cost, Using = s.isUsing, Mandatory = s.isMandatory, Unlocked = s.unlocked, Cooldown = s.currentCooldown });
            Guard("locations", () =>
            {
                foreach (var l in AfterhourLocation.GetDiscovered())
                    if (l != null && l.IsAvailable()) view.Locations.Add(l.Id);
            });
            var plan = Human.Night(view);
            if (plan.Outing != null) Outing(plan);
            if (plan.Services != null)
                foreach (var s in PlayerStore.Instance.storeService)
                    if (s.unlocked && !s.isMandatory) s.isUsing = plan.Services.Contains(s.id);
        }

        void Outing(NightPlan plan)
        {
            var loc = AfterhourLocation.GetDiscovered().FirstOrDefault(l => l != null && l.Id == plan.Outing);
            if (loc == null) return;
            var map = MapUIManager.Instance;
            Mod.Human(HumanActions.GoOut, null, null, new JObject { ["location"] = loc.Id }, "night:outing");
            if (!Guard("travel", () => map.Travel(loc))) return;
            Scheduler.Settle();
            if (plan.Scavenge && loc.Id == "dumping_ground")
            {
                int tries = 0;
                while (tries++ < 10 && ScavHelper.GetScavTimeLeft() > 0 && !Over())
                {
                    var odds = new JObject { ["attempts_left"] = ScavHelper.GetScavTimeLeft(), ["minor_pct"] = ScavHelper.GetMinorWoundChance(), ["major_pct"] = ScavHelper.GetMajorWoundChance() };
                    Mod.Human(HumanActions.Scavenge, null, null, odds, "night:scavenge");
                    if (!Guard("scavenge", () => map.ScavengeButtonClick())) break;
                    Scheduler.Settle();
                }
                // Loot lands in the afterhours bag; bring it home like a player who keeps everything.
                var e = EmporiumEntry.Instance;
                foreach (var i in In(e.afterhourInventory).ToList()) Move(e.invElement, i);
            }
            Mod.Human(HumanActions.Return, null, null, new JObject { ["location"] = loc.Id }, "night:outing");
            Guard("return", () => map.Return());
            Scheduler.Settle();
        }

        // EndOfDayUIManager's tablet rules, then its sleep callback.
        void Sleep()
        {
            var ps = PlayerStore.Instance;
            int total = 0;
            foreach (var s in ps.storeService.Where(s => s.unlocked))
            {
                if (s.isMandatory && !s.isUsing) s.isUsing = true;
                if (s.currentCooldown > 0 && s.isUsing && !s.isMandatory) s.isUsing = false;
                if (s.isUsing) total += s.cost;
            }
            foreach (var s in ps.storeService.Where(s => s.unlocked && s.isUsing && !s.isMandatory).OrderByDescending(s => s.cost).ThenBy(s => s.id, StringComparer.Ordinal))
            {
                if (total <= ps.playerCash) break;
                s.isUsing = false;
                total -= s.cost;
                Defaults.Take("night:service_dropped_for_cash", s.id);
            }
            Mod.Human(HumanActions.ServicesSet, null, null, ServiceSelection.Diff(servicesAtClose, Mod.DayReads.Services()), "night:services");
            Mod.Snapshot(SnapshotPhases.Sleep);
            Mod.Human(HumanActions.Sleep, null, null, null, "night:sleep");
            Mod.Emit("going_sleep", new JObject());
            Guard("sleep", SleepCallback);
            Scheduler.Settle();
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        static void SleepCallback()
        {
            var closure = typeof(EndOfDayUIManager).GetNestedTypes(BindingFlags.NonPublic).First(t => t.GetMethods(BindingFlags.Instance | BindingFlags.NonPublic | BindingFlags.Public).Any(m => m.Name.Contains("OnEndDayButtonClick>b__0")));
            var o = Activator.CreateInstance(closure);
            closure.GetField("<>4__this").SetValue(o, EndOfDayUIManager.Instance);
            var sw = closure.GetField("stopwatch");
            sw?.SetValue(o, System.Diagnostics.Stopwatch.StartNew());
            try
            {
                closure.GetMethods(BindingFlags.Instance | BindingFlags.NonPublic | BindingFlags.Public).First(m => m.Name.Contains("OnEndDayButtonClick>b__0")).Invoke(o, null);
            }
            catch (TargetInvocationException e) when (e.InnerException != null)
            {
                System.Runtime.ExceptionServices.ExceptionDispatchInfo.Capture(e.InnerException).Throw();
            }
        }
    }
}
