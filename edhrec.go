package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

const (
	percentageMultiplier     = 100.0
	priceTierBudget          = "budget"
	priceTierExpensive       = "expensive"
	maxThemesListed          = 15
	defaultEDHRECLimit       = 10
	defaultSetCardsLimit     = 25
	wubrgOrder               = "wubrg"
	topSlugMulticolor        = "multicolor"
	rankingSectionTop        = "top"
	rankingSectionCommanders = "commanders"
)

// EDHRECResponse represents the top-level response structure.
type EDHRECResponse struct {
	Header    string           `json:"header"`
	Container EDHRECContainer  `json:"container"`
	TagCounts []EDHRECTagCount `json:"tag_counts"`
}

// EDHRECTagCount is one theme EDHREC tracks for a commander, as listed on the commander page.
type EDHRECTagCount struct {
	Slug  string `json:"slug"`
	Value string `json:"value"`
	Count int    `json:"count"`
}

// EDHRECContainer wraps the JSON dictionary.
type EDHRECContainer struct {
	JSONDict EDHRECData `json:"json_dict"`
}

// EDHRECData contains the main data structure.
type EDHRECData struct {
	Card      EDHRECCardInfo   `json:"card"`
	CardLists []EDHRECCardList `json:"cardlists"`
}

// EDHRECCardInfo represents commander information.
type EDHRECCardInfo struct {
	Name      string   `json:"name"`
	Sanitized string   `json:"sanitized"`
	ColorID   []string `json:"color_id"`
	NumDecks  int      `json:"num_decks"`
}

// EDHRECCardList represents a category of cards.
type EDHRECCardList struct {
	Header    string           `json:"header"`
	Tag       string           `json:"tag"`
	CardViews []EDHRECCardView `json:"cardviews"`
}

// EDHRECCardView represents a card with statistics.
type EDHRECCardView struct {
	Name           string  `json:"name"`
	Sanitized      string  `json:"sanitized"`
	NumDecks       int     `json:"num_decks"`
	PotentialDecks int     `json:"potential_decks"`
	Synergy        float64 `json:"synergy"`
	Salt           float64 `json:"salt"`
}

// EDHRECComboResponse represents combo data.
type EDHRECComboResponse struct {
	Container EDHRECComboContainer `json:"container"`
}

// EDHRECComboContainer wraps combo data.
type EDHRECComboContainer struct {
	JSONDict EDHRECComboData `json:"json_dict"`
}

// EDHRECComboData contains combo information.
type EDHRECComboData struct {
	CardLists []EDHRECComboList `json:"cardlists"`
}

// EDHRECComboList represents a combo entry in the new format.
type EDHRECComboList struct {
	Header    string           `json:"header"`
	CardViews []EDHRECCardView `json:"cardviews"`
	Combo     *EDHRECCombo     `json:"combo,omitempty"`
}

// EDHRECCombo represents a card combo.
type EDHRECCombo struct {
	ComboID string   `json:"comboId"`
	Cards   []string `json:"cards"`
	Results []string `json:"results"`
	// Colors is the combo page's colour identity in upper-case letters ("WU"); "" on colorless.
	Colors string `json:"colors"`
}

// EDHRECRecFilter narrows a commander recommendation request. An empty field means "no filter".
// EDHREC serves the theme segment before the price tier: commanders/<slug>/<theme>/<tier>.json.
// The reverse order answers 403.
type EDHRECRecFilter struct {
	Theme     string
	PriceTier string
}

// segments renders the filter as URL path segments in EDHREC's required order.
func (f EDHRECRecFilter) segments() string {
	var path strings.Builder
	if f.Theme != "" {
		path.WriteString("/")
		path.WriteString(url.PathEscape(f.Theme))
	}
	if f.PriceTier != "" {
		path.WriteString("/")
		path.WriteString(url.PathEscape(f.PriceTier))
	}

	return path.String()
}

