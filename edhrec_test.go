package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSanitizeCardName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "simple name",
			input: "Sol Ring",
			want:  "sol-ring",
		},
		{
			name:  "name with comma",
			input: "Atraxa, Praetors' Voice",
			want:  "atraxa-praetors-voice",
		},
		{
			name:  "name with apostrophe",
			input: "Jace's Ingenuity",
			want:  "jaces-ingenuity",
		},
		{
			name:  "name with special characters",
			input: "Teferi, Hero of Dominaria",
			want:  "teferi-hero-of-dominaria",
		},
		{
			name:  "name with multiple spaces",
			input: "Black   Lotus",
			want:  "black-lotus",
		},
		{
			name:  "name with hyphens",
			input: "Will-o'-the-Wisp",
			want:  "will-o-the-wisp",
		},
		{
			name:  "already sanitized",
			input: "lightning-bolt",
			want:  "lightning-bolt",
		},
		{
			name:  "all caps",
			input: "TEFERI",
			want:  "teferi",
		},
		{
			name:  "with numbers",
			input: "Mox Opal 2",
			want:  "mox-opal-2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeCardName(tt.input)
			if got != tt.want {
				t.Errorf("SanitizeCardName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetCommanderRecommendations(t *testing.T) {
	tests := []struct {
		name          string
		commanderName string
		filter        EDHRECRecFilter
		mockResponse  EDHRECResponse
		mockStatus    int
		wantErr       bool
		checkURL      bool
		expectedURL   string
	}{
		{
			name:          "successful request",
			commanderName: "Atraxa, Praetors' Voice",
			mockResponse: EDHRECResponse{
				Container: EDHRECContainer{
					JSONDict: EDHRECData{
						Card: EDHRECCardInfo{
							Name:      "Atraxa, Praetors' Voice",
							Sanitized: "atraxa-praetors-voice",
							ColorID:   []string{"W", "U", "B", "G"},
							NumDecks:  50000,
						},
						CardLists: []EDHRECCardList{
							{
								Header: "High Synergy Cards",
								Tag:    "highsynergy",
								CardViews: []EDHRECCardView{
									{
										Name:      "Doubling Season",
										Sanitized: "doubling-season",
										NumDecks:  25000,
										Synergy:   0.35,
									},
								},
							},
						},
					},
				},
			},
			mockStatus:  http.StatusOK,
			wantErr:     false,
			checkURL:    true,
			expectedURL: "/commanders/atraxa-praetors-voice.json",
		},
		{
			name:          "404 not found",
			commanderName: "Nonexistent Commander",
			mockStatus:    http.StatusNotFound,
			wantErr:       true,
		},
		{
			name:          "500 server error",
			commanderName: "Atraxa",
			mockStatus:    http.StatusInternalServerError,
			wantErr:       true,
		},
		{
			name:          "unfiltered URL",
			commanderName: "Atraxa, Praetors' Voice",
			filter:        EDHRECRecFilter{},
			mockStatus:    http.StatusOK,
			checkURL:      true,
			expectedURL:   "/commanders/atraxa-praetors-voice.json",
		},
		{
			name:          "theme filter composes URL",
			commanderName: "Atraxa, Praetors' Voice",
			filter:        EDHRECRecFilter{Theme: "infect"},
			mockStatus:    http.StatusOK,
			checkURL:      true,
			expectedURL:   "/commanders/atraxa-praetors-voice/infect.json",
		},
		{
			name:          "price tier filter composes URL",
			commanderName: "Atraxa, Praetors' Voice",
			filter:        EDHRECRecFilter{PriceTier: "budget"},
			mockStatus:    http.StatusOK,
			checkURL:      true,
			expectedURL:   "/commanders/atraxa-praetors-voice/budget.json",
		},
		{
			name:          "theme and price tier compose in theme-then-tier order",
			commanderName: "Atraxa, Praetors' Voice",
			filter:        EDHRECRecFilter{Theme: "infect", PriceTier: "budget"},
			mockStatus:    http.StatusOK,
			checkURL:      true,
			expectedURL:   "/commanders/atraxa-praetors-voice/infect/budget.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.checkURL && !strings.HasSuffix(r.URL.Path, tt.expectedURL) {
					t.Errorf("Request URL = %v, want suffix %v", r.URL.Path, tt.expectedURL)
				}

				w.WriteHeader(tt.mockStatus)
				if tt.mockStatus == http.StatusOK {
					_ = json.NewEncoder(w).Encode(tt.mockResponse)
				}
			}))
			defer server.Close()

			ctx := context.Background()
			got, err := getCommanderPageWithURL(ctx, tt.commanderName, tt.filter, server.URL)

			if (err != nil) != tt.wantErr {
				t.Errorf("getCommanderPageWithURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && got != nil {
				if got.Container.JSONDict.Card.Name != tt.mockResponse.Container.JSONDict.Card.Name {
					t.Errorf("getCommanderPageWithURL() card name = %v, want %v",
						got.Container.JSONDict.Card.Name, tt.mockResponse.Container.JSONDict.Card.Name)
				}
			}
		})
	}
}

