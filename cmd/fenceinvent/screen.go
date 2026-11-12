package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/gallowaysoftware/fence/internal/route"
)

// arm is one route the block plays: the reference or a proposal.
type arm struct {
	name string
	r    route.Route
}

func loadArms(dir, ref, only string) ([]arm, error) {
	if dir == "" {
		return nil, fmt.Errorf("needs -dir")
	}
	r, err := route.Load("", ref)
	if err != nil {
		return nil, err
	}
	arms := []arm{{ref, r}}
	paths, _ := filepath.Glob(filepath.Join(dir, "routes", "*.json"))
	want := map[string]bool{}
	for _, n := range strings.Split(only, ",") {
		if n != "" {
			want[n] = true
		}
	}
	for _, p := range paths {
		name := strings.TrimSuffix(filepath.Base(p), ".json")
		if len(want) > 0 && !want[name] {
			continue
		}
		r, err := route.Load("", p)
		if err != nil {
			return nil, err
		}
		arms = append(arms, arm{name, r})
	}
	return arms, nil
}

// screen plays every arm on seeds from..from+n-1, skipping runs already summarised; each run
// also writes its demand trace for the trading fingerprint.
func screen(ctx context.Context, block, gym string, arms []arm, from, n, par int) error {
	gym, err := filepath.Abs(gym)
	if err != nil {
		return err
	}
	type job struct {
		a    arm
		seed int
	}
	jobs := make(chan job)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var failed []string
	for range max(1, par) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				if err := runGym(ctx, block, gym, j.a, j.seed); err != nil {
					mu.Lock()
					failed = append(failed, fmt.Sprintf("%s s%d: %v", j.a.name, j.seed, err))
					mu.Unlock()
				}
			}
		}()
	}
	for s := from; s < from+n; s++ {
		for _, a := range arms {
			if ctx.Err() != nil {
				break
			}
			jobs <- job{a, s}
		}
	}
	close(jobs)
	wg.Wait()
	if len(failed) > 0 {
		return fmt.Errorf("%d runs failed: %s", len(failed), strings.Join(failed, "; "))
	}
	return ctx.Err()
}

func runGym(ctx context.Context, block, gym string, a arm, seed int) error {
	dir := filepath.Join(block, a.name)
	out := filepath.Join(dir, "s"+strconv.Itoa(seed)+".json")
	if _, err := os.Stat(out); err == nil {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path, err := filepath.Abs(a.r.Path)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "nice", "-n", "10", "dotnet", gym, "run", "-seed", strconv.Itoa(seed), "-days", "1000",
		"-difficulty", "hard", "-start", a.r.Start, "-perks", a.r.Perks, "-human", "player", "-directives", path,
		"-no-trajectory", "-summary", out+".tmp")
	cmd.Dir = filepath.Dir(gym)
	cmd.Env = append(os.Environ(), "FENCE_DEMAND_TRACE="+filepath.Join(dir, "trace"))
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if b, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%v: %s", err, lastLine(string(b)))
	}
	return os.Rename(out+".tmp", out)
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return lines[len(lines)-1]
}

// summary is the part of a gym run summary the report reads.
type summary struct {
	Seed       int     `json:"seed"`
	DaysPlayed int     `json:"days_played"`
	GameOver   string  `json:"game_over"`
	NetWorth   []int64 `json:"net_worth_by_day"`
	PlayerDays []struct {
		CashFlow map[string]float64 `json:"cash_flow"`
	} `json:"player_days"`
	Player struct {
		Appraisal struct {
			FeesTotal float64 `json:"fees_total"`
		} `json:"appraisal"`
		Furnace struct {
			IngotSales float64 `json:"ingot_sales"`
		} `json:"furnace"`
	} `json:"player"`
}

func (s summary) won() bool { return strings.HasPrefix(s.GameOver, "buyout") }

func loadRuns(dir string) map[int]summary {
	out := map[int]summary{}
	paths, _ := filepath.Glob(filepath.Join(dir, "s*.json"))
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var s summary
		if json.Unmarshal(b, &s) == nil {
			out[s.Seed] = s
		}
	}
	return out
}

// income is sim/lab/scripts/income.py's split: Board, counter, production, Georgio.
func income(runs map[int]summary) (shares [4]float64, n int) {
	var sum [4]float64
	for _, s := range runs {
		flow := map[string]float64{}
		for _, d := range s.PlayerDays {
			for k, v := range d.CashFlow {
				flow[k] += v
			}
		}
		ingots := s.Player.Furnace.IngotSales
		sum[0] += s.Player.Appraisal.FeesTotal
		sum[1] += flow["trade_sales"] - ingots + flow["vending_sales"] - flow["trade_purchases"]
		sum[2] += flow["wine_sales"] + ingots + flow["water_sales"] + flow["fountain_sales"] - flow["input_costs"]
		sum[3] += flow["georgio_payback"] - flow["georgio_loan"]
		n++
	}
	var pos float64
	for _, v := range sum {
		pos += max(0, v)
	}
	if pos == 0 {
		return shares, n
	}
	for i, v := range sum {
		shares[i] = 100 * max(0, v) / pos
	}
	return shares, n
}
