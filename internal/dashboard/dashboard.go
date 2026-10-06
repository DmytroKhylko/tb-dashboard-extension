// Package dashboard edits the resources of an exported ThingsBoard dashboard JSON
// while keeping the rest of the document byte-for-byte intact.
package dashboard

import (
	"bytes"
	"cmp"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/thingsboard/tb-dashboard-extension/internal/jsonscan"
)

const (
	ResourcePrefix = "tb-resource;"

	TypeJSModule     = "JS_MODULE"
	TypeImage        = "IMAGE"
	SubTypeExtension = "EXTENSION"

	jsModuleLinkPrefix = "/api/resource/js_module/tenant/"
	jsMediaType        = "application/javascript"
)

var (
	ErrNotDashboard     = errors.New("document is not a dashboard: missing \"configuration\" object")
	ErrURLNotFound      = errors.New("url is not referenced in the dashboard configuration")
	ErrResourceNotFound = errors.New("resource is not embedded in the dashboard")
	ErrNothingToEmbed   = errors.New("resource is neither referenced nor embedded; pass a url to replace")
)

// Resource mirrors ThingsBoard's ResourceExportData; field order matches TB's export.
type Resource struct {
	Link              string `json:"link"`
	Title             string `json:"title,omitempty"`
	Type              string `json:"type"`
	SubType           string `json:"subType,omitempty"`
	ResourceKey       string `json:"resourceKey,omitempty"`
	FileName          string `json:"fileName,omitempty"`
	PublicResourceKey string `json:"publicResourceKey,omitempty"`
	MediaType         string `json:"mediaType,omitempty"`
	Data              string `json:"data,omitempty"`
	Public            bool   `json:"public"`
}

// Size returns the decoded size of the embedded data in bytes.
func (r Resource) Size() int {
	data, err := base64.StdEncoding.DecodeString(r.Data)
	if err != nil {
		return 0
	}
	return len(data)
}

// Reference is a resource-like string found in the dashboard configuration.
type Reference struct {
	Value string
	Count int
}

type Dashboard struct {
	doc []byte
}

func Parse(doc []byte) (*Dashboard, error) {
	if !json.Valid(doc) {
		return nil, jsonscan.ErrInvalid
	}
	d := &Dashboard{doc: bytes.Clone(doc)}
	if _, err := d.configuration(); err != nil {
		return nil, err
	}
	return d, nil
}

func (d *Dashboard) Bytes() []byte {
	return bytes.Clone(d.doc)
}

func (d *Dashboard) Resources() ([]Resource, error) {
	member, found, err := d.member("resources")
	if err != nil || !found {
		return nil, err
	}
	var resources []Resource
	if err := json.Unmarshal(d.doc[member.Value.Start:member.Value.End], &resources); err != nil {
		return nil, fmt.Errorf("decode resources: %w", err)
	}
	return resources, nil
}

// References lists tb-resource links and http(s) URLs used in the configuration.
func (d *Dashboard) References() ([]Reference, error) {
	values, err := d.configurationStrings()
	if err != nil {
		return nil, err
	}
	var refs []Reference
	index := map[string]int{}
	for _, v := range values {
		if !isResourceLike(v.Value) {
			continue
		}
		if i, ok := index[v.Value]; ok {
			refs[i].Count++
			continue
		}
		index[v.Value] = len(refs)
		refs = append(refs, Reference{Value: v.Value, Count: 1})
	}
	return refs, nil
}

type EmbedOptions struct {
	Data       []byte
	FileName   string
	Key        string
	Title      string
	SubType    string
	ReplaceURL string
}

type EmbedResult struct {
	Link     string
	Replaced int
	Updated  bool
}

