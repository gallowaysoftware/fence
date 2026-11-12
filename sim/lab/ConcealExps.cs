using System;
using System.Collections.Generic;
using System.Linq;
using Fence.Gym;
using Fence.Sim;

namespace Fence.Lab
{
    // docs/re/CONCEALMENT.md: where an owned item can sit, and who finds it there (inspectors by personality,
    // security visitors at the counter, burglars, the repo man, the night cleanup).
    public static class ConcealExps
    {
        static EmporiumEntry E => W.E;
        static double Sec => StoreReputation.GetSecReputation();

        static GameGridInventory Inner(GameItem container) => GraphUtils.FindChildrenType<GameGridInventory>(container).FirstOrDefault();

        static GameItem Owned(string id)
        {
            var it = W.Make(id);
            GeneralHelper.SetItemOwned(it, true);
            return it;
        }

        // Catalog rows are id:shape:value:fits, fits one digit per place in Catalog's order.
        static int Fit(List<string> rows, int place) => rows.Count(r => r.Split(':')[3][place] == '1');

        static string Shape(GameItem it) => $"{it.shape.width}x{it.shape.height}/{it.GetItemSize()}";

        static bool Fits(GameInventory inv, GameItem it)
        {
            if (inv == null) return false;
            bool ok = GraphUtils.TryAcceptAll(inv, it, -1) == 0;
            if (ok) inv.Expel(it);
            return ok;
        }

        // Every searched or unsearched place the shop has, emptied (the trash can and dossier keep themselves).
        static void ClearShop()
        {
            foreach (var inv in new GameInventory[] { E.invElement, E.frontInvinvElement, E.backInvinvElement, E.showcaseElement, E.hiddenElement, E.trashcanInvElement, Inner(E.dossier) })
                if (inv != null) foreach (var it in inv.childItems.ToList()) inv.Expel(it);
        }

        static void SeatInspector(int personality)
        {
            SeatClient(StoreClientList.CreateInspectionClient(personality));
        }

        static void SeatClient(StoreClient c)
        {
            c.spriteName ??= "";
            c.possibleSprites ??= new List<string>();
            W.PS.currentClientInstance = StoreClientInstance.CreateClientInstance(c);
            W.PS.isClientArrived = true;
        }

        static void Inspect(int personality)
        {
            SeatInspector(personality);
            ContrabandHelper.StartInspection(personality);
            Scheduler.Settle();
            W.Unseat();
        }

        static bool Alive(GameItem it) => E.GetAllItems().Contains(it);

        // A non-bay container that takes a 1x1 item (a backpack, crate, case...), for nesting tests.
        static string PlainContainer(string probe)
        {
            foreach (var id in DirectoryMaster.GetIdentifierList<GameItem>().Distinct().OrderBy(x => x, StringComparer.Ordinal))
            {
                if (id.Contains("smuggler") || id == "dossier" || id == "trashcan") continue;
                GameItem c;
                try { c = Owned(id); } catch { continue; }
                if (c == null || !c.IsTag("CONTAINER_TAG") || c.IsTag("LOCKED_CONTAINER_TAG") || c.IsGameItemType("MACHINE")) continue;
                var inner = Inner(c);
                if (inner == null || !(inner is GameGridInventory g) || g.inventoryShape.width < 2) continue;
                var p = Owned(probe);
                if (!Fits(inner, p)) continue;
                var bay = Owned("smuggler_bay_mod");
                if (Fits(Inner(bay), c)) return id;
            }
            return null;
        }

        // Plays a dialogue chain: start actions, then choice `pick` wherever there are choices, then end actions.
        static int Walk(Dialogue d, int pick, int maxSteps = 40)
        {
            int choices = -1;
            for (int i = 0; d != null && i < maxSteps; i++)
            {
                d.startAction?.Invoke();
                if (d.choices.Count > 0)
                {
                    choices = d.choices.Count;
                    var (_, next, act) = d.choices[Math.Min(pick, d.choices.Count - 1)];
                    act?.Invoke();
                    d = next;
                    continue;
                }
                d.endAction?.Invoke();
                Scheduler.Settle();
                d = d.nextDialogue;
            }
            return choices;
        }