// SanitizeCardName converts a card name to EDHREC URL format.
func SanitizeCardName(name string) string {
	// Lowercase
	sanitized := strings.ToLower(name)

	// Remove special characters and replace spaces with hyphens
	reg := regexp.MustCompile("[^a-z0-9-]+")
	sanitized = reg.ReplaceAllString(strings.ReplaceAll(sanitized, " ", "-"), "")

	// Remove duplicate hyphens
	reg2 := regexp.MustCompile("-+")
	sanitized = reg2.ReplaceAllString(sanitized, "-")

	// Trim hyphens from start and end
	sanitized = strings.Trim(sanitized, "-")

	return sanitized
}

// fetchEDHRECPage GETs an EDHREC JSON page and decodes the body into target. EDHREC is an
// undocumented reverse-engineered API: unknown slugs answer 403 with an XML body, so any
// non-200 status is reported as an error without attempting to decode.
func fetchEDHRECPage(ctx context.Context, reqURL, pageKind string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return err
	}

	req.Header.Set("User-Agent", "MTG-Commander-MCP-Server/1.0")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("EDHREC %s returned status %d", pageKind, resp.StatusCode)
	}

	if decodeErr := json.NewDecoder(resp.Body).Decode(target); decodeErr != nil {
		return fmt.Errorf("failed to decode EDHREC %s response: %w", pageKind, decodeErr)
	}

	return nil
}

// getCommanderPageWithURL fetches a commander page from baseURL, optionally narrowed by filter.
func getCommanderPageWithURL(
	ctx context.Context,
	commanderName string,
	filter EDHRECRecFilter,
	baseURL string,
) (*EDHRECResponse, error) {
	sanitized := SanitizeCardName(commanderName)
	reqURL := fmt.Sprintf("%s/commanders/%s%s.json", baseURL, url.PathEscape(sanitized), filter.segments())

	var page EDHRECResponse
	if err := fetchEDHRECPage(ctx, reqURL, "commander page", &page); err != nil {
		return nil, err
	}

	return &page, nil
}

// GetCommanderRecommendations fetches unfiltered EDHREC recommendations for a commander.
func GetCommanderRecommendations(ctx context.Context, commanderName string) (*EDHRECData, error) {
	page, err := getCommanderPageWithURL(ctx, commanderName, EDHRECRecFilter{}, defaultEDHRECBaseURL)
	if err != nil {
		return nil, err
	}

	return &page.Container.JSONDict, nil
}

// colorIdentity is one of the 32 colour identities EDHREC publishes pages for: its letters in WUBRG
// order ("c" for colorless), its EDHREC name (also the combo page slug, pages/combos/<name>.json),
// and the slug of its top-cards page (pages/top/<topSlug>.json), which differs for mono-colour.
type colorIdentity struct {
	letters string
	name    string
	topSlug string
}

// colorIdentities lists EDHREC's 32 colour identities (pages/combos.json), grouped by number of
// colours.
func colorIdentities() []colorIdentity {
	return []colorIdentity{
		{"c", "colorless", "colorless"},
		{"w", "mono-white", "white"}, {"u", "mono-blue", "blue"}, {"b", "mono-black", "black"},
		{"r", "mono-red", "red"}, {"g", "mono-green", "green"},
		{"wu", "azorius", "azorius"}, {"ub", "dimir", "dimir"}, {"br", "rakdos", "rakdos"},
		{"rg", "gruul", "gruul"}, {"wg", "selesnya", "selesnya"}, {"wb", "orzhov", "orzhov"},
		{"ur", "izzet", "izzet"}, {"bg", "golgari", "golgari"}, {"wr", "boros", "boros"},
		{"ug", "simic", "simic"},
		{"wub", "esper", "esper"}, {"ubr", "grixis", "grixis"}, {"brg", "jund", "jund"},
		{"wrg", "naya", "naya"}, {"wug", "bant", "bant"}, {"wbg", "abzan", "abzan"},
		{"wur", "jeskai", "jeskai"}, {"ubg", "sultai", "sultai"}, {"wbr", "mardu", "mardu"},
		{"urg", "temur", "temur"},
		{"wubr", "yore-tiller", "yore-tiller"}, {"ubrg", "glint-eye", "glint-eye"},
		{"wbrg", "dune-brood", "dune-brood"}, {"wurg", "ink-treader", "ink-treader"},
		{"wubg", "witch-maw", "witch-maw"},
		{"wubrg", "five-color", "five-color"},
	}
}

