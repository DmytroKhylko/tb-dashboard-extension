# tbres

Embeds a JS extension bundle into an exported ThingsBoard dashboard, or swaps an
embedded bundle back to an external URL such as a local dev server. The rest of
the dashboard JSON is left byte-for-byte unchanged, so diffs stay small.

## Build

With [mise](https://mise.jdx.dev) (installs the pinned Go version):

```sh
mise run build     # dist/tbres
mise run check     # gofmt, go vet, tests
mise run publish   # build and copy to $TBRES_PUBLISH_DIR
```

`publish` needs `TBRES_PUBLISH_DIR`. Set it per machine in `mise.local.toml`,
which is git-ignored:

```toml
[env]
TBRES_PUBLISH_DIR = "/path/to/your/bin"
```

Without mise: `go build -o dist/tbres ./cmd/tbres`, or
`go install github.com/thingsboard/tb-dashboard-extension/cmd/tbres@latest`.

## Usage

```sh
# what is embedded and what the configuration references
tbres list dashboard.json

# dev dashboard -> self-contained dashboard
tbres embed -file thingsboard-extension-widgets.js \
  -replace-url http://localhost:5000/static/widgets/thingsboard-extension-widgets.js \
  -title "FE UI Extension" -out dashboard-embedded.json dashboard.json

# refresh the bundle inside an already embedded dashboard
tbres embed -file thingsboard-extension-widgets.js -in-place dashboard-embedded.json

# embedded dashboard -> dev dashboard
tbres link -resource thingsboard-extension-widgets.js \
  -url http://localhost:5000/static/widgets/thingsboard-extension-widgets.js \
  -out dashboard.json dashboard-embedded.json
```

Without `-out` or `-in-place` the result is written to stdout. Every string in
`configuration` equal to the URL is replaced, not only widget resources, so
references such as action `customResources` are covered too.

## How ThingsBoard imports an embedded bundle

- An existing resource is reused only if its content is identical (sha256).
- A changed bundle is created as a new resource with a deduplicated key, for
  example `thingsboard-extension-widgets_(1).js`, and only the imported dashboard
  is pointed at it. Older copies stay in the resource library.
- Non-image resources are not created for customer-owned dashboards.

Use embedded dashboards for handover and new environments. For repeated deploys
to the same tenant, update the existing resource instead of re-importing.
