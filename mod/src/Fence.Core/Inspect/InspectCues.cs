using System.Collections.Generic;
using System.Globalization;
using System.Linq;

namespace Fence.Inspect
{
    // What the inspection views read off an item (docs/re/INSPECT.md): int and string tags, and the
    // condition its authenticity feature shows. The live game, the headless gym and tests implement it.
    public interface IInspectItem
    {
        bool IsTag(string tag);
        int Int(string tag);
        string Str(string tag);
        // The condition identifier the item shows for a feature category: the customer's claim until exposed.
        string ShownCondition(string category);
        // HIDDEN: the true condition identifier.
        string RealCondition(string category);
    }

    public static class ItemKinds
    {
        public const string Cigarettes = "cigarettes";
        public const string FoodStamp = "food_stamp";
        public const string Injector = "injector";
        public const string Ribwich = "ribwich";
        public const string Hand = "prosthetic_hand";
        public const string Plasma = "plasma_cell";

        // InspectionUIManager.OnInspectItem's dispatch, in its order. Berries have no verdict and are left out.
        static readonly (string tag, string kind)[] ByTag =
        {
            ("CIGARETTE_TAG", Cigarettes),
            ("PLASMA_CELL_TAG", Plasma),
            ("FOOD_STAMP_TAG", FoodStamp),
            ("INS_INJECTOR_TAG", Injector),
            ("RIBWICH_TAG", Ribwich),
            ("HAND_TAG", Hand),
        };

        public static string Of(IInspectItem item)
        {
            foreach (var (tag, kind) in ByTag)
                if (item.IsTag(tag)) return kind;
            return null;
        }

        public static bool IsAuthenticity(string kind) => kind == Cigarettes || kind == FoodStamp || kind == Injector;

        public static bool IsAppraisal(string kind) => kind == Ribwich || kind == Hand || kind == Plasma;

        // The ItemFeature category whose condition is the claim.
        public static string ClaimCategory(string kind)
        {
            switch (kind)
            {
                case Cigarettes: return "CATEGORY_GENUINE_CIGARETTE";
                case FoodStamp: return "CATEGORY_GENUINE_STAMP";
                case Injector: return "CATEGORY_GENUINE_INJECTOR";
                default: return null;
            }
        }
    }

    // The confront panel's five toggles per kind, in order (InspectionUIManager.On*Confront): the option
    // id suffix, the toggle's caption and the issue tag the game checks.
    public sealed class ConfrontIssue
    {
        public string Id;
        public string Caption;
        public string Tag;

        public ConfrontIssue(string id, string caption, string tag)
        {
            Id = id;
            Caption = caption;
            Tag = tag;
        }
    }

    public static class ConfrontIssues
    {
        static readonly Dictionary<string, ConfrontIssue[]> ByKind = new Dictionary<string, ConfrontIssue[]>
        {
            [ItemKinds.Cigarettes] = new[]
            {
                new ConfrontIssue("lettering", "lettering", "CIGARETTE_LETTERING_ISSUE"),
                new ConfrontIssue("color", "colour", "CIGARETTE_COLOR_ISSUE"),
                new ConfrontIssue("seal", "seal", "CIGARETTE_SEAL_ISSUE"),
                new ConfrontIssue("logo", "logo", "CIGARETTE_LOGO_ISSUE"),
                new ConfrontIssue("box", "box", "CIGARETTE_BOX_ISSUE"),
            },
            [ItemKinds.FoodStamp] = new[]
            {
                new ConfrontIssue("lettering", "lettering", "STAMP_LETTERING_ISSUE"),
                new ConfrontIssue("logo", "logo", "STAMP_LOGO_ISSUE"),
                new ConfrontIssue("seal", "seal", "STAMP_SEAL_ISSUE"),
                new ConfrontIssue("amount", "value", "STAMP_AMOUNT_ISSUE"),
                new ConfrontIssue("chip", "chip", "STAMP_CHIP_ISSUE"),
            },
            [ItemKinds.Injector] = new[]
            {
                new ConfrontIssue("indicator", "indicator", "ISSUE_INDICATOR"),
                new ConfrontIssue("label", "label", "ISSUE_LABEL"),
                new ConfrontIssue("serial", "serial", "ISSUE_SERIAL"),
                new ConfrontIssue("body", "body", "ISSUE_BODY"),
                new ConfrontIssue("color", "colour", "ISSUE_COLOR"),
            },
        };

