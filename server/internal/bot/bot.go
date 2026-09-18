// Package bot is a heuristic AI player. It only ever picks from the engine's
// LegalActions, so it can never issue an illegal command, and the same code
// doubles as the "safe default" the room uses when a human times out.
package bot

import (
	"monopsony/server/internal/game"
)

// Profile tunes a bot's behaviour.
type Profile struct {
	Name       string
	Reserve    int     // cash the bot tries to keep on hand
	Aggression float64 // multiplier on property valuations when bidding/buying
}

// Presets.
var (
	Cautious   = Profile{Name: "cautious", Reserve: 400, Aggression: 0.9}
	Balanced   = Profile{Name: "balanced", Reserve: 250, Aggression: 1.1}
	Aggressive = Profile{Name: "aggressive", Reserve: 100, Aggression: 1.35}
)

// Bot decides commands for one seat.
type Bot struct {
	PlayerID string
	Profile  Profile

	lastTradeTurn int // throttle: at most one proposal per turn
}

// New returns a bot for a seat.
func New(playerID string, p Profile) *Bot { return &Bot{PlayerID: playerID, Profile: p} }

// Decide returns the next command for this bot, or nil if it has nothing to do.
func (b *Bot) Decide(s *game.State) game.Command {
	acts := game.LegalActions(s, b.PlayerID)
	if len(acts) == 0 {
		return nil
	}
	has := map[string][]game.Action{}
	for _, a := range acts {
		has[a.Type] = append(has[a.Type], a)
	}
	p := s.PlayerByID(b.PlayerID)
	base := game.Base{PlayerID: b.PlayerID}

	// Incoming trade: evaluate first, regardless of phase.
	if t := s.Trade; t != nil && t.ToID == b.PlayerID {
		if b.tradeGain(s, t) > 0 {
			return &game.AcceptTrade{Base: base, TradeID: t.ID}
		}
		return &game.RejectTrade{Base: base, TradeID: t.ID}
	}

	switch s.Turn.Phase {
	case game.PhaseRaisingFunds:
		return b.raiseFunds(s, has, base)
	case game.PhaseAuction:
		return b.bid(s, p, has, base)
	case game.PhaseResolving:
		if _, ok := has["BuyProperty"]; ok && b.wantsToBuy(s, p, p.Position, s.Config().Space(p.Position).Price) {
			return &game.BuyProperty{Base: base}
		}
		if _, ok := has["DeclineBuy"]; ok {
			return &game.DeclineBuy{Base: base}
		}
	case game.PhasePreRoll, game.PhasePostRoll:
		if cmd := b.improve(s, p, has, base); cmd != nil {
			return cmd
		}
		if _, ok := has["ProposeTrade"]; ok && s.Turn.Number != b.lastTradeTurn {
			b.lastTradeTurn = s.Turn.Number
			if cmd := b.proposeTrade(s, p, base); cmd != nil {
				return cmd
			}
		}
		if s.Turn.Phase == game.PhasePreRoll {
			if _, ok := has["UseJailCard"]; ok {
				return &game.UseJailCard{Base: base}
			}
			if _, ok := has["PayJailFine"]; ok && b.shouldPayJailFine(s, p) {
				return &game.PayJailFine{Base: base}
			}
			if _, ok := has["RollDice"]; ok {
				return &game.RollDice{Base: base}
			}
		}
		if _, ok := has["EndTurn"]; ok {
			return &game.EndTurn{Base: base}
		}
	}
	return nil
}

// SafeDefault is the action taken for a human who timed out: never spends money.
func SafeDefault(s *game.State, playerID string) game.Command {
	base := game.Base{PlayerID: playerID}
	for _, a := range game.LegalActions(s, playerID) {
		switch a.Type {
		case "DeclineBuy":
			return &game.DeclineBuy{Base: base}
		case "PassBid":
			return &game.PassBid{Base: base}
		case "RollDice":
			return &game.RollDice{Base: base}
		case "EndTurn":
			return &game.EndTurn{Base: base}
		case "RejectTrade":
			return &game.RejectTrade{Base: base, TradeID: a.TradeID}
		}
	}
	// Raising funds: let the bot liquidate sensibly rather than stall.
	if s.Turn.Phase == game.PhaseRaisingFunds {
		return New(playerID, Cautious).Decide(s)
	}
	return nil
}

