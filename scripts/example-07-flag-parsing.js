#!/usr/bin/env osm script

// Flag Parsing Demo - demonstrates osm:flag for structured argument parsing.
//
// Run: osm script scripts/example-07-flag-parsing.js -- --name world --count 3 --verbose
//
// This script demonstrates:
//   1. Creating a FlagSet and defining typed flags (string, int, bool, float64)
//   2. Parsing arguments with error handling
//   3. Accessing parsed values via get()
//   4. Inspecting remaining (positional) arguments via args()
//   5. Flag introspection: lookup(), nArg(), nFlag(), defaults()
//   6. Iterating flags with visit() and visitAll()

var flag = require('osm:flag');

// --- 1. Create FlagSet and define flags ---

output.print("=== Flag Parsing Demo ===\n");

var flags = flag.newFlagSet("demo")
    .string("name", "osm", "Your name")
    .int("count", 1, "Number of repetitions")
    .bool("verbose", false, "Enable verbose output")
    .float64("threshold", 0.75, "Confidence threshold");

// --- 2. Parse arguments ---

// In real scripts, use the global `args` array. For this demo,
// we'll parse args if provided, or use defaults if not.
var parseArgs = (typeof args !== 'undefined' && args.length > 0) ? args : [];
var result = flags.parse(parseArgs);

if (result.error !== null) {
    output.printf("Parse error: %s\n", result.error);
    output.print("Usage:");
    output.print(flags.defaults());
} else {
    // --- 3. Access parsed values ---

    var name = flags.get("name");
    var count = flags.get("count");
    var verbose = flags.get("verbose");
    var threshold = flags.get("threshold");

    output.printf("name:      %s", name);
    output.printf("count:     %d", count);
    output.printf("verbose:   %s", verbose);
    output.printf("threshold: %s", threshold);

    // --- 4. Positional arguments ---

    output.printf("\nPositional args (%d):", flags.nArg());
    var remaining = flags.args();
    for (var i = 0; i < remaining.length; i++) {
        output.printf("  [%d] %s", i, remaining[i]);
    }

    // --- 5. Flag introspection ---

    output.printf("\nFlags set: %d", flags.nFlag());

    // lookup() returns metadata for a single flag
    var info = flags.lookup("name");
    if (info !== null) {
        output.printf("\nlookup('name'):");
        output.printf("  name:     %s", info.name);
        output.printf("  usage:    %s", info.usage);
        output.printf("  defValue: %s", info.defValue);
        output.printf("  value:    %s", info.value);
    }

    // lookup() returns null for unknown flags
    var unknown = flags.lookup("nonexistent");
    output.printf("lookup('nonexistent'): %s", unknown);

    // get() returns undefined for unknown flags
    var undef = flags.get("nonexistent");
    output.printf("get('nonexistent'): %s", undef);

    // defaults() returns the auto-generated usage text
    output.print("\n--- Usage (from defaults()) ---");
    output.print(flags.defaults());

    // --- 6. visit() and visitAll() ---

    if (verbose) {
        output.print("--- Flags set by user (visit) ---");
        flags.visit(function(f) {
            output.printf("  --%s = %s (default: %s) — %s", f.name, f.value, f.defValue, f.usage);
        });

        output.print("\n--- All defined flags (visitAll) ---");
        flags.visitAll(function(f) {
            output.printf("  --%s = %s (default: %s) — %s", f.name, f.value, f.defValue, f.usage);
        });
    }
}

output.print("\nDone.");
