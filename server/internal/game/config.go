// Package game is the pure, deterministic Monopoly-rules engine.
//
// It has no I/O and no goroutines: callers feed Commands into Apply and get
// back Events. Everything the UI, the bots, and the persistence layer need is
// derived from the State and the Event stream.
package game

import "fmt"

// SpaceType classifies the 40 board spaces.
type SpaceType string

const (
	SpaceGo             SpaceType = "go"
	SpaceStreet         SpaceType = "street"
	SpaceRailroad       SpaceType = "railroad"
	SpaceUtility        SpaceType = "utility"
	SpaceChance         SpaceType = "chance"
	SpaceCommunityChest SpaceType = "community_chest"
	SpaceTax            SpaceType = "tax"
	SpaceJail           SpaceType = "jail"
	SpaceFreeParking    SpaceType = "free_parking"
	SpaceGoToJail       SpaceType = "go_to_jail"
)

// Group identifiers for non-street ownable spaces.
const (
	GroupRailroad = "railroad"
	GroupUtility  = "utility"
)

// SpaceDef is the static definition of a board space (admin-editable).
type SpaceDef struct {
	Index     int       `json:"index"`
	Name      string    `json:"name"`
	Type      SpaceType `json:"type"`
	Group     string    `json:"group,omitempty"` // colour group id, "railroad" or "utility"
	Color     string    `json:"color,omitempty"` // hex, for the UI only
	Price     int       `json:"price,omitempty"`
	Rent      [6]int    `json:"rent"` // streets: base, 1..4 houses, hotel
	HouseCost int       `json:"houseCost,omitempty"`
	TaxAmount int       `json:"taxAmount,omitempty"`
}

// Ownable reports whether a space can be bought.
func (s SpaceDef) Ownable() bool {
	return s.Type == SpaceStreet || s.Type == SpaceRailroad || s.Type == SpaceUtility
}

// MortgageValue is half the purchase price.
func (s SpaceDef) MortgageValue() int { return s.Price / 2 }

// CardEffect is the kind of thing a Chance / Community Chest card does.
type CardEffect string

const (
	CardMoney           CardEffect = "money"             // Amount from (negative) or to (positive) the player
	CardMoveTo          CardEffect = "move_to"           // Target space, collecting Go salary if passed
	CardMoveBack        CardEffect = "move_back"         // Steps backwards
	CardMoveToNearest   CardEffect = "move_to_nearest"   // Group railroad/utility; special rent
	CardGoToJail        CardEffect = "go_to_jail"        //
	CardGetOutOfJail    CardEffect = "get_out_of_jail"   // kept by player until used
	CardRepairs         CardEffect = "repairs"           // PerHouse / PerHotel to the bank
	CardCollectFromEach CardEffect = "collect_from_each" // Amount from every other player
	CardPayEach         CardEffect = "pay_each"          // Amount to every other player
)

// CardDef is a single Chance / Community Chest card (admin-editable).
type CardDef struct {
	ID       string     `json:"id"`
	Text     string     `json:"text"`
	Effect   CardEffect `json:"effect"`
	Amount   int        `json:"amount,omitempty"`
	Target   int        `json:"target,omitempty"`
	Steps    int        `json:"steps,omitempty"`
	Group    string     `json:"group,omitempty"`
	PerHouse int        `json:"perHouse,omitempty"`
	PerHotel int        `json:"perHotel,omitempty"`
}

// Rules are the numeric / boolean knobs. The classic values are the defaults;
// house-rule toggles are gated by the paid tier at the lobby level, not here.
type Rules struct {
	StartCash           int  `json:"startCash"`
	GoSalary            int  `json:"goSalary"`
	JailFine            int  `json:"jailFine"`
	MaxJailTurns        int  `json:"maxJailTurns"` // failed rolls before the fine is forced
	MaxDoubles          int  `json:"maxDoubles"`   // consecutive doubles that send you to jail
	HouseSupply         int  `json:"houseSupply"`
	HotelSupply         int  `json:"hotelSupply"`
	MortgageInterestPct int  `json:"mortgageInterestPct"`
	BuildingSellPct     int  `json:"buildingSellPct"`
	AuctionsEnabled     bool `json:"auctionsEnabled"`
	// House rules (off by default):
	FreeParkingJackpot      bool `json:"freeParkingJackpot"`
	DoubleSalaryOnGoLanding bool `json:"doubleSalaryOnGoLanding"`
	NoRentInJail            bool `json:"noRentInJail"`
	TurnLimit               int  `json:"turnLimit"` // 0 = play to last player standing
}

// Config is a complete, immutable game definition. Games pin a config for life.
type Config struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	Currency       string     `json:"currency"`
	Spaces         []SpaceDef `json:"spaces"`
	Chance         []CardDef  `json:"chance"`
	CommunityChest []CardDef  `json:"communityChest"`
	Rules          Rules      `json:"rules"`
}

