using System;
using System.Collections.Generic;
using System.Linq;
using System.Text.RegularExpressions;

namespace Fence.Aug
{
    // Recognises the undisguised aug script (docs/re/AUGS.md §2.2) in the lines a customer spoke, by their
    // displayed text: an expert who has read the game's lines knows them. Only the keys live here; the text comes
    // from the installed game's string table at run time (Install), so nothing of the game's is committed.
    public static class AugLines
    {
        // AugHelper.CreateRandomAugBuyDialogue / CreateRandomMed- / CreateRandomMaterialAugDialogue and the budget
        // lines. The gun's reactions (aug_dialog_unholster*) are left out: the plain ones are every customer's.
        public static readonly string[] Keys =
        {
            "aug_dialog_tier1_0_a", "aug_dialog_tier1_0_b", "aug_dialog_tier1_1_a", "aug_dialog_tier1_1_b",
            "aug_dialog_tier2_clean_0_a", "aug_dialog_tier2_clean_0_b", "aug_dialog_tier2_clean_1_a",
            "aug_dialog_tier2_hint_req_a", "aug_dialog_tier2_hint_req_b", "aug_dialog_tier2_hint_unit_a", "aug_dialog_tier2_hint_unit_b",
            "aug_dialog_tier3_req_a",
            "aug_dialog_budget_clean_0", "aug_dialog_budget_clean_1", "aug_dialog_budget_clean_2",
            "aug_dialog_budget_sus_0", "aug_dialog_budget_sus_1", "aug_dialog_budget_sus_2",
            "aug_dialog_med_t2_0_b", "aug_dialog_med_t2_1_b", "aug_dialog_med_t2_2_b",
            "aug_dialog_med_t3_0_a", "aug_dialog_med_t3_0_b", "aug_dialog_med_t3_1_a", "aug_dialog_med_t3_1_b",
            "aug_dialog_med_t3_2_a", "aug_dialog_med_t3_2_b", "aug_dialog_med_t3_3_b", "aug_dialog_med_t3o_0_b", "aug_dialog_med_t3o_2_b",
            "aug_dialog_med_t4_0_a", "aug_dialog_med_t4_0_b", "aug_dialog_med_t4_0_c", "aug_dialog_med_t4_1_a", "aug_dialog_med_t4_1_b", "aug_dialog_med_t4_1_c",
            "aug_dialog_mat_t2_0_b", "aug_dialog_mat_t2_1_b", "aug_dialog_mat_t2_2_b",
            "aug_dialog_mat_t3_0_b", "aug_dialog_mat_t3_1_a", "aug_dialog_mat_t3_1_b", "aug_dialog_mat_t3_2_a", "aug_dialog_mat_t3_2_b",
            "aug_dialog_mat_t3_3_a", "aug_dialog_mat_t3_3_b", "aug_dialog_mat_t3o_0_b", "aug_dialog_mat_t3o_2_b",
            "aug_dialog_mat_t4_0_a", "aug_dialog_mat_t4_0_b", "aug_dialog_mat_t4_0_c", "aug_dialog_mat_t4_1_a", "aug_dialog_mat_t4_1_b", "aug_dialog_mat_t4_1_c",
        };

        // The argument passed for {0} when reading a template; it becomes a wildcard.
        public const string Slot = "\u0001";

        static readonly Regex Tags = new Regex("<[^>]*>");
        static readonly Regex Space = new Regex(@"\s+");
        static List<Regex> templates;

        public static bool Installed => templates != null && templates.Count > 0;

        // read(key) is the localized line with Slot for its argument, or null when the table has no such key.
        // A template that is blank, or the key itself (the table is not loaded), is skipped.
        public static int Install(Func<string, string> read)
        {
            var list = new List<Regex>();
            foreach (var key in Keys)
            {
                string text;
                try
                {
                    text = read(key);
                }
                catch (Exception)
                {
                    continue;
                }
                if (string.IsNullOrWhiteSpace(text) || text == key) continue;
                string norm = Normalize(text);
                if (norm.Replace(Slot, "").Trim().Length < 8) continue;
                list.Add(new Regex("^" + string.Join(".+?", norm.Split(new[] { Slot }, StringSplitOptions.None).Select(Regex.Escape)) + "$",
                    RegexOptions.Singleline | RegexOptions.CultureInvariant));
            }
            templates = list;
            return list.Count;
        }

        public static void Reset() => templates = null;

        // The dialogue history prefixes each line with its speaker ("Name: line").
        public static bool IsAugLine(string line)
        {
            if (templates == null || string.IsNullOrWhiteSpace(line)) return false;
            string norm = Normalize(line);
            if (templates.Any(t => t.IsMatch(norm))) return true;
            int colon = norm.IndexOf(": ", StringComparison.Ordinal);
            if (colon < 0) return false;
            string said = norm.Substring(colon + 2);
            return templates.Any(t => t.IsMatch(said));
        }

        public static bool AnyShown(IEnumerable<string> lines) => lines != null && lines.Any(IsAugLine);

        // DialogUIManager.RemoveTag keeps b/u/i; the typewriter adds its own. Both sides lose every tag.
        static string Normalize(string s) => Space.Replace(Tags.Replace(s, ""), " ").Trim();
    }
}
