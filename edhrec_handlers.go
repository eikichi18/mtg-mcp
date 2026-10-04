package main

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

// optionalStringArg returns the trimmed value of an optional string argument. A missing argument
// yields "", a present non-string argument is an error rather than a silent default.
func optionalStringArg(args map[string]any, key string) (string, error) {
	raw, present := args[key]
	if !present || raw == nil {
		return "", nil
	}

	str, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("argument %q must be a string", key)
	}

	return strings.TrimSpace(str), nil
}

// recFilterFromArgs builds a recommendation filter from raw MCP arguments, rejecting wrong-typed
// values and unknown price tiers instead of silently ignoring them.
func recFilterFromArgs(args map[string]any) (EDHRECRecFilter, error) {
	theme, err := optionalStringArg(args, paramTheme)
	if err != nil {
		return EDHRECRecFilter{}, err
	}
	theme = strings.ToLower(theme)

	tier, err := optionalStringArg(args, paramPriceTier)
	if err != nil {
		return EDHRECRecFilter{}, err
	}
	tier = strings.ToLower(tier)

	if tier != "" && tier != priceTierBudget && tier != priceTierExpensive {
		return EDHRECRecFilter{}, fmt.Errorf(
			"argument %q must be %q or %q, got %q", paramPriceTier, priceTierBudget, priceTierExpensive, tier,
		)
	}

	return EDHRECRecFilter{Theme: theme, PriceTier: tier}, nil
}

// topCardsSlugFromArgs resolves the pages/top slug from get_edhrec_top_cards' mutually exclusive
// "list" and "color" arguments, rejecting wrong types, unknown values, and both-or-neither.
func topCardsSlugFromArgs(args map[string]any) (string, error) {
	list, err := optionalStringArg(args, paramList)
	if err != nil {
		return "", err
	}

	color, err := optionalStringArg(args, paramColor)
	if err != nil {
		return "", err
	}

	switch {
	case list != "" && color != "":
		return "", fmt.Errorf("pass either %q or %q, not both", paramList, paramColor)
	case list != "":
		slug := strings.ToLower(list)
		if !slices.Contains(topCardLists(), slug) {
			return "", fmt.Errorf(
				"argument %q must be one of %s; got %q", paramList, strings.Join(topCardLists(), ", "), list,
			)
		}

		return slug, nil
	case color != "":
		return resolveTopColorSlug(color)
	default:
		return "", fmt.Errorf("one of %q or %q is required", paramList, paramColor)
	}
}

func (s *MTGCommanderServer) handleGetEDHRECRecommendations(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	commander, err := request.RequireString(paramCommander)
	if err != nil {
		GetLogger().Error().Err(err).Str("tool", "get_edhrec_recommendations").Msg("Missing commander parameter")
		return mcp.NewToolResultError(err.Error()), nil
	}

	filter, err := recFilterFromArgs(request.GetArguments())
	if err != nil {
		GetLogger().Error().Err(err).Str("tool", "get_edhrec_recommendations").Msg("Invalid filter arguments")
		return mcp.NewToolResultError(err.Error()), nil
	}

	limit := request.GetInt("limit", defaultEDHRECLimit)
	if limit < 0 {
		return mcp.NewToolResultError("argument \"limit\" must be zero or greater"), nil
	}

	GetLogger().Info().
		Str("tool", "get_edhrec_recommendations").
		Str(paramCommander, commander).
		Str(paramTheme, filter.Theme).
		Str(paramPriceTier, filter.PriceTier).
		Int("limit", limit).
		Msg("Fetching EDHREC recommendations")

	page, err := getCommanderPageWithURL(ctx, commander, filter, s.edhrecBaseURL)
	if err != nil {
		GetLogger().Error().
			Err(err).
			Str("tool", "get_edhrec_recommendations").
			Str(paramCommander, commander).
			Str(paramTheme, filter.Theme).
			Str(paramPriceTier, filter.PriceTier).
			Msg("Failed to fetch EDHREC recommendations")

		if filter.Theme != "" {
			if hint := commanderThemeHint(ctx, commander, filter.Theme, s.edhrecBaseURL); hint != "" {
				return mcp.NewToolResultError(hint), nil
			}
		}

		return mcp.NewToolResultError(fmt.Sprintf("Failed to fetch EDHREC recommendations: %v", err)), nil
	}

	GetLogger().Info().
		Str("tool", "get_edhrec_recommendations").
		Str(paramCommander, commander).
		Str(paramTheme, filter.Theme).
		Str(paramPriceTier, filter.PriceTier).
		Int("num_decks", page.Container.JSONDict.Card.NumDecks).
		Int("card_lists", len(page.Container.JSONDict.CardLists)).
		Msg("Successfully fetched EDHREC recommendations")

	output := FormatCommanderRecsForDisplay(page, filter, limit)
	return mcp.NewToolResultText(output), nil
}