// BoardSize is fixed by the rules engine; configs may rename/re-price but not resize.
const BoardSize = 40

// Validate checks structural invariants an admin-edited config must satisfy.
func (c *Config) Validate() error {
	if len(c.Spaces) != BoardSize {
		return fmt.Errorf("config: expected %d spaces, got %d", BoardSize, len(c.Spaces))
	}
	counts := map[SpaceType]int{}
	groups := map[string]int{}
	for i, s := range c.Spaces {
		if s.Index != i {
			return fmt.Errorf("config: space %d has index %d", i, s.Index)
		}
		if s.Name == "" {
			return fmt.Errorf("config: space %d has no name", i)
		}
		counts[s.Type]++
		switch s.Type {
		case SpaceStreet:
			if s.Group == "" || s.Price <= 0 || s.HouseCost <= 0 {
				return fmt.Errorf("config: street %q needs group, price and houseCost", s.Name)
			}
			for r := 1; r < 6; r++ {
				if s.Rent[r] < s.Rent[r-1] {
					return fmt.Errorf("config: street %q rents must be non-decreasing", s.Name)
				}
			}
			groups[s.Group]++
		case SpaceRailroad:
			if s.Price <= 0 {
				return fmt.Errorf("config: railroad %q needs price", s.Name)
			}
			if s.Group != GroupRailroad {
				return fmt.Errorf("config: railroad %q must be in group %q", s.Name, GroupRailroad)
			}
		case SpaceUtility:
			if s.Price <= 0 {
				return fmt.Errorf("config: utility %q needs price", s.Name)
			}
			if s.Group != GroupUtility {
				return fmt.Errorf("config: utility %q must be in group %q", s.Name, GroupUtility)
			}
		case SpaceTax:
			if s.TaxAmount <= 0 {
				return fmt.Errorf("config: tax %q needs taxAmount", s.Name)
			}
		}
	}
	for _, t := range []SpaceType{SpaceGo, SpaceJail, SpaceFreeParking, SpaceGoToJail} {
		if counts[t] != 1 {
			return fmt.Errorf("config: expected exactly one %s space, got %d", t, counts[t])
		}
	}
	if c.Spaces[0].Type != SpaceGo {
		return fmt.Errorf("config: space 0 must be Go")
	}
	for g, n := range groups {
		if n < 2 || n > 3 {
			return fmt.Errorf("config: colour group %q has %d streets (need 2 or 3)", g, n)
		}
	}
	if len(c.Chance) == 0 || len(c.CommunityChest) == 0 {
		return fmt.Errorf("config: both card decks must be non-empty")
	}
	seen := map[string]bool{}
	for _, d := range [][]CardDef{c.Chance, c.CommunityChest} {
		for _, card := range d {
			if card.ID == "" || seen[card.ID] {
				return fmt.Errorf("config: card id %q missing or duplicated", card.ID)
			}
			seen[card.ID] = true
			if card.Effect == CardMoveTo && (card.Target < 0 || card.Target >= BoardSize) {
				return fmt.Errorf("config: card %q target out of range", card.ID)
			}
		}
	}
	r := c.Rules
	if r.StartCash <= 0 || r.GoSalary < 0 || r.JailFine < 0 || r.MaxJailTurns <= 0 || r.MaxDoubles <= 0 {
		return fmt.Errorf("config: rules have invalid numeric values")
	}
	if r.HouseSupply < 0 || r.HotelSupply < 0 || r.BuildingSellPct <= 0 || r.BuildingSellPct > 100 {
		return fmt.Errorf("config: rules have invalid supply/sell values")
	}
	return nil
}

// Space returns the definition at an index (panics on out-of-range; callers validate).
func (c *Config) Space(i int) SpaceDef { return c.Spaces[i] }

// GroupSpaces returns the indices of all spaces sharing a group.
func (c *Config) GroupSpaces(group string) []int {
	var out []int
	for _, s := range c.Spaces {
		if s.Group == group {
			out = append(out, s.Index)
		}
	}
	return out
}

// FindSpace returns the first index of a space type, or -1.
func (c *Config) FindSpace(t SpaceType) int {
	for _, s := range c.Spaces {
		if s.Type == t {
			return s.Index
		}
	}
	return -1
}

// Card looks up a card by id across both decks.
func (c *Config) Card(id string) (CardDef, bool) {
	for _, d := range [][]CardDef{c.Chance, c.CommunityChest} {
		for _, card := range d {
			if card.ID == id {
				return card, true
			}
		}
	}
	return CardDef{}, false
}

// DefaultRules are the classic rules.
func DefaultRules() Rules {
	return Rules{
		StartCash:           1500,
		GoSalary:            200,
		JailFine:            50,
		MaxJailTurns:        3,
		MaxDoubles:          3,
		HouseSupply:         32,
		HotelSupply:         12,
		MortgageInterestPct: 10,
		BuildingSellPct:     50,
		AuctionsEnabled:     true,
	}
}
