package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

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
				browseractions.Action(rt, action, optionalArg(args), cmd)
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
				browseractions.ActionSimple(rt, action, args, cmd)
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
				browseractions.Action(rt, action, args[0], cmd)
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
				browseractions.MouseAction(rt, action, args, cmd)
			})
		},
	}
}

var clickCmd = newOptionalRefActionCmd("click <ref>", "Click element", "click")

var dblclickCmd = newOptionalRefActionCmd("dblclick <ref>", "Double-click element", "dblclick")

var typeCmd = newSimpleActionCmd("type <ref> <text>", "Type into element", "type", cobra.MinimumNArgs(2))

var pressCmd = newSimpleActionCmd("press [ref] <key|chord>", "Press a key or chord (Enter, Ctrl+A, Shift+ArrowLeft), optionally focusing a ref first", "press", cobra.MinimumNArgs(1))

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

Two forms, both one HTTP "drag" action. They differ in how the endpoint of the
drag is expressed — a target or an offset — and both drive the interpolated
pointer sequence HTML5 drag-and-drop needs, so a draggable element fires
dragstart and the destination receives drop.

  pinchtab drag <from> <to>
      TARGET form: drag onto another element. Each target is a selector (CSS,
      ref, text:) or an "x,y" coord pair. Symmetric with the HTTP /action body
      {"kind":"drag","selector":"...","toSelector":"..."}.

  pinchtab drag <selector> --drag-x <n> --drag-y <n>
      OFFSET form: drag by a pixel delta from the element's current position,
      for handles and sliders with no element to drop onto. Symmetric with
      {"kind":"drag","selector":"...","dragX":N,"dragY":N}.`,
	Args: cobra.RangeArgs(1, 2),
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.Drag(rt, args, cmd)
		})
	},
}

var focusCmd = newOptionalRefActionCmd("focus <ref>", "Focus element", "focus")

var scrollCmd = &cobra.Command{
	Use:   "scroll <pixels|direction|selector>",
	Short: "Scroll the page by pixels, in a direction, or to an element",
	Long: `Scroll the page. Give either --dy/--dx or one positional argument.

  1. Pixels: --dy <n> vertically, --dx <n> horizontally (positive down/right).
     A negative count works either way; both forms take --tab in any position.
     pinchtab scroll --dy 800
     pinchtab scroll --dy -300
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

A positional integer still works for a positive count (pinchtab scroll 800).

