using System;
using System.Collections.Generic;
using System.Linq;

namespace Fence.Gym
{
    // What the player knows of an owned item's true condition (docs/research/FAIR-PLAY.md): its own reads and
    // records, never the item's hidden fields. This file is the measurement layer: the only executor code
    // allowed to touch an instrument's truth (the AquaScan's purity, the chem scanner's grade), and only while
    // the player owns that instrument. Everything else asks Known.
    //
    // Only five feature categories can lie in this build (ItemFeature.SetConditions with fake != real: cigarettes,
    // food stamps, injectors, chemical purity, water purity; plus the labeler's own rewrites), so every other
    // claim is the truth by the rules. Of the five:
    // - cigarettes, stamps, injectors: the magnifier's inspection view (InspectionUIManager.OnInspectItem) draws
    //   the cues, and the guides' rules turn them into a verdict (docs/re/INSPECT.md);
    // - water: the label is the truth after a faucet fill, an emptying, or a strip/AquaScan test; a bought
    //   container's label is the seller's word until then;
    // - chemical purity: the shop's own pressing labels the truth; a bought product's label is the seller's word
    //   unless the chem scanner reads it.
    internal static class Known
    {
        public const string Water = "CATEGORY_WATER_PURITY", Chem = "CATEGORY_CHEMICAL_PURITY",
            Cigarette = "CATEGORY_GENUINE_CIGARETTE", Stamp = "CATEGORY_GENUINE_STAMP", Injector = "CATEGORY_GENUINE_INJECTOR";

        // uid -> category -> the condition the player's own read showed.
        static readonly Dictionary<int, Dictionary<string, string>> reads = new Dictionary<int, Dictionary<string, string>>();
        // Injectors whose only fault is a recalled serial: no counter buyer checks recalls (ClientCanExposeUnusableInjector).
        static readonly HashSet<int> recalledOnly = new HashSet<int>();
        // Containers whose water label the executor has seen made true (a faucet fill, a test) since it last poured, and
        // those it poured into since: a pour leaves the receiver's label as it was (WaterFeatureHelper.UpdateWaterFeature).
        static readonly HashSet<int> trustedLabel = new HashSet<int>(), pouredLabel = new HashSet<int>();

        public static int Inspections, Suspects;

        public static void Reset()
        {
            reads.Clear();
            recalledOnly.Clear();
            trustedLabel.Clear();
            pouredLabel.Clear();
            Inspections = Suspects = 0;
            tools = null;
            toolsDay = -1;
        }

        static readonly string[] Instruments = { "magnifier", "aquascan", "chem_scanner" };
        static HashSet<string> tools;
        static int toolsDay = -1;

        // Re-read once a day: a tool bought or handed over today counts from tomorrow.
        static bool Owns(string id)
        {
            int day = PlayerDay.Current?.Day ?? -1;
            if (tools == null || day < 0 || day != toolsDay)
            {
                tools = new HashSet<string>(EmporiumEntry.Instance.GetAllOwnedItems(false, true).Where(i => i != null && Instruments.Contains(i.identifier)).Select(i => i.identifier), StringComparer.Ordinal);
                toolsDay = day;
            }
            return tools.Contains(id);
        }

        // ---- instruments: a reading only while the player owns the tool ----------------------------------------

        static bool Has(string id) => EmporiumEntry.Instance.GetAllOwnedItems(false, true).Any(i => i != null && i.identifier == id);

        // The AquaScan's purity (basis points) and per-liquid share (WATER.md §5), or null without one.
        public static int? AquaScan(GameItem c) => Has("aquascan") ? WaterHelper.GetWaterPurity(c) : (int?)null;
        public static int? AquaScan(GameItem c, string liquid) => Has("aquascan") ? WaterHelper.GetContaminantPercentage(c, liquid) : (int?)null;

