# Keyboard

Low-level keyboard input commands for typing text and dispatching key events.

## keyboard type

Type text by dispatching individual key events (keyDown/keyUp) for each character. This triggers keyboard event listeners that some applications depend on.

```bash
pinchtab keyboard type "hello world"
```

**Performance note:** For strings longer than 20 characters, PinchTab uses a hybrid approach to avoid CDP timeouts: the first and last 5 characters are typed with real key events, while the middle portion uses `Input.insertText`. This provides realistic keystroke simulation at boundaries while keeping performance acceptable for long strings.

## keyboard inserttext

Insert text directly without dispatching key events. Equivalent to pasting text — faster but won't trigger keydown/keypress/keyup listeners.

```bash
pinchtab keyboard inserttext "test@pinchtab.com"
```

Use `inserttext` when:
- You need maximum speed
- The target doesn't rely on key event listeners
- You're filling forms programmatically

Use `type` when:
- The application validates input on keypress
- You need to trigger autocomplete or live search
- You're simulating realistic user input

## keydown / keyup

Hold or release individual keys. Useful for modifier keys (Shift, Ctrl, Alt) or testing key-hold behaviors.

```bash
pinchtab keydown Shift
pinchtab keyboard type "abc"   # Types "ABC" (shift held)
pinchtab keyup Shift
```

## API Equivalent

```bash
curl -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"keyboard-type","text":"hello world"}'

curl -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"keyboard-inserttext","text":"hello world"}'
```

## Related Pages

- [Type](./type.md) — Type into a specific element by selector/ref
- [Fill](./fill.md) — Set input value directly
- [Press](./press.md) — Press a single key