func TestGetSetCardsWithURL(t *testing.T) {
	tests := []struct {
		name        string
		setCode     string
		mockStatus  int
		wantErr     bool
		expectedURL string
	}{
		{
			name:        "lowercase code",
			setCode:     "rna",
			mockStatus:  http.StatusOK,
			expectedURL: "/sets/rna.json",
		},
		{
			name:        "uppercase code is lowercased",
			setCode:     "RNA",
			mockStatus:  http.StatusOK,
			expectedURL: "/sets/rna.json",
		},
		{
			name:        "whitespace is trimmed",
			setCode:     " rna ",
			mockStatus:  http.StatusOK,
			expectedURL: "/sets/rna.json",
		},
		{
			name:       "403 forbidden yields an error",
			setCode:    "notaset",
			mockStatus: http.StatusForbidden,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.expectedURL != "" && !strings.HasSuffix(r.URL.Path, tt.expectedURL) {
					t.Errorf("Request URL = %v, want suffix %v", r.URL.Path, tt.expectedURL)
				}

				w.WriteHeader(tt.mockStatus)
				if tt.mockStatus == http.StatusOK {
					_ = json.NewEncoder(w).Encode(EDHRECResponse{Header: "Ravnica Allegiance"})
				}
			}))
			defer server.Close()

			_, err := getSetCardsWithURL(context.Background(), tt.setCode, server.URL)
			if (err != nil) != tt.wantErr {
				t.Errorf("getSetCardsWithURL() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestResolveComboSlug(t *testing.T) {
	resolves := []struct {
		colors string
		want   string
	}{
		{"wu", "azorius"},
		{"UW", "azorius"},
		{" ub ", "dimir"},
		{"ubrg", "glint-eye"},
		{"GRBU", "glint-eye"},
		{"wubrg", "five-color"},
		{"c", "colorless"},
		{"C", "colorless"},
		{"azorius", "azorius"},
		{"Mono-White", "mono-white"},
		{"five-color", "five-color"},
		{"colorless", "colorless"},
		{"gur", "temur"},
	}
	for _, tt := range resolves {
		t.Run("resolves "+tt.colors, func(t *testing.T) {
			got, err := resolveComboSlug(tt.colors)
			if err != nil {
				t.Fatalf("resolveComboSlug(%q) error = %v", tt.colors, err)
			}
			if got != tt.want {
				t.Errorf("resolveComboSlug(%q) = %q, want %q", tt.colors, got, tt.want)
			}
		})
	}

	for _, colors := range []string{"", "   ", "xyz", "wwu", "cw", "w u", "azorius-x"} {
		t.Run(fmt.Sprintf("rejects %q", colors), func(t *testing.T) {
			got, err := resolveComboSlug(colors)
			if err == nil {
				t.Fatalf("resolveComboSlug(%q) = %q, want error", colors, got)
			}
			if !strings.Contains(err.Error(), fmt.Sprintf("%q", colors)) {
				t.Errorf("error does not quote the input %q: %v", colors, err)
			}
			if !strings.Contains(err.Error(), "glint-eye") {
				t.Errorf("error does not list the valid names: %v", err)
			}
		})
	}
}

// TestColorIdentitiesMatchEDHRECIndex checks the identity table against independent oracles: the
// slugs linked from pages/combos.json (snapshot 2026-10-04), every WUBRG subset plus "c", and the
// top-cards slug rule (EDHREC's top pages drop the "mono-" prefix of the five mono-colour names).
func TestColorIdentitiesMatchEDHRECIndex(t *testing.T) {
	indexSlugs := []string{
		"abzan", "azorius", "bant", "boros", "colorless", "dimir", "dune-brood", "esper", "five-color",
		"glint-eye", "golgari", "grixis", "gruul", "ink-treader", "izzet", "jeskai", "jund", "mardu",
		"mono-black", "mono-blue", "mono-green", "mono-red", "mono-white", "naya", "orzhov", "rakdos",
		"selesnya", "simic", "sultai", "temur", "witch-maw", "yore-tiller",
	}

	wantLetters := map[string]bool{"c": true}
	for mask := 1; mask < 1<<len(wubrgOrder); mask++ {
		var letters strings.Builder
		for i, letter := range wubrgOrder {
			if mask&(1<<i) != 0 {
				letters.WriteRune(letter)
			}
		}
		wantLetters[letters.String()] = true
	}

	identities := colorIdentities()
	gotSlugs := map[string]bool{}
	gotLetters := map[string]bool{}
	for _, identity := range identities {
		if gotSlugs[identity.name] {
			t.Errorf("duplicate name %q", identity.name)
		}
		if gotLetters[identity.letters] {
			t.Errorf("duplicate letters %q", identity.letters)
		}
		gotSlugs[identity.name] = true
		gotLetters[identity.letters] = true

		if wantTop := strings.TrimPrefix(identity.name, "mono-"); identity.topSlug != wantTop {
			t.Errorf("identity %q has top slug %q, want %q", identity.name, identity.topSlug, wantTop)
		}
	}

	if len(identities) != len(indexSlugs) {
		t.Errorf("table has %d identities, EDHREC index has %d", len(identities), len(indexSlugs))
	}
	for _, slug := range indexSlugs {
		if !gotSlugs[slug] {
			t.Errorf("EDHREC slug %q missing from the table", slug)
		}
	}
	for letters := range wantLetters {
		if !gotLetters[letters] {
			t.Errorf("canonical letters %q missing from the table", letters)
		}
	}
	for letters := range gotLetters {
		if !wantLetters[letters] {
			t.Errorf("table key %q is not canonical WUBRG letters", letters)
		}
	}
}

func TestTopCardsSlugFromArgs(t *testing.T) {
	resolves := []struct {
		name string
		args map[string]any
		want string
	}{
		{"list salt", map[string]any{"list": "salt"}, "salt"},
		{"list trimmed and lowercased", map[string]any{"list": " Creatures "}, "creatures"},
		{"list year", map[string]any{"list": "year"}, "year"},
		{"color letters", map[string]any{"color": "wu"}, "azorius"},
		{"color mono letter", map[string]any{"color": "W"}, "white"},
		{"color mono name", map[string]any{"color": "mono-white"}, "white"},
		{"color colorless", map[string]any{"color": "c"}, "colorless"},
		{"color multicolor", map[string]any{"color": "Multicolor"}, "multicolor"},
		{"color five-color", map[string]any{"color": "five-color"}, "five-color"},
		{"color unordered letters", map[string]any{"color": "gur"}, "temur"},
	}
	for _, tt := range resolves {
		t.Run("resolves "+tt.name, func(t *testing.T) {
			got, err := topCardsSlugFromArgs(tt.args)
			if err != nil {
				t.Fatalf("topCardsSlugFromArgs(%v) error = %v", tt.args, err)
			}
			if got != tt.want {
				t.Errorf("topCardsSlugFromArgs(%v) = %q, want %q", tt.args, got, tt.want)
			}
		})
	}

	rejects := []struct {
		name         string
		args         map[string]any
		wantContains []string
	}{
		{"neither", map[string]any{}, nil},
		{"both", map[string]any{"list": "salt", "color": "wu"}, nil},
		{"unknown list", map[string]any{"list": "bogus"}, []string{`"bogus"`, "game-changers"}},
		{"unknown color", map[string]any{"color": "xyz"}, []string{`"color"`, `"xyz"`, "glint-eye", "multicolor"}},
		{"list wrong type", map[string]any{"list": float64(42)}, nil},
		{"color wrong type", map[string]any{"color": true}, nil},
		{"blank color", map[string]any{"color": "   "}, nil},
	}
	for _, tt := range rejects {
		t.Run("rejects "+tt.name, func(t *testing.T) {
			got, err := topCardsSlugFromArgs(tt.args)
			if err == nil {
				t.Fatalf("topCardsSlugFromArgs(%v) = %q, want error", tt.args, got)
			}
			for _, want := range tt.wantContains {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error does not contain %s: %v", want, err)
				}
			}
		})
	}
}

