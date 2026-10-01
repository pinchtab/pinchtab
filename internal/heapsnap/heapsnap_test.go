package heapsnap

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

const fixturePath = "testdata/small.heapsnapshot"

func readFixture(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestSummaryOverTheFixtureIsDeterministic(t *testing.T) {
	want := Summary{
		NodeCount:     20,
		EdgeCount:     6,
		TotalSelfSize: 3536,
		Constructors:  10,
		TopBySize: []Constructor{
			{Name: "Array", Count: 3, SelfSize: 1664},
			{Name: "(array)", Count: 1, SelfSize: 800},
			{Name: "(compiled code)", Count: 1, SelfSize: 300},
			{Name: "(string)", Count: 8, SelfSize: 228},
		},
		TopByCount: []Constructor{
			{Name: "(string)", Count: 8, SelfSize: 228},
			{Name: "Array", Count: 3, SelfSize: 1664},
			{Name: "Object", Count: 2, SelfSize: 112},
			{Name: "(array)", Count: 1, SelfSize: 800},
		},
		DuplicateStrings: []DuplicateString{
			{Value: "secret-token-abc", Length: 16, Count: 3, SelfSize: 120},
			{Value: `a string with "escapes" é`, Length: 25, Count: 2, SelfSize: 48},
			{Value: "hello", Length: 5, Count: 2, SelfSize: 40},
		},
	}
	for i := range 5 {
		got, err := SummarizeFile(fixturePath, 4)
		if err != nil {
			t.Fatalf("summarize: %v", err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("run %d summary =\n%+v\nwant\n%+v", i, got, want)
		}
	}
}

func TestAggregateCarriesEveryConstructorSortedByName(t *testing.T) {
	agg, err := ParseFile(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, c := range agg.Constructors {
		names = append(names, c.Name)
	}
	want := []string{"(array)", "(closure)", "(compiled code)", "(string)", "(synthetic)", "(system)", "Array", "HTMLDivElement", "Object", "Window"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("constructors = %v, want %v", names, want)
	}
}

func TestReadHeaderReturnsMetaAndCounts(t *testing.T) {
	h, err := ReadHeader(strings.NewReader(readFixture(t)))
	if err != nil {
		t.Fatal(err)
	}
	if h.NodeCount != 20 || h.EdgeCount != 6 {
		t.Fatalf("header counts = %d/%d, want 20/6", h.NodeCount, h.EdgeCount)
	}
	if len(h.Meta.NodeFields) != 7 || h.Meta.NodeFields[3] != "self_size" {
		t.Fatalf("node_fields = %v", h.Meta.NodeFields)
	}
}

func TestClampTop(t *testing.T) {
	for in, want := range map[int]int{0: DefaultTop, -3: DefaultTop, 5: 5, MaxTop + 1: MaxTop} {
		if got := ClampTop(in); got != want {
			t.Errorf("ClampTop(%d) = %d, want %d", in, got, want)
		}
	}
}

func TestMalformedSnapshotsNameTheSection(t *testing.T) {
	fixture := readFixture(t)
	cases := []struct {
		name    string
		input   string
		section string
	}{
		{"empty", "", SectionDocument},
		{"not an object", "[1,2]", SectionDocument},
		{"no meta", strings.Replace(fixture, `"meta":{`, `"other":{`, 1), SectionMeta},
		{"meta lacks self_size", strings.Replace(fixture, `"self_size"`, `"size"`, 1), SectionMeta},
		{"node count disagrees", strings.Replace(fixture, `"node_count":20`, `"node_count":21`, 1), SectionNodes},
		{"edge count disagrees", strings.Replace(fixture, `"edge_count":6`, `"edge_count":5`, 1), SectionEdges},
		{"truncated nodes", fixture[:strings.Index(fixture, `,2,5,17`)], SectionNodes},
		{"non-integer node", strings.Replace(fixture, `,3,4,11,56`, `,3,4,11,"x"`, 1), SectionNodes},
		{"node type outside enum", strings.Replace(fixture, `,3,4,11,56`, `,99,4,11,56`, 1), SectionNodes},
		{"no strings", strings.Replace(fixture, `"strings":`, `"other":`, 1), SectionStrings},
		{"unterminated string table", fixture[:strings.Index(fixture, `"hello"`)+3], SectionStrings},
		{"name index past the table", strings.Replace(fixture, `,3,4,13,56`, `,3,400,13,56`, 1), SectionStrings},
		{"nodes before snapshot", `{"nodes":[1],` + fixture[1:], SectionNodes},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(strings.NewReader(tc.input))
			var pe *ParseError
			if !errors.As(err, &pe) {
				t.Fatalf("err = %v, want a *ParseError", err)
			}
			if pe.Section != tc.section {
				t.Fatalf("section = %q (%v), want %q", pe.Section, err, tc.section)
			}
			if !strings.Contains(err.Error(), tc.section) {
				t.Fatalf("message %q does not name section %q", err.Error(), tc.section)
			}
		})
	}
}

func TestSinkStreamsTwentyMegabytesWithBoundedAllocation(t *testing.T) {
	const chunkSize = 256 << 10
	const total = 20 << 20
	chunk := strings.Repeat("7", chunkSize)
	f, err := os.Create(filepath.Join(t.TempDir(), "stream"+Ext))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	sink := NewSink(f, 0)

	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	for written := 0; written < total; written += chunkSize {
		if err := sink.WriteChunk(chunk); err != nil {
			t.Fatal(err)
		}
	}
	runtime.ReadMemStats(&after)

	if sink.Bytes() != total {
		t.Fatalf("sink wrote %d bytes, want %d", sink.Bytes(), total)
	}
	if grew := after.TotalAlloc - before.TotalAlloc; grew >= 2*chunkSize {
		t.Fatalf("streaming %d bytes allocated %d, want under 2x the %d-byte chunk", total, grew, chunkSize)
	}
	info, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != total {
		t.Fatalf("file holds %d bytes, want %d", info.Size(), total)
	}
}

func TestSinkRefusesTheChunkThatCrossesTheCap(t *testing.T) {
	var buf bytes.Buffer
	sink := NewSink(&buf, 10)
	if err := sink.WriteChunk("12345"); err != nil {
		t.Fatal(err)
	}
	if err := sink.WriteChunk("123456"); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err = %v, want ErrTooLarge", err)
	}
	if err := sink.WriteChunk("1"); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("a sink past its cap accepted more: %v", err)
	}
	if buf.String() != "12345" || sink.Bytes() != 5 {
		t.Fatalf("sink wrote %q (%d), want only the chunk under the cap", buf.String(), sink.Bytes())
	}
}

func TestPathForIDRefusesAnythingButAPlainName(t *testing.T) {
	dir := t.TempDir()
	got, err := PathForID(dir, "heap_20260913_101010-1")
	if err != nil || got != filepath.Join(dir, "heap_20260913_101010-1"+Ext) {
		t.Fatalf("PathForID = %q, %v", got, err)
	}
	if IDFromPath(got) != "heap_20260913_101010-1" {
		t.Fatalf("IDFromPath(%q) = %q", got, IDFromPath(got))
	}
	for _, bad := range []string{"", "../etc/passwd", "a/b", ".hidden", "x.heapsnapshot", strings.Repeat("a", 200)} {
		if _, err := PathForID(dir, bad); !errors.Is(err, ErrInvalidID) {
			t.Errorf("PathForID(%q) err = %v, want ErrInvalidID", bad, err)
		}
	}
}
