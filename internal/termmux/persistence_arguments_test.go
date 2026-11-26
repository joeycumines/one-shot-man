package termmux

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"
)

func TestCaptureSession_VerbatimPersistence(t *testing.T) {
	if testing.Short() {
		t.Skip("runs a session manager and saves session state")
	}
	manager, shutdown := startManager(t)
	defer shutdown()
	args := []string{"/d", "/s", "/c", `""C:\program files\helper.exe" "two words" && exit /b 7"`}
	for _, verbatim := range []bool{false, true} {
		name := "default"
		if verbatim {
			name = "verbatim"
		}
		session := NewCaptureSession(CaptureConfig{Command: "cmd.exe", Args: args, WindowsVerbatimArguments: verbatim})
		if _, err := manager.Register(session, SessionTarget{Name: name}); err != nil {
			t.Fatal(err)
		}
		// ExportConfig is a snapshot; changing it must not change later
		// persisted command parsing.
		exported := session.ExportConfig()
		exported.WindowsVerbatimArguments = !verbatim
		exported.Args[0] = "mutated"
	}
	state, err := manager.ExportState()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "session-state.json")
	if err := SaveManagerState(path, state); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadManagerState(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, session := range loaded.Sessions {
		if err := manager.Unregister(SessionID(session.SessionID)); err != nil {
			t.Fatal(err)
		}
	}
	result, err := manager.RestoreFromState(loaded, func(session PersistedSession) (InteractiveSession, error) {
		return NewCaptureSession(CaptureConfig{Command: session.Command, Args: session.Args, WindowsVerbatimArguments: session.WindowsVerbatimArguments}), nil
	})
	if err != nil || len(result.Failed) != 0 || len(result.Restored) != 2 {
		t.Fatalf("restore: result=%+v err=%v", result, err)
	}
	restored, err := manager.ExportState()
	if err != nil {
		t.Fatal(err)
	}
	for stage, snapshot := range map[string]*PersistedManagerState{"exported": state, "loaded": loaded, "restored": restored} {
		if len(snapshot.Sessions) != 2 {
			t.Fatalf("%s sessions=%d, want 2", stage, len(snapshot.Sessions))
		}
		for _, session := range snapshot.Sessions {
			want := session.Target.Name == "verbatim"
			if session.Command != "cmd.exe" || !reflect.DeepEqual(session.Args, args) || session.WindowsVerbatimArguments != want {
				t.Errorf("%s changed command parsing: %+v", stage, session)
			}
			encoded, err := json.Marshal(session)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(encoded, &fields); err != nil {
				t.Fatal(err)
			}
			if _, exists := fields["windowsVerbatimArguments"]; exists != want {
				t.Errorf("%s optional parsing mode exists=%v, want %v: %s", stage, exists, want, encoded)
			}
		}
	}
	// Existing state files have no parsing-mode field and must retain argv
	// escaping when decoded by the new schema.
	var legacy PersistedSession
	if err := json.Unmarshal([]byte(`{"command":"helper.exe","args":["two words"]}`), &legacy); err != nil {
		t.Fatal(err)
	}
	if legacy.WindowsVerbatimArguments {
		t.Fatal("omitted parsing mode enabled verbatim arguments")
	}
}