func TestGetTopCardsPageWithURL(t *testing.T) {
	t.Run("requests the slug page and decodes it", func(t *testing.T) {
		mock := EDHRECResponse{
			Header: "Top Azorius Cards",
			Container: EDHRECContainer{
				JSONDict: EDHRECData{
					CardLists: []EDHRECCardList{
						{Header: "Top Cards", CardViews: []EDHRECCardView{{Name: "Sol Ring"}}},
						{Header: "Creatures", CardViews: []EDHRECCardView{{Name: "Esper Sentinel"}}},
					},
				},
			},
		}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasSuffix(r.URL.Path, "/top/azorius.json") {
				t.Errorf("Request URL = %v, want suffix /top/azorius.json", r.URL.Path)
			}
			_ = json.NewEncoder(w).Encode(mock)
		}))
		defer server.Close()

		page, err := getTopCardsPageWithURL(context.Background(), "azorius", server.URL)
		if err != nil {
			t.Fatalf("getTopCardsPageWithURL() error = %v", err)
		}
		if page.Header != "Top Azorius Cards" {
			t.Errorf("header = %q, want %q", page.Header, "Top Azorius Cards")
		}
		lists := page.Container.JSONDict.CardLists
		if len(lists) != 2 || lists[1].Header != "Creatures" || lists[1].CardViews[0].Name != "Esper Sentinel" {
			t.Errorf("cardlists not decoded: %+v", lists)
		}
	})

	t.Run("403 returns an error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		}))
		defer server.Close()

		if _, err := getTopCardsPageWithURL(context.Background(), "mono-white", server.URL); err == nil {
			t.Error("expected an error for a 403 page")
		}
	})
}