        // Where a mid-level contraband item (seized when found) survives a full inspection, by place and inspector.
        // Expected from the rule: searched = stock, both counters, showcase, trash unless lazy, poster stash only
        // when open; never inside a smuggler bay; everything else (dossier, vending, outside) is never visited.
        public static void Search(int seed)
        {
            W.New(seed);
            W.Day = 20;
            W.Cash = 100000;
            string box = PlainContainer("dream_dust");
            R.Info("plain container used for nesting", box ?? "none");
            var cases = new List<(string name, bool searchedNormal, bool searchedLazy, Func<GameItem, bool> put, bool posterOpen)>
            {
                ("stock", true, true, it => W.Place(E.invElement, it), false),
                ("front counter", true, true, it => W.Place(E.frontInvinvElement, it), false),
                ("back counter", true, true, it => W.Place(E.backInvinvElement, it), false),
                ("showcase", true, true, it => W.Place(E.showcaseElement, it), false),
                ("trash can", true, false, it => W.Place(E.trashcanInvElement, it), false),
                ("poster stash, poster down (closed)", false, false, it => W.Place(E.hiddenElement, it), false),
                ("poster stash, poster up (open)", true, true, it => W.Place(E.hiddenElement, it), true),
                ("smuggler_bay_mini in stock", false, false, it => W.Place(Inner(W.Own("smuggler_bay_mini")), it), false),
                ("smuggler_bay in stock", false, false, it => W.Place(Inner(W.Own("smuggler_bay")), it), false),
                ("smuggler_bay_mod in stock", false, false, it => W.Place(Inner(W.Own("smuggler_bay_mod")), it), false),
                ("smuggler_bay on the back counter", false, false, it => { var b = Owned("smuggler_bay"); W.Place(E.backInvinvElement, b); return W.Place(Inner(b), it); }, false),
                ("smuggler_bay in the showcase", false, false, it => { var b = Owned("smuggler_bay"); W.Place(E.showcaseElement, b); return W.Place(Inner(b), it); }, false),
                ("smuggler_bay in the trash can", false, false, it => { var b = Owned("smuggler_bay"); W.Place(E.trashcanInvElement, b); return W.Place(Inner(b), it); }, false),
                ("smuggler_bay_mini in the poster stash, open", false, false, it => { var b = Owned("smuggler_bay_mini"); W.Place(E.hiddenElement, b); return W.Place(Inner(b), it); }, true),
                ("dossier", false, false, it => W.Place(Inner(E.dossier), it), false),
            };
            if (box != null)
            {
                cases.Add(($"{box} (plain container) in stock", true, true, it => { var c = Owned(box); W.Place(E.invElement, c); return W.Place(Inner(c), it); }, false));
                cases.Add(($"{box} inside smuggler_bay_mod", false, false, it => { var c = Owned(box); var b = W.Own("smuggler_bay_mod"); W.Place(Inner(b), c); return W.Place(Inner(c), it); }, false));
                cases.Add(($"{box} in the trash can", true, false, it => { var c = Owned(box); W.Place(E.trashcanInvElement, c); return W.Place(Inner(c), it); }, false));
            }
            foreach (var (name, sn, sl, put, open) in cases)
            {
                foreach (var (pname, pers, lazyBlueprint) in new[] { ("normal", 0, false), ("shady", 1, false), ("lazy", 2, true), ("normal arg, lazy blueprint", 0, true) })
                {
                    ClearShop();
                    W.ResetReps();
                    StorePoster.Instance.isPulled = open;
                    var dd = Owned("dream_dust");
                    if (!put(dd)) { R.Info($"search: {name}: dream_dust rejected", "rejected"); break; }
                    var c = StoreClientList.CreateInspectionClient(pers);
                    c.isLazyInspector = lazyBlueprint;
                    SeatClient(c);
                    double rep0 = Sec;
                    ContrabandHelper.StartInspection(pers);
                    Scheduler.Settle();
                    double dRep = Sec - rep0;
                    W.Unseat();
                    bool searched = lazyBlueprint ? sl : sn;
                    R.Exact($"search: dream_dust (mid) {name}, {pname} inspector: seized", searched, !Alive(dd), 1, $"Sec rep {dRep:+0;-0;0}");
                }
            }
            StorePoster.Instance.isPulled = false;
            // A clean shop: +3 Security; a clean shop with something concealed: 0.
            ClearShop(); W.ResetReps();
            Inspect(0);
            R.Exact("search: nothing flaggable anywhere: Security rep change", 3, Sec);
            ClearShop(); W.ResetReps();
            W.Place(Inner(W.Own("smuggler_bay")), Owned("dream_dust"));
            Inspect(0);
            R.Exact("search: only concealed contraband (bay): Security rep change", 0, Sec);
            // Low contraband found alone: warning, item kept, rep -(1+1).
            ClearShop(); W.ResetReps();
            var low = Owned("small_pistol_ammo");
            W.Place(E.invElement, low);
            Inspect(0);
            R.Exact("search: low contraband in stock: kept / Security rep", "True/-2", $"{Alive(low)}/{Sec}");
            // Low next to mid: the mid is seized, the low one stays.
            ClearShop(); W.ResetReps();
            low = Owned("small_pistol_ammo");
            var mid = Owned("dream_dust");
            W.Place(E.invElement, low); W.Place(E.invElement, mid);
            Inspect(0);
            R.Exact("search: low + mid in stock: low kept / mid kept / Security rep", "True/False/-5", $"{Alive(low)}/{Alive(mid)}/{Sec}");
            // Stolen at heat 100 (non-contraband) inside a bay vs in stock.
            foreach (var inBay in new[] { false, true })
            {
                ClearShop(); W.ResetReps();
                var ing = Owned("surgery_tool");
                StolenHelper.InitStolenItem(ing, 100);
                if (!W.Place(inBay ? Inner(W.Own("smuggler_bay_mod")) : E.invElement, ing)) throw new InvalidOperationException("surgery_tool did not fit");
                Inspect(0);
                R.Exact($"search: stolen surgery_tool (base 200) at heat 100 {(inBay ? "in a modified bay" : "in stock")}: seized / Security rep", inBay ? "False/0" : "True/-5", $"{!Alive(ing)}/{Sec}");
            }
        }

