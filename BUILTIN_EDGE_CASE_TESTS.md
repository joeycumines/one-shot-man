# Built-in Commands Edge Case Tests Documentation

This document describes the edge case tests added to `internal/command/builtin_edge_test.go` for built-in commands in the one-shot-man project.

## Tests Added

### 1. TestCodeReviewCommandEdgeCases

Tests edge cases for the `code-review` command.

**Subtests:**
- **EmptyTargetList**: Tests execution with empty arguments
- **NonExistentTargetFile**: Tests handling of non-existent target file
- **NonExistentTargetDirectory**: Tests handling of non-existent target directory
- **DirectoryInsteadOfFile**: Tests handling when a directory is passed instead of a file
- **ExtremelyLongPath**: Tests handling of extremely long file paths (200+ characters)
- **PathWithSpecialCharacters**: Tests handling of paths with spaces and special characters
- **PermissionDeniedFile**: Tests handling of files with no read permissions

**Test Status:** ✅ All 7 subtests pass

### 2. TestGoalCommandEdgeCases

Tests edge cases for the `goal` command.

**Subtests:**
- **NonExistentGoalFile**: Tests that non-existent goal returns an error
- **GoalFileIsDirectory**: Tests that directories are skipped during goal discovery
- **GoalFileContainingInvalidJSON**: Tests handling of invalid JSON in goal files
- **EmptyGoalArray**: Tests registry behavior with no built-in goals
- **GoalContainingInvalidTemplateSyntax**: Tests handling of invalid template syntax in goals
- **GoalCommandWithNoArgsAndNoInteractive**: Tests behavior when no arguments provided and interactive mode is off
- **GoalCommandInvalidFlagCombination**: Tests behavior with invalid flag combinations

**Test Status:** ✅ All 7 subtests pass

### 3. TestPromptFlowCommandEdgeCases

Tests edge cases for the `prompt-flow` command.

**Subtests:**
- **MissingContext**: Tests execution without context
- **VeryLongPromptText**: Tests handling of very long prompt text (1000 repetitions)
- **InvalidPromptTemplateSyntax**: Tests handling of invalid template syntax
- **UnicodeContentInPrompt**: Tests handling of unicode content in prompts
- **PromptFlowWithArgsOnlyNoInteractive**: Tests behavior with only arguments and no interactive mode

**Test Status:** ✅ All 5 subtests pass

### 4. TestSuperDocumentCommandEdgeCases

Tests edge cases for the `super-document` command.

**Subtests:**
- **EmptyInput**: Tests execution with empty input
- **InputContainingOnlyWhitespace**: Tests handling of whitespace-only input
- **InputIsNonExistentFilePath**: Tests handling of non-existent file path input
- **InputIsDirectory**: Tests handling when a directory is passed as input
- **ExtremelyLongInput**: Tests handling of extremely long input (100,000 characters)
- **SuperDocumentCommandWithNilConfig**: Tests execution with nil configuration
- **SuperDocumentCommandFlagsCombination**: Tests that all flags are properly defined

**Test Status:** ✅ All 7 subtests pass

### 5. TestGoalCommandGoalLoadingEdgeCases

Tests additional edge cases for goal loading and processing.

**Subtests:**
- **GoalWithCircularTemplateReferences**: Tests goals with self-referencing templates
- **GoalWithEmptyRequiredFields**: Tests goals with minimal/empty fields
- **GoalWithDuplicateCommands**: Tests goals with duplicate command definitions
- **GoalWithSpecialCharactersInName**: Tests goals with special characters in names
- **GoalJSONMarshalUnmarshal**: Tests JSON marshaling and unmarshaling of Goal structs

**Test Status:** ✅ All 5 subtests pass

### 6. TestCodeReviewCommandWithVariousFlags

Tests the `code-review` command with various flag combinations.

**Subtests:**
- **NonInteractiveWithSessionOverride**: Tests non-interactive mode with custom session ID
- **WithStoreBackendFS**: Tests with filesystem storage backend
- **FlagParsingEdgeCases**: Tests various flag parsing combinations

**Test Status:** ✅ All 3 subtests pass

### 7. TestSuperDocumentCommandWithVariousFlags

Tests the `super-document` command with various flag combinations.

**Subtests:**
- **ShellModeFlag**: Tests shell mode flag
- **InteractiveFalseWithArgs**: Tests interactive false mode with arguments

**Test Status:** ✅ All 2 subtests pass

## Summary

| Test Function | Subtests | Status |
|--------------|----------|--------|
| TestCodeReviewCommandEdgeCases | 7 | ✅ Pass |
| TestGoalCommandEdgeCases | 7 | ✅ Pass |
| TestPromptFlowCommandEdgeCases | 5 | ✅ Pass |
| TestSuperDocumentCommandEdgeCases | 7 | ✅ Pass |
| TestGoalCommandGoalLoadingEdgeCases | 5 | ✅ Pass |
| TestCodeReviewCommandWithVariousFlags | 3 | ✅ Pass |
| TestSuperDocumentCommandWithVariousFlags | 2 | ✅ Pass |
| **Total** | **36** | **✅ All Pass** |

## Verification

All tests have been verified to:
- ✅ Compile without errors
- ✅ Pass all subtests
- ✅ Pass `go vet`
- ✅ Pass `staticcheck`
- ✅ Pass `deadcode`

## Test File Location

`/Users/joeyc/dev/one-shot-man/internal/command/builtin_edge_test.go`

## Commands Tested

1. **code-review**: Single-prompt code review with context
2. **goal**: Access pre-written goals for common development tasks
3. **prompt-flow**: Interactive prompt builder: goal/context/template -> generate -> assemble
4. **super-document**: TUI for merging documents into a single internally consistent super-document
