package game

import "fmt"

// ---- buying & auctions ------------------------------------------------------------

func (e *engine) buyProperty(p *Player) error {
	if err := e.requireCurrent(p, PhaseResolving); err != nil {
		return err
	}
	def := e.s.cfg.Space(p.Position)
	if p.Cash < def.Price {
		return errf(ErrInsufficientFunds, "need %d to buy %s", def.Price, def.Name)
	}
	e.debit(p, def.Price, "purchase")
	e.s.Spaces[p.Position].OwnerID = p.ID
	e.emit(PropertyBought{PlayerID: p.ID, Space: def.Index, Price: def.Price})
	e.afterLanding()
	return nil
}

func (e *engine) declineBuy(p *Player) error {
	if err := e.requireCurrent(p, PhaseResolving); err != nil {
		return err
	}
	e.emit(PurchaseDeclined{PlayerID: p.ID, Space: p.Position})
	if e.rules().AuctionsEnabled {
		e.startAuction(p.Position, PhasePostRoll)
		return nil
	}
	e.afterLanding()
	return nil
}

// startAuction opens bidding on a space among all active players, starting
// with the player after the current one.
func (e *engine) startAuction(space int, resume Phase) {
	s := e.s
	var bidders []string
	n := len(s.Players)
	for i := 1; i <= n; i++ {
		pl := s.Players[(s.Turn.PlayerIdx+i)%n]
		if !pl.Bankrupt {
			bidders = append(bidders, pl.ID)
		}
	}
	if len(bidders) == 0 {
		e.setPhase(resume)
		return
	}
	s.Auction = &Auction{SpaceIndex: space, Active: bidders, TurnID: bidders[0], ResumePhase: resume}
	e.emit(AuctionStarted{Space: space, Bidders: bidders, TurnID: bidders[0]})
	e.setPhase(PhaseAuction)
}

func (e *engine) requireBidTurn(p *Player) error {
	if err := e.requirePhase(PhaseAuction); err != nil {
		return err
	}
	if e.s.Auction.TurnID != p.ID {
		return errf(ErrNotYourTurn, "it is not your bid")
	}
	return nil
}

func (e *engine) placeBid(p *Player, amount int) error {
	if err := e.requireBidTurn(p); err != nil {
		return err
	}
	a := e.s.Auction
	if amount <= a.HighBid {
		return errf(ErrInvalid, "bid must exceed %d", a.HighBid)
	}
	if amount > p.Cash {
		return errf(ErrInsufficientFunds, "bid exceeds your cash")
	}
	a.HighBid = amount
	a.HighBidderID = p.ID
	next := e.nextBidder(p.ID)
	e.emit(BidPlaced{PlayerID: p.ID, Amount: amount, NextTurnID: next})
	e.advanceAuction(next)
	return nil
}

func (e *engine) passBid(p *Player) error {
	if err := e.requireBidTurn(p); err != nil {
		return err
	}
	a := e.s.Auction
	if a.HighBidderID == p.ID {
		return errf(ErrInvalid, "the high bidder cannot pass")
	}
	next := e.nextBidder(p.ID)
	for i, id := range a.Active {
		if id == p.ID {
			a.Active = append(a.Active[:i:i], a.Active[i+1:]...)
			break
		}
	}
	e.emit(BidPassed{PlayerID: p.ID, NextTurnID: next})
	e.advanceAuction(next)
	return nil
}

// nextBidder returns the active bidder after id (skipping id if it dropped out).
func (e *engine) nextBidder(id string) string {
	a := e.s.Auction
	n := len(a.Active)
	for i, x := range a.Active {
		if x == id {
			for k := 1; k <= n; k++ {
				c := a.Active[(i+k)%n]
				if c != id {
					return c
				}
			}
		}
	}
	if n > 0 {
		return a.Active[0]
	}
	return ""
}

func (e *engine) advanceAuction(next string) {
	a := e.s.Auction
	switch {
	case len(a.Active) == 0:
		e.finishAuction("", 0)
	case len(a.Active) == 1 && a.HighBidderID == a.Active[0]:
		e.finishAuction(a.HighBidderID, a.HighBid)
	default:
		a.TurnID = next
	}
}

func (e *engine) finishAuction(winnerID string, amount int) {
	s := e.s
	a := s.Auction
	space := a.SpaceIndex
	resume := a.ResumePhase
	s.Auction = nil
	if winnerID != "" {
		w := s.PlayerByID(winnerID)
		e.debit(w, amount, "auction")
		s.Spaces[space].OwnerID = winnerID
	}
	e.emit(AuctionEnded{Space: space, WinnerID: winnerID, Amount: amount})
	if len(s.PendingAuctions) > 0 {
		next := s.PendingAuctions[0]
		s.PendingAuctions = s.PendingAuctions[1:]
		e.startAuction(next, resume)
		return
	}
	e.resume(resume)
}

// resume returns to a phase after an interruption, skipping a bankrupt
// current player.
func (e *engine) resume(phase Phase) {
	if e.s.CurrentPlayer().Bankrupt {
		e.advanceTurn()
		return
	}
	if len(e.s.Debts) > 0 {
		e.enterRaisingFunds(phase)
		return
	}
	e.setPhase(phase)
}

// ---- building ---------------------------------------------------------------------

func (e *engine) canBuild(p *Player, space int) error {
	s := e.s
	if space < 0 || space >= BoardSize {
		return errf(ErrInvalid, "no such space")
	}
	def := s.cfg.Space(space)
	st := s.Spaces[space]
	if def.Type != SpaceStreet {
		return errf(ErrInvalid, "can only build on streets")
	}
	if st.OwnerID != p.ID {
		return errf(ErrInvalid, "you do not own %s", def.Name)
	}
	if !s.OwnsGroup(p.ID, def.Group) {
		return errf(ErrInvalid, "you must own the whole %s group", def.Group)
	}
	if st.Houses >= 5 {
		return errf(ErrInvalid, "%s already has a hotel", def.Name)
	}
	for _, i := range s.cfg.GroupSpaces(def.Group) {
		if s.Spaces[i].Mortgaged {
			return errf(ErrInvalid, "cannot build while a property in the group is mortgaged")
		}
		if s.Spaces[i].Houses < st.Houses {
			return errf(ErrInvalid, "build evenly across the group")
		}
	}
	if st.Houses == 4 {
		if s.Bank.Hotels < 1 {
			return errf(ErrInvalid, "the bank has no hotels left")
		}
	} else if s.Bank.Houses < 1 {
		return errf(ErrInvalid, "the bank has no houses left")
	}
	if p.Cash < def.HouseCost {
		return errf(ErrInsufficientFunds, "need %d to build", def.HouseCost)
	}
	return nil
}

func (e *engine) buildHouse(p *Player, space int) error {
	if err := e.requireCurrent(p, PhasePreRoll, PhasePostRoll); err != nil {
		return err
	}
	if err := e.canBuild(p, space); err != nil {
		return err
	}
	s := e.s
	def := s.cfg.Space(space)
	st := &s.Spaces[space]
	e.debit(p, def.HouseCost, "build")
	st.Houses++
	if st.Houses == 5 {
		s.Bank.Hotels--
		s.Bank.Houses += 4
	} else {
		s.Bank.Houses--
	}
	e.emit(HouseBuilt{PlayerID: p.ID, Space: space, Houses: st.Houses, Cost: def.HouseCost})
	return nil
}

func (e *engine) canSell(p *Player, space int) error {
	s := e.s
	if space < 0 || space >= BoardSize {
		return errf(ErrInvalid, "no such space")
	}
	def := s.cfg.Space(space)
	st := s.Spaces[space]
	if st.OwnerID != p.ID {
		return errf(ErrInvalid, "you do not own %s", def.Name)
	}
	if st.Houses == 0 {
		return errf(ErrInvalid, "%s has no buildings", def.Name)
	}
	for _, i := range s.cfg.GroupSpaces(def.Group) {
		if s.Spaces[i].Houses > st.Houses {
			return errf(ErrInvalid, "sell evenly across the group")
		}
	}
	return nil
}