        // Stolen non-contraband in a searched place is found with chance = heat; a luck roll voids the whole find.
        public static void Odds(int seed)
        {
            W.New(seed);
            W.Day = 20;
            int n = Math.Min(Program.N, 4000);
            foreach (int heat in new[] { 25, 50, 75 })
            {
                int found = 0;
                for (int i = 0; i < n; i++)
                {
                    ClearShop();
                    var it = Owned("surgery_tool");
                    StolenHelper.InitStolenItem(it, heat);
                    W.Place(E.invElement, it);
                    Inspect(0);
                    if (!Alive(it)) found++;
                }
                R.Rate($"odds: stolen item at heat {heat} in stock: seized at an inspection", heat / 100.0, found, n);
            }
            foreach (int luck in new[] { 15, 20 })
            {
                W.PS.healthData.luck = luck;
                int saved = 0;
                for (int i = 0; i < n; i++)
                {
                    ClearShop();
                    var a = Owned("dream_dust"); var b = Owned("dream_dust");
                    W.Place(E.invElement, a); W.Place(E.trashcanInvElement, b);
                    Inspect(0);
                    bool sa = Alive(a), sb = Alive(b);
                    if (sa != sb) { R.Info("odds: luck saved one of two items, not both", $"{sa}/{sb}"); break; }
                    if (sa) saved++;
                }
                R.Rate($"odds: luck {luck}: whole find overlooked (both items kept)", luck / 100.0, saved, n);
            }
            W.PS.healthData.luck = 0;
            // Liquid luck (LiquidLuckHelper: each bottle adds 3 days, +75 luck while any are left); a voided find
            // should also withhold the clean-inspection Security bonus.
            W.PS.healthData.liquidLuckDaysLeft = 3;
            int kept = 0;
            var keptSec = new List<double>();
            for (int i = 0; i < n; i++)
            {
                ClearShop();
                var a = Owned("dream_dust");
                W.Place(E.invElement, a);
                double s0 = Sec;
                Inspect(0);
                if (Alive(a)) { kept++; keptSec.Add(Sec - s0); }
            }
            R.Rate("odds: liquid luck active, base luck 0: whole find overlooked", 0.75, kept, n);
            if (keptSec.Count > 0) R.Exact("odds: liquid luck voided a find: Security rep change", 0.0, keptSec.Average(), keptSec.Count, $"min {keptSec.Min():0.##} max {keptSec.Max():0.##}");
            W.PS.healthData.liquidLuckDaysLeft = 0;
            var clean = new List<double>();
            for (int i = 0; i < Math.Min(n, 200); i++)
            {
                ClearShop();
                double s0 = Sec;
                Inspect(0);
                clean.Add(Sec - s0);
            }
            R.Info("odds: clean shop, no luck: Security rep change per inspection", $"{clean.Average():0.###} (min {clean.Min():0.##} max {clean.Max():0.##})");
        }