func (s *MTGCommanderServer) handleGetEDHRECCombos(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	colors, err := request.RequireString(paramColors)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	slug, err := resolveComboSlug(colors)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	limit := request.GetInt("limit", defaultEDHRECLimit)

	data, err := getCombosForIdentityWithURL(ctx, slug, s.edhrecBaseURL)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to fetch EDHREC combos: %v", err)), nil
	}

	output := FormatCombosForDisplay(data, limit)
	return mcp.NewToolResultText(output), nil
}

func (s *MTGCommanderServer) handleGetEDHRECSetCards(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	setCode, err := request.RequireString(paramSet)
	if err != nil {
		GetLogger().Error().Err(err).Str("tool", "get_edhrec_set_cards").Msg("Missing set parameter")
		return mcp.NewToolResultError(err.Error()), nil
	}

	code := strings.ToLower(strings.TrimSpace(setCode))
	if code == "" {
		return mcp.NewToolResultError(fmt.Sprintf("argument %q must not be empty", paramSet)), nil
	}

	limit := request.GetInt("limit", defaultSetCardsLimit)
	if limit < 0 {
		return mcp.NewToolResultError("argument \"limit\" must be zero or greater"), nil
	}

	GetLogger().Info().
		Str("tool", "get_edhrec_set_cards").
		Str(paramSet, code).
		Int("limit", limit).
		Msg("Fetching EDHREC set cards")

	page, err := getSetCardsWithURL(ctx, code, s.edhrecBaseURL)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf(
			"Failed to fetch EDHREC data for set %q: %v. "+
				"EDHREC keys set pages by lowercase short code (e.g. 'rna', 'c21'); unknown codes answer HTTP 403.",
			code, err,
		)), nil
	}

	GetLogger().Info().
		Str("tool", "get_edhrec_set_cards").
		Str(paramSet, code).
		Int("card_lists", len(page.Container.JSONDict.CardLists)).
		Msg("Successfully fetched EDHREC set cards")

	return mcp.NewToolResultText(FormatSetCardsForDisplay(page, code, limit)), nil
}

func (s *MTGCommanderServer) handleGetEDHRECTopCards(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	slug, err := topCardsSlugFromArgs(request.GetArguments())
	if err != nil {
		GetLogger().Error().Err(err).Str("tool", "get_edhrec_top_cards").Msg("Invalid arguments")
		return mcp.NewToolResultError(err.Error()), nil
	}

	limit := request.GetInt("limit", defaultEDHRECLimit)
	if limit < 0 {
		return mcp.NewToolResultError("argument \"limit\" must be zero or greater"), nil
	}

	GetLogger().Info().
		Str("tool", "get_edhrec_top_cards").
		Str("page", slug).
		Int("limit", limit).
		Msg("Fetching EDHREC top cards")

	page, err := getTopCardsPageWithURL(ctx, slug, s.edhrecBaseURL)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to fetch EDHREC top cards page %q: %v", slug, err)), nil
	}

	return mcp.NewToolResultText(FormatTopCardsForDisplay(page, slug, limit)), nil
}
