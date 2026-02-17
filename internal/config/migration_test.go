package config

import (
	"testing"
)

func TestDetectSchemaVersion_Absent(t *testing.T) {
	t.Parallel()
	c := NewConfig()
	if got := detectSchemaVersion(c); got != 0 {
		t.Fatalf("expected 0 for absent key, got %d", got)
	}
}

func TestDetectSchemaVersion_Valid(t *testing.T) {
	t.Parallel()
	c := NewConfig()
	c.SetGlobalOption("config.schema-version", "1")
	if got := detectSchemaVersion(c); got != 1 {
		t.Fatalf("expected 1, got %d", got)
	}

	c2 := NewConfig()
	c2.SetGlobalOption("config.schema-version", "42")
	if got := detectSchemaVersion(c2); got != 42 {
		t.Fatalf("expected 42, got %d", got)
	}
}

func TestDetectSchemaVersion_Invalid(t *testing.T) {
	t.Parallel()
	cases := []string{"abc", "1.5", "", "true"}
	for _, v := range cases {
		c := NewConfig()
		c.SetGlobalOption("config.schema-version", v)
		if got := detectSchemaVersion(c); got != 0 {
			t.Errorf("expected 0 for invalid value %q, got %d", v, got)
		}
	}
}

func TestMigrateConfig_AlreadyCurrent(t *testing.T) {
	t.Parallel()
	c := NewConfig()
	c.SetGlobalOption("config.schema-version", "1")

	result, err := MigrateConfig(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil result for already-current config, got %+v", result)
	}
}

func TestMigrateConfig_V0ToV1(t *testing.T) {
	t.Parallel()
	c := NewConfig()
	// No config.schema-version set → version 0.

	result, err := MigrateConfig(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for v0→v1 migration")
	}
	if result.FromVersion != 0 {
		t.Errorf("expected FromVersion=0, got %d", result.FromVersion)
	}
	if result.ToVersion != SchemaVersion {
		t.Errorf("expected ToVersion=%d, got %d", SchemaVersion, result.ToVersion)
	}
	if len(result.Changes) == 0 {
		t.Error("expected at least one change description")
	}
}

func TestMigrateConfig_FutureVersion(t *testing.T) {
	t.Parallel()
	c := NewConfig()
	c.SetGlobalOption("config.schema-version", "999")

	result, err := MigrateConfig(c)
	if err == nil {
		t.Fatal("expected error for future version")
	}
	if result != nil {
		t.Fatalf("expected nil result on error, got %+v", result)
	}
}

func TestMigrateConfig_SetsVersionField(t *testing.T) {
	t.Parallel()
	c := NewConfig()
	// No version → v0.

	_, err := MigrateConfig(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	v, ok := c.GetGlobalOption("config.schema-version")
	if !ok {
		t.Fatal("expected config.schema-version to be set after migration")
	}
	if v != "1" {
		t.Fatalf("expected config.schema-version=1, got %q", v)
	}
}

func TestSchemaVersion_InDefaultSchema(t *testing.T) {
	t.Parallel()
	s := DefaultSchema()
	opt := s.Lookup("", "config.schema-version")
	if opt == nil {
		t.Fatal("expected config.schema-version to be registered in DefaultSchema")
	}
	if opt.Type != TypeInt {
		t.Errorf("expected TypeInt, got %q", opt.Type)
	}
	if opt.Default != "1" {
		t.Errorf("expected default '1', got %q", opt.Default)
	}
}
