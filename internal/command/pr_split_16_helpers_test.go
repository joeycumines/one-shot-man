package command

// ---------------------------------------------------------------------------
//  T020: Comprehensive keyboard & mouse event handling tests for chunk 16
//
//  Covers: overlays (report, editor dialogs, Claude conversation, inline
//  title edit), live verify session, split-view, all mouse zone clicks,
//  focus activation, plan editor keys, navigation handlers, and edge cases.
//
//  Does NOT duplicate tests already in pr_split_13_tui_test.go (help toggle,
//  ctrl+c, confirm cancel y/n/esc/enter, WindowSize, j/k navigation in
//  PLAN_REVIEW, esc back, plan editor shortcut 'e', mouse wheel scroll,
//  msg.string regression, AllKeyBindingsRespond).
// ---------------------------------------------------------------------------


