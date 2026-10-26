package update

import "testing"

func TestAssetFileName(t *testing.T) {
	t.Parallel()
	got, err := AssetFileName("0.9.3", "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	if got != "foxxycode_0.9.3_linux_amd64.tar.gz" {
		t.Fatalf("got %q", got)
	}
	got, err = AssetFileName("1.0.0", "windows", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	if got != "foxxycode_1.0.0_windows_amd64.zip" {
		t.Fatalf("got %q", got)
	}
	for _, goarch := range []string{"arm64", "amd64"} {
		got, err = AssetFileName("1.2.30", "android", goarch)
		if err != nil {
			t.Fatal(err)
		}
		if want := "foxxycode_1.2.30_android_" + goarch + ".tar.gz"; got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	}
}

func TestAssetFileName_unsupported(t *testing.T) {
	t.Parallel()
	if _, err := AssetFileName("0.1.0", "freebsd", "amd64"); err == nil {
		t.Fatal("expected error for unsupported platform")
	}
	// Android is published for its two 64-bit architectures.
	for _, goarch := range []string{"arm", "386"} {
		if _, err := AssetFileName("0.1.0", "android", goarch); err == nil {
			t.Fatalf("expected error for android/%s", goarch)
		}
	}
}
