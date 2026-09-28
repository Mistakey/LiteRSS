package shell

import (
	"errors"
	"testing"

	"LiteRSS/internal/identity"
)

// fakeItems stands in for the Run key, so tests never touch the user's.
type fakeItems struct {
	entries map[string]string
	err     error
}

func (f *fakeItems) set(name, command string) error {
	if f.err != nil {
		return f.err
	}
	f.entries[name] = command
	return nil
}

func (f *fakeItems) remove(name string) error {
	if f.err != nil {
		return f.err
	}
	delete(f.entries, name)
	return nil
}

func TestAutostartUsesTheIdentityValueName(t *testing.T) {
	a, err := NewAutostart()
	if err != nil {
		t.Fatal(err)
	}
	if a.name != identity.Current().AutostartValueName || a.name == "" {
		t.Errorf("value name = %q, want identity's %q", a.name, identity.Current().AutostartValueName)
	}
	if a.name == "MrRSS" {
		t.Error("value name collides with the legacy MrRSS entry")
	}
}

func TestAutostartApply(t *testing.T) {
	items := &fakeItems{entries: map[string]string{"MrRSS": `"C:\old\MrRSS.exe"`}}
	a := newAutostart("LiteRSS", `C:\Apps\LiteRSS\LiteRSS.exe`, items)

	if err := a.Apply(true); err != nil {
		t.Fatal(err)
	}
	if got := items.entries["LiteRSS"]; got != launchCommand(`C:\Apps\LiteRSS\LiteRSS.exe`) {
		t.Errorf("entry = %q", got)
	}
	if err := a.Apply(false); err != nil {
		t.Fatal(err)
	}
	if _, ok := items.entries["LiteRSS"]; ok {
		t.Error("entry still present after disabling")
	}
	if _, ok := items.entries["MrRSS"]; !ok {
		t.Error("the legacy entry was touched")
	}
	// Disabling when nothing is registered is fine.
	if err := a.Apply(false); err != nil {
		t.Errorf("second disable: %v", err)
	}

	items.err = errors.New("access denied")
	if err := a.Apply(true); err == nil {
		t.Error("a failed write was not reported")
	}
}
