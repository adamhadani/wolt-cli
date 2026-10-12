package payloadutil

import "strings"

// OptionValuePriceIndex maps option value ids to the price the catalog
// payloads publish for them. Earlier payloads win, so callers pass the most
// specific payload first.
func OptionValuePriceIndex(payloads ...map[string]any) map[string]int {
	index := map[string]int{}
	for _, payload := range payloads {
		for _, spec := range ExtractOptionSpecs(payload) {
			for valueID, value := range spec.Values {
				valueID = strings.TrimSpace(valueID)
				if valueID == "" || !value.HasPrice {
					continue
				}
				if _, exists := index[valueID]; !exists {
					index[valueID] = value.Price
				}
			}
		}
	}
	return index
}

// UnpricedOptionValueIDs lists the selected option values of a basket line
// that carry no price. Wolt's basket responses list selected values by id and
// count only, so every line with options needs catalog prices before it can be
// sent back to the basket or checkout endpoints.
func UnpricedOptionValueIDs(line map[string]any) []string {
	ids := []string{}
	for _, rawOption := range Slice(line["options"]) {
		for _, rawValue := range Slice(Map(rawOption)["values"]) {
			value := Map(rawValue)
			if value == nil {
				continue
			}
			if _, hasPrice := optionPrice(value); hasPrice {
				continue
			}
			if valueID := strings.TrimSpace(String(value["id"])); valueID != "" {
				ids = append(ids, valueID)
			}
		}
	}
	return ids
}

// WithOptionValuePrices returns a copy of the basket line in which option
// values without a price take their price from prices. Values with no known
// price stay unpriced, so BuildBasketUpsertItem still rejects the line instead
// of sending a guessed price.
func WithOptionValuePrices(line map[string]any, prices map[string]int) map[string]any {
	if len(prices) == 0 || len(UnpricedOptionValueIDs(line)) == 0 {
		return line
	}
	options := Slice(line["options"])
	filledOptions := make([]any, 0, len(options))
	for _, rawOption := range options {
		option := Map(rawOption)
		if option == nil {
			filledOptions = append(filledOptions, rawOption)
			continue
		}
		values := Slice(option["values"])
		filledValues := make([]any, 0, len(values))
		for _, rawValue := range values {
			value := Map(rawValue)
			if value == nil {
				filledValues = append(filledValues, rawValue)
				continue
			}
			if _, hasPrice := optionPrice(value); !hasPrice {
				if price, known := prices[strings.TrimSpace(String(value["id"]))]; known {
					value = withKey(value, "price", price)
				}
			}
			filledValues = append(filledValues, value)
		}
		filledOptions = append(filledOptions, withKey(option, "values", filledValues))
	}
	return withKey(line, "options", filledOptions)
}

func withKey(source map[string]any, key string, value any) map[string]any {
	copied := make(map[string]any, len(source)+1)
	for existingKey, existingValue := range source {
		copied[existingKey] = existingValue
	}
	copied[key] = value
	return copied
}
