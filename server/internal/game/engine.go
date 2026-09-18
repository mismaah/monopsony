package game

import (
	"fmt"
)

// Error is a rule violation reported back to the acting player. The state is
// left untouched when Apply returns an Error.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func errf(code, format string, args ...any) error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

const (
	ErrNotYourTurn       = "not_your_turn"
	ErrWrongPhase        = "wrong_phase"
	ErrInvalid           = "invalid"
	ErrInsufficientFunds = "insufficient_funds"
	ErrGameOver          = "game_over"
	ErrUnknownPlayer     = "unknown_player"
)

// Seat describes a participant when a game is created.
type Seat struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	IsBot bool   `json:"isBot"`
}

const (
	MinPlayers = 2
	MaxPlayers = 8
)

// NewGame builds the initial state and emits the opening events.
func NewGame(cfg *Config, seats []Seat, rng RNG) (*State, []Event, error) {
	if err := cfg.Validate(); err != nil {
		return nil, nil, err
	}
	if len(seats) < MinPlayers || len(seats) > MaxPlayers {
		return nil, nil, errf(ErrInvalid, "need %d-%d players, got %d", MinPlayers, MaxPlayers, len(seats))
	}
	seen := map[string]bool{}
	s := &State{ConfigID: cfg.ID, cfg: cfg}
	for _, seat := range seats {
		if seat.ID == "" || seen[seat.ID] {
			return nil, nil, errf(ErrInvalid, "player id %q missing or duplicated", seat.ID)
		}
		seen[seat.ID] = true
		s.Players = append(s.Players, Player{
			ID: seat.ID, Name: seat.Name, IsBot: seat.IsBot,
			Cash: cfg.Rules.StartCash, JailCards: []string{},
		})
	}
	s.Spaces = make([]SpaceState, BoardSize)
	s.Bank = Bank{Houses: cfg.Rules.HouseSupply, Hotels: cfg.Rules.HotelSupply}
	for _, c := range cfg.Chance {
		s.Decks.Chance = append(s.Decks.Chance, c.ID)
	}
	for _, c := range cfg.CommunityChest {
		s.Decks.CommunityChest = append(s.Decks.CommunityChest, c.ID)
	}
	shuffle(rng, s.Decks.Chance)
	shuffle(rng, s.Decks.CommunityChest)
	s.Turn = Turn{Number: 1, PlayerIdx: 0, Phase: PhasePreRoll}

	e := &engine{s: s, rng: rng}
	e.emit(GameStarted{ConfigID: cfg.ID, Players: append([]Player(nil), s.Players...)})
	e.emit(TurnChanged{PlayerID: s.Players[0].ID, Number: 1})
	e.emit(PhaseChanged{Phase: PhasePreRoll})
	return s, e.events, nil
}

// Apply validates and executes a command, returning the events it produced.
// On error the state is unchanged.
func Apply(s *State, cmd Command, rng RNG) ([]Event, error) {
	if s.cfg == nil {
		return nil, errf(ErrInvalid, "state has no config bound")
	}
	if s.Turn.Phase == PhaseGameOver {
		return nil, errf(ErrGameOver, "the game is over")
	}
	p := s.PlayerByID(cmd.Actor())
	if p == nil {
		return nil, errf(ErrUnknownPlayer, "unknown player %q", cmd.Actor())
	}
	if p.Bankrupt {
		return nil, errf(ErrInvalid, "player is bankrupt")
	}
	// A pending trade pauses the game: nothing else may happen until the
	// recipient answers or the proposer withdraws it. Surrendering is the
	// one exception: a player may always leave.
	if s.Trade != nil {
		switch cmd.(type) {
		case *AcceptTrade, *RejectTrade, *Surrender:
		default:
			return nil, errf(ErrInvalid, "a trade is pending")
		}
	}
	e := &engine{s: s, rng: rng}
	var err error
	switch c := cmd.(type) {
	case *RollDice:
		err = e.rollDice(p)
	case *BuyProperty:
		err = e.buyProperty(p)
	case *DeclineBuy:
		err = e.declineBuy(p)
	case *PlaceBid:
		err = e.placeBid(p, c.Amount)
	case *PassBid:
		err = e.passBid(p)
	case *EndTurn:
		err = e.endTurn(p)
	case *BuildHouse:
		err = e.buildHouse(p, c.Space)
	case *SellHouse:
		err = e.sellHouse(p, c.Space)
	case *Mortgage:
		err = e.mortgage(p, c.Space)
	case *Unmortgage:
		err = e.unmortgage(p, c.Space)
	case *ProposeTrade:
		err = e.proposeTrade(p, c)
	case *AcceptTrade:
		err = e.acceptTrade(p, c.TradeID)
	case *RejectTrade:
		err = e.rejectTrade(p, c.TradeID)
	case *PayJailFine:
		err = e.payJailFine(p)
	case *UseJailCard:
		err = e.useJailCard(p)
	case *DeclareBankruptcy:
		err = e.declareBankruptcy(p)
	case *Surrender:
		err = e.surrender(p)
	default:
		err = errf(ErrInvalid, "unsupported command %T", cmd)
	}
	if err != nil {
		return nil, err
	}
	return e.events, nil
}

