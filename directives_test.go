package gqlx

import (
	"testing"
)

func TestBuiltInDirectives(t *testing.T) {
	dirs := BuiltInDirectives()
	if len(dirs) != 4 {
		t.Fatalf("expected 4 built-in directives, got %d", len(dirs))
	}

	names := map[string]bool{}
	for _, d := range dirs {
		names[d.Name_] = true
	}

	for _, name := range []string{"include", "skip", "deprecated", "specifiedBy"} {
		if !names[name] {
			t.Errorf("missing built-in directive @%s", name)
		}
	}
}

func TestIncludeDirective(t *testing.T) {
	if IncludeDirective.Name_ != "include" {
		t.Errorf("name = %q", IncludeDirective.Name_)
	}
	if len(IncludeDirective.Locations) != 3 {
		t.Errorf("expected 3 locations, got %d", len(IncludeDirective.Locations))
	}
	ifArg, ok := IncludeDirective.Args["if"]
	if !ok {
		t.Fatal("missing 'if' argument")
	}
	if _, isNN := ifArg.Type.(*NonNullOfType); !isNN {
		t.Error("'if' argument should be non-null")
	}
}

func TestSkipDirective(t *testing.T) {
	if SkipDirective.Name_ != "skip" {
		t.Errorf("name = %q", SkipDirective.Name_)
	}
	if len(SkipDirective.Locations) != 3 {
		t.Errorf("expected 3 locations, got %d", len(SkipDirective.Locations))
	}
}

func TestDeprecatedDirective(t *testing.T) {
	if DeprecatedDirective.Name_ != "deprecated" {
		t.Errorf("name = %q", DeprecatedDirective.Name_)
	}
	reasonArg, ok := DeprecatedDirective.Args["reason"]
	if !ok {
		t.Fatal("missing 'reason' argument")
	}
	if reasonArg.DefaultValue != "No longer supported" {
		t.Errorf("default value = %q", reasonArg.DefaultValue)
	}
}

func TestSpecifiedByDirective(t *testing.T) {
	if SpecifiedByDirective.Name_ != "specifiedBy" {
		t.Errorf("name = %q", SpecifiedByDirective.Name_)
	}
	urlArg, ok := SpecifiedByDirective.Args["url"]
	if !ok {
		t.Fatal("missing 'url' argument")
	}
	if _, isNN := urlArg.Type.(*NonNullOfType); !isNN {
		t.Error("'url' argument should be non-null")
	}
}
