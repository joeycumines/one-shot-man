#!/usr/bin/env osm script -i

// Test script for initial-command integration test
// Registers a startup mode with an initialCommand that switches to a target mode

tui.registerMode({
    name: "target",
    tui: {prompt: "[target]> "}
});

tui.registerMode({
    name: "startup",
    initialCommand: "mode target",
    tui: {prompt: "[startup]> "}
});

// Activate the startup mode; the initialCommand should execute when the prompt starts
// causing the visible prompt to be the target mode prompt.
tui.switchMode("startup");
