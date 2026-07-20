package build_shared

import (
	"strings"

	"github.com/sagernet/sing-box/common/badversion"
	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/shell"
)

// normalizeTag keeps artifacts from this fork identifiable even when the
// integration branch inherits reF1nd tags.
func normalizeTag(tag string) string {
	return strings.ReplaceAll(tag, "reF1nd", "xiaobaf14g")
}

func ReadTag() (string, error) {
	currentTag, err := shell.Exec("git", "describe", "--tags").ReadOutput()
	if err != nil {
		return currentTag, err
	}
	currentTag = normalizeTag(currentTag)
	currentTagRev, _ := shell.Exec("git", "describe", "--tags", "--abbrev=0").ReadOutput()
	currentTagRev = normalizeTag(currentTagRev)
	if currentTagRev == currentTag {
		return currentTag[1:], nil
	}
	shortCommit, _ := shell.Exec("git", "rev-parse", "--short", "HEAD").ReadOutput()
	// Preserve the complete custom pre-release suffix; badversion.String drops
	// the fork identifier from tags such as v1.14.0-alpha.48-xiaobaf14g.
	return currentTagRev[1:] + "-" + shortCommit, nil
}

func ReadTagVersionRev() (badversion.Version, error) {
	currentTagRev := common.Must1(shell.Exec("git", "describe", "--tags", "--abbrev=0").ReadOutput())
	currentTagRev = normalizeTag(currentTagRev)
	return badversion.Parse(currentTagRev[1:]), nil
}

func ReadTagVersion() (badversion.Version, error) {
	currentTag := common.Must1(shell.Exec("git", "describe", "--tags").ReadOutput())
	currentTagRev := common.Must1(shell.Exec("git", "describe", "--tags", "--abbrev=0").ReadOutput())
	currentTag = normalizeTag(currentTag)
	currentTagRev = normalizeTag(currentTagRev)
	version := badversion.Parse(currentTagRev[1:])
	if currentTagRev != currentTag {
		if version.PreReleaseIdentifier == "" {
			version.Patch++
		}
	}
	return version, nil
}
