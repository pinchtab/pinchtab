package heapsnap

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

const grownFixturePath = "testdata/grown.heapsnapshot"

func parseFixture(t testing.TB, path string, retained bool) *Aggregate {
	t.Helper()
	agg, err := ParseFileWith(path, ParseOptions{Retained: retained})
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return agg
}

func TestCompareOverTheFixturesReturnsTheDeltaTableDeterministically(t *testing.T) {
	want := Comparison{
		Base:      Totals{NodeCount: 20, EdgeCount: 6, TotalSelfSize: 3536},
		Head:      Totals{NodeCount: 15, EdgeCount: 17, TotalSelfSize: 10820},
		NodeDelta: -5,
		SizeDelta: 7284,
		Changed:   7,
		Constructors: []ConstructorDelta{
			{Name: "(array)", BaseCount: 1, HeadCount: 2, CountDelta: 1, BaseSelfSize: 800, HeadSelfSize: 10000, SizeDelta: 9200},
			{Name: "Array", BaseCount: 3, HeadCount: 2, CountDelta: -1, BaseSelfSize: 1664, HeadSelfSize: 64, SizeDelta: -1600},
			{Name: "HTMLDivElement", BaseCount: 1, CountDelta: -1, BaseSelfSize: 200, SizeDelta: -200},
			{Name: "(string)", BaseCount: 8, HeadCount: 5, CountDelta: -3, BaseSelfSize: 228, HeadSelfSize: 160, SizeDelta: -68},
			{Name: "Leaker", HeadCount: 1, CountDelta: 1, HeadSelfSize: 64, SizeDelta: 64},
			{Name: "(closure)", BaseCount: 1, CountDelta: -1, BaseSelfSize: 64, SizeDelta: -64},
			{Name: "(system)", BaseCount: 1, CountDelta: -1, BaseSelfSize: 48, SizeDelta: -48},
		},
		NewDuplicateStrings: []DuplicateString{
			{Value: "new-dup-string", Length: 14, Count: 2, SelfSize: 80},
		},
	}
	var first []byte
	for run := range 2 {
		got, err := Compare(parseFixture(t, fixturePath, false), parseFixture(t, grownFixturePath, false), Options{})
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("run %d comparison =\n%+v\nwant\n%+v", run, got, want)
		}
		raw, err := json.Marshal(got)
		if err != nil {
			t.Fatal(err)
		}
		if run == 0 {
			first = raw
		} else if string(raw) != string(first) {
			t.Fatalf("second run marshalled differently:\n%s\n%s", first, raw)
		}
	}
	if strings.Contains(string(first), "retainedSize") {
		t.Fatalf("a comparison without retained carries retainedSize: %s", first)
	}
}