// ---- valuation ----------------------------------------------------------------

// value estimates what a space is worth to this bot.
func (b *Bot) value(s *game.State, space int) float64 {
	def := s.Config().Space(space)
	v := float64(def.Price)
	if def.Group == "" {
		return v
	}
	group := s.Config().GroupSpaces(def.Group)
	mine, theirs := 0, 0
	for _, i := range group {
		switch owner := s.Spaces[i].OwnerID; {
		case owner == b.PlayerID:
			mine++
		case owner != "" && i != space:
			theirs++
		}
	}
	switch {
	case mine == len(group)-1: // completes a monopoly
		v *= 1.6
	case theirs == len(group)-1: // blocks someone else's
		v *= 1.25
	case mine > 0:
		v *= 1.15
	}
	if def.Type == game.SpaceUtility {
		v *= 0.8
	}
	return v * b.Profile.Aggression
}

func (b *Bot) wantsToBuy(s *game.State, p *game.Player, space, price int) bool {
	if p.Cash < price {
		return false
	}
	v := b.value(s, space)
	if v >= float64(price)*1.5 { // monopoly-completing: buy even below reserve
		return true
	}
	return p.Cash-price >= b.Profile.Reserve && v >= float64(price)
}

func (b *Bot) bid(s *game.State, p *game.Player, has map[string][]game.Action, base game.Base) game.Command {
	a := s.Auction
	if a == nil {
		return nil
	}
	if bids, ok := has["PlaceBid"]; ok {
		limit := int(b.value(s, a.SpaceIndex))
		if limit > p.Cash-b.Profile.Reserve/2 {
			limit = p.Cash - b.Profile.Reserve/2
		}
		min := bids[0].MinBid
		if min <= limit {
			// raise by ~10% of price, never above our limit
			inc := s.Config().Space(a.SpaceIndex).Price / 10
			if inc < 1 {
				inc = 1
			}
			amt := a.HighBid + inc
			if amt < min {
				amt = min
			}
			if amt > limit {
				amt = limit
			}
			return &game.PlaceBid{Base: base, Amount: amt}
		}
	}
	if _, ok := has["PassBid"]; ok {
		return &game.PassBid{Base: base}
	}
	return nil
}

// improve spends spare cash on houses (best rent-per-cost first) and lifting mortgages.
func (b *Bot) improve(s *game.State, p *game.Player, has map[string][]game.Action, base game.Base) game.Command {
	cfg := s.Config()
	bestSpace, bestScore := -1, 0.0
	for _, a := range has["BuildHouse"] {
		def := cfg.Space(a.Space)
		if p.Cash-def.HouseCost < b.Profile.Reserve {
			continue
		}
		h := s.Spaces[a.Space].Houses
		gain := float64(def.Rent[h+1]-def.Rent[h]) / float64(def.HouseCost)
		if gain > bestScore {
			bestScore, bestSpace = gain, a.Space
		}
	}
	if bestSpace >= 0 {
		return &game.BuildHouse{Base: base, Space: bestSpace}
	}
	for _, a := range has["Unmortgage"] {
		def := cfg.Space(a.Space)
		cost := def.MortgageValue() + def.MortgageValue()*cfg.Rules.MortgageInterestPct/100
		if p.Cash-cost >= b.Profile.Reserve*2 {
			return &game.Unmortgage{Base: base, Space: a.Space}
		}
	}
	return nil
}

func (b *Bot) shouldPayJailFine(s *game.State, p *game.Player) bool {
	// Early game: get out and buy property. Late game (lots owned): sit tight.
	owned := 0
	for _, st := range s.Spaces {
		if st.OwnerID != "" {
			owned++
		}
	}
	return owned < 20 && p.Cash-s.Config().Rules.JailFine >= b.Profile.Reserve
}