// engine wraps a State for the duration of one Apply call and collects events.
type engine struct {
	s      *State
	rng    RNG
	events []Event
}

func (e *engine) emit(ev Event) {
	e.s.Seq++
	e.events = append(e.events, ev)
}

func (e *engine) rules() Rules { return e.s.cfg.Rules }

func (e *engine) setPhase(p Phase) {
	if e.s.Turn.Phase != p {
		e.s.Turn.Phase = p
		e.emit(PhaseChanged{Phase: p})
	}
}

func (e *engine) isCurrent(p *Player) bool { return e.s.Turn.PlayerIdx == e.s.PlayerIndex(p.ID) }

func (e *engine) requireCurrent(p *Player, phases ...Phase) error {
	if !e.isCurrent(p) {
		return errf(ErrNotYourTurn, "it is not your turn")
	}
	return e.requirePhase(phases...)
}

func (e *engine) requirePhase(phases ...Phase) error {
	for _, ph := range phases {
		if e.s.Turn.Phase == ph {
			return nil
		}
	}
	return errf(ErrWrongPhase, "not allowed during %s", e.s.Turn.Phase)
}

// ---- money -----------------------------------------------------------------

func (e *engine) credit(p *Player, amt int, reason string) {
	if amt == 0 {
		return
	}
	p.Cash += amt
	e.emit(CashChanged{PlayerID: p.ID, Delta: amt, Balance: p.Cash, Reason: reason})
}

func (e *engine) debit(p *Player, amt int, reason string) {
	if amt == 0 {
		return
	}
	p.Cash -= amt
	e.emit(CashChanged{PlayerID: p.ID, Delta: -amt, Balance: p.Cash, Reason: reason})
}

// charge moves amt from p to creditor ("" = bank). If p cannot pay in full a
// Debt is recorded and false is returned; nothing is transferred.
func (e *engine) charge(p *Player, amt int, creditorID, reason string) bool {
	if amt <= 0 {
		return true
	}
	if p.Cash >= amt {
		e.debit(p, amt, reason)
		if creditorID != "" {
			e.credit(e.s.PlayerByID(creditorID), amt, reason)
		} else if e.rules().FreeParkingJackpot {
			e.s.Bank.FreeParkingPool += amt
		}
		return true
	}
	d := Debt{DebtorID: p.ID, CreditorID: creditorID, Amount: amt, Reason: reason}
	e.s.Debts = append(e.s.Debts, d)
	e.emit(DebtIncurred{Debt: d})
	return false
}

// ---- turn flow ----------------------------------------------------------------

func (e *engine) rollDice(p *Player) error {
	if err := e.requireCurrent(p, PhasePreRoll); err != nil {
		return err
	}
	s := e.s
	dice := rollDice(e.rng)
	total := dice[0] + dice[1]
	doubles := dice[0] == dice[1]
	s.Turn.LastRoll = dice
	s.Turn.Rolled = true
	s.Turn.CanRollAgain = false
	s.Turn.SpecialRent = false

	if p.InJail {
		e.emit(DiceRolled{PlayerID: p.ID, Dice: dice, Doubles: doubles, DoublesCount: p.DoublesCount, InJail: true})
		if doubles {
			p.InJail = false
			p.JailTurns = 0
			e.emit(LeftJail{PlayerID: p.ID, How: "doubles"})
			e.moveForward(p, total, false)
			e.resolveLanding(p)
			return nil
		}
		p.JailTurns++
		if p.JailTurns >= e.rules().MaxJailTurns {
			fine := e.rules().JailFine
			if e.charge(p, fine, "", "jail_fine") {
				e.emit(JailFinePaid{PlayerID: p.ID, Amount: fine, Forced: true})
				p.InJail = false
				p.JailTurns = 0
				e.emit(LeftJail{PlayerID: p.ID, How: "fine"})
				e.moveForward(p, total, false)
				e.resolveLanding(p)
				return nil
			}
			s.Turn.PendingJailMove = true
			e.enterRaisingFunds(PhasePostRoll)
			return nil
		}
		e.emit(JailTurnServed{PlayerID: p.ID, Turns: p.JailTurns})
		e.setPhase(PhasePostRoll)
		return nil
	}

	if doubles {
		p.DoublesCount++
	}
	e.emit(DiceRolled{PlayerID: p.ID, Dice: dice, Doubles: doubles, DoublesCount: p.DoublesCount})
	if doubles && p.DoublesCount >= e.rules().MaxDoubles {
		e.sendToJail(p, "doubles")
		e.afterLanding()
		return nil
	}
	if doubles {
		s.Turn.CanRollAgain = true
	}
	e.moveForward(p, total, false)
	e.resolveLanding(p)
	return nil
}

func (e *engine) endTurn(p *Player) error {
	if err := e.requireCurrent(p, PhasePostRoll); err != nil {
		return err
	}
	if e.s.Turn.CanRollAgain && !p.InJail {
		e.s.Turn.CanRollAgain = false
		e.s.Turn.Rolled = false
		e.setPhase(PhasePreRoll)
		return nil
	}
	e.advanceTurn()
	return nil
}

// advanceTurn passes play to the next non-bankrupt player.
func (e *engine) advanceTurn() {
	s := e.s
	s.CurrentPlayer().DoublesCount = 0
	e.cancelTrade("turn_ended")
	if e.checkGameEnd() {
		return
	}
	n := len(s.Players)
	idx := s.Turn.PlayerIdx
	for i := 1; i <= n; i++ {
		j := (idx + i) % n
		if !s.Players[j].Bankrupt {
			idx = j
			break
		}
	}
	s.Turn = Turn{Number: s.Turn.Number + 1, PlayerIdx: idx, Phase: s.Turn.Phase}
	e.emit(TurnChanged{PlayerID: s.Players[idx].ID, Number: s.Turn.Number})
	if lim := e.rules().TurnLimit; lim > 0 && s.Turn.Number > lim {
		e.endGame(e.richest(), "turn_limit")
		return
	}
	e.setPhase(PhasePreRoll)
}

func (e *engine) richest() string {
	best, bestID := -1, ""
	for _, p := range e.s.ActivePlayers() {
		if nw := e.s.NetWorth(p.ID); nw > best {
			best, bestID = nw, p.ID
		}
	}
	return bestID
}

func (e *engine) checkGameEnd() bool {
	active := e.s.ActivePlayers()
	if len(active) == 1 {
		e.endGame(active[0].ID, "last_standing")
		return true
	}
	return false
}

func (e *engine) endGame(winnerID, reason string) {
	s := e.s
	s.WinnerID = winnerID
	s.Auction = nil
	s.Trade = nil
	s.Debts = nil
	s.PendingAuctions = nil
	e.setPhase(PhaseGameOver)
	e.emit(GameEnded{WinnerID: winnerID, Reason: reason})
}

// ---- movement -------------------------------------------------------------------

func (e *engine) moveForward(p *Player, steps int, viaCard bool) {
	from := p.Position
	to := (from + steps) % BoardSize
	passedGo := to < from || steps >= BoardSize
	p.Position = to
	e.emit(TokenMoved{PlayerID: p.ID, From: from, To: to, PassedGo: passedGo, ViaCard: viaCard})
	if passedGo {
		e.collectSalary(p)
	}
}

func (e *engine) moveTo(p *Player, target int, viaCard bool) {
	steps := (target - p.Position + BoardSize) % BoardSize
	if steps == 0 {
		steps = BoardSize // lapping the board (e.g. "Advance to Go" while on Go)
	}
	e.moveForward(p, steps, viaCard)
}

func (e *engine) moveBack(p *Player, steps int) {
	from := p.Position
	to := ((from-steps)%BoardSize + BoardSize) % BoardSize
	p.Position = to
	e.emit(TokenMoved{PlayerID: p.ID, From: from, To: to, ViaCard: true, Backward: true})
}

func (e *engine) collectSalary(p *Player) {
	amt := e.rules().GoSalary
	if amt > 0 {
		e.credit(p, amt, "salary")
		e.emit(SalaryCollected{PlayerID: p.ID, Amount: amt})
	}
}

