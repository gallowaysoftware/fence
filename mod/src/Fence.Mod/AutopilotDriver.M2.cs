using System;
using System.Collections.Generic;
using System.Linq;
using Fence.Autopilot;
using Fence.Barter;
using Fence.Gym;
using Fence.Protocol;
using Fence.Recording;
using Newtonsoft.Json.Linq;

namespace Fence.Mod
{
    // M2 (docs/STREAM.md section 5): the engines' chores, the phone, Wild's Network, the night's outing, the
    // appraisal board, the permit official's donations and the exposable pull. Every press is the hands' behind
    // the step's ActGate; every action is re-read, and a miss stops the autopilot.
    sealed partial class AutopilotDriver
    {
        const long TravelMs = 30000;
        const int MaxScavenges = 10;

        ActGate actGate;
        readonly Dictionary<int, int> soldByDay = new Dictionary<int, int>();
        NightPlan nightPlan;
        Appraisal.Answer answer;
        int scavenges;

        // The game books a trade; the day's totals feed the plan's spend limits and tomorrow's ledger. Dr.
        // Jackson's shop, outside, is not the shop's trade (as in the gym).
        void Traded(bool sold, GameItem item, int amount)
        {
            var p = PlayerDay.Current;
            if (p == null || (last != null && last.Outside)) return;
            if (sold)
            {
                p.Sold += amount;
                p.SoldItems++;
                soldByDay[p.Day] = (soldByDay.TryGetValue(p.Day, out int s) ? s : 0) + amount;
            }
            else
            {
                p.Spent += amount;
                p.Bought++;
            }
        }

        // A shared engine's chore inside the current action; an exception stops the autopilot.
        void RunChore(string what, Action chore)
        {
            try
            {
                chore();
            }
            catch (Exception e)
            {
                Stop(AutopilotState.Faulted, what + ": " + e.GetType().Name + ": " + e.Message);
            }
        }

        // The engines' hands (Fence.Gym.Chore) live: each move and machine action needs the step's open gate, is
        // re-read (the item is under where it was sent) and goes into the feed. A refusal or a miss throws, which
        // the engine's own guard turns into a fault: the autopilot stops, it never retries.
        sealed class ChoreHands : IChoreHands
        {
            readonly AutopilotDriver d;

            public ChoreHands(AutopilotDriver d) => this.d = d;

            public int Accept(GraphNodeStorage to, GameItem item, int amount)
            {
                var gate = d.actGate ?? throw new InvalidOperationException("an engine moved an item outside an autopilot action");
                int left = d.hands.Accept(gate, item, to, amount);
                if (left < 0) throw new InvalidOperationException("the move of " + LiveGame.Name(item) + " was refused (paused)");
                if (left == 0 && !LiveGame.Under(item, to)) throw new InvalidOperationException("the move of " + LiveGame.Name(item) + " did not take");
                if (left == 0 && d.choreQuiet) d.quietChores++;
                else if (left == 0) d.Record("chore_move", d.Planned(item), "into " + Where(to), "chore", null);
                return left;
            }

            public T Act<T>(string what, GameItem target, Func<T> act)
            {
                var gate = d.actGate ?? throw new InvalidOperationException("an engine acted outside an autopilot action");
                if (!d.hands.Chore(gate, act, out T result)) throw new InvalidOperationException(what + " was refused (paused)");
                if (d.choreQuiet) d.quietChores++;
                else d.Record("chore", target == null ? null : d.Planned(target), what, "chore", null);
                return result;
            }

            static string Where(GraphNodeStorage to) => to is GameItem g ? LiveGame.Name(g) : to?.GetType().Name ?? "?";
        }

        PlannedMove Planned(GameItem item) => new PlannedMove { Item = item, Uid = game.Uid(item), Name = LiveGame.Name(item) };

