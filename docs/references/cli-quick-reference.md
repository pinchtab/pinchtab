# CLI Quick Reference

This quick reference matches the current CLI implementation on this branch.

## Startup

```bash
pinchtab
pinchtab server
pinchtab bridge
```

## Utility Commands

```bash
pinchtab --version
pinchtab help

pinchtab config init
pinchtab config show
pinchtab config path
pinchtab config validate

pinchtab connect <profile>
pinchtab connect <profile> --json
pinchtab connect <profile> --dashboard http://localhost:9867
```

## Environment

```bash
export PINCHTAB_URL=http://127.0.0.1:9867
export PINCHTAB_TOKEN=secret_token
```

Startup-related envs:

```bash
PINCHTAB_PORT=9868 pinchtab
PINCHTAB_BIND=0.0.0.0 pinchtab
CHROME_BIN=/usr/bin/google-chrome pinchtab
```

## Top-Level Browser Control

```bash
pinchtab nav https://pinchtab.com
pinchtab nav https://pinchtab.com --new-tab
pinchtab nav https://pinchtab.com --block-images --block-ads

pinchtab snap
pinchtab snap -i
pinchtab snap -i -c
pinchtab snap -d
pinchtab snap --selector 'main'
pinchtab snap --tab <tabId>

pinchtab click e5
pinchtab type e12 "hello world"
pinchtab fill e12 "value"
pinchtab fill 'input[name=email]' "user@example.com"
pinchtab press Enter
pinchtab hover e5
pinchtab scroll e5
pinchtab scroll 500
pinchtab select e7 option2
pinchtab focus e7

pinchtab text
pinchtab text --raw
pinchtab text --tab <tabId>

pinchtab ss -o out.jpg
pinchtab ss -q 85 -o out.jpg
pinchtab ss --tab <tabId> -o out.jpg

pinchtab eval "document.title"

pinchtab pdf --tab <tabId> -o page.pdf
pinchtab pdf --tab <tabId> -o page.pdf --landscape
pinchtab pdf --tab <tabId> --scale 0.9 -o page.pdf

pinchtab health
pinchtab quick https://pinchtab.com
```

## Instance Commands

```bash
pinchtab instances

pinchtab instance start
pinchtab instance start --mode headed
pinchtab instance start --profileId prof_123
pinchtab instance start --port 9999

pinchtab instance launch
pinchtab instance launch --mode headed

pinchtab instance navigate inst_abc123 https://pinchtab.com

pinchtab instance logs inst_abc123
pinchtab instance logs --id inst_abc123

pinchtab instance stop inst_abc123
pinchtab instance stop --id inst_abc123
```

Notes:
- `launch` is a CLI alias for `start`
- there is no CLI `attach` command yet
- there is no `pinchtab instance <id> logs` grammar

## Tabs

### List And Legacy Lifecycle

```bash
pinchtab tabs
pinchtab tab

pinchtab tabs new https://pinchtab.com
pinchtab tab new https://pinchtab.com

pinchtab tabs close tab_xyz789
pinchtab tab close tab_xyz789
```

### Explicit Tab Operations

```bash
pinchtab tab navigate tab_xyz789 https://google.com
pinchtab tab snapshot tab_xyz789 -i -c
pinchtab tab screenshot tab_xyz789 -o out.png
pinchtab tab click tab_xyz789 e5
pinchtab tab type tab_xyz789 e12 "hello world"
pinchtab tab fill tab_xyz789 e12 "value"
pinchtab tab press tab_xyz789 Enter
pinchtab tab hover tab_xyz789 e5
pinchtab tab scroll tab_xyz789 down
pinchtab tab scroll tab_xyz789 500
pinchtab tab select tab_xyz789 e7 option2
pinchtab tab focus tab_xyz789 e7
pinchtab tab text tab_xyz789 --raw
pinchtab tab eval tab_xyz789 "document.title"
pinchtab tab pdf tab_xyz789 -o page.pdf
pinchtab tab cookies tab_xyz789
pinchtab tab lock tab_xyz789 --owner my-agent --ttl 60
pinchtab tab unlock tab_xyz789 --owner my-agent
pinchtab tab locks tab_xyz789
pinchtab tab info tab_xyz789
```

Important:
- the implemented order is `pinchtab tab <operation> <tabId> ...`
- not `pinchtab tab <tabId> <operation>`

## Profiles

```bash
pinchtab profiles
```

Current behavior:
- lists profile names from the server
- prints human-friendly output, not a structured JSON array

## Common Flows

### Start And Drive A Page

```bash
pinchtab

pinchtab nav https://github.com/pinchtab/pinchtab
pinchtab snap -i -c
pinchtab click e5
pinchtab text --raw
```

### Launch An Instance Then Navigate It

```bash
INST=$(pinchtab instance start --mode headed | jq -r '.id')
pinchtab instance navigate "$INST" https://pinchtab.com
pinchtab tabs
```

### Work Directly With A Known Tab

```bash
TAB=tab_xyz789

pinchtab tab snapshot "$TAB" -i -c
pinchtab tab click "$TAB" e5
pinchtab tab text "$TAB" --raw
pinchtab tab pdf "$TAB" -o page.pdf
```
