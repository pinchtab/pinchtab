package bridge

const (
	ActionClick          = "click"
	ActionDoubleClick    = "dblclick"
	ActionType           = "type"
	ActionFill           = "fill"
	ActionPress          = "press"
	ActionFocus          = "focus"
	ActionHover          = "hover"
	ActionSelect         = "select"
	ActionScroll         = "scroll"
	ActionDrag           = "drag"
	ActionHumanClick     = "humanClick"
	ActionHumanType      = "humanType"
	ActionCheck          = "check"
	ActionUncheck        = "uncheck"
	ActionKeyboardType   = "keyboard-type"
	ActionKeyboardInsert = "keyboard-inserttext"
	ActionKeyDown        = "keydown"
	ActionKeyUp          = "keyup"
	ActionScrollIntoView = "scrollintoview"
)

func (b *Bridge) InitActionRegistry() {
	b.Actions = map[string]ActionFunc{
		ActionClick:          b.actionClick,
		ActionDoubleClick:    b.actionDoubleClick,
		ActionType:           b.actionType,
		ActionFill:           b.actionFill,
		ActionPress:          b.actionPress,
		ActionFocus:          b.actionFocus,
		ActionHover:          b.actionHover,
		ActionSelect:         b.actionSelect,
		ActionScroll:         b.actionScroll,
		ActionDrag:           b.actionDrag,
		ActionHumanClick:     b.actionHumanClick,
		ActionHumanType:      b.actionHumanType,
		ActionCheck:          b.actionCheck,
		ActionUncheck:        b.actionUncheck,
		ActionKeyboardType:   b.actionKeyboardType,
		ActionKeyboardInsert: b.actionKeyboardInsert,
		ActionKeyDown:        b.actionKeyDown,
		ActionKeyUp:          b.actionKeyUp,
		ActionScrollIntoView: b.actionScrollIntoView,
	}
}
