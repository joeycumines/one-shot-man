#!/usr/bin/env osm script

// API Client Demo - demonstrates osm:fetch for HTTP requests.
//
// Run: osm script scripts/example-06-api-client.js
//
// This script demonstrates:
//   1. Simple GET request with response parsing
//   2. POST request with JSON body and custom headers
//   3. Streaming response with fetchStream (line-by-line reading)
//   4. Error handling for network failures
//   5. Timeout configuration

var http = require('osm:fetch');

// --- 1. Simple GET ---

output.print("=== 1. Simple GET Request ===");
try {
    var resp = http.fetch("https://httpbin.org/get?demo=osm");
    output.printf("Status: %d (%s)", resp.status, resp.statusText);
    output.printf("OK: %s", resp.ok);
    output.printf("Final URL: %s", resp.url);

    // Parse JSON response
    var data = resp.json();
    output.printf("Query param 'demo': %s", data.args.demo);
    output.printf("User-Agent: %s", data.headers["User-Agent"]);
} catch (e) {
    output.printf("GET failed (expected if offline): %s", e.message || e);
}

output.print("");

// --- 2. POST with JSON body ---

output.print("=== 2. POST with JSON Body ===");
try {
    var payload = JSON.stringify({ name: "osm", version: "0.1.0" });
    var resp = http.fetch("https://httpbin.org/post", {
        method: "POST",
        headers: {
            "Content-Type": "application/json",
            "X-Custom-Header": "osm-demo"
        },
        body: payload,
        timeout: 10  // seconds
    });
    output.printf("Status: %d", resp.status);

    var data = resp.json();
    output.printf("Echoed body: %s", data.data);
    output.printf("Content-Type received: %s", data.headers["Content-Type"]);
    output.printf("X-Custom-Header: %s", data.headers["X-Custom-Header"]);
} catch (e) {
    output.printf("POST failed (expected if offline): %s", e.message || e);
}

output.print("");

// --- 3. Streaming response ---

output.print("=== 3. Streaming Response ===");
try {
    // httpbin.org/stream/N returns N JSON lines
    var stream = http.fetchStream("https://httpbin.org/stream/3", {
        timeout: 15
    });
    output.printf("Stream status: %d", stream.status);

    var lineCount = 0;
    while (true) {
        var line = stream.readLine();
        if (line === null) break;
        if (line === '') continue;
        lineCount++;
        // Parse each line as JSON
        var obj = JSON.parse(line);
        output.printf("  Line %d: id=%d, url=%s", lineCount, obj.id, obj.url);
    }
    stream.close();
    output.printf("Total lines: %d", lineCount);
} catch (e) {
    output.printf("Stream failed (expected if offline): %s", e.message || e);
}

output.print("");

// --- 4. Error handling ---

output.print("=== 4. Error Handling ===");

// 4a. HTTP error status (not a JS exception — check resp.ok)
try {
    var resp = http.fetch("https://httpbin.org/status/404");
    output.printf("Status: %d, OK: %s", resp.status, resp.ok);
    if (!resp.ok) {
        output.print("  → HTTP error detected via resp.ok === false");
    }
} catch (e) {
    output.printf("Unexpected error: %s", e.message || e);
}

// 4b. Network error (throws exception)
try {
    http.fetch("http://192.0.2.1/unreachable", { timeout: 2 });
    output.print("  → Should not reach here");
} catch (e) {
    output.printf("  → Network error caught: %s", String(e).substring(0, 80));
}

output.print("");

// --- 5. Response headers ---

output.print("=== 5. Response Headers ===");
try {
    var resp = http.fetch("https://httpbin.org/response-headers?X-Demo=hello");
    output.printf("X-Demo header: %s", resp.headers["x-demo"]);
    output.printf("Content-Type: %s", resp.headers["content-type"]);
} catch (e) {
    output.printf("Headers demo failed (expected if offline): %s", e.message || e);
}

output.print("\nDone. All fetch API features demonstrated.");
