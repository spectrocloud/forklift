package hook

import (
	"regexp"
	"testing"
)

func TestRegexpHelpers_LiteralExpressionAndAnchoring(t *testing.T) {
	// literal() should create a regexp that is fully literal.
	re := literal(".+*?^$")
	if re.String() != regexp.QuoteMeta(".+*?^$") {
		t.Fatalf("unexpected literal regexp: %q", re.String())
	}

	// expression() concatenates patterns.
	e := expression(literal("a"), match("b+"), literal("c"))
	if e.String() != "a"+"b+"+"c" {
		t.Fatalf("unexpected expression: %q", e.String())
	}

	// group/capture/optional/repeated/anchored wrappers.
	g := group(match("a"))
	if g.String() != "(?:a)" {
		t.Fatalf("unexpected group: %q", g.String())
	}
	c := capture(match("a"), match("b"))
	if c.String() != "(ab)" {
		t.Fatalf("unexpected capture: %q", c.String())
	}
	o := optional(match("a"))
	if o.String() != "(?:a)?" {
		t.Fatalf("unexpected optional: %q", o.String())
	}
	r := repeated(match("a"))
	if r.String() != "(?:a)+" {
		t.Fatalf("unexpected repeated: %q", r.String())
	}
	a := anchored(match("a"), match("b"))
	if a.String() != "^ab$" {
		t.Fatalf("unexpected anchored: %q", a.String())
	}
}

func TestWellKnownRegexps_MatchSamples(t *testing.T) {
	// These are used by the hook controller to validate image references.
	if !anchoredTagRegexp.MatchString("v1.2.3") {
		t.Fatalf("expected tag to match")
	}
	if anchoredTagRegexp.MatchString("") {
		t.Fatalf("expected empty tag to not match")
	}
	if !anchoredDigestRegexp.MatchString("sha256:0123456789abcdef0123456789abcdef") {
		t.Fatalf("expected digest to match")
	}
	if !anchoredIdentifierRegexp.MatchString("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef") {
		t.Fatalf("expected identifier to match")
	}
	if !anchoredShortIdentifierRegexp.MatchString("012345") {
		t.Fatalf("expected short identifier to match")
	}

	// Full reference: name[:tag][@digest]
	ok := "example.com/repo/name:tag@sha256:0123456789abcdef0123456789abcdef"
	if !ReferenceRegexp.MatchString(ok) {
		t.Fatalf("expected reference to match")
	}
}