        // Inspector personality odds and the bribe price.
        public static void Personality(int seed)
        {
            W.New(seed);
            var sec = W.PS.secData;
            int n = Math.Max(Program.N, 30000);
            var counts = new int[3];
            for (int i = 0; i < n; i++) counts[sec.GetInspectorPersonality()]++;
            R.Rate("personality: normal share", 1 / 3.0, counts[0], n);
            R.Rate("personality: shady share", 1 / 3.0, counts[1], n);
            R.Rate("personality: lazy share", 1 / 3.0, counts[2], n);
            sec.crimeTypes.Add("BRIBERY"); sec.crimeAmount.Add(500);
            int normal = 0;
            for (int i = 0; i < 5000; i++) if (sec.GetInspectorPersonality() == 0) normal++;
            R.Exact("personality: bribery crime 500 -> always normal", 5000, normal, 5000);
            sec.crimeTypes.Clear(); sec.crimeAmount.Clear();
            var got = new List<string>(); bool ok = true;
            foreach (var (day, rep) in new[] { (5, 0), (20, 0), (20, 50), (20, -50), (150, -50), (5, 100) })
            {
                W.Day = day; W.SetRep("FACTION_SECURITY", rep);
                int want = Math.Clamp(100 + 2 * day - 2 * StoreReputation.GetSecReputation(), 40, 400);
                int g = ContrabandHelper.GetBribeAmount();
                got.Add($"d{day}/sec{rep}:{g}");
                ok &= g == want;
            }
            R.Exact("bribe = clamp(100 + 2 day - 2 Sec, 40, 400)", true, ok, got.Count, string.Join(" ", got));
        }

        // A shady inspector who sees contraband on the counter at arrival skips straight to the generic last line.
        public static void Shady(int seed)
        {
            W.New(seed);
            W.Day = 20;
            W.Cash = 100000;
            foreach (var counter in new[] { false, true })
            {
                ClearShop(); W.ResetReps();
                if (counter) W.Place(E.backInvinvElement, Owned("dream_dust"));
                var stockItem = Owned("dream_dust");
                W.Place(E.invElement, stockItem);
                var c = StoreClientList.CreateInspectionClient(1);
                SeatClient(c);
                c.OnArrival?.Invoke();
                // Pick the last choice: the bribe when it is offered, else accept.
                int choices = Walk(c.mainDialogue, 9);
                bool searched = !Alive(stockItem);
                W.Unseat();
                R.Exact($"shady inspector, contraband on the back counter={counter}: choices at the last line / stock searched after picking the last choice",
                    counter ? "1/True" : "2/False", $"{choices}/{searched}");
            }
        }

