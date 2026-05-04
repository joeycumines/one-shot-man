package command

import (
	"encoding/json"
	"regexp"
	"testing"
)

// FuzzGoalJSONParsing fuzzes Goal struct JSON unmarshaling combined with
// validateGoal and isValidGoalName. Seeds cover valid goals, minimal objects,
// edge-case strings, and non-JSON input.
func FuzzGoalJSONParsing(f *testing.F) {
	validGoalNameRE := regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9-]*$`)

	// Seed corpus
	seeds := []string{
		// Valid goal JSON
		`{"name":"code-review","description":"Review code changes","category":"dev"}`,
		// Minimal valid
		`{"name":"a","description":"d"}`,
		// Empty object
		`{}`,
		// Empty string
		``,
		// Not JSON at all
		`not json`,
		// Binary-ish
		"\x00\x01\x02\xff",
		// Deeply nested
		`{"name":"x","description":"y","stateVars":{"a":{"b":{"c":{"d":1}}}}}`,
		// Huge name
		`{"name":"` + string(make([]byte, 1024)) + `","description":"d"}`,
		// Unicode
		`{"name":"日本語","description":"テスト"}`,
		// Name with spaces (invalid)
		`{"name":"has space","description":"d"}`,
		// Name starting with hyphen (invalid)
		`{"name":"-bad","description":"d"}`,
		// Valid with all fields
		`{"name":"full-goal","description":"A full goal","category":"testing","tuiTitle":"Title","tuiPrompt":"Prompt","promptInstructions":"Do stuff","commands":[{"name":"cmd1","type":"custom"}]}`,
		// Array instead of object
		`[1,2,3]`,
		// Null
		`null`,
		// Number
		`42`,
		// String
		`"just a string"`,
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, data string) {
		var goal Goal

		unmarshalErr := json.Unmarshal([]byte(data), &goal)
		validateErr := validateGoal(&goal)

		// Invariant 1: If unmarshal AND validation both succeed,
		// Name must be non-empty, match the regex, and Description must be non-empty.
		if unmarshalErr == nil && validateErr == nil {
			if goal.Name == "" {
				t.Fatal("validateGoal passed but Name is empty")
			}
			if !validGoalNameRE.MatchString(goal.Name) {
				t.Fatalf("validateGoal passed but Name %q does not match regex", goal.Name)
			}
			if goal.Description == "" {
				t.Fatal("validateGoal passed but Description is empty")
			}
		}

		// Invariant 2: If validateGoal passes, isValidGoalName(goal.Name) must be true.
		if validateErr == nil {
			if !isValidGoalName(goal.Name) {
				t.Fatalf("validateGoal passed but isValidGoalName(%q) returned false", goal.Name)
			}
		}

		// Invariant 3: isValidGoalName must be consistent with the regex.
		result := isValidGoalName(goal.Name)
		regexResult := validGoalNameRE.MatchString(goal.Name)
		if result != regexResult {
			t.Fatalf("isValidGoalName(%q)=%v but regex says %v", goal.Name, result, regexResult)
		}
	})
}