func (e *engine) sendToJail(p *Player, reason string) {
	jail := e.s.cfg.FindSpace(SpaceJail)
	from := p.Position
	p.Position = jail
	p.InJail = true
	p.JailTurns = 0
	p.DoublesCount = 0
	e.s.Turn.CanRollAgain = false
	e.emit(SentToJail{PlayerID: p.ID, Reason: reason})
	e.emit(TokenMoved{PlayerID: p.ID, From: from, To: jail, ViaCard: true})
}

// ---- landing ----------------------------------------------------------------------

// resolveLanding applies the effect of the space the player now stands on.
func (e *engine) resolveLanding(p *Player) {
	s := e.s
	def := s.cfg.Space(p.Position)
	st := &s.Spaces[p.Position]
	switch {
	case def.Ownable() && st.OwnerID == "":
		e.emit(PurchaseOffered{PlayerID: p.ID, Space: def.Index, Price: def.Price})
		e.setPhase(PhaseResolving)
		return
	case def.Ownable() && st.OwnerID != p.ID && !st.Mortgaged:
		owner := s.PlayerByID(st.OwnerID)
		if e.rules().NoRentInJail && owner.InJail {
			break
		}
		rent := e.rentFor(def.Index, p)
		if e.charge(p, rent, owner.ID, "rent") {
			e.emit(RentPaid{PayerID: p.ID, OwnerID: owner.ID, Space: def.Index, Amount: rent})
		}
	case def.Type == SpaceTax:
		if e.charge(p, def.TaxAmount, "", "tax") {
			e.emit(TaxPaid{PlayerID: p.ID, Space: def.Index, Amount: def.TaxAmount})
		}
	case def.Type == SpaceChance:
		e.drawCard(p, "chance")
		return // drawCard finishes the landing itself
	case def.Type == SpaceCommunityChest:
		e.drawCard(p, "community_chest")
		return
	case def.Type == SpaceGoToJail:
		e.sendToJail(p, "space")
	case def.Type == SpaceGo:
		if e.rules().DoubleSalaryOnGoLanding {
			e.collectSalary(p)
		}
	case def.Type == SpaceFreeParking:
		if e.rules().FreeParkingJackpot && s.Bank.FreeParkingPool > 0 {
			amt := s.Bank.FreeParkingPool
			s.Bank.FreeParkingPool = 0
			e.credit(p, amt, "free_parking")
			e.emit(FreeParkingCollected{PlayerID: p.ID, Amount: amt})
		}
	}
	e.afterLanding()
}

// afterLanding moves to PostRoll unless somebody owes money.
func (e *engine) afterLanding() {
	if len(e.s.Debts) > 0 {
		e.enterRaisingFunds(PhasePostRoll)
		return
	}
	e.setPhase(PhasePostRoll)
}

// rentFor computes the rent due on a space for the current landing.
func (e *engine) rentFor(space int, payer *Player) int {
	s := e.s
	def := s.cfg.Space(space)
	st := s.Spaces[space]
	owner := st.OwnerID
	switch def.Type {
	case SpaceStreet:
		if st.Houses > 0 {
			return def.Rent[st.Houses]
		}
		if s.OwnsGroup(owner, def.Group) {
			return def.Rent[0] * 2
		}
		return def.Rent[0]
	case SpaceRailroad:
		n := s.CountOwnedInGroup(owner, GroupRailroad)
		rent := def.Rent[n-1]
		if s.Turn.SpecialRent {
			rent *= 2
		}
		return rent
	case SpaceUtility:
		n := s.CountOwnedInGroup(owner, GroupUtility)
		mult := def.Rent[0]
		if n >= 2 {
			mult = def.Rent[1]
		}
		dice := s.Turn.LastRoll
		if s.Turn.SpecialRent {
			mult = 10
			dice = rollDice(e.rng)
			e.emit(UtilityRoll{PlayerID: payer.ID, Dice: dice})
		}
		return mult * (dice[0] + dice[1])
	}
	return 0
}

// ---- cards ---------------------------------------------------------------------------

