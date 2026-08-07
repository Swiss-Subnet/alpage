package nns

// Version is the released version of this tree, and the single source of truth
// for it: the flake stamps it into main.version, and the release workflow
// refuses a tag that disagrees with it. Bump it in the release-prep commit.
const Version = "v0.4.0"
