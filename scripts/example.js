// Example JavaScript script demonstrating the deferred/declarative API
// Usage: one-shot-man script scripts/example.js

ctx.log("Starting example script");

// Demonstrate deferred execution (cleanup)
ctx.defer(function() {
    ctx.log("Cleaning up resources");
});

ctx.defer(function() {
    ctx.log("Saving final state");
});

// Demonstrate sub-tests similar to testing.T.run()
ctx.run("setup", function() {
    ctx.log("Setting up test environment");
    ctx.logf("Environment: %s", env("PATH") ? "defined" : "undefined");
    
    ctx.defer(function() {
        ctx.log("Cleaning up test environment");
    });
});

ctx.run("main_operations", function() {
    ctx.log("Performing main operations");
    
    ctx.run("database_test", function() {
        ctx.log("Testing database connection");
        // Simulate some work
        sleep(100);
        ctx.log("Database test completed");
    });
    
    ctx.run("api_test", function() {
        ctx.log("Testing API endpoints");
        
        // Test multiple endpoints
        var endpoints = ["health", "users", "data"];
        for (var i = 0; i < endpoints.length; i++) {
            ctx.run("endpoint_" + endpoints[i], function() {
                ctx.logf("Testing endpoint: %s", endpoints[i]);
                // Simulate API call
                sleep(50);
                ctx.log("Endpoint test passed");
            });
        }
    });
});

ctx.run("validation", function() {
    ctx.log("Running validation tests");
    
    // Access command line arguments
    if (args && args.length > 0) {
        ctx.logf("Script called with %d arguments", args.length);
        for (var i = 0; i < args.length; i++) {
            ctx.logf("  arg[%d]: %s", i, args[i]);
        }
    } else {
        ctx.log("No command line arguments provided");
    }
});

// Demonstrate console API
console.log("Script execution completed successfully");

ctx.log("Example script finished");