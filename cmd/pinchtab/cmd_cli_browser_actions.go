package main

import (
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

var mouseMoveCmd = newOptionalRefActionCmd("move [ref|selector]", "Move mouse to coordinates or element center", "mousemove")

var mouseDownCmd = newOptionalRefActionCmd("down [ref|selector]", "Press mouse button", "mousedown")

var mouseUpCmd = newOptionalRefActionCmd("up [ref|selector]", "Release mouse button", "mouseup")

var mouseWheelCmd = newOptionalRefActionCmd("wheel [ref|selector]", "Dispatch mouse wheel deltas", "mousewheel")

var focusCmd = newOptionalRefActionCmd("focus <ref>", "Focus element", "focus")

var scrollCmd = newSimpleActionCmd("scroll <ref|pixels>", "Scroll to element or by pixels", "scroll", cobra.MinimumNArgs(1))

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
