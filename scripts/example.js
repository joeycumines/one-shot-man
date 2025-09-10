// Example JavaScript script demonstrating the deferred/declarative API
// Usage: one-shot-man script scripts/example.js

ctx.Log("Starting example script");

// Demonstrate deferred execution (cleanup)
ctx.Defer(function() {
    ctx.Log("Cleaning up resources");
});

ctx.Defer(function() {
    ctx.Log("Saving final state");
});

// Demonstrate sub-tests similar to testing.T.Run()
ctx.Run("setup", function() {
    ctx.Log("Setting up test environment");
    ctx.Logf("Environment: %s", env("PATH") ? "defined" : "undefined");
    
    ctx.Defer(function() {
        ctx.Log("Cleaning up test environment");
    });
});

ctx.Run("main_operations", function() {
    ctx.Log("Performing main operations");
    
    ctx.Run("database_test", function() {
        ctx.Log("Testing database connection");
        // Simulate some work
        sleep(100);
        ctx.Log("Database test completed");
    });
    
    ctx.Run("api_test", function() {
        ctx.Log("Testing API endpoints");
        
        // Test multiple endpoints
        var endpoints = ["health", "users", "data"];
        for (var i = 0; i < endpoints.length; i++) {
            ctx.Run("endpoint_" + endpoints[i], function() {
                ctx.Logf("Testing endpoint: %s", endpoints[i]);
                // Simulate API call
                sleep(50);
                ctx.Log("Endpoint test passed");
            });
        }
    });
});

ctx.Run("validation", function() {
    ctx.Log("Running validation tests");
    
    // Access command line arguments
    if (args && args.length > 0) {
        ctx.Logf("Script called with %d arguments", args.length);
        for (var i = 0; i < args.length; i++) {
            ctx.Logf("  arg[%d]: %s", i, args[i]);
        }
    } else {
        ctx.Log("No command line arguments provided");
    }
});

// Demonstrate console API
console.log("Script execution completed successfully");

ctx.Log("Example script finished");