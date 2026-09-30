package heapsnap

import (
	"sort"
)

const (
	DefaultTop = 20
	MaxTop     = 200
)

type Summary struct {
	NodeCount        int               `json:"nodeCount"`
	EdgeCount        int               `json:"edgeCount"`
	TotalSelfSize    int64             `json:"totalSelfSize"`
	Constructors     int               `json:"constructors"`
	TopBySize        []Constructor     `json:"topBySize"`
	TopByCount       []Constructor     `json:"topByCount"`
	DuplicateStrings []DuplicateString `json:"duplicateStrings"`
}

func ClampTop(top int) int {
	if top <= 0 {
		return DefaultTop
	}
	if top > MaxTop {
		return MaxTop
	}
	return top
}

func (a *Aggregate) Summary(top int) Summary {
	top = ClampTop(top)
	bySize := append([]Constructor(nil), a.Constructors...)
	sort.SliceStable(bySize, func(i, j int) bool {
		x, y := bySize[i], bySize[j]
		if x.SelfSize != y.SelfSize {
			return x.SelfSize > y.SelfSize
		}
		if x.Count != y.Count {
			return x.Count > y.Count
		}
		return x.Name < y.Name
	})
	byCount := append([]Constructor(nil), a.Constructors...)
	sort.SliceStable(byCount, func(i, j int) bool {
		x, y := byCount[i], byCount[j]
		if x.Count != y.Count {
			return x.Count > y.Count
		}
		if x.SelfSize != y.SelfSize {
			return x.SelfSize > y.SelfSize
		}
		return x.Name < y.Name
	})
	return Summary{
		NodeCount:        a.NodeCount,
		EdgeCount:        a.EdgeCount,
		TotalSelfSize:    a.TotalSelfSize,
		Constructors:     len(a.Constructors),
		TopBySize:        head(bySize, top),
		TopByCount:       head(byCount, top),
		DuplicateStrings: head(a.DuplicateStrings, top),
	}
}

func head[T any](items []T, n int) []T {
	if len(items) > n {
		items = items[:n]
	}
	return append(make([]T, 0, len(items)), items...)
}
