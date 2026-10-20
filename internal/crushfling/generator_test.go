package crushfling

// The operator's brief fixes the persona: a female profile, looking for men,
// in one of five Western markets. Those are not random choices, so they are
// asserted rather than left to chance.

import (
	"strings"
	"testing"
)

func TestGeneratedProfilesMatchTheBrief(t *testing.T) {
	allowed := map[string]bool{}
	for _, code := range countries {
		allowed[code] = true
	}
	// USA plus Europe, exactly the brief: US must be present and every other
	// code must be a European country we listed.
	if !allowed["US"] {
		t.Fatal("US missing from the country list")
	}
	for code := range allowed {
		if code == "US" {
			continue
		}
		if _, ok := cities[code]; !ok {
			t.Fatalf("country %q has no city list", code)
		}
	}
	seen := map[string]int{}
	for i := 0; i < 500; i++ {
		p := GenerateProfile("x@example.test")
		if p.Gender != GenderWoman {
			t.Fatalf("profile %d: gender = %q, want %q", i, p.Gender, GenderWoman)
		}
		if p.LookingFor != LookingForGuys {
			t.Fatalf("profile %d: looking_for = %q, want %q", i, p.LookingFor, LookingForGuys)
		}
		if !allowed[p.Country] {
			t.Fatalf("profile %d: country = %q, outside USA+Europe", i, p.Country)
		}
		if p.City == "" || cities[p.Country] == nil {
			t.Fatalf("profile %d: empty city for %s", i, p.Country)
		}
		seen[p.Country]++
		if p.Age < 19 || p.Age > 24 {
			t.Fatalf("profile %d: age = %d, outside 19-24", i, p.Age)
		}
		if p.Name == "" || !strings.Contains(p.Name, " ") {
			t.Fatalf("profile %d: name %q is not a display name", i, p.Name)
		}
		if p.Username == "" || p.Username != lowerOnly(p.Username) {
			t.Fatalf("profile %d: username %q is not lowercase", i, p.Username)
		}
		if p.Bio == "" || p.PlatformUsername == "" {
			t.Fatalf("profile %d: incomplete profile %+v", i, p)
		}
	}
	// 500 draws over 30 countries: a deterministic-but-tiny list would show
	// up as a handful of countries. Require a healthy spread.
	if len(seen) < 15 {
		t.Fatalf("only %d country/countries used in 500 profiles: %v", len(seen), seen)
	}
	if seen["US"] == 0 {
		t.Fatal("US never generated in 500 profiles")
	}
}

func lowerOnly(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r >= 'A' && r <= 'Z' {
			r += 'a' - 'A'
		}
		out = append(out, r)
	}
	return string(out)
}

func TestProfilePayloadCarriesBothKeyStyles(t *testing.T) {
	p := ProfileData{
		Username: "alice", Age: 24, Gender: GenderWoman, LookingFor: LookingForGuys,
		Country: "GB", City: "London", Platform: PlatformTelegram,
		PlatformUsername: "alice_tg", Bio: "hi",
	}
	payload := profilePayload(p, "https://s3.example/avatar.webp")

	want := map[string]any{
		"username":          "alice",
		"age":               24,
		"gender":            GenderWoman,
		"lookingFor":        LookingForGuys,
		"looking_for":       LookingForGuys,
		"country":           "GB",
		"city":              "London",
		"platform":          PlatformTelegram,
		"platformUsername":  "alice_tg",
		"platform_username": "alice_tg",
		"bio":               "hi",
		"avatarUrl":         "https://s3.example/avatar.webp",
		"avatar_url":        "https://s3.example/avatar.webp",
	}
	for k, v := range want {
		if payload[k] != v {
			t.Errorf("payload[%q] = %v, want %v", k, payload[k], v)
		}
	}

	// Optional fields must disappear rather than be sent as empty strings:
	// Laravel's `sometimes|url` rules reject "".
	bare := profilePayload(ProfileData{Username: "bob", Age: 30, Gender: GenderWoman, LookingFor: LookingForGuys, Country: "US", Platform: PlatformKik, PlatformUsername: "bob", Bio: "x"}, "")
	for _, k := range []string{"city", "avatarUrl", "avatar_url"} {
		if _, ok := bare[k]; ok {
			t.Errorf("payload unexpectedly contains %q for an empty value", k)
		}
	}
}
