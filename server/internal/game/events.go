package game

import (
	"encoding/json"
	"fmt"
)

// Event is something that happened. Events are the unit of persistence
// (append-only log), of broadcast (every client sees the same stream) and of
// animation (the 3D client plays them in order).
type Event interface {
	EventType() string
}

type (
	GameStarted struct {
		ConfigID string   `json:"configId"`
		Players  []Player `json:"players"`
	}
	DiceRolled struct {
		PlayerID     string `json:"playerId"`
		Dice         [2]int `json:"dice"`
		Doubles      bool   `json:"doubles"`
		DoublesCount int    `json:"doublesCount"`
		InJail       bool   `json:"inJail"`
	}
	// TokenMoved is emitted for every displacement. Forward moves pass Go
	// when To < From; card teleports set ViaCard.
	TokenMoved struct {
		PlayerID string `json:"playerId"`
		From     int    `json:"from"`
		To       int    `json:"to"`
		PassedGo bool   `json:"passedGo"`
		ViaCard  bool   `json:"viaCard"`
		Backward bool   `json:"backward"`
	}
	// CashChanged accompanies every cash movement (bank or player side).
	CashChanged struct {
		PlayerID string `json:"playerId"`
		Delta    int    `json:"delta"`
		Balance  int    `json:"balance"`
		Reason   string `json:"reason"`
	}
	SalaryCollected struct {
		PlayerID string `json:"playerId"`
		Amount   int    `json:"amount"`
	}
	PurchaseOffered struct {
		PlayerID string `json:"playerId"`
		Space    int    `json:"space"`
		Price    int    `json:"price"`
	}
	PropertyBought struct {
		PlayerID string `json:"playerId"`
		Space    int    `json:"space"`
		Price    int    `json:"price"`
	}
	PurchaseDeclined struct {
		PlayerID string `json:"playerId"`
		Space    int    `json:"space"`
	}
	AuctionStarted struct {
		Space   int      `json:"space"`
		Bidders []string `json:"bidders"`
		TurnID  string   `json:"turnId"`
	}
	BidPlaced struct {
		PlayerID   string `json:"playerId"`
		Amount     int    `json:"amount"`
		NextTurnID string `json:"nextTurnId"`
	}
	BidPassed struct {
		PlayerID   string `json:"playerId"`
		NextTurnID string `json:"nextTurnId"`
	}
	AuctionEnded struct {
		Space    int    `json:"space"`
		WinnerID string `json:"winnerId,omitempty"` // empty = unsold
		Amount   int    `json:"amount"`
	}
	RentPaid struct {
		PayerID string `json:"payerId"`
		OwnerID string `json:"ownerId"`
		Space   int    `json:"space"`
		Amount  int    `json:"amount"`
	}
	TaxPaid struct {
		PlayerID string `json:"playerId"`
		Space    int    `json:"space"`
		Amount   int    `json:"amount"`
	}
	CardDrawn struct {
		PlayerID string `json:"playerId"`
		Deck     string `json:"deck"` // "chance" | "community_chest"
		CardID   string `json:"cardId"`
		Text     string `json:"text"`
	}
	CardKept struct {
		PlayerID string `json:"playerId"`
		CardID   string `json:"cardId"`
	}
	UtilityRoll struct {
		PlayerID string `json:"playerId"`
		Dice     [2]int `json:"dice"`
	}
	SentToJail struct {
		PlayerID string `json:"playerId"`
		Reason   string `json:"reason"` // "space" | "card" | "doubles"
	}
	JailFinePaid struct {
		PlayerID string `json:"playerId"`
		Amount   int    `json:"amount"`
		Forced   bool   `json:"forced"`
	}
	JailCardUsed struct {
		PlayerID string `json:"playerId"`
		CardID   string `json:"cardId"`
	}
	LeftJail struct {
		PlayerID string `json:"playerId"`
		How      string `json:"how"` // "fine" | "card" | "doubles"
	}
	JailTurnServed struct {
		PlayerID string `json:"playerId"`
		Turns    int    `json:"turns"`
	}
	HouseBuilt struct {
		PlayerID string `json:"playerId"`
		Space    int    `json:"space"`
		Houses   int    `json:"houses"` // resulting count (5 = hotel)
		Cost     int    `json:"cost"`
	}
	HouseSold struct {
		PlayerID string `json:"playerId"`
		Space    int    `json:"space"`
		Houses   int    `json:"houses"` // resulting count
		Refund   int    `json:"refund"`
	}
	Mortgaged struct {
		PlayerID string `json:"playerId"`
		Space    int    `json:"space"`
		Amount   int    `json:"amount"`
	}
	Unmortgaged struct {
		PlayerID string `json:"playerId"`
		Space    int    `json:"space"`
		Amount   int    `json:"amount"`
	}
	TradeProposed struct {
		Trade Trade `json:"trade"`
	}
	TradeAccepted struct {
		TradeID string `json:"tradeId"`
	}
	TradeRejected struct {
		TradeID string `json:"tradeId"`
		ByID    string `json:"byId"`
	}
	PropertyTransferred struct {
		Space  int    `json:"space"`
		FromID string `json:"fromId,omitempty"`
		ToID   string `json:"toId,omitempty"`
		Reason string `json:"reason"` // "trade" | "bankruptcy"
	}
	DebtIncurred struct {
		Debt Debt `json:"debt"`
	}
	DebtSettled struct {
		Debt Debt `json:"debt"`
	}
	PlayerBankrupt struct {
		PlayerID   string `json:"playerId"`
		CreditorID string `json:"creditorId,omitempty"`
	}
	FreeParkingCollected struct {
		PlayerID string `json:"playerId"`
		Amount   int    `json:"amount"`
	}
	PhaseChanged struct {
		Phase Phase `json:"phase"`
	}
	TurnChanged struct {
		PlayerID string `json:"playerId"`
		Number   int    `json:"number"`
	}
	GameEnded struct {
		WinnerID string `json:"winnerId"`
		Reason   string `json:"reason"` // "last_standing" | "turn_limit"
	}
)