        // Security visitors (here the patrol officer, a one-off minor client) check both counters and the showcase on
        // arrival: contraband always, stolen with chance = heat (then heat 100). A hit turns the visit into a full search.
        public static void Patrol(int seed)
        {
            W.New(seed);
            W.Day = 20;
            W.Cash = 100000;
            foreach (var (where, inv, expect) in new (string, GameGridInventory, bool)[]
            {
                ("front counter", E.frontInvinvElement, true), ("back counter", E.backInvinvElement, true), ("showcase", E.showcaseElement, true),
                ("stock", E.invElement, false), ("trash can", E.trashcanInvElement, false), ("poster stash (open)", E.hiddenElement, false),
            })
            {
                foreach (var id in new[] { "small_pistol_ammo", "dream_dust" })
                {
                    ClearShop(); W.ResetReps();
                    StorePoster.Instance.isPulled = true;
                    W.Place(inv, Owned(id));
                    var witness = Owned("dream_dust");
                    var bay = W.Own("smuggler_bay_mini");
                    if (where == "stock") W.Place(Inner(bay), witness); else W.Place(E.invElement, witness);
                    var c = StoreClientListMinor.CreatePatrolOfficer();
                    SeatClient(c);
                    c.OnArrival?.Invoke();
                    bool hit = !c.dismissable;
                    Walk(c.mainDialogue, 0);
                    bool searched = !Alive(witness);
                    W.Unseat();
                    R.Exact($"patrol arrival, {id} (level {ContrabandHelper.GetContrabandLevelInInt(W.Make(id))}) in the {where}: triggers / whole shop searched (mid witness in stock)",
                        $"{expect}/{expect && where != "stock"}", $"{hit}/{searched}");
                }
            }
            StorePoster.Instance.isPulled = false;
            // Contraband inside a bay that sits on the counter or in the showcase: invisible to the arrival check
            // and to the showcase-inspector trigger (both look only at the top-level items).
            foreach (var (where, inv) in new (string, GameGridInventory)[] { ("back counter", E.backInvinvElement), ("showcase", E.showcaseElement) })
            {
                ClearShop();
                var b = Owned("smuggler_bay");
                W.Place(inv, b);
                W.Place(Inner(b), Owned("dream_dust"));
                var c = StoreClientListMinor.CreatePatrolOfficer();
                SeatClient(c);
                c.OnArrival?.Invoke();
                bool hit = !c.dismissable;
                W.Unseat();
                W.PS.storeClientManager.clientStack.Clear();
                W.PS.storeClientManager.HandleShowCaseSecurity();
                int extra = W.PS.storeClientManager.clientStack.Count;
                W.PS.storeClientManager.clientStack.Clear();
                R.Exact($"bay holding dream_dust on the {where}: patrol arrival triggers / showcase inspector queued", "False/0", $"{hit}/{extra}");
            }
            int n = Math.Min(Program.N, 4000);
            foreach (int heat in new[] { 25, 75 })
            {
                int hits = 0, heat100 = 0;
                for (int i = 0; i < n; i++)
                {
                    ClearShop();
                    var it = Owned("surgery_tool");
                    StolenHelper.InitStolenItem(it, heat);
                    W.Place(E.backInvinvElement, it);
                    var c = StoreClientListMinor.CreatePatrolOfficer();
                    SeatClient(c);
                    c.OnArrival?.Invoke();
                    if (!c.dismissable) { hits++; if (StolenHelper.GetFoundChance(it) == 100) heat100++; }
                    W.Unseat();
                }
                R.Rate($"patrol arrival, stolen at heat {heat} on the back counter: spotted", heat / 100.0, hits, n, $"heat set to 100 on {heat100} of {hits}");
            }
            // Putting contraband on a counter while a security visitor stands there.
            foreach (var (where, inv) in new (string, GameGridInventory)[] { ("front counter", E.frontInvinvElement), ("back counter", E.backInvinvElement), ("showcase", E.showcaseElement), ("stock", E.invElement) })
            {
                ClearShop();
                var c = StoreClientListMinor.CreatePatrolOfficer();
                SeatClient(c);
                c.OnArrival?.Invoke();
                bool before = c.dismissable;
                var dd = Owned("dream_dust");
                W.Place(inv, dd);
                Scheduler.Settle();
                bool wired = before && !c.dismissable;
                // The handlers themselves, called the way GameItem.OnAddedTo*Area calls them in the game.
                var hook = where == "front counter" ? c.onItemAddedToClient : where == "back counter" ? c.onItemAddedToWeighted : where == "showcase" ? c.OnItemAddedToShowcase : null;
                try { DialogUIManager.Instance.EndConversation(); } catch { }
                c.dismissable = true;
                hook?.Invoke(dd);
                R.Info($"patrol present, dream_dust dropped on the {where}: visit turns non-dismissable via the placement / via the area handler", $"{wired}/{hook != null && !c.dismissable}");
                W.Unseat();
            }
        }

