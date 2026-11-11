using System;
using System.Collections.Generic;
using System.Linq;
using Newtonsoft.Json.Linq;

namespace Fence.Gym
{
    // `printer: "run"` (MachinePrinter, docs/re/RECIPES.md §1): the 3D printer (the Gunsmith start's, or the retired
    // gunsmith's) works a chip each night: in its ready state with the chip's inputs in the input grid it consumes
    // them, then adds 24 progress a night (8 power) and drops the chip's output in its output grid; the chip is never
    // used up. Each morning, at the printer's window: the output goes to stock, a charged cell into the battery and,
    // when the printer is idle, the first chip in `printer_chips` that is owned, fits the printer's modules and whose
    // inputs are owned goes into the chip slot with those inputs. The default order is by computed margin per night:
    // the silencer and improved springs (one night on scrap), the sights (plastic or scrap and an electronic), then the
    // melee blades. Printed pistols are contraband and are left out. Chips, plastic and a spare cell
    // are bought as engine inputs (chips from the retired gunsmith's stock); the output is ordinary stock.
    internal static class Printer
    {
        public static bool On => Directives.Current.Raw.Value<string>("printer") == "run";

        static readonly string[] DefaultChips =
        {
            "printer_chip_silencer", "printer_chip_internal_springs_improved", "printer_chip_makeshift_rds", "printer_chip_rds", "printer_chip_laser_sight",
            "printer_chip_combat_machete", "printer_chip_combat_knife", "printer_chip_kitchen_knife", "printer_chip_kitchen_cleaver", "printer_chip_hatchet",
            "printer_chip_box_cutter",
        };

        static IList<string> Chips => (Directives.Current.Raw["printer_chips"] as JArray)?.Select(x => (string)x).ToList() ?? (IList<string>)DefaultChips;
        // Chips are bought until `printer_buy_chips` (default 2) of the order are owned, with 1 500 over the rent.
        static int BuyChips => Directives.Current.Raw.Value<int?>("printer_buy_chips") ?? 2;

        const string Scrap = "scrap_metal", Plastic = "printer_plastic", Cell = "energy_credit", Electronic = "common_electronic";

        public static int Loads, FirstDay;
        public static long MadeValue, InputValue, ChipSpend;
        static readonly SortedDictionary<string, int> byChip = new SortedDictionary<string, int>(StringComparer.Ordinal);

        public static void Reset()
        {
            Loads = FirstDay = 0;
            MadeValue = InputValue = ChipSpend = 0;
            byChip.Clear();
        }

        static IEnumerable<GameItem> Owned() => EmporiumEntry.Instance.GetAllOwnedItems(false, true).Where(i => i != null);
        static GameItem Parent(GameItem i) => i.parentInventory?.GetParentItem();
        static int Energy(GameItem cell) => cell == null ? 0 : PowerHelper.GetAvailableEnergyFromItem(cell, false);

        public static List<GameItem> Printers() => EmporiumEntry.Instance == null ? new List<GameItem>() : Owned().Where(i => i.IsTag("PRINTER_TAG") && !i.IsTag("BROKEN_MACHINE_TAG") && Parent(i) == null).ToList();

        // MachinePrinter.CreateMachineInventoryWindow: battery, module grid, input grid, chip slot, output grid, note.
        static GameGridInventory Input(GameItem p) => MachineHelper.GetBodyStorage(p, 2) as GameGridInventory;
        static GameSlotInventory ChipSlot(GameItem p) => MachineHelper.GetBodyStorage(p, 3) as GameSlotInventory;
        static GameGridInventory Output(GameItem p) => MachineHelper.GetBodyStorage(p, 4) as GameGridInventory;

        static bool IsChip(GameItem i) => i.IsTag("PRINTER_CHIP_TAG");

        // A chip's inputs: (item id, count) pairs from its REQUIRED_ITEM_ID_n / REQUIRED_ITEM_COUNT_n tags.
        static List<(string id, int n)> Needs(GameItem chip)
        {
            var l = new List<(string, int)>();
            int kinds = chip.GetTagReadonly("REQUIRED_ITEM_TYPE_COUNT_TAG").GetInt();
            for (int k = 0; k < kinds; k++)
            {
                string id = chip.GetTagReadonly("REQUIRED_ITEM_ID_" + k).GetString();
                int n = chip.GetTagReadonly("REQUIRED_ITEM_COUNT_" + k).GetInt();
                if (!string.IsNullOrEmpty(id) && n > 0) l.Add((id, n));
            }
            return l;
        }

        static IEnumerable<GameItem> Loose(string id) => Owned().Where(i => i.identifier == id && Parent(i) == null && !StolenHelper.IsStolenItem(i));

        public static bool Reserved(GameItem i)
        {
            if (!On || Printers().Count == 0) return false;
            if (i.IsTag("PRINTER_TAG") || IsChip(i) || i.identifier == Plastic || i.identifier == "printer_module_metal" || i.identifier == "printer_module_cnc") return true;
            return false;
        }

        public static bool IsInput(string id) => On && id != null && (Chips.Contains(id) || id == Plastic || id == Scrap || id == Electronic || id == Cell || id == "printer_module_metal");

