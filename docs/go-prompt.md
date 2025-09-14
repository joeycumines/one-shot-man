# go-prompt Integration Analysis for one-shot-man Scripting Engine

## Executive Summary

This document provides a comprehensive analysis of integrating `github.com/elk-language/go-prompt` v1.3.1 with the one-shot-man scripting engine. The current implementation already uses go-prompt for basic TUI functionality, but there are significant opportunities for deeper integration and enhanced API exposure.

## Current State Analysis

### Existing Integration
The project currently uses go-prompt through the `TUIManager` in `internal/scripting/tui.go`:

```go
type TUIManager struct {
    // ... existing fields ...
    prompt *prompt.Prompt
}
```

The integration is basic:
- Simple prompt creation with `prompt.New()`
- Basic command execution via `executor` function
- Limited configuration options

### Current Limitations
1. **Shallow API Exposure**: Only basic prompt functionality is exposed to JavaScript
2. **Limited Customization**: Few prompt options are configurable from scripts
3. **No Advanced Features**: Completion, syntax highlighting, and advanced key bindings are not exposed
4. **Stateless Integration**: No persistence of prompt state between sessions

## Detailed Codebase Analysis

### Current TUIConfig Structure
The existing `TUIConfig` struct shows minimal utilization of go-prompt features:

```go
type TUIConfig struct {
    Title         string
    Prompt        string
    CompletionFn  goja.Callable  // Not currently used
    ValidatorFn   goja.Callable  // Not currently used
    HistoryFile   string
    EnableHistory bool
    CustomSuggest []prompt.Suggest  // Not currently used
}
```

**Gap Analysis**: Fields like `CompletionFn`, `ValidatorFn`, and `CustomSuggest` are defined but not integrated with the prompt instance.

### Current Prompt Initialization
The prompt is created with minimal options:

```go
tm.prompt = prompt.New(
    tm.executor,
    prompt.WithTitle("one-shot-man rich TUI"),
)
```

**Missing Options**: No completer, history, key bindings, lexer, or other advanced features.

### Context Manager Integration Opportunities
The `ContextManager` provides rich data that could power intelligent completion:

```go
// Available methods for completion candidates
func (cm *ContextManager) GetFilesByExtension(ext string) []string
func (cm *ContextManager) FilterPaths(pattern string) ([]string, error)
func (cm *ContextManager) ListPaths() []string
```

## Proposed Integration Architecture

### Core API Extensions

#### 1. Enhanced Prompt Configuration
Expose comprehensive prompt configuration options to JavaScript:

```javascript
// Proposed JavaScript API
tui.createAdvancedPrompt({
    title: "Advanced Script Editor",
    prefix: "script> ",
    completer: function(document) {
        // Custom completion logic
        return suggestions;
    },
    lexer: customLexer,
    keyBindings: [...],
    history: {
        file: ".script_history",
        size: 1000
    },
    colors: {
        prefix: "cyan",
        input: "white",
        suggestions: "yellow"
    }
});
```

#### 2. Completion System Integration
Leverage go-prompt's completion system with the existing context manager:

```javascript
// Context-aware completion
tui.registerCompleter("file", function(document) {
    var word = document.getWordBeforeCursor();
    var files = context.getFilesByExt("*");
    return files.filter(f => f.startsWith(word)).map(f => ({
        text: f,
        description: "File from context"
    }));
});
```

#### 3. Syntax Highlighting Support
Integrate with go-prompt's lexer system for JavaScript syntax highlighting:

```javascript
// JavaScript lexer integration
tui.registerLexer("javascript", {
    tokenize: function(input) {
        // Tokenize JavaScript code
        return tokens;
    }
});
```

#### 4. Advanced Key Bindings
Expose key binding system for custom shortcuts:

```javascript
tui.registerKeyBinding("ctrl+r", function(prompt) {
    // Custom key handler
    prompt.InsertText("console.log('debug');");
    return true; // rerender
});
```

### Technical Implementation Details

#### 1. Prompt Instance Management
Extend `TUIManager` to support multiple prompt instances:

```go
type TUIManager struct {
    // ... existing fields ...
    prompts map[string]*PromptInstance
    activePrompt *PromptInstance
}
```

#### 2. JavaScript Bridge Extensions
Add comprehensive JavaScript bindings:

```go
// Proposed new methods in TUIManager
func (tm *TUIManager) jsCreateAdvancedPrompt(config interface{}) *PromptWrapper
func (tm *TUIManager) jsRegisterCompleter(name string, completer goja.Callable) error
func (tm *TUIManager) jsRegisterKeyBinding(key string, handler goja.Callable) error
func (tm *TUIManager) jsSetLexer(lexer interface{}) error
```