        // What the night does to each place: the front counter is wiped, the janitor empties the trash can (a
        // modified bay and its contents excepted), everything else stays. dream_dust is 1x1, so it fits everywhere.
        public static void Night(int seed) => NightFor(seed, true);
        public static void NightNoJanitor(int seed) => NightFor(seed, false);

        static void NightFor(int seed, bool janitor)
        {
            W.New(seed);
            W.Day = 20;
            W.Cash = 100000;
            ClearShop();
            var jan = StoreService.GetJanitorialService();
            jan.unlocked = true; jan.isUsing = janitor;
            var mod = Owned("smuggler_bay_mod");
            var bay = Owned("smuggler_bay");
            var where = new (string name, GameGridInventory inv, GameItem it)[]
            {
                ("front counter", E.frontInvinvElement, Owned("dream_dust")), ("back counter", E.backInvinvElement, Owned("dream_dust")),
                ("showcase", E.showcaseElement, Owned("dream_dust")), ("stock", E.invElement, Owned("dream_dust")),
                ("poster stash", E.hiddenElement, Owned("dream_dust")), ("trash can", E.trashcanInvElement, Owned("dream_dust")),
                ("smuggler_bay_mod in the trash can", Inner(mod), Owned("dream_dust")), ("smuggler_bay in the trash can", Inner(bay), Owned("dream_dust")),
            };
            W.Place(E.trashcanInvElement, mod); W.Place(E.trashcanInvElement, bay);
            foreach (var (name, inv, it) in where)
                if (!W.Place(inv, it)) throw new InvalidOperationException("could not place dream_dust in " + name);
            W.Open();
            W.CloseAndSleep();
            foreach (var (name, _, it) in where)
            {
                bool want = name != "front counter" && !(janitor && (name == "trash can" || name == "smuggler_bay in the trash can"));
                R.Exact($"night, janitor service {(janitor ? "on" : "off")}: dream_dust in the {name} still there in the morning", want, Alive(it));
            }
            R.Exact($"night, janitor service {(janitor ? "on" : "off")}: modified bay / plain bay in the trash can kept", janitor ? "True/False" : "True/True", $"{Alive(mod)}/{Alive(bay)}");
        }

        // Burglary and repossession pick from the same pool (IsEligibleForRepo over owned items, poster stash excluded).
        public static void Burglary(int seed)
        {
            W.New(seed);
            W.Day = 30;
            ClearShop();
            var mini = W.Own("smuggler_bay_mini");
            var items = new (string name, GameGridInventory inv, GameItem it, bool expect)[]
            {
                ("stock", E.invElement, Owned("dream_dust"), true),
                ("back counter", E.backInvinvElement, Owned("dream_dust"), true),
                ("showcase", E.showcaseElement, Owned("dream_dust"), true),
                ("trash can", E.trashcanInvElement, Owned("dream_dust"), true),
                ("poster stash", E.hiddenElement, Owned("dream_dust"), false),
                ("inside a mini bay", Inner(mini), Owned("dream_dust"), false),
                ("dossier (magnifier)", Inner(E.dossier), Owned("magnifier"), false),
            };
            foreach (var (name, inv, it, _) in items)
                if (!W.Place(inv, it)) throw new InvalidOperationException("could not place in " + name);
            var pool = W.PS.GetAllRepoEligibleItems();
            foreach (var (name, _, it, expect) in items)
                R.Exact($"burglary/repo pool contains an item in the {name}", expect, pool.Contains(it));
            R.Exact("burglary/repo pool contains the smuggler bay itself (IMPORTANT_TAG)", false, pool.Contains(mini));
            ClearShop();
            var mp = Owned("smuggler_bay_mini"); W.Place(E.hiddenElement, mp);
            var inMp = Owned("dream_dust"); W.Place(Inner(mp), inMp);
            pool = W.PS.GetAllRepoEligibleItems();
            R.Exact("burglary/repo pool: mini bay in the poster stash, or its contents", "False/False", $"{pool.Contains(mp)}/{pool.Contains(inMp)}");
            // Nightly odds, uninsured, first night: 33 + 5.
            var ins = StoreService.GetInsuranceService();
            ins.unlocked = true; ins.isUsing = false;
            int n = Math.Min(Program.N, 4000), stolen = 0;
            for (int i = 0; i < n; i++)
            {
                ClearShop();
                var it = W.Own("metal_ingot");
                W.PS.dayWithoutInsurance = 0;
                W.PS.isSecurityAlarmActive = false;
                W.PS.HandleInsurance();
                if (!Alive(it)) stolen++;
            }
            R.Rate("burglary: uninsured, first night: one item stolen", 0.38, stolen, n);
        }