        void ExecuteM2(Step step, LiveView v, ActGate gate, long now)
        {
            switch (step.Kind)
            {
                case StepKind.Chores:
                    memory.ChoresDone = true;
                    RunChore("morning chores", () => ExecutorMorning.Chores(host));
                    Record("chores", null, "the engines' morning: machines tended, bays and vending stocked" + (memory.Calls.Count > 0 ? "; " + memory.Calls.Count + " call(s) to make" : ""), "chore", null);
                    return;
                case StepKind.EveningChores:
                    memory.EveningDone = true;
                    RunChore("vending", () => Vending.Evening(host));
                    RunChore("fountain", () => Fountain.Evening(host));
                    RunChore("lab", Lab.EndMorning);
                    return;
                case StepKind.OpenPhone:
                    hands.OpenPhone(gate);
                    Record("open_phone", null, "calling " + memory.Call.Who + ": " + memory.Call.Why, "phone", null);
                    expecting = Expectation.Within("the phone opens", now, ExpectMs, x => x.PhoneOpen);
                    return;
                case StepKind.Dial:
                    Dial(gate, now);
                    return;
                case StepKind.PhoneChoose:
                    PhoneChoose(gate, now);
                    return;
                case StepKind.PhoneAdvance:
                {
                    var line = LiveGame.CurrentPhoneLine();
                    hands.PhoneAdvance(gate);
                    expecting = Expectation.Within("the call's line advances", now, ExpectMs, x => !x.PhoneDialogue || !ReferenceEquals(LiveGame.CurrentPhoneLine(), line));
                    return;
                }
                case StepKind.ClosePhone:
                    hands.ClosePhone(gate);
                    Record("hang_up", null, "the call with " + memory.Call.Who + " is over", "phone", null);
                    expecting = Expectation.Within("the phone closes", now, ExpectMs, x => !x.PhoneOpen);
                    return;
                case StepKind.WildBuy:
                    WildBuy(gate, v, now);
                    return;
                case StepKind.WildClose:
                    hands.WildClose(gate);
                    memory.WildBuys = null;
                    Record("wild_close", null, "Wild's Network closed", "wild", null);
                    expecting = Expectation.Within("Wild's window closes", now, ExpectMs, x => !x.WildOpen);
                    return;
                case StepKind.PlanNight:
                    PlanNight(v);
                    return;
                case StepKind.OpenDoor:
                    OpenDoor(gate, now);
                    return;
                case StepKind.ConfirmGoOutside:
                    hands.ConfirmGoOutside(gate);
                    Record("go_outside_confirm", null, "going out with items still on the back counter", "night:outing", null);
                    expecting = Expectation.Within("the afterhours choices open", now, ExpectMs, x => x.ResponseOpen || x.BlockedBy != null);
                    return;
                case StepKind.ChooseBag:
                    if (!LiveGame.FirstChoiceIsNoBag())
                    {
                        Stop(AutopilotState.Handover, "the door's choices are not the afterhours bag choice");
                        return;
                    }
                    hands.ChooseNoBag(gate);
                    Record("choose_bag", null, "out without a backpack (the gym's outing)", "night:outing", null);
                    expecting = Expectation.Within("the map opens", now, TravelMs, x => x.MapOpen || (x.BlockedBy != null && !x.ResponseOpen));
                    return;
                case StepKind.Travel:
                    Travel(gate, now);
                    return;
                case StepKind.Scavenge:
                    Scavenge(gate, now);
                    return;
                case StepKind.PlanLoot:
                    memory.LootPlanned = true;
                    foreach (var i in NightOuting.LootOrder(LiveGame.Ground()))
                        Queue(MoveKinds.Loot, i, null, "the loot home, most valuable first", "night:loot", null);
                    Record("plan_loot", null, LiveGame.Ground().Count + " item(s) in the bag to bring home", "night:loot", null);
                    return;
                case StepKind.PlanJackson:
                    PlanJackson(v);
                    return;
                case StepKind.JacksonBuy:
                    JacksonBuy(v, now);
                    return;
                case StepKind.PlanCarryHome:
                    memory.CarriedHome = true;
                    foreach (var i in LiveGame.Ground().Concat(LiveGame.OwnedOnBack()).ToList())
                        Queue(MoveKinds.CarryHome, i, null, "bought at Dr. Jackson's: into stock (the case, or his counter, which comes home with us, when stock is full)", "player:jackson_buy", null);
                    return;
                case StepKind.Return:
                    memory.Returning = true;
                    hands.Return(gate);
                    host.Human(HumanActions.Return, null, null, new JObject { ["location"] = memory.Outing }, "night:outing");
                    Record("return", null, "home from " + memory.Outing, "night:outing", null);
                    expecting = Expectation.Within("back in the shop", now, TravelMs, x => !x.Outside || x.LeavePromptOpen);
                    return;
                case StepKind.LeaveOnGround:
                {
                    int left = LiveGame.Ground().Count;
                    hands.LeaveOnGround(gate);
                    host.Human(HumanActions.Return, null, null, new JObject { ["location"] = memory.Outing, ["left_on_ground"] = left }, "night:leave_on_ground");
                    Record("leave_on_ground", null, left + " item(s) did not fit and stay on the ground", "night:leave_on_ground", null);
                    expecting = Expectation.Within("back in the shop", now, TravelMs, x => !x.Outside);
                    return;
                }
                case StepKind.Inspect:
                    Inspect(gate, v, now);
                    return;
                case StepKind.SetEstimate:
                {
                    int want = memory.Estimate.Estimate, delta = want - v.EstimateField;
                    hands.NudgeEstimate(gate, delta);
                    Record("estimate_set", MoveOf(memory.Estimate), "the appraisal field from " + v.EstimateField + " to " + want, "player:appraise", null, v);
                    expecting = Expectation.Within("the appraisal field reads " + want, now, ExpectMs, x => x.EstimateField == want);
                    return;
                }
                case StepKind.SubmitEstimate:
                    SubmitEstimate(gate, v, now);
                    return;
                case StepKind.PlanBribe:
                    PlanBribe(v);
                    return;
                case StepKind.Donate:
                    Donate(gate, v, now);
                    return;
                case StepKind.PullExposable:
                {
                    memory.ExposableChecked = true;
                    var hidden = new[] { "item_exposable" };
                    var pull = PlayerCounter.Exposable(LiveGame.Client());
                    foreach (var i in pull) Queue(MoveKinds.Unstage, i, v.ClientKey, "the buyer would expose it on the counter (rep lost, sale or not)", "unstage_exposable", hidden);
                    if (pull.Count > 0) Record("pull_exposable", null, pull.Count + " staged item(s) the buyer would expose", "unstage_exposable", hidden, v);
                    return;
                }
                default:
                    if (!ExecuteM3(step, v, gate, now)) Stop(AutopilotState.Faulted, "no action for step " + step.Kind);
                    return;
            }
        }

