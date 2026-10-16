# Session Cleanup

An [Agent of Empires](https://github.com/agent-of-empires/agent-of-empires)
plugin that cleans up a session's worktree after the session is **trashed** or
**deleted**, according to a per-project list of paths. The typical use: delete
the `target` directory of a trashed session to reclaim disk space.

Needs an aoe with per-project plugin settings (`per_project` settings,
`api_version` 18 and the `project_path` parameter of `config.get`). On an older
host the per-project values are never seen, so nothing is cleaned.

## How it works

The worker listens to `session.lifecycle.changed`:

- **trash**: aoe marks the session trashed first and moves its worktree into
  `<worktrees>/.aoe-trash/<session-id>` afterwards (announced as
  `workspace_changed` with the new path). The plugin waits for that move and
  then cleans the *moved* worktree. Worktrees aoe does not move (a default
  branch's checkout, unmanaged worktrees) are never cleaned on trash. If the
  move does not happen within `relocation_timeout_secs` the cleanup is skipped.
- **delete**: the worktree at the path last seen for the session (or its trash
  holding path) is cleaned, if it still exists. aoe reports changes with a
  delay, so after a restore and a quick delete that path can be stale; the
  plugin then asks git's own bookkeeping (`<repo>/.git/worktrees/*`, read as
  files) which worktree has the session's branch checked out. The answer is
  used only if the worktree's own `.git` file points back at that admin entry
  (a main checkout, whose `.git` is a directory, never qualifies) and no other
  known session already works there.
- On start the worker also cleans sessions already in the trash
  (`sweep_on_start`), catching events missed while the daemon was down.

Only aoe-managed, single-repo worktree sessions are ever touched. Plain
sessions and multi-repo workspaces are ignored, and a directory without `.git`
is refused.

## Settings

Per project (set in the project's settings; falls back to the global value):

| Setting | Meaning |
|---|---|
| `paths` | Paths relative to the worktree root. Single-level globs (`target`, `crates/*/target`). No absolute paths, no `..`, `.git` is never touched, and matches whose parent resolves outside the worktree through a symlink are skipped. A project is cleaned only when this list contains something. |
| `on` | `trash`, `delete` or `both` (default). |

The project is the session's main repository, so it must be a registered
project.

Global:

| Setting | Meaning |
|---|---|
| `enabled` | Master toggle. |
| `sweep_on_start` | Also clean already-trashed sessions when the worker starts. |
| `relocation_timeout_secs` | How long to wait for aoe to move a trashed worktree. |

## Capabilities

`runtime.worker`, `session.read` (lifecycle events, `sessions.list`) and
`fs.read` / `fs.write` (the worktree lives outside the plugin directory).

## Install into Agent of Empires

```bash
aoe plugin install ./aoe-plugin-session-cleanup
```

## Layout

```
aoe-plugin.toml         manifest (settings, capabilities, runtime)
worker.go               JSON-RPC worker loop
rpc.go                  message shapes + response builders
hostrpc.go              outbound host calls (config.get, sessions.list) + inbound demux
model.go                session shapes, trash holding-path convention
gitworktree.go          find a worktree by branch from git admin files
settings.go             the per-project paths/on setting, parsed and matched
cleanup.go              path deletion, confined to the worktree
cleaner.go              lifecycle tracking and trigger logic
*_test.go               unit tests, plus a contract test driving the real binary
magefiles/              zero-install Mage build
```

## Develop

```bash
go run -C magefiles mage.go -w .. -l       # list targets
go run -C magefiles mage.go -w .. check    # fmt, vet, lint, test (default)
go run -C magefiles mage.go -w .. build    # compile the worker binary
go run -C magefiles mage.go -w .. testRace # test suite with the race detector
```

## License

MIT
