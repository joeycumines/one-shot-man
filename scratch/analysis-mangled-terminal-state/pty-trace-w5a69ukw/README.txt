PTY trace

events-*.jsonl contains timestamped records. Each io record has a direction and base64-encoded raw bytes: to_command is input written to the command's PTY; from_command is output read from it. Resize and process lifecycle events are recorded in the same sequence.

Direct mode runs one command in a PTY. Shim mode prepends a private PATH directory so an outer command can launch the selected command through this recorder.

This records observable PTY traffic, not a command's internal state or an outer terminal emulator's in-memory cell snapshots; raw output can be replayed to reconstruct terminal state.

Traces can contain private input and output. Review before sharing. Only terminal/locale environment values and command option names are recorded as metadata; full arguments and other environment values are not.
