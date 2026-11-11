// Command fenceexemplars indexes step-mode games (sim/gym/README.md, "Playing as an agent")
// into the strong-play exemplars the bridge's -exemplars flag loads.
//
//	fenceexemplars -out var/exemplars.jsonl var/play/claude-1/normal-90001 ...
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/gallowaysoftware/fence/internal/strategy"
)

func main() {
	out := flag.String("out", "var/exemplars.jsonl", "index to write")
	flag.Parse()
	if err := run(*out, flag.Args()); err != nil {
		fmt.Fprintln(os.Stderr, "fenceexemplars:", err)
		os.Exit(1)
	}
}

func run(out string, dirs []string) error {
	if len(dirs) == 0 {
		return fmt.Errorf("no game directories given")
	}
	xs, err := strategy.BuildExemplars(dirs)
	if err != nil {
		return err
	}
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	for _, x := range xs {
		if err := enc.Encode(x); err != nil {
			f.Close()
			return err
		}
	}
	fmt.Fprintf(os.Stderr, "%d exemplars from %d games -> %s\n", len(xs), len(dirs), out)
	return f.Close()
}