#### 3. State Persistence
Implement prompt state serialization:

```go
type PromptState struct {
    History []string
    Variables map[string]interface{}
    LastCommand string
    CursorPosition int
}
```

## Comprehensive go-prompt API Mapping

Based on the analysis of `github.com/elk-language/go-prompt` v1.3.1, here's the complete API surface that should be considered for exposure:

### Core Types to Expose

#### 1. Prompt Methods
```go
// Direct method exposure to JavaScript
type JSPromptAPI struct {
    // Core interaction
    Run()                                    // Start the prompt
    Input() string                          // Get current input
    Close()                                 // Clean up resources

    // Text manipulation
    InsertText(text string, overwrite bool)
    InsertTextMoveCursor(text string, overwrite bool)
    Delete(count istrings.GraphemeNumber) string
    DeleteBeforeCursor(count istrings.GraphemeNumber) string
    DeleteRunes(count istrings.RuneNumber) string
    DeleteBeforeCursorRunes(count istrings.RuneNumber) string

    // Cursor movement
    CursorDown(count int) bool
    CursorLeft(count istrings.GraphemeNumber) bool
    CursorLeftRunes(count istrings.RuneNumber) bool
    CursorRight(count istrings.GraphemeNumber) bool
    CursorRightRunes(count istrings.RuneNumber) bool
    CursorUp(count int) bool

    // History access
    History() HistoryInterface

    // Buffer operations
    Buffer() *Buffer

    // Utility methods
    IndentSize() int
    TerminalColumns() istrings.Width
    TerminalRows() int
    UserInputColumns() istrings.Width
}
```

#### 2. Document API
```go
// Document state access
type JSDocumentAPI struct {
    Text() string
    CurrentLine() string
    CurrentLineBeforeCursor() string
    CurrentLineAfterCursor() string
    TextBeforeCursor() string
    TextAfterCursor() string

    // Position information
    CurrentRuneIndex() istrings.RuneNumber
    CursorPositionCol() istrings.Width
    CursorPositionRow() istrings.RuneNumber
    DisplayCursorPosition(columns istrings.Width) Position

    // Word operations
    GetWordBeforeCursor() string
    GetWordAfterCursor() string
    FindStartOfPreviousWord() istrings.ByteNumber
    FindEndOfCurrentWord() istrings.ByteNumber

    // Line operations
    Lines() []string
    LineCount() int
    OnLastLine() bool
    PreviousLine() (string, bool)

    // Navigation helpers
    GetCursorLeftPosition(count istrings.GraphemeNumber) istrings.RuneNumber
    GetCursorRightPosition(count istrings.GraphemeNumber) istrings.RuneNumber
    GetCursorUpPosition(count int, preferredColumn istrings.Width) istrings.RuneNumber
    GetCursorDownPosition(count int, preferredColumn istrings.Width) istrings.RuneNumber
}
```

#### 3. Buffer API
```go
// Advanced buffer manipulation
type JSBufferAPI struct {
    Text() string
    Document() *Document
    DisplayCursorPosition(columns istrings.Width) Position

    // Complex operations
    InsertTextMoveCursor(text string, columns istrings.Width, rows int, overwrite bool)
    JoinNextLine(separator string, col istrings.Width, row int)
    NewLine(columns istrings.Width, rows int, copyMargin bool)
    SwapCharactersBeforeCursor(col istrings.Width, row int)
}
```

#### 4. History API
```go
// History management
type JSHistoryAPI struct {
    Add(input string)
    Clear()
    DeleteAll()
    Entries() []string
    Get(i int) (string, bool)
    Newer(buf *Buffer, columns istrings.Width, rows int) (*Buffer, bool)
    Older(buf *Buffer, columns istrings.Width, rows int) (*Buffer, bool)
}
```

### Configuration Options to Expose

