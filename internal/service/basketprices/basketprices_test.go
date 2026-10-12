package basketprices

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
	"github.com/mekedron/wolt-cli/internal/service/payloadutil"
)

// catalogStub serves the two catalog endpoints Fill reads; any other call
// panics through the nil embedded interface.
type catalogStub struct {
	woltgateway.API
	assortment     map[string]any
	assortmentErr  error
	itemPages      map[string]map[string]any
	assortmentSlug []string
	itemPageCalls  []string
}

func (s *catalogStub) AssortmentByVenueSlug(_ context.Context, slug string) (map[string]any, error) {
	s.assortmentSlug = append(s.assortmentSlug, slug)
	return s.assortment, s.assortmentErr
}

func (s *catalogStub) VenueItemPage(_ context.Context, venueID, itemID string) (map[string]any, error) {
	s.itemPageCalls = append(s.itemPageCalls, venueID+"/"+itemID)
	if page, ok := s.itemPages[itemID]; ok {
		return page, nil
	}
	return nil, errors.New("not found")
}

func pricedGroup(groupID string, valueID string, price int) map[string]any {
	return map[string]any{"options": []any{map[string]any{
		"id":     groupID,
		"values": []any{map[string]any{"id": valueID, "price": price}},
	}}}
}

func basketWith(lines ...any) map[string]any {
	return map[string]any{
		"id":    "basket-1",
		"venue": map[string]any{"id": "venue-id", "slug": "venue-slug"},
		"items": lines,
	}
}

func line(itemID string, valueID string, price any) map[string]any {
	value := map[string]any{"id": valueID, "count": 1}
	if price != nil {
		value["price"] = price
	}
	return map[string]any{
		"id":      itemID,
		"count":   1,
		"price":   1000,
		"options": []any{map[string]any{"id": "grp-" + itemID, "values": []any{value}}},
	}
}

func valuePrice(t *testing.T, basket map[string]any, index int) (int, bool) {
	t.Helper()
	item := payloadutil.Map(payloadutil.Slice(basket["items"])[index])
	value := payloadutil.Map(payloadutil.Slice(payloadutil.Map(payloadutil.Slice(item["options"])[0])["values"])[0])
	raw, ok := value["price"]
	return payloadutil.Int(raw), ok
}

func TestFillLeavesCompleteBasketsUntouchedWithoutCatalogRequests(t *testing.T) {
	stub := &catalogStub{}
	basket := basketWith(line("item-1", "val-1", 0), map[string]any{"id": "item-2", "count": 1, "price": 500})
	got, warnings := Fill(context.Background(), stub, basket)
	if !reflect.DeepEqual(got, basket) || len(warnings) != 0 {
		t.Fatalf("Fill() changed a complete basket: %#v %v", got, warnings)
	}
	if len(stub.assortmentSlug)+len(stub.itemPageCalls) != 0 {
		t.Fatalf("Fill() made catalog requests for a complete basket")
	}
}

func TestFillUsesAssortmentThenItemPageForTheRest(t *testing.T) {
	stub := &catalogStub{
		assortment: pricedGroup("lib-1", "val-1", 400),
		itemPages:  map[string]map[string]any{"item-2": pricedGroup("grp-item-2", "val-2", 120)},
	}
	basket := basketWith(line("item-1", "val-1", nil), line("item-2", "val-2", nil))
	got, warnings := Fill(context.Background(), stub, basket)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings %v", warnings)
	}
	if price, ok := valuePrice(t, got, 0); !ok || price != 400 {
		t.Fatalf("item-1 value price = %d (set=%v), want 400 from the assortment", price, ok)
	}
	if price, ok := valuePrice(t, got, 1); !ok || price != 120 {
		t.Fatalf("item-2 value price = %d (set=%v), want 120 from the item page", price, ok)
	}
	if !reflect.DeepEqual(stub.assortmentSlug, []string{"venue-slug"}) ||
		!reflect.DeepEqual(stub.itemPageCalls, []string{"venue-id/item-2"}) {
		t.Fatalf("catalog requests: assortment=%v item pages=%v", stub.assortmentSlug, stub.itemPageCalls)
	}
	if _, ok := valuePrice(t, basket, 0); ok {
		t.Fatalf("Fill() mutated the input basket")
	}
}

func TestFillKeepsUnknownValuesUnpricedAndWarns(t *testing.T) {
	stub := &catalogStub{assortmentErr: errors.New("boom")}
	got, warnings := Fill(context.Background(), stub, basketWith(line("item-1", "val-1", nil)))
	if _, ok := valuePrice(t, got, 0); ok {
		t.Fatalf("value without any catalog price must stay unpriced")
	}
	joined := strings.Join(warnings, "\n")
	if !strings.Contains(joined, "venue assortment") || !strings.Contains(joined, "item item-1") {
		t.Fatalf("warnings = %v, want assortment and item page failures", warnings)
	}
	// The mutation helpers still refuse the unpriced value.
	if _, _, err := payloadutil.RemoveBasketItems(got, "item-1", 1); err == nil || !strings.Contains(err.Error(), "has no current price") {
		t.Fatalf("RemoveBasketItems() error = %v, want the missing price refusal", err)
	}
}
