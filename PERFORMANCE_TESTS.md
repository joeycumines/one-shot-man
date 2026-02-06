# Performance Tests

This document describes the performance regression tests for one-shot-man.

## Overview

The performance tests are located in `internal/benchmark_test.go` and provide benchmarks and regression tests for critical code paths in the one-shot-man project.

## Running Performance Tests

### Benchmarks

Run benchmarks with:
```bash
go test -bench=. -benchmem ./internal/
```

### Regression Tests

Run regression tests with:
```bash
go test -run TestPerformanceRegression ./internal/
go test -run TestMemoryUsageRegression ./internal/
```

Run in short mode (skips slow tests):
```bash
go test -short ./internal/
```

## Benchmarks

### Session Operations (`BenchmarkSessionOperations`)

- **SessionIDGeneration**: Measures the performance of session ID generation
- **SessionCreation**: Measures session struct allocation
- **SessionPersistenceWrite**: Measures session save operations to in-memory backend
- **SessionPersistenceRead**: Measures session load operations from in-memory backend
- **ConcurrentSessionAccess**: Measures thread-safe concurrent session access
- **SessionJSONMarshaling**: Measures JSON serialization of sessions with 100 history entries
- **SessionJSONUnmarshaling**: Measures JSON deserialization of sessions

### Configuration Loading (`BenchmarkConfigLoading`)

- **NewConfig**: Measures default config creation
- **ConfigOptionGet**: Measures config option retrieval
- **ConfigOptionSet**: Measures config option setting
- **ConfigLoadFromReader**: Measures config loading from reader
- **ConfigPathResolution**: Measures config path lookup
- **ConfigValidation**: Measures config validation operations

### Scripting Engine (`BenchmarkScriptingEngine`)

- **VMCreation**: Measures goja.Runtime VM allocation
- **RuntimeCreation**: Measures full scripting.Runtime creation with event loop
- **GlobalRegistration**: Measures global variable registration in VM
- **SimpleScriptExecution**: Measures script loading and execution
- **ScriptCompilation**: Measures JavaScript compilation
- **ScriptExecutionWithVM**: Measures complete VM + program execution

### Command Execution (`BenchmarkCommandExecution`)

- **CommandRegistration**: Measures command registry creation and registration

## Regression Tests

### TestPerformanceRegression

Validates that critical operations complete within acceptable thresholds:

| Operation | Threshold (μs) |
|-----------|-----------------|
| Session ID Generation | 100 |
| Session Persistence Write | 1000 |
| Session Persistence Read | 500 |
| Scripting Runtime Creation | 50000 |
| Script Execution | 1000 |

### TestMemoryUsageRegression

Tests for memory leaks in critical operations:

- **RuntimeCreationNoLeak**: Verifies runtime creation doesn't leak memory
- **SessionBackendNoLeak**: Verifies session backend operations don't leak memory

## Performance Thresholds

Performance thresholds are defined as constants in `internal/benchmark_test.go`:

```go
const (
    // Session operation thresholds (microseconds)
    thresholdSessionIDGeneration     = 100
    thresholdSessionCreation         = 500
    thresholdSessionPersistenceWrite = 1000
    thresholdSessionPersistenceRead  = 500
    thresholdConcurrentSessionAccess = 2000

    // Scripting engine thresholds (microseconds)
    thresholdRuntimeCreation    = 50000
    thresholdGlobalRegistration = 100
    thresholdSimpleScriptExec   = 1000
    thresholdVMCreation         = 5000
)
```

## Interpreting Results

### Benchmark Output

Benchmark results show operations per second (ops/sec) and memory allocation:

```
BenchmarkSessionOperations/SessionIDGeneration-8    1234567    98.5 ns/op    0 B/op    0 allocs/op
```

- `-8`: GOMAXPROCS value
- `1234567`: Operations per second
- `98.5 ns/op`: Nanoseconds per operation
- `0 B/op`: Bytes allocated per operation
- `0 allocs/op`: Allocations per operation

### Regression Test Output

Passing tests log performance metrics:

```
=== RUN   TestPerformanceRegression/SessionIDGeneration
    benchmark_test.go:123: Session ID generation: avg 45 μs (threshold: 100 μs)
--- PASS: TestPerformanceRegression/SessionIDGeneration (0.00s)
```

Failing tests report threshold violations:

```
=== RUN   TestPerformanceRegression/SessionIDGeneration
    benchmark_test.go:123: Session ID generation: avg 145 μs (threshold: 100 μs)
    benchmark_test.go:124: FAIL: Session ID generation too slow: avg 145 μs (threshold: 100 μs)
--- FAIL: TestPerformanceRegression/SessionIDGeneration (0.00s)
```

## Best Practices

1. **Run benchmarks** after changes to critical paths
2. **Monitor trends** over time, not just pass/fail
3. **Consider system load** when interpreting results
4. **Update thresholds** when optimizing or as hardware improves
5. **Run with `-short`** for quick sanity checks during development

## CI/CD Integration

Performance tests run as part of the full test suite:

```bash
make test        # Run all tests
make lint        # Run static analysis (includes performance tests)
```

## Notes

- Benchmarks use `testing.B` and report allocations with `b.ReportAllocs()`
- Regression tests skip in short mode with `testing.Short()`
- Memory tests use `runtime.MemStats` with garbage collection before measurements
- Concurrent benchmarks use `b.RunParallel()` for parallel execution