        public static IList<ConfrontIssue> Of(string kind) => kind != null && ByKind.TryGetValue(kind, out var a) ? a : new ConfrontIssue[0];

        // The issue the confront button submits: the last checked toggle in code order wins.
        public static string Chosen(string kind, IList<bool> toggles)
        {
            var issues = Of(kind);
            string chosen = null;
            for (int i = 0; i < issues.Count && toggles != null && i < toggles.Count; i++)
                if (toggles[i]) chosen = issues[i].Id;
            return chosen;
        }
    }

    // The cue dictionary: each visual tell as the words for what it depicts, and the printed text as the
    // views print it. Derived from the tags exactly as the views' OnInit render them (CigaretteUI, StampUI,
    // InjectorUI, RibwichUI, AppHandUI, FuelUI), so it never carries a sprite index: index 0 is the
    // genuine variant by construction, and an index would be the verdict in disguise.
    // internal/policy/testdata/inspect_cues.json lists every word; the Go rule checker reads the same list.
    public sealed class Cues
    {
        public readonly SortedDictionary<string, string> Text = new SortedDictionary<string, string>();
        public readonly SortedDictionary<string, string> Visual = new SortedDictionary<string, string>();
    }

    public static class CueWords
    {
        public const string Matches = "matches reference";
        public const string Missing = "missing";
        public const string Present = "present";
        public const string Hidden = "hidden under double meat";

        // A reference-or-not sprite: 0 is the reference, -1 is not drawn, anything else a wrong variant.
        static string Sprite(int i, string wrong) => i == 0 ? Matches : i < 0 ? Missing : wrong;

        public static Cues Of(string kind, IInspectItem item)
        {
            var c = new Cues();
            switch (kind)
            {
                case ItemKinds.Cigarettes: Cigarettes(item, c); break;
                case ItemKinds.FoodStamp: Stamp(item, c); break;
                case ItemKinds.Injector: Injector(item, c); break;
                case ItemKinds.Ribwich: Ribwich(item, c); break;
                case ItemKinds.Hand: Hand(item, c); break;
                case ItemKinds.Plasma: Plasma(item, c); break;
            }
            return c;
        }

        static void Cigarettes(IInspectItem i, Cues c)
        {
            c.Visual["seal"] = Sprite(i.Int("CIGARETTE_SEAL"), "broken");
            c.Visual["lettering"] = Sprite(i.Int("CIGARETTE_LETTERING"), "misprinted");
            c.Visual["logo"] = Sprite(i.Int("CIGARETTE_LOGO"), "wrong logo");
            // The base box is always drawn; a non-zero CIGARETTE_BOX lays a colour variant over it.
            int box = i.Int("CIGARETTE_BOX");
            c.Visual["box_colour"] = box == 0 || box == -1 ? "matches colour card" : "off-shade";
        }

        static void Stamp(IInspectItem i, Cues c)
        {
            c.Text["amount"] = N(i.Int("STAMP_AMOUNT")) + " credits";
            c.Visual["seal"] = i.Int("STAMP_SEAL") == -1 ? Missing : Present;
            c.Visual["lettering"] = Sprite(i.Int("STAMP_LETTERING"), "misprinted");
            c.Visual["logo"] = Sprite(i.Int("STAMP_LOGO"), "wrong logo");
            c.Visual["chip"] = Sprite(i.Int("STAMP_CHIP"), "wrong chip");
        }

