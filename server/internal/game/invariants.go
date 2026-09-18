package game

import "fmt"

// CheckInvariants verifies structural properties that must hold after every
// command. The simulator runs it after each step; tests run it at checkpoints.
func CheckInvariants(s *State) error {
	cfg := s.cfg
	if cfg == nil {
		return fmt.Errorf("no config bound")
	}
	houses, hotels := 0, 0
	cards := len(s.Decks.Chance) + len(s.Decks.CommunityChest)
	for i, st := range s.Spaces {
		if st.Houses < 0 || st.Houses > 5 {
			return fmt.Errorf("space %d has %d houses", i, st.Houses)
		}
		if st.Houses == 5 {
			hotels++
		} else {
			houses += st.Houses
		}
		if st.OwnerID != "" {
			p := s.PlayerByID(st.OwnerID)
			if p == nil || p.Bankrupt {
				return fmt.Errorf("space %d owned by missing/bankrupt player %q", i, st.OwnerID)
			}
			if !cfg.Spaces[i].Ownable() {
				return fmt.Errorf("space %d is not ownable but has owner", i)
			}
		}
		if st.Houses > 0 && (st.OwnerID == "" || st.Mortgaged) {
			return fmt.Errorf("space %d has buildings but is unowned/mortgaged", i)
		}
	}
	if houses+s.Bank.Houses != cfg.Rules.HouseSupply {
		return fmt.Errorf("house supply mismatch: board=%d bank=%d supply=%d", houses, s.Bank.Houses, cfg.Rules.HouseSupply)
	}
	if hotels+s.Bank.Hotels != cfg.Rules.HotelSupply {
		return fmt.Errorf("hotel supply mismatch: board=%d bank=%d supply=%d", hotels, s.Bank.Hotels, cfg.Rules.HotelSupply)
	}
	active := 0
	for _, p := range s.Players {
		if p.Cash < 0 {
			return fmt.Errorf("player %s has negative cash %d", p.ID, p.Cash)
		}
		if p.Position < 0 || p.Position >= BoardSize {
			return fmt.Errorf("player %s off board at %d", p.ID, p.Position)
		}
		cards += len(p.JailCards)
		if p.Bankrupt {
			if p.Cash != 0 || len(s.OwnedBy(p.ID)) != 0 {
				return fmt.Errorf("bankrupt player %s still holds assets", p.ID)
			}
		} else {
			active++
		}
	}
	if total := len(cfg.Chance) + len(cfg.CommunityChest); cards != total {
		return fmt.Errorf("card count mismatch: %d in play, %d defined", cards, total)
	}
	switch s.Turn.Phase {
	case PhaseGameOver:
		if s.WinnerID == "" {
			return fmt.Errorf("game over without winner")
		}
	default:
		if active < 2 {
			return fmt.Errorf("%d active players but game not over", active)
		}
		if s.CurrentPlayer().Bankrupt && s.Turn.Phase != PhaseAuction && s.Turn.Phase != PhaseRaisingFunds {
			return fmt.Errorf("bankrupt player has the turn in phase %s", s.Turn.Phase)
		}
	}
	if (s.Auction != nil) != (s.Turn.Phase == PhaseAuction) {
		return fmt.Errorf("auction/phase mismatch: auction=%v phase=%s", s.Auction != nil, s.Turn.Phase)
	}
	if (len(s.Debts) > 0) != (s.Turn.Phase == PhaseRaisingFunds) {
		return fmt.Errorf("debts/phase mismatch: debts=%d phase=%s", len(s.Debts), s.Turn.Phase)
	}
	if s.Turn.Phase != PhaseGameOver && len(WaitingOn(s)) == 0 {
		return fmt.Errorf("nobody to act in phase %s", s.Turn.Phase)
	}
	return nil
}