        // ---- moves with their own destinations ----

        bool MoveSpecial(ActGate gate, PlannedMove m, GameItem item, LiveView v)
        {
            switch (m.Kind)
            {
                case MoveKinds.StowKit:
                {
                    var dossier = Appraisal.Dossier();
                    if (dossier != null && hands.Accept(gate, item, dossier, -1) == 0 && LiveGame.Under(item, dossier))
                    {
                        host.Human(HumanActions.Unstage, null, m.Uid, new JObject { ["owned"] = true, ["to"] = "dossier" }, m.Rule);
                        Record(m.Kind, m, m.Why, m.Rule, null, v);
                    }
                    else Queue(MoveKinds.Unstage, item, m.ClientKey, "no room in the dossier: back to stock", m.Rule, null);
                    return true;
                }
                case MoveKinds.BinSlip:
                {
                    var bin = LiveGame.Bin();
                    if (bin == null || hands.Accept(gate, item, bin, -1) != 0 || !LiveGame.Under(item, bin)) Record(m.Kind, m, "skipped: the bin does not take it (it goes with the counter at night)", m.Rule, null, v);
                    else Record(m.Kind, m, m.Why, m.Rule, null, v);
                    return true;
                }
                case MoveKinds.Loot:
                    LootHome(gate, m, item, v);
                    return true;
                case MoveKinds.CarryHome:
                    foreach (var to in new[] { LiveHands.Stock, LiveHands.Case, LiveHands.Back })
                        if (hands.Move(gate, item, to) && LiveGame.IsIn(item, to))
                        {
                            host.Human(HumanActions.Unstage, null, m.Uid, new JObject { ["owned"] = true, ["to"] = to }, m.Rule);
                            Record(m.Kind, m, "into " + to, m.Rule, null, v);
                            return true;
                        }
                    Record(m.Kind, m, "skipped: it fits nowhere (Dr. Jackson's Leave carries what is left on the ground to the back counter)", m.Rule, null, v);
                    return true;
                default:
                    return false;
            }
        }

