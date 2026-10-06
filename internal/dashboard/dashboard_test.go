package dashboard

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

const (
	devURL      = "http://localhost:5000/static/widgets/thingsboard-extension-widgets.js"
	bundleName  = "thingsboard-extension-widgets.js"
	bundleLink  = "/api/resource/js_module/tenant/thingsboard-extension-widgets.js"
	bundleTitle = "FE UI Extension"

	// Four HTML containers plus a map action button's custom resource.
	bundleReferences = 5
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func parse(t *testing.T, doc []byte) *Dashboard {
	t.Helper()
	d, err := Parse(doc)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func assertSameDocument(t *testing.T, got, want []byte) {
	t.Helper()
	got, want = bytes.TrimRight(got, "\n"), bytes.TrimRight(want, "\n")
	if bytes.Equal(got, want) {
		return
	}
	n := 0
	for n < len(got) && n < len(want) && got[n] == want[n] {
		n++
	}
	from := max(0, n-80)
	t.Fatalf("documents differ at byte %d\ngot:  %q\nwant: %q", n, got[from:min(len(got), n+80)], want[from:min(len(want), n+80)])
}

func TestEmbedMatchesThingsBoardExport(t *testing.T) {
	d := parse(t, readFixture(t, "linked.json"))

	result, err := d.Embed(EmbedOptions{
		Data:       readFixture(t, bundleName),
		FileName:   bundleName,
		Title:      bundleTitle,
		ReplaceURL: devURL,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Replaced != bundleReferences || result.Updated || result.Link != bundleLink {
		t.Fatalf("unexpected result %+v", result)
	}
	assertSameDocument(t, d.Bytes(), readFixture(t, "embedded.json"))
}

func TestLinkMatchesDevDashboard(t *testing.T) {
	for _, ref := range []string{bundleName, bundleLink} {
		t.Run(ref, func(t *testing.T) {
			d := parse(t, readFixture(t, "embedded.json"))

			replaced, err := d.Link(ref, devURL)
			if err != nil {
				t.Fatal(err)
			}
			if replaced != bundleReferences {
				t.Fatalf("replaced %d references, want %d", replaced, bundleReferences)
			}
			assertSameDocument(t, d.Bytes(), readFixture(t, "linked.json"))
		})
	}
}

func TestEmbedUpdatesExistingResource(t *testing.T) {
	d := parse(t, readFixture(t, "embedded.json"))

	result, err := d.Embed(EmbedOptions{Data: []byte("new bundle"), FileName: bundleName, Title: bundleTitle})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Updated || result.Replaced != 0 {
		t.Fatalf("unexpected result %+v", result)
	}
	resources, err := d.Resources()
	if err != nil {
		t.Fatal(err)
	}
	if len(resources) != 5 || resources[0].Link != bundleLink || resources[0].Size() != len("new bundle") {
		t.Fatalf("resource was not updated in place: %+v", resources[0])
	}
}

func TestEmbedErrors(t *testing.T) {
	tests := []struct {
		name string
		opts EmbedOptions
		want error
	}{
		{"url not referenced", EmbedOptions{Data: []byte("x"), FileName: bundleName, ReplaceURL: "http://localhost:1/x.js"}, ErrURLNotFound},
		{"nothing to embed", EmbedOptions{Data: []byte("x"), FileName: "other.js"}, ErrNothingToEmbed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := readFixture(t, "linked.json")
			d := parse(t, original)
			if _, err := d.Embed(tt.opts); !errors.Is(err, tt.want) {
				t.Fatalf("got %v, want %v", err, tt.want)
			}
		})
	}
}

func TestEmbedRejectsInvalidKey(t *testing.T) {
	d := parse(t, readFixture(t, "linked.json"))
	if _, err := d.Embed(EmbedOptions{Data: []byte("x"), Key: "../evil.js", ReplaceURL: devURL}); err == nil {
		t.Fatal("expected an error for a key with a path separator")
	}
}

func TestLinkUnknownResource(t *testing.T) {
	d := parse(t, readFixture(t, "embedded.json"))
	if _, err := d.Link("animal-ok.svg", devURL); !errors.Is(err, ErrResourceNotFound) {
		t.Fatalf("images must not be linkable, got %v", err)
	}
}

func TestReferences(t *testing.T) {
	d := parse(t, readFixture(t, "embedded.json"))
	refs, err := d.References()
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, r := range refs {
		if r.Value == ResourcePrefix+bundleLink {
			found = r.Count == bundleReferences
		}
	}
	if !found {
		t.Fatalf("bundle reference with %d uses not listed: %+v", bundleReferences, refs)
	}
}

func TestEmbedCreatesResourcesArray(t *testing.T) {
	tests := []struct {
		name string
		doc  string
	}{
		{"pretty without resources", "{\n  \"configuration\": {\n    \"url\": \"" + devURL + "\"\n  }\n}\n"},
		{"pretty with empty resources", "{\n  \"configuration\": {\n    \"url\": \"" + devURL + "\"\n  },\n  \"resources\": []\n}\n"},
		{"compact without resources", "{\"configuration\":{\"url\":\"" + devURL + "\"}}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := parse(t, []byte(tt.doc))
			if _, err := d.Embed(EmbedOptions{Data: []byte("js"), FileName: bundleName, ReplaceURL: devURL}); err != nil {
				t.Fatal(err)
			}
			if !json.Valid(d.Bytes()) {
				t.Fatalf("produced invalid JSON:\n%s", d.Bytes())
			}
			resources, err := d.Resources()
			if err != nil {
				t.Fatal(err)
			}
			if len(resources) != 1 || resources[0].Link != bundleLink {
				t.Fatalf("unexpected resources %+v", resources)
			}

			replaced, err := d.Link(bundleName, devURL)
			if err != nil {
				t.Fatal(err)
			}
			if replaced != 1 || !json.Valid(d.Bytes()) {
				t.Fatalf("link round trip failed (%d replaced):\n%s", replaced, d.Bytes())
			}
		})
	}
}

func TestParseRejectsNonDashboard(t *testing.T) {
	if _, err := Parse([]byte(`{"title": "x"}`)); !errors.Is(err, ErrNotDashboard) {
		t.Fatalf("got %v, want %v", err, ErrNotDashboard)
	}
	if _, err := Parse([]byte(`{"configuration": `)); err == nil {
		t.Fatal("expected an error for invalid JSON")
	}
}