func TestGetCombosForIdentityWithURL(t *testing.T) {
	t.Run("requests the slug page and decodes it", func(t *testing.T) {
		mock := EDHRECComboResponse{
			Container: EDHRECComboContainer{
				JSONDict: EDHRECComboData{
					CardLists: []EDHRECComboList{
						{Header: "Combo A", CardViews: []EDHRECCardView{{Name: "Card A"}}},
						{Header: "Combo B", CardViews: []EDHRECCardView{{Name: "Card B"}}},
					},
				},
			},
		}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasSuffix(r.URL.Path, "/combos/azorius.json") {
				t.Errorf("Request URL = %v, want suffix /combos/azorius.json", r.URL.Path)
			}
			_ = json.NewEncoder(w).Encode(mock)
		}))
		defer server.Close()

		got, err := getCombosForIdentityWithURL(context.Background(), "azorius", server.URL)
		if err != nil {
			t.Fatalf("getCombosForIdentityWithURL() error = %v", err)
		}
		if len(got.CardLists) != len(mock.Container.JSONDict.CardLists) {
			t.Errorf("combo count = %d, want %d", len(got.CardLists), len(mock.Container.JSONDict.CardLists))
		}
	})

	t.Run("upstream 403 is an error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		}))
		defer server.Close()

		if _, err := getCombosForIdentityWithURL(context.Background(), "azorius", server.URL); err == nil {
			t.Error("expected error for HTTP 403")
		}
	})
}

