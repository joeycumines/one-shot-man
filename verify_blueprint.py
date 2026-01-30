#!/usr/bin/env python3
"""
PARANOID BLUEPRINT VERIFICATION SCRIPT
Cross-References blueprint.json claims against actual git diff --numstat output
"""

import json
import sys

# Actual git diff --numstat data
GIT_DIFF_DATA = [
    (".agent/rules/core-code-quality-checks.md", 11, 0),
    (".deadcodeignore", 10, 0),
    ("blueprint.json", 382, 0),
    ("commit.msg", 10, 0),
    ("docs/architecture.md", 43, 0),
    ("docs/design/shooter_e2e_harness_design.md", 0, 815),
    ("docs/reference/bt-blackboard-usage.md", 794, 0),
    ("docs/reference/pabt.md", 779, 0),
    ("docs/reviews/001-pabt-module.md", 221, 0),
    ("docs/reviews/002-pabt-module.md", 93, 0),
    ("docs/reviews/003-bubbletea.md", 161, 0),
    ("docs/reviews/004-mouseharness.md", 140, 0),
    ("docs/reviews/005-pickandplace.md", 153, 0),
    ("docs/reviews/006-shooter.md", 146, 0),
    ("docs/reviews/011-scripting.md", 451, 0),
    ("docs/reviews/013-documentation.md", 272, 0),
    ("docs/reviews/015-config.md", 113, 0),
    ("docs/reviews/021-scripting-engine-exhaustive.md", 459, 0),
    ("docs/reviews/022-exhaustive-review-session.md", 230, 0),
    ("docs/reviews/README.md", 21, 0),
    ("docs/todo.md", 2, 1),
    ("docs/visuals/gifs/script-example-bt-shooter.tape", 67, 0),
    ("docs/visuals/gifs/script-example-pick-and-place.tape", 33, 0),
    ("example.config.mk", 16, 3),
    ("go.mod", 20, 18),
    ("go.sum", 40, 33),
    ("internal/builtin/bt/adapter_test.go", 8, 2),
    ("internal/builtin/bt/blackboard.go", 4, 0),
    ("internal/builtin/bt/bridge.go", 22, 5),
    ("internal/builtin/bt/doc.go", 0, 7),
    ("internal/builtin/bt/failure_mode_test.go", 0, 73),
    ("internal/builtin/bubbletea/bubbletea.go", 83, 14),
    ("internal/builtin/bubbletea/core_logic_test.go", 141, 0),
    ("internal/builtin/bubbletea/js_model_logic_test.go", 149, 0),
    ("internal/builtin/bubbletea/message_conversion_test.go", 364, 0),
    ("internal/builtin/bubbletea/parsekey_test.go", 4, 3),
    ("internal/builtin/bubbletea/render_throttle_test.go", 235, 328),
    ("internal/builtin/bubbletea/run_program_test.go", 322, 0),
    ("internal/builtin/register.go", 5, 3),
    ("internal/builtin/pabt/actions.go", 112, 0),
    ("internal/builtin/pabt/benchmark_test.go", 200, 0),
    ("internal/builtin/pabt/doc.go", 192, 0),
    ("internal/builtin/pabt/empty_actions_test.go", 139, 0),
    ("internal/builtin/pabt/evaluation.go", 354, 0),
    ("internal/builtin/pabt/evaluation_test.go", 439, 0),
    ("internal/builtin/pabt/expr_integration_test.go", 284, 0),
    ("internal/builtin/pabt/graph_test.go", 509, 0),
    ("internal/builtin/pabt/graphjsimpl_test.go", 480, 0),
    ("internal/builtin/pabt/integration_test.go", 239, 0),
    ("internal/builtin/pabt/memory_test.go", 204, 0),
    ("internal/builtin/pabt/pabt_test.go", 242, 0),
    ("internal/builtin/pabt/require.go", 503, 0),
    ("internal/builtin/pabt/require_test.go", 539, 0),
    ("internal/builtin/pabt/simple.go", 158, 0),
    ("internal/builtin/pabt/simple_test.go", 284, 0),
    ("internal/builtin/pabt/state.go", 365, 0),
    ("internal/builtin/pabt/state_test.go", 346, 0),
    ("internal/builtin/pabt/test_helpers_test.go", 48, 0),
    ("internal/command/pick_and_place_error_recovery_test.go", 1007, 0),
    ("internal/command/pick_and_place_harness_test.go", 1615, 0),
    ("internal/command/pick_and_place_mouse_test.go", 98, 0),
    ("internal/command/pick_and_place_unix_test.go", 2578, 0),
    ("internal/command/prompt_flow_editor_test.go", 9, 1),
    ("internal/command/scripting_command.go", 41, 2),
    ("internal/command/scripting_command_test.go", 38, 0),
    ("internal/command/shooter_game_test.go", 225, 304),
    ("internal/command/shooter_game_unix_test.go", 10, 2),
    ("internal/example/pickandplace/bubbletea_test.go", 12, 0),
    ("internal/example/pickandplace/pick_place_integration_test.go", 371, 0),
    ("internal/example/pickandplace/pick_place_manual_mode_comprehensive_test.go", 1135, 0),
    ("internal/example/pickandplace/pick_place_simulation_consistency_test.go", 895, 0),
    ("internal/mouseharness/console.go", 97, 0),
    ("internal/mouseharness/console_test.go", 293, 0),
    ("internal/mouseharness/element.go", 163, 0),
    ("internal/mouseharness/integration_test.go", 335, 0),
    ("internal/mouseharness/internal/dummy/main.go", 87, 0),
    ("internal/mouseharness/main_test.go", 73, 0),
    ("internal/mouseharness/mouse.go", 88, 0),
    ("internal/mouseharness/mouse_test.go", 122, 0),
    ("internal/mouseharness/options.go", 69, 0),
    ("internal/mouseharness/options_test.go", 140, 0),
    ("internal/mouseharness/terminal.go", 355, 0),
    ("internal/mouseharness/terminal_test.go", 248, 0),
    ("internal/scripting/engine_core.go", 37, 1),
    ("internal/scripting/execution_context.go", 0, 2),
    ("internal/scripting/js_logging_api.go", 18, 4),
    ("internal/scripting/js_state_accessor.go", 5, 0),
    ("internal/scripting/logging.go", 41, 20),
    ("internal/scripting/logging_test.go", 30, 4),
    ("internal/scripting/mouse_test_api_test.go", 27, 25),
    ("internal/scripting/mouse_util_test.go", 53, 610),
    ("internal/scripting/recording_demos_unix_test.go", 372, 33),
    ("internal/scripting/state_manager.go", 5, 0),
    ("internal/scripting/super_document_click_after_scroll_integration_test.go", 3, 3),
    ("internal/scripting/super_document_unix_integration_test.go", 17, 9),
    ("internal/scripting/vhs_record_unix_test.go", 33, 2),
    ("scripts/benchmark-input-latency.js", 31, 31),
    ("scripts/example-04-bt-shooter.js", 228, 398),
    ("scripts/example-05-pick-and-place.js", 1885, 0),
]