        // Inspection early warning (SEC_INFORMANT): which night line comes how many openings before the inspector.
        public static void Warning(int seed)
        {
            W.New(seed);
            W.Day = 20;
            var svc = StoreService.GetInspectionEarlyWarningService();
            svc.unlocked = true; svc.isUsing = true;
            var sec = W.PS.secData;
            string t2 = LocHelper.GetLocalizedMechanic("mech_nightlog_inspection_2days"), t1 = LocHelper.GetLocalizedMechanic("mech_nightlog_inspection_tomorrow"),
                t0 = LocHelper.GetLocalizedMechanic("mech_nightlog_inspection_imminent");
            foreach (var total in new[] { 100, 500, 800 })
            {
                var events = new List<(int day, string kind)>();
                var sources = new Dictionary<int, string>();
                for (int d = 0; d < 300 && !W.Over; d++)
                {
                    W.Cash = 100000;
                    sec.crimeTypes.Clear(); sec.crimeAmount.Clear(); sec.crimeTypes.Add("FENCING"); sec.crimeAmount.Add(total);
                    sec.evidenceLevel = 0;
                    var q = W.Open();
                    var src = string.Join("+", q.Where(c => c.clientIntent == StoreClient.ClientIntent.INSPECTION).Select(c => c.identifier + (string.IsNullOrEmpty(c.eventSourceId) ? "" : "@" + c.eventSourceId)));
                    if (src.Length > 0) { events.Add((d, "INSPECTOR")); sources[d] = src; }
                    W.PS.storeClientManager.clientStack.Clear();
                    if (W.PS.currentClientInstance != null) W.PS.DismissCurrentClient(false, true, true);
                    StoreShutterButton.Instance.EndAndClose();
                    Scheduler.Settle();
                    W.MarkHandlersAlive();
                    foreach (var s in W.PS.storeService.Where(s => s.unlocked && s.isMandatory)) s.isUsing = true;
                    svc.isUsing = true;
                    var ins = StoreService.GetInsuranceService();
                    if (ins != null) { ins.unlocked = true; ins.isUsing = true; }
                    W.Call(typeof(Episode), "SleepCallback");
                    Scheduler.Settle();
                    foreach (var line in W.PS.nightLogs ?? new List<string>())
                    {
                        if (line.Contains(t2)) events.Add((d, "in 2 days"));
                        if (line.Contains(t1)) events.Add((d, "tomorrow"));
                        if (line.Contains(t0)) events.Add((d, "imminent"));
                    }
                    W.PS.nightLogs?.Clear();
                }
                // For each warning night, openings until the next inspector (1 = the next morning).
                var lags = new Dictionary<string, Dictionary<int, int>>();
                foreach (var (day, kind) in events.Where(e => e.kind != "INSPECTOR"))
                {
                    var next = events.FirstOrDefault(e => e.kind == "INSPECTOR" && e.day > day);
                    if (next.kind == null) continue;
                    lags.TryAdd(kind, new Dictionary<int, int>());
                    lags[kind][next.day - day] = lags[kind].GetValueOrDefault(next.day - day) + 1;
                }
                int inspections = events.Count(e => e.kind == "INSPECTOR");
                int warned = events.Where(e => e.kind == "INSPECTOR").Count(e => events.Any(w => w.kind == "imminent" && w.day == e.day - 1));
                R.Info($"early warning, FENCING {total}: line -> openings until the inspector (count)",
                    string.Join("; ", lags.OrderBy(k => k.Key).Select(k => k.Key + ": " + string.Join(",", k.Value.OrderBy(x => x.Key).Select(x => $"{x.Key}:{x.Value}")))), inspections);
                var scheduled = events.Where(e => e.kind == "INSPECTOR" && e.day > 0 && sources[e.day].Contains("securityInspector")).ToList();
                int warnedScheduled = scheduled.Count(e => events.Any(w => w.kind == "imminent" && w.day == e.day - 1));
                var unwarned = events.Where(e => e.kind == "INSPECTOR" && !events.Any(w => w.kind == "imminent" && w.day == e.day - 1)).Select(e => $"d{e.day}:{sources[e.day]}");
                R.Exact($"early warning, FENCING {total}: every scheduled inspector preceded by 'imminent' the night before", scheduled.Count, warnedScheduled, scheduled.Count, "unwarned inspectors (any source, day 0 has no night before): " + string.Join(" ", unwarned));
            }
        }