        public static int Wants(string id)
        {
            if (!On || id == null || Printers().Count == 0) return 0;
            var owned = Owned().ToList();
            if (Chips.Contains(id))
            {
                int have = Chips.Count(c => owned.Any(i => i.identifier == c));
                return have < BuyChips && !owned.Any(i => i.identifier == id) && PlayerStore.Instance.playerCash > (PlayerDay.Current?.RentDue ?? 0) + 1500 ? 1 : 0;
            }
            if (id == Cell)
                return Printers().Any(p => Energy(MachineHelper.GetBatterySlot(p)?.childItem) < 3 * MachineryHelper.GetMachinePowerUsage(p)) && !Loose(Cell).Any(c => Energy(c) >= 24) ? 1 : 0;
            if (id == "printer_module_metal")
                return owned.Any(i => i.identifier == id) || !owned.Any(i => IsChip(i) && (i.identifier == "printer_chip_combat_machete" || i.identifier == "printer_chip_combat_knife")) ? 0 : 1;
            // Two runs' worth of each input the owned chips use.
            int want = owned.Where(i => IsChip(i) && Chips.Contains(i.identifier)).SelectMany(Needs).Where(n => n.id == id).Select(n => 2 * n.n).DefaultIfEmpty(0).Max();
            return Math.Max(0, want - owned.Count(i => i.identifier == id && Parent(i) == null));
        }

        public static void Morning(IExecutorHost ep)
        {
            if (!On) return;
            var stock = EmporiumEntry.Instance.invElement;
            foreach (var p in Printers())
            {
                if (FirstDay == 0) FirstDay = ep.Day;
                try { Tend(ep, p, stock); }
                catch (Exception e) { ep.ProductionError(e); }
            }
        }

        static void Tend(IExecutorHost ep, GameItem p, GameGridInventory stock)
        {
            if (!ReferenceEquals(p.parentInventory, stock)) Chore.Accept(stock, p, -1);
            var input = Input(p);
            var slot = ChipSlot(p);
            var output = Output(p);
            if (input == null || slot == null || output == null) return;
            foreach (var it in output.childItems.Where(i => i != null).ToList())
            {
                MadeValue += PlayerRole.Est(it);
                Chore.Accept(stock, it, -1);
            }
            // Power for the night: a cell with more charge than the one in the battery.
            int usage = MachineryHelper.GetMachinePowerUsage(p);
            var bat = MachineHelper.GetBatterySlot(p);
            if (bat != null && Energy(bat.childItem) < usage)
            {
                var cell = Loose(Cell).OrderByDescending(Energy).FirstOrDefault();
                if (cell != null && Energy(cell) > Energy(bat.childItem))
                {
                    var old = bat.childItem;
                    if (old == null || Chore.Accept(stock, old, -1) == 0) Chore.Accept(bat, cell, -1);
                }
            }
            var owned = Owned().ToList();
            if (Environment.GetEnvironmentVariable("FENCE_PRINTER_DEBUG") != null)
                Console.Error.WriteLine($"printer d{ep.Day} state={MachineProgressHelper.GetMachineState(p)} chip={slot.childItem?.identifier} bat={Energy(bat?.childItem)} use={usage} scrap={Loose(Scrap).Count()} plastic={Loose(Plastic).Count()} cells={Loose(Cell).Count()} chips={string.Join(",", owned.Where(IsChip).Select(i => i.identifier))}");
            if (MachineProgressHelper.GetMachineState(p) != "STATE_READY") return;
            GameItem pick = null;
            foreach (var id in Chips)
            {
                var chip = owned.FirstOrDefault(i => i.identifier == id && IsChip(i) && (Parent(i) == null || ReferenceEquals(i.parentInventory, slot)));
                if (chip == null || !MachineProgressHelper.CanStart(chip, p)) continue;
                if (Needs(chip).All(n => input.childItems.Count(i => i != null && i.identifier == n.id) + Loose(n.id).Count() >= n.n)) { pick = chip; break; }
            }
            if (pick == null) return;
            if (!ReferenceEquals(slot.childItem, pick))
            {
                if (slot.childItem != null && Chore.Accept(stock, slot.childItem, -1) != 0) return;
                if (Chore.Accept(slot, pick, -1) != 0) return;
            }
            // Only what this chip uses stays in the input grid.
            var needs = Needs(pick);
            foreach (var it in input.childItems.Where(i => i != null && !needs.Any(n => n.id == i.identifier)).ToList()) Chore.Accept(stock, it, -1);
            long loaded = 0;
            foreach (var (id, n) in needs)
                foreach (var it in Loose(id).Take(Math.Max(0, n - input.childItems.Count(i => i != null && i.identifier == id))).ToList())
                {
                    loaded += PlayerRole.Est(it);
                    Chore.Accept(input, it, -1);
                }
            InputValue += loaded;
            Loads++;
            byChip[pick.identifier] = (byChip.TryGetValue(pick.identifier, out int c) ? c : 0) + 1;
            ep.Human("production", null, ep.Uid(p), new JObject { ["action"] = "print", ["chip"] = pick.identifier }, "player:printer");
        }

        public static void Trade(bool sold, GameItem item, int amount)
        {
            if (On && !sold && item != null && IsChip(item)) ChipSpend += amount;
        }

        public static JObject Status()
        {
            if (!On) return null;
            return new JObject
            {
                ["first_day"] = FirstDay, ["printers"] = Printers().Count, ["loads"] = Loads, ["made_value"] = MadeValue, ["input_value"] = InputValue,
                ["chip_spend"] = ChipSpend, ["by_chip"] = JObject.FromObject(byChip),
                ["chips_owned"] = new JArray(Owned().Where(IsChip).Select(i => i.identifier).OrderBy(x => x, StringComparer.Ordinal)),
            };
        }
    }
}