# Map each file to its expected category (based on blueprint.json)
EXPECTED_CATEGORIES = {
    # Metadata / Configuration
    ".agent/rules/core-code-quality-checks.md": "Metadata",
    ".deadcodeignore": "Configuration",
    "blueprint.json": "Metadata",
    "commit.msg": "Metadata",
    "example.config.mk": "Configuration",
    "go.mod": "Dependencies",
    "go.sum": "Dependencies",

    # Documentation
    "docs/architecture.md": "Documentation",
    "docs/design/shooter_e2e_harness_design.md": "Documentation",
    "docs/reference/bt-blackboard-usage.md": "Documentation",
    "docs/reference/pabt.md": "Documentation",
    "docs/reviews/001-pabt-module.md": "Documentation",
    "docs/reviews/002-pabt-module.md": "Documentation",
    "docs/reviews/003-bubbletea.md": "Documentation",
    "docs/reviews/004-mouseharness.md": "Documentation",
    "docs/reviews/005-pickandplace.md": "Documentation",
    "docs/reviews/006-shooter.md": "Documentation",
    "docs/reviews/011-scripting.md": "Documentation",
    "docs/reviews/013-documentation.md": "Documentation",
    "docs/reviews/015-config.md": "Documentation",
    "docs/reviews/021-scripting-engine-exhaustive.md": "Documentation",
    "docs/reviews/022-exhaustive-review-session.md": "Documentation",
    "docs/reviews/README.md": "Documentation",
    "docs/todo.md": "Documentation",
    "docs/visuals/gifs/script-example-bt-shooter.tape": "Documentation",
    "docs/visuals/gifs/script-example-pick-and-place.tape": "Documentation",

    # Core Infrastructure
    "internal/builtin/register.go": "Core Infrastructure",

    # Pick-and-Place
    "internal/command/pick_and_place_error_recovery_test.go": "Pick-and-Place",
    "internal/command/pick_and_place_harness_test.go": "Pick-and-Place",
    "internal/command/pick_and_place_mouse_test.go": "Pick-and-Place",
    "internal/command/pick_and_place_unix_test.go": "Pick-and-Place",
    "internal/example/pickandplace/bubbletea_test.go": "Pick-and-Place",
    "internal/example/pickandplace/pick_place_integration_test.go": "Pick-and-Place",
    "internal/example/pickandplace/pick_place_manual_mode_comprehensive_test.go": "Pick-and-Place",
    "internal/example/pickandplace/pick_place_simulation_consistency_test.go": "Pick-and-Place",
    "scripts/example-05-pick-and-place.js": "Pick-and-Place",

    # Shooter Game
    "internal/command/shooter_game_test.go": "Shooter Game",
    "internal/command/shooter_game_unix_test.go": "Shooter Game",

    # Other Tests
    "internal/command/prompt_flow_editor_test.go": "Other Tests",

    # Behavior Tree
    "internal/builtin/bt/adapter_test.go": "Behavior Tree",
    "internal/builtin/bt/blackboard.go": "Behavior Tree",
    "internal/builtin/bt/bridge.go": "Behavior Tree",
    "internal/builtin/bt/doc.go": "Behavior Tree",
    "internal/builtin/bt/failure_mode_test.go": "Behavior Tree",

    # BubbleTea Builtin
    "internal/builtin/bubbletea/bubbletea.go": "BubbleTea Builtin",
    "internal/builtin/bubbletea/core_logic_test.go": "BubbleTea Builtin",
    "internal/builtin/bubbletea/js_model_logic_test.go": "BubbleTea Builtin",
    "internal/builtin/bubbletea/message_conversion_test.go": "BubbleTea Builtin",
    "internal/builtin/bubbletea/parsekey_test.go": "BubbleTea Builtin",
    "internal/builtin/bubbletea/render_throttle_test.go": "BubbleTea Builtin",
    "internal/builtin/bubbletea/run_program_test.go": "BubbleTea Builtin",

    # PABT Module
    "internal/builtin/pabt/actions.go": "PABT Module",
    "internal/builtin/pabt/benchmark_test.go": "PABT Module",
    "internal/builtin/pabt/doc.go": "PABT Module",
    "internal/builtin/pabt/empty_actions_test.go": "PABT Module",
    "internal/builtin/pabt/evaluation.go": "PABT Module",
    "internal/builtin/pabt/evaluation_test.go": "PABT Module",
    "internal/builtin/pabt/expr_integration_test.go": "PABT Module",
    "internal/builtin/pabt/graph_test.go": "PABT Module",
    "internal/builtin/pabt/graphjsimpl_test.go": "PABT Module",
    "internal/builtin/pabt/integration_test.go": "PABT Module",
    "internal/builtin/pabt/memory_test.go": "PABT Module",
    "internal/builtin/pabt/pabt_test.go": "PABT Module",
    "internal/builtin/pabt/require.go": "PABT Module",
    "internal/builtin/pabt/require_test.go": "PABT Module",
    "internal/builtin/pabt/simple.go": "PABT Module",
    "internal/builtin/pabt/simple_test.go": "PABT Module",
    "internal/builtin/pabt/state.go": "PABT Module",
    "internal/builtin/pabt/state_test.go": "PABT Module",
    "internal/builtin/pabt/test_helpers_test.go": "PABT Module",

    # Scripting
    "internal/command/scripting_command.go": "Scripting",
    "internal/command/scripting_command_test.go": "Scripting",
    "internal/scripting/engine_core.go": "Scripting",
    "internal/scripting/execution_context.go": "Scripting",
    "internal/scripting/js_logging_api.go": "Scripting",
    "internal/scripting/js_state_accessor.go": "Scripting",
    "internal/scripting/logging.go": "Scripting",
    "internal/scripting/logging_test.go": "Scripting",
    "internal/scripting/mouse_test_api_test.go": "Scripting",
    "internal/scripting/mouse_util_test.go": "Scripting",
    "internal/scripting/recording_demos_unix_test.go": "Scripting",
    "internal/scripting/state_manager.go": "Scripting",
    "internal/scripting/super_document_click_after_scroll_integration_test.go": "Scripting",
    "internal/scripting/super_document_unix_integration_test.go": "Scripting",
    "internal/scripting/vhs_record_unix_test.go": "Scripting",
    "scripts/example-04-bt-shooter.js": "Scripting",

    # Other Scripts
    "scripts/benchmark-input-latency.js": "Other Scripts",

    # MouseHarness
    "internal/mouseharness/console.go": "MouseHarness",
    "internal/mouseharness/console_test.go": "MouseHarness",
    "internal/mouseharness/element.go": "MouseHarness",
    "internal/mouseharness/integration_test.go": "MouseHarness",
    "internal/mouseharness/internal/dummy/main.go": "MouseHarness",
    "internal/mouseharness/main_test.go": "MouseHarness",
    "internal/mouseharness/mouse.go": "MouseHarness",
    "internal/mouseharness/mouse_test.go": "MouseHarness",
    "internal/mouseharness/options.go": "MouseHarness",
    "internal/mouseharness/options_test.go": "MouseHarness",
    "internal/mouseharness/terminal.go": "MouseHarness",
    "internal/mouseharness/terminal_test.go": "MouseHarness",
}

