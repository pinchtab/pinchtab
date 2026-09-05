package types

import (
	"encoding/json"
	"github.com/pinchtab/pinchtab/internal/srccensus"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/bridge/observe"
)

// captureGoldens are real /capture envelopes taken from a running bridge before
// this type existed, one per shape that adds keys the others do not: output=file
// (image.path), output=inline (image.base64), a selector scope (image.clip and
// coordinateSpace "clip") and a frame scope (the frame disclosure). The image
// base64 is truncated and the node list cut to two — both are values, not keys,
// and the shape is what these pin.
var captureGoldens = map[string]string{
	"file":   `{"capturedAt":"2026-09-05T01:35:25.181601Z","epoch":{"domEpoch":"ep_RZZZQO64TBSDY7W2FXUA","frameId":"8D31B612CC7B7B35A5D51AD58BDB885C","loaderId":"6BB9389B34CCB9EDBA5C34AF3A613163"},"image":{"bytes":25862,"coordinateSpace":"viewport","devicePixelRatio":1,"format":"jpeg","path":"/tmp/pt320/.pinchtab/captures/cap-20260905-033525.jpg","viewport":{"h":1297,"scrollX":0,"scrollY":0,"w":2560}},"pairing":{"captureDurationMs":723,"navigated":false},"snapshot":{"filter":"interactive","nodeCount":7,"nodes":[{"boundingBox":{"h":37,"w":2544,"x":8,"y":21.4375},"depth":3,"frameId":"8D31B612CC7B7B35A5D51AD58BDB885C","frameUrl":"http://localhost:18787/","name":"Hello Sim","nodeId":15,"ref":"e0","role":"heading","tag":"h1","text":"Hello Sim","visible":true},{"boundingBox":{"h":18,"w":64,"x":8,"y":80.875},"depth":3,"frameId":"8D31B612CC7B7B35A5D51AD58BDB885C","frameUrl":"http://localhost:18787/","name":"Go to two","nodeId":16,"ref":"e1","role":"link","tag":"a","text":"Go to two","visible":true}]},"status":"ok","tabId":"8D31B612CC7B7B35A5D51AD58BDB885C","title":"Sim Fixture","url":"http://localhost:18787/"}`,
	"inline": `{"capturedAt":"2026-09-05T01:35:25.914715Z","epoch":{"domEpoch":"ep_RZZZQO64TBSDY7W2FXUA","frameId":"8D31B612CC7B7B35A5D51AD58BDB885C","loaderId":"6BB9389B34CCB9EDBA5C34AF3A613163"},"image":{"base64":"/9j/4AAQSkZJRgABAQAAAQAB","bytes":25862,"coordinateSpace":"viewport","devicePixelRatio":1,"format":"jpeg","viewport":{"h":1297,"scrollX":0,"scrollY":0,"w":2560}},"pairing":{"captureDurationMs":666,"navigated":false},"snapshot":{"filter":"interactive","nodeCount":7,"nodes":[{"boundingBox":{"h":37,"w":2544,"x":8,"y":21.4375},"depth":3,"frameId":"8D31B612CC7B7B35A5D51AD58BDB885C","frameUrl":"http://localhost:18787/","name":"Hello Sim","nodeId":15,"ref":"e0","role":"heading","tag":"h1","text":"Hello Sim","visible":true},{"boundingBox":{"h":18,"w":64,"x":8,"y":80.875},"depth":3,"frameId":"8D31B612CC7B7B35A5D51AD58BDB885C","frameUrl":"http://localhost:18787/","name":"Go to two","nodeId":16,"ref":"e1","role":"link","tag":"a","text":"Go to two","visible":true}]},"status":"ok","tabId":"8D31B612CC7B7B35A5D51AD58BDB885C","title":"Sim Fixture","url":"http://localhost:18787/"}`,
	"scoped": `{"capturedAt":"2026-09-05T01:35:35.526551Z","epoch":{"domEpoch":"ep_RZZZQO64TBSDY7W2FXUA","frameId":"8D31B612CC7B7B35A5D51AD58BDB885C","loaderId":"6BB9389B34CCB9EDBA5C34AF3A613163"},"image":{"base64":"/9j/4AAQSkZJRgABAQAAAQAB","bytes":1375,"clip":{"h":21,"w":72.296875,"x":76,"y":79.875},"coordinateSpace":"clip","devicePixelRatio":1,"format":"jpeg","viewport":{"h":1297,"scrollX":0,"scrollY":0,"w":2560}},"pairing":{"captureDurationMs":296,"navigated":false},"snapshot":{"filter":"interactive","nodeCount":1,"nodes":[{"boundingBox":{"h":21,"w":72.296875,"x":0,"y":0},"depth":0,"frameId":"8D31B612CC7B7B35A5D51AD58BDB885C","frameUrl":"http://localhost:18787/","name":"Press me","nodeId":17,"ref":"e2","role":"button","tag":"button","text":"Press me","visible":true}]},"status":"ok","tabId":"8D31B612CC7B7B35A5D51AD58BDB885C","title":"Sim Fixture","url":"http://localhost:18787/"}`,
	"framed": `{"capturedAt":"2026-09-05T01:36:05.210717Z","epoch":{"domEpoch":"ep_G6KTRCNAKPWEO74G6VTA","frameId":"5D79864DE9323EC32D22E25F711870E6","loaderId":"2A706AD9D509E32C01134E66D4E244D6"},"frame":{"frameId":"16ECA3752CA9A2F4D476F32266412D2A","frameName":"f","frameTitle":"Child Frame","frameUrl":"http://localhost:18796/child.html"},"image":{"base64":"/9j/4AAQSkZJRgABAQAAAQAB","bytes":25050,"coordinateSpace":"viewport","devicePixelRatio":1,"format":"jpeg","viewport":{"h":1297,"scrollX":0,"scrollY":0,"w":2560}},"pairing":{"captureDurationMs":443,"navigated":false},"snapshot":{"filter":"interactive","nodeCount":2,"nodes":[{"boundingBox":{"h":28,"w":384,"x":18,"y":101.78125},"depth":3,"frameId":"16ECA3752CA9A2F4D476F32266412D2A","frameName":"f","frameUrl":"http://localhost:18796/child.html","name":"child","nodeId":14,"ref":"e0","role":"heading","tag":"h2","text":"child","visible":true},{"boundingBox":{"h":21,"w":84.1875,"x":18,"y":149.6875},"depth":3,"frameId":"16ECA3752CA9A2F4D476F32266412D2A","frameName":"f","frameUrl":"http://localhost:18796/child.html","name":"child button","nodeId":16,"ref":"e1","role":"button","tag":"button","text":"child button","visible":true}]},"status":"ok","tabId":"5D79864DE9323EC32D22E25F711870E6","title":"Parent","url":"http://localhost:18796/parent.html"}`,
}

func keyPaths(t *testing.T, raw string) []string {
	t.Helper()
	var decoded any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		t.Fatalf("golden is not JSON: %v", err)
	}
	var paths []string
	var walk func(node any, prefix string)
	walk = func(node any, prefix string) {
		switch value := node.(type) {
		case map[string]any:
			for key, child := range value {
				walk(child, prefix+key+".")
			}
		case []any:
			for _, child := range value {
				walk(child, prefix)
			}
		default:
			paths = append(paths, strings.TrimSuffix(prefix, "."))
		}
	}
	walk(decoded, "")
	sort.Strings(paths)
	return paths
}

// A field the type is missing decodes to nothing and vanishes on re-encode, so
// comparing the golden's key paths with the round-tripped ones is the same
// assertion as "no field the golden populates is left zero".
func TestEveryGoldenEnvelopeRoundTripsWithoutLosingAKey(t *testing.T) {
	for name, golden := range captureGoldens {
		t.Run(name, func(t *testing.T) {
			var envelope CaptureEnvelope
			if err := json.Unmarshal([]byte(golden), &envelope); err != nil {
				t.Fatalf("decode: %v", err)
			}
			encoded, err := json.Marshal(envelope)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			want := keyPaths(t, golden)
			got := keyPaths(t, string(encoded))
			if strings.Join(want, ",") != strings.Join(got, ",") {
				t.Fatalf("round trip changed the envelope:\n golden %v\n  typed %v", want, got)
			}
		})
	}
}

