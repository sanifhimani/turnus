package launchd

import (
	"os"
	"testing"
)

func TestParseDomain(t *testing.T) {
	f, err := os.Open("testdata/print-gui.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	d := parseDomain(f)

	if len(d.services) != 6 {
		t.Fatalf("got %d services, want 6", len(d.services))
	}

	mysql := d.services["homebrew.mxcl.mysql"]
	if mysql.pid != 1820 || mysql.lastExit != nil {
		t.Errorf("mysql = %+v", mysql)
	}
	colima := d.services["homebrew.mxcl.colima"]
	if colima.pid != 0 || colima.lastExit == nil || *colima.lastExit != 1 {
		t.Errorf("colima = %+v", colima)
	}
	if s := d.services["com.apple.xpc.loginitemregisterd"]; s.pid != 41452 || s.lastExit != nil {
		t.Errorf("loginitemregisterd = %+v", s)
	}

	if !d.disabled["com.apple.Siri.agent"] {
		t.Error("Siri agent should be disabled")
	}
	if d.disabled["com.docker.helper"] {
		t.Error("docker helper should be enabled")
	}
}