func TestFormatCommanderRecsForDisplay(t *testing.T) {
	page := &EDHRECResponse{
		Container: EDHRECContainer{
			JSONDict: EDHRECData{
				Card: EDHRECCardInfo{
					Name:     "Test Commander",
					ColorID:  []string{"W", "U"},
					NumDecks: 1000,
				},
				CardLists: []EDHRECCardList{
					{
						Header: "High Synergy Cards",
						CardViews: []EDHRECCardView{
							{
								Name:     "Card 1",
								NumDecks: 500,
								Synergy:  0.35,
								Salt:     1.5,
							},
							{
								Name:     "Card 2",
								NumDecks: 400,
								Synergy:  0.25,
							},
						},
					},
				},
			},
		},
	}

	tests := []struct {
		name          string
		page          *EDHRECResponse
		limit         int
		wantContains  []string
		wantCardCount int
	}{
		{
			name:  "with limit",
			page:  page,
			limit: 1,
			wantContains: []string{
				"Test Commander",
				"High Synergy Cards",
				"Card 1",
				"Synergy:",
				"Salt Score:",
			},
			wantCardCount: 1,
		},
		{
			name:  "without limit",
			page:  page,
			limit: 0,
			wantContains: []string{
				"Test Commander",
				"Card 1",
				"Card 2",
			},
			wantCardCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatCommanderRecsForDisplay(tt.page, EDHRECRecFilter{}, tt.limit)

			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("FormatCommanderRecsForDisplay() missing %q in output", want)
				}
			}

			// Check if the correct number of cards are shown
			if tt.limit > 0 {
				card2Count := strings.Count(got, "Card 2")
				if tt.wantCardCount == 1 && card2Count > 0 {
					t.Error("FormatCommanderRecsForDisplay() should limit cards but found Card 2")
				}
			}
		})
	}
}

func TestFormatCommanderRecsForDisplayDeckStats(t *testing.T) {
	t.Run("nonzero total decks shows percentage", func(t *testing.T) {
		page := &EDHRECResponse{
			Container: EDHRECContainer{
				JSONDict: EDHRECData{
					Card: EDHRECCardInfo{Name: "Test Commander", NumDecks: 1000},
					CardLists: []EDHRECCardList{
						{Header: "Top Cards", CardViews: []EDHRECCardView{{Name: "Sol Ring", NumDecks: 500}}},
					},
				},
			},
		}
		got := FormatCommanderRecsForDisplay(page, EDHRECRecFilter{}, 10)
		if !strings.Contains(got, "Decks: 500 of 1000 (50.0%)") {
			t.Errorf("expected 50.0%% deck stat, got:\n%s", got)
		}
		if strings.Contains(got, "NaN") {
			t.Errorf("output must never contain NaN, got:\n%s", got)
		}
	})

	t.Run("zero total decks shows raw count only", func(t *testing.T) {
		page := &EDHRECResponse{
			Container: EDHRECContainer{
				JSONDict: EDHRECData{
					Card: EDHRECCardInfo{Name: "Test Commander", NumDecks: 0},
					CardLists: []EDHRECCardList{
						{Header: "Top Cards", CardViews: []EDHRECCardView{{Name: "Sol Ring", NumDecks: 500}}},
					},
				},
			},
		}
		got := FormatCommanderRecsForDisplay(page, EDHRECRecFilter{}, 10)
		if !strings.Contains(got, "Decks: 500") {
			t.Errorf("expected raw deck count, got:\n%s", got)
		}
		if strings.Contains(got, "NaN") || strings.Contains(got, "%") {
			t.Errorf("zero denominator must not print a percentage, got:\n%s", got)
		}
	})
}

func TestFormatCommanderRecsForDisplayThemes(t *testing.T) {
	tags := make([]EDHRECTagCount, 20)
	for i := range tags {
		tags[i] = EDHRECTagCount{
			Slug: fmt.Sprintf("theme-%02d", i), Value: fmt.Sprintf("Theme %02d", i), Count: 100 - i,
		}
	}

	page := &EDHRECResponse{
		Header:    "Test Commander (Commander)",
		TagCounts: tags,
		Container: EDHRECContainer{
			JSONDict: EDHRECData{Card: EDHRECCardInfo{Name: "Test Commander"}},
		},
	}

	t.Run("unfiltered shows available themes", func(t *testing.T) {
		got := FormatCommanderRecsForDisplay(page, EDHRECRecFilter{}, 10)
		if !strings.Contains(got, "## Available Themes") {
			t.Error("expected Available Themes section")
		}
		if !strings.Contains(got, "theme-00") {
			t.Error("expected highest-count theme slug")
		}
		if !strings.Contains(got, "and 5 more themes") {
			t.Errorf("expected omitted-theme count, got:\n%s", got)
		}
		if strings.Contains(got, "theme-15") {
			t.Error("16th theme slug should not be listed")
		}
	})

	t.Run("filtered suppresses available themes", func(t *testing.T) {
		got := FormatCommanderRecsForDisplay(page, EDHRECRecFilter{Theme: "infect"}, 10)
		if strings.Contains(got, "## Available Themes") {
			t.Error("Available Themes section should be suppressed when a filter is active")
		}
	})
}