        // The spectrometer's reading of one liquid (µL) and of a product's purity (CHEMISTRY.md §8), or null without one.
        public static int? Spectrometer(GameItem c, string liquid) => Has("chem_scanner") ? c.GetTagReadonly("CURRENT_PART_" + liquid.ToUpperInvariant()).GetInt() : (int?)null;
        public static double? Spectrometer(GameItem product) => Has("chem_scanner") ? ChemicalProductHelper.GetPurity(product) : (double?)null;

        // The gene scanner's trait rank on a seed (PLANTS.md §5), or null without one.
        public static int? GeneScanner(GameItem seed, string trait) => Has("gene_scanner") ? PlantGeneticsHelper.GetTraitRank(seed, trait) : (int?)null;

        // The plant analyser's growth score (WATER.md), or null without one.
        public static int? PlantAnalyser(GameItem seed) => Has("plant_analyser") ? HydroponicHelper.GetHealthPercent(seed) : (int?)null;

        // ---- the player's own records ---------------------------------------------------------------------------

        public static void Record(GameItem i, string category, string condition, bool recalled = false)
        {
            if (!reads.TryGetValue(i.uniqueId, out var m)) reads[i.uniqueId] = m = new Dictionary<string, string>(StringComparer.Ordinal);
            m[category] = condition;
            if (recalled) recalledOnly.Add(i.uniqueId);
        }

        // The executor's chores that make a container's label the truth (ExecutorHost.Chore).
        public static void Chore(string what, GameItem target)
        {
            if (target == null) return;
            int uid = target.uniqueId;
            if (what == "tap" || what == "test_strip") { trustedLabel.Add(uid); pouredLabel.Remove(uid); }
            else if (what == "pour") { pouredLabel.Add(uid); trustedLabel.Remove(uid); }
            else if (what == "empty") { pouredLabel.Remove(uid); trustedLabel.Remove(uid); }
        }

        // ---- the magnifier's view (docs/re/INSPECT.md) ----------------------------------------------------------

        static (string company, string model)? reference;

        // What the inspection view's injector means (InsInjectorHelper.SetExpired/SetUnusable/SetCounterfeit); `recalled`
        // when the recalled serial is the only fault.
        public static string InjectorRead(GameItem i, out bool recalled)
        {
            recalled = false;
            int indicator = i.GetTagReadonly("INJECTOR_INDICATOR").GetInt(), body = i.GetTagReadonly("INJECTOR_BODY").GetInt();
            if (indicator == 1) return "expiredInjector";
            if (indicator == 3 || (body >= 1 && body <= 3)) return "unusableInjector";
            if (body == -1) return "counterfeitInjector";
            // The guide's reference: a freshly made injector is genuine by construction.
            if (reference == null)
            {
                var g = DirectoryMaster.Item("large_purple_injector");
                reference = (g.GetTagReadonly("INJECTOR_COMPANY").GetString(), g.GetTagReadonly("INJECTOR_MODEL").GetString());
            }
            if (i.GetTagReadonly("INJECTOR_COMPANY").GetString() != reference.Value.company || i.GetTagReadonly("INJECTOR_MODEL").GetString() != reference.Value.model) return "counterfeitInjector";
            string serial = i.GetTagReadonly("INJECTOR_SERIAL").GetString() ?? "";
            if (serial.Length != 7 || !serial.All(char.IsDigit)) return "counterfeitInjector";
            int prefix = int.Parse(serial.Substring(0, 3));
            if (prefix < 125 || prefix > 455) return "counterfeitInjector";
            if (InsInjectorHelper.RECALLED_PREFIX.Contains(prefix)) { recalled = true; return "unusableInjector"; }
            return "genuineInjector";
        }

        public static string CigaretteRead(GameItem i) =>
            new[] { "CIGARETTE_SEAL", "CIGARETTE_LETTERING", "CIGARETTE_LOGO", "CIGARETTE_BOX" }.Any(t => i.GetTagReadonly(t).GetInt() != 0) ? "counterfeitCigarette" : "genuineCigarette";

