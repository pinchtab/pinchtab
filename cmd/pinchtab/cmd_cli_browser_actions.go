package main

import (
	"github.com/pinchtab/pinchtab/internal/bridge"
	browseractions "github.com/pinchtab/pinchtab/internal/cli/actions"
	"github.com/spf13/cobra"
)

func newOptionalRefActionCmd(use, short, action string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			runCLI(func(rt cliRuntime) {
				browseractions.Action(rt.client, rt.base, rt.token, action, optionalArg(args), cmd)
			})
		},
	}
}

func newSimpleActionCmd(use, short, action string, argsValidator cobra.PositionalArgs) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  argsValidator,
		Run: func(cmd *cobra.Command, args []string) {
			runCLI(func(rt cliRuntime) {
				browseractions.ActionSimple(rt.client, rt.base, rt.token, action, args, cmd)
			})
		},
	}
}

func newRequiredRefActionCmd(use, short, action string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			runCLI(func(rt cliRuntime) {
				browseractions.Action(rt.client, rt.base, rt.token, action, args[0], cmd)
			})
		},
	}
}

func newMouseActionCmd(use, short, action string, argsValidator cobra.PositionalArgs) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  argsValidator,
		Run: func(cmd *cobra.Command, args []string) {
			runCLI(func(rt cliRuntime) {
				browseractions.MouseAction(rt.client, rt.base, rt.token, action, args, cmd)
			})
		},
	}
}

var clickCmd = newOptionalRefActionCmd("click <ref>", "Click element", "click")

var dblclickCmd = newOptionalRefActionCmd("dblclick <ref>", "Double-click element", "dblclick")

var typeCmd = newSimpleActionCmd("type <ref> <text>", "Type into element", "type", cobra.MinimumNArgs(2))

var pressCmd = newSimpleActionCmd("press <key>", "Press key (Enter, Tab, Escape...)", "press", cobra.MinimumNArgs(1))

var fillCmd = newSimpleActionCmd("fill <ref|selector> <text>", "Fill input directly", "fill", cobra.MinimumNArgs(2))

var hoverCmd = newOptionalRefActionCmd("hover <ref>", "Hover element", "hover")

var mouseCmd = &cobra.Command{
	Use:   "mouse",
	Short: "Low-level mouse actions (move, down, up, wheel)",
}

var mouseMoveCmd = newMouseActionCmd("move [x y|ref|selector]", "Move mouse to coordinates or element center", bridge.ActionMouseMove, cobra.RangeArgs(0, 2))

var mouseDownCmd = newMouseActionCmd("down [ref|selector]", "Press mouse button", bridge.ActionMouseDown, cobra.MaximumNArgs(1))

var mouseUpCmd = newMouseActionCmd("up [ref|selector]", "Release mouse button", bridge.ActionMouseUp, cobra.MaximumNArgs(1))

var mouseWheelCmd = newMouseActionCmd("wheel [dy|ref|selector]", "Dispatch mouse wheel deltas", bridge.ActionMouseWheel, cobra.MaximumNArgs(1))

var dragCmd = &cobra.Command{
	Use:   "drag <from> <to> | <selector> --drag-x <n> --drag-y <n>",
	Short: "Drag from one target to another (or by pixel offset)",
	Long: `Drag a DOM element.

Two forms:
  pinchtab drag <from> <to>
      Synthesizes mouse-move → mouse-down → mouse-move → mouse-up.
      Each target is a selector (CSS, ref, text:) or an "x,y" coord pair.

  pinchtab drag <selector> --drag-x <n> --drag-y <n>
      Single-step HTTP "drag" action with pixel offsets from the element's
      current position. Symmetric with the HTTP /action body
      {"kind":"drag","selector":"...","dragX":N,"dragY":N}.`,
	Args: cobra.RangeArgs(1, 2),
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.Drag(rt.client, rt.base, rt.token, args, cmd)
		})
	},
}

var focusCmd = newOptionalRefActionCmd("focus <ref>", "Focus element", "focus")

var scrollCmd = &cobra.Command{
	Use:   "scroll <pixels|direction|selector>",
	Short: "Scroll the page by pixels, in a direction, or to an element",
	Long: `Scroll the page. The single positional argument is interpreted by precedence:

  1. Integer → scrollY in pixels (positive down, negative up).
     pinchtab scroll 800
     pinchtab scroll -300

  2. Direction keyword: up | down | left | right (defaults to 800px per step).
     pinchtab scroll down
     pinchtab scroll right

  3. Otherwise, a unified selector — ref, CSS, XPath, text:, or semantic:.
     The element is scrolled into view.
     pinchtab scroll e12
     pinchtab scroll '#footer'
     pinchtab scroll '//footer'
     pinchtab scroll 'text:Load more'

Precedence: integer and direction keywords win over selector parsing so that
'up'/'down' are treated as directions, not as CSS tag selectors.`,
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.ActionSimple(rt.client, rt.base, rt.token, "scroll", args, cmd)
		})
	},
}

var selectCmd = newSimpleActionCmd("select <ref> <value>", "Select option in dropdown", "select", cobra.MinimumNArgs(2))

var checkCmd = newRequiredRefActionCmd("check <selector>", "Check a checkbox or radio", "check")

var uncheckCmd = newRequiredRefActionCmd("uncheck <selector>", "Uncheck a checkbox or radio", "uncheck")

var keyboardCmd = &cobra.Command{
	Use:   "keyboard",
	Short: "Keyboard commands (type, inserttext)",
}

var keyboardTypeCmd = newSimpleActionCmd("type <text>", "Type text at current focus via keystroke events", "keyboard-type", cobra.MinimumNArgs(1))

var keyboardInsertTextCmd = newSimpleActionCmd("inserttext <text>", "Insert text at current focus (paste-like, no key events)", "keyboard-inserttext", cobra.MinimumNArgs(1))

var keydownCmd = newSimpleActionCmd("keydown <key>", "Hold a key down", "keydown", cobra.ExactArgs(1))

var keyupCmd = newSimpleActionCmd("keyup <key>", "Release a key", "keyup", cobra.ExactArgs(1))

var scrollintoviewCmd = newOptionalRefActionCmd("scrollintoview <selector>", "Scroll element into view and return bounding box", "scrollintoview")

var dialogCmd = &cobra.Command{
	Use:   "dialog",
	Short: "Handle JavaScript dialogs (alert, confirm, prompt)",
}

var dialogAcceptCmd = &cobra.Command{
	Use:   "accept [text]",
	Short: "Accept (OK) the current dialog",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.Dialog(rt.client, rt.base, rt.token, "accept", optionalArg(args), stringFlag(cmd, "tab"))
		})
	},
}

var dialogDismissCmd = &cobra.Command{
	Use:   "dismiss",
	Short: "Dismiss (Cancel) the current dialog",
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.Dialog(rt.client, rt.base, rt.token, "dismiss", "", stringFlag(cmd, "tab"))
		})
	},
}