#### 1. Prompt Options
```javascript
// JavaScript configuration object
var promptConfig = {
    // Basic options
    title: "Custom Prompt",
    prefix: ">>> ",
    initialText: "starting text",

    // Completion
    completer: completionFunction,
    maxSuggestions: 10,
    completionOnDown: true,
    completionWordSeparator: " ",
    showCompletionAtStart: false,

    // History
    history: ["cmd1", "cmd2"],
    historySize: 1000,
    customHistory: customHistoryObject,

    // Appearance
    inputTextColor: prompt.DefaultColor,
    inputBGColor: prompt.DefaultColor,
    prefixTextColor: prompt.DefaultColor,
    prefixBGColor: prompt.DefaultColor,
    suggestionTextColor: prompt.DefaultColor,
    suggestionBGColor: prompt.DefaultColor,
    selectedSuggestionTextColor: prompt.DefaultColor,
    selectedSuggestionBGColor: prompt.DefaultColor,
    descriptionTextColor: prompt.DefaultColor,
    descriptionBGColor: prompt.DefaultColor,
    scrollbarThumbColor: prompt.DefaultColor,
    scrollbarBGColor: prompt.DefaultColor,

    // Behavior
    indentSize: 2,
    lexer: customLexer,
    keyBindMode: prompt.CommonKeyBind,
    exitChecker: exitCheckFunction,
    executeOnEnter: executeOnEnterFunction,
    breakLineCallback: breakLineFunction,

    // I/O
    reader: customReader,
    writer: customWriter
};
```

#### 2. Key Binding API
```javascript
// Key binding registration
tui.registerKeyBinding({
    key: "Ctrl+R",
    description: "Reverse search history",
    handler: function(prompt) {
        // Implementation
        return true; // rerender
    }
});

// Key binding modes
tui.setKeyBindMode(prompt.CommonKeyBind); // or EmacsKeyBind, ViKeyBind
```

#### 3. Lexer Integration
```javascript
// Custom lexer registration
tui.registerLexer("javascript", {
    init: function(input) {
        // Initialize lexer state
    },
    next: function() {
        // Return next token
        return { token, hasNext };
    }
});
```

## Specific Integration Opportunities

### 1. Document API Integration
The `Document` type from go-prompt provides rich text manipulation capabilities that could be exposed:

```javascript
// Proposed Document API exposure
var doc = prompt.getDocument();
var word = doc.getWordBeforeCursor();
var line = doc.currentLine();
var position = doc.cursorPosition();
```

**Current Gap**: No access to document state or text manipulation from JavaScript.

### 2. Buffer Operations
The `Buffer` type enables advanced text editing operations:

```javascript
// Proposed Buffer API
prompt.insertText("text", false);  // Insert without overwriting
prompt.delete(5);                  // Delete 5 characters
prompt.cursorLeft(3);              // Move cursor left
```

**Current Gap**: No programmatic control over text editing operations.

### 3. History Management
Advanced history operations beyond basic storage:

```javascript
// Proposed History API
var history = prompt.getHistory();
history.add("custom command");
history.search("pattern");  // Incremental search
```

**Current Gap**: History is managed internally with no JavaScript access.

### 4. Multi-line Support
go-prompt supports multi-line editing which could enhance script editing:

```javascript
// Multi-line editing configuration
tui.createPrompt({
    multiLine: true,
    lineContinuation: function(line) {
        return line.endsWith("\\");
    }
});
```

**Current Gap**: All input is treated as single-line.

## Advanced go-prompt Features Not Currently Used

### 1. Lexer System
- **Purpose**: Syntax highlighting and tokenization
- **Current State**: Not implemented
- **Integration Potential**: JavaScript syntax highlighting for better code editing

### 2. Key Bindings
- **Purpose**: Custom keyboard shortcuts
- **Current State**: Only default bindings
- **Integration Potential**: Mode-specific shortcuts, Emacs/Vim keybindings

### 3. Completion Filtering
- **Purpose**: Intelligent suggestion filtering
- **Current State**: No completion system
- **Integration Potential**: Fuzzy matching, prefix/suffix filtering

### 4. Color Customization
- **Purpose**: Theming and visual customization
- **Current State**: Default colors only
- **Integration Potential**: Theme support, accessibility options

### 5. Input Validation
- **Purpose**: Real-time input validation
- **Current State**: No validation
- **Integration Potential**: Syntax validation, command validation

## Integration Points with Existing Systems

### 1. Context Manager Integration
The existing `ContextManager` can provide completion candidates:

```go
func (cm *ContextManager) GetCompletionCandidates(prefix string) []prompt.Suggest {
    // Return context-aware suggestions
}
```

### 2. Logging System Integration
Prompt interactions can be logged through the existing logger:

```go
func (tm *TUIManager) logPromptInteraction(action, details string) {
    tm.engine.logger.Info("Prompt interaction", "action", action, "details", details)
}
```

### 3. Script Mode Integration
Each script mode can have its own prompt configuration:

