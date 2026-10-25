package web

import (
	"io/fs"
	"testing"
)

func TestDistIsRootedAtDist(t *testing.T) {
	// The placeholder is at dist/.gitkeep, so it is at the root of Dist.
	if _, err := fs.Stat(Dist(), ".gitkeep"); err != nil {
		t.Errorf("placeholder not at the root of Dist: %v", err)
	}
}
