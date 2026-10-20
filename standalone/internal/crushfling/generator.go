package crushfling

import (
	"fmt"
	"math/rand"
	"strings"
	"time"
)

var (
	firstNames = []string{
		"Emma", "Olivia", "Ava", "Sophia", "Isabella", "Mia", "Amelia", "Harper", "Evelyn", "Abigail",
		"Luna", "Aria", "Chloe", "Layla", "Zoe", "Nora", "Lily", "Avery", "Mila", "Ella",
		"Sofia", "Camila", "Aaliyah", "Scarlett", "Madison", "Ariana", "Grace", "Chloe", "Victoria", "Luna",
		"Hannah", "Aubrey", "Lily", "Addison", "Natalie", "Brooklyn", "Leah", "Zoe", "Stella", "Hazel",
		"Emily", "Elizabeth", "Maya", "Amelie", "Ava", "Isla", "Mia", "Ivy", "Lila", "Nova",
	}
	lastNames = []string{
		"Rose", "Grace", "Hope", "Love", "Bliss", "Dream", "Star", "Moon", "Angel", "Sweet",
		"Heart", "Kiss", "Wild", "Fox", "Lane", "Rain", "Skye", "Belle", "Honey", "Sugar",
		"Dreamy", "Lovely", "Sexy", "Naughty", "Babe", "Baby", "Queen", "Princess", "Kitty", "Doll",
		"Xo", "Love", "Hot", "Cute", "Sweet", "Sassy", "Flirty", "Tease", "Muse", "Desire",
	}
	bioTemplates = []string{
		"Hi I'm at home and alone. Send me a friend request 💋 I will send you my unlimited nudes and sextape. Add me on {platform} >> {username} 😘",
		"Hey babe 😏 looking for fun tonight? I'm 100% real and verified. Message me on {platform}: {username} 💦",
		"Single and ready to mingle 🌸 Love sexting and trading pics. Find me on {platform} @ {username} - let's have fun!",
		"Just here for naughty chats 😈 No time wasters pls. DM me on {platform}: {username} if you're serious 🔥",
		"Bored and horny 😽 wanna sext? Add my {platform} {username} - I reply fast! No free content without request.",
		"Looking for a sexting partner who is respectful and fun 😊 My {platform} is {username} - let's chat!",
		"Open-minded girl looking for discreet fun 💋 {platform} >> {username} | Let's trade and have a good time",
		"I'm new here! Show me around? 😉 Add me: {platform} {username} - I love sexting and video calls",
		"Real girl, real nudes 📸 Add my {platform} {username} if you want to see more. Serious only!",
		"Let's get naughty together 🍑 My {platform} is {username} - message me anytime babe 💕",
	}
	// Target audience is fixed by the operator's brief: the profile is always a
	// woman looking for men, and every account gets one random country from
	// the USA plus the European list below (each with its own city list).
	// Order is intentional: US first so a quick eyeball of a small run is
	// representative.
	countries = []string{
		"US",
		// Western / Northern Europe
		"GB", "IE", "DE", "FR", "NL", "BE", "LU", "AT", "CH",
		// Nordics
		"SE", "NO", "DK", "FI",
		// Southern Europe
		"ES", "PT", "IT", "GR", "MT", "CY",
		// Central / Eastern Europe
		"PL", "CZ", "SK", "HU", "RO", "BG", "HR", "SI", "EE", "LV", "LT",
	}
	platforms = []string{PlatformTelegram, PlatformSnapchat, PlatformDiscord, PlatformWhatsapp, PlatformKik, PlatformInstagram}
	cities    = map[string][]string{
		"US": {"Miami", "Los Angeles", "New York", "Houston", "Chicago", "Las Vegas", "Virginia Beach", "Phoenix", "Seattle", "Denver", "Atlanta", "Austin"},
		"GB": {"London", "Manchester", "Birmingham", "Leeds", "Liverpool", "Bristol", "Glasgow", "Edinburgh"},
		"IE": {"Dublin", "Cork", "Galway", "Limerick", "Waterford"},
		"DE": {"Berlin", "Munich", "Hamburg", "Cologne", "Frankfurt", "Stuttgart", "Dusseldorf", "Leipzig"},
		"FR": {"Paris", "Lyon", "Marseille", "Toulouse", "Nice", "Nantes", "Bordeaux"},
		"NL": {"Amsterdam", "Rotterdam", "The Hague", "Utrecht", "Eindhoven"},
		"BE": {"Brussels", "Antwerp", "Ghent", "Bruges", "Liege"},
		"LU": {"Luxembourg", "Esch-sur-Alzette", "Differdange"},
		"AT": {"Vienna", "Graz", "Linz", "Salzburg", "Innsbruck"},
		"CH": {"Zurich", "Geneva", "Basel", "Bern", "Lausanne"},
		"SE": {"Stockholm", "Gothenburg", "Malmo", "Uppsala", "Linkoping"},
		"NO": {"Oslo", "Bergen", "Trondheim", "Stavanger", "Tromso"},
		"DK": {"Copenhagen", "Aarhus", "Odense", "Aalborg", "Esbjerg"},
		"FI": {"Helsinki", "Espoo", "Tampere", "Vantaa", "Turku"},
		"ES": {"Madrid", "Barcelona", "Valencia", "Seville", "Malaga", "Bilbao"},
		"PT": {"Lisbon", "Porto", "Braga", "Coimbra", "Faro"},
		"IT": {"Rome", "Milan", "Naples", "Turin", "Florence", "Bologna", "Venice"},
		"GR": {"Athens", "Thessaloniki", "Patras", "Heraklion", "Larissa"},
		"MT": {"Valletta", "Sliema", "Birkirkara", "Mosta"},
		"CY": {"Nicosia", "Limassol", "Larnaca", "Paphos"},
		"PL": {"Warsaw", "Krakow", "Lodz", "Wroclaw", "Poznan", "Gdansk"},
		"CZ": {"Prague", "Brno", "Ostrava", "Plzen", "Olomouc"},
		"SK": {"Bratislava", "Kosice", "Presov", "Zilina", "Nitra"},
		"HU": {"Budapest", "Debrecen", "Szeged", "Miskolc", "Pecs"},
		"RO": {"Bucharest", "Cluj-Napoca", "Timisoara", "Iasi", "Constanta"},
		"BG": {"Sofia", "Plovdiv", "Varna", "Burgas", "Ruse"},
		"HR": {"Zagreb", "Split", "Rijeka", "Osijek", "Zadar"},
		"SI": {"Ljubljana", "Maribor", "Celje", "Kranj", "Koper"},
		"EE": {"Tallinn", "Tartu", "Narva", "Parnu", "Kohtla-Jarve"},
		"LV": {"Riga", "Daugavpils", "Liepaja", "Jelgava", "Jurmala"},
		"LT": {"Vilnius", "Kaunas", "Klaipeda", "Siauliai", "Panevezys"},
	}
)