func TestFormatCommanderRecsForDisplayHeader(t *testing.T) {
	t.Run("uses page header when present", func(t *testing.T) {
		page := &EDHRECResponse{
			Header: "X (Commander) - Budget Infect",
			Container: EDHRECContainer{
				JSONDict: EDHRECData{Card: EDHRECCardInfo{Name: "X"}},
			},
		}
		got := FormatCommanderRecsForDisplay(page, EDHRECRecFilter{Theme: "infect", PriceTier: "budget"}, 10)
		if !strings.Contains(got, "X (Commander) - Budget Infect") {
			t.Errorf("expected page header in title, got:\n%s", got)
		}
	})

	t.Run("falls back to card name when header is empty", func(t *testing.T) {
		page := &EDHRECResponse{
			Container: EDHRECContainer{
				JSONDict: EDHRECData{Card: EDHRECCardInfo{Name: "Fallback Commander"}},
			},
		}
		got := FormatCommanderRecsForDisplay(page, EDHRECRecFilter{}, 10)
		if !strings.Contains(got, "Fallback Commander") {
			t.Errorf("expected card name fallback in title, got:\n%s", got)
		}
	})
}

func TestFormatSetCardsForDisplay(t *testing.T) {
	t.Run("commander and card views", func(t *testing.T) {
		page := &EDHRECResponse{
			Header: "Ravnica Allegiance",
			Container: EDHRECContainer{
				JSONDict: EDHRECData{
					CardLists: []EDHRECCardList{
						{
							Header:    "Commanders",
							CardViews: []EDHRECCardView{{Name: "Teysa Karlov", NumDecks: 21469, PotentialDecks: 0}},
						},
						{
							Header: "Cards",
							CardViews: []EDHRECCardView{
								{Name: "Smothering Tithe", NumDecks: 500, PotentialDecks: 2000},
							},
						},
					},
				},
			},
		}
		got := FormatSetCardsForDisplay(page, "rna", 25)
		if !strings.Contains(got, "(rna)") {
			t.Errorf("expected set code in title, got:\n%s", got)
		}
		if strings.Contains(got, "Decks: 21469 of") || !strings.Contains(got, "Decks: 21469\n") {
			t.Errorf("commander view should show a bare deck count, got:\n%s", got)
		}
		if !strings.Contains(got, "Decks: 500 of 2000 (25.0%)") {
			t.Errorf("card view should show a percentage, got:\n%s", got)
		}
	})

	t.Run("truncation and unlimited", func(t *testing.T) {
		page := &EDHRECResponse{
			Container: EDHRECContainer{
				JSONDict: EDHRECData{
					CardLists: []EDHRECCardList{
						{
							Header: "Cards",
							CardViews: []EDHRECCardView{
								{Name: "Card One"},
								{Name: "Card Two"},
								{Name: "Card Three"},
							},
						},
					},
				},
			},
		}

		limited := FormatSetCardsForDisplay(page, "rna", 1)
		if !strings.Contains(limited, "and 2 more cards") {
			t.Errorf("expected truncation suffix, got:\n%s", limited)
		}

		unlimited := FormatSetCardsForDisplay(page, "rna", 0)
		for _, want := range []string{"Card One", "Card Two", "Card Three"} {
			if !strings.Contains(unlimited, want) {
				t.Errorf("limit 0 should show all cards, missing %q in:\n%s", want, unlimited)
			}
		}
		if strings.Contains(unlimited, "more cards") {
			t.Errorf("limit 0 should not truncate, got:\n%s", unlimited)
		}
	})
}

