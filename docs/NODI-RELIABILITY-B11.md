# B11 public command help

Stacked on B09 / PR #11. All public commands accept `-h` and `--help` before action/config/network dispatch, including auth login/logout, setup --completion and completion SHELL. Output contains command usage/options. Unknown commands/options/extra arguments still fail. `--` retains literal path/argument semantics; an option value is not interpreted as help.

RED on B09: all 40 public-command/help combinations fail. GREEN: all 40 succeed with no input/config/store/cwd/open/connect access; absent config remains absent. Unknown option/command/subcommand/shell and literal-help cases fail; help before/after positional arguments succeeds. Full Windows tests/vet PASS; [evidence](reliability/batch/b11). Native CI checks the same suite. Existing root help/version and completion formats remain compatible. No public method signatures or manifest changes.
