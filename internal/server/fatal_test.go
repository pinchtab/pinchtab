package server

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/token"
	"io"
	"log/slog"
	"net"
	"os"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/srccensus"
)

type fatalRun struct {
	stderr string
	codes  []int
}

func runFatal(t *testing.T, fn func()) fatalRun {
	t.Helper()

	origLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(new(bytes.Buffer), nil)))
	origExit := exitProcess
	var codes []int
	exitProcess = func(code int) { codes = append(codes, code) }
	origStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	os.Stderr = w
	t.Cleanup(func() {
		os.Stderr = origStderr
		exitProcess = origExit
		slog.SetDefault(origLogger)
	})

	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(&buf, r)
		_ = r.Close()
		close(done)
	}()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("close writer error = %v", err)
	}
	<-done
	os.Stderr = origStderr
	return fatalRun{stderr: buf.String(), codes: codes}
}

func TestFatalStartupReportsOnStderrWhileLoggerIsDiscarded(t *testing.T) {
	run := runFatal(t, func() {
		slog.Error("swallowed by the discard handler", "err", "invisible")
		fatalStartup("activity store", errors.New("open ledger: permission denied"))
	})

	for _, want := range []string{"activity store", "open ledger: permission denied"} {
		if !strings.Contains(run.stderr, want) {
			t.Fatalf("stderr %q does not contain %q", run.stderr, want)
		}
	}
	if strings.Contains(run.stderr, "swallowed by the discard handler") {
		t.Fatalf("discarded logger still reached stderr: %q", run.stderr)
	}
	if len(run.codes) != 1 || run.codes[0] != 1 {
		t.Fatalf("exit codes = %v, want [1]", run.codes)
	}
}

func TestFatalStartupPortInUseNamesAddressAndRemedy(t *testing.T) {
	held, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	defer func() { _ = held.Close() }()
	addr := held.Addr().String()

	_, listenErr := net.Listen("tcp", addr)
	if listenErr == nil {
		t.Fatal("expected the second listen on a held address to fail")
	}

	run := runFatal(t, func() {
		fatalStartup("cannot listen on "+addr, listenErr)
	})

	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("net.SplitHostPort() error = %v", err)
	}
	for _, want := range []string{port, "already listening", "pinchtab server stop", "pinchtab daemon"} {
		if !strings.Contains(run.stderr, want) {
			t.Fatalf("stderr %q does not contain %q", run.stderr, want)
		}
	}
}

func TestStartupFatalHintsOnlyForAddressInUse(t *testing.T) {
	if hints := startupFatalHints(errors.New("open ledger: permission denied")); len(hints) != 0 {
		t.Fatalf("unrelated error produced hints: %v", hints)
	}
}

// The discard handler can silence any slog-based fatal, so every log-then-exit
// pair must funnel through fatalStartup; this guards the funnel from a new
// call site re-introducing a direct exit.
func TestFatalStartupIsTheOnlyExitOneSite(t *testing.T) {
	fset, files := srccensus.Load(t, ".", 10).Syntax()
	var offenders []string
	funnelExits := false
	eachFuncCall(files, func(fn *ast.FuncDecl, call *ast.CallExpr) {
		if !isExitOne(call) {
			return
		}
		if fn.Name.Name == "fatalStartup" {
			funnelExits = true
			return
		}
		offenders = append(offenders, fmt.Sprintf("%s (in %s)", fset.Position(call.Pos()), fn.Name.Name))
	})
	if len(offenders) != 0 {
		t.Fatalf("exit-1 outside fatalStartup: %v", offenders)
	}
	if !funnelExits {
		t.Fatal("fatalStartup no longer exits with code 1")
	}
}

func isExitOne(call *ast.CallExpr) bool {
	name := calleeName(call)
	if name != "os.Exit" && name != "exitProcess" || len(call.Args) != 1 {
		return false
	}
	code, ok := call.Args[0].(*ast.BasicLit)
	return ok && code.Value == "1"
}

func eachFuncCall(files map[string]*ast.File, visit func(fn *ast.FuncDecl, call *ast.CallExpr)) {
	for _, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				if call, ok := node.(*ast.CallExpr); ok {
					visit(fn, call)
				}
				return true
			})
		}
	}
}

func calleeName(call *ast.CallExpr) string {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		return fun.Name
	case *ast.SelectorExpr:
		if recv, ok := fun.X.(*ast.Ident); ok {
			return recv.Name + "." + fun.Sel.Name
		}
	}
	return ""
}

// "dashboard started" must follow a successful bind; the bind is the step that
// fails when the port is taken, so announcing it first misreports the failure.
func TestDashboardStartedIsLoggedAfterBind(t *testing.T) {
	_, files := srccensus.Load(t, ".", 10).Syntax()
	calls := map[*ast.FuncDecl]map[string]token.Pos{}
	eachFuncCall(files, func(fn *ast.FuncDecl, call *ast.CallExpr) {
		marker := calleeName(call)
		if marker == "slog.Info" && len(call.Args) > 0 {
			if msg, ok := call.Args[0].(*ast.BasicLit); ok && msg.Value == `"dashboard started"` {
				marker = "started"
			}
		}
		if calls[fn] == nil {
			calls[fn] = map[string]token.Pos{}
		}
		if _, seen := calls[fn][marker]; !seen {
			calls[fn][marker] = call.Pos()
		}
	})

	announcers := 0
	for fn, markers := range calls {
		started, ok := markers["started"]
		if !ok {
			continue
		}
		announcers++
		bind, serve := markers["net.Listen"], markers["srv.Serve"]
		if bind == token.NoPos || serve == token.NoPos {
			t.Fatalf("%s logs \"dashboard started\" but no longer binds with net.Listen and serves with srv.Serve; pin the ordering against whatever binds the dashboard now", fn.Name.Name)
		}
		if bind >= started || started >= serve {
			t.Fatalf("%s must bind, then log \"dashboard started\", then serve", fn.Name.Name)
		}
	}
	if announcers != 1 {
		t.Fatalf("found %d functions logging \"dashboard started\", want 1", announcers)
	}
}