func TestFormatCombosForDisplay(t *testing.T) {
	data := &EDHRECComboData{
		CardLists: []EDHRECComboList{
			{
				Header: "Card A + Card B (1000 decks)",
				CardViews: []EDHRECCardView{
					{Name: "Card A"},
					{Name: "Card B"},
				},
				Combo: &EDHRECCombo{
					ComboID: "123-456",
					Results: []string{"Win the game", "Infinite mana"},
				},
			},
			{
				Header: "Card C + Card D (500 decks)",
				CardViews: []EDHRECCardView{
					{Name: "Card C"},
					{Name: "Card D"},
				},
				Combo: &EDHRECCombo{
					ComboID: "789-012",
					Results: []string{"Infinite tokens"},
				},
			},
		},
	}

	tests := []struct {
		name         string
		data         *EDHRECComboData
		limit        int
		wantContains []string
	}{
		{
			name:  "with limit",
			data:  data,
			limit: 1,
			wantContains: []string{
				"Popular Combos",
				"Card A + Card B",
				"1000 decks",
				"Win the game",
			},
		},
		{
			name:  "without limit",
			data:  data,
			limit: 0,
			wantContains: []string{
				"Card A + Card B",
				"Card C + Card D",
				"500 decks",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatCombosForDisplay(tt.data, tt.limit)

			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("FormatCombosForDisplay() missing %q in output", want)
				}
			}

			if tt.limit == 1 && strings.Contains(got, "Card C + Card D") {
				t.Error("FormatCombosForDisplay() should limit combos but found second combo")
			}
		})
	}
}

func TestFormatTopCardsForDisplay(t *testing.T) {
	colorPage := &EDHRECResponse{
		Header: "Top Azorius Cards",
		Container: EDHRECContainer{
			JSONDict: EDHRECData{
				CardLists: []EDHRECCardList{
					{
						Header: "Top Cards",
						CardViews: []EDHRECCardView{
							{Name: "Swords to Plowshares", NumDecks: 603382, PotentialDecks: 2339647},
							{Name: "Counterspell", NumDecks: 461071, PotentialDecks: 2339647},
							{Name: "Teferi's Protection", NumDecks: 444642, PotentialDecks: 2339647},
						},
					},
					{
						Header:    "Creatures",
						CardViews: []EDHRECCardView{{Name: "Esper Sentinel", NumDecks: 1000, PotentialDecks: 4000}},
					},
					{Header: "Empty", CardViews: []EDHRECCardView{}},
				},
			},
		},
	}

	t.Run("colour page with limit", func(t *testing.T) {
		got := FormatTopCardsForDisplay(colorPage, "azorius", 2)
		for _, want := range []string{
			"# EDHREC Top Azorius Cards (azorius)",
			"## Top Cards (3 cards)",
			"Swords to Plowshares",
			"Counterspell",
			"Decks: 603382 of 2339647 (25.8%)",
			"*...and 1 more cards*",
			"## Creatures (1 cards)",
			"Esper Sentinel",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("missing %q in:\n%s", want, got)
			}
		}
		for _, unwanted := range []string{"Teferi's Protection", "## Empty", "NaN"} {
			if strings.Contains(got, unwanted) {
				t.Errorf("unexpected %q in:\n%s", unwanted, got)
			}
		}
	})

	t.Run("salt page with unheaded list", func(t *testing.T) {
		page := &EDHRECResponse{
			Header: "Top 100 Saltiest Cards",
			Container: EDHRECContainer{
				JSONDict: EDHRECData{
					CardLists: []EDHRECCardList{
						{
							Header: "",
							CardViews: []EDHRECCardView{
								{Name: "Stasis", NumDecks: 18097, PotentialDecks: 0, Salt: 3.0572},
							},
						},
					},
				},
			},
		}
		got := FormatTopCardsForDisplay(page, "salt", 10)
		for _, want := range []string{"- Decks: 18097\n", "Salt Score: 3.06/4.0"} {
			if !strings.Contains(got, want) {
				t.Errorf("missing %q in:\n%s", want, got)
			}
		}
		for _, line := range strings.Split(got, "\n") {
			if strings.HasPrefix(line, "## ") {
				t.Errorf("unheaded list must not get a heading, got line %q in:\n%s", line, got)
			}
		}
	})

	t.Run("limit 0 shows all cards", func(t *testing.T) {
		got := FormatTopCardsForDisplay(colorPage, "azorius", 0)
		if !strings.Contains(got, "Teferi's Protection") {
			t.Errorf("limit 0 should show every card, got:\n%s", got)
		}
		if strings.Contains(got, "more cards") {
			t.Errorf("limit 0 should not truncate, got:\n%s", got)
		}
	})
}