        static void Injector(IInspectItem i, Cues c)
        {
            c.Text["serial"] = i.Str("INJECTOR_SERIAL") ?? "";
            c.Text["company"] = i.Str("INJECTOR_COMPANY") ?? "";
            c.Text["model"] = i.Str("INJECTOR_MODEL") ?? "";
            switch (i.Int("INJECTOR_INDICATOR"))
            {
                case 0: c.Visual["indicator"] = "normal"; break;
                case 1: c.Visual["indicator"] = "expired warning"; break;
                case 3: c.Visual["indicator"] = "fault warning"; break;
                case -1: c.Visual["indicator"] = "blank"; break;
                default: c.Visual["indicator"] = "unrecognised"; break;
            }
            // INJECTOR_BODY 1-3 swaps the body for a broken one; a colour variant replaces the body sprite.
            c.Visual["body"] = i.Int("INJECTOR_BODY") >= 1 ? "cracked" : "intact";
            c.Visual["body_colour"] = i.Int("INJECTOR_COLOR") != -1 ? "off-colour" : "standard";
        }

        static void Ribwich(IInspectItem i, Cues c)
        {
            bool barcode = i.Int("RIBWICH_PRODUCTION_TAG") == 0;
            bool facility = i.Int("RIBWICH_FACILITY_TAG") == 0;
            c.Visual["barcode_stamp"] = barcode ? Present : Missing;
            c.Visual["facility_stamp"] = facility ? Present : Missing;
            if (barcode) c.Text["production_date"] = N(i.Int("RIBWICH_PRODUCTION_MONTH_TAG")) + "/ " + N(i.Int("RIBWICH_PRODUCTION_YEAR_TAG"));
            if (facility) c.Text["facility"] = "#" + N(i.Int("RIBWICH_FACILITY_NUMBER_TAG"));
            bool doubleMeat = i.Int("RIBWICH_MEAT_TAG") == 1;
            c.Visual["meat"] = doubleMeat ? "double" : "single";
            // The double patty replaces the top bun and pickles in the view.
            c.Visual["top_bun"] = doubleMeat ? Hidden : i.Int("RIBWICH_BUN_TAG") == 1 ? "deluxe" : "regular";
            c.Visual["pickles"] = doubleMeat ? Hidden : i.Int("RIBWICH_PICKLES_TAG") == 1 ? "extra" : "regular";
            int wrap = i.Int("RIBWICH_WRAP_TAG");
            c.Visual["wrap"] = wrap == 1 || wrap == 2 ? "opened" : "sealed";
            c.Visual["davre_logo"] = Sprite(i.Int("RIBWICH_DAVRE_LOGO_TAG"), "wrong logo");
            int logo = i.Int("RIBWICH_RIBWICH_LOGO_TAG");
            c.Visual["ribwich_logo"] = logo == 0 ? Present : logo < 0 ? "absent" : "wrong logo";
        }

        static void Hand(IInspectItem i, Cues c)
        {
            switch (i.Str("HAND_MATERIAL"))
            {
                case "GOLD_PLATED": c.Visual["material"] = "gold plated"; break;
                case "CARBON_COMPOSITE": c.Visual["material"] = "carbon composite"; break;
                default: c.Visual["material"] = "aluminium alloy"; break;
            }
            string marking;
            switch (i.Str("HAND_INTERFACE"))
            {
                case "EMG": marking = "EMG"; break;
                case "EMG_PLUS": marking = "EMG+"; break;
                case "NEURAL": marking = "Neural"; break;
                default: marking = "none"; break;
            }
            c.Visual["interface_marking"] = marking;
            if (marking != "none") c.Visual["marking_style"] = i.Str("HAND_INTERFACE_MARKING") == "SYMBOL" ? "symbol" : "full label";
            switch (i.Str("HAND_CONDITION"))
            {
                case "WEAR": c.Visual["condition"] = "worn"; break;
                case "DAMAGE": c.Visual["condition"] = "damaged"; break;
                default: c.Visual["condition"] = "pristine"; break;
            }
            c.Visual["armor_plating"] = i.Str("HAND_ADDON_ARMORED") == "ARMORED" ? "fitted" : "none";
            c.Visual["blade"] = i.Str("HAND_ADDON_BLADED") == "BLADED" ? "fitted" : "none";
        }