        // A genuine stamp's printed amount is a multiple of 40 and carries the seal iff it is 160 or more
        // (InsStampHelper.SetGenuine); logo, lettering and chip match the guide.
        public static string StampRead(GameItem i)
        {
            int amount = i.GetTagReadonly("STAMP_AMOUNT").GetInt();
            bool seal = i.GetTagReadonly("STAMP_SEAL").GetInt() == 0;
            bool fake = amount % 40 != 0 || seal != amount >= 160
                || new[] { "STAMP_LOGO", "STAMP_LETTERING", "STAMP_CHIP" }.Any(t => i.GetTagReadonly(t).GetInt() != 0);
            return fake ? "counterfeitStamp" : "genuineStamp";
        }

        // One look through the magnifier at an inspectable item; false when there is no magnifier or nothing to read.
        public static bool Inspect(GameItem i)
        {
            string cat, read;
            bool recalled = false;
            if (i.IsTag("INS_INJECTOR_TAG")) { cat = Injector; read = InjectorRead(i, out recalled); }
            else if (i.IsTag("CIGARETTE_TAG")) { cat = Cigarette; read = CigaretteRead(i); }
            else if (i.IsTag("FOOD_STAMP_TAG")) { cat = Stamp; read = StampRead(i); }
            else return false;
            if (!Owns("magnifier")) return false;
            Record(i, cat, read, recalled);
            Inspections++;
            return true;
        }

        // ---- the truth as the player knows it -------------------------------------------------------------------

        static bool Bought(GameItem i) => i.IsTag("BOUGHT_PRICE_TAG");

        // The true condition of one feature as far as the player knows, or null.
        static ItemCondition Truth(GameItem i, ItemFeature f)
        {
            var claim = f.GetCurrentItemCondition();
            if (f.IsExposed()) return claim;
            var list = ItemConditionList.conditionLookup.TryGetValue(f.category ?? "", out var l) ? l : null;
            ItemCondition Named(string id) => list?.FirstOrDefault(c => c.identifier == id);
            if (reads.TryGetValue(i.uniqueId, out var m) && m.TryGetValue(f.category, out string seen)) return Named(seen);
            switch (f.category)
            {
                case Cigarette:
                case Stamp:
                case Injector:
                    if (!reads.ContainsKey(i.uniqueId) && Inspect(i) && reads[i.uniqueId].TryGetValue(f.category, out seen)) return Named(seen);
                    return null;
                case Water:
                    if (WaterHelper.IsEmpty(i) || trustedLabel.Contains(i.uniqueId) || (!Bought(i) && !pouredLabel.Contains(i.uniqueId))) return claim;
                    if (Owns("aquascan")) return WaterFeatureHelper.GetConditionFromPurity(WaterHelper.GetWaterPurity(i));
                    return null;
                case Chem:
                    if (!Bought(i)) return claim;
                    if (Owns("chem_scanner")) return f.realCondition;
                    return null;
                default:
                    return claim;
            }
        }

        // The claim is better than the truth (StoreClient.CanClientExposeThisFeature's IsRealFeatureCheaper).
        static bool Over(ItemCondition claim, ItemCondition truth) =>
            claim.modValue == 0 && truth.modValue == 0 ? claim.compareValue > truth.compareValue : claim.modValue > truth.modValue;

        // The conditions this feature may truly be: the known one, or every one the claim could be hiding.
        static IEnumerable<ItemCondition> Possible(GameItem i, ItemFeature f)
        {
            var claim = f.GetCurrentItemCondition();
            if (claim == null) return Enumerable.Empty<ItemCondition>();
            var t = Truth(i, f);
            if (t != null) return new[] { t };
            return ItemConditionList.conditionLookup.TryGetValue(f.category ?? "", out var l) ? l : Enumerable.Empty<ItemCondition>();
        }

        enum Buyer { Board, Scav, Lower, Tourist, Security, Upper, Basic, Any }

