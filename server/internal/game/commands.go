package game

import (
	"encoding/json"
	"fmt"
)

// Command is a player's intent. Every command carries the acting player so the
// engine can enforce turn order and ownership.
type Command interface {
	CommandType() string
	Actor() string
}

// Base is embedded by every command.
type Base struct {
	PlayerID string `json:"playerId"`
}

// Actor implements Command.
func (b Base) Actor() string { return b.PlayerID }

type (
	// RollDice starts the move (or a jail escape attempt).
	RollDice struct{ Base }
	// BuyProperty accepts the purchase offer for the space just landed on.
	BuyProperty struct{ Base }
	// DeclineBuy refuses the purchase; the space goes to auction if enabled.
	DeclineBuy struct{ Base }
	// PlaceBid bids in the active auction.
	PlaceBid struct {
		Base
		Amount int `json:"amount"`
	}
	// PassBid drops out of the active auction.
	PassBid struct{ Base }
	// EndTurn finishes the post-roll phase.
	EndTurn struct{ Base }
	// BuildHouse adds a house (or upgrades to a hotel) on a street.
	BuildHouse struct {
		Base
		Space int `json:"space"`
	}
	// SellHouse removes a house (or downgrades a hotel) from a street.
	SellHouse struct {
		Base
		Space int `json:"space"`
	}
	// Mortgage mortgages a property.
	Mortgage struct {
		Base
		Space int `json:"space"`
	}
	// Unmortgage lifts a mortgage (price + interest).
	Unmortgage struct {
		Base
		Space int `json:"space"`
	}
	// ProposeTrade offers a deal to another player.
	ProposeTrade struct {
		Base
		ToID    string    `json:"toId"`
		Give    TradeSide `json:"give"`
		Receive TradeSide `json:"receive"`
	}
	// AcceptTrade is sent by the counterparty.
	AcceptTrade struct {
		Base
		TradeID string `json:"tradeId"`
	}
	// RejectTrade is sent by either party (the proposer may cancel).
	RejectTrade struct {
		Base
		TradeID string `json:"tradeId"`
	}
	// PayJailFine pays to leave jail before rolling.
	PayJailFine struct{ Base }
	// UseJailCard spends a Get Out of Jail Free card.
	UseJailCard struct{ Base }
	// DeclareBankruptcy gives up while in RaisingFunds.
	DeclareBankruptcy struct{ Base }
)

func (RollDice) CommandType() string          { return "RollDice" }
func (BuyProperty) CommandType() string       { return "BuyProperty" }
func (DeclineBuy) CommandType() string        { return "DeclineBuy" }
func (PlaceBid) CommandType() string          { return "PlaceBid" }
func (PassBid) CommandType() string           { return "PassBid" }
func (EndTurn) CommandType() string           { return "EndTurn" }
func (BuildHouse) CommandType() string        { return "BuildHouse" }
func (SellHouse) CommandType() string         { return "SellHouse" }
func (Mortgage) CommandType() string          { return "Mortgage" }
func (Unmortgage) CommandType() string        { return "Unmortgage" }
func (ProposeTrade) CommandType() string      { return "ProposeTrade" }
func (AcceptTrade) CommandType() string       { return "AcceptTrade" }
func (RejectTrade) CommandType() string       { return "RejectTrade" }
func (PayJailFine) CommandType() string       { return "PayJailFine" }
func (UseJailCard) CommandType() string       { return "UseJailCard" }
func (DeclareBankruptcy) CommandType() string { return "DeclareBankruptcy" }

var commandFactories = map[string]func() Command{
	"RollDice":          func() Command { return &RollDice{} },
	"BuyProperty":       func() Command { return &BuyProperty{} },
	"DeclineBuy":        func() Command { return &DeclineBuy{} },
	"PlaceBid":          func() Command { return &PlaceBid{} },
	"PassBid":           func() Command { return &PassBid{} },
	"EndTurn":           func() Command { return &EndTurn{} },
	"BuildHouse":        func() Command { return &BuildHouse{} },
	"SellHouse":         func() Command { return &SellHouse{} },
	"Mortgage":          func() Command { return &Mortgage{} },
	"Unmortgage":        func() Command { return &Unmortgage{} },
	"ProposeTrade":      func() Command { return &ProposeTrade{} },
	"AcceptTrade":       func() Command { return &AcceptTrade{} },
	"RejectTrade":       func() Command { return &RejectTrade{} },
	"PayJailFine":       func() Command { return &PayJailFine{} },
	"UseJailCard":       func() Command { return &UseJailCard{} },
	"DeclareBankruptcy": func() Command { return &DeclareBankruptcy{} },
}

// CommandTypes lists every command type name (for protocol generation/tests).
func CommandTypes() []string {
	out := make([]string, 0, len(commandFactories))
	for k := range commandFactories {
		out = append(out, k)
	}
	return out
}

// DecodeCommand builds a command from its type name and JSON payload.
func DecodeCommand(typ string, payload []byte) (Command, error) {
	f, ok := commandFactories[typ]
	if !ok {
		return nil, fmt.Errorf("unknown command type %q", typ)
	}
	c := f()
	if len(payload) > 0 {
		if err := json.Unmarshal(payload, c); err != nil {
			return nil, fmt.Errorf("decode %s: %w", typ, err)
		}
	}
	return c, nil
}
