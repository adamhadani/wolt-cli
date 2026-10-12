package payloadutil

import (
	"reflect"
	"testing"
)

func TestOptionValuePriceIndexPrefersEarlierPayloads(t *testing.T) {
	specific := map[string]any{"options": []any{map[string]any{
		"id":     "grp",
		"values": []any{map[string]any{"id": "val-a", "price": 150}, map[string]any{"id": "val-unpriced"}},
	}}}
	venueWide := map[string]any{"options": []any{map[string]any{
		"id": "lib",
		"values": []any{
			map[string]any{"id": "val-a", "price": 100},
			map[string]any{"id": "val-b", "price": map[string]any{"amount": 75}},
		},
	}}}
	got := OptionValuePriceIndex(specific, nil, venueWide)
	want := map[string]int{"val-a": 150, "val-b": 75}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("OptionValuePriceIndex() = %v, want %v", got, want)
	}
}

func TestUnpricedOptionValueIDs(t *testing.T) {
	line := map[string]any{"options": []any{
		map[string]any{"id": "grp-1", "values": []any{
			map[string]any{"id": "val-free", "price": 0},
			map[string]any{"id": "val-missing", "count": 1},
			map[string]any{"id": "val-null", "price": nil},
			map[string]any{"id": "val-empty-amount", "price": map[string]any{"amount": nil}},
		}},
	}}
	got := UnpricedOptionValueIDs(line)
	want := []string{"val-missing", "val-null", "val-empty-amount"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("UnpricedOptionValueIDs() = %v, want %v", got, want)
	}
	if ids := UnpricedOptionValueIDs(map[string]any{"options": []any{}}); len(ids) != 0 {
		t.Fatalf("line without options reported %v", ids)
	}
}

func TestWithOptionValuePricesFillsOnlyMissingPricesWithoutMutatingInput(t *testing.T) {
	line := map[string]any{
		"id":    "item-1",
		"price": 1890,
		"options": []any{map[string]any{"id": "grp-1", "values": []any{
			map[string]any{"id": "val-known", "count": 1},
			map[string]any{"id": "val-priced", "count": 1, "price": 50},
			map[string]any{"id": "val-unknown", "count": 1},
		}}},
	}
	filled := WithOptionValuePrices(line, map[string]int{"val-known": 400, "val-priced": 999})

	values := Slice(Map(Slice(filled["options"])[0])["values"])
	if got := Int(Map(values[0])["price"]); got != 400 {
		t.Fatalf("missing price = %d, want catalog 400", got)
	}
	if got := Int(Map(values[1])["price"]); got != 50 {
		t.Fatalf("existing price = %d, want it kept at 50", got)
	}
	if _, has := Map(values[2])["price"]; has {
		t.Fatalf("value without a catalog price must stay unpriced, got %#v", values[2])
	}
	original := Slice(Map(Slice(line["options"])[0])["values"])
	if _, has := Map(original[0])["price"]; has {
		t.Fatalf("input line was mutated: %#v", original[0])
	}
	if _, err := BuildBasketUpsertItem(filled, 1); err == nil {
		t.Fatalf("BuildBasketUpsertItem must still reject the value no catalog prices")
	}
}

func TestWithOptionValuePricesReturnsCompleteLinesAsIs(t *testing.T) {
	line := map[string]any{"id": "item-1", "options": []any{map[string]any{"id": "grp", "values": []any{
		map[string]any{"id": "val", "price": 0},
	}}}}
	if got := WithOptionValuePrices(line, map[string]int{"val": 100}); !reflect.DeepEqual(got, line) {
		t.Fatalf("complete line changed: %#v", got)
	}
}