func TestCompareTopCutsTheTableButCountsEveryChangedConstructor(t *testing.T) {
	got, err := Compare(parseFixture(t, fixturePath, false), parseFixture(t, grownFixturePath, false), Options{Top: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Constructors) != 2 || got.Constructors[0].Name != "(array)" || got.Constructors[1].Name != "Array" {
		t.Fatalf("top 2 = %+v", got.Constructors)
	}
	if got.Changed != 7 {
		t.Fatalf("changed = %d, want 7", got.Changed)
	}
}

func TestCompareOfASnapshotWithItselfReportsNoChange(t *testing.T) {
	agg := parseFixture(t, grownFixturePath, false)
	got, err := Compare(agg, agg, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Changed != 0 || len(got.Constructors) != 0 || len(got.NewDuplicateStrings) != 0 || got.SizeDelta != 0 {
		t.Fatalf("self comparison = %+v", got)
	}
	if got.Constructors == nil || got.NewDuplicateStrings == nil {
		t.Fatal("empty tables must marshal as [] not null")
	}
}

func TestRetainedSizesFollowTheDominatorTree(t *testing.T) {
	agg := parseFixture(t, grownFixturePath, true)
	want := map[string]int64{
		"(synthetic)":     10820,
		"Window":          10440,
		"Leaker":          10168,
		"Array":           10064,
		"(array)":         10000,
		"(compiled code)": 300,
		"(string)":        160,
		"Object":          152,
	}
	if !reflect.DeepEqual(agg.Retained, want) {
		t.Fatalf("retained = %v, want %v", agg.Retained, want)
	}

	plain := parseFixture(t, grownFixturePath, false)
	if plain.Retained != nil {
		t.Fatalf("a parse without retained computed %v", plain.Retained)
	}
	agg.Retained = nil
	if !reflect.DeepEqual(agg, plain) {
		t.Fatalf("retained parse changed the aggregate:\n%+v\n%+v", agg, plain)
	}
}

func TestCompareWithRetainedAnnotatesTheReturnedRows(t *testing.T) {
	got, err := Compare(parseFixture(t, fixturePath, false), parseFixture(t, grownFixturePath, true), Options{Top: 5, Retained: true})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int64{"(array)": 10000, "Array": 10064, "HTMLDivElement": 0, "(string)": 160, "Leaker": 10168}
	if len(got.Constructors) != len(want) {
		t.Fatalf("rows = %+v", got.Constructors)
	}
	for _, row := range got.Constructors {
		if row.RetainedSize == nil || *row.RetainedSize != want[row.Name] {
			t.Errorf("%s retainedSize = %v, want %d", row.Name, row.RetainedSize, want[row.Name])
		}
	}
	raw, _ := json.Marshal(got.Constructors[2])
	if !strings.Contains(string(raw), `"retainedSize":0`) {
		t.Fatalf("a requested zero retained size must still be present: %s", raw)
	}

	if _, err := Compare(parseFixture(t, fixturePath, false), parseFixture(t, grownFixturePath, false), Options{Retained: true}); !errors.Is(err, ErrRetainedNotParsed) {
		t.Fatalf("err = %v, want ErrRetainedNotParsed", err)
	}
}

func TestRetainedParseRefusesAnInconsistentEdgeTable(t *testing.T) {
	src, err := os.ReadFile(grownFixturePath)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		"to_node off a node boundary": strings.Replace(string(src), "\"edges\":[1,1,7", "\"edges\":[1,1,8", 1),
		"to_node past the last node":  strings.Replace(string(src), "\"edges\":[1,1,7", "\"edges\":[1,1,700", 1),
		"edge_count sum too small":    strings.Replace(string(src), "\"nodes\":[9,1,1,0,5", "\"nodes\":[9,1,1,0,4", 1),
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ParseWith(strings.NewReader(body), ParseOptions{Retained: true})
			var pe *ParseError
			if !errors.As(err, &pe) || pe.Section != SectionEdges {
				t.Fatalf("err = %v, want a ParseError naming edges", err)
			}
		})
	}
}

func TestCacheParsesEachSnapshotOnceUntilItChanges(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "heap_1"+Ext)
	src, err := os.ReadFile(grownFixturePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, src, 0600); err != nil {
		t.Fatal(err)
	}
	var calls []bool
	c := &Cache{parse: func(p string, opts ParseOptions) (*Aggregate, error) {
		calls = append(calls, opts.Retained)
		return ParseFileWith(p, opts)
	}}

	first, err := c.Load(path, false)
	if err != nil {
		t.Fatal(err)
	}
	again, err := c.Load(path, false)
	if err != nil || again != first {
		t.Fatalf("second load = %p, %v; want the cached %p", again, err, first)
	}
	withRetained, err := c.Load(path, true)
	if err != nil || withRetained.Retained == nil {
		t.Fatalf("retained load = %+v, %v", withRetained, err)
	}
	if plain, _ := c.Load(path, false); plain != withRetained {
		t.Fatal("a plain load after a retained one must reuse the richer entry")
	}
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Load(path, false); err != nil {
		t.Fatal(err)
	}
	if want := []bool{false, true, false}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("parse calls = %v, want %v", calls, want)
	}
	if _, err := c.Load(filepath.Join(dir, "missing"+Ext), false); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing file err = %v, want os.ErrNotExist", err)
	}
}