func (e *engine) sellHouse(p *Player, space int) error {
	if err := e.requireSellPhase(p); err != nil {
		return err
	}
	if err := e.canSell(p, space); err != nil {
		return err
	}
	s := e.s
	def := s.cfg.Space(space)
	st := &s.Spaces[space]
	refund := def.HouseCost * e.rules().BuildingSellPct / 100
	switch {
	case st.Houses == 5 && s.Bank.Houses < 4:
		// House shortage: the hotel cannot be broken into houses, so the
		// whole thing is sold back at once (official shortage rule).
		refund *= 5
		s.Bank.Hotels++
		st.Houses = 0
	case st.Houses == 5:
		s.Bank.Hotels++
		s.Bank.Houses -= 4
		st.Houses--
	default:
		s.Bank.Houses++
		st.Houses--
	}
	e.credit(p, refund, "sell_building")
	e.emit(HouseSold{PlayerID: p.ID, Space: space, Houses: st.Houses, Refund: refund})
	e.trySettleDebts()
	return nil
}

// requireSellPhase: the current player on their turn, or any debtor while
// raising funds.
func (e *engine) requireSellPhase(p *Player) error {
	if e.s.Turn.Phase == PhaseRaisingFunds {
		if !e.isDebtor(p.ID) {
			return errf(ErrNotYourTurn, "only debtors may act now")
		}
		return nil
	}
	return e.requireCurrent(p, PhasePreRoll, PhasePostRoll)
}

func (e *engine) isDebtor(id string) bool {
	for _, d := range e.s.Debts {
		if d.DebtorID == id {
			return true
		}
	}
	return false
}

// ---- mortgages ---------------------------------------------------------------------

func (e *engine) canMortgage(p *Player, space int) error {
	s := e.s
	if space < 0 || space >= BoardSize {
		return errf(ErrInvalid, "no such space")
	}
	def := s.cfg.Space(space)
	st := s.Spaces[space]
	if st.OwnerID != p.ID {
		return errf(ErrInvalid, "you do not own %s", def.Name)
	}
	if st.Mortgaged {
		return errf(ErrInvalid, "%s is already mortgaged", def.Name)
	}
	if def.Type == SpaceStreet && s.GroupHasBuildings(def.Group) {
		return errf(ErrInvalid, "sell all buildings in the group first")
	}
	return nil
}

func (e *engine) mortgage(p *Player, space int) error {
	if err := e.requireSellPhase(p); err != nil {
		return err
	}
	if err := e.canMortgage(p, space); err != nil {
		return err
	}
	def := e.s.cfg.Space(space)
	e.s.Spaces[space].Mortgaged = true
	e.credit(p, def.MortgageValue(), "mortgage")
	e.emit(Mortgaged{PlayerID: p.ID, Space: space, Amount: def.MortgageValue()})
	e.trySettleDebts()
	return nil
}

func (e *engine) unmortgageCost(space int) int {
	mv := e.s.cfg.Space(space).MortgageValue()
	return mv + mv*e.rules().MortgageInterestPct/100
}

func (e *engine) canUnmortgage(p *Player, space int) error {
	s := e.s
	if space < 0 || space >= BoardSize {
		return errf(ErrInvalid, "no such space")
	}
	def := s.cfg.Space(space)
	st := s.Spaces[space]
	if st.OwnerID != p.ID {
		return errf(ErrInvalid, "you do not own %s", def.Name)
	}
	if !st.Mortgaged {
		return errf(ErrInvalid, "%s is not mortgaged", def.Name)
	}
	if cost := e.unmortgageCost(space); p.Cash < cost {
		return errf(ErrInsufficientFunds, "need %d to lift the mortgage", cost)
	}
	return nil
}

