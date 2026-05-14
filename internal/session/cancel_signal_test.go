package session

import (
	"testing"
)

func TestCancelRequestWriteClearExists(t *testing.T) {
	dir := t.TempDir()
	if err := WriteCancelRequest(dir); err != nil {
		t.Fatal(err)
	}
	ok, err := CancelRequestExists(dir)
	if err != nil || !ok {
		t.Fatalf("exists=%v err=%v", ok, err)
	}
	if err := ClearCancelRequest(dir); err != nil {
		t.Fatal(err)
	}
	ok, err = CancelRequestExists(dir)
	if err != nil || ok {
		t.Fatalf("after clear exists=%v err=%v", ok, err)
	}
}