// The two output modes are mutually exclusive on the wire and omitempty is what
// reproduces that; a missing tag would put an empty path beside the base64.
func TestTheTwoOutputModesStayMutuallyExclusive(t *testing.T) {
	var file, inline CaptureEnvelope
	if err := json.Unmarshal([]byte(captureGoldens["file"]), &file); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(captureGoldens["inline"]), &inline); err != nil {
		t.Fatal(err)
	}
	if file.Image.Path == "" || file.Image.Base64 != "" {
		t.Errorf("output=file carried path=%q base64=%q, want the path alone", file.Image.Path, file.Image.Base64)
	}
	if inline.Image.Base64 == "" || inline.Image.Path != "" {
		t.Errorf("output=inline carried path=%q base64=%q, want the bytes alone", inline.Image.Path, inline.Image.Base64)
	}
	var scoped, framed CaptureEnvelope
	if err := json.Unmarshal([]byte(captureGoldens["scoped"]), &scoped); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(captureGoldens["framed"]), &framed); err != nil {
		t.Fatal(err)
	}
	if scoped.Image.Clip == nil || scoped.Image.CoordinateSpace != "clip" {
		t.Errorf("a selector-scoped capture lost its clip: %+v", scoped.Image)
	}
	if file.Image.Clip != nil || file.Frame != nil {
		t.Errorf("an unscoped capture grew a clip or a frame disclosure: %+v", file)
	}
	if framed.Frame == nil || framed.Frame.FrameTitle == "" {
		t.Errorf("a frame-scoped capture lost its disclosure: %+v", framed.Frame)
	}
}

// producerKeys are the string keys HandleCapture writes into its response maps,
// read from the source: a golden ages, the producer does not.
func producerKeys(t *testing.T) map[string]bool {
	t.Helper()
	handlers := srccensus.Load(t, filepath.Join("..", "..", "handlers"), 50)
	producer, ok := handlers.Func("HandleCapture")
	if !ok {
		t.Fatalf("HandleCapture is not declared in %s; the capture producer moved or was renamed, and this census must follow it", handlers.Dir())
	}
	path := filepath.Join(handlers.Dir(), producer.File)
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parse producer: %v", err)
	}
	keys := map[string]bool{}
	record := func(node ast.Expr) {
		lit, ok := node.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return
		}
		if value, err := strconv.Unquote(lit.Value); err == nil {
			keys[value] = true
		}
	}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "HandleCapture" {
			continue
		}
		ast.Inspect(fn, func(node ast.Node) bool {
			switch n := node.(type) {
			case *ast.CompositeLit:
				if _, isMap := n.Type.(*ast.MapType); isMap {
					for _, elt := range n.Elts {
						if kv, ok := elt.(*ast.KeyValueExpr); ok {
							record(kv.Key)
						}
					}
				}
			case *ast.AssignStmt:
				for _, lhs := range n.Lhs {
					if index, ok := lhs.(*ast.IndexExpr); ok {
						record(index.Index)
					}
				}
			}
			return true
		})
	}
	if len(keys) < 20 {
		t.Fatalf("only %d producer keys read from %s; the walk found almost nothing and would pass vacuously", len(keys), path)
	}
	return keys
}

// typeKeyNames collects every json name in the envelope, stopping at the two
// sub-trees the producer does not spell out itself: the snapshot nodes come from
// observe.A11yNode and the frame block from the shared scope disclosure, each
// pinned against its own owner below.
func typeKeyNames(typ reflect.Type, into map[string]bool) {
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		name := strings.Split(field.Tag.Get("json"), ",")[0]
		into[name] = true
		leaf := field.Type
		for leaf.Kind() == reflect.Ptr || leaf.Kind() == reflect.Slice {
			leaf = leaf.Elem()
		}
		if leaf.Kind() != reflect.Struct || leaf == reflect.TypeOf(CaptureNode{}) || leaf == reflect.TypeOf(CaptureFrame{}) {
			continue
		}
		typeKeyNames(leaf, into)
	}
}

// attachedByTheSharedOwner records the keys HandleCapture does NOT spell: the
// scope disclosure is published by frameDisclosure.attach, the shared owner the
// other scoped readers use, so it appears on the wire without a literal here.
var attachedByTheSharedOwner = map[string]string{
	"frame":            "written by frameDisclosure.attach in internal/handlers/frame.go, not by HandleCapture",
	"untrustedContent": "written by trustBoundary.attach in internal/handlers/snapshot_idpi.go, the one owner of the trust boundary every scoped reader uses",
	"idpiNotice":       "written by trustBoundary.attach in internal/handlers/snapshot_idpi.go beside untrustedContent",
}

