// Command sim plays bot-vs-bot games headlessly and checks engine invariants
// after every command. It is the fuzz harness for the rules engine.
//
//	go run ./cmd/sim -games 1000 -seed 42 -players 4
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"time"

	"monopsony/server/internal/bot"
	"monopsony/server/internal/game"
)

func main() {
	games := flag.Int("games", 200, "number of games to play")
	seed := flag.Uint64("seed", 42, "base RNG seed")
	players := flag.Int("players", 4, "players per game (2-8)")
	maxSteps := flag.Int("max-steps", 20000, "commands per game before declaring a stall")
	turnLimit := flag.Int("turn-limit", 0, "optional turn limit house rule (0 = none)")
	surrender := flag.Int("surrender", 0, "if >0, roughly one in N commands is a random active player surrendering")
	verbose := flag.Bool("v", false, "print per-game summaries")
	flag.Parse()

	cfg := game.HarboursideConfig()
	cfg.Rules.TurnLimit = *turnLimit
	profiles := []bot.Profile{bot.Balanced, bot.Aggressive, bot.Cautious}

	var (
		wins       = map[string]int{}
		totalTurns int
		totalSteps int
		maxTurns   int
		stalls     int
		failures   int
		start      = time.Now()
	)

	for g := 0; g < *games; g++ {
		gameSeed := *seed + uint64(g)*7919
		rng := game.NewSeededRNG(gameSeed)
		var seats []game.Seat
		bots := map[string]*bot.Bot{}
		for i := 0; i < *players; i++ {
			id := fmt.Sprintf("p%d", i)
			prof := profiles[i%len(profiles)]
			seats = append(seats, game.Seat{ID: id, Name: fmt.Sprintf("%s-%d", prof.Name, i), IsBot: true})
			bots[id] = bot.New(id, prof)
		}
		s, _, err := game.NewGame(cfg, seats, rng)
		if err != nil {
			fatal("new game: %v", err)
		}

		steps := 0
		var lastCmd game.Command
		for s.Turn.Phase != game.PhaseGameOver && steps < *maxSteps {
			progressed := false
			// Surrender fuzz: any active player may quit in any phase, waited on or not.
			if *surrender > 0 && rng.IntN(*surrender) == 0 {
				active := s.ActivePlayers()
				cmd := &game.Surrender{Base: game.Base{PlayerID: active[rng.IntN(len(active))].ID}}
				lastCmd = cmd
				if _, err := game.Apply(s, cmd, rng); err != nil {
					failures++
					report(g, gameSeed, s, cmd, fmt.Errorf("apply: %w", err))
					break
				}
				if err := game.CheckInvariants(s); err != nil {
					failures++
					report(g, gameSeed, s, cmd, fmt.Errorf("invariant: %w", err))
					break
				}
				steps++
				continue
			}
			for _, pid := range game.WaitingOn(s) {
				cmd := bots[pid].Decide(s)
				if cmd == nil {
					continue
				}
				lastCmd = cmd
				evs, err := game.Apply(s, cmd, rng)
				if err != nil {
					failures++
					report(g, gameSeed, s, cmd, fmt.Errorf("apply: %w", err))
					break
				}
				for _, ev := range evs {
					if _, err := game.Wrap(0, ev); err != nil {
						failures++
						report(g, gameSeed, s, cmd, fmt.Errorf("event encode: %w", err))
					}
				}
				if err := game.CheckInvariants(s); err != nil {
					failures++
					report(g, gameSeed, s, cmd, fmt.Errorf("invariant: %w", err))
					break
				}
				progressed = true
				steps++
				break // re-evaluate WaitingOn after every command
			}
			if !progressed {
				failures++
				report(g, gameSeed, s, lastCmd, fmt.Errorf("no bot could act (phase %s, waiting on %v)", s.Turn.Phase, game.WaitingOn(s)))
				break
			}
		}
		if s.Turn.Phase != game.PhaseGameOver {
			stalls++
			if *verbose {
				fmt.Printf("game %d: STALL after %d steps (turn %d, phase %s)\n", g, steps, s.Turn.Number, s.Turn.Phase)
			}
			continue
		}
		w := s.PlayerByID(s.WinnerID)
		wins[w.Name[:len(w.Name)-2]]++
		totalTurns += s.Turn.Number
		totalSteps += steps
		if s.Turn.Number > maxTurns {
			maxTurns = s.Turn.Number
		}
		if *verbose {
			fmt.Printf("game %d: winner %s after %d turns / %d commands\n", g, w.Name, s.Turn.Number, steps)
		}
	}

	finished := *games - stalls
	fmt.Printf("\n%d games, %d finished, %d stalled, %d failures in %s\n", *games, finished, stalls, failures, time.Since(start).Round(time.Millisecond))
	if finished > 0 {
		fmt.Printf("avg turns %.1f (max %d), avg commands %.1f\n", float64(totalTurns)/float64(finished), maxTurns, float64(totalSteps)/float64(finished))
	}
	var names []string
	for n := range wins {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		fmt.Printf("  %-10s %d wins\n", n, wins[n])
	}
	if failures > 0 || stalls > 0 {
		os.Exit(1)
	}
}

func report(g int, seed uint64, s *game.State, cmd game.Command, err error) {
	b, _ := json.Marshal(s)
	c, _ := json.Marshal(cmd)
	fmt.Fprintf(os.Stderr, "game %d (seed %d) FAILED: %v\n  last command: %s\n  state: %s\n", g, seed, err, c, b)
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
