// Package basketprices restores the option value prices that Wolt leaves out
// of basket responses.
//
// A basket response lists each selected option value by id and count only,
// while the basket replacement endpoint needs the price of every value. Every
// mutation sends the complete items array back, so one line with options in a
// venue basket would otherwise block adding or removing any item there.
package basketprices

import (
	"context"
	"fmt"
	"strings"

	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
	"github.com/mekedron/wolt-cli/internal/service/payloadutil"
)

// Fill returns the basket with catalog prices on the option values Wolt
// returned unpriced. It reads the venue assortment first and asks the item
// endpoint only for lines the assortment does not price. A basket that needs
// no prices is returned as is. Values priced nowhere stay unpriced, so the
// mutation still fails closed rather than sending a guessed price.
func Fill(ctx context.Context, wolt woltgateway.API, basket map[string]any) (map[string]any, []string) {
	lines := payloadutil.Slice(basket["items"])
	missing := map[string]struct{}{}
	for _, rawLine := range lines {
		for _, valueID := range payloadutil.UnpricedOptionValueIDs(payloadutil.Map(rawLine)) {
			missing[valueID] = struct{}{}
		}
	}
	if len(missing) == 0 || wolt == nil {
		return basket, nil
	}

	warnings := []string{}
	venue := payloadutil.Map(basket["venue"])
	prices := map[string]int{}
	merge := func(payload map[string]any) {
		for valueID, price := range payloadutil.OptionValuePriceIndex(payload) {
			if _, exists := prices[valueID]; !exists {
				prices[valueID] = price
			}
		}
	}
	unresolved := func(line map[string]any) bool {
		for _, valueID := range payloadutil.UnpricedOptionValueIDs(line) {
			if _, known := prices[valueID]; !known {
				return true
			}
		}
		return false
	}

	slug := strings.TrimSpace(payloadutil.String(payloadutil.CoalesceAny(
		venue["slug"],
		venue["venue_slug"],
		venue["public_slug"],
		venue["url_slug"],
		basket["venue_slug"],
	)))
	if slug != "" {
		if payload, err := wolt.AssortmentByVenueSlug(ctx, slug); err == nil {
			merge(payload)
		} else {
			warnings = append(warnings, fmt.Sprintf("unable to load venue assortment for option prices (slug=%s)", slug))
		}
	}
	venueID := strings.TrimSpace(payloadutil.String(payloadutil.CoalesceAny(venue["id"], basket["venue_id"])))
	fetched := map[string]struct{}{}
	for _, rawLine := range lines {
		line := payloadutil.Map(rawLine)
		itemID := strings.TrimSpace(payloadutil.String(line["id"]))
		if venueID == "" || itemID == "" || !unresolved(line) {
			continue
		}
		if _, done := fetched[itemID]; done {
			continue
		}
		fetched[itemID] = struct{}{}
		if payload, err := wolt.VenueItemPage(ctx, venueID, itemID); err == nil {
			merge(payload)
		} else {
			warnings = append(warnings, fmt.Sprintf("unable to load item %s for option prices", itemID))
		}
	}

	filled := make([]any, 0, len(lines))
	for _, rawLine := range lines {
		if line := payloadutil.Map(rawLine); line != nil {
			filled = append(filled, payloadutil.WithOptionValuePrices(line, prices))
			continue
		}
		filled = append(filled, rawLine)
	}
	copied := make(map[string]any, len(basket))
	for key, value := range basket {
		copied[key] = value
	}
	copied["items"] = filled
	return copied, warnings
}
