package main

import (
	"regexp"
	"strings"
	"testing"

	"github.com/swiss-subnet/alpage/nns"
)

// Placeholders like <name> are HTML tags to a markdown renderer, which drops
// them: "subnet.<name>.id" rendered as "subnet..id" on the site and on GitHub.
func TestCodePlaceholders(t *testing.T) {
	cases := []struct{ in, want string }{
		{"referenced as subnet.<name>.id.", "referenced as `subnet.<name>.id`."},
		{"the same <kind>.<name>.id form", "the same `<kind>.<name>.id` form"},
		{"a replica_version_<id> record", "a `replica_version_<id>` record"},
		{"Id of its dc (data_center.<name>.id).", "Id of its dc (`data_center.<name>.id`)."},
		{"no placeholders here", "no placeholders here"},
		{"--force submits anyway", "`--force` submits anyway"},
		{"the --host flag.", "the `--host` flag."},
		{"a well--formed dash", "a well--formed dash"},
	}
	for _, c := range cases {
		if got := codePlaceholders(c.in); got != c.want {
			t.Errorf("codePlaceholders(%q)\n got %q\nwant %q", c.in, got, c.want)
		}
	}
}

func TestBlockRefs(t *testing.T) {
	cases := []struct{ in, want string }{
		{"See the add / remove block.", "See the [add / remove](#add-remove-) block."},
		{"Nested in a membership block.", "Nested in a [membership](#membership-) block."},
		{"no block mention here", "no block mention here"},
	}
	for _, c := range cases {
		if got := linkBlockRefs(c.in); got != c.want {
			t.Errorf("linkBlockRefs(%q)\n got %q\nwant %q", c.in, got, c.want)
		}
	}
}

// A block's own intro must not link to itself.
func TestBlockRefsSkipsSelf(t *testing.T) {
	in := "Inside a membership block: a node to add or remove."
	if got := linkBlockRefs(in, "membership"); got != in {
		t.Errorf("self-link not skipped:\n got %q\nwant %q", got, in)
	}
}

// Every in-page link must target a heading the page actually emits; the site
// slugs headings independently, so a divergence would ship a dead anchor.
func TestBlockRefAnchorsResolve(t *testing.T) {
	md, err := render()
	if err != nil {
		t.Fatal(err)
	}
	headings := map[string]bool{"schema-history": true}
	for _, blk := range nns.SchemaBlocks {
		headings[headingSlug(blk.Name)] = true
	}
	for _, m := range regexp.MustCompile(`\(#([a-z0-9_-]+)\)`).FindAllStringSubmatch(md, -1) {
		if !headings[m[1]] {
			t.Errorf("link to #%s has no matching heading", m[1])
		}
	}
}

// Every angle-bracket placeholder in the rendered page must sit inside a code
// span, or the renderer eats it.
func TestRenderEscapesPlaceholders(t *testing.T) {
	md, err := render()
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(md, "\n") {
		if !strings.Contains(line, "<") {
			continue
		}
		for _, ph := range []string{"<name>", "<kind>", "<id>"} {
			idx := strings.Index(line, ph)
			if idx < 0 {
				continue
			}
			if strings.Count(line[:idx], "`")%2 == 0 {
				t.Errorf("bare %s outside a code span:\n%s", ph, line)
			}
		}
	}
}