// raiseFunds sells the least valuable things first: houses on the cheapest
// group, then mortgages of non-monopoly properties, then anything.
func (b *Bot) raiseFunds(s *game.State, has map[string][]game.Action, base game.Base) game.Command {
	cfg := s.Config()
	if sells := has["SellHouse"]; len(sells) > 0 {
		best := sells[0]
		for _, a := range sells[1:] {
			if cfg.Space(a.Space).HouseCost < cfg.Space(best.Space).HouseCost {
				best = a
			}
		}
		return &game.SellHouse{Base: base, Space: best.Space}
	}
	if ms := has["Mortgage"]; len(ms) > 0 {
		best, bestScore := ms[0], 1e9
		for _, a := range ms {
			def := cfg.Space(a.Space)
			score := float64(def.Price)
			if def.Group != "" && s.OwnsGroup(b.PlayerID, def.Group) {
				score *= 3 // keep monopolies as long as possible
			}
			if score < bestScore {
				best, bestScore = a, score
			}
		}
		return &game.Mortgage{Base: base, Space: best.Space}
	}
	if _, ok := has["DeclareBankruptcy"]; ok {
		return &game.DeclareBankruptcy{Base: base}
	}
	return nil
}

// tradeGain is (value in) - (value out) from this bot's perspective.
func (b *Bot) tradeGain(s *game.State, t *game.Trade) float64 {
	in, out := float64(t.Give.Cash), float64(t.Receive.Cash) // Give is what the proposer gives us
	for _, sp := range t.Give.Properties {
		in += b.value(s, sp)
	}
	for _, sp := range t.Receive.Properties {
		out += b.value(s, sp) * 1.2 // we value what we hold a bit more
	}
	in += float64(t.Give.JailCards) * 50
	out += float64(t.Receive.JailCards) * 50
	return in - out
}

// proposeTrade looks for a colour group where this bot holds all but one
// street and offers the holder cash (plus a sweetener property they are
// collecting) for it.
func (b *Bot) proposeTrade(s *game.State, p *game.Player, base game.Base) game.Command {
	cfg := s.Config()
	seen := map[string]bool{}
	for _, def := range cfg.Spaces {
		if def.Type != game.SpaceStreet || seen[def.Group] {
			continue
		}
		seen[def.Group] = true
		group := cfg.GroupSpaces(def.Group)
		missing, holder := -1, ""
		mine := 0
		for _, i := range group {
			switch owner := s.Spaces[i].OwnerID; owner {
			case b.PlayerID:
				mine++
			case "":
				missing = -2 // still unowned; wait for it
			default:
				if missing == -1 {
					missing, holder = i, owner
				} else {
					missing = -2
				}
			}
		}
		if missing < 0 || mine != len(group)-1 {
			continue
		}
		if s.Spaces[missing].Houses > 0 {
			continue
		}
		other := s.PlayerByID(holder)
		if other == nil || other.Bankrupt {
			continue
		}
		price := cfg.Space(missing).Price
		offer := int(float64(price) * 1.6 * b.Profile.Aggression)
		if offer > p.Cash-b.Profile.Reserve {
			offer = p.Cash - b.Profile.Reserve
		}
		give := game.TradeSide{Cash: offer}
		// Sweetener: a street of ours in a group where the holder owns the rest.
		if sweet := b.sweetener(s, holder); sweet >= 0 {
			give.Properties = []int{sweet}
		}
		if give.Cash < price && len(give.Properties) == 0 {
			continue
		}
		if give.Cash < 0 {
			give.Cash = 0
		}
		receive := game.TradeSide{Properties: []int{missing}}
		if game.ValidateTrade(s, b.PlayerID, holder, give, receive) != nil {
			continue
		}
		return &game.ProposeTrade{Base: base, ToID: holder, Give: give, Receive: receive}
	}
	return nil
}

// sweetener finds a street we own alone in a group where `holder` owns every
// other street, and which carries no buildings.
func (b *Bot) sweetener(s *game.State, holder string) int {
	cfg := s.Config()
	for _, i := range s.OwnedBy(b.PlayerID) {
		def := cfg.Space(i)
		if def.Type != game.SpaceStreet || s.Spaces[i].Houses > 0 {
			continue
		}
		ok := true
		for _, j := range cfg.GroupSpaces(def.Group) {
			if j != i && s.Spaces[j].OwnerID != holder {
				ok = false
				break
			}
		}
		if ok {
			return i
		}
	}
	return -1
}
