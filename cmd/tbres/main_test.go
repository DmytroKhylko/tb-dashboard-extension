package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const devURL = "http://localhost:5000/static/widgets/thingsboard-extension-widgets.js"

func fixture(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join("..", "..", "internal", "dashboard", "testdata", name)
}

func copyFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(fixture(t, name))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestEmbedThenLinkRoundTrip(t *testing.T) {
	linked := copyFixture(t, "linked.json")
	embedded := filepath.Join(t.TempDir(), "embedded.json")
	var stdout, stderr bytes.Buffer

	err := run([]string{"embed", linked, "-file", fixture(t, "thingsboard-extension-widgets.js"),
		"-replace-url", devURL, "-title", "FE UI Extension", "-out", embedded}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("embed: %v\n%s", err, stderr.String())
	}
	if !strings.Contains(stderr.String(), "replaced 5 reference(s)") {
		t.Fatalf("unexpected embed report: %s", stderr.String())
	}

	if err := run([]string{"link", "-in-place", "-resource", "thingsboard-extension-widgets.js", "-url", devURL, embedded}, &stdout, &stderr); err != nil {
		t.Fatalf("link: %v\n%s", err, stderr.String())
	}

	got, _ := os.ReadFile(embedded)
	want, _ := os.ReadFile(linked)
	if !bytes.Equal(got, want) {
		t.Fatal("embed followed by link did not restore the original dashboard")
	}
	if stdout.Len() != 0 {
		t.Fatalf("nothing should be written to stdout when -out/-in-place is used, got %d bytes", stdout.Len())
	}
}

func TestListShowsResourcesAndReferences(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run([]string{"list", fixture(t, "embedded.json")}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"/api/resource/js_module/tenant/thingsboard-extension-widgets.js",
		"tb-resource;/api/resource/js_module/tenant/thingsboard-extension-widgets.js  5",
		"/api/images/tenant/animal-ok.svg",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("list output is missing %q:\n%s", want, stdout.String())
		}
	}
}

func TestUsageErrors(t *testing.T) {
	tests := [][]string{
		nil,
		{"unknown"},
		{"embed", fixture(t, "linked.json")},
		{"link", "-resource", "x", fixture(t, "embedded.json")},
		{"list"},
		{"list", "-bogus", fixture(t, "embedded.json")},
		{"embed", "-file", "x.js", "-out", "a.json", "-in-place", fixture(t, "linked.json")},
	}
	for _, args := range tests {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if err := run(args, &stdout, &stderr); !errors.Is(err, errUsage) {
				t.Fatalf("got %v, want usage error", err)
			}
		})
	}
}