        // One item of loot into stock; when it does not fit, cheap stock is left on the ground for it
        // (NightOuting.RoomFor, as the gym does) or the loot stays there itself. A failed fit is a choice, not a fault.
        void LootHome(ActGate gate, PlannedMove m, GameItem loot, LiveView v)
        {
            if (hands.Move(gate, loot, LiveHands.Stock) && LiveGame.IsIn(loot, LiveHands.Stock))
            {
                host.Human(HumanActions.Unstage, null, m.Uid, new JObject { ["owned"] = true }, m.Rule);
                Record(m.Kind, m, m.Why, m.Rule, null, v);
                return;
            }
            var dropped = new List<GameItem>();
            bool fitted = false;
            foreach (var j in NightOuting.RoomFor(loot, LiveGame.Stock(), PlayerStock.FreeCells()))
            {
                if (!hands.Move(gate, j, LiveHands.Ground) || !LiveGame.IsIn(j, LiveHands.Ground)) continue;
                dropped.Add(j);
                if (fitted = hands.Move(gate, loot, LiveHands.Stock) && LiveGame.IsIn(loot, LiveHands.Stock)) break;
            }
            if (fitted)
            {
                host.Human(HumanActions.Unstage, null, m.Uid, new JObject { ["owned"] = true, ["left_on_ground"] = new JArray(dropped.Select(j => game.Uid(j))) }, "night:loot_room");
                Record("loot_room", m, "made room by leaving " + string.Join(", ", dropped.Select(LiveGame.Name)) + " on the ground", "night:loot_room", null, v);
                return;
            }
            foreach (var j in dropped)
                if (!hands.Move(gate, j, LiveHands.Stock) || !LiveGame.IsIn(j, LiveHands.Stock)) Record("loot_room", Planned(j), "could not go back to stock: left on the ground", "night:loot_room", null, v);
            Record(m.Kind, m, "no room worth making: left on the ground", m.Rule, null, v);
        }

        // ---- the phone ----

        void Dial(ActGate gate, long now)
        {
            var call = memory.Call;
            call.Dialled = true;
            if (!hands.Dial(gate, call.Number))
            {
                Record("call_skipped", null, call.Who + " will not answer now (at the shop, not responding, or no such line)", "phone", null);
                return;
            }
            host.Human("phone_call", null, null, new JObject { ["number"] = call.Number, ["who"] = call.Who }, "phone");
            Record("dial", null, call.Who + ": " + call.Why, "phone", null, detail: call.Number.ToString());
            expecting = Expectation.Within("the call is answered", now, ExpectMs, x => x.PhoneDialogue || !x.PhoneOpen);
        }

        // A call's menu: the order the engine chose (or Wild's "browse"), once; anything else is not ours to answer.
        void PhoneChoose(ActGate gate, long now)
        {
            var call = memory.Call;
            int n = LiveGame.PhoneChoices();
            if (call.Chosen || call.Choice < 0 || call.Choice >= n)
            {
                Stop(AutopilotState.Handover, call.Who + "'s call offers choices the plan did not expect (" + n + ")");
                return;
            }
            call.Chosen = true;
            var line = LiveGame.CurrentPhoneLine();
            hands.PhoneChoose(gate, call.Choice);
            Record("phone_choice", null, call.Why, "phone", null, detail: call.Who + " [" + call.Choice + "]");
            expecting = Expectation.Within("the call's choice takes", now, ExpectMs, x => !ReferenceEquals(LiveGame.CurrentPhoneLine(), line));
        }

        // ---- Wild's Network ----

        // The executor's purchase list for this opening (PlayerRole.Upgrades on the window's slots), then one
        // purchase per step through the window's own Use button.
        void WildBuy(ActGate gate, LiveView v, long now)
        {
            if (memory.WildBuys == null)
            {
                string via = v.ClientPresent && !v.Outside ? "joe" : "phone";
                var picks = role.Upgrades(LiveGame.Wild(via)) ?? new List<string>();
                memory.WildBuys = new Queue<string>(picks);
                Record("wild_plan", null, picks.Count == 0 ? "nothing worth buying tonight" : "to buy: " + string.Join(", ", picks), lastDefault ?? "wild", null);
                return;
            }
            string id = memory.WildBuys.Dequeue();
            var view = LiveGame.Wild("check").Upgrades.FirstOrDefault(u => u.Id == id);
            if (view == null || !view.Buyable)
            {
                Record("wild_skip", null, id + " is no longer buyable", "wild", null);
                return;
            }
            int cash0 = LiveGame.Cash();
            hands.WildBuy(gate, id);
            if (!LiveGame.Unlocked(id))
            {
                Stop(AutopilotState.Faulted, "the purchase of " + id + " did not take");
                return;
            }
            host.Human(HumanActions.UpgradeBuy, null, null, new JObject { ["upgrade"] = id, ["cost"] = view.Cost, ["cash_spent"] = cash0 - LiveGame.Cash() }, "wild");
            Record("wild_buy", null, id + " for " + view.Cost + " (" + (cash0 - LiveGame.Cash()) + " in cash)", "player:wild_buy", null);
        }

        // ---- the night ----

        // The executor's night (PlayerRole.Night): Wild by phone when his line answers, then the outing.
        void PlanNight(LiveView v)
        {
            memory.NightPlanned = true;
            var view = LiveGame.Night(v.Day);
            view.Locations.AddRange(LiveGame.Locations());
            nightPlan = role.Night(view);
            string rule = lastDefault;
            memory.Outing = nightPlan.Outing;
            memory.Scavenge = nightPlan.Scavenge;
            scavenges = 0;
            if (nightPlan.CallWild && WildRead.Reachable())
                memory.Calls.Enqueue(new PhoneCall { Number = StorePhoneClient.WILD_CLIENT_NUMBER, Choice = 0, Who = "Wild's Network", Why = "browse the network" });
            Record("plan_night", null, (nightPlan.Outing == null ? "no outing" : "out to " + nightPlan.Outing + (nightPlan.Scavenge ? ", searching" : ""))
                + (nightPlan.CallWild ? WildRead.Reachable() ? "; Wild by phone" : "; Wild unreachable (" + WildRead.Why() + ")" : ""), rule, null);
        }

        void OpenDoor(ActGate gate, long now)
        {
            string shut = NightOuting.DoorShut();
            if (shut != null)
            {
                memory.OutingDone = true;
                Record("outing_skipped", null, "the door is shut tonight: " + shut, "night:door_shut", null);
                return;
            }
            if (!hands.OpenDoor(gate))
            {
                Stop(AutopilotState.Faulted, "the door is not in the scene");
                return;
            }
            host.Human(HumanActions.GoOut, null, null, new JObject { ["location"] = memory.Outing }, "night:outing");
            Record("open_door", null, "going out to " + memory.Outing, "night:outing", null);
            expecting = Expectation.Within("the door opens its choices", now, ExpectMs, x => x.GoOutsideConfirmOpen || x.ResponseOpen);
        }