// canonicalWUBRG reorders a lowercase colour-letter string into WUBRG order. It reports false
// for an empty string, a repeated letter, or any character outside w/u/b/r/g; "c" (colorless)
// is returned unchanged.
func canonicalWUBRG(input string) (string, bool) {
	if input == "c" {
		return input, true
	}

	var canonical strings.Builder
	for _, letter := range wubrgOrder {
		occurrences := strings.Count(input, string(letter))
		if occurrences > 1 {
			return "", false
		}
		if occurrences == 1 {
			canonical.WriteRune(letter)
		}
	}

	if canonical.Len() == 0 || canonical.Len() != len(input) {
		return "", false
	}

	return canonical.String(), true
}

// resolveColorIdentity looks up a caller's colour identity — WUBRG letters in any order, "c" for
// colorless, or an EDHREC identity name, any case — in colorIdentities.
func resolveColorIdentity(input string) (colorIdentity, bool) {
	normalized := strings.ToLower(strings.TrimSpace(input))
	letters, isLetters := canonicalWUBRG(normalized)

	for _, identity := range colorIdentities() {
		if identity.name == normalized || (isLetters && identity.letters == letters) {
			return identity, true
		}
	}

	return colorIdentity{}, false
}

// colorIdentityNames returns the EDHREC names of colorIdentities, in table order.
func colorIdentityNames() []string {
	identities := colorIdentities()
	names := make([]string, len(identities))
	for i, identity := range identities {
		names[i] = identity.name
	}

	return names
}

// invalidColorIdentityError reports a colour-identity argument that resolved to nothing, naming
// the argument, the rejected value, and the accepted names.
func invalidColorIdentityError(argName, got string, validNames []string) error {
	return fmt.Errorf(
		"argument %q must be WUBRG letters in any order (e.g. \"wu\", \"ubrg\"), \"c\" for colorless, "+
			"or an EDHREC identity name; got %q. Valid names: %s",
		argName, got, strings.Join(validNames, ", "),
	)
}

// resolveComboSlug turns get_edhrec_combos' "colors" argument into its combo page slug.
func resolveComboSlug(colors string) (string, error) {
	identity, ok := resolveColorIdentity(colors)
	if !ok {
		return "", invalidColorIdentityError(paramColors, colors, colorIdentityNames())
	}

	return identity.name, nil
}

// resolveTopColorSlug turns get_edhrec_top_cards' "color" argument into its top-cards page slug.
// Besides the colour identities it accepts "multicolor", EDHREC's page of all multicolour cards.
func resolveTopColorSlug(color string) (string, error) {
	if strings.EqualFold(strings.TrimSpace(color), topSlugMulticolor) {
		return topSlugMulticolor, nil
	}

	identity, ok := resolveColorIdentity(color)
	if !ok {
		return "", invalidColorIdentityError(paramColor, color, append(colorIdentityNames(), topSlugMulticolor))
	}

	return identity.topSlug, nil
}

// resolveCommanderColorSlug turns get_edhrec_top_commanders' "color" argument into the slug of its
// commanders page, which is the identity's EDHREC name (pages/commanders/mono-white.json).
func resolveCommanderColorSlug(color string) (string, error) {
	identity, ok := resolveColorIdentity(color)
	if !ok {
		return "", invalidColorIdentityError(paramColor, color, colorIdentityNames())
	}

	return identity.name, nil
}

// GetCombosForColors fetches combos for a colour identity given as WUBRG letters, "c", or an
// EDHREC identity name.
func GetCombosForColors(ctx context.Context, colors string) (*EDHRECComboData, error) {
	slug, err := resolveComboSlug(colors)
	if err != nil {
		return nil, err
	}

	return getCombosForIdentityWithURL(ctx, slug, defaultEDHRECBaseURL)
}