func (e *engine) unmortgage(p *Player, space int) error {
	if err := e.requireCurrent(p, PhasePreRoll, PhasePostRoll); err != nil {
		return err
	}
	if err := e.canUnmortgage(p, space); err != nil {
		return err
	}
	cost := e.unmortgageCost(space)
	e.debit(p, cost, "unmortgage")
	e.s.Spaces[space].Mortgaged = false
	e.emit(Unmortgaged{PlayerID: p.ID, Space: space, Amount: cost})
	return nil
}

// ---- trades ---------------------------------------------------------------------------

func (e *engine) validateSide(owner *Player, side TradeSide) error {
	if side.Cash < 0 || side.JailCards < 0 {
		return errf(ErrInvalid, "negative amounts in trade")
	}
	if side.Cash > owner.Cash {
		return errf(ErrInsufficientFunds, "%s does not have %d cash", owner.Name, side.Cash)
	}
	if side.JailCards > len(owner.JailCards) {
		return errf(ErrInvalid, "%s does not hold that many jail cards", owner.Name)
	}
	seen := map[int]bool{}
	for _, sp := range side.Properties {
		if sp < 0 || sp >= BoardSize || seen[sp] {
			return errf(ErrInvalid, "invalid property in trade")
		}
		seen[sp] = true
		def := e.s.cfg.Space(sp)
		st := e.s.Spaces[sp]
		if st.OwnerID != owner.ID {
			return errf(ErrInvalid, "%s does not own %s", owner.Name, def.Name)
		}
		if def.Type == SpaceStreet && e.s.GroupHasBuildings(def.Group) {
			return errf(ErrInvalid, "sell buildings on the %s group before trading it", def.Group)
		}
	}
	return nil
}

// mortgageInterestDue is the immediate 10% owed by whoever receives mortgaged properties.
func (e *engine) mortgageInterestDue(props []int) int {
	total := 0
	for _, sp := range props {
		if e.s.Spaces[sp].Mortgaged {
			mv := e.s.cfg.Space(sp).MortgageValue()
			total += mv * e.rules().MortgageInterestPct / 100
		}
	}
	return total
}

func (e *engine) validateTrade(t *Trade) error {
	from := e.s.PlayerByID(t.FromID)
	to := e.s.PlayerByID(t.ToID)
	if from == nil || to == nil || from.Bankrupt || to.Bankrupt {
		return errf(ErrInvalid, "trade party is not in the game")
	}
	if from.ID == to.ID {
		return errf(ErrInvalid, "cannot trade with yourself")
	}
	if err := e.validateSide(from, t.Give); err != nil {
		return err
	}
	if err := e.validateSide(to, t.Receive); err != nil {
		return err
	}
	if t.Give.Cash == 0 && t.Receive.Cash == 0 && len(t.Give.Properties) == 0 &&
		len(t.Receive.Properties) == 0 && t.Give.JailCards == 0 && t.Receive.JailCards == 0 {
		return errf(ErrInvalid, "empty trade")
	}
	// Each side must be able to cover cash out + interest on mortgaged property in.
	if from.Cash-t.Give.Cash+t.Receive.Cash-e.mortgageInterestDue(t.Receive.Properties) < 0 {
		return errf(ErrInsufficientFunds, "%s cannot cover the mortgage interest", from.Name)
	}
	if to.Cash-t.Receive.Cash+t.Give.Cash-e.mortgageInterestDue(t.Give.Properties) < 0 {
		return errf(ErrInsufficientFunds, "%s cannot cover the mortgage interest", to.Name)
	}
	return nil
}

// ValidateTrade reports whether a proposal would be accepted by the engine
// right now (ownership, buildings, cash and mortgage interest). Bots and the
// UI use it to avoid sending doomed proposals.
func ValidateTrade(s *State, fromID, toID string, give, receive TradeSide) error {
	e := &engine{s: s}
	return e.validateTrade(&Trade{FromID: fromID, ToID: toID, Give: give, Receive: receive})
}

