package store

import "marquee/internal/i18n"

func init() {
	i18n.Register(map[string]string{
		// The operator's ads node is named once, in the hub language of that moment.
		"Моята реклама": "My ads",
	})
}