// getCombosForIdentityWithURL fetches the combo page of an EDHREC identity slug (see
// colorIdentities) from baseURL.
func getCombosForIdentityWithURL(ctx context.Context, slug, baseURL string) (*EDHRECComboData, error) {
	reqURL := fmt.Sprintf("%s/combos/%s.json", baseURL, url.PathEscape(slug))

	var comboResp EDHRECComboResponse
	if err := fetchEDHRECPage(ctx, reqURL, "combos", &comboResp); err != nil {
		return nil, err
	}

	return &comboResp.Container.JSONDict, nil
}

// GetSetCards fetches the EDHREC page for a Magic set code.
func GetSetCards(ctx context.Context, setCode string) (*EDHRECResponse, error) {
	return getSetCardsWithURL(ctx, setCode, defaultEDHRECBaseURL)
}

// getSetCardsWithURL fetches a set page with a custom base URL. EDHREC keys set pages by the
// lowercase short set code ("rna", "c21"); any other casing answers 403.
func getSetCardsWithURL(ctx context.Context, setCode, baseURL string) (*EDHRECResponse, error) {
	code := strings.ToLower(strings.TrimSpace(setCode))
	reqURL := fmt.Sprintf("%s/sets/%s.json", baseURL, url.PathEscape(code))

	var page EDHRECResponse
	if err := fetchEDHRECPage(ctx, reqURL, "set page", &page); err != nil {
		return nil, err
	}

	return &page, nil
}

// topCardLists returns the format-wide rankings get_edhrec_top_cards accepts as "list"; each is the
// slug of pages/top/<slug>.json, as linked from edhrec.com/top.
func topCardLists() []string {
	return []string{
		"salt", "game-changers", "week", "month", "year",
		"artifacts", "auras", "battles", "color-fixing-lands", "creatures", "enchantments", "equipment",
		"instants", "lands", "mana-artifacts", "planeswalkers", "sorceries", "utility-artifacts",
		"utility-lands",
	}
}

// commanderPeriods returns the time windows get_edhrec_top_commanders accepts as "period"; each is
// the slug of pages/commanders/<slug>.json. EDHREC's "year" page covers the past two years.
func commanderPeriods() []string {
	return []string{"week", "month", "year"}
}

// getRankingPageWithURL fetches the first page of an EDHREC ranking, pages/<section>/<slug>.json,
// from baseURL. section is rankingSectionTop or rankingSectionCommanders; slug must already be
// resolved from fixed values (see exclusiveSelectorSlug), never taken from raw input.
func getRankingPageWithURL(ctx context.Context, section, slug, baseURL string) (*EDHRECResponse, error) {
	reqURL := fmt.Sprintf("%s/%s/%s.json", baseURL, section, url.PathEscape(slug))

	var page EDHRECResponse
	if err := fetchEDHRECPage(ctx, reqURL, section+" ranking", &page); err != nil {
		return nil, err
	}

	return &page, nil
}

// topThemes returns the highest-count themes, capped at maxThemesListed, plus the number
// omitted. tag_counts arrives sorted by count descending.
func topThemes(tags []EDHRECTagCount) ([]EDHRECTagCount, int) {
	count := len(tags)
	if count > maxThemesListed {
		count = maxThemesListed
	}

	return tags[:count], len(tags) - count
}

// commanderThemeHint returns a caller-facing message naming the themes EDHREC lists for a
// commander. It returns "" when the unfiltered page is unreachable, lists no themes, or already
// contains the requested slug — in those cases the failure is not the theme and the caller must
// surface the underlying EDHREC error instead.
func commanderThemeHint(ctx context.Context, commanderName, theme, baseURL string) string {
	page, err := getCommanderPageWithURL(ctx, commanderName, EDHRECRecFilter{}, baseURL)
	if err != nil || len(page.TagCounts) == 0 {
		return ""
	}

	suggestion := ""

	for _, tag := range page.TagCounts {
		if strings.EqualFold(tag.Slug, theme) {
			return ""
		}

		if suggestion == "" && strings.EqualFold(tag.Value, theme) {
			suggestion = fmt.Sprintf("Did you mean %q? ", tag.Slug)
		}
	}

	shown, omitted := topThemes(page.TagCounts)

	entries := make([]string, len(shown))
	for i, tag := range shown {
		entries[i] = fmt.Sprintf("%s (%d decks)", tag.Slug, tag.Count)
	}

	joined := strings.Join(entries, ", ")
	if omitted > 0 {
		joined += fmt.Sprintf(", and %d more", omitted)
	}

	return suggestion + fmt.Sprintf(
		"theme %q is not available for %s. Available themes include: %s. %d themes total.",
		theme, commanderName, joined, len(page.TagCounts),
	)
}

