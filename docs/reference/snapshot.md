# Snapshot

Get an accessibility snapshot of the current page, including element refs that can be reused by action commands.

```bash
curl "http://localhost:9867/snapshot?filter=interactive"
# CLI Alternative
pinchtab snap -i
# Response
{
  "url": "https://pinchtab.com",
  "title": "Example Domain",
  "nodes": [
    {
      "ref": "e5",
      "role": "link",
      "name": "More information..."
    }
  ],
  "count": 1
}
```

Useful flags:

- CLI: `-i`, `-c`, `-d`, `--selector`, `--max-tokens`, `--depth`
- API query: `filter`, `format`, `diff`, `selector`, `maxTokens`, `depth`

## Related Pages

- [Click](./click.md)
- [Tabs](./tabs.md)