        // The buyer's checks are fixed by his client type (StoreClient's canClientExposeFeature, set at creation from
        // the faction; the permit official uses the trade board's): a rule a knowledgeable player knows.
        static Buyer KindOf(StoreClient c)
        {
            if (c == null) return Buyer.Any;
            if (c.identifier == "businessPermitOfficial") return Buyer.Board;
            if (c.IsTag("SCAV")) return Buyer.Scav;
            switch (c.clientFaction)
            {
                case "FACTION_LOWER_LEVEL":
                case "FACTION_BLACK_MARKET": return Buyer.Lower;
                case "FACTION_TOURIST": return Buyer.Tourist;
                case "FACTION_SECURITY": return Buyer.Security;
                case "FACTION_UPPER_LEVEL": return Buyer.Upper;
                default: return Buyer.Basic;
            }
        }

        // ClientCanExposeFunc's table, applied to a truth the player holds possible.
        static bool Checks(Buyer b, StoreClient c, GameItem i, ItemFeature f, ItemCondition claim, ItemCondition truth)
        {
            if (!Over(claim, truth)) return false;
            if (b == Buyer.Board || b == Buyer.Any) return !(f.category == Water && WaterHelper.IsEmpty(i));
            switch (f.category)
            {
                case Water:
                    if (WaterHelper.IsEmpty(i)) return false;
                    if (b == Buyer.Lower || b == Buyer.Security) return true;
                    // The flashlight only shows gutterflow and rust water.
                    return WaterFeatureHelper.GetPurityArrayIndex(Floor(truth)) >= 4;
                case Cigarette:
                    return b != Buyer.Tourist && b != Buyer.Upper;
                case Injector:
                    if (b == Buyer.Basic) return false;
                    if (truth.identifier == "unusableInjector") return !recalledOnly.Contains(i.uniqueId);
                    if (truth.identifier == "expiredInjector") return claim.identifier != "unusableInjector" && claim.identifier != "counterfeitInjector";
                    return truth.identifier == "counterfeitInjector" && claim.identifier != "unusableInjector" && b != Buyer.Tourist && b != Buyer.Upper;
                case Chem:
                    // A buyer with a chem scanner says so (the doctor's and nurse's "scanner" lines, StoreClientListTier).
                    return b != Buyer.Tourist && b != Buyer.Basic && c.CanExposeChem;
                default:
                    return false;
            }
        }

        static readonly int[] WaterFloors = { 9990, 9900, 9800, 9600, 9000, 0 };
        static int Floor(ItemCondition water) => WaterFloors[Math.Max(0, ItemConditionList.conditionWaterPurity.FindIndex(c => c.identifier == water.identifier))];

        static IEnumerable<GameItem> WithChildren(GameItem i) => new[] { i }.Concat(StoreClient.GetInspectableChildren(i));

        // Might buyer `c` (null: the strictest buyer) expose something on this item, from what the player knows?
        public static bool MayExpose(StoreClient c, GameItem i)
        {
            var b = KindOf(c);
            foreach (var g in WithChildren(i))
            {
                // A packed ammo box's count and calibres are checked against its label; its contents are not on show.
                if (AmmoBoxHelper.IsAmmoBox(g)) return true;
                foreach (var f in g.itemFeatures)
                {
                    if (f == null || f.isDisabled || f.IsExposed() || !(f.useCondition || f.isExposable)) continue;
                    var claim = f.GetCurrentItemCondition();
                    if (claim == null) continue;
                    if (Possible(g, f).Any(t => Checks(b, c, g, f, claim, t))) { Suspects++; return true; }
                }
            }
            return false;
        }

        // A claim the player knows or suspects to be better than the truth: what any buyer able to check it would find.
        public static bool MayHideFake(GameItem i) => MayExpose(null, i);

        // A container's water label is not yet known to be the truth.
        public static bool WaterUnverified(GameItem i) =>
            i.IsTag("LIQUID_CONTAINER_TAG") && !WaterHelper.IsEmpty(i) && WaterFeatureHelper.GetWaterFeature(i) is ItemFeature f && !f.IsExposed() && Truth(i, f) == null;
    }
}