Precedence: integer and direction keywords win over selector parsing so that
'up'/'down' are treated as directions, not as CSS tag selectors.`,
	Args: scrollArgs,
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.ActionSimple(rt, "scroll", args, cmd)
		})
	},
}

// scrollArgs refuses a second positional, which is what made a mistyped scroll
// silently scroll the WRONG tab: a negative count used to be spellable only as
// `scroll -- -300`, everything after `--` is a positional, and MinimumNArgs(1)
// accepted `--tab <id>` as args[1:] and dropped it — so the action ran on the
// current tab and reported OK. A hand-written `--` still lands here, and still
// refuses. It also refuses the empty and the doubly-specified forms, since
// --dy/--dx and the positional are two spellings of one argument.
func scrollArgs(cmd *cobra.Command, args []string) error {
	byFlag := cmd.Flags().Changed("dy") || cmd.Flags().Changed("dx")
	if len(args) > 1 {
		return fmt.Errorf("accepts at most 1 positional argument, received %d (%s); a negative count needs no escape (pinchtab scroll -300), and flags must not follow --", len(args), strings.Join(args, " "))
	}
	if len(args) == 0 && !byFlag {
		return fmt.Errorf("needs a positional <pixels|direction|selector> or --dy/--dx")
	}
	if len(args) == 1 && byFlag {
		return fmt.Errorf("give either %q or --dy/--dx, not both", args[0])
	}
	// The server owns this rule; the local copy only saves a round trip.
	if len(args) == 1 {
		if px, err := strconv.Atoi(args[0]); err == nil && px == 0 {
			return errZeroScrollDelta
		}
	} else if byFlag && scrollFlagDelta(cmd, "dy") == 0 && scrollFlagDelta(cmd, "dx") == 0 {
		return errZeroScrollDelta
	}
	return nil
}

var errZeroScrollDelta = errors.New("a zero delta is not a scroll: pass a non-zero count, a direction, or a selector to scroll into view")

// scrollFlagDelta is the value of an int scroll flag, 0 when unset or unreadable.
func scrollFlagDelta(cmd *cobra.Command, name string) int {
	if !cmd.Flags().Changed(name) {
		return 0
	}
	value, err := cmd.Flags().GetInt(name)
	if err != nil {
		return 0
	}
	return value
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
			browseractions.Dialog(rt, "accept", optionalArg(args), cmd)
		})
	},
}

var dialogDismissCmd = &cobra.Command{
	Use:   "dismiss",
	Short: "Dismiss (Cancel) the current dialog",
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.Dialog(rt, "dismiss", "", cmd)
		})
	},
}

func configureBrowserActionsFlags() {
	addPointerActionFlags(clickCmd, "click")
	clickCmd.Flags().Bool("wait-nav", false, "Wait for navigation after click")
	clickCmd.Flags().Bool("dismiss-banners", false, "Dismiss cookie/consent banners after a wait-nav click (no-op without --wait-nav)")
	clickCmd.Flags().Bool("dismiss-known-interstitials", false, "Dismiss a recognized portal interstitial before resolving the click target")
	addPostActionFlags(clickCmd, "action", true)
	clickCmd.Flags().String("dialog-action", "", "Auto-handle a JS dialog opened by the click: "+strings.Join(bridge.DialogActions(), " | "))
	clickCmd.Flags().String("dialog-text", "", "Prompt response text (with --dialog-action accept on prompt())")
	clickCmd.Flags().String("mode", "", "Click delivery mode override: "+strings.Join(bridge.ClickModes(), " | "))
	addVocabRefusal(clickCmd, "mode", bridge.ParseClickMode)
	addVocabRefusal(clickCmd, "dialog-action", func(v string) (string, error) { return bridge.ParseDialogAction("dialogAction", v) })
	clickCmd.Flags().Bool("submit", false, "Dispatch one DOM click and report bounded post-submit state")
	addPointerActionFlags(dblclickCmd, "dblclick")
	addPointerActionFlags(hoverCmd, "hover")
	addPointerActionFlags(mouseMoveCmd, bridge.ActionMouseMove)
	addPointerActionFlags(mouseDownCmd, bridge.ActionMouseDown)
	addMouseButtonFlag(mouseDownCmd)
	addPointerActionFlags(mouseUpCmd, bridge.ActionMouseUp)
	addMouseButtonFlag(mouseUpCmd)
	addPointerActionFlags(mouseWheelCmd, bridge.ActionMouseWheel)
	typeCmd.Flags().Bool("humanize", false, "Use humanized per-character keypress timing (overrides instance config)")
	addPostActionFlags(pressCmd, "key press", true)
	mouseWheelCmd.Flags().Int("dx", 0, "Wheel delta X")
	mouseWheelCmd.Flags().Int("dy", 0, "Wheel delta Y")
	scrollCmd.Flags().Int("dy", 0, "Vertical scroll in pixels (negative scrolls up)")
	scrollCmd.Flags().Int("dx", 0, "Horizontal scroll in pixels (negative scrolls left)")
	addMouseButtonFlag(dragCmd)
	dragCmd.Flags().Int("drag-x", 0, "Horizontal pixel offset for single-step drag action")
	dragCmd.Flags().Int("drag-y", 0, "Vertical pixel offset for single-step drag action")
	focusCmd.Flags().String("css", "", "CSS selector instead of ref")
	addPostActionFlags(fillCmd, "fill", true)
	fillCmd.Flags().Bool("submit", false, "Press Enter after filling the field")
	addPostActionFlags(selectCmd, "select", true)
	addPostActionFlags(scrollCmd, "scroll", false)
	addTabFlag(clickCmd)
	addTabFlag(dblclickCmd)
	addTabFlag(hoverCmd)
	addTabFlag(mouseMoveCmd)
	addTabFlag(mouseDownCmd)
	addTabFlag(mouseUpCmd)
	addTabFlag(mouseWheelCmd)
	addTabFlag(dragCmd)
	addTabFlag(focusCmd)
	addTabFlag(typeCmd)
	addTabFlag(pressCmd)
	addTabFlag(fillCmd)
	addTabFlag(scrollCmd)
	addTabFlag(selectCmd)
	addTabFlag(checkCmd)
	addTabFlag(uncheckCmd)
	addTabFlag(keyboardTypeCmd)
	addTabFlag(keyboardInsertTextCmd)
	addTabFlag(keydownCmd)
	addTabFlag(keyupCmd)
	addTabFlag(scrollintoviewCmd)
	addTabFlag(dialogAcceptCmd)
	addTabFlag(dialogDismissCmd)
	addJSONFlag(clickCmd)
	addJSONFlag(dblclickCmd)
	addJSONFlag(hoverCmd)
	addJSONFlag(mouseMoveCmd)
	addJSONFlag(mouseDownCmd)
	addJSONFlag(mouseUpCmd)
	addJSONFlag(mouseWheelCmd)
	addJSONFlag(dragCmd)
	addJSONFlag(focusCmd)
	addJSONFlag(typeCmd)
	addJSONFlag(pressCmd)
	addJSONFlag(fillCmd)
	addJSONFlag(scrollCmd)
	addJSONFlag(selectCmd)
	addJSONFlag(checkCmd)
	addJSONFlag(uncheckCmd)
	addJSONFlag(scrollintoviewCmd)
	addJSONFlag(dialogAcceptCmd)
	addJSONFlag(dialogDismissCmd)
	scrollintoviewCmd.Flags().String("css", "", "CSS selector instead of ref")
}