func writeSyntheticSnapshot(tb testing.TB, path string, targetBytes int) {
	tb.Helper()
	f, err := os.Create(path)
	if err != nil {
		tb.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	const perObject, rootEvery = 3, 16
	objects := targetBytes / 115
	nodes := 1 + objects*perObject
	rootEdges := (objects + rootEvery - 1) / rootEvery
	edges := rootEdges + objects*4
	objectOffset := func(i int) int { return (1 + i*perObject) * 7 }
	meta := strings.SplitN(readFixtureTB(tb), `"node_count"`, 2)[0]
	var b strings.Builder
	b.WriteString(meta)
	fmt.Fprintf(&b, `"node_count":%d,"edge_count":%d,"trace_function_count":0},`+"\n", nodes, edges)
	fmt.Fprintf(&b, `"nodes":[9,1,1,0,%d,0,0`, rootEdges)
	id := 3
	for i := range objects {
		fmt.Fprintf(&b, "\n,3,%d,%d,%d,4,0,0", 2+i%4, id, 32+i%64)
		fmt.Fprintf(&b, "\n,1,6,%d,%d,0,0,0", id+2, 16+i%512)
		fmt.Fprintf(&b, "\n,2,%d,%d,%d,0,0,0", 6+i%3, id+4, 20+i%8)
		id += 6
	}
	b.WriteString("],\n\"edges\":[")
	sep := ""
	for i := 0; i < objects; i += rootEvery {
		fmt.Fprintf(&b, "%s1,%d,%d", sep, i, objectOffset(i))
		sep = "\n,"
	}
	for i := range objects {
		obj := 1 + i*perObject
		fmt.Fprintf(&b, "\n,3,8,%d\n,2,7,%d\n,2,6,%d\n,2,8,%d", (obj+1)*7, (obj+2)*7, objectOffset((i*7919+1)%objects), objectOffset((i+1)%objects))
	}
	b.WriteString("],\n\"strings\":[\"<dummy>\",\"(GC roots)\",\"Window\",\"Array\",\"Object\",\"Leaker\",\"hello\",\"leak\",\"items\"]}\n")
	if _, err := f.WriteString(b.String()); err != nil {
		tb.Fatal(err)
	}
	if info, err := f.Stat(); err == nil && targetBytes >= 1<<20 && info.Size() < int64(targetBytes)*9/10 {
		tb.Fatalf("synthetic snapshot is %d bytes, want about %d", info.Size(), targetBytes)
	}
}

func readFixtureTB(tb testing.TB) string {
	tb.Helper()
	raw, err := os.ReadFile(fixturePath)
	if err != nil {
		tb.Fatal(err)
	}
	return string(raw)
}

func TestSyntheticSnapshotParsesWithAndWithoutRetained(t *testing.T) {
	path := filepath.Join(t.TempDir(), "synthetic"+Ext)
	writeSyntheticSnapshot(t, path, 64<<10)
	plain := parseFixture(t, path, false)
	rich := parseFixture(t, path, true)
	if rich.Retained["(synthetic)"] != plain.TotalSelfSize {
		t.Fatalf("root retains %d, want every byte %d", rich.Retained["(synthetic)"], plain.TotalSelfSize)
	}
}

func benchmarkCompare(b *testing.B, retained bool) {
	path := filepath.Join(b.TempDir(), "synthetic"+Ext)
	writeSyntheticSnapshot(b, path, 20<<20)
	info, err := os.Stat(path)
	if err != nil {
		b.Fatal(err)
	}
	b.SetBytes(info.Size())
	base, err := ParseFile(path)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		head, err := ParseFileWith(path, ParseOptions{Retained: retained})
		if err != nil {
			b.Fatal(err)
		}
		if _, err := Compare(base, head, Options{Retained: retained}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCompare20MB(b *testing.B)         { benchmarkCompare(b, false) }
func BenchmarkCompare20MBRetained(b *testing.B) { benchmarkCompare(b, true) }