        // Contraband levels, footprints and which hiding places accept which items (local data, not committed).
        public static void Catalog(int seed)
        {
            W.New(seed);
            var places = new (string name, Func<GameInventory> inv)[]
            {
                ("poster 2x2", () => E.hiddenElement),
                ("mini bay", () => Inner(W.Own("smuggler_bay_mini"))),
                ("bay", () => Inner(W.Own("smuggler_bay"))),
                ("mod bay", () => Inner(W.Own("smuggler_bay_mod"))),
                ("dossier", () => Inner(E.dossier)),
                ("trash can", () => E.trashcanInvElement),
            };
            var invs = places.Select(p => (p.name, inv: p.inv())).ToList();
            foreach (var (name, inv) in invs)
                R.Info($"place {name}: grid", inv == null ? "none" : $"{inv.GetType().Name} {(inv as GameGridInventory)?.inventoryShape.width}x{(inv as GameGridInventory)?.inventoryShape.height}");
            foreach (var id in new[] { "smuggler_bay_mini", "smuggler_bay", "smuggler_bay_mod" })
            {
                var b = W.Make(id);
                GeneralHelper.SetItemOwned(b, true);
                string fit = string.Join(", ", invs.Concat(new[] { ("back counter", (GameInventory)E.backInvinvElement), ("showcase", E.showcaseElement), ("front counter", E.frontInvinvElement) })
                    .Select(p => $"{p.Item1}={Fits(p.Item2, b)}"));
                R.Info($"{id}: footprint w x h / cells, value, type STORAGE, fits", $"{Shape(b)}, {b.unitValue}", 1, fit);
            }
            var ids = DirectoryMaster.GetIdentifierList<GameItem>().Distinct().OrderBy(x => x, StringComparer.Ordinal).ToList();
            var byLevel = new Dictionary<int, List<string>>();
            int docs = 0;
            var docContra = new List<string>();
            foreach (var id in ids)
            {
                GameItem it;
                try { it = W.Make(id); } catch { continue; }
                if (it == null) continue;
                GeneralHelper.SetItemOwned(it, true);
                int lvl = ContrabandHelper.GetContrabandLevelInInt(it);
                bool dossierOk = Fits(Inner(E.dossier), it);
                if (dossierOk) docs++;
                if (lvl == 0) { if (dossierOk && it.GetBaseValue() > 17) docContra.Add($"{id}(v{it.GetBaseValue()},stealable)"); continue; }
                if (dossierOk) docContra.Add($"{id}(L{lvl})");
                if (!byLevel.ContainsKey(lvl)) byLevel[lvl] = new List<string>();
                string fits = string.Concat(invs.Take(4).Select(p => Fits(p.inv, it) ? "1" : "0"));
                byLevel[lvl].Add($"{id}:{Shape(it)}:v{it.GetBaseValue()}:{fits}");
            }
            foreach (var kv in byLevel.OrderBy(k => k.Key))
            {
                var l = kv.Value;
                R.Info($"contraband level {kv.Key}: count / fit poster,mini,bay,mod", $"{l.Count}; poster {Fit(l, 0)}, mini {Fit(l, 1)}, bay {Fit(l, 2)}, mod {Fit(l, 3)}",
                    l.Count, string.Join(" ", l));
            }
            R.Info("dossier accepts (owned): item count; contraband or stealable (base > 17) among them", docs, 1, string.Join(" ", docContra));
        }
    }
}
