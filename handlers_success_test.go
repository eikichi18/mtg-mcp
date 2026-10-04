package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// jsonServer starts a test server that replies to every request with the given value as JSON.
func jsonServer(t *testing.T, status int, body any) string {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		if status == http.StatusOK {
			_ = json.NewEncoder(w).Encode(body)
		}
	}))
	t.Cleanup(ts.Close)
	return ts.URL
}

func TestHandleGetMoxfieldDeckSuccess(t *testing.T) {
	deck := MoxfieldDeck{
		PublicID: "abc123",
		Name:     "Atraxa Superfriends",
		Format:   "commander",
		Commanders: map[string]MoxfieldCardEntry{
			"atraxa": {
				Quantity: 1,
				Card:     MoxfieldCardInfo{Name: "Atraxa, Praetors' Voice", TypeLine: "Legendary Creature"},
			},
		},
		Mainboard: map[string]MoxfieldCardEntry{
			"sol": {Quantity: 1, Card: MoxfieldCardInfo{Name: "Sol Ring", TypeLine: "Artifact"}},
		},
	}
	s := &MTGCommanderServer{moxfieldBaseURL: jsonServer(t, http.StatusOK, deck)}

	res, err := s.handleGetMoxfieldDeck(context.Background(), toolRequest(map[string]any{"deck_id": "abc123"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatal("unexpected error result")
	}
	if !strings.Contains(resultText(t, res), "Atraxa Superfriends") {
		t.Errorf("expected deck name in output:\n%s", resultText(t, res))
	}
}

func TestHandleGetMoxfieldDeckFailure(t *testing.T) {
	s := &MTGCommanderServer{moxfieldBaseURL: jsonServer(t, http.StatusInternalServerError, nil)}
	res, _ := s.handleGetMoxfieldDeck(context.Background(), toolRequest(map[string]any{"deck_id": "abc123"}))
	if !res.IsError {
		t.Error("expected error result on fetch failure")
	}
}

func TestHandleGetMoxfieldUserDecksSuccess(t *testing.T) {
	resp := MoxfieldUserDecksResponse{
		PageNumber:   1,
		PageSize:     20,
		TotalResults: 1,
		TotalPages:   1,
		Data: []MoxfieldDeckSummary{
			{
				PublicID:  "d1",
				Name:      "Deck One",
				Format:    "commander",
				PublicURL: "https://moxfield.com/d1",
				ViewCount: 10,
				LikeCount: 2,
			},
		},
	}
	s := &MTGCommanderServer{moxfieldBaseURL: jsonServer(t, http.StatusOK, resp)}

	res, err := s.handleGetMoxfieldUserDecks(context.Background(), toolRequest(map[string]any{
		"username":  "tester",
		"page_size": float64(200), // exercises the max clamp
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := resultText(t, res)
	for _, want := range []string{"Decks by tester", "Deck One", "Deck ID: d1"} {
		if !strings.Contains(text, want) {
			t.Errorf("output missing %q\n%s", want, text)
		}
	}
}

func TestHandleGetMoxfieldUserDecksFailure(t *testing.T) {
	s := &MTGCommanderServer{moxfieldBaseURL: jsonServer(t, http.StatusInternalServerError, nil)}
	res, _ := s.handleGetMoxfieldUserDecks(context.Background(), toolRequest(map[string]any{"username": "tester"}))
	if !res.IsError {
		t.Error("expected error result on fetch failure")
	}
}

func TestHandleSearchMoxfieldDecks(t *testing.T) {
	t.Run("with results and overrides", func(t *testing.T) {
		resp := MoxfieldSearchResponse{
			PageNumber:   1,
			TotalResults: 1,
			TotalPages:   1,
			Data: []MoxfieldDeckSummary{
				{PublicID: "s1", Name: "Found Deck", Format: "commander", PublicURL: "https://moxfield.com/s1"},
			},
		}
		s := &MTGCommanderServer{moxfieldBaseURL: jsonServer(t, http.StatusOK, resp)}
		res, err := s.handleSearchMoxfieldDecks(context.Background(), toolRequest(map[string]any{
			"commander":      "Atraxa",
			"format":         "commander",
			"sort_type":      "views",
			"sort_direction": "Ascending",
			"page_size":      float64(5),
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(resultText(t, res), "Found Deck") {
			t.Errorf("expected deck in output:\n%s", resultText(t, res))
		}
	})

	t.Run("no results", func(t *testing.T) {
		resp := MoxfieldSearchResponse{PageNumber: 1, TotalResults: 0, TotalPages: 0, Data: nil}
		s := &MTGCommanderServer{moxfieldBaseURL: jsonServer(t, http.StatusOK, resp)}
		res, _ := s.handleSearchMoxfieldDecks(context.Background(), toolRequest(map[string]any{"commander": "Nobody"}))
		if !strings.Contains(resultText(t, res), "No decks found") {
			t.Error("expected no-decks message")
		}
	})

	t.Run("failure", func(t *testing.T) {
		s := &MTGCommanderServer{moxfieldBaseURL: jsonServer(t, http.StatusInternalServerError, nil)}
		res, _ := s.handleSearchMoxfieldDecks(context.Background(), toolRequest(map[string]any{"commander": "Atraxa"}))
		if !res.IsError {
			t.Error("expected error result")
		}
	})
}

func sampleArchidektDeck() ArchidektDeck {
	return ArchidektDeck{
		ID:         123,
		Name:       "Test Deck",
		DeckFormat: 3,
		Cards: []ArchidektCardEntry{
			{
				Quantity:   1,
				Categories: []string{"Commander"},
				Card: ArchidektCard{
					OracleCard: ArchidektOracleCard{Name: "Atraxa, Praetors' Voice", Types: []string{"Creature"}},
				},
			},
			{
				Quantity:   1,
				Categories: []string{"Land"},
				Card: ArchidektCard{
					OracleCard: ArchidektOracleCard{Name: "Command Tower", Types: []string{"Land"}},
				},
			},
		},
	}
}

func TestHandleGetArchidektDeckSuccess(t *testing.T) {
	url := jsonServer(t, http.StatusOK, sampleArchidektDeck())

	t.Run("full deck", func(t *testing.T) {
		s := &MTGCommanderServer{archidektBaseURL: url}
		res, err := s.handleGetArchidektDeck(context.Background(), toolRequest(map[string]any{"deck_id": "123"}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(resultText(t, res), "Test Deck") {
			t.Errorf("expected deck name in output:\n%s", resultText(t, res))
		}
	})

	t.Run("lands only", func(t *testing.T) {
		s := &MTGCommanderServer{archidektBaseURL: url}
		res, err := s.handleGetArchidektDeck(context.Background(), toolRequest(map[string]any{
			"deck_id":    "123",
			"lands_only": true,
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(resultText(t, res), "Command Tower") {
			t.Errorf("expected land in lands-only output:\n%s", resultText(t, res))
		}
	})
}

func TestHandleGetArchidektDeckFailure(t *testing.T) {
	s := &MTGCommanderServer{archidektBaseURL: jsonServer(t, http.StatusInternalServerError, nil)}
	res, _ := s.handleGetArchidektDeck(context.Background(), toolRequest(map[string]any{"deck_id": "123"}))
	if !res.IsError {
		t.Error("expected error result on fetch failure")
	}
}

func TestHandleGetArchidektUserDecks(t *testing.T) {
	t.Run("with results and pagination", func(t *testing.T) {
		resp := ArchidektUserDecksResponse{
			Count: 2,
			Next:  "https://archidekt.com/api/decks/v3/?page=2",
			Results: []ArchidektDeckSummary{
				{ID: 1, Name: "Deck A", DeckFormat: 3, ViewCount: 5, UpdatedAt: "2024-01-01"},
			},
		}
		s := &MTGCommanderServer{archidektBaseURL: jsonServer(t, http.StatusOK, resp)}
		res, err := s.handleGetArchidektUserDecks(context.Background(), toolRequest(map[string]any{
			"username": "tester",
			"page":     float64(1),
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		text := resultText(t, res)
		for _, want := range []string{"Archidekt Decks by tester", "Deck A", "use page 2 to continue"} {
			if !strings.Contains(text, want) {
				t.Errorf("output missing %q\n%s", want, text)
			}
		}
	})

	t.Run("no results", func(t *testing.T) {
		resp := ArchidektUserDecksResponse{Count: 0, Next: "", Results: nil}
		s := &MTGCommanderServer{archidektBaseURL: jsonServer(t, http.StatusOK, resp)}
		res, _ := s.handleGetArchidektUserDecks(context.Background(), toolRequest(map[string]any{"username": "tester"}))
		if !strings.Contains(resultText(t, res), "No public decks found") {
			t.Error("expected no-decks message")
		}
	})

	t.Run("failure", func(t *testing.T) {
		s := &MTGCommanderServer{archidektBaseURL: jsonServer(t, http.StatusInternalServerError, nil)}
		res, _ := s.handleGetArchidektUserDecks(context.Background(), toolRequest(map[string]any{"username": "tester"}))
		if !res.IsError {
			t.Error("expected error result")
		}
	})
}

func TestHandleSearchArchidektDecks(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		resp := ArchidektUserDecksResponse{
			Count: 1,
			Results: []ArchidektDeckSummary{
				{ID: 9, Name: "Atraxa Brew", DeckFormat: 3, ViewCount: 99, UpdatedAt: "2024-02-02"},
			},
		}
		s := &MTGCommanderServer{archidektBaseURL: jsonServer(t, http.StatusOK, resp)}
		res, err := s.handleSearchArchidektDecks(context.Background(), toolRequest(map[string]any{
			"commander": "Atraxa, Praetors' Voice",
			"bracket":   float64(4),
			"limit":     float64(5),
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(resultText(t, res), "Atraxa Brew") {
			t.Errorf("expected deck in output:\n%s", resultText(t, res))
		}
	})

	t.Run("failure", func(t *testing.T) {
		s := &MTGCommanderServer{archidektBaseURL: jsonServer(t, http.StatusInternalServerError, nil)}
		res, _ := s.handleSearchArchidektDecks(context.Background(), toolRequest(map[string]any{"commander": "Atraxa"}))
		if !res.IsError {
			t.Error("expected error result")
		}
	})
}

func TestHandleGetEDHRECRecommendations(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		resp := EDHRECResponse{
			Container: EDHRECContainer{
				JSONDict: EDHRECData{
					Card: EDHRECCardInfo{
						Name: "Atraxa, Praetors' Voice", ColorID: []string{"W", "U", "B", "G"}, NumDecks: 1000,
					},
					CardLists: []EDHRECCardList{
						{
							Header:    "High Synergy Cards",
							CardViews: []EDHRECCardView{{Name: "Doubling Season", NumDecks: 500}},
						},
					},
				},
			},
		}
		s := &MTGCommanderServer{edhrecBaseURL: jsonServer(t, http.StatusOK, resp)}
		res, err := s.handleGetEDHRECRecommendations(context.Background(), toolRequest(map[string]any{
			"commander": "Atraxa, Praetors' Voice",
			"limit":     float64(5),
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(resultText(t, res), "Doubling Season") {
			t.Errorf("expected recommendation in output:\n%s", resultText(t, res))
		}
	})

	t.Run("failure", func(t *testing.T) {
		s := &MTGCommanderServer{edhrecBaseURL: jsonServer(t, http.StatusNotFound, nil)}
		res, _ := s.handleGetEDHRECRecommendations(
			context.Background(),
			toolRequest(map[string]any{"commander": "Nobody"}),
		)
		if !res.IsError {
			t.Error("expected error result")
		}
	})
}

func TestHandleGetEDHRECCombos(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		resp := EDHRECComboResponse{
			Container: EDHRECComboContainer{
				JSONDict: EDHRECComboData{
					CardLists: []EDHRECComboList{
						{
							Header:    "Basalt Monolith + Forsaken Monument",
							CardViews: []EDHRECCardView{{Name: "Basalt Monolith"}, {Name: "Forsaken Monument"}},
							Combo:     &EDHRECCombo{ComboID: "c1", Results: []string{"Infinite mana"}},
						},
					},
				},
			},
		}
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasSuffix(r.URL.Path, "/combos/azorius.json") {
				t.Errorf("Request URL = %v, want suffix /combos/azorius.json", r.URL.Path)
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		t.Cleanup(ts.Close)
		s := &MTGCommanderServer{edhrecBaseURL: ts.URL}
		res, err := s.handleGetEDHRECCombos(context.Background(), toolRequest(map[string]any{
			"colors": "UW",
			"limit":  float64(5),
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(resultText(t, res), "Infinite mana") {
			t.Errorf("expected combo in output:\n%s", resultText(t, res))
		}
	})

	t.Run("failure", func(t *testing.T) {
		s := &MTGCommanderServer{edhrecBaseURL: jsonServer(t, http.StatusForbidden, nil)}
		res, _ := s.handleGetEDHRECCombos(context.Background(), toolRequest(map[string]any{"colors": "wu"}))
		if !res.IsError {
			t.Error("expected error result")
		}
	})

	t.Run("invalid colors never reach EDHREC", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Errorf("unexpected EDHREC request %s", r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}))
		t.Cleanup(ts.Close)
		s := &MTGCommanderServer{edhrecBaseURL: ts.URL}

		for _, colors := range []any{"xyz", "wwu", "cw", "   ", float64(42)} {
			res, err := s.handleGetEDHRECCombos(context.Background(), toolRequest(map[string]any{"colors": colors}))
			if err != nil {
				t.Fatalf("colors %v: unexpected error: %v", colors, err)
			}
			if !res.IsError {
				t.Errorf("colors %v: expected error result, got %s", colors, resultText(t, res))
			}
		}
	})
}

func TestHandleGetEDHRECSetCards(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		resp := EDHRECResponse{
			Header: "Ravnica Allegiance",
			Container: EDHRECContainer{
				JSONDict: EDHRECData{
					CardLists: []EDHRECCardList{
						{
							Header:    "Commanders",
							CardViews: []EDHRECCardView{{Name: "Teysa Karlov", NumDecks: 21469}},
						},
					},
				},
			},
		}
		s := &MTGCommanderServer{edhrecBaseURL: jsonServer(t, http.StatusOK, resp)}
		res, err := s.handleGetEDHRECSetCards(context.Background(), toolRequest(map[string]any{"set": "rna"}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(resultText(t, res), "Teysa Karlov") {
			t.Errorf("expected card name in output:\n%s", resultText(t, res))
		}
	})

	t.Run("failure", func(t *testing.T) {
		s := &MTGCommanderServer{edhrecBaseURL: jsonServer(t, http.StatusForbidden, nil)}
		res, _ := s.handleGetEDHRECSetCards(context.Background(), toolRequest(map[string]any{"set": "notaset"}))
		if !res.IsError {
			t.Error("expected error result")
		}
		if !strings.Contains(resultText(t, res), "notaset") {
			t.Errorf("expected set code in error message:\n%s", resultText(t, res))
		}
	})
}

func TestHandleGetEDHRECRecommendationsThemeHint(t *testing.T) {
	t.Run("invalid theme suggests the real slug", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case strings.HasSuffix(r.URL.Path, "/commanders/atraxa-praetors-voice/infekt.json"):
				w.WriteHeader(http.StatusForbidden)
			case strings.HasSuffix(r.URL.Path, "/commanders/atraxa-praetors-voice.json"):
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(EDHRECResponse{
					TagCounts: []EDHRECTagCount{{Slug: "infect", Value: "Infect", Count: 4066}},
				})
			default:
				w.WriteHeader(http.StatusForbidden)
			}
		}))
		defer ts.Close()

		s := &MTGCommanderServer{edhrecBaseURL: ts.URL}
		res, err := s.handleGetEDHRECRecommendations(context.Background(), toolRequest(map[string]any{
			"commander": "Atraxa, Praetors' Voice",
			"theme":     "infekt",
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error result")
		}
		text := resultText(t, res)
		if !strings.Contains(text, "infect") || !strings.Contains(text, "4066") {
			t.Errorf("expected theme hint naming infect (4066 decks), got:\n%s", text)
		}
	})

	t.Run("valid-looking theme that still fails does not misreport an outage", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		}))
		defer ts.Close()

		s := &MTGCommanderServer{edhrecBaseURL: ts.URL}
		res, err := s.handleGetEDHRECRecommendations(context.Background(), toolRequest(map[string]any{
			"commander": "Atraxa, Praetors' Voice",
			"theme":     "infect",
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error result")
		}
		if !strings.Contains(resultText(t, res), "Failed to fetch EDHREC recommendations") {
			t.Errorf("expected generic failure message, got:\n%s", resultText(t, res))
		}
	})
}

func TestHandleGetEDHRECRecommendationsBadPriceTier(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("no HTTP request should be made for an invalid price tier")
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	s := &MTGCommanderServer{edhrecBaseURL: ts.URL}
	res, err := s.handleGetEDHRECRecommendations(context.Background(), toolRequest(map[string]any{
		"commander":  "Atraxa, Praetors' Voice",
		"price_tier": "middle",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Error("expected error result")
	}
}

func TestHandleGetEDHRECTopCards(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		resp := EDHRECResponse{
			Header: "Top Azorius Cards",
			Container: EDHRECContainer{
				JSONDict: EDHRECData{
					CardLists: []EDHRECCardList{
						{
							Header: "Top Cards",
							CardViews: []EDHRECCardView{
								{Name: "Swords to Plowshares", NumDecks: 603382, PotentialDecks: 2339647},
								{Name: "Counterspell", NumDecks: 461071, PotentialDecks: 2339647},
							},
						},
					},
				},
			},
		}
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasSuffix(r.URL.Path, "/top/azorius.json") {
				t.Errorf("Request URL = %v, want suffix /top/azorius.json", r.URL.Path)
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		t.Cleanup(ts.Close)
		s := &MTGCommanderServer{edhrecBaseURL: ts.URL}
		res, err := s.handleGetEDHRECTopCards(context.Background(), toolRequest(map[string]any{
			"color": "UW",
			"limit": float64(1),
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error result: %s", resultText(t, res))
		}
		text := resultText(t, res)
		if !strings.Contains(text, "Swords to Plowshares") {
			t.Errorf("expected first card in output:\n%s", text)
		}
		if strings.Contains(text, "Counterspell") {
			t.Errorf("limit 1 should hide the second card:\n%s", text)
		}
		if !strings.Contains(text, "*...and 1 more cards*") {
			t.Errorf("expected truncation footer:\n%s", text)
		}
	})

	t.Run("upstream 403", func(t *testing.T) {
		s := &MTGCommanderServer{edhrecBaseURL: jsonServer(t, http.StatusForbidden, nil)}
		res, err := s.handleGetEDHRECTopCards(context.Background(), toolRequest(map[string]any{"list": "salt"}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !res.IsError {
			t.Error("expected error result")
		}
		if !strings.Contains(resultText(t, res), "salt") {
			t.Errorf("expected page slug in error message:\n%s", resultText(t, res))
		}
	})

	t.Run("invalid arguments never reach EDHREC", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Errorf("unexpected EDHREC request %s", r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}))
		t.Cleanup(ts.Close)
		s := &MTGCommanderServer{edhrecBaseURL: ts.URL}

		for _, args := range []map[string]any{
			{},
			{"list": "salt", "color": "wu"},
			{"list": "bogus"},
			{"color": "xyz"},
			{"list": float64(42)},
			{"list": "salt", "limit": float64(-1)},
		} {
			res, err := s.handleGetEDHRECTopCards(context.Background(), toolRequest(args))
			if err != nil {
				t.Fatalf("args %v: unexpected error: %v", args, err)
			}
			if !res.IsError {
				t.Errorf("args %v: expected error result, got %s", args, resultText(t, res))
			}
		}
	})
}

func TestHandleGetEDHRECTopCommanders(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		resp := EDHRECResponse{
			Header: "Top Azorius Commanders",
			Container: EDHRECContainer{
				JSONDict: EDHRECData{
					CardLists: []EDHRECCardList{
						{
							Header: "Azorius Commanders",
							CardViews: []EDHRECCardView{
								{Name: "Shorikai, Genesis Engine", NumDecks: 21031},
								{Name: "Brago, King Eternal", NumDecks: 3000},
							},
						},
					},
				},
			},
		}
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasSuffix(r.URL.Path, "/commanders/azorius.json") {
				t.Errorf("Request URL = %v, want suffix /commanders/azorius.json", r.URL.Path)
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		t.Cleanup(ts.Close)
		s := &MTGCommanderServer{edhrecBaseURL: ts.URL}
		res, err := s.handleGetEDHRECTopCommanders(context.Background(), toolRequest(map[string]any{
			"color": "UW",
			"limit": float64(1),
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error result: %s", resultText(t, res))
		}
		text := resultText(t, res)
		if !strings.Contains(text, "Shorikai, Genesis Engine") {
			t.Errorf("expected first commander in output:\n%s", text)
		}
		if strings.Contains(text, "Brago, King Eternal") {
			t.Errorf("limit 1 should hide the second commander:\n%s", text)
		}
		if !strings.Contains(text, "*...and 1 more commanders*") {
			t.Errorf("expected truncation footer:\n%s", text)
		}
	})

	t.Run("upstream 403", func(t *testing.T) {
		s := &MTGCommanderServer{edhrecBaseURL: jsonServer(t, http.StatusForbidden, nil)}
		res, err := s.handleGetEDHRECTopCommanders(context.Background(), toolRequest(map[string]any{"period": "week"}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !res.IsError {
			t.Error("expected error result")
		}
		if !strings.Contains(resultText(t, res), "week") {
			t.Errorf("expected page slug in error message:\n%s", resultText(t, res))
		}
	})

	t.Run("invalid arguments never reach EDHREC", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Errorf("unexpected EDHREC request %s", r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}))
		t.Cleanup(ts.Close)
		s := &MTGCommanderServer{edhrecBaseURL: ts.URL}

		for _, args := range []map[string]any{
			{},
			{"period": "week", "color": "wu"},
			{"period": "day"},
			{"color": "multicolor"},
			{"period": float64(7)},
			{"period": "week", "limit": float64(-1)},
		} {
			res, err := s.handleGetEDHRECTopCommanders(context.Background(), toolRequest(args))
			if err != nil {
				t.Fatalf("args %v: unexpected error: %v", args, err)
			}
			if !res.IsError {
				t.Errorf("args %v: expected error result, got %s", args, resultText(t, res))
			}
		}
	})
}
