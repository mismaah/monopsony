package game

// Phase is where the game is waiting for input.
type Phase string

const (
	PhasePreRoll      Phase = "pre_roll"      // current player may roll, trade, build, mortgage, or resolve jail
	PhaseResolving    Phase = "resolving"     // current player must decide on an unowned property
	PhaseAuction      Phase = "auction"       // bidding in progress
	PhasePostRoll     Phase = "post_roll"     // current player may trade/build/mortgage, then end turn
	PhaseRaisingFunds Phase = "raising_funds" // a debtor must sell/mortgage or declare bankruptcy
	PhaseGameOver     Phase = "game_over"
)

// Player is a seat at the table.
type Player struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	IsBot        bool     `json:"isBot"`
	Cash         int      `json:"cash"`
	Position     int      `json:"position"`
	InJail       bool     `json:"inJail"`
	JailTurns    int      `json:"jailTurns"` // failed roll attempts while in jail
	JailCards    []string `json:"jailCards"` // card IDs held
	Bankrupt     bool     `json:"bankrupt"`
	DoublesCount int      `json:"doublesCount"` // consecutive doubles this turn
}

// SpaceState is the mutable part of a board space.
type SpaceState struct {
	OwnerID   string `json:"ownerId,omitempty"`
	Houses    int    `json:"houses"` // 0-4 houses, 5 = hotel
	Mortgaged bool   `json:"mortgaged"`
}

// Hotel reports whether the space has a hotel.
func (s SpaceState) Hotel() bool { return s.Houses == 5 }

// Turn tracks the current player's turn progress.
type Turn struct {
	Number          int    `json:"number"` // increments each time play passes to a new player
	PlayerIdx       int    `json:"playerIdx"`
	Phase           Phase  `json:"phase"`
	LastRoll        [2]int `json:"lastRoll"`
	Rolled          bool   `json:"rolled"`          // has rolled at least once this turn
	CanRollAgain    bool   `json:"canRollAgain"`    // rolled doubles and wasn't jailed
	SpecialRent     bool   `json:"specialRent"`     // landed via a "nearest railroad/utility" card
	PendingJailMove bool   `json:"pendingJailMove"` // forced jail fine went to debt; move after it clears
}

// Auction is an in-progress auction for one space. Bidding rotates among
// Active players; a player either outbids or passes (dropping out). The
// auction ends when one active player remains with a bid, or when every
// player has passed with no bid.
type Auction struct {
	SpaceIndex   int      `json:"spaceIndex"`
	HighBid      int      `json:"highBid"`
	HighBidderID string   `json:"highBidderId,omitempty"`
	Active       []string `json:"active"` // player IDs still in, in bid order
	TurnID       string   `json:"turnId"` // whose bid it is
	ResumePhase  Phase    `json:"resumePhase"`
}

// TradeSide is what one party gives.
type TradeSide struct {
	Cash       int   `json:"cash"`
	Properties []int `json:"properties"`
	JailCards  int   `json:"jailCards"`
}

// Trade is a pending proposal.
type Trade struct {
	ID          string    `json:"id"`
	FromID      string    `json:"fromId"`
	ToID        string    `json:"toId"`
	Give        TradeSide `json:"give"`    // what From gives
	Receive     TradeSide `json:"receive"` // what From receives
	ResumePhase Phase     `json:"resumePhase"`
}

// Debt is an obligation that could not be paid from cash.
type Debt struct {
	DebtorID   string `json:"debtorId"`
	CreditorID string `json:"creditorId,omitempty"` // "" = the bank
	Amount     int    `json:"amount"`
	Reason     string `json:"reason"`
}

// Decks holds the shuffled draw order for each deck. Drawn cards go to the
// bottom; a Get Out of Jail Free card is removed until the holder uses it.
type Decks struct {
	Chance         []string `json:"chance"`
	CommunityChest []string `json:"communityChest"`
}

// Bank tracks limited resources.
type Bank struct {
	Houses          int `json:"houses"`
	Hotels          int `json:"hotels"`
	FreeParkingPool int `json:"freeParkingPool"` // house rule only
}

