package nns

import (
	"bufio"
	"cmp"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var semverTag = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)

func TestVersionIsSemverTag(t *testing.T) {
	if !semverTag.MatchString(Version) {
		t.Errorf("Version = %q, want a vX.Y.Z tag", Version)
	}
}

// The CHANGELOG's newest section is what the release notes are cut from, so a
// Version that does not match it would ship notes for the wrong release.
func TestVersionMatchesChangelog(t *testing.T) {
	f, err := os.Open(filepath.Join("..", "CHANGELOG.md"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	heading := regexp.MustCompile(`^## \[(\d+\.\d+\.\d+)\]`)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if m := heading.FindStringSubmatch(sc.Text()); m != nil {
			if got := "v" + m[1]; got != Version {
				t.Errorf("newest CHANGELOG section is %s, want %s (bump both together)", got, Version)
			}
			return
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	t.Fatal("CHANGELOG.md has no ## [X.Y.Z] section")
}

// SchemaUnreleased names the version being developed, so it is either empty (the
// tree is at a release) or strictly ahead of it -- never equal, which would make
// the docs link a tag while calling it unreleased.
func TestSchemaUnreleasedIsNotVersion(t *testing.T) {
	if SchemaUnreleased == "" {
		return
	}
	if SchemaUnreleased == Version {
		t.Errorf("SchemaUnreleased = %q equals Version; clear it when the release ships", SchemaUnreleased)
	}
	if !semverTag.MatchString(SchemaUnreleased) {
		t.Errorf("SchemaUnreleased = %q, want empty or a vX.Y.Z tag", SchemaUnreleased)
	}
}

// No Since value may name a version later than this tree claims to be: a field
// marked v0.4.0 in a v0.3.0 tree would tell readers to get a binary that the
// schema history cannot link.
func TestSchemaSinceNotAheadOfVersion(t *testing.T) {
	for key, since := range SchemaFieldSince {
		if since == SchemaUnreleased {
			continue
		}
		if compareTags(since, Version) > 0 {
			t.Errorf("SchemaFieldSince[%q] = %s is ahead of Version %s", key, since, Version)
		}
	}
	for _, blk := range SchemaBlocks {
		if blk.Since == SchemaUnreleased {
			continue
		}
		if compareTags(blk.Since, Version) > 0 {
			t.Errorf("SchemaBlocks %q Since = %s is ahead of Version %s", blk.Name, blk.Since, Version)
		}
	}
}

// compareTags orders two vX.Y.Z tags numerically per component.
func compareTags(a, b string) int {
	as := strings.Split(strings.TrimPrefix(a, "v"), ".")
	bs := strings.Split(strings.TrimPrefix(b, "v"), ".")
	for i := 0; i < len(as) && i < len(bs); i++ {
		x, _ := strconv.Atoi(as[i])
		y, _ := strconv.Atoi(bs[i])
		if x != y {
			return cmp.Compare(x, y)
		}
	}
	return 0
}
