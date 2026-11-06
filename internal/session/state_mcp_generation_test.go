package session

import (
	"testing"

	"github.com/hijera/foxxycode-agent/internal/mcp"
)

func TestConfiguredMCPConnectCannotPublishAnOlderWorkspace(t *testing.T) {
	st := &State{}
	defer st.CloseAll()
	oldDone, oldPublish := st.beginConfiguredMCPConnectWithPublisher()
	newDone, newPublish := st.beginConfiguredMCPConnectWithPublisher()
	current := mcp.NewStaticClient("current-workspace", nil)
	if !newPublish([]*mcp.Client{current}) {
		t.Fatal("current connection was rejected")
	}
	newDone()
	if oldPublish([]*mcp.Client{mcp.NewStaticClient("old-workspace", nil)}) {
		t.Fatal("a connection from an older workspace was installed")
	}
	oldDone()
	got := st.GetMCPClients()
	if len(got) != 1 || got[0] != current || !st.MCPReady() {
		t.Fatalf("current workspace lost its clients or readiness: %+v", got)
	}
}

func TestConfiguredMCPConnectCannotPublishAfterClose(t *testing.T) {
	st := &State{}
	done, publish := st.beginConfiguredMCPConnectWithPublisher()
	st.CloseAll()
	if publish([]*mcp.Client{mcp.NewStaticClient("late", nil)}) {
		t.Fatal("a connection was installed after closing the session")
	}
	done()
	if len(st.GetMCPClients()) != 0 || !st.MCPReady() {
		t.Fatal("a closed session retained clients or a pending connection")
	}
}
