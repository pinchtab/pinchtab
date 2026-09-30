package heapsnap

import "io"

func SummarizeFile(path string, top int) (Summary, error) {
	agg, err := ParseFile(path)
	if err != nil {
		return Summary{}, err
	}
	return agg.Summary(top), nil
}

func Parse(r io.Reader) (*Aggregate, error) {
	return ParseWith(r, ParseOptions{})
}

func ParseFile(path string) (*Aggregate, error) {
	return ParseFileWith(path, ParseOptions{})
}
