package shell

import "testing"

func TestRunKeyEntry(t *testing.T) {
	if RunKey != `Software\Microsoft\Windows\CurrentVersion\Run` {
		t.Errorf("RunKey = %q", RunKey)
	}
	if got := launchCommand(`C:\Program Files\LiteRSS\LiteRSS.exe`); got != `"C:\Program Files\LiteRSS\LiteRSS.exe"` {
		t.Errorf("launchCommand = %q, want the path quoted", got)
	}
}