        void Travel(ActGate gate, long now)
        {
            var loc = LiveGame.Location(memory.Outing);
            if (loc == null)
            {
                Stop(AutopilotState.Handover, memory.Outing + " is not available tonight and the map is open");
                return;
            }
            hands.Travel(gate, loc);
            string to = memory.Outing;
            Record("travel", null, "to " + to, "night:outing", null);
            expecting = Expectation.Within("arrived at " + to, now, TravelMs, x => x.Outside && x.OutsideAt == to);
        }

        // The dump's search button while searches are left and the wound odds stay under the plan's limit.
        void Scavenge(ActGate gate, long now)
        {
            var (left, wound) = LiveGame.ScavengeOdds();
            if (left <= 0 || wound > (nightPlan?.MaxWoundPct ?? double.MaxValue) || scavenges >= MaxScavenges)
            {
                memory.ScavengeDone = true;
                Record("scavenge_done", null, left <= 0 ? "no searches left" : scavenges >= MaxScavenges ? "enough searches" : "wound odds " + wound.ToString("0.#") + " % over the plan's limit", "night:scavenge", null);
                return;
            }
            scavenges++;
            int bag = LiveGame.Ground().Count;
            hands.Scavenge(gate);
            host.Human(HumanActions.Scavenge, null, null, new JObject { ["attempts_left"] = left, ["wound_pct"] = Math.Round(wound, 2) }, "night:scavenge");
            Record("scavenge", null, left + " search(es) left, wound odds " + wound.ToString("0.#") + " %", "night:scavenge", null);
            expecting = Expectation.Within("the search lands", now, ExpectMs, x => LiveGame.ScavengeOdds().left < left || LiveGame.Ground().Count != bag);
        }

        // Dr. Jackson's goods the executor wants (PlayerCounter.JacksonStage), staged on his back counter.
        void PlanJackson(LiveView v)
        {
            memory.JacksonStaged = true;
            var offered = LiveGame.TheirsOnCounter();
            var picks = PlayerCounter.JacksonStage(offered, LiveGame.Cash(), _ => true);
            foreach (var i in picks) Queue(MoveKinds.JacksonStage, i, null, "on tonight's list (the plan's signs, the engines' equipment)", "player:jackson_buy", null);
            if (picks.Count == 0) memory.JacksonBought = true;
            Record("plan_jackson", null, picks.Count + " of his " + offered.Count + " goods to buy", "player:jackson_buy", null);
        }

        void JacksonBuy(LiveView v, long now)
        {
            memory.JacksonBought = true;
            if (!LiveGame.OnBack(false).Any())
            {
                Record("jackson_skip", null, "nothing of his staged", "player:jackson_buy", null);
                return;
            }
            int cash0 = LiveGame.Cash();
            if (!advisor.ExecuteForAutopilot(ActCommands.Parse(OptionIds.BuyAll), () => Enabled && sw.Running))
            {
                Stop(AutopilotState.Faulted, "the buy at Dr. Jackson's was refused");
                return;
            }
            Record("jackson_buy", null, "bought what is staged", "player:jackson_buy", null);
            expecting = Expectation.Within("the purchase lands", now, ExpectMs, x => LiveGame.Cash() != cash0 || !LiveGame.OnBack(false).Any());
        }

        // ---- the appraisal board ----

        // The magnifier on the client's next item; the cues are read off the inspection view's item (Appraisal.Perceive,
        // what the view draws) and priced with the guides' tables.
        void Inspect(ActGate gate, LiveView v, long now)
        {
            answer = LiveGame.NextEstimate();
            var item = answer?.Item;
            if (item == null || !LiveGame.Inspectable(item))
            {
                Stop(AutopilotState.Handover, "an appraisal item the magnifier cannot open");
                return;
            }
            memory.Estimate = new PendingEstimate { Item = item, Uid = game.Uid(item), Name = LiveGame.Name(item), Estimate = answer.Estimate, Why = "read " + answer.Value + " off the guide; asking " + answer.Estimate };
            hands.Inspect(gate, item);
            host.Human("appraise", advisor.InForce(), memory.Estimate.Uid, new JObject { ["estimate"] = answer.Estimate, ["cues"] = answer.Cues }, "player:appraise");
            Record("inspect", MoveOf(memory.Estimate), memory.Estimate.Why, "player:appraise", null, v, detail: answer.Cues.ToString(Newtonsoft.Json.Formatting.None));
            expecting = Expectation.Within("the inspection view opens", now, ExpectMs, x => x.InspectionOpen);
        }