func TestFormatCommanderRecsForDisplayEdgeCases(t *testing.T) {
	t.Run("empty card list skipped", func(t *testing.T) {
		page := &EDHRECResponse{
			Container: EDHRECContainer{
				JSONDict: EDHRECData{
					Card: EDHRECCardInfo{Name: "Test Commander", NumDecks: 100},
					CardLists: []EDHRECCardList{
						{Header: "Skipped Section", CardViews: []EDHRECCardView{}},
						{Header: "Non-Empty Section", CardViews: []EDHRECCardView{
							{Name: "Sol Ring", NumDecks: 90},
						}},
					},
				},
			},
		}
		got := FormatCommanderRecsForDisplay(page, EDHRECRecFilter{}, 10)
		if strings.Contains(got, "Skipped Section") {
			t.Error("FormatCommanderRecsForDisplay() should skip card lists with no cards")
		}
		if !strings.Contains(got, "Non-Empty Section") {
			t.Error("FormatCommanderRecsForDisplay() missing non-empty section")
		}
	})

	t.Run("zero synergy and zero salt omitted", func(t *testing.T) {
		page := &EDHRECResponse{
			Container: EDHRECContainer{
				JSONDict: EDHRECData{
					Card: EDHRECCardInfo{Name: "Test Commander", NumDecks: 100},
					CardLists: []EDHRECCardList{
						{Header: "Top Cards", CardViews: []EDHRECCardView{
							{Name: "Sol Ring", NumDecks: 90, Synergy: 0, Salt: 0},
						}},
					},
				},
			},
		}
		got := FormatCommanderRecsForDisplay(page, EDHRECRecFilter{}, 10)
		if strings.Contains(got, "Synergy:") {
			t.Error("FormatCommanderRecsForDisplay() should omit Synergy line when zero")
		}
		if strings.Contains(got, "Salt Score:") {
			t.Error("FormatCommanderRecsForDisplay() should omit Salt Score line when zero")
		}
	})

	t.Run("truncation suffix shown", func(t *testing.T) {
		page := &EDHRECResponse{
			Container: EDHRECContainer{
				JSONDict: EDHRECData{
					Card: EDHRECCardInfo{Name: "Test Commander", NumDecks: 100},
					CardLists: []EDHRECCardList{
						{Header: "Top Cards", CardViews: []EDHRECCardView{
							{Name: "Sol Ring", NumDecks: 90},
							{Name: "Arcane Signet", NumDecks: 80},
							{Name: "Command Tower", NumDecks: 70},
						}},
					},
				},
			},
		}
		got := FormatCommanderRecsForDisplay(page, EDHRECRecFilter{}, 1)
		if !strings.Contains(got, "and 2 more cards") {
			t.Errorf("FormatCommanderRecsForDisplay() missing truncation suffix, got:\n%s", got)
		}
	})
}

func TestRecFilterFromArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    map[string]any
		want    EDHRECRecFilter
		wantErr bool
	}{
		{
			name: "no args",
			args: map[string]any{},
			want: EDHRECRecFilter{},
		},
		{
			name: "price tier is lowercased",
			args: map[string]any{"price_tier": "Budget"},
			want: EDHRECRecFilter{PriceTier: "budget"},
		},
		{
			name:    "invalid price tier names both valid values",
			args:    map[string]any{"price_tier": "middle"},
			wantErr: true,
		},
		{
			name:    "wrong-typed price tier is an error",
			args:    map[string]any{"price_tier": 42},
			wantErr: true,
		},
		{
			name: "theme is trimmed and lowercased",
			args: map[string]any{"theme": " Infect "},
			want: EDHRECRecFilter{Theme: "infect"},
		},
		{
			name:    "wrong-typed theme is an error",
			args:    map[string]any{"theme": true},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := recFilterFromArgs(tt.args)
			if (err != nil) != tt.wantErr {
				t.Fatalf("recFilterFromArgs() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				if tt.name == "invalid price tier names both valid values" {
					missingBudget := !strings.Contains(err.Error(), priceTierBudget)
					missingExpensive := !strings.Contains(err.Error(), priceTierExpensive)
					if missingBudget || missingExpensive {
						t.Errorf("error should name both valid tiers, got: %v", err)
					}
				}
				return
			}
			if got != tt.want {
				t.Errorf("recFilterFromArgs() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