// writeCardDeckStats writes one card's deck line. denominator is the deck universe the card
// competes in: the commander's own deck count on recommendation pages, or the card's
// potential_decks on set and top-cards pages. A non-positive denominator prints the raw count only.
func writeCardDeckStats(output *strings.Builder, numDecks, denominator int) {
	if denominator <= 0 {
		_, _ = fmt.Fprintf(output, "   - Decks: %d\n", numDecks)

		return
	}

	percentage := float64(numDecks) / float64(denominator) * percentageMultiplier
	_, _ = fmt.Fprintf(output, "   - Decks: %d of %d (%.1f%%)\n", numDecks, denominator, percentage)
}

// writeRecommendationCard writes one card's body for recommendation, set and top-cards pages,
// including its deck stats and the optional synergy/salt lines.
func writeRecommendationCard(output *strings.Builder, index int, card EDHRECCardView, denominator int) {
	_, _ = fmt.Fprintf(output, "%d. **%s**\n", index+1, card.Name)
	writeCardDeckStats(output, card.NumDecks, denominator)

	if card.Synergy != 0 {
		_, _ = fmt.Fprintf(output, "   - Synergy: %.2f\n", card.Synergy)
	}

	if card.Salt > 0 {
		_, _ = fmt.Fprintf(output, "   - Salt Score: %.2f/4.0\n", card.Salt)
	}

	output.WriteString("\n")
}

// FormatCommanderRecsForDisplay formats EDHREC recommendations for text display. filter is used
// only to decide whether to advertise the commander's other themes.
func FormatCommanderRecsForDisplay(page *EDHRECResponse, filter EDHRECRecFilter, limit int) string {
	data := &page.Container.JSONDict

	var output strings.Builder

	title := page.Header
	if title == "" {
		title = data.Card.Name
	}

	_, _ = fmt.Fprintf(&output, "# EDHREC Recommendations for %s\n\n", title)
	_, _ = fmt.Fprintf(&output, "**Total Decks:** %d\n", data.Card.NumDecks)

	if len(data.Card.ColorID) > 0 {
		_, _ = fmt.Fprintf(&output, "**Color Identity:** %s\n\n", strings.Join(data.Card.ColorID, ", "))
	}

	if filter.Theme == "" && filter.PriceTier == "" && len(page.TagCounts) > 0 {
		shown, omitted := topThemes(page.TagCounts)

		output.WriteString("\n## Available Themes\n\n")
		for _, tag := range shown {
			_, _ = fmt.Fprintf(&output, "- %s (%d decks)\n", tag.Slug, tag.Count)
		}

		if omitted > 0 {
			_, _ = fmt.Fprintf(&output, "\n*...and %d more themes*\n", omitted)
		}
	}

	// Show each card list category
	for _, cardList := range data.CardLists {
		if len(cardList.CardViews) == 0 {
			continue
		}

		_, _ = fmt.Fprintf(&output, "\n## %s\n\n", cardList.Header)

		// Limit number of cards shown per category
		count := len(cardList.CardViews)
		if limit > 0 && count > limit {
			count = limit
		}

		for i := range count {
			writeRecommendationCard(&output, i, cardList.CardViews[i], data.Card.NumDecks)
		}

		if len(cardList.CardViews) > count {
			_, _ = fmt.Fprintf(&output, "*...and %d more cards*\n", len(cardList.CardViews)-count)
		}
	}

	return output.String()
}

