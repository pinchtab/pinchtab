package catalog

import (
	"slices"

	"github.com/pinchtab/pinchtab/internal/autosolver"
)

func IsAvailable(name string, cfg autosolver.Config) bool {
	return slices.Contains(Available(cfg), name)
}