// GenerateUsername creates a plausible crushfling username
func GenerateUsername() string {
	fn := firstNames[rand.Intn(len(firstNames))]
	ln := lastNames[rand.Intn(len(lastNames))]
	num := rand.Intn(9000) + 1000
	// mix case styles like real users: gisellejs709, athena_rosey, luna170
	styles := []string{
		strings.ToLower(fn + ln + fmt.Sprint(num)),
		strings.ToLower(fn + "_" + ln + fmt.Sprint(rand.Intn(99))),
		strings.ToLower(fn + fmt.Sprint(num)),
		strings.ToLower(fn + ln[:2] + fmt.Sprint(num)),
	}
	return styles[rand.Intn(len(styles))]
}

// GenerateName creates the display name shown on the profile ("Emma Rose").
// The username stays a handle; the name is what visitors read.
func GenerateName() string {
	return firstNames[rand.Intn(len(firstNames))] + " " + lastNames[rand.Intn(len(lastNames))]
}

// GenerateProfile creates a random ProfileData ready for registration
func GenerateProfile(email string) ProfileData {
	username := GenerateUsername()
	platform := platforms[rand.Intn(len(platforms))]
	// Brief: every account is 19-24 years old, picked uniformly.
	age := 19 + rand.Intn(6)
	// Gender and "looking for" are constants, not random: the brief says the
	// account is a female profile that is looking for a male.
	gender := GenderWoman
	country := countries[rand.Intn(len(countries))]
	city := ""
	if list := cities[country]; len(list) > 0 {
		city = list[rand.Intn(len(list))]
	}
	bioTmpl := bioTemplates[rand.Intn(len(bioTemplates))]
	platformUser := username
	if rand.Float32() < 0.3 {
		platformUser = strings.ReplaceAll(platformUser, "_", "") + fmt.Sprint(rand.Intn(99))
	}
	bio := strings.ReplaceAll(bioTmpl, "{platform}", platform)
	bio = strings.ReplaceAll(bio, "{username}", platformUser)
	bio = strings.ReplaceAll(bio, "{Platform}", strings.Title(platform))

	return ProfileData{
		Name:             GenerateName(),
		Username:         username,
		Email:            email,
		Age:              age,
		Gender:           gender,
		LookingFor:       LookingForGuys,
		Country:          country,
		City:             city,
		Platform:         platform,
		PlatformUsername: platformUser,
		Bio:              bio,
		CreatedAt:        time.Now().UTC(),
	}
}

// GenerateEmail creates a random email using temp-mail domain pattern
func GenerateEmail(domain string) string {
	if domain == "" {
		domain = "mail.tm"
	}
	user := strings.ToLower(GenerateUsername()) + fmt.Sprint(rand.Intn(999))
	// ensure RFC compliance
	user = strings.ReplaceAll(user, "_", ".")
	return user + "@" + domain
}

// RandomDelay returns a duration between min and max milliseconds
func RandomDelay(minMs, maxMs int) time.Duration {
	if maxMs <= minMs {
		return time.Duration(minMs) * time.Millisecond
	}
	d := minMs + rand.Intn(maxMs-minMs)
	return time.Duration(d) * time.Millisecond
}

// RandomUserAgent returns a shuffled UA to avoid fingerprinting
func RandomUserAgent() string {
	uagents := []string{
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:133.0) Gecko/20100101 Firefox/133.0",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
		"Mozilla/5.0 (iPhone; CPU iPhone OS 17_2 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.2 Mobile/15E148 Safari/604.1",
	}
	return uagents[rand.Intn(len(uagents))]
}
