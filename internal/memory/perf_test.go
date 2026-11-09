package memory

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestBriefQueriesUseIndexes(t *testing.T) {
	s := openStore(t, "", Options{})
	tables := []string{"prices", "bargain_stats", "interactions", "day_stats", "day_rep", "lessons", "sessions", "runs"}
	for _, q := range []string{qLessons, qPrices, qPricesCondition, qLastPrice, qOddsBand, qOddsFaction, qClient, qDay, qDayRep, qSessionRun, qLessonsAndDay} {
		n := strings.Count(q, "?")
		args := make([]any, n)
		for i := range args {
			args[i] = "x"
		}
		rows, err := s.db.Query("EXPLAIN QUERY PLAN "+q, args...)
		if err != nil {
			t.Fatal(err)
		}
		var plan []string
		for rows.Next() {
			var id, parent, notused int
			var detail string
			rows.Scan(&id, &parent, &notused, &detail)
			plan = append(plan, detail)
		}
		rows.Close()
		for _, d := range plan {
			scansTable := slices.ContainsFunc(tables, func(tb string) bool { return d == "SCAN "+tb || strings.HasPrefix(d, "SCAN "+tb+" ") })
			if scansTable || strings.Contains(d, "TEMP B-TREE") {
				t.Errorf("query not index-backed: %s\nplan: %v", strings.Join(strings.Fields(q), " "), plan)
			}
		}
	}
}

func TestBriefFastOnBigDB(t *testing.T) {
	s := openStore(t, "", Options{})
	tx, err := s.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	const items, perItem, clients = 300, 40, 200
	for i := 0; i < items*perItem; i++ {
		side := []string{"sold", "bought"}[i%2]
		tx.Exec(`INSERT INTO prices (item_id, item_name, condition, side, price, estimated_value, faction, run_id, game_day, created_at)
			VALUES (?, ?, ?, ?, ?, ?, 'lower_levels', ?, ?, '')`,
			fmt.Sprintf("item%d", i%items), "Item", []string{"", "Worn", "Mint"}[i%3], side, 100+i%50, 120, 1+i%3, i%100)
	}
	for i := 0; i < 20000; i++ {
		tx.Exec(`INSERT INTO interactions (run_id, client_name, client_faction, game_day, decision_id, summary) VALUES (?, ?, 'lower_levels', ?, 'd', 'we sold Item for 100')`,
			1+i%3, fmt.Sprintf("Client %d", i%clients), i%100)
	}
	for _, m := range []string{"add_markup", "offer_discount", "demand_discount", "offer_markup"} {
		for _, p := range []int{10, 25, 50, 75} {
			for _, b := range []string{"-10..10", "10..50", "50..100"} {
				tx.Exec(`INSERT INTO bargain_stats VALUES (?, ?, 'lower_levels', ?, 5, 3, 1)`, m, p, b)
			}
		}
	}
	for d := 1; d <= 100; d++ {
		tx.Exec(`INSERT INTO day_stats VALUES (1, ?, 30, 10, 5, 900, 2)`, d)
		tx.Exec(`INSERT INTO day_rep VALUES (1, ?, 'lower_levels', 1.5)`, d)
		tx.Exec(`INSERT INTO lessons VALUES (1, ?, ?, ?, 'test', '')`, d, d, strings.Repeat("- rule\n", 200))
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	s.runs.resolve("s1", 50, false, "")

	var ours, theirs []string
	for i := 0; i < 6; i++ {
		ours = append(ours, itemJSON(i, fmt.Sprintf("item%d", i), "Item", 100, 120, "Worn"))
		theirs = append(theirs, itemJSON(100+i, fmt.Sprintf("item%d", 100+i), "Item", 100, 120, "Mint"))
	}
	req := barter("s1", "q", 50, "Client 7", "lower_levels", 12, ours, theirs,
		"sell_all", "add_markup_10", "add_markup_25", "add_markup_50", "add_markup_75", "offer_discount_10", "dismiss")
	s.Brief(context.Background(), req, 6000)
	var times []time.Duration
	var out string
	for i := 0; i < 7; i++ {
		start := time.Now()
		out = s.Brief(context.Background(), req, 6000)
		times = append(times, time.Since(start))
	}
	slices.Sort(times)
	limit := 20 * time.Millisecond
	if raceEnabled {
		limit *= 5
	}
	if med := times[len(times)/2]; med > limit {
		t.Fatalf("median Brief %v over %v (all %v)", med, limit, times)
	}
	for _, want := range []string{"Item (item0): we sold", "add_markup_25: 9 tries", `"Client 7"`, "Today so far (day 50)", "Lessons from earlier days"} {
		if !strings.Contains(out, want) {
			t.Errorf("brief missing %q:\n%s", want, out)
		}
	}
	t.Logf("median Brief %v over %d prices, %d bytes", times[len(times)/2], items*perItem, len(out))
}
