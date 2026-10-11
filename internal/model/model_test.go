package model

import (
	"strings"
	"testing"
)

func TestIdentifiers(t *testing.T) {
	for _, p := range []string{"macstand", "a", "plate-02"} {
		if e := ValidateProject(p); e != nil {
			t.Fatal(e)
		}
	}
	for _, p := range []string{"", "../x", "X", "con", "a/b", "a b"} {
		if ValidateProject(p) == nil {
			t.Fatalf("accepted %q", p)
		}
	}
	for _, v := range []string{"1.0.0", "0.0.1", "10.20.30"} {
		if e := ValidateVersion(v); e != nil {
			t.Fatal(e)
		}
	}
	for _, v := range []string{"v1.0.0", "01.0.0", "1.0", "../x", "1.0.0-beta", "1000000000.0.0"} {
		if ValidateVersion(v) == nil {
			t.Fatalf("accepted %q", v)
		}
	}
	if CompareVersion("1.10.0", "1.9.0") <= 0 {
		t.Fatal("lexicographic version ordering")
	}
	p, v, ok := ParseTag("macstand/v1.0.0")
	if !ok || p != "macstand" || v != "1.0.0" {
		t.Fatal("tag")
	}
}
func TestUnsafePaths(t *testing.T) {
	for _, p := range []string{"../x", "/etc/x", `C:\x`, `a\x`, "a/../x", "a//x", "a/CON.txt", "NUL.dxf", "x.", "x ", "x:stream", "a\x00b", "a\nfile", "a?b"} {
		if SafePath(p) == nil {
			t.Fatalf("accepted %q", p)
		}
	}
	if e := SafePath("drawings/деталь.dxf"); e != nil {
		t.Fatal(e)
	}
}
func TestMalformedManifest(t *testing.T) {
	for _, s := range []string{"{}", "{", `{"schema":99}`, strings.Repeat(" ", int(MaxManifest)+1)} {
		if _, e := Decode(strings.NewReader(s)); e == nil {
			t.Fatal("accepted invalid manifest")
		}
	}
}