func (e *engine) drawCard(p *Player, deckName string) {
	s := e.s
	deck := &s.Decks.Chance
	if deckName == "community_chest" {
		deck = &s.Decks.CommunityChest
	}
	if len(*deck) == 0 {
		e.afterLanding()
		return
	}
	id := (*deck)[0]
	card, _ := s.cfg.Card(id)
	if card.Effect == CardGetOutOfJail {
		*deck = (*deck)[1:] // held by the player until used
	} else {
		*deck = append((*deck)[1:], id)
	}
	e.emit(CardDrawn{PlayerID: p.ID, Deck: deckName, CardID: id, Text: card.Text})
	e.applyCard(p, card)
}

func (e *engine) applyCard(p *Player, card CardDef) {
	s := e.s
	switch card.Effect {
	case CardMoney:
		if card.Amount >= 0 {
			e.credit(p, card.Amount, "card")
		} else {
			e.charge(p, -card.Amount, "", "card")
		}
	case CardMoveTo:
		e.moveTo(p, card.Target, true)
		e.resolveLanding(p)
		return
	case CardMoveBack:
		e.moveBack(p, card.Steps)
		e.resolveLanding(p)
		return
	case CardMoveToNearest:
		target := e.nearest(p.Position, card.Group)
		if target >= 0 {
			e.moveTo(p, target, true)
			s.Turn.SpecialRent = true
			e.resolveLanding(p)
			s.Turn.SpecialRent = false
			return
		}
	case CardGoToJail:
		e.sendToJail(p, "card")
	case CardGetOutOfJail:
		p.JailCards = append(p.JailCards, card.ID)
		e.emit(CardKept{PlayerID: p.ID, CardID: card.ID})
	case CardRepairs:
		houses, hotels := s.BuildingCounts(p.ID)
		e.charge(p, houses*card.PerHouse+hotels*card.PerHotel, "", "repairs")
	case CardCollectFromEach:
		for _, other := range s.ActivePlayers() {
			if other.ID != p.ID {
				e.charge(other, card.Amount, p.ID, "card")
			}
		}
	case CardPayEach:
		for _, other := range s.ActivePlayers() {
			if other.ID != p.ID {
				e.charge(p, card.Amount, other.ID, "card")
			}
		}
	}
	e.afterLanding()
}

// nearest finds the next space of a group strictly ahead of pos (wrapping).
func (e *engine) nearest(pos int, group string) int {
	for d := 1; d <= BoardSize; d++ {
		i := (pos + d) % BoardSize
		if e.s.cfg.Spaces[i].Group == group {
			return i
		}
	}
	return -1
}

// ---- jail --------------------------------------------------------------------------------

func (e *engine) payJailFine(p *Player) error {
	if err := e.requireCurrent(p, PhasePreRoll); err != nil {
		return err
	}
	if !p.InJail {
		return errf(ErrInvalid, "not in jail")
	}
	fine := e.rules().JailFine
	if p.Cash < fine {
		return errf(ErrInsufficientFunds, "need %d to pay the fine", fine)
	}
	e.debit(p, fine, "jail_fine")
	if e.rules().FreeParkingJackpot {
		e.s.Bank.FreeParkingPool += fine
	}
	e.emit(JailFinePaid{PlayerID: p.ID, Amount: fine})
	p.InJail = false
	p.JailTurns = 0
	e.emit(LeftJail{PlayerID: p.ID, How: "fine"})
	return nil
}

func (e *engine) useJailCard(p *Player) error {
	if err := e.requireCurrent(p, PhasePreRoll); err != nil {
		return err
	}
	if !p.InJail {
		return errf(ErrInvalid, "not in jail")
	}
	if len(p.JailCards) == 0 {
		return errf(ErrInvalid, "no Get Out of Jail Free card")
	}
	id := p.JailCards[0]
	p.JailCards = p.JailCards[1:]
	e.returnCard(id)
	e.emit(JailCardUsed{PlayerID: p.ID, CardID: id})
	p.InJail = false
	p.JailTurns = 0
	e.emit(LeftJail{PlayerID: p.ID, How: "card"})
	return nil
}

// returnCard puts a held card at the bottom of its deck.
func (e *engine) returnCard(id string) {
	for _, c := range e.s.cfg.Chance {
		if c.ID == id {
			e.s.Decks.Chance = append(e.s.Decks.Chance, id)
			return
		}
	}
	e.s.Decks.CommunityChest = append(e.s.Decks.CommunityChest, id)
}

// Abandon ends a game administratively; the richest player wins.
func Abandon(s *State) []Event {
	if s.Turn.Phase == PhaseGameOver {
		return nil
	}
	e := &engine{s: s}
	e.endGame(e.richest(), "abandoned")
	return e.events
}