// FormatCombosForDisplay formats combo data for text display.
func FormatCombosForDisplay(data *EDHRECComboData, limit int) string {
	var output strings.Builder

	output.WriteString("# Popular Combos\n\n")
	_, _ = fmt.Fprintf(&output, "**Total Combos:** %d\n\n", len(data.CardLists))

	count := len(data.CardLists)
	if limit > 0 && count > limit {
		count = limit
	}

	for i := range count {
		comboList := data.CardLists[i]

		_, _ = fmt.Fprintf(&output, "%d. **%s**\n", i+1, comboList.Header)

		if len(comboList.CardViews) > 0 {
			cardNames := make([]string, len(comboList.CardViews))
			for j, card := range comboList.CardViews {
				cardNames[j] = card.Name
			}
			_, _ = fmt.Fprintf(&output, "   **Cards:** %s\n", strings.Join(cardNames, " + "))
		}

		if comboList.Combo != nil && len(comboList.Combo.Results) > 0 {
			_, _ = fmt.Fprintf(&output, "   **Results:** %s\n", strings.Join(comboList.Combo.Results, ", "))
		}

		output.WriteString("\n")
	}

	if len(data.CardLists) > count {
		_, _ = fmt.Fprintf(&output, "*...and %d more combos*\n", len(data.CardLists)-count)
	}

	return output.String()
}

// FormatSetCardsForDisplay formats an EDHREC set page for text display. setCode is echoed so the
// caller can see which code was resolved. limit caps each cardlist; limit <= 0 shows everything.
func FormatSetCardsForDisplay(page *EDHRECResponse, setCode string, limit int) string {
	var output strings.Builder

	title := page.Header
	if title == "" {
		_, _ = fmt.Fprintf(&output, "# EDHREC Set Overview: %s\n\n", setCode)
	} else {
		_, _ = fmt.Fprintf(&output, "# EDHREC Set Overview: %s (%s)\n\n", title, setCode)
	}

	writeCardLists(&output, page.Container.JSONDict.CardLists, limit, "cards")

	return output.String()
}

// writeCardLists writes every non-empty cardlist, each capped at limit entries (limit <= 0 shows
// all). noun names the entries in the counts ("cards", "commanders"). A list with an empty header
// gets no heading of its own. Each entry's deck share is measured against its own potential_decks.
func writeCardLists(output *strings.Builder, lists []EDHRECCardList, limit int, noun string) {
	for _, cardList := range lists {
		if len(cardList.CardViews) == 0 {
			continue
		}

		if cardList.Header != "" {
			_, _ = fmt.Fprintf(output, "## %s (%d %s)\n\n", cardList.Header, len(cardList.CardViews), noun)
		}

		count := len(cardList.CardViews)
		if limit > 0 && count > limit {
			count = limit
		}

		for i := range count {
			card := cardList.CardViews[i]
			writeRecommendationCard(output, i, card, card.PotentialDecks)
		}

		if len(cardList.CardViews) > count {
			_, _ = fmt.Fprintf(output, "*...and %d more %s*\n\n", len(cardList.CardViews)-count, noun)
		}
	}
}

// formatRankingPage formats an EDHREC ranking page for text display. slug is echoed so the caller
// sees which page was resolved; fallbackTitle is used when the page has no header; noun names the
// entries in counts; limit caps each cardlist (limit <= 0 shows everything).
func formatRankingPage(page *EDHRECResponse, slug string, limit int, fallbackTitle, noun string) string {
	var output strings.Builder

	title := page.Header
	if title == "" {
		title = fallbackTitle
	}
	_, _ = fmt.Fprintf(&output, "# EDHREC %s (%s)\n\n", title, slug)

	writeCardLists(&output, page.Container.JSONDict.CardLists, limit, noun)

	return output.String()
}

// FormatTopCardsForDisplay formats an EDHREC top-cards page (pages/top) for text display.
func FormatTopCardsForDisplay(page *EDHRECResponse, slug string, limit int) string {
	return formatRankingPage(page, slug, limit, "Top Cards", "cards")
}

// FormatTopCommandersForDisplay formats an EDHREC top-commanders page (pages/commanders/<period or
// identity>) for text display. Commander pages carry no potential_decks, so deck counts are raw.
func FormatTopCommandersForDisplay(page *EDHRECResponse, slug string, limit int) string {
	return formatRankingPage(page, slug, limit, "Top Commanders", "commanders")
}
