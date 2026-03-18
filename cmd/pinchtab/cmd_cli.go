package main

import (
	"fmt"
	"strings"

	"github.com/pinchtab/pinchtab/internal/urls"
	"github.com/spf13/cobra"
)

func normalizeRequiredURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		cobra.CheckErr(fmt.Errorf("empty URL"))
	}
	return urls.Normalize(trimmed)
}