func (GameStarted) EventType() string          { return "GameStarted" }
func (DiceRolled) EventType() string           { return "DiceRolled" }
func (TokenMoved) EventType() string           { return "TokenMoved" }
func (CashChanged) EventType() string          { return "CashChanged" }
func (SalaryCollected) EventType() string      { return "SalaryCollected" }
func (PurchaseOffered) EventType() string      { return "PurchaseOffered" }
func (PropertyBought) EventType() string       { return "PropertyBought" }
func (PurchaseDeclined) EventType() string     { return "PurchaseDeclined" }
func (AuctionStarted) EventType() string       { return "AuctionStarted" }
func (BidPlaced) EventType() string            { return "BidPlaced" }
func (BidPassed) EventType() string            { return "BidPassed" }
func (AuctionEnded) EventType() string         { return "AuctionEnded" }
func (RentPaid) EventType() string             { return "RentPaid" }
func (TaxPaid) EventType() string              { return "TaxPaid" }
func (CardDrawn) EventType() string            { return "CardDrawn" }
func (CardKept) EventType() string             { return "CardKept" }
func (UtilityRoll) EventType() string          { return "UtilityRoll" }
func (SentToJail) EventType() string           { return "SentToJail" }
func (JailFinePaid) EventType() string         { return "JailFinePaid" }
func (JailCardUsed) EventType() string         { return "JailCardUsed" }
func (LeftJail) EventType() string             { return "LeftJail" }
func (JailTurnServed) EventType() string       { return "JailTurnServed" }
func (HouseBuilt) EventType() string           { return "HouseBuilt" }
func (HouseSold) EventType() string            { return "HouseSold" }
func (Mortgaged) EventType() string            { return "Mortgaged" }
func (Unmortgaged) EventType() string          { return "Unmortgaged" }
func (TradeProposed) EventType() string        { return "TradeProposed" }
func (TradeAccepted) EventType() string        { return "TradeAccepted" }
func (TradeRejected) EventType() string        { return "TradeRejected" }
func (PropertyTransferred) EventType() string  { return "PropertyTransferred" }
func (DebtIncurred) EventType() string         { return "DebtIncurred" }
func (DebtSettled) EventType() string          { return "DebtSettled" }
func (PlayerBankrupt) EventType() string       { return "PlayerBankrupt" }
func (FreeParkingCollected) EventType() string { return "FreeParkingCollected" }
func (PhaseChanged) EventType() string         { return "PhaseChanged" }
func (TurnChanged) EventType() string          { return "TurnChanged" }
func (GameEnded) EventType() string            { return "GameEnded" }

