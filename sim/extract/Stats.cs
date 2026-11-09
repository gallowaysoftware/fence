using System;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using System.Text.Json;

namespace Fence.Extract
{
    // `extract stats`: summary numbers from var/gamedata, the baseline recorded play is compared
    // against. Reads the JSON only; never loads the game.
    static class Stats
    {
        public static int Print(string dir)
        {
            if (!System.IO.Directory.Exists(dir)) { Console.Error.WriteLine("no " + dir + "; run `extract` first"); return 1; }
            var shas = new HashSet<string>();
            JsonElement Load(string table)
            {
                var doc = JsonDocument.Parse(File.ReadAllBytes(Path.Combine(dir, table + ".json")));
                shas.Add(doc.RootElement.GetProperty("source").GetProperty("assembly_csharp_sha256").GetString());
                return doc.RootElement;
            }

            var items = Load("items");
            var list = items.GetProperty("items").EnumerateArray().ToList();
            Console.WriteLine($"items          {list.Count} built, {items.GetProperty("failed").GetArrayLength()} failed");
            var bv = list.Select(i => Mean(i, "base_value")).Where(x => x.HasValue).Select(x => x.Value).OrderBy(x => x).ToList();
            Console.WriteLine($"  base value   min {bv.First():0} p25 {Pct(bv, 25):0} median {Pct(bv, 50):0} p75 {Pct(bv, 75):0} max {bv.Last():0}");
            Console.WriteLine($"  contraband   {list.Count(i => (Mean(i, "contraband_level") ?? 0) > 0)} items (level>0)");
            Console.WriteLine($"  with features {list.Count(i => i.TryGetProperty("features", out var f) && f.GetArrayLength() > 0)}");
            Console.WriteLine("  types        " + Top(list.SelectMany(i => i.GetProperty("types").EnumerateArray().Select(t => t.GetString())), 12));

            var clients = Load("clients");
            var cl = clients.GetProperty("clients").EnumerateArray().ToList();
            Console.WriteLine($"clients        {cl.Count} factories in StoreClientListDict");
            foreach (var phase in clients.GetProperty("phases").EnumerateObject().Select(p => p.Name))
            {
                var ph = cl.Where(c => c.TryGetProperty(phase, out _)).Select(c => c.GetProperty(phase)).ToList();
                int errors = ph.Count(p => p.TryGetProperty("error", out _));
                int stock = ph.Count(p => p.TryGetProperty("stock_samples", out _));
                int stolen = ph.Count(p => p.TryGetProperty("stock_heat", out _));
                var budgets = ph.Select(p => Mean(p, "budget")).Where(x => x.HasValue).Select(x => x.Value).OrderBy(x => x).ToList();
                Console.WriteLine($"  {phase,-18} errors {errors}, selling with stock {stock}, with stolen stock {stolen}, buyers {budgets.Count} (budget median {Pct(budgets, 50):0}, max {budgets.LastOrDefault():0})");
                Console.WriteLine("    intents      " + Top(ph.SelectMany(p => p.TryGetProperty("intent", out var it) ? it.EnumerateObject().Select(o => o.Name) : Enumerable.Empty<string>()), 10));
            }
            Console.WriteLine("  factions     " + Top(cl.Where(c => c.TryGetProperty("faction", out _)).Select(c => c.GetProperty("faction").GetString()), 10));

            var pools = Load("client_pools").GetProperty("pools").EnumerateArray().ToList();
            Console.WriteLine($"pools          {pools.Count}: " + string.Join(", ", pools.Select(p => p.GetProperty("faction").GetString() + "×" + p.GetProperty("entries").GetArrayLength())));

            var mix = Load("normal_mix").GetProperty("cells").EnumerateArray().ToList();
            Console.WriteLine($"normal mix     {mix.Count} cells (attractiveness × reputation), normal customers/day " +
                string.Join(" ", mix.Select(c => $"{c.GetProperty("attractiveness")}/{c.GetProperty("reputation_all")}:{c.GetProperty("normal_customer_count")}")));

            var days = Load("days").GetProperty("days").EnumerateArray().Where(d => d.TryGetProperty("day", out _)).ToList();
            var q = days.Select(d => d.GetProperty("queue").GetArrayLength()).ToList();
            Console.WriteLine($"days           {days.Count} simulated (idle policy), queue mean {q.Average():0.0} min {q.Min()} max {q.Max()}, errors {days.Count(d => d.TryGetProperty("error", out _))}");
            Console.WriteLine("  most queued  " + Top(days.SelectMany(d => d.GetProperty("queue").EnumerateArray().Select(c => c.GetProperty("id").GetString())), 10));
            Console.WriteLine($"  cash         day {days.First().GetProperty("day")} {days.First().GetProperty("cash")} → day {days.Last().GetProperty("day")} {days.Last().GetProperty("cash")}");

            var rep = Load("reputation").GetProperty("factions").EnumerateArray().ToList();
            Console.WriteLine($"reputation     {rep.Count} factions, {rep.Sum(f => f.GetProperty("perks").GetArrayLength())} perks");
            Console.WriteLine($"upgrades       {Load("upgrades").GetProperty("upgrades").GetArrayLength()}");
            Console.WriteLine($"services       {Load("services").GetProperty("services").GetArrayLength()}");
            var diff = Load("difficulty").GetProperty("difficulties").EnumerateArray().Select(d => d.GetProperty("id").GetString());
            Console.WriteLine($"difficulties   {string.Join(", ", diff)}");
            var ev = Load("market_events");
            Console.WriteLine($"market events  {ev.GetProperty("normalEventBlueprints").GetArrayLength()} normal, {ev.GetProperty("threatEventBlueprints").GetArrayLength()} threat, {ev.GetProperty("cosmeticEventBlueprints").GetArrayLength()} cosmetic blueprints; {ev.GetProperty("event_factories").GetArrayLength()} factories");
            Console.WriteLine($"starting perks {Load("starting_perks").GetProperty("perks").GetArrayLength()}");
            Console.WriteLine($"afterhours     {Load("afterhours").GetProperty("definitions").GetArrayLength()} event definitions");
            Load("constants");

            var problems = Load("extract_report").GetProperty("problems").GetArrayLength();
            Console.WriteLine($"problems       {problems} (extract_report.json)");
            Console.WriteLine(shas.Count == 1 ? "source         Assembly-CSharp " + shas.First()[..12] : "source         MIXED builds: " + string.Join(", ", shas));
            return shas.Count == 1 ? 0 : 1;
        }

        static double? Mean(JsonElement e, string name)
        {
            if (!e.TryGetProperty(name, out var v)) return null;
            if (v.ValueKind == JsonValueKind.Number) return v.GetDouble();
            if (v.ValueKind == JsonValueKind.Object && v.TryGetProperty("mean", out var m)) return m.GetDouble();
            return null;
        }

        static double Pct(List<double> sorted, int p) => sorted.Count == 0 ? 0 : sorted[Math.Min(sorted.Count - 1, sorted.Count * p / 100)];

        static string Top(IEnumerable<string> xs, int n) =>
            string.Join(", ", xs.GroupBy(x => x).OrderByDescending(g => g.Count()).ThenBy(g => g.Key, StringComparer.Ordinal).Take(n).Select(g => $"{g.Key} {g.Count()}"));
    }
}