func (e *engine) proposeTrade(p *Player, c *ProposeTrade) error {
	if err := e.requirePhase(PhasePreRoll, PhasePostRoll, PhaseRaisingFunds); err != nil {
		return err
	}
	if e.s.Trade != nil {
		return errf(ErrInvalid, "a trade is already pending")
	}
	t := &Trade{
		FromID: p.ID, ToID: c.ToID, Give: c.Give, Receive: c.Receive, ResumePhase: e.s.Turn.Phase,
	}
	if err := e.validateTrade(t); err != nil {
		return err
	}
	e.s.NextTradeID++
	t.ID = fmt.Sprintf("t%d-%d", e.s.Turn.Number, e.s.NextTradeID)
	e.s.Trade = t
	e.emit(TradeProposed{Trade: *t})
	return nil
}

func (e *engine) acceptTrade(p *Player, id string) error {
	t := e.s.Trade
	if t == nil || t.ID != id {
		return errf(ErrInvalid, "no such pending trade")
	}
	if t.ToID != p.ID {
		return errf(ErrNotYourTurn, "only the recipient can accept")
	}
	if err := e.validateTrade(t); err != nil {
		// The world changed since the proposal (cash spent, property sold):
		// the trade is void. This is a successful command, not an error.
		e.s.Trade = nil
		e.emit(TradeRejected{TradeID: t.ID, ByID: ""})
		return nil
	}
	from := e.s.PlayerByID(t.FromID)
	to := e.s.PlayerByID(t.ToID)
	e.s.Trade = nil
	e.transferSide(from, to, t.Give)
	e.transferSide(to, from, t.Receive)
	e.emit(TradeAccepted{TradeID: t.ID})
	e.trySettleDebts()
	return nil
}

func (e *engine) transferSide(from, to *Player, side TradeSide) {
	if side.Cash > 0 {
		e.debit(from, side.Cash, "trade")
		e.credit(to, side.Cash, "trade")
	}
	for i := 0; i < side.JailCards; i++ {
		card := from.JailCards[0]
		from.JailCards = from.JailCards[1:]
		to.JailCards = append(to.JailCards, card)
	}
	for _, sp := range side.Properties {
		e.transferProperty(sp, from, to, "trade")
	}
}

// transferProperty moves ownership and charges the receiver interest on a
// mortgaged property (the "pay 10% now" option of the official rules).
func (e *engine) transferProperty(space int, from, to *Player, reason string) {
	st := &e.s.Spaces[space]
	st.OwnerID = to.ID
	e.emit(PropertyTransferred{Space: space, FromID: from.ID, ToID: to.ID, Reason: reason})
	if st.Mortgaged {
		mv := e.s.cfg.Space(space).MortgageValue()
		interest := mv * e.rules().MortgageInterestPct / 100
		if interest > 0 && to.Cash >= interest {
			e.debit(to, interest, "mortgage_interest")
		}
	}
}

func (e *engine) rejectTrade(p *Player, id string) error {
	t := e.s.Trade
	if t == nil || t.ID != id {
		return errf(ErrInvalid, "no such pending trade")
	}
	if t.ToID != p.ID && t.FromID != p.ID {
		return errf(ErrNotYourTurn, "you are not part of this trade")
	}
	e.s.Trade = nil
	e.emit(TradeRejected{TradeID: t.ID, ByID: p.ID})
	return nil
}

func (e *engine) cancelTrade(by string) {
	if t := e.s.Trade; t != nil {
		e.s.Trade = nil
		e.emit(TradeRejected{TradeID: t.ID, ByID: by})
	}
}

// ---- debts & bankruptcy -----------------------------------------------------------------

func (e *engine) enterRaisingFunds(resume Phase) {
	if e.s.Turn.Phase != PhaseRaisingFunds {
		e.s.ResumePhase = resume
		e.setPhase(PhaseRaisingFunds)
	}
}

