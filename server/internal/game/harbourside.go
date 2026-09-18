package game

// HarboursideConfig is the shipped default board: an original port-city theme
// that matches the Monopsony brand (the octagon coin, Mono the octopus). It
// keeps the classic layout — same space types, colour-group sizes, prices and
// rents at every index — so the engine tests written against ClassicConfig
// describe this board too, while every name and card text is our own.
//
// Reading the board clockwise from Set Sail: the cheap shoreline (Tidepool,
// Shallows), the working harbour (Fish Market, Old Docks), the town above it
// (Lighthouse Quarter, Yacht Basin) and the money on the hill (Merchant
// Heights, The Cape). Ferries stand in for railroads; the two utilities are
// the tidal plant and the desalination works. The decks are Tide (luck) and
// Harbour Fund (community).
func HarboursideConfig() *Config {
	st := func(i int, name, group, color string, price, house int, rent [6]int) SpaceDef {
		return SpaceDef{Index: i, Name: name, Type: SpaceStreet, Group: group, Color: color, Price: price, HouseCost: house, Rent: rent}
	}
	ferry := func(i int, name string) SpaceDef {
		return SpaceDef{Index: i, Name: name, Type: SpaceRailroad, Group: GroupRailroad, Price: 200, Rent: [6]int{25, 50, 100, 200}}
	}
	works := func(i int, name string) SpaceDef {
		return SpaceDef{Index: i, Name: name, Type: SpaceUtility, Group: GroupUtility, Price: 150, Rent: [6]int{4, 10}}
	}
	plain := func(i int, name string, t SpaceType) SpaceDef { return SpaceDef{Index: i, Name: name, Type: t} }
	tax := func(i int, name string, amt int) SpaceDef {
		return SpaceDef{Index: i, Name: name, Type: SpaceTax, TaxAmount: amt}
	}

	const (
		tidepool   = "#8a5a3c"
		shallows   = "#6cc4e6"
		fishmarket = "#e06fa4"
		olddocks   = "#f28c28"
		lighthouse = "#e03e3e"
		yachtbasin = "#f5c451"
		merchant   = "#2e9e4f"
		cape       = "#1e3a8a"
	)

	spaces := []SpaceDef{
		plain(0, "Set Sail", SpaceGo),
		st(1, "Kelp Lane", "tidepool", tidepool, 60, 50, [6]int{2, 10, 30, 90, 160, 250}),
		plain(2, "Harbour Fund", SpaceCommunityChest),
		st(3, "Barnacle Row", "tidepool", tidepool, 60, 50, [6]int{4, 20, 60, 180, 320, 450}),
		tax(4, "Harbour Dues", 200),
		ferry(5, "Harbour Ferry"),
		st(6, "Driftwood Walk", "shallows", shallows, 100, 50, [6]int{6, 30, 90, 270, 400, 550}),
		plain(7, "Tide", SpaceChance),
		st(8, "Gull Street", "shallows", shallows, 100, 50, [6]int{6, 30, 90, 270, 400, 550}),
		st(9, "Pebble Beach", "shallows", shallows, 120, 50, [6]int{8, 40, 100, 300, 450, 600}),
		plain(10, "Dry Dock", SpaceJail),
		st(11, "Oyster Court", "fishmarket", fishmarket, 140, 100, [6]int{10, 50, 150, 450, 625, 750}),
		works(12, "Tidal Power"),
		st(13, "Net Loft Lane", "fishmarket", fishmarket, 140, 100, [6]int{10, 50, 150, 450, 625, 750}),
		st(14, "Salt Market", "fishmarket", fishmarket, 160, 100, [6]int{12, 60, 180, 500, 700, 900}),
		ferry(15, "Island Ferry"),
		st(16, "Rope Walk", "olddocks", olddocks, 180, 100, [6]int{14, 70, 200, 550, 750, 950}),
		plain(17, "Harbour Fund", SpaceCommunityChest),
		st(18, "Chandler's Yard", "olddocks", olddocks, 180, 100, [6]int{14, 70, 200, 550, 750, 950}),
		st(19, "Dockside Square", "olddocks", olddocks, 200, 100, [6]int{16, 80, 220, 600, 800, 1000}),
		plain(20, "Safe Harbour", SpaceFreeParking),
		st(21, "Beacon Row", "lighthouse", lighthouse, 220, 150, [6]int{18, 90, 250, 700, 875, 1050}),
		plain(22, "Tide", SpaceChance),
		st(23, "Signal Street", "lighthouse", lighthouse, 220, 150, [6]int{18, 90, 250, 700, 875, 1050}),
		st(24, "Lantern Square", "lighthouse", lighthouse, 240, 150, [6]int{20, 100, 300, 750, 925, 1100}),
		ferry(25, "Channel Ferry"),
		st(26, "Regatta Row", "yachtbasin", yachtbasin, 260, 150, [6]int{22, 110, 330, 800, 975, 1150}),
		st(27, "Marina Terrace", "yachtbasin", yachtbasin, 260, 150, [6]int{22, 110, 330, 800, 975, 1150}),
		works(28, "Desalination Works"),
		st(29, "Anchorage Avenue", "yachtbasin", yachtbasin, 280, 150, [6]int{24, 120, 360, 850, 1025, 1200}),
		plain(30, "Run Aground", SpaceGoToJail),
		st(31, "Compass Hill", "merchant", merchant, 300, 200, [6]int{26, 130, 390, 900, 1100, 1275}),
		st(32, "Admiralty Row", "merchant", merchant, 300, 200, [6]int{26, 130, 390, 900, 1100, 1275}),
		plain(33, "Harbour Fund", SpaceCommunityChest),
		st(34, "Custom House Square", "merchant", merchant, 320, 200, [6]int{28, 150, 450, 1000, 1200, 1400}),
		ferry(35, "Ocean Liner"),
		plain(36, "Tide", SpaceChance),
		st(37, "Cliffside Drive", "cape", cape, 350, 200, [6]int{35, 175, 500, 1100, 1300, 1500}),
		tax(38, "Mooring Fee", 100),
		st(39, "Captain's Reach", "cape", cape, 400, 200, [6]int{50, 200, 600, 1400, 1700, 2000}),
	}

	tide := []CardDef{
		{ID: "td01", Text: "Fair winds: sail straight back to Set Sail and collect $200.", Effect: CardMoveTo, Target: 0},
		{ID: "td02", Text: "Festival night at Lantern Square. Head there now; collect $200 if you pass Set Sail.", Effect: CardMoveTo, Target: 24},
		{ID: "td03", Text: "Fresh catch at Oyster Court. Head there now; collect $200 if you pass Set Sail.", Effect: CardMoveTo, Target: 11},
		{ID: "td04", Text: "Power cut! Move to the nearest works. If it's unowned you may buy it; if owned, roll and pay the owner ten times the throw.", Effect: CardMoveToNearest, Group: GroupUtility},
		{ID: "td05", Text: "Catch the next ferry. Move to the nearest ferry; buy it if unowned, otherwise pay the owner double fare.", Effect: CardMoveToNearest, Group: GroupRailroad},
		{ID: "td06", Text: "Catch the next ferry. Move to the nearest ferry; buy it if unowned, otherwise pay the owner double fare.", Effect: CardMoveToNearest, Group: GroupRailroad},
		{ID: "td07", Text: "The harbour co-op pays a dividend. Collect $50.", Effect: CardMoney, Amount: 50},
		{ID: "td08", Text: "Harbourmaster's pass: leave Dry Dock free. Keep this card until you need it.", Effect: CardGetOutOfJail},
		{ID: "td09", Text: "Missed the tide. Drift back 3 spaces.", Effect: CardMoveBack, Steps: 3},
		{ID: "td10", Text: "Hull breach! Go straight to Dry Dock. Do not pass Set Sail, do not collect $200.", Effect: CardGoToJail},
		{ID: "td11", Text: "Storm damage across your properties: pay $25 per house and $100 per hotel.", Effect: CardRepairs, PerHouse: 25, PerHotel: 100},
		{ID: "td12", Text: "Speeding in the no-wake zone. Fine: $15.", Effect: CardMoney, Amount: -15},
		{ID: "td13", Text: "Take the Harbour Ferry. Move there; collect $200 if you pass Set Sail.", Effect: CardMoveTo, Target: 5},
		{ID: "td14", Text: "Invited up to Captain's Reach. Move there now.", Effect: CardMoveTo, Target: 39},
		{ID: "td15", Text: "Elected Harbourmaster. Buy a round: pay every player $50.", Effect: CardPayEach, Amount: 50},
		{ID: "td16", Text: "Your boat loan is paid off. Collect $150.", Effect: CardMoney, Amount: 150},
	}

	fund := []CardDef{
		{ID: "hf01", Text: "Sail straight back to Set Sail and collect $200.", Effect: CardMoveTo, Target: 0},
		{ID: "hf02", Text: "The harbour bank miscounted in your favour. Collect $200.", Effect: CardMoney, Amount: 200},
		{ID: "hf03", Text: "Ship's doctor. Pay $50.", Effect: CardMoney, Amount: -50},
		{ID: "hf04", Text: "You sell your share of the catch. Collect $50.", Effect: CardMoney, Amount: 50},
		{ID: "hf05", Text: "Harbourmaster's pass: leave Dry Dock free. Keep this card until you need it.", Effect: CardGetOutOfJail},
		{ID: "hf06", Text: "Caught smuggling. Go straight to Dry Dock. Do not pass Set Sail, do not collect $200.", Effect: CardGoToJail},
		{ID: "hf07", Text: "Regatta prize money. Collect $100.", Effect: CardMoney, Amount: 100},
		{ID: "hf08", Text: "Harbour dues refund. Collect $20.", Effect: CardMoney, Amount: 20},
		{ID: "hf09", Text: "It's your birthday! Collect $10 from every player.", Effect: CardCollectFromEach, Amount: 10},
		{ID: "hf10", Text: "Salvage rights pay out. Collect $100.", Effect: CardMoney, Amount: 100},
		{ID: "hf11", Text: "Lifeboat crew fundraiser. Pay $100.", Effect: CardMoney, Amount: -100},
		{ID: "hf12", Text: "Sailing school fees. Pay $50.", Effect: CardMoney, Amount: -50},
		{ID: "hf13", Text: "You pilot a freighter into port. Collect $25.", Effect: CardMoney, Amount: 25},
		{ID: "hf14", Text: "Assessed for sea-wall repairs: $40 per house, $115 per hotel.", Effect: CardRepairs, PerHouse: 40, PerHotel: 115},
		{ID: "hf15", Text: "Second place in the sandcastle contest. Collect $10.", Effect: CardMoney, Amount: 10},
		{ID: "hf16", Text: "A distant aunt leaves you her boat. Collect $100.", Effect: CardMoney, Amount: 100},
	}

	return &Config{
		ID:             "harbourside",
		Name:           "Harbourside",
		Currency:       "$",
		Spaces:         spaces,
		Chance:         tide,
		CommunityChest: fund,
		Rules:          DefaultRules(),
	}
}
