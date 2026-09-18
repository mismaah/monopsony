package game

// ClassicConfig returns the standard US board, decks and rules. It is the
// engine test fixture only: the names and card texts are Hasbro's, so nothing
// ships with it. The published default is HarboursideConfig, which shares this
// layout space-for-space.
func ClassicConfig() *Config {
	st := func(i int, name, group, color string, price, house int, rent [6]int) SpaceDef {
		return SpaceDef{Index: i, Name: name, Type: SpaceStreet, Group: group, Color: color, Price: price, HouseCost: house, Rent: rent}
	}
	rr := func(i int, name string) SpaceDef {
		return SpaceDef{Index: i, Name: name, Type: SpaceRailroad, Group: GroupRailroad, Price: 200, Rent: [6]int{25, 50, 100, 200}}
	}
	ut := func(i int, name string) SpaceDef {
		return SpaceDef{Index: i, Name: name, Type: SpaceUtility, Group: GroupUtility, Price: 150, Rent: [6]int{4, 10}}
	}
	plain := func(i int, name string, t SpaceType) SpaceDef { return SpaceDef{Index: i, Name: name, Type: t} }
	tax := func(i int, name string, amt int) SpaceDef {
		return SpaceDef{Index: i, Name: name, Type: SpaceTax, TaxAmount: amt}
	}

	spaces := []SpaceDef{
		plain(0, "Go", SpaceGo),
		st(1, "Mediterranean Avenue", "brown", "#8B4513", 60, 50, [6]int{2, 10, 30, 90, 160, 250}),
		plain(2, "Community Chest", SpaceCommunityChest),
		st(3, "Baltic Avenue", "brown", "#8B4513", 60, 50, [6]int{4, 20, 60, 180, 320, 450}),
		tax(4, "Income Tax", 200),
		rr(5, "Reading Railroad"),
		st(6, "Oriental Avenue", "lightblue", "#87CEEB", 100, 50, [6]int{6, 30, 90, 270, 400, 550}),
		plain(7, "Chance", SpaceChance),
		st(8, "Vermont Avenue", "lightblue", "#87CEEB", 100, 50, [6]int{6, 30, 90, 270, 400, 550}),
		st(9, "Connecticut Avenue", "lightblue", "#87CEEB", 120, 50, [6]int{8, 40, 100, 300, 450, 600}),
		plain(10, "Jail", SpaceJail),
		st(11, "St. Charles Place", "pink", "#FF69B4", 140, 100, [6]int{10, 50, 150, 450, 625, 750}),
		ut(12, "Electric Company"),
		st(13, "States Avenue", "pink", "#FF69B4", 140, 100, [6]int{10, 50, 150, 450, 625, 750}),
		st(14, "Virginia Avenue", "pink", "#FF69B4", 160, 100, [6]int{12, 60, 180, 500, 700, 900}),
		rr(15, "Pennsylvania Railroad"),
		st(16, "St. James Place", "orange", "#FFA500", 180, 100, [6]int{14, 70, 200, 550, 750, 950}),
		plain(17, "Community Chest", SpaceCommunityChest),
		st(18, "Tennessee Avenue", "orange", "#FFA500", 180, 100, [6]int{14, 70, 200, 550, 750, 950}),
		st(19, "New York Avenue", "orange", "#FFA500", 200, 100, [6]int{16, 80, 220, 600, 800, 1000}),
		plain(20, "Free Parking", SpaceFreeParking),
		st(21, "Kentucky Avenue", "red", "#FF0000", 220, 150, [6]int{18, 90, 250, 700, 875, 1050}),
		plain(22, "Chance", SpaceChance),
		st(23, "Indiana Avenue", "red", "#FF0000", 220, 150, [6]int{18, 90, 250, 700, 875, 1050}),
		st(24, "Illinois Avenue", "red", "#FF0000", 240, 150, [6]int{20, 100, 300, 750, 925, 1100}),
		rr(25, "B. & O. Railroad"),
		st(26, "Atlantic Avenue", "yellow", "#FFFF00", 260, 150, [6]int{22, 110, 330, 800, 975, 1150}),
		st(27, "Ventnor Avenue", "yellow", "#FFFF00", 260, 150, [6]int{22, 110, 330, 800, 975, 1150}),
		ut(28, "Water Works"),
		st(29, "Marvin Gardens", "yellow", "#FFFF00", 280, 150, [6]int{24, 120, 360, 850, 1025, 1200}),
		plain(30, "Go To Jail", SpaceGoToJail),
		st(31, "Pacific Avenue", "green", "#008000", 300, 200, [6]int{26, 130, 390, 900, 1100, 1275}),
		st(32, "North Carolina Avenue", "green", "#008000", 300, 200, [6]int{26, 130, 390, 900, 1100, 1275}),
		plain(33, "Community Chest", SpaceCommunityChest),
		st(34, "Pennsylvania Avenue", "green", "#008000", 320, 200, [6]int{28, 150, 450, 1000, 1200, 1400}),
		rr(35, "Short Line"),
		plain(36, "Chance", SpaceChance),
		st(37, "Park Place", "darkblue", "#00008B", 350, 200, [6]int{35, 175, 500, 1100, 1300, 1500}),
		tax(38, "Luxury Tax", 100),
		st(39, "Boardwalk", "darkblue", "#00008B", 400, 200, [6]int{50, 200, 600, 1400, 1700, 2000}),
	}

	chance := []CardDef{
		{ID: "ch01", Text: "Advance to Go. Collect $200.", Effect: CardMoveTo, Target: 0},
		{ID: "ch02", Text: "Advance to Illinois Avenue. If you pass Go, collect $200.", Effect: CardMoveTo, Target: 24},
		{ID: "ch03", Text: "Advance to St. Charles Place. If you pass Go, collect $200.", Effect: CardMoveTo, Target: 11},
		{ID: "ch04", Text: "Advance to the nearest Utility. If unowned, you may buy it. If owned, throw dice and pay the owner ten times the amount thrown.", Effect: CardMoveToNearest, Group: GroupUtility},
		{ID: "ch05", Text: "Advance to the nearest Railroad. If unowned, you may buy it. If owned, pay the owner twice the rental.", Effect: CardMoveToNearest, Group: GroupRailroad},
		{ID: "ch06", Text: "Advance to the nearest Railroad. If unowned, you may buy it. If owned, pay the owner twice the rental.", Effect: CardMoveToNearest, Group: GroupRailroad},
		{ID: "ch07", Text: "Bank pays you a dividend of $50.", Effect: CardMoney, Amount: 50},
		{ID: "ch08", Text: "Get Out of Jail Free.", Effect: CardGetOutOfJail},
		{ID: "ch09", Text: "Go back 3 spaces.", Effect: CardMoveBack, Steps: 3},
		{ID: "ch10", Text: "Go to Jail. Go directly to Jail. Do not pass Go. Do not collect $200.", Effect: CardGoToJail},
		{ID: "ch11", Text: "Make general repairs on all your property: $25 per house, $100 per hotel.", Effect: CardRepairs, PerHouse: 25, PerHotel: 100},
		{ID: "ch12", Text: "Speeding fine: $15.", Effect: CardMoney, Amount: -15},
		{ID: "ch13", Text: "Take a trip to Reading Railroad. If you pass Go, collect $200.", Effect: CardMoveTo, Target: 5},
		{ID: "ch14", Text: "Take a walk on the Boardwalk. Advance to Boardwalk.", Effect: CardMoveTo, Target: 39},
		{ID: "ch15", Text: "You have been elected Chairman of the Board. Pay each player $50.", Effect: CardPayEach, Amount: 50},
		{ID: "ch16", Text: "Your building loan matures. Collect $150.", Effect: CardMoney, Amount: 150},
	}

	chest := []CardDef{
		{ID: "cc01", Text: "Advance to Go. Collect $200.", Effect: CardMoveTo, Target: 0},
		{ID: "cc02", Text: "Bank error in your favor. Collect $200.", Effect: CardMoney, Amount: 200},
		{ID: "cc03", Text: "Doctor's fee. Pay $50.", Effect: CardMoney, Amount: -50},
		{ID: "cc04", Text: "From sale of stock you get $50.", Effect: CardMoney, Amount: 50},
		{ID: "cc05", Text: "Get Out of Jail Free.", Effect: CardGetOutOfJail},
		{ID: "cc06", Text: "Go to Jail. Go directly to Jail. Do not pass Go. Do not collect $200.", Effect: CardGoToJail},
		{ID: "cc07", Text: "Holiday fund matures. Receive $100.", Effect: CardMoney, Amount: 100},
		{ID: "cc08", Text: "Income tax refund. Collect $20.", Effect: CardMoney, Amount: 20},
		{ID: "cc09", Text: "It is your birthday. Collect $10 from every player.", Effect: CardCollectFromEach, Amount: 10},
		{ID: "cc10", Text: "Life insurance matures. Collect $100.", Effect: CardMoney, Amount: 100},
		{ID: "cc11", Text: "Pay hospital fees of $100.", Effect: CardMoney, Amount: -100},
		{ID: "cc12", Text: "Pay school fees of $50.", Effect: CardMoney, Amount: -50},
		{ID: "cc13", Text: "Receive $25 consultancy fee.", Effect: CardMoney, Amount: 25},
		{ID: "cc14", Text: "You are assessed for street repairs: $40 per house, $115 per hotel.", Effect: CardRepairs, PerHouse: 40, PerHotel: 115},
		{ID: "cc15", Text: "You have won second prize in a beauty contest. Collect $10.", Effect: CardMoney, Amount: 10},
		{ID: "cc16", Text: "You inherit $100.", Effect: CardMoney, Amount: 100},
	}

	return &Config{
		ID:             "classic",
		Name:           "Classic",
		Currency:       "$",
		Spaces:         spaces,
		Chance:         chance,
		CommunityChest: chest,
		Rules:          DefaultRules(),
	}
}
