package game

import (
	"encoding/json"
	"testing"
)

// scriptRNG feeds predetermined values (die faces are value+1).
type scriptRNG struct {
	vals []int
	i    int
}

func (r *scriptRNG) IntN(n int) int {
	if r.i >= len(r.vals) {
		return 0
	}
	v := r.vals[r.i] % n
	r.i++
	return v
}

// dice builds an RNG that yields the given rolls in order.
func dice(rolls ...[2]int) *scriptRNG {
	var vals []int
	for _, r := range rolls {
		vals = append(vals, r[0]-1, r[1]-1)
	}
	return &scriptRNG{vals: vals}
}

func newTest(t *testing.T, n int) *State {
	t.Helper()
	cfg := ClassicConfig()
	seats := []Seat{{ID: "a", Name: "Alice"}, {ID: "b", Name: "Bob"}, {ID: "c", Name: "Cara"}, {ID: "d", Name: "Dan"}}[:n]
	s, _, err := NewGame(cfg, seats, NewSeededRNG(1))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// ap applies a command and fails the test on error.
func ap(t *testing.T, s *State, cmd Command, rng RNG) []Event {
	t.Helper()
	evs, err := Apply(s, cmd, rng)
	if err != nil {
		t.Fatalf("unexpected error applying %s: %v", cmd.CommandType(), err)
	}
	return evs
}

// apErr applies a command and asserts it fails with the given code.
func apErr(t *testing.T, code string, s *State, cmd Command, rng RNG) {
	t.Helper()
	_, err := Apply(s, cmd, rng)
	if err == nil {
		t.Fatalf("expected error %s applying %s, got nil", code, cmd.CommandType())
	}
	ge, ok := err.(*Error)
	if !ok || ge.Code != code {
		t.Fatalf("expected error code %s, got %v", code, err)
	}
}

func hasEvent[T Event](evs []Event) (T, bool) {
	var zero T
	for _, e := range evs {
		if v, ok := e.(T); ok {
			return v, true
		}
	}
	return zero, false
}

func expectPhase(t *testing.T, s *State, p Phase) {
	t.Helper()
	if s.Turn.Phase != p {
		t.Fatalf("expected phase %s, got %s", p, s.Turn.Phase)
	}
}

func TestClassicConfigValid(t *testing.T) {
	if err := ClassicConfig().Validate(); err != nil {
		t.Fatal(err)
	}
	bad := ClassicConfig()
	bad.Spaces = bad.Spaces[:39]
	if bad.Validate() == nil {
		t.Fatal("expected validation error for 39 spaces")
	}
}

func TestNewGame(t *testing.T) {
	s := newTest(t, 3)
	if s.Players[0].Cash != 1500 || s.Bank.Houses != 32 || s.Bank.Hotels != 12 {
		t.Fatalf("bad initial state: %+v %+v", s.Players[0], s.Bank)
	}
	expectPhase(t, s, PhasePreRoll)
	if len(s.Decks.Chance) != 16 || len(s.Decks.CommunityChest) != 16 {
		t.Fatal("decks not populated")
	}
	if _, _, err := NewGame(ClassicConfig(), []Seat{{ID: "x"}}, NewSeededRNG(1)); err == nil {
		t.Fatal("expected error for 1 player")
	}
}

func TestRollBuyEndTurn(t *testing.T) {
	s := newTest(t, 2)
	evs := ap(t, s, &RollDice{Base{"a"}}, dice([2]int{1, 2}))
	if s.Players[0].Position != 3 {
		t.Fatalf("expected position 3, got %d", s.Players[0].Position)
	}
	if _, ok := hasEvent[PurchaseOffered](evs); !ok {
		t.Fatal("expected PurchaseOffered")
	}
	expectPhase(t, s, PhaseResolving)
	apErr(t, ErrNotYourTurn, s, &BuyProperty{Base{"b"}}, nil)
	apErr(t, ErrWrongPhase, s, &EndTurn{Base{"a"}}, nil)
	ap(t, s, &BuyProperty{Base{"a"}}, nil)
	if s.Spaces[3].OwnerID != "a" || s.Players[0].Cash != 1440 {
		t.Fatalf("purchase not applied: %+v cash=%d", s.Spaces[3], s.Players[0].Cash)
	}
	expectPhase(t, s, PhasePostRoll)
	evs = ap(t, s, &EndTurn{Base{"a"}}, nil)
	if tc, ok := hasEvent[TurnChanged](evs); !ok || tc.PlayerID != "b" {
		t.Fatal("expected turn to pass to b")
	}
	expectPhase(t, s, PhasePreRoll)
}

func TestPassGoSalary(t *testing.T) {
	s := newTest(t, 2)
	s.Players[0].Position = 38 // Luxury Tax; move 4 -> 2 (Community Chest) passing Go
	// Put a harmless card on top.
	s.Decks.CommunityChest = append([]string{"cc15"}, s.Decks.CommunityChest...)
	evs := ap(t, s, &RollDice{Base{"a"}}, dice([2]int{1, 3}))
	if _, ok := hasEvent[SalaryCollected](evs); !ok {
		t.Fatal("expected salary")
	}
	if s.Players[0].Cash != 1500+200+10 {
		t.Fatalf("cash=%d", s.Players[0].Cash)
	}
}

func TestDoublesRollAgainAndThirdDoublesJail(t *testing.T) {
	s := newTest(t, 2)
	ap(t, s, &RollDice{Base{"a"}}, dice([2]int{1, 1})) // -> 2 (Community Chest)
	// Whatever the card did, a non-blocking outcome should leave us in PostRoll
	// or Resolving; make the test deterministic by placing a known card.
	s = newTest(t, 2)
	s.Decks.CommunityChest = append([]string{"cc15"}, s.Decks.CommunityChest...)
	ap(t, s, &RollDice{Base{"a"}}, dice([2]int{1, 1}))
	expectPhase(t, s, PhasePostRoll)
	if !s.Turn.CanRollAgain {
		t.Fatal("expected roll again after doubles")
	}
	ap(t, s, &EndTurn{Base{"a"}}, nil)
	expectPhase(t, s, PhasePreRoll)
	if s.CurrentPlayer().ID != "a" {
		t.Fatal("doubles should keep the turn")
	}
	// second doubles: 2 -> 6 Oriental (offer), decline -> auction; skip via disabling auctions
	s.cfg.Rules.AuctionsEnabled = false
	ap(t, s, &RollDice{Base{"a"}}, dice([2]int{2, 2}))
	ap(t, s, &DeclineBuy{Base{"a"}}, nil)
	ap(t, s, &EndTurn{Base{"a"}}, nil)
	// third doubles -> jail
	evs := ap(t, s, &RollDice{Base{"a"}}, dice([2]int{3, 3}))
	if j, ok := hasEvent[SentToJail](evs); !ok || j.Reason != "doubles" {
		t.Fatal("expected jail for third doubles")
	}
	if !s.Players[0].InJail || s.Players[0].Position != 10 {
		t.Fatal("player should be in jail")
	}
	ap(t, s, &EndTurn{Base{"a"}}, nil)
	if s.CurrentPlayer().ID != "b" {
		t.Fatal("turn should pass after jail")
	}
}

func TestRentStreetsRailroadsUtilities(t *testing.T) {
	s := newTest(t, 2)
	// b owns Baltic (3): base rent 4
	s.Spaces[3].OwnerID = "b"
	ap(t, s, &RollDice{Base{"a"}}, dice([2]int{1, 2}))
	if s.Players[0].Cash != 1496 || s.Players[1].Cash != 1504 {
		t.Fatalf("base rent wrong: %d %d", s.Players[0].Cash, s.Players[1].Cash)
	}
	// monopoly doubles unimproved rent
	s.Spaces[1].OwnerID = "b"
	s.Players[0].Position = 0
	s.Turn.Phase = PhasePreRoll
	ap(t, s, &RollDice{Base{"a"}}, dice([2]int{1, 2}))
	if s.Players[0].Cash != 1496-8 {
		t.Fatalf("monopoly rent wrong: %d", s.Players[0].Cash)
	}
	// houses
	s.Spaces[3].Houses = 3
	s.Players[0].Position = 0
	s.Turn.Phase = PhasePreRoll
	ap(t, s, &RollDice{Base{"a"}}, dice([2]int{1, 2}))
	if s.Players[0].Cash != 1488-180 {
		t.Fatalf("house rent wrong: %d", s.Players[0].Cash)
	}
	// railroads: b owns 3 -> 100
	s.Spaces[5].OwnerID, s.Spaces[15].OwnerID, s.Spaces[25].OwnerID = "b", "b", "b"
	s.Players[0].Position = 0
	s.Turn.Phase = PhasePreRoll
	cash := s.Players[0].Cash
	ap(t, s, &RollDice{Base{"a"}}, dice([2]int{2, 3}))
	if s.Players[0].Cash != cash-100 {
		t.Fatalf("railroad rent wrong: %d", cash-s.Players[0].Cash)
	}
	// utility: one owned -> 4x roll
	s.Spaces[12].OwnerID = "b"
	s.Players[0].Position = 10
	s.Turn.Phase = PhasePreRoll
	cash = s.Players[0].Cash
	ap(t, s, &RollDice{Base{"a"}}, dice([2]int{1, 1}))
	if s.Players[0].Cash != cash-8 {
		t.Fatalf("utility rent wrong: %d", cash-s.Players[0].Cash)
	}
	// mortgaged: no rent
	s.Spaces[12].Mortgaged = true
	s.Players[0].Position = 10
	s.Turn.Phase = PhasePreRoll
	s.Turn.CanRollAgain = false
	s.Players[0].DoublesCount = 0
	cash = s.Players[0].Cash
	ap(t, s, &RollDice{Base{"a"}}, dice([2]int{1, 1}))
	if s.Players[0].Cash != cash {
		t.Fatal("mortgaged property should not collect rent")
	}
}

func TestTax(t *testing.T) {
	s := newTest(t, 2)
	ap(t, s, &RollDice{Base{"a"}}, dice([2]int{1, 3})) // Income Tax
	if s.Players[0].Cash != 1300 {
		t.Fatalf("cash=%d", s.Players[0].Cash)
	}
}

func TestJailOptions(t *testing.T) {
	// Pay fine
	s := newTest(t, 2)
	s.Players[0].InJail, s.Players[0].Position = true, 10
	ap(t, s, &PayJailFine{Base{"a"}}, nil)
	if s.Players[0].InJail || s.Players[0].Cash != 1450 {
		t.Fatal("fine not applied")
	}
	expectPhase(t, s, PhasePreRoll)

	// Use card
	s = newTest(t, 2)
	s.Players[0].InJail, s.Players[0].Position = true, 10
	s.Players[0].JailCards = []string{"ch08"}
	before := len(s.Decks.Chance)
	ap(t, s, &UseJailCard{Base{"a"}}, nil)
	if s.Players[0].InJail || len(s.Players[0].JailCards) != 0 || len(s.Decks.Chance) != before+1 {
		t.Fatal("card not applied")
	}

	// Roll doubles: leaves, moves, no bonus roll
	s = newTest(t, 2)
	s.Players[0].InJail, s.Players[0].Position = true, 10
	ap(t, s, &RollDice{Base{"a"}}, dice([2]int{2, 2}))
	if s.Players[0].InJail || s.Players[0].Position != 14 || s.Turn.CanRollAgain {
		t.Fatalf("doubles from jail: %+v canRollAgain=%v", s.Players[0], s.Turn.CanRollAgain)
	}

	// Fail 3 times -> forced fine then move
	s = newTest(t, 2)
	s.Players[0].InJail, s.Players[0].Position = true, 10
	for i := 0; i < 2; i++ {
		ap(t, s, &RollDice{Base{"a"}}, dice([2]int{1, 2}))
		expectPhase(t, s, PhasePostRoll)
		if !s.Players[0].InJail {
			t.Fatal("should still be in jail")
		}
		ap(t, s, &EndTurn{Base{"a"}}, nil)
		ap(t, s, &RollDice{Base{"b"}}, dice([2]int{1, 2}))
		ap(t, s, &DeclineBuy{Base{"b"}}, nil)
		ap(t, s, &PassBid{Base{"a"}}, nil)
		ap(t, s, &PassBid{Base{"b"}}, nil)
		ap(t, s, &EndTurn{Base{"b"}}, nil)
	}
	evs := ap(t, s, &RollDice{Base{"a"}}, dice([2]int{1, 2}))
	if f, ok := hasEvent[JailFinePaid](evs); !ok || !f.Forced {
		t.Fatal("expected forced fine")
	}
	if s.Players[0].InJail || s.Players[0].Position != 13 || s.Players[0].Cash != 1450 {
		t.Fatalf("forced fine outcome: %+v", s.Players[0])
	}
}

func TestAuction(t *testing.T) {
	s := newTest(t, 3)
	ap(t, s, &RollDice{Base{"a"}}, dice([2]int{1, 2})) // Baltic
	ap(t, s, &DeclineBuy{Base{"a"}}, nil)
	expectPhase(t, s, PhaseAuction)
	if s.Auction.TurnID != "b" {
		t.Fatalf("bidding should start with b, got %s", s.Auction.TurnID)
	}
	apErr(t, ErrNotYourTurn, s, &PlaceBid{Base{"a"}, 10}, nil)
	ap(t, s, &PlaceBid{Base{"b"}, 10}, nil)
	apErr(t, ErrInvalid, s, &PlaceBid{Base{"c"}, 10}, nil)
	ap(t, s, &PlaceBid{Base{"c"}, 50}, nil)
	ap(t, s, &PassBid{Base{"a"}}, nil)
	ap(t, s, &PassBid{Base{"b"}}, nil)
	expectPhase(t, s, PhasePostRoll)
	if s.Spaces[3].OwnerID != "c" || s.Players[2].Cash != 1450 {
		t.Fatalf("auction result: %+v cash=%d", s.Spaces[3], s.Players[2].Cash)
	}

	// Everyone passes: unsold
	s = newTest(t, 2)
	ap(t, s, &RollDice{Base{"a"}}, dice([2]int{1, 2}))
	ap(t, s, &DeclineBuy{Base{"a"}}, nil)
	ap(t, s, &PassBid{Base{"b"}}, nil)
	evs := ap(t, s, &PassBid{Base{"a"}}, nil)
	if ae, ok := hasEvent[AuctionEnded](evs); !ok || ae.WinnerID != "" {
		t.Fatal("expected unsold auction")
	}
	if s.Spaces[3].OwnerID != "" {
		t.Fatal("space should remain unowned")
	}
	expectPhase(t, s, PhasePostRoll)
}

func TestBuildEvenAndHotels(t *testing.T) {
	s := newTest(t, 2)
	s.Spaces[1].OwnerID, s.Spaces[3].OwnerID = "a", "a"
	s.Players[0].Cash = 5000
	apErr(t, ErrInvalid, s, &BuildHouse{Base{"a"}, 5}, nil) // railroad
	ap(t, s, &BuildHouse{Base{"a"}, 1}, nil)
	apErr(t, ErrInvalid, s, &BuildHouse{Base{"a"}, 1}, nil) // uneven
	ap(t, s, &BuildHouse{Base{"a"}, 3}, nil)
	for i := 0; i < 3; i++ {
		ap(t, s, &BuildHouse{Base{"a"}, 1}, nil)
		ap(t, s, &BuildHouse{Base{"a"}, 3}, nil)
	}
	if s.Spaces[1].Houses != 4 || s.Spaces[3].Houses != 4 || s.Bank.Houses != 24 {
		t.Fatalf("houses: %d %d bank=%d", s.Spaces[1].Houses, s.Spaces[3].Houses, s.Bank.Houses)
	}
	ap(t, s, &BuildHouse{Base{"a"}, 1}, nil) // hotel
	if s.Spaces[1].Houses != 5 || s.Bank.Hotels != 11 || s.Bank.Houses != 28 {
		t.Fatalf("hotel: %d hotels=%d houses=%d", s.Spaces[1].Houses, s.Bank.Hotels, s.Bank.Houses)
	}
	apErr(t, ErrInvalid, s, &BuildHouse{Base{"a"}, 1}, nil)
	if s.Players[0].Cash != 5000-50*9 {
		t.Fatalf("cash=%d", s.Players[0].Cash)
	}
	// Sell: must sell evenly (hotel first)
	apErr(t, ErrInvalid, s, &SellHouse{Base{"a"}, 3}, nil)
	ap(t, s, &SellHouse{Base{"a"}, 1}, nil)
	if s.Spaces[1].Houses != 4 || s.Bank.Hotels != 12 || s.Bank.Houses != 24 || s.Players[0].Cash != 4575 {
		t.Fatalf("sell hotel: %+v bank=%+v cash=%d", s.Spaces[1], s.Bank, s.Players[0].Cash)
	}
	// Mortgage blocked while buildings exist
	apErr(t, ErrInvalid, s, &Mortgage{Base{"a"}, 1}, nil)
	// Building supply exhaustion blocks building
	s.Bank.Houses, s.Bank.Hotels = 0, 0
	apErr(t, ErrInvalid, s, &BuildHouse{Base{"a"}, 1}, nil)
}

func TestSellHotelDuringHouseShortage(t *testing.T) {
	s := newTest(t, 2)
	s.Spaces[1].OwnerID, s.Spaces[3].OwnerID = "a", "a"
	s.Spaces[1].Houses, s.Spaces[3].Houses = 5, 5
	s.Bank.Hotels, s.Bank.Houses = 10, 2 // fewer than 4 houses available
	cash := s.Players[0].Cash
	ap(t, s, &SellHouse{Base{"a"}, 1}, nil)
	if s.Spaces[1].Houses != 0 || s.Bank.Hotels != 11 || s.Bank.Houses != 2 || s.Players[0].Cash != cash+125 {
		t.Fatalf("shortage sale: %+v bank=%+v cash=%d", s.Spaces[1], s.Bank, s.Players[0].Cash)
	}
}

func TestMortgage(t *testing.T) {
	s := newTest(t, 2)
	s.Spaces[39].OwnerID = "a"
	ap(t, s, &Mortgage{Base{"a"}, 39}, nil)
	if !s.Spaces[39].Mortgaged || s.Players[0].Cash != 1700 {
		t.Fatal("mortgage failed")
	}
	apErr(t, ErrInvalid, s, &Mortgage{Base{"a"}, 39}, nil)
	ap(t, s, &Unmortgage{Base{"a"}, 39}, nil)
	if s.Spaces[39].Mortgaged || s.Players[0].Cash != 1700-220 {
		t.Fatalf("unmortgage failed: cash=%d", s.Players[0].Cash)
	}
	apErr(t, ErrNotYourTurn, s, &Mortgage{Base{"b"}, 39}, nil)
}

func TestTrade(t *testing.T) {
	s := newTest(t, 2)
	s.Spaces[1].OwnerID = "a"
	s.Spaces[3].OwnerID = "b"
	s.Spaces[3].Mortgaged = true
	s.Players[0].JailCards = []string{"ch08"}
	ap(t, s, &ProposeTrade{Base: Base{"a"}, ToID: "b",
		Give: TradeSide{Cash: 100, Properties: []int{1}, JailCards: 1}, Receive: TradeSide{Properties: []int{3}}}, nil)
	apErr(t, ErrInvalid, s, &ProposeTrade{Base: Base{"a"}, ToID: "b", Give: TradeSide{Cash: 1}}, nil)
	apErr(t, ErrNotYourTurn, s, &AcceptTrade{Base{"a"}, s.Trade.ID}, nil)
	ap(t, s, &AcceptTrade{Base{"b"}, s.Trade.ID}, nil)
	if s.Spaces[1].OwnerID != "b" || s.Spaces[3].OwnerID != "a" || s.Trade != nil {
		t.Fatal("properties not swapped")
	}
	// a paid 100 cash + 3 interest on mortgaged Baltic (30*10%)
	if s.Players[0].Cash != 1500-100-3 || s.Players[1].Cash != 1600 {
		t.Fatalf("cash after trade: %d %d", s.Players[0].Cash, s.Players[1].Cash)
	}
	if len(s.Players[1].JailCards) != 1 || len(s.Players[0].JailCards) != 0 {
		t.Fatal("jail card not moved")
	}
	// Reject path and cancellation on turn end
	ap(t, s, &ProposeTrade{Base: Base{"b"}, ToID: "a", Give: TradeSide{Cash: 5}}, nil)
	ap(t, s, &RejectTrade{Base{"a"}, s.Trade.ID}, nil)
	if s.Trade != nil {
		t.Fatal("trade should be cleared")
	}
	ap(t, s, &ProposeTrade{Base: Base{"b"}, ToID: "a", Give: TradeSide{Cash: 5}}, nil)
	s.Turn.Phase = PhasePostRoll
	evs := ap(t, s, &EndTurn{Base{"a"}}, nil)
	if _, ok := hasEvent[TradeRejected](evs); !ok || s.Trade != nil {
		t.Fatal("pending trade should be cancelled at turn end")
	}
}

func TestDebtLiquidationRules(t *testing.T) {
	s := newTest(t, 2)
	s.Spaces[39].OwnerID, s.Spaces[39].Houses = "b", 5
	s.Spaces[34].OwnerID = "a" // Pennsylvania Ave, mortgage value 160
	s.Players[0].Cash = 100
	s.Players[0].Position = 36
	// Rent 2000 owed; liquidation = 100 + 160 = 260 < 2000 -> bankruptcy allowed.
	ap(t, s, &RollDice{Base{"a"}}, dice([2]int{1, 2}))
	expectPhase(t, s, PhaseRaisingFunds)
	acts := LegalActions(s, "a")
	found := false
	for _, a := range acts {
		if a.Type == "DeclareBankruptcy" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected DeclareBankruptcy in %v", acts)
	}
	// Mortgaging first is fine, then bankrupt -> creditor gets everything.
	ap(t, s, &Mortgage{Base{"a"}, 34}, nil)
	expectPhase(t, s, PhaseRaisingFunds)
	evs := ap(t, s, &DeclareBankruptcy{Base{"a"}}, nil)
	if pb, ok := hasEvent[PlayerBankrupt](evs); !ok || pb.CreditorID != "b" {
		t.Fatal("expected bankruptcy to b")
	}
	if ge, ok := hasEvent[GameEnded](evs); !ok || ge.WinnerID != "b" {
		t.Fatal("expected b to win")
	}
	if s.Spaces[34].OwnerID != "b" || !s.Spaces[34].Mortgaged {
		t.Fatal("creditor should receive mortgaged property")
	}
	expectPhase(t, s, PhaseGameOver)
}

func TestDebtSettledBySelling(t *testing.T) {
	s := newTest(t, 2)
	s.Spaces[24].OwnerID = "b" // Illinois, base rent 20
	s.Players[0].Cash = 10
	s.Players[0].Position = 21
	s.Spaces[1].OwnerID, s.Spaces[3].OwnerID = "a", "a"
	s.Spaces[1].Houses, s.Spaces[3].Houses = 1, 1
	s.Bank.Houses -= 2
	ap(t, s, &RollDice{Base{"a"}}, dice([2]int{1, 2}))
	expectPhase(t, s, PhaseRaisingFunds)
	apErr(t, ErrInvalid, s, &DeclareBankruptcy{Base{"a"}}, nil) // can raise 10+25+25+30+30
	evs := ap(t, s, &SellHouse{Base{"a"}, 1}, nil)
	if _, ok := hasEvent[DebtSettled](evs); !ok {
		t.Fatal("expected debt settled after selling")
	}
	expectPhase(t, s, PhasePostRoll)
	if s.Players[0].Cash != 15 || s.Players[1].Cash != 1520 {
		t.Fatalf("cash: %d %d", s.Players[0].Cash, s.Players[1].Cash)
	}
}

func TestBankruptcyToBankAuctionsProperties(t *testing.T) {
	s := newTest(t, 3)
	s.Players[0].Cash = 50
	s.Players[0].Position = 2 // -> Income Tax (200) with roll 2
	s.Spaces[1].OwnerID = "a"
	ap(t, s, &RollDice{Base{"a"}}, dice([2]int{1, 1}))
	expectPhase(t, s, PhaseRaisingFunds)
	ap(t, s, &DeclareBankruptcy{Base{"a"}}, nil)
	if !s.Players[0].Bankrupt || s.Spaces[1].OwnerID != "" {
		t.Fatal("bankruptcy to bank should free the property")
	}
	expectPhase(t, s, PhaseAuction)
	if s.Auction.SpaceIndex != 1 || len(s.Auction.Active) != 2 {
		t.Fatalf("auction: %+v", s.Auction)
	}
	ap(t, s, &PlaceBid{Base{s.Auction.TurnID}, 10}, nil)
	ap(t, s, &PassBid{Base{s.Auction.TurnID}}, nil)
	// After the auction the bankrupt player's turn is skipped.
	expectPhase(t, s, PhasePreRoll)
	if s.CurrentPlayer().ID != "b" {
		t.Fatalf("expected b's turn, got %s", s.CurrentPlayer().ID)
	}
}

func TestCards(t *testing.T) {
	// Advance to nearest railroad pays double
	s := newTest(t, 2)
	s.Spaces[15].OwnerID = "b"
	s.Decks.Chance = append([]string{"ch05"}, s.Decks.Chance...)
	s.Players[0].Position = 4
	ap(t, s, &RollDice{Base{"a"}}, dice([2]int{1, 2})) // -> 7 Chance
	if s.Players[0].Position != 15 || s.Players[0].Cash != 1450 || s.Players[1].Cash != 1550 {
		t.Fatalf("nearest railroad: pos=%d cash=%d/%d", s.Players[0].Position, s.Players[0].Cash, s.Players[1].Cash)
	}

	// Nearest utility pays 10x a fresh roll
	s = newTest(t, 2)
	s.Spaces[12].OwnerID = "b"
	s.Decks.Chance = append([]string{"ch04"}, s.Decks.Chance...)
	s.Players[0].Position = 4
	evs := ap(t, s, &RollDice{Base{"a"}}, dice([2]int{1, 2}, [2]int{4, 5}))
	if _, ok := hasEvent[UtilityRoll](evs); !ok {
		t.Fatal("expected utility roll")
	}
	if s.Players[0].Position != 12 || s.Players[0].Cash != 1500-90 {
		t.Fatalf("nearest utility: pos=%d cash=%d", s.Players[0].Position, s.Players[0].Cash)
	}

	// Go back 3 from 7 lands on Income Tax
	s = newTest(t, 2)
	s.Decks.Chance = append([]string{"ch09"}, s.Decks.Chance...)
	s.Players[0].Position = 4
	ap(t, s, &RollDice{Base{"a"}}, dice([2]int{1, 2}))
	if s.Players[0].Position != 4 || s.Players[0].Cash != 1300 {
		t.Fatalf("go back 3: pos=%d cash=%d", s.Players[0].Position, s.Players[0].Cash)
	}

	// Advance to Go collects salary once
	s = newTest(t, 2)
	s.Decks.Chance = append([]string{"ch01"}, s.Decks.Chance...)
	s.Players[0].Position = 4
	ap(t, s, &RollDice{Base{"a"}}, dice([2]int{1, 2}))
	if s.Players[0].Position != 0 || s.Players[0].Cash != 1700 {
		t.Fatalf("advance to go: pos=%d cash=%d", s.Players[0].Position, s.Players[0].Cash)
	}

	// Repairs
	s = newTest(t, 2)
	s.Decks.Chance = append([]string{"ch11"}, s.Decks.Chance...)
	s.Players[0].Position = 4
	s.Spaces[1].OwnerID, s.Spaces[3].OwnerID = "a", "a"
	s.Spaces[1].Houses, s.Spaces[3].Houses = 5, 2
	ap(t, s, &RollDice{Base{"a"}}, dice([2]int{1, 2}))
	if s.Players[0].Cash != 1500-100-50 {
		t.Fatalf("repairs: cash=%d", s.Players[0].Cash)
	}

	// Birthday: collect from each
	s = newTest(t, 3)
	s.Decks.CommunityChest = append([]string{"cc09"}, s.Decks.CommunityChest...)
	ap(t, s, &RollDice{Base{"a"}}, dice([2]int{1, 1})) // -> 2
	if s.Players[0].Cash != 1520 || s.Players[1].Cash != 1490 || s.Players[2].Cash != 1490 {
		t.Fatalf("birthday: %d %d %d", s.Players[0].Cash, s.Players[1].Cash, s.Players[2].Cash)
	}

	// Get out of jail free is kept and removed from the deck
	s = newTest(t, 2)
	s.Decks.Chance = append([]string{"ch08"}, s.Decks.Chance...)
	s.Players[0].Position = 4
	ap(t, s, &RollDice{Base{"a"}}, dice([2]int{1, 2}))
	if len(s.Players[0].JailCards) != 1 || len(s.Decks.Chance) != 16 {
		t.Fatalf("GOOJF: cards=%v deck=%d", s.Players[0].JailCards, len(s.Decks.Chance))
	}

	// Go to jail card
	s = newTest(t, 2)
	s.Decks.Chance = append([]string{"ch10"}, s.Decks.Chance...)
	s.Players[0].Position = 4
	ap(t, s, &RollDice{Base{"a"}}, dice([2]int{1, 2}))
	if !s.Players[0].InJail || s.Players[0].Position != 10 {
		t.Fatal("go to jail card")
	}
	expectPhase(t, s, PhasePostRoll)
}

func TestGoToJailSpace(t *testing.T) {
	s := newTest(t, 2)
	s.Players[0].Position = 27
	ap(t, s, &RollDice{Base{"a"}}, dice([2]int{1, 2}))
	if !s.Players[0].InJail || s.Players[0].Position != 10 || s.Players[0].Cash != 1500 {
		t.Fatal("go to jail space")
	}
}

func TestErrorsLeaveStateUnchanged(t *testing.T) {
	s := newTest(t, 2)
	snap, _ := json.Marshal(s)
	seqBefore := s.Seq
	bad := []Command{
		&BuyProperty{Base{"a"}}, &EndTurn{Base{"a"}}, &RollDice{Base{"b"}}, &BuildHouse{Base{"a"}, 1},
		&Mortgage{Base{"a"}, 1}, &PlaceBid{Base{"a"}, 5}, &DeclareBankruptcy{Base{"a"}},
		&ProposeTrade{Base: Base{"a"}, ToID: "b", Give: TradeSide{Properties: []int{1}}},
		&RollDice{Base{"zzz"}},
	}
	for _, c := range bad {
		if _, err := Apply(s, c, dice([2]int{1, 1})); err == nil {
			t.Fatalf("expected error for %T", c)
		}
	}
	after, _ := json.Marshal(s)
	if string(snap) != string(after) || s.Seq != seqBefore {
		t.Fatalf("state changed by rejected commands:\n%s\n%s", snap, after)
	}
}

func TestSnapshotRoundTrip(t *testing.T) {
	s := newTest(t, 3)
	ap(t, s, &RollDice{Base{"a"}}, dice([2]int{1, 2}))
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var s2 State
	if err := json.Unmarshal(b, &s2); err != nil {
		t.Fatal(err)
	}
	s2.Bind(ClassicConfig())
	b2, _ := json.Marshal(&s2)
	if string(b) != string(b2) {
		t.Fatal("snapshot did not round-trip")
	}
	ap(t, &s2, &BuyProperty{Base{"a"}}, nil)
}

func TestEventEnvelopeRoundTrip(t *testing.T) {
	ev := RentPaid{PayerID: "a", OwnerID: "b", Space: 3, Amount: 4}
	env, err := Wrap(7, ev)
	if err != nil {
		t.Fatal(err)
	}
	back, err := DecodeEvent(env.Type, env.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if rp := back.(*RentPaid); *rp != ev {
		t.Fatalf("got %+v", rp)
	}
	cmd, err := DecodeCommand("PlaceBid", []byte(`{"playerId":"a","amount":12}`))
	if err != nil || cmd.(*PlaceBid).Amount != 12 || cmd.Actor() != "a" {
		t.Fatalf("decode command: %v %+v", err, cmd)
	}
}

func TestLegalActionsPreRoll(t *testing.T) {
	s := newTest(t, 2)
	acts := LegalActions(s, "a")
	types := map[string]bool{}
	for _, a := range acts {
		types[a.Type] = true
	}
	if !types["RollDice"] || !types["ProposeTrade"] || types["EndTurn"] {
		t.Fatalf("pre-roll actions: %v", acts)
	}
	if len(LegalActions(s, "b")) != 1 { // only ProposeTrade
		t.Fatalf("b should only be able to propose a trade: %v", LegalActions(s, "b"))
	}
}

func TestTurnLimitEndsByNetWorth(t *testing.T) {
	s := newTest(t, 2)
	s.cfg.Rules.TurnLimit = 1
	s.Players[1].Cash = 1600
	ap(t, s, &RollDice{Base{"a"}}, dice([2]int{1, 3})) // tax
	evs := ap(t, s, &EndTurn{Base{"a"}}, nil)
	if ge, ok := hasEvent[GameEnded](evs); !ok || ge.WinnerID != "b" || ge.Reason != "turn_limit" {
		t.Fatalf("expected b to win on turn limit: %v", evs)
	}
}

// Regression: a forced jail fine that goes to debt, followed by a landing that
// creates a second debt, must not leave the game in an empty phase.
func TestForcedJailFineDebtThenRentDebt(t *testing.T) {
	s := newTest(t, 2)
	s.Players[0].InJail, s.Players[0].Position, s.Players[0].JailTurns = true, 10, 2
	s.Players[0].Cash = 20
	s.Spaces[1].OwnerID = "a"  // can mortgage for 30 to cover the 50 fine
	s.Spaces[13].OwnerID = "b" // States Ave: rent 10 after moving 3
	ap(t, s, &RollDice{Base{"a"}}, dice([2]int{1, 2}))
	expectPhase(t, s, PhaseRaisingFunds)
	if !s.Turn.PendingJailMove {
		t.Fatal("expected pending jail move")
	}
	ap(t, s, &Mortgage{Base{"a"}, 1}, nil) // 50 cash -> pays fine -> moves to 13 -> owes 10 with 0 cash
	if err := CheckInvariants(s); err != nil {
		t.Fatal(err)
	}
	expectPhase(t, s, PhaseRaisingFunds)
	if s.Players[0].Position != 13 || s.Players[0].InJail || len(s.Debts) != 1 || s.Debts[0].Amount != 10 {
		t.Fatalf("state after forced move: %+v debts=%+v", s.Players[0], s.Debts)
	}
	ap(t, s, &DeclareBankruptcy{Base{"a"}}, nil)
	expectPhase(t, s, PhaseGameOver)
	if err := CheckInvariants(s); err != nil {
		t.Fatal(err)
	}
}

func TestInvariantsHoldAcrossScenarios(t *testing.T) {
	s := newTest(t, 3)
	if err := CheckInvariants(s); err != nil {
		t.Fatal(err)
	}
	ap(t, s, &RollDice{Base{"a"}}, dice([2]int{1, 2}))
	ap(t, s, &BuyProperty{Base{"a"}}, nil)
	if err := CheckInvariants(s); err != nil {
		t.Fatal(err)
	}
}