// trySettleDebts pays every debt whose debtor can now afford it. When none
// remain, play resumes where it was interrupted.
func (e *engine) trySettleDebts() {
	s := e.s
	if s.Turn.Phase != PhaseRaisingFunds {
		return
	}
	remaining := s.Debts[:0:0]
	for _, d := range s.Debts {
		debtor := s.PlayerByID(d.DebtorID)
		if debtor.Bankrupt {
			continue
		}
		if debtor.Cash >= d.Amount {
			e.debit(debtor, d.Amount, d.Reason)
			if d.CreditorID != "" {
				e.credit(s.PlayerByID(d.CreditorID), d.Amount, d.Reason)
			} else if e.rules().FreeParkingJackpot {
				s.Bank.FreeParkingPool += d.Amount
			}
			e.emit(DebtSettled{Debt: d})
			continue
		}
		remaining = append(remaining, d)
	}
	s.Debts = remaining
	if len(s.Debts) == 0 {
		e.resumeAfterDebts()
	}
}

func (e *engine) resumeAfterDebts() {
	s := e.s
	s.Debts = nil
	resume := s.ResumePhase
	s.ResumePhase = ""
	if resume == "" {
		resume = PhasePostRoll
	}
	// Leave RaisingFunds before continuing: the continuation below may land
	// on rent and re-enter it with a fresh resume phase.
	e.setPhase(resume)
	if s.Turn.PendingJailMove {
		s.Turn.PendingJailMove = false
		p := s.CurrentPlayer()
		if !p.Bankrupt {
			e.emit(JailFinePaid{PlayerID: p.ID, Amount: e.rules().JailFine, Forced: true})
			p.InJail = false
			p.JailTurns = 0
			e.emit(LeftJail{PlayerID: p.ID, How: "fine"})
			e.moveForward(p, s.Turn.LastRoll[0]+s.Turn.LastRoll[1], false)
			e.resolveLanding(p)
			return
		}
	}
	if len(s.PendingAuctions) > 0 && len(s.ActivePlayers()) > 1 {
		next := s.PendingAuctions[0]
		s.PendingAuctions = s.PendingAuctions[1:]
		e.startAuction(next, resume)
		return
	}
	s.PendingAuctions = nil
	e.resume(resume)
}

func (e *engine) owed(id string) int {
	total := 0
	for _, d := range e.s.Debts {
		if d.DebtorID == id {
			total += d.Amount
		}
	}
	return total
}

func (e *engine) canDeclareBankruptcy(p *Player) error {
	if err := e.requirePhase(PhaseRaisingFunds); err != nil {
		return err
	}
	if !e.isDebtor(p.ID) {
		return errf(ErrInvalid, "you owe nothing")
	}
	if e.s.LiquidationValue(p.ID) >= e.owed(p.ID) {
		return errf(ErrInvalid, "you can still raise the funds by selling or mortgaging")
	}
	return nil
}

func (e *engine) declareBankruptcy(p *Player) error {
	if err := e.canDeclareBankruptcy(p); err != nil {
		return err
	}
	e.eliminate(p, "insolvent")
	if e.checkGameEnd() {
		return nil
	}
	if len(e.s.Debts) == 0 {
		e.resumeAfterDebts()
	}
	return nil
}

// surrender removes a player from the game in any phase. It settles them
// exactly like a bankruptcy (creditor first, otherwise the bank), drops them
// from a running auction, and hands the turn on if it was theirs.
func (e *engine) surrender(p *Player) error {
	s := e.s
	// Anything still owed to the leaver is owed to the bank instead: the
	// debtor stays in RaisingFunds and the bank inherits the claim along
	// with the rest of the leaver's assets.
	for i := range s.Debts {
		if s.Debts[i].CreditorID == p.ID {
			s.Debts[i].CreditorID = ""
		}
	}
	e.eliminate(p, "surrender")
	if e.checkGameEnd() {
		return nil
	}
	switch s.Turn.Phase {
	case PhaseAuction:
		// Leaving a live auction is a pass; finishing it resumes play, which
		// skips the leaver's turn and runs any auctions the surrender queued.
		e.dropBidder(p)
	case PhaseRaisingFunds:
		if len(s.Debts) == 0 {
			e.resumeAfterDebts()
		}
	default:
		if len(s.PendingAuctions) > 0 {
			// Auction the freed deeds now; resume() afterwards returns to this
			// phase or, if the leaver held the turn, passes it on.
			next := s.PendingAuctions[0]
			s.PendingAuctions = s.PendingAuctions[1:]
			e.startAuction(next, s.Turn.Phase)
			return nil
		}
		if e.isCurrent(p) {
			e.advanceTurn()
		}
	}
	return nil
}

