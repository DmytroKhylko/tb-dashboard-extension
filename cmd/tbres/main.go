// Command tbres embeds a JS module into an exported ThingsBoard dashboard, or
// swaps an embedded module back to an external URL such as a local dev server.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/thingsboard/tb-dashboard-extension/internal/dashboard"
)

const usage = `Usage:
  tbres list  <dashboard.json>
  tbres embed [-out FILE | -in-place] -file BUNDLE.js [-replace-url URL] [-key KEY] [-title TITLE] [-subtype SUBTYPE] <dashboard.json>
  tbres link  [-out FILE | -in-place] -resource KEY|LINK -url URL <dashboard.json>

Commands:
  list   show embedded resources and resource references in the configuration
  embed  embed BUNDLE.js as a JS module and point references to -replace-url at it
  link   remove an embedded JS module and point its references at URL

Without -out or -in-place the resulting dashboard is written to stdout.
`

var errUsage = errors.New("invalid usage")

func main() {
	err := run(os.Args[1:], os.Stdout, os.Stderr)
	switch {
	case err == nil, errors.Is(err, flag.ErrHelp):
	case errors.Is(err, errUsage):
		os.Exit(2)
	default:
		fmt.Fprintln(os.Stderr, "tbres:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return errUsage
	}
	switch args[0] {
	case "list":
		return runList(args[1:], stdout, stderr)
	case "embed":
		return runEmbed(args[1:], stdout, stderr)
	case "link":
		return runLink(args[1:], stdout, stderr)
	case "help", "-h", "-help", "--help":
		fmt.Fprint(stdout, usage)
		return nil
	default:
		fmt.Fprintf(stderr, "unknown command %q\n\n%s", args[0], usage)
		return errUsage
	}
}

type output struct {
	path    string
	inPlace bool
}

func (o *output) register(fs *flag.FlagSet) {
	fs.StringVar(&o.path, "out", "", "write the result to `FILE`")
	fs.BoolVar(&o.inPlace, "in-place", false, "overwrite the input dashboard")
}

func (o *output) validate(fs *flag.FlagSet) error {
	if o.inPlace && o.path != "" {
		return usageError(fs, "-out and -in-place are mutually exclusive")
	}
	return nil
}

func (o *output) write(input string, data []byte, stdout io.Writer) error {
	switch {
	case o.inPlace:
		return writeFileAtomic(input, data)
	case o.path != "":
		return writeFileAtomic(o.path, data)
	default:
		_, err := stdout.Write(data)
		return err
	}
}

func runList(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("list", stderr)
	path, err := parse(fs, args)
	if err != nil {
		return err
	}
	d, err := load(path)
	if err != nil {
		return err
	}
	resources, err := d.Resources()
	if err != nil {
		return err
	}
	refs, err := d.References()
	if err != nil {
		return err
	}

	w := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "EMBEDDED RESOURCES")
	fmt.Fprintln(w, "LINK\tTYPE\tSUBTYPE\tKEY\tSIZE")
	for _, r := range resources {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\n", r.Link, r.Type, r.SubType, r.ResourceKey, r.Size())
	}
	fmt.Fprintln(w, "\nREFERENCES IN CONFIGURATION")
	fmt.Fprintln(w, "VALUE\tCOUNT")
	for _, r := range refs {
		fmt.Fprintf(w, "%s\t%d\n", r.Value, r.Count)
	}
	return w.Flush()
}

func runEmbed(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("embed", stderr)
	var out output
	out.register(fs)
	file := fs.String("file", "", "JS bundle to embed (required)")
	replaceURL := fs.String("replace-url", "", "external `URL` whose references are replaced by the embedded resource")
	key := fs.String("key", "", "resource key (default: bundle file name)")
	title := fs.String("title", "", "resource title shown in ThingsBoard (default: resource key)")
	subType := fs.String("subtype", dashboard.SubTypeExtension, "resource subtype: EXTENSION or MODULE")

	path, err := parse(fs, args)
	if err != nil {
		return err
	}
	if err := out.validate(fs); err != nil {
		return err
	}
	if *file == "" {
		return usageError(fs, "-file is required")
	}
	if *subType != dashboard.SubTypeExtension && *subType != "MODULE" {
		return usageError(fs, "-subtype must be EXTENSION or MODULE")
	}
	d, err := load(path)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(*file)
	if err != nil {
		return err
	}

	result, err := d.Embed(dashboard.EmbedOptions{
		Data:       data,
		FileName:   filepath.Base(*file),
		Key:        *key,
		Title:      *title,
		SubType:    *subType,
		ReplaceURL: *replaceURL,
	})
	if err != nil {
		return err
	}
	action := "added"
	if result.Updated {
		action = "updated"
	}
	fmt.Fprintf(stderr, "%s %s (%d bytes), replaced %d reference(s)\n", action, result.Link, len(data), result.Replaced)
	return out.write(path, d.Bytes(), stdout)
}

func runLink(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("link", stderr)
	var out output
	out.register(fs)
	resource := fs.String("resource", "", "embedded resource key or link (required)")
	url := fs.String("url", "", "external `URL` to reference instead (required)")

	path, err := parse(fs, args)
	if err != nil {
		return err
	}
	if err := out.validate(fs); err != nil {
		return err
	}
	if *resource == "" || *url == "" {
		return usageError(fs, "-resource and -url are required")
	}
	d, err := load(path)
	if err != nil {
		return err
	}

	replaced, err := d.Link(*resource, *url)
	if err != nil {
		return err
	}
	if replaced == 0 {
		fmt.Fprintf(stderr, "warning: %s was embedded but not referenced; removed it\n", *resource)
	} else {
		fmt.Fprintf(stderr, "removed %s, replaced %d reference(s) with %s\n", *resource, replaced, *url)
	}
	return out.write(path, d.Bytes(), stdout)
}

func newFlagSet(name string, stderr io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprint(stderr, usage, "\nFlags:\n")
		fs.PrintDefaults()
	}
	return fs
}

// parse accepts flags before or after the single positional dashboard path.
func parse(fs *flag.FlagSet, args []string) (string, error) {
	if err := parseFlags(fs, args); err != nil {
		return "", err
	}
	var positional []string
	for fs.NArg() > 0 {
		positional = append(positional, fs.Arg(0))
		if err := parseFlags(fs, fs.Args()[1:]); err != nil {
			return "", err
		}
	}
	if len(positional) != 1 {
		return "", usageError(fs, "expected exactly one dashboard file")
	}
	return positional[0], nil
}

func parseFlags(fs *flag.FlagSet, args []string) error {
	err := fs.Parse(args)
	if err == nil || errors.Is(err, flag.ErrHelp) {
		return err
	}
	return errUsage
}

func usageError(fs *flag.FlagSet, msg string) error {
	fmt.Fprintf(fs.Output(), "%s\n\n", msg)
	fs.Usage()
	return errUsage
}

func load(path string) (*dashboard.Dashboard, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	d, err := dashboard.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return d, nil
}

func writeFileAtomic(path string, data []byte) error {
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tbres-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