var eventFactories = map[string]func() Event{
	"GameStarted":          func() Event { return &GameStarted{} },
	"DiceRolled":           func() Event { return &DiceRolled{} },
	"TokenMoved":           func() Event { return &TokenMoved{} },
	"CashChanged":          func() Event { return &CashChanged{} },
	"SalaryCollected":      func() Event { return &SalaryCollected{} },
	"PurchaseOffered":      func() Event { return &PurchaseOffered{} },
	"PropertyBought":       func() Event { return &PropertyBought{} },
	"PurchaseDeclined":     func() Event { return &PurchaseDeclined{} },
	"AuctionStarted":       func() Event { return &AuctionStarted{} },
	"BidPlaced":            func() Event { return &BidPlaced{} },
	"BidPassed":            func() Event { return &BidPassed{} },
	"AuctionEnded":         func() Event { return &AuctionEnded{} },
	"RentPaid":             func() Event { return &RentPaid{} },
	"TaxPaid":              func() Event { return &TaxPaid{} },
	"CardDrawn":            func() Event { return &CardDrawn{} },
	"CardKept":             func() Event { return &CardKept{} },
	"UtilityRoll":          func() Event { return &UtilityRoll{} },
	"SentToJail":           func() Event { return &SentToJail{} },
	"JailFinePaid":         func() Event { return &JailFinePaid{} },
	"JailCardUsed":         func() Event { return &JailCardUsed{} },
	"LeftJail":             func() Event { return &LeftJail{} },
	"JailTurnServed":       func() Event { return &JailTurnServed{} },
	"HouseBuilt":           func() Event { return &HouseBuilt{} },
	"HouseSold":            func() Event { return &HouseSold{} },
	"Mortgaged":            func() Event { return &Mortgaged{} },
	"Unmortgaged":          func() Event { return &Unmortgaged{} },
	"TradeProposed":        func() Event { return &TradeProposed{} },
	"TradeAccepted":        func() Event { return &TradeAccepted{} },
	"TradeRejected":        func() Event { return &TradeRejected{} },
	"PropertyTransferred":  func() Event { return &PropertyTransferred{} },
	"DebtIncurred":         func() Event { return &DebtIncurred{} },
	"DebtSettled":          func() Event { return &DebtSettled{} },
	"PlayerBankrupt":       func() Event { return &PlayerBankrupt{} },
	"FreeParkingCollected": func() Event { return &FreeParkingCollected{} },
	"PhaseChanged":         func() Event { return &PhaseChanged{} },
	"TurnChanged":          func() Event { return &TurnChanged{} },
	"GameEnded":            func() Event { return &GameEnded{} },
}

// EventTypes lists every event type name.
func EventTypes() []string {
	out := make([]string, 0, len(eventFactories))
	for k := range eventFactories {
		out = append(out, k)
	}
	return out
}

// DecodeEvent builds an event from its type name and JSON payload.
func DecodeEvent(typ string, payload []byte) (Event, error) {
	f, ok := eventFactories[typ]
	if !ok {
		return nil, fmt.Errorf("unknown event type %q", typ)
	}
	e := f()
	if err := json.Unmarshal(payload, e); err != nil {
		return nil, fmt.Errorf("decode %s: %w", typ, err)
	}
	return e, nil
}

// EventEnvelope is the log/wire form of an event.
type EventEnvelope struct {
	Seq     int             `json:"seq"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// Wrap serialises an event into an envelope.
func Wrap(seq int, e Event) (EventEnvelope, error) {
	b, err := json.Marshal(e)
	if err != nil {
		return EventEnvelope{}, err
	}
	return EventEnvelope{Seq: seq, Type: e.EventType(), Payload: b}, nil
}