// Embed adds or updates a JS module resource and points matching references at it.
func (d *Dashboard) Embed(opts EmbedOptions) (EmbedResult, error) {
	key := opts.Key
	if key == "" {
		key = opts.FileName
	}
	if key == "" || strings.ContainsAny(key, "/\\") {
		return EmbedResult{}, fmt.Errorf("invalid resource key %q", key)
	}
	resource := Resource{
		Link:        jsModuleLinkPrefix + key,
		Title:       cmp.Or(opts.Title, key),
		Type:        TypeJSModule,
		SubType:     cmp.Or(opts.SubType, SubTypeExtension),
		ResourceKey: key,
		FileName:    cmp.Or(opts.FileName, key),
		MediaType:   jsMediaType,
		Data:        base64.StdEncoding.EncodeToString(opts.Data),
	}
	result := EmbedResult{Link: resource.Link}
	reference := ResourcePrefix + resource.Link

	if opts.ReplaceURL != "" {
		replaced, err := d.replaceConfigurationStrings(opts.ReplaceURL, reference)
		if err != nil {
			return EmbedResult{}, err
		}
		if replaced == 0 {
			return EmbedResult{}, fmt.Errorf("%w: %s", ErrURLNotFound, opts.ReplaceURL)
		}
		result.Replaced = replaced
	}

	updated, err := d.upsertResource(resource)
	if err != nil {
		return EmbedResult{}, err
	}
	result.Updated = updated

	if opts.ReplaceURL == "" && !updated {
		count, err := d.countConfigurationStrings(reference)
		if err != nil {
			return EmbedResult{}, err
		}
		if count == 0 {
			return EmbedResult{}, ErrNothingToEmbed
		}
	}
	return result, nil
}

// Link removes an embedded resource, found by link or resource key, and points its references at url.
func (d *Dashboard) Link(resource, url string) (int, error) {
	resources, err := d.Resources()
	if err != nil {
		return 0, err
	}
	index := -1
	for i, r := range resources {
		if r.Type != TypeImage && (r.Link == resource || r.ResourceKey == resource) {
			index = i
			break
		}
	}
	if index < 0 {
		return 0, fmt.Errorf("%w: %s", ErrResourceNotFound, resource)
	}
	replaced, err := d.replaceConfigurationStrings(ResourcePrefix+resources[index].Link, url)
	if err != nil {
		return 0, err
	}
	if err := d.removeResource(index); err != nil {
		return 0, err
	}
	return replaced, nil
}

func (d *Dashboard) configuration() (jsonscan.Span, error) {
	member, found, err := d.member("configuration")
	if err != nil {
		return jsonscan.Span{}, err
	}
	if !found || d.doc[member.Value.Start] != '{' {
		return jsonscan.Span{}, ErrNotDashboard
	}
	return member.Value, nil
}

func (d *Dashboard) configurationStrings() ([]jsonscan.StringValue, error) {
	span, err := d.configuration()
	if err != nil {
		return nil, err
	}
	values, err := jsonscan.StringValues(d.doc[span.Start:span.End])
	if err != nil {
		return nil, err
	}
	for i := range values {
		values[i].Span.Start += span.Start
		values[i].Span.End += span.Start
	}
	return values, nil
}

func (d *Dashboard) countConfigurationStrings(value string) (int, error) {
	values, err := d.configurationStrings()
	if err != nil {
		return 0, err
	}
	count := 0
	for _, v := range values {
		if v.Value == value {
			count++
		}
	}
	return count, nil
}

func (d *Dashboard) replaceConfigurationStrings(from, to string) (int, error) {
	values, err := d.configurationStrings()
	if err != nil {
		return 0, err
	}
	literal, err := marshal(to, "", "")
	if err != nil {
		return 0, err
	}
	var out bytes.Buffer
	last, count := 0, 0
	for _, v := range values {
		if v.Value != from {
			continue
		}
		out.Write(d.doc[last:v.Span.Start])
		out.Write(literal)
		last = v.Span.End
		count++
	}
	if count > 0 {
		out.Write(d.doc[last:])
		d.doc = out.Bytes()
	}
	return count, nil
}