// State is the complete game state. It is JSON-serialisable and is the
// snapshot sent to clients (with per-viewer redaction handled elsewhere).
type State struct {
	ConfigID        string       `json:"configId"`
	Players         []Player     `json:"players"`
	Spaces          []SpaceState `json:"spaces"`
	Turn            Turn         `json:"turn"`
	Bank            Bank         `json:"bank"`
	Decks           Decks        `json:"decks"`
	Auction         *Auction     `json:"auction,omitempty"`
	Trade           *Trade       `json:"trade,omitempty"`
	Debts           []Debt       `json:"debts,omitempty"`
	PendingAuctions []int        `json:"pendingAuctions,omitempty"` // spaces to auction after a bank bankruptcy
	ResumePhase     Phase        `json:"resumePhase,omitempty"`     // where to return after debts clear
	WinnerID        string       `json:"winnerId,omitempty"`
	Seq             int          `json:"seq"` // number of events emitted so far
	NextTradeID     int          `json:"nextTradeId"`

	cfg *Config
}

// Config returns the config this state is bound to.
func (s *State) Config() *Config { return s.cfg }

// Bind attaches a config to a state (needed after JSON decoding).
func (s *State) Bind(cfg *Config) { s.cfg = cfg }

// CurrentPlayer returns the player whose turn it is.
func (s *State) CurrentPlayer() *Player { return &s.Players[s.Turn.PlayerIdx] }

// PlayerByID returns a player pointer or nil.
func (s *State) PlayerByID(id string) *Player {
	for i := range s.Players {
		if s.Players[i].ID == id {
			return &s.Players[i]
		}
	}
	return nil
}

// PlayerIndex returns the seat index of a player or -1.
func (s *State) PlayerIndex(id string) int {
	for i := range s.Players {
		if s.Players[i].ID == id {
			return i
		}
	}
	return -1
}

// ActivePlayers returns the non-bankrupt players in seat order.
func (s *State) ActivePlayers() []*Player {
	var out []*Player
	for i := range s.Players {
		if !s.Players[i].Bankrupt {
			out = append(out, &s.Players[i])
		}
	}
	return out
}

// OwnedBy lists the spaces owned by a player.
func (s *State) OwnedBy(id string) []int {
	var out []int
	for i := range s.Spaces {
		if s.Spaces[i].OwnerID == id {
			out = append(out, i)
		}
	}
	return out
}

// OwnsGroup reports whether a player owns every space in a group.
func (s *State) OwnsGroup(id, group string) bool {
	idx := s.cfg.GroupSpaces(group)
	if len(idx) == 0 {
		return false
	}
	for _, i := range idx {
		if s.Spaces[i].OwnerID != id {
			return false
		}
	}
	return true
}

// CountOwnedInGroup counts a player's holdings in a group (railroads/utilities).
func (s *State) CountOwnedInGroup(id, group string) int {
	n := 0
	for _, i := range s.cfg.GroupSpaces(group) {
		if s.Spaces[i].OwnerID == id {
			n++
		}
	}
	return n
}

// GroupHasBuildings reports whether any space in the group has houses/hotel.
func (s *State) GroupHasBuildings(group string) bool {
	for _, i := range s.cfg.GroupSpaces(group) {
		if s.Spaces[i].Houses > 0 {
			return true
		}
	}
	return false
}

// LiquidationValue is the most cash a player could raise without trading:
// cash + mortgage value of unmortgaged properties + building sale proceeds.
func (s *State) LiquidationValue(id string) int {
	p := s.PlayerByID(id)
	if p == nil {
		return 0
	}
	total := p.Cash
	for _, i := range s.OwnedBy(id) {
		def := s.cfg.Space(i)
		st := s.Spaces[i]
		if !st.Mortgaged {
			total += def.MortgageValue()
		}
		if st.Houses > 0 {
			total += st.Houses * def.HouseCost * s.cfg.Rules.BuildingSellPct / 100
		}
	}
	return total
}

// NetWorth is cash + full property value + building cost (for tie-breaks / stats).
func (s *State) NetWorth(id string) int {
	p := s.PlayerByID(id)
	if p == nil {
		return 0
	}
	total := p.Cash
	for _, i := range s.OwnedBy(id) {
		def := s.cfg.Space(i)
		st := s.Spaces[i]
		if st.Mortgaged {
			total += def.MortgageValue()
		} else {
			total += def.Price
		}
		total += st.Houses * def.HouseCost
	}
	return total
}

// BuildingCounts returns (houses, hotels) owned by a player.
func (s *State) BuildingCounts(id string) (houses, hotels int) {
	for _, i := range s.OwnedBy(id) {
		if s.Spaces[i].Hotel() {
			hotels++
		} else {
			houses += s.Spaces[i].Houses
		}
	}
	return
}
