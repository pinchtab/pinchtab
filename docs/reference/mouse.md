# Mouse

Low-level pointer controls for drag handles, canvas-like UIs, hover-driven menus, and flows where DOM-native `click` or `hover` are not enough.

## CLI

```bash
pinchtab mouse move <x> <y>
pinchtab mouse move <selector>

pinchtab mouse down [selector] --button left
pinchtab mouse up [selector] --button left

pinchtab mouse wheel <dy> [--dx <n>]
pinchtab mouse wheel [selector]

pinchtab drag <from> <to>
```

Examples:

```bash
# Move to an element, then use current-pointer semantics
pinchtab mouse move e5
pinchtab mouse down --button left
pinchtab mouse move 400 320
pinchtab mouse up --button left

# Explicitly target down/up at an element
pinchtab mouse down e5 --button left
pinchtab mouse up e5 --button left

# Wheel at current pointer
pinchtab mouse wheel 240 --dx 40

# Wheel at a fresh target
pinchtab mouse wheel e5

# Drag from an element to coordinates
pinchtab drag e5 400,320
```

Notes:

- `mouse move` accepts either coordinates or a unified selector.
- `mouse down` and `mouse up` accept an optional selector. Without one, they use the current pointer position.
- `mouse wheel` accepts either a delta form (`<dy> [--dx <n>]`) or an optional selector. Without a selector, it uses the current pointer position.
- `drag <from> <to>` accepts selector/ref targets or `x,y` coordinate pairs.
- `button` supports `left`, `right`, and `middle`.

## HTTP API

Canonical action kinds:

- `mouse-move`
- `mouse-down`
- `mouse-up`
- `mouse-wheel`
- `drag`

Targeting fields:

- `ref`
- `selector`
- `nodeId`
- `x` and `y`

Wheel fields:

- `deltaX`
- `deltaY`

Examples:

```bash
# Move to an element
curl -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"mouse-move","ref":"e5"}'

# Move to coordinates
curl -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"mouse-move","x":120,"y":220}'

# Press/release at current pointer
curl -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"mouse-down","button":"left"}'

curl -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"mouse-up","button":"left"}'

# Press/release at an explicit target
curl -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"mouse-down","ref":"e5","button":"left"}'

curl -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"mouse-up","ref":"e5","button":"left"}'

# Wheel at current pointer
curl -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"mouse-wheel","deltaY":240,"deltaX":40}'

# Wheel at explicit coordinates
curl -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"mouse-wheel","x":400,"y":320,"deltaY":240}'
```

Tab-scoped example:

```bash
curl -X POST http://localhost:9867/tabs/<tabId>/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"mouse-move","ref":"e5"}'
```

## Behavior

- POST coordinate bodies work with plain `x` and `y`; no extra `hasXY` flag is required.
- `mouse-down`, `mouse-up`, and `mouse-wheel` use per-tab current-pointer state when you omit a fresh target.
- If no current pointer position is known yet, `mouse-down`, `mouse-up`, and `mouse-wheel` fail with a clear error. Use `mouse-move` first or pass an explicit target.
- `mouse-wheel` defaults to vertical scrolling when only `deltaY` is provided.

## Related Pages

- [Click](./click.md)
- [Hover](./hover.md)
- [Scroll](./scroll.md)
- [CLI](./cli.md)
