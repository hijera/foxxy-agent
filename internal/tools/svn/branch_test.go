package svn_test

import (
	"context"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/svnws/svntest"
	toolsvn "github.com/hijera/foxxycode-agent/internal/tools/svn"
)

// BranchFor is what a session export asks: the branch of the working copy, read
// locally. Listing the repository's branches contacts the server, which an
// export has no reason to wait for, whatever vcs.svn.branch_lookup says.
func TestBranchForReadsTheWorkingCopyBranchWithoutContactingTheServer(t *testing.T) {
	h := newHarness(t)
	state := svntest.NewState(repoRoot, h.wc)
	state.WorkingCopies[h.wc].Branch = "branches/feature-x"
	if err := h.fake.WriteState(state); err != nil {
		t.Fatal(err)
	}
	lookup := true
	h.cfg.VCS.SVN.BranchLookup = &lookup

	if got := toolsvn.BranchFor(context.Background(), h.cfg, h.wc); got != "branches/feature-x" {
		t.Fatalf("BranchFor = %q, want branches/feature-x", got)
	}
	if call, ok := h.fake.FindCall("list"); ok {
		t.Fatalf("the branch was read with a server round trip: svn %v", call.Args)
	}
}

func TestBranchForIsEmptyWhenSubversionSupportIsOff(t *testing.T) {
	h := newHarness(t)
	off := false
	h.cfg.VCS.SVN.Enabled = &off

	if got := toolsvn.BranchFor(context.Background(), h.cfg, h.wc); got != "" {
		t.Fatalf("BranchFor = %q with vcs.svn.enable: false, want nothing", got)
	}
	if calls, _ := h.fake.Calls(); len(calls) > 0 {
		t.Fatalf("svn was run although Subversion support is off: %v", calls)
	}
}

func TestBranchForIsEmptyOutsideAWorkingCopy(t *testing.T) {
	h := newHarness(t)
	if got := toolsvn.BranchFor(context.Background(), h.cfg, t.TempDir()); got != "" {
		t.Fatalf("BranchFor = %q for a plain folder, want nothing", got)
	}
	if got := toolsvn.BranchFor(context.Background(), nil, h.wc); got != "" {
		t.Fatalf("BranchFor = %q without a config, want nothing", got)
	}
}
