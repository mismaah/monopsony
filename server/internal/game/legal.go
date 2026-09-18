package game

// Action is one thing a player may do right now. The UI renders these as
// buttons; bots pick from them. Space/MinBid/TradeID are filled where relevant.
type Action struct {
	Type    string `json:"type"`
	Space   int    `json:"space,omitempty"`
	MinBid  int    `json:"minBid,omitempty"`
	TradeID string `json:"tradeId,omitempty"`
}

// LegalActions enumerates the commands the engine would currently accept from
// a player. It never mutates state.
func LegalActions(s *State, playerID string) []Action {
	p := s.PlayerByID(playerID)
	if p == nil || p.Bankrupt || s.Turn.Phase == PhaseGameOver || s.cfg == nil {
		return nil
	}
	e := &engine{s: s, rng: nil}
	var out []Action
	add := func(a Action) { out = append(out, a) }
	current := e.isCurrent(p)

	switch s.Turn.Phase {
	case PhasePreRoll:
		if current {
			if p.InJail {
				if p.Cash >= s.cfg.Rules.JailFine {
					add(Action{Type: "PayJailFine"})
				}
				if len(p.JailCards) > 0 {
					add(Action{Type: "UseJailCard"})
				}
			}
			add(Action{Type: "RollDice"})
			out = append(out, e.propertyActions(p, true)...)
		}
	case PhaseResolving:
		if current {
			if p.Cash >= s.cfg.Space(p.Position).Price {
				add(Action{Type: "BuyProperty"})
			}
			add(Action{Type: "DeclineBuy"})
		}
	case PhaseAuction:
		if a := s.Auction; a != nil && a.TurnID == p.ID {
			if p.Cash > a.HighBid {
				add(Action{Type: "PlaceBid", MinBid: a.HighBid + 1})
			}
			if a.HighBidderID != p.ID {
				add(Action{Type: "PassBid"})
			}
		}
	case PhasePostRoll:
		if current {
			add(Action{Type: "EndTurn"})
			out = append(out, e.propertyActions(p, true)...)
		}
	case PhaseRaisingFunds:
		if e.isDebtor(p.ID) {
			out = append(out, e.propertyActions(p, false)...)
			if e.canDeclareBankruptcy(p) == nil {
				add(Action{Type: "DeclareBankruptcy"})
			}
		}
	}

	// Trades
	if s.Trade == nil && len(s.ActivePlayers()) > 1 {
		switch s.Turn.Phase {
		case PhasePreRoll, PhasePostRoll:
			add(Action{Type: "ProposeTrade"})
		case PhaseRaisingFunds:
			if e.isDebtor(p.ID) {
				add(Action{Type: "ProposeTrade"})
			}
		}
	}
	if t := s.Trade; t != nil {
		if t.ToID == p.ID {
			add(Action{Type: "AcceptTrade", TradeID: t.ID})
			add(Action{Type: "RejectTrade", TradeID: t.ID})
		} else if t.FromID == p.ID {
			add(Action{Type: "RejectTrade", TradeID: t.ID})
		}
	}
	return out
}

// propertyActions lists build/sell/mortgage/unmortgage moves. full=false is the
// raising-funds subset (sell and mortgage only).
func (e *engine) propertyActions(p *Player, full bool) []Action {
	var out []Action
	for _, sp := range e.s.OwnedBy(p.ID) {
		if full && e.canBuild(p, sp) == nil {
			out = append(out, Action{Type: "BuildHouse", Space: sp})
		}
		if e.canSell(p, sp) == nil {
			out = append(out, Action{Type: "SellHouse", Space: sp})
		}
		if e.canMortgage(p, sp) == nil {
			out = append(out, Action{Type: "Mortgage", Space: sp})
		}
		if full && e.canUnmortgage(p, sp) == nil {
			out = append(out, Action{Type: "Unmortgage", Space: sp})
		}
	}
	return out
}

// WaitingOn returns the IDs of players whose input the game is waiting for.
// A pending trade's recipient is listed first so they answer before play
// moves on.
func WaitingOn(s *State) []string {
	if s.Turn.Phase == PhaseGameOver {
		return nil
	}
	var out []string
	if t := s.Trade; t != nil {
		out = append(out, t.ToID)
	}
	switch s.Turn.Phase {
	case PhaseAuction:
		if s.Auction != nil {
			out = appendUnique(out, s.Auction.TurnID)
		}
	case PhaseRaisingFunds:
		for _, d := range s.Debts {
			out = appendUnique(out, d.DebtorID)
		}
	default:
		out = appendUnique(out, s.CurrentPlayer().ID)
	}
	return out
}

func appendUnique(xs []string, x string) []string {
	for _, y := range xs {
		if y == x {
			return xs
		}
	}
	return append(xs, x)
}