// dropBidder removes a player from the running auction. A high bid they held
// is withdrawn, so the remaining bidders start over from nothing.
func (e *engine) dropBidder(p *Player) {
	a := e.s.Auction
	idx := -1
	for i, id := range a.Active {
		if id == p.ID {
			idx = i
		}
	}
	if idx < 0 {
		return // already passed; the auction carries on without them
	}
	next := a.TurnID
	if next == p.ID {
		next = e.nextBidder(p.ID)
	}
	a.Active = append(a.Active[:idx:idx], a.Active[idx+1:]...)
	if a.HighBidderID == p.ID {
		a.HighBid, a.HighBidderID = 0, ""
	}
	e.emit(BidPassed{PlayerID: p.ID, NextTurnID: next})
	e.advanceAuction(next)
}

// eliminate strips a player of everything and marks them bankrupt. Assets go
// to the creditor of their first outstanding debt, or to the bank (deeds are
// queued for auction when the rules allow). It does not move play on; the
// caller decides how the game continues.
func (e *engine) eliminate(p *Player, reason string) {
	s := e.s
	creditorID := ""
	for _, d := range s.Debts {
		if d.DebtorID == p.ID {
			creditorID = d.CreditorID
			break
		}
	}
	// Buildings are always sold back to the bank first. A hotel goes straight
	// back as a hotel (no 4-house swap) so the supply invariant holds.
	for _, sp := range s.OwnedBy(p.ID) {
		st := &s.Spaces[sp]
		def := s.cfg.Space(sp)
		if st.Houses == 0 {
			continue
		}
		refund := st.Houses * def.HouseCost * e.rules().BuildingSellPct / 100
		if st.Hotel() {
			s.Bank.Hotels++
		} else {
			s.Bank.Houses += st.Houses
		}
		st.Houses = 0
		e.credit(p, refund, "sell_building")
		e.emit(HouseSold{PlayerID: p.ID, Space: sp, Houses: 0, Refund: refund})
	}
	// Remove this player's debts before transferring assets.
	kept := s.Debts[:0:0]
	for _, d := range s.Debts {
		if d.DebtorID != p.ID {
			kept = append(kept, d)
		}
	}
	s.Debts = kept
	if t := s.Trade; t != nil && (t.FromID == p.ID || t.ToID == p.ID) {
		e.cancelTrade("bankruptcy")
	}

	if creditorID != "" {
		creditor := s.PlayerByID(creditorID)
		if p.Cash > 0 {
			amt := p.Cash
			e.debit(p, amt, "bankruptcy")
			e.credit(creditor, amt, "bankruptcy")
		}
		for _, sp := range s.OwnedBy(p.ID) {
			e.transferProperty(sp, p, creditor, "bankruptcy")
		}
		creditor.JailCards = append(creditor.JailCards, p.JailCards...)
	} else {
		if p.Cash > 0 {
			e.debit(p, p.Cash, "bankruptcy")
		}
		for _, sp := range s.OwnedBy(p.ID) {
			s.Spaces[sp] = SpaceState{}
			e.emit(PropertyTransferred{Space: sp, FromID: p.ID, Reason: "bankruptcy"})
			if e.rules().AuctionsEnabled {
				s.PendingAuctions = append(s.PendingAuctions, sp)
			}
		}
		for _, id := range p.JailCards {
			e.returnCard(id)
		}
	}
	p.JailCards = []string{}
	p.Bankrupt = true
	p.InJail = false
	e.emit(PlayerBankrupt{PlayerID: p.ID, CreditorID: creditorID, Reason: reason})
}