```javascript
tui.registerMode({
    name: "advanced-editor",
    prompt: {
        config: advancedPromptConfig,
        onInput: function(input) { /* handle input */ },
        onCompletion: function(selected) { /* handle completion */ }
    }
});
```

## Implementation Priority Matrix

### High Priority (Immediate Value)
1. **Basic Completion System** - File and command completion
2. **History Persistence** - Save/restore command history
3. **Enhanced Configuration** - Expose more prompt options
4. **Document Access** - Read current input state

### Medium Priority (Enhanced UX)
1. **Syntax Highlighting** - JavaScript lexer integration
2. **Key Bindings** - Custom shortcuts
3. **Multi-line Support** - Better code editing
4. **Color Customization** - Theming support

### Low Priority (Advanced Features)
1. **Session Management** - Save/restore prompt sessions
2. **Macro System** - Record/playback command sequences
3. **Plugin Architecture** - Extensible prompt behavior
4. **Advanced Filtering** - Fuzzy search, ranking

## Performance Considerations

### 1. Memory Management
- Prompt instances should be properly cleaned up
- History should be bounded to prevent memory leaks
- Token caching for syntax highlighting

### 2. Rendering Optimization
- Debounce rapid updates
- Virtual scrolling for large suggestion lists
- Incremental rendering for large documents

### 3. Threading Considerations
- Prompt operations should be thread-safe
- Background completion processing
- Non-blocking history loading

## Security Implications

### 1. Input Validation
- Sanitize completion suggestions
- Validate key binding inputs
- Safe evaluation of dynamic JavaScript

### 2. Resource Limits
- Limit history size
- Restrict file system access in completions
- Timeout long-running operations

## Testing Strategy

### Unit Tests
- Test prompt creation and configuration
- Validate completion functionality
- Test key binding registration

### Integration Tests
- End-to-end prompt interaction testing
- Multi-mode prompt switching
- History persistence validation

### Performance Tests
- Large history handling
- Complex completion scenarios
- Memory usage monitoring

## Migration Strategy

### Phase 1: Core Enhancement
- Extend existing `TUIManager` with new methods
- Add basic completion support
- Implement history persistence

### Phase 2: Advanced Features
- Add syntax highlighting
- Implement custom key bindings
- Create plugin system

### Phase 3: Ecosystem Integration
- Integrate with context manager
- Add session management
- Create comprehensive examples

## Example Usage Scenarios

### 1. Interactive Script Development
```javascript
// Create a development prompt with JavaScript completion
var devPrompt = tui.createAdvancedPrompt({
    title: "JavaScript Development",
    completer: jsCompleter,
    lexer: jsLexer,
    history: { file: ".js_history" }
});

// Register development commands
tui.registerCommand({
    name: "run",
    handler: function(args) {
        var code = devPrompt.input();
        eval(code);
    }
});
```

### 2. File Management Interface
```javascript
// File operations with context-aware completion
tui.registerCompleter("files", function(doc) {
    var word = doc.getWordBeforeCursor();
    return context.getFilesByExt("*").filter(f =>
        f.toLowerCase().startsWith(word.toLowerCase())
    ).map(f => ({ text: f, description: "File" }));
});
```

### 3. Database Query Interface
```javascript
// SQL prompt with syntax highlighting
var sqlPrompt = tui.createAdvancedPrompt({
    prefix: "sql> ",
    lexer: sqlLexer,
    completer: sqlCompleter
});
```

## Implementation Roadmap and Timeline

### Phase 1: Foundation (Weeks 1-2)
**Goal**: Establish core infrastructure for enhanced go-prompt integration

**Tasks:**
1. **Extend TUIManager Structure**
   - Add prompt instance management
   - Implement configuration storage
   - Create JavaScript bridge foundations

2. **Basic API Exposure**
   - Expose core prompt methods to JavaScript
   - Add document state access
   - Implement basic text manipulation

3. **Configuration System**
   - Parse JavaScript configuration objects
   - Map to go-prompt options
   - Add validation and error handling

**Deliverables:**
- Enhanced TUIManager with multiple prompt support
- Basic JavaScript API for prompt manipulation
- Configuration parsing and validation

### Phase 2: Completion System (Weeks 3-4)
**Goal**: Implement intelligent completion using existing context manager

**Tasks:**
1. **Context Integration**
   - Create completion candidate provider from ContextManager
   - Implement file and path completion
   - Add command completion

2. **Completion API**
   - Expose completer registration to JavaScript
   - Implement completion filtering and ranking
   - Add async completion support

3. **Advanced Completion Features**
   - Fuzzy matching implementation
   - Completion history and learning
   - Multi-source completion aggregation