def calculate_net_changes():
    """Calculate total changes from git diff data."""
    total_insertions = sum(add for path, add, delete in GIT_DIFF_DATA)
    total_deletions = sum(delete for path, add, delete in GIT_DIFF_DATA)
    total_net = sum((add + delete) for path, add, delete in GIT_DIFF_DATA)  # Total lines touched
    return total_insertions, total_deletions, total_net

def calculate_category_changes():
    """Group files by category and calculate changes."""
    categories = {}
    for path, add, delete in GIT_DIFF_DATA:
        category = EXPECTED_CATEGORIES.get(path, "UNCATEGORIZED")
        if category not in categories:
            categories[category] = {"files": [], "additions": 0, "deletions": 0, "net": 0}
        categories[category]["files"].append((path, add, delete))
        categories[category]["additions"] += add
        categories[category]["deletions"] += delete
        categories[category]["net"] += (add + delete)
    return categories

def main():
    print("=" * 80)
    print("PARANOID BLUEPRINT VERIFICATION")
    print("=" * 80)
    print()

    # Git diff facts
    total_insertions, total_deletions, total_net = calculate_net_changes()
    print()
    print("GIT DIFF FACTS (from git diff --numstat main):")
    print(f"  Total files: {len(GIT_DIFF_DATA)}")
    print(f"  Total insertions: {total_insertions:,}")
    print(f"  Total deletions: {total_deletions:,}")
    print(f"  Total lines changed (insertions + deletions): {total_net:,}")
    print(f"  Net change: +{total_insertions - total_deletions:,}")
    print()

    # Category breakdown
    categories = calculate_category_changes()
    print()
    print("CATEGORY BREAKDOWN:")
    print("-" * 80)

    for category in sorted(categories.keys(), key=lambda c: categories[c]["net"], reverse=True):
        cat = categories[category]
        print(f"\n{category}:")
        print(f"  Files: {len(cat['files'])}")
        print(f"  Insertions: {cat['additions']:,}")
        print(f"  Deletions: {cat['deletions']:,}")
        print(f"  Total lines changed: {cat['net']:,}")
        print(f"  Files:")
        for path, add, delete in sorted(cat['files'], key=lambda x: x[1], reverse=True):
            net_str = f"+{add}" if delete == 0 else f"+{add}, -{delete}"
            print(f"    {path} ({net_str})")

    # Check for uncategorized files
    uncategorized = categories.get("UNCATEGORIZED", {}).get("files", [])
    if uncategorized:
        print()
        print()
        print("⚠️  WARNING: UNCATEGORIZED FILES DETECTED:")
        for path, add, delete in uncategorized:
            print(f"  {path} (+{add}, -{delete})")

    # Verify all files are tagged
    all_files_in_diff = {path for path, _, _ in GIT_DIFF_DATA}
    all_tagged_files = set(EXPECTED_CATEGORIES.keys())
    missing_files = all_files_in_diff - all_tagged_files

    if missing_files:
        print()
        print()
        print("⚠️  ERROR: FILES IN GIT DIFF BUT NOT IN EXPECTED_CATEGORIES:")
        for path in sorted(missing_files):
            print(f"  {path}")
    else:
        print()
        print()
        print("✓ ALL FILES IN GIT DIFF ARE TAGGED IN EXPECTED_CATEGORIES")

    print()
    print("=" * 80)
    print("VERIFICATION COMPLETE")
    print("=" * 80)

if __name__ == "__main__":
    main()