func TestCaptureEnvelopeAndTheProducerNameTheSameKeys(t *testing.T) {
	produced := producerKeys(t)
	declared := map[string]bool{}
	typeKeyNames(reflect.TypeOf(CaptureEnvelope{}), declared)

	for key := range produced {
		if !declared[key] {
			t.Errorf("HandleCapture writes %q and CaptureEnvelope has no field for it; the CLI and the MCP server would decode it as a zero value", key)
		}
	}
	for key := range declared {
		if produced[key] {
			continue
		}
		if reason, recorded := attachedByTheSharedOwner[key]; recorded {
			t.Logf("%q is not written by HandleCapture: %s", key, reason)
			continue
		}
		t.Errorf("CaptureEnvelope declares %q and HandleCapture writes no such key; either the producer renamed it or the field is describing something the server never sends", key)
	}
}

func jsonNamesOf(typ reflect.Type) []string {
	var names []string
	for i := 0; i < typ.NumField(); i++ {
		names = append(names, strings.Split(typ.Field(i).Tag.Get("json"), ",")[0])
	}
	sort.Strings(names)
	return names
}

func TestCaptureNodeMirrorsTheSnapshotNodeItCarries(t *testing.T) {
	produced := jsonNamesOf(reflect.TypeOf(observe.A11yNode{}))
	declared := jsonNamesOf(reflect.TypeOf(CaptureNode{}))
	if strings.Join(produced, ",") != strings.Join(declared, ",") {
		t.Fatalf("CaptureNode keys %v, want observe.A11yNode's %v", declared, produced)
	}
	if len(produced) < 20 {
		t.Fatalf("walked only %d node keys; the mirror would prove little", len(produced))
	}
}

func TestCaptureFrameMirrorsTheSharedScopeDisclosure(t *testing.T) {
	declared := jsonNamesOf(reflect.TypeOf(CaptureFrame{}))
	produced := append(jsonNamesOf(reflect.TypeOf(bridge.FrameScope{})), "frameTitle")
	sort.Strings(produced)
	if strings.Join(produced, ",") != strings.Join(declared, ",") {
		t.Fatalf("CaptureFrame keys %v, want the disclosure's %v (bridge.FrameScope plus the frameTitle it reads at disclosure time)", declared, produced)
	}
}

// The MCP tool description spells the envelope out key by key, and it is the one
// description nothing compiles against: an agent reads it to decide what it will
// get back. A key it names that the envelope does not have is a promise the
// server stopped keeping.
func TestTheCaptureToolDescriptionNamesOnlyKeysTheEnvelopeHas(t *testing.T) {
	source, err := os.ReadFile(filepath.Join("..", "..", "mcp", "tools.go"))
	if err != nil {
		t.Fatalf("read the tool catalogue: %v", err)
	}
	description := regexp.MustCompile("(?s)pinchtab_capture\",.*?\n\t\t\t" + regexp.QuoteMeta("tabIDParam()")).Find(source)
	if description == nil {
		t.Fatal("the pinchtab_capture description no longer parses out of internal/mcp/tools.go; re-point this guard rather than deleting it")
	}
	spans := regexp.MustCompile("`[^`]*`").FindAllString(string(description), -1)
	if len(spans) == 0 {
		t.Fatal("the description names no keys at all; it used to spell the envelope out")
	}

	declared := map[string]bool{}
	typeKeyNames(reflect.TypeOf(CaptureEnvelope{}), declared)
	typeKeyNames(reflect.TypeOf(CaptureNode{}), declared)
	typeKeyNames(reflect.TypeOf(CaptureFrame{}), declared)

	// The description writes types beside a few keys; a type name is not a promise
	// about a key, so it is recorded here rather than silently skipped.
	notAKey := map[string]string{"bool": "the declared type of visible, not a key"}

	named := 0
	for _, span := range spans {
		for _, word := range regexp.MustCompile("[A-Za-z][A-Za-z0-9]*").FindAllString(span, -1) {
			if declared[word] {
				named++
				continue
			}
			if _, recorded := notAKey[word]; recorded {
				continue
			}
			t.Errorf("the pinchtab_capture description names %q, which CaptureEnvelope does not carry; an agent is being told to expect a key the server does not send", word)
		}
	}
	if named < 15 {
		t.Fatalf("matched only %d envelope keys in the description; the extraction stopped seeing it", named)
	}
}
