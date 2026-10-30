package tml

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func decode(t *testing.T, s string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

// TestPatchRFC6902 runs examples from RFC 6902 appendix A.
func TestPatchRFC6902(t *testing.T) {
	cases := []struct{ doc, patch, want string }{
		{`{"foo":"bar"}`, `[{"op":"add","path":"/baz","value":"qux"}]`, `{"baz":"qux","foo":"bar"}`},
		{`{"foo":["bar","baz"]}`, `[{"op":"add","path":"/foo/1","value":"qux"}]`, `{"foo":["bar","qux","baz"]}`},
		{`{"baz":"qux","foo":"bar"}`, `[{"op":"remove","path":"/baz"}]`, `{"foo":"bar"}`},
		{`{"foo":["bar","qux","baz"]}`, `[{"op":"remove","path":"/foo/1"}]`, `{"foo":["bar","baz"]}`},
		{`{"baz":"qux","foo":"bar"}`, `[{"op":"replace","path":"/baz","value":"boo"}]`, `{"baz":"boo","foo":"bar"}`},
		{`{"foo":{"bar":"baz","waldo":"fred"},"qux":{"corge":"grault"}}`, `[{"op":"move","from":"/foo/waldo","path":"/qux/thud"}]`,
			`{"foo":{"bar":"baz"},"qux":{"corge":"grault","thud":"fred"}}`},
		{`{"foo":["all","grass","cows","eat"]}`, `[{"op":"move","from":"/foo/1","path":"/foo/3"}]`, `{"foo":["all","cows","eat","grass"]}`},
		{`{"baz":"qux","foo":["a",2,"c"]}`, `[{"op":"test","path":"/baz","value":"qux"},{"op":"test","path":"/foo/1","value":2}]`, `{"baz":"qux","foo":["a",2,"c"]}`},
		{`{"foo":"bar"}`, `[{"op":"add","path":"/child","value":{"grandchild":{}}}]`, `{"foo":"bar","child":{"grandchild":{}}}`},
		{`{"foo":["bar"]}`, `[{"op":"add","path":"/foo/-","value":["abc","def"]}]`, `{"foo":["bar",["abc","def"]]}`},
		{`{"/":9,"~1":10}`, `[{"op":"test","path":"/~01","value":10}]`, `{"/":9,"~1":10}`},
		{`{"a":{"b":1}}`, `[{"op":"copy","from":"/a","path":"/c"}]`, `{"a":{"b":1},"c":{"b":1}}`},
	}
	for _, c := range cases {
		ops, err := DecodePatch([]byte(c.patch), nil)
		if err != nil {
			t.Fatalf("%s: %v", c.patch, err)
		}
		got, _, err := applyPatch(decode(t, c.doc), 0, ops, DefaultLimits())
		if err != nil {
			t.Errorf("%s: %v", c.patch, err)
			continue
		}
		if !reflect.DeepEqual(got, decode(t, c.want)) {
			t.Errorf("%s: got %v", c.patch, got)
		}
	}
	for _, c := range []struct{ doc, patch string }{
		{`{"foo":"bar"}`, `[{"op":"add","path":"/baz/bat","value":"qux"}]`},
		{`{"baz":"qux"}`, `[{"op":"test","path":"/baz","value":"bar"}]`},
		{`{"foo":[1]}`, `[{"op":"add","path":"/foo/5","value":2}]`},
		{`{"foo":[1]}`, `[{"op":"remove","path":"/foo/01"}]`},
		{`{"a":{"b":1}}`, `[{"op":"move","from":"/a","path":"/a/b/c"}]`},
		{`{}`, `[{"op":"replace","path":"/x","value":1}]`},
	} {
		ops, err := DecodePatch([]byte(c.patch), nil)
		if err != nil {
			continue
		}
		doc := decode(t, c.doc)
		if _, _, err := applyPatch(doc, 0, ops, DefaultLimits()); err == nil {
			t.Errorf("%s applied; it should not", c.patch)
		}
		if !reflect.DeepEqual(doc, decode(t, c.doc)) {
			t.Errorf("%s changed the document on failure", c.patch)
		}
	}
}

// TestStore checks atomic patches, the size limit and the changed paths.
func TestStore(t *testing.T) {
	lim := DefaultLimits()
	lim.StateBytes = 64
	s, err := NewStore(map[string]any{"n": 1.0, "items": []any{}}, &lim)
	if err != nil {
		t.Fatal(err)
	}
	ops, _ := DecodePatch([]byte(`[{"op":"replace","path":"/n","value":2},{"op":"test","path":"/n","value":3}]`), &lim)
	if _, err := s.Apply(ops); err == nil {
		t.Fatal("a failed test applied")
	}
	if s.Get("/n") != 1.0 || s.Version() != 0 {
		t.Fatalf("a failed patch changed the store: %v", s.State())
	}
	ops, _ = DecodePatch([]byte(`[{"op":"add","path":"/items/-","value":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}]`), &lim)
	if _, err := s.Apply(ops); err == nil {
		t.Fatal("a patch past the state limit applied")
	}
	changed, err := s.Set("state.form.title", "x")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(changed, []string{"/form", "/form/title"}) || s.Get("/form/title") != "x" {
		t.Errorf("changed %v, state %v", changed, s.State())
	}
	if _, err := DecodePatch(make([]byte, lim.PatchBytes+1), &lim); err == nil {
		t.Error("a patch past the size limit decoded")
	}
}

func TestBoundContent(t *testing.T) {
	doc, problems, err := Check([]byte(`<tml version="1" state='{"hunk":"-a\n+b"}'><diff content="{state.hunk}"/></tml>`), nil)
	if err != nil || len(problems) > 0 {
		t.Fatalf("a bound diff does not validate: %v %v", err, problems)
	}
	s, err := StoreFor(doc, nil)
	if err != nil {
		t.Fatal(err)
	}
	ops, _ := DecodePatch([]byte(`[{"op":"replace","path":"/hunk","value":"x"}]`), nil)
	if _, err := s.Apply(ops); err != nil {
		t.Fatalf("a small bound hunk did not apply: %v", err)
	}

	big := DefaultLimits().DocBytes + 1
	doc, problems, err = Check([]byte(`<tml version="1" state='{"hunk":"x"}'><diff content="{state.hunk}"/></tml>`), nil)
	if err != nil {
		t.Fatal(err)
	}
	// The expression machinery caps every bound string at maxString, so the
	// rendered content of one node never passes 64 KiB however big the state
	// value is.
	tpl, err := ParseTemplate("{state.hunk}")
	if err != nil {
		t.Fatal(err)
	}
	raw := tpl.Render(&Env{Vars: map[string]any{"state": map[string]any{"hunk": strings.Repeat("x", big)}}})
	if len(raw) != maxString {
		t.Errorf("a bound diff content rendered %d bytes, the cap is %d", len(raw), maxString)
	}

	_, problems, err = Check([]byte(`<tml version="1" state='{"hunk":"x"}'><diff content="{state.hunk}">literal</diff></tml>`), nil)
	if err == nil && len(problems) == 0 {
		t.Error("content and children together did not fail")
	}

	// A literal document without the attribute is unchanged.
	_, problems, err = Check([]byte("<tml version=\"1\"><diff lang=\"go\">--- a\n+++ b</diff></tml>"), nil)
	if err != nil || len(problems) > 0 {
		t.Errorf("a literal diff does not validate: %v %v", err, problems)
	}
}