func (d *Dashboard) upsertResource(resource Resource) (bool, error) {
	members, err := jsonscan.ObjectMembers(d.doc, 0)
	if err != nil {
		return false, err
	}
	unit := indentUnit(d.doc, members)

	member, found := findMember(members, "resources")
	if !found {
		keyIndent := unit
		array, err := arrayLiteral(resource, keyIndent, unit)
		if err != nil {
			return false, err
		}
		key, _ := marshal("resources", "", "")
		insert := ",\n" + string(keyIndent) + string(key) + ": " + string(array)
		if len(unit) == 0 {
			insert = "," + string(key) + ":" + string(array)
		}
		d.splice(members[len(members)-1].Value.End, members[len(members)-1].Value.End, []byte(insert))
		return false, nil
	}

	elements, err := jsonscan.ArrayElements(d.doc, member.Value.Start)
	if err != nil {
		return false, err
	}
	if len(elements) == 0 {
		array, err := arrayLiteral(resource, jsonscan.LineIndent(d.doc, member.Value.Start), unit)
		if err != nil {
			return false, err
		}
		d.splice(member.Value.Start, member.Value.End, array)
		return false, nil
	}

	for _, element := range elements {
		var existing Resource
		if err := json.Unmarshal(d.doc[element.Start:element.End], &existing); err != nil {
			return false, fmt.Errorf("decode resource: %w", err)
		}
		if existing.Link != resource.Link {
			continue
		}
		entry, err := marshal(resource, string(jsonscan.LineIndent(d.doc, element.Start)), string(unit))
		if err != nil {
			return false, err
		}
		d.splice(element.Start, element.End, entry)
		return true, nil
	}

	first := elements[0]
	entry, err := marshal(resource, string(jsonscan.LineIndent(d.doc, first.Start)), string(unit))
	if err != nil {
		return false, err
	}
	separator := d.doc[member.Value.Start+1 : first.Start]
	insert := append(append(entry, ','), separator...)
	d.splice(first.Start, first.Start, insert)
	return false, nil
}

func (d *Dashboard) removeResource(index int) error {
	member, found, err := d.member("resources")
	if err != nil {
		return err
	}
	if !found {
		return ErrResourceNotFound
	}
	elements, err := jsonscan.ArrayElements(d.doc, member.Value.Start)
	if err != nil {
		return err
	}
	switch {
	case len(elements) == 1:
		d.splice(member.Value.Start, member.Value.End, []byte("[]"))
	case index < len(elements)-1:
		d.splice(elements[index].Start, elements[index+1].Start, nil)
	default:
		d.splice(elements[index-1].End, elements[index].End, nil)
	}
	return nil
}

func (d *Dashboard) member(key string) (jsonscan.Member, bool, error) {
	members, err := jsonscan.ObjectMembers(d.doc, 0)
	if err != nil {
		return jsonscan.Member{}, false, err
	}
	member, found := findMember(members, key)
	return member, found, nil
}

func (d *Dashboard) splice(start, end int, insert []byte) {
	out := make([]byte, 0, len(d.doc)-(end-start)+len(insert))
	out = append(out, d.doc[:start]...)
	out = append(out, insert...)
	out = append(out, d.doc[end:]...)
	d.doc = out
}

func findMember(members []jsonscan.Member, key string) (jsonscan.Member, bool) {
	for _, m := range members {
		if m.Key == key {
			return m, true
		}
	}
	return jsonscan.Member{}, false
}

// indentUnit infers the indentation step from the first top-level member; empty means compact JSON.
func indentUnit(doc []byte, members []jsonscan.Member) []byte {
	if len(members) == 0 {
		return []byte("  ")
	}
	start := members[0].Value.Start
	if bytes.LastIndexByte(doc[:start], '\n') < 0 {
		return nil
	}
	return jsonscan.LineIndent(doc, start)
}

func arrayLiteral(resource Resource, keyIndent, unit []byte) ([]byte, error) {
	if len(unit) == 0 {
		entry, err := marshal(resource, "", "")
		if err != nil {
			return nil, err
		}
		return append(append([]byte("["), entry...), ']'), nil
	}
	elementIndent := string(keyIndent) + string(unit)
	entry, err := marshal(resource, elementIndent, string(unit))
	if err != nil {
		return nil, err
	}
	return []byte("[\n" + elementIndent + string(entry) + "\n" + string(keyIndent) + "]"), nil
}

func marshal(v any, prefix, indent string) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent(prefix, indent)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

func isResourceLike(s string) bool {
	return strings.HasPrefix(s, ResourcePrefix) || strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}