        void SubmitEstimate(ActGate gate, LiveView v, long now)
        {
            var est = memory.Estimate;
            var item = (GameItem)est.Item;
            est.Submitted = true;
            hands.SubmitEstimate(gate);
            bool accepted = !LiveGame.OnFront(item);
            if (accepted)
            {
                Appraisal.Items++;
                foreach (var slip in Appraisal.Slips()) Queue(MoveKinds.BinSlip, slip, v.ClientKey, "the appraisal slip, read: into the bin", "player:appraise", null);
                Record("estimate", MoveOf(est), "accepted at " + est.Estimate, "player:appraise", null, v);
            }
            else
            {
                int feedback = Appraisal.Refused(answer, DialogUIManager.Instance.GetCurrentDialog());
                Record("estimate", MoveOf(est), "refused as too " + (feedback < 0 ? "low" : "high") + " at " + est.Estimate, "player:appraise", null, v);
            }
            memory.Estimate = null;
            expecting = Expectation.Within("the inspection view closes", now, ExpectMs, x => !x.InspectionOpen);
        }

        static PlannedMove MoveOf(PendingEstimate e) => new PlannedMove { Item = e.Item, Uid = e.Uid, Name = e.Name };

        // ---- the permit official ----

        // His last line: goods worth the week's bribe go to the back counter (PlayerCounter.BribePicks), then the
        // Donate button (the tablet's Sell). Two rounds at most; short of goods, the line is answered instead.
        void PlanBribe(LiveView v)
        {
            memory.BribeTries++;
            var c = LiveGame.Client();
            var picks = PlayerCounter.BribePicks(c, LiveGame.Cash(), defaults, out int need, out long sum);
            var hidden = new[] { "item_exposable" };
            if (picks == null)
            {
                memory.BribeTries = 2;
                Record("bribe_short", null, "too few goods for the " + need + " owed; answering instead", lastDefault, hidden, v);
                return;
            }
            memory.DonationStaged = true;
            foreach (var i in picks) Queue(MoveKinds.Donate, i, v.ClientKey, "a donation toward the " + need + " owed", "player:bribe", hidden);
            Record("plan_bribe", null, picks.Count + " item(s) worth " + sum + " toward " + need + " owed", "player:bribe", hidden, v);
        }

        void Donate(ActGate gate, LiveView v, long now)
        {
            memory.DonationStaged = false;
            foreach (var i in PlayerCounter.Exposable(LiveGame.Client()))
                if (hands.Move(gate, i, LiveHands.Stock) && LiveGame.IsIn(i, LiveHands.Stock))
                    Record(MoveKinds.Unstage, Planned(i), "he would expose it: not a donation", "unstage_exposable", new[] { "item_exposable" }, v);
            if (LiveGame.OwnedOnBack().Count == 0)
            {
                Record("donate_skip", null, "nothing left to donate", "player:bribe", null, v);
                return;
            }
            int owed = LiveGame.BribeOwed();
            if (!advisor.ExecuteForAutopilot(ActCommands.Parse(OptionIds.SellAll), () => Enabled && sw.Running))
            {
                Stop(AutopilotState.Faulted, "the donation was refused");
                return;
            }
            Record("donate", null, "the staged goods as a donation (" + owed + " owed)", "player:bribe", null, v);
            expecting = Expectation.Within("the donation lands", now, ExpectMs, x => LiveGame.BribeOwed() != owed || LiveGame.OwnedOnBack().Count == 0);
        }
    }
}
