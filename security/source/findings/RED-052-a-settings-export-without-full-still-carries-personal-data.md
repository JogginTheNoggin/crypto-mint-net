# RED-052: A settings export without `-full` still carries personal data

| Field | Value |
| --- | --- |
| Severity | Medium |
| Status | fixed |
| Found | Source Round 20 Part B, on `main` @ `522e891` (D155) |
| Area | `pkg/nodepolicy` (`ExportSettings`, `Strip`, `MachineLocal`), `pkg/cli` (`settings export`) |
| Raised by | Red, and Source Green (GREEN-045, PR #111) saw the file paths |
| For Black | No consensus rule is touched. |
| Fixed by | `claude/project-thread-ggo8dt` @ 63e37ed (BLUE-021) |

## What is wrong

D155 and `docs/FEATURES.md` say a settings file holds "nothing
personal": no key, seed, passphrase or token, and machine-local
settings only with `-full`. That is true of keys and tokens, but node.json
holds other things that identify the owner or open a door, and
`MachineLocal` (`rpc`, `no_sync`, `p2p.listen`, `p2p.advertise`,
`grind.device`, `grind.threads`, `grind.jobs.device`,
`grind.jobs.threads`) does not list them. A plain `poon settings export`
(and `poon policy export`) writes all of these:

| In the file | Why it is personal |
| --- | --- |
| `p2p.registries`, a local file such as `/home/alice/regs/mine.json` | The user name and the folder layout of the machine. The import already drops local files without `-full` (`Merge` keeps only http and https registries), but the export writes them. |
| `p2p.registries`, a URL such as `https://bob:hunter2@reg.example/list.json?key=SECRET` | A user name and password in the URL, or a key in the query. Nothing checks the URL for them. |
| `p2p.peers` | The addresses the owner stays connected to: a home or LAN address (`192.168.1.20:8646`), a friend's node. |
| `tracking.peers` | The same addresses again, each with the template the owner chose for it. |
| `grind.jobs[].target` and `until_from` | `tx:TID` and entry targets name the owner's own pending transaction. |

The `-full` help of `settings import` also lists "registry files" as
machine-local, but they are not in `MachineLocal`, so an export
never leaves them out.

## How I checked it

A node.json with the first four rows, then:

```
$ poon settings export | grep -n "alice\|hunter2\|SECRET\|192.168"
18:            "192.168.1.20:8646"
21:            "/home/alice/regs/mine.json",
22:            "https://bob:hunter2@reg.example/list.json?key=SECRET"
27:            "192.168.1.20:8646": "detailed"
```

`poon policy export` writes the same. The file is meant to be shared
("share a setup"), so this goes out with it.

## What could happen

- A user shares a setup file in a forum. It carries their user name, the
  password in a registry URL, and the addresses of their own machines.
- The preview of the person who imports it shows these as plain
  settings; nothing says the file holds somebody else's addresses.

## Suggested fix (Blue's call)

1. On export, leave out what is personal unless `-full`:
   local registry files (as the import drops them), and any registry URL
   with user information or a query, which is either dropped or listed
   in a note.
2. Treat `p2p.peers` and `tracking.peers` as addresses of the owner's
   choice: export them, but print what the file holds ("N peers, M
   registries") when it is written, so the user sees it before sharing.
3. Make the docs say what is in the file instead of "nothing
   personal", and fix the `-full` help to match `MachineLocal`.

If `MachineLocal` grows, `Strip`, `withMachine` and `machineSet` follow
it, so the registry rule belongs in one place with them.

## Not tested

Whether the GUI export form shows the same file. It uses the same code
(the GUI is a layer on the headless node), so I expect it does.

## Resolution

Fixed (D156).

- A plain export, `poon settings export` and `poon policy export` alike,
  leaves out what is personal (`nodepolicy.Personal`, in
  `ExportPolicy`, which `ExportSettings` uses). That covers three things:
  - a registry that is a local file;
  - a registry URL with a user, password, query or fragment;
  - a grind job on one payment or entry (`tx:TID` or an entry hash; its
    `until_from` goes with it).

  `-full` keeps them, with the machine-local settings.
- Peers and `tracking.peers` are exported, as Red suggested: a shared
  setup is made of them. The export says what the file holds and what
  it left out (`ExportNotes`), for example "left out 2 registries that
  are a local file or carry a user, password, query or fragment (-full
  keeps them)" and "the file holds 1 peer address (p2p.peers), 1 tracked
  peer (tracking.peers): whoever reads it sees them". It prints these
  after the "exported" line, or on stderr when the bundle is printed, so
  the JSON stays whole.
- The docs (nodepolicy.md, running-a-node.md, FEATURES.md) no longer say
  "nothing personal". The `-full` help of both exports and both imports
  says what it keeps and takes.
- Tests: `TestExportLeavesOutPersonal` (pkg/nodepolicy), and
  `TestSettingsExportImport` now checks the plain export and its notes.
