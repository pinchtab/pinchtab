package extract

import "testing"

func TestFieldQuery_SplitsCaseBoundariesIntoWords(t *testing.T) {
	cases := map[string]string{
		"itemPrice":    "item price",
		"OrderStatus":  "order status",
		"URLPath":      "url path",
		"giftWrap":     "gift wrap",
		"item2Price":   "item2 price",
		"item_price":   "item_price",
		"order-status": "order-status",
		"gift wrap":    "gift wrap",
		"price":        "price",
	}
	for name, want := range cases {
		if got := fieldQuery(Property{Name: name}); got != want {
			t.Errorf("fieldQuery(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestHeaderKey_FoldsEverySpellingToOneKey(t *testing.T) {
	for _, name := range []string{"item_price", "item-price", "item price", "itemPrice", "Item Price", "ItemPrice"} {
		if got := headerKey(name); got != "itemprice" {
			t.Errorf("headerKey(%q) = %q, want %q", name, got, "itemprice")
		}
	}
}
