# TODO - NOT for AI, for people only

- Fix the command history serialisation to avoid encoding the json as a string pointlessly. Probably kill `internal/scripting/state_history.go` and norm on `internal/storage/schema.go` while at it.
- Make the shell prompt AND mode id of `code-review` be that e.g.`(code-review) >` (originally killed off because a REPL command `code-review` was annoying because it got in the way of tab completion of the copy command, which is no longer relevant)
- Make a breaking change while I still can - standardise the field names used to persist session state and other fields exposed as JSON
- Consider making commands and subcommands correctly fail upon receiving unexpected arguments or options