        static void Plasma(IInspectItem i, Cues c)
        {
            // An unknown colour draws no overlay and its value rule is unclear: leave the cue out.
            switch (i.Str("PLASMA_COLOR"))
            {
                case "STANDARD": c.Visual["colour"] = "standard"; break;
                case "HIGH_QUALITY": c.Visual["colour"] = "high quality"; break;
                case "POOR1":
                case "POOR2": c.Visual["colour"] = "poor"; break;
            }
            switch (i.Str("PLASMA_SEAL"))
            {
                case "LL": c.Visual["seal_brand"] = "LL"; break;
                case "ZHONGHUA": c.Visual["seal_brand"] = "Zhonghua"; break;
                case "REPUBLIC": c.Visual["seal_brand"] = "Republic"; break;
                case "IGNITION_LAB": c.Visual["seal_brand"] = "Ignition Lab"; break;
                default: c.Visual["seal_brand"] = "none"; break;
            }
            switch (i.Str("PLASMA_CASING_CONDITION_WEAR"))
            {
                case "WEAR": c.Visual["casing_wear"] = "worn"; break;
                case "WEAR_HEAVY": c.Visual["casing_wear"] = "heavily worn"; break;
                default: c.Visual["casing_wear"] = "none"; break;
            }
            c.Visual["crack"] = i.Str("PLASMA_CASING_CONDITION_CRACK") == "CRACK" ? "cracked" : "none";
            c.Visual["screw"] = i.Str("PLASMA_CASING_CONDITION_UNCOVERED") == "UNCOVERED" ? "uncovered" : "covered";
            c.Visual["reinforcement"] = i.Str("PLASMA_CASING_CONDITION_REINFORCED") == "REINFORCED" ? "reinforced" : "no";
        }

        // The tags the views render, for the hidden block (raw indices are labels, never context).
        public static readonly Dictionary<string, string[]> IntTags = new Dictionary<string, string[]>
        {
            [ItemKinds.Cigarettes] = new[] { "CIGARETTE_SEAL", "CIGARETTE_LETTERING", "CIGARETTE_LOGO", "CIGARETTE_BOX" },
            [ItemKinds.FoodStamp] = new[] { "STAMP_AMOUNT", "STAMP_SEAL", "STAMP_LETTERING", "STAMP_LOGO", "STAMP_CHIP" },
            [ItemKinds.Injector] = new[] { "INJECTOR_INDICATOR", "INJECTOR_BODY", "INJECTOR_COLOR" },
            [ItemKinds.Ribwich] = new[]
            {
                "RIBWICH_MEAT_TAG", "RIBWICH_BUN_TAG", "RIBWICH_WRAP_TAG", "RIBWICH_PICKLES_TAG", "RIBWICH_DAVRE_LOGO_TAG", "RIBWICH_RIBWICH_LOGO_TAG",
                "RIBWICH_FACILITY_NUMBER_TAG", "RIBWICH_PRODUCTION_MONTH_TAG", "RIBWICH_PRODUCTION_YEAR_TAG", "RIBWICH_PRODUCTION_TAG", "RIBWICH_FACILITY_TAG",
            },
        };

        public static readonly Dictionary<string, string[]> StringTags = new Dictionary<string, string[]>
        {
            [ItemKinds.Hand] = new[] { "HAND_MATERIAL", "HAND_INTERFACE", "HAND_INTERFACE_MARKING", "HAND_CONDITION", "HAND_ADDON_ARMORED", "HAND_ADDON_BLADED" },
            [ItemKinds.Plasma] = new[]
            {
                "PLASMA_COLOR", "PLASMA_SEAL", "PLASMA_CASING_CONDITION_WEAR", "PLASMA_CASING_CONDITION_CRACK", "PLASMA_CASING_CONDITION_UNCOVERED",
                "PLASMA_CASING_CONDITION_REINFORCED",
            },
        };

        public static IEnumerable<string> IssueTags(string kind, IInspectItem item) => ConfrontIssues.Of(kind).Where(x => item.IsTag(x.Tag)).Select(x => x.Tag);

        static string N(int n) => n.ToString(CultureInfo.InvariantCulture);
    }
}
