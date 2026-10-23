package privdrop

import (
	"os/user"
	"strconv"
	"testing"
)

func TestResolve(t *testing.T) {
	root, err := Resolve("root")
	if err != nil || root.UID != 0 {
		t.Fatalf("root: %+v %v", root, err)
	}
	me, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	got, err := Resolve(me.Username)
	if err != nil || strconv.Itoa(got.UID) != me.Uid {
		t.Fatalf("current user: %+v %v", got, err)
	}
	if _, err := Resolve("no-such-user-cybercity"); err == nil {
		t.Fatal("unknown user resolved")
	}
	if _, err := Resolve(""); err == nil {
		t.Fatal("empty name resolved")
	}
}

func TestPlan(t *testing.T) {
	cases := []struct {
		euid      int
		requested string
		target    string
		warns     bool
	}{
		{0, "", DefaultUser, false},
		{0, "cybercity", "cybercity", false},
		{0, "root", "", true},
		{1000, "", "", false},
		{1000, "nobody", "", true},
	}
	for _, tc := range cases {
		target, warn := Plan(tc.euid, tc.requested)
		if target != tc.target || (warn != "") != tc.warns {
			t.Errorf("Plan(%d, %q) = %q, %q", tc.euid, tc.requested, target, warn)
		}
	}
}