**Deliverables:**
- Context-aware file completion
- JavaScript completer registration API
- Fuzzy search and ranking system

### Phase 3: Advanced Features (Weeks 5-6)
**Goal**: Add syntax highlighting, key bindings, and advanced editing

**Tasks:**
1. **Syntax Highlighting**
   - Implement JavaScript lexer
   - Add token coloring and styling
   - Integrate with prompt rendering

2. **Key Binding System**
   - Expose key binding registration
   - Support custom key handlers
   - Add key binding modes (Emacs, Vi)

3. **Multi-line Editing**
   - Implement multi-line prompt support
   - Add line continuation logic
   - Enhanced editing capabilities

**Deliverables:**
- JavaScript syntax highlighting
- Custom key binding system
- Multi-line editing support

### Phase 4: Polish and Optimization (Weeks 7-8)
**Goal**: Performance optimization, testing, and documentation

**Tasks:**
1. **Performance Optimization**
   - Implement completion caching
   - Add debounced updates
   - Optimize rendering performance

2. **Comprehensive Testing**
   - Unit tests for all new functionality
   - Integration tests for end-to-end scenarios
   - Performance benchmarking

3. **Documentation and Examples**
   - Update API documentation
   - Create usage examples
   - Add migration guides

**Deliverables:**
- Performance-optimized implementation
- Comprehensive test suite
- Complete documentation and examples

## Success Criteria

### Functional Requirements
- [ ] All existing TUI functionality preserved
- [ ] Context-aware completion working
- [ ] JavaScript syntax highlighting active
- [ ] Custom key bindings functional
- [ ] Multi-line editing supported
- [ ] History persistence implemented

### Performance Requirements
- [ ] Completion response time < 100ms
- [ ] Memory usage increase < 50MB for large histories
- [ ] No degradation in existing functionality
- [ ] Smooth rendering at 60fps

### Quality Requirements
- [ ] Test coverage > 80% for new code
- [ ] No breaking changes to existing API
- [ ] Comprehensive error handling
- [ ] Full backward compatibility

## Risk Assessment and Mitigation

### Technical Risks
1. **go-prompt Version Compatibility**
   - *Risk*: Future updates may break integration
   - *Mitigation*: Pin version and create abstraction layer

2. **Performance Impact**
   - *Risk*: Advanced features may slow down simple usage
   - *Mitigation*: Lazy loading and feature flags

3. **JavaScript API Complexity**
   - *Risk*: Overwhelming API surface for users
   - *Mitigation*: Progressive disclosure and good defaults

### Project Risks
1. **Scope Creep**
   - *Risk*: Feature requests expand beyond timeline
   - *Mitigation*: Strict prioritization and phased delivery

2. **Testing Complexity**
   - *Risk*: TUI testing is challenging
   - *Mitigation*: Invest in robust testing infrastructure early

## Dependencies and Prerequisites

### External Dependencies
- `github.com/elk-language/go-prompt v1.3.1` (already included)
- No additional dependencies required

### Internal Dependencies
- ContextManager for completion candidates
- Logging system for interaction tracking
- Existing JavaScript bridge infrastructure

## Monitoring and Metrics

### Key Metrics to Track
1. **Usage Metrics**
   - Completion usage frequency
   - Key binding adoption
   - Mode switching patterns

2. **Performance Metrics**
   - Completion response times
   - Memory usage trends
   - Rendering performance

3. **Quality Metrics**
   - Error rates in prompt interactions
   - Test failure rates
   - User-reported issues

## Conclusion

This comprehensive integration plan transforms the basic go-prompt usage in one-shot-man into a sophisticated interactive scripting environment. By systematically exposing the full API surface and integrating with existing systems, we create a powerful platform for interactive script development.

The phased approach ensures:
- **Minimal Risk**: Gradual implementation with backward compatibility
- **Maximum Value**: Each phase delivers tangible user benefits
- **Sustainable Development**: Proper testing and documentation throughout

The result will be a state-of-the-art interactive scripting environment that leverages the full power of go-prompt while maintaining the simplicity and reliability of the existing system.

## Next Steps

1. **Planning Phase**: Review and approve implementation plan
2. **Kickoff**: Set up development environment and baseline metrics
3. **Phase 1 Start**: Begin foundation implementation
4. **Regular Reviews**: Weekly progress reviews and adjustments
5. **Beta Testing**: Internal testing before full release

This integration represents a significant enhancement to the one-shot-man platform, positioning it as a leading interactive scripting environment with rich terminal capabilities.
