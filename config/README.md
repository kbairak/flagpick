# Writing flagpick configs

A config teaches flagpick how to build one command's argv. This document is the
full spec for the config format.

> Status: this is the implemented format. The loader, TUI, and
> `config/rg.yaml` all follow this spec.

---

## 1. Files and commands

- One config per command: `config/<command>.yaml` (e.g. `direnv.yaml`).
- The filename (minus `.yaml`) is the command name. `fp direnv ...` loads
  `direnv.yaml`.
- flagpick looks the file up in the user-override dir first
  (`$XDG_CONFIG_HOME/flagpick/<command>.yaml`, fallback
  `~/.config/flagpick/`), then the downloaded dir
  (`$XDG_DATA_HOME/flagpick/<command>.yaml`, fallback `~/.local/share/flagpick/`).
- `config/manifest.json` maps each config filename to its md5; keep it in sync
  with `make manifest`.

---

## 2. A config is a tree of nodes

A config file is a single **root node**. Nodes nest:

- `modes` — alternative invocation shapes of the *same* command (chosen by
  cycling in the UI). No literal token is emitted for a mode.
- `subcommands` — named child commands (chosen by a literal verb token that
  **is** emitted).

Root, every mode, and every subcommand is a **node** with the same fields. That
makes the format fully recursive: a subcommand may itself have `modes`, its own
`subcommands`, its own `pool`, and so on, to any depth.

### 2.1 Node fields

A node is either a **fork** (defines `modes`) or a **shape** (defines its own
arguments). The two kinds accept different fields.

**Fork node** (`modes` present):

| field   | type          | meaning                                                        |
| ------- | ------------- | -------------------------------------------------------------- |
| `pool`  | pool object   | local reusable definitions and groups (§4)                     |
| `modes` | list of nodes | alternative child shapes; the fork delegates to them           |

A fork does **not** declare `options`, `usage`, `description`, or
`subcommands`; those belong on each mode.

**Shape node** (`modes` absent):

| field         | type          | meaning                                                             |
| ------------- | ------------- | ------------------------------------------------------------------- |
| `pool`        | pool object   | local reusable definitions and groups (§4)                          |
| `options`     | list of refs  | this shape's selection from the visible pool                        |
| `usage`       | string        | one-line usage string for the UI                                    |
| `description` | string        | free text shown in the UI                                           |
| `subcommands` | list of nodes | verb-selected children dispatched after this shape's own arguments  |

where a **pool object** is `{options: [definitions], groups: {name: [refs]}}`
(§4.1).

**Subcommand nodes** — a node used as a `subcommands` entry, of either kind,
additionally accepts:

| field     | type            | meaning                                           |
| --------- | --------------- | ------------------------------------------------- |
| `name`    | string          | the emitted verb                                  |
| `aliases` | list of strings | alternative spellings (emit the canonical `name`) |

`modes` and `subcommands` describe different things and belong at different
levels: a **fork** fans out into modes, and each **shape** may fan out into
subcommands. Do not put `modes` and `subcommands` on the same node.

### 2.2 Single-shape shorthand

There is no required wrapper for "one shape". A node with no `modes` **is** a
single shape, described directly by its `usage`/`description`/`options`.

This applies to the root and to subcommands alike. A command with one shape and
no subcommands is just:

```yaml
pool:
  options:
    - {name: PATTERN, kind: positional, type: string, required: true}
options: [PATTERN]
usage: mycmd PATTERN
```

And a single-shape subcommand is just one entry:

```yaml
subcommands:
  - name: allow
    usage: direnv allow [PATH_TO_RC]
    options: [PATH_TO_RC]
```

### 2.3 Subcommands

Each `subcommands` entry is a node plus:

- `name` (required): the literal token emitted. Must be a single token with no
  leading `-`.
- `aliases` (optional): alternative spellings the user may type. Parsing
  accepts any of them; assembly always emits the canonical `name`.

Sibling `name`s and `aliases` must be unique. A verb/alias may not start with
`-`.

When a node has `subcommands`, the first non-flag token in that position is the
verb. Consequently a node with `subcommands` should restrict its own `options`
to flags (global options); leading positionals are not supported alongside
`subcommands`.

### 2.4 Recursion

Because every node shares the same fields, this composes without limit:

```yaml
subcommands:
  - name: remote
    usage: mycmd remote COMMAND
    subcommands:               # a subcommand that is itself a dispatcher
      - name: add
        usage: mycmd remote add <name> <url>
        options: [NAME, URL]
      - name: remove
        usage: mycmd remote remove <name>
        options: [NAME]
  - name: status
    usage: mycmd status
    modes:                     # a subcommand with alternative shapes
      - {name: Brief, usage: mycmd status, options: [SHORT]}
      - {name: Full,  usage: mycmd status --json, options: [JSON]}
```

A subcommand can also define its own `pool.options`/`pool.groups` instead of
borrowing the root's (see §4.6).

---

## 3. Assembly order

For a chosen path through the tree (root shape → subcommand shape → …), the
argv is built depth-first:

1. emit the node's own arguments — **flags first, then positionals**, each in
   the order they appear in the node's `options` selection;
2. emit the chosen child's verb token (`name`);
3. recurse into the child.

So globals precede the verb, and subcommand arguments follow it:

```
mycmd --global remote add origin <url>
      └ root ┘  └verb┘ └ subcommand args ┘
```

Output is deterministic, which makes it golden-testable. Short forms are never
emitted; only long forms and the verb names are.

---

## 4. Options

### 4.1 The `pool` object

`pool` holds a node's reusable pieces and is inherited by all descendants
(§4.6):

```yaml
pool:
  options:                       # reusable definitions (flags + positionals)
    - {name: PATH_TO_RC, kind: positional, type: string, required: true}
    - {name: Ignore case, kind: flag, type: bool, long: --ignore-case, short: "-i"}
  groups:                        # reusable named selections
    common: [Ignore case, Hidden]
```

### 4.2 `pool.options` — definitions

A list of option **definitions**, each either a flag or a positional
(discriminated by `kind`). Definitions are referenced elsewhere by `name`; they
are not exposed by themselves.

Flags (`kind: flag`) accept:

| field            | required | meaning                                                        |
| ---------------- | -------- | -------------------------------------------------------------- |
| `name`           | yes      | display label and identity (used by `conflicts`, `requires`, selection) |
| `type`           | yes      | `bool`, `string`, `int`, `enum`, `size`, `count`               |
| `long`           | yes      | long form including `--`                                        |
| `short`          | no       | short form including `-`, or omitted                           |
| `description`    | no       | shown in the description pane                                  |
| `negative`       | no       | a `--no-...` form; makes a bool tri-state (see §4.5)           |
| `negative_short` | no       | short form for `negative` (e.g. `-N`); requires `negative`     |
| `repeatable`     | no       | collect multiple values, one occurrence each                   |
| `conflicts`      | no       | names of flags that cannot be set at the same time             |
| `requires`       | no       | names of flags that must also be set                           |
| `values`         | enum     | the allowed values for `type: enum`                            |

Positionals (`kind: positional`) accept:

| field         | required | meaning                                            |
| ------------- | -------- | -------------------------------------------------- |
| `name`        | yes      | display label and identity                         |
| `type`        | yes      | `string`, `path`, `enum`, or `passthrough` (see §6) |
| `description` | no       | shown in the description pane                      |
| `required`    | no       | must have a value (default false)                  |
| `variadic`    | no       | collects zero or more values; must be last          |
| `values`      | enum     | the allowed values for `type: enum`                |

A `passthrough` positional is always variadic (it collects all remaining
tokens), so `variadic` is not a valid field on one: declaring it is an error.
Use only `name`, `type`, `description`, and `required`.

### 4.3 `options` — a shape's selection

`options` is an ordered list of references. Each ref is a `pool.options` entry
`name` or a `pool.groups` name. It defines which options a shape exposes and in
what order. Refs are deduplicated within a selection (first occurrence wins).

```yaml
pool:
  options:
    - {name: PATTERN, kind: positional, type: string, required: true}
    - {name: Ignore case, kind: flag, type: bool, long: --ignore-case, short: "-i"}
  groups:
    common: [Ignore case]
options: [common, PATTERN]
```

### 4.4 `pool.groups` — reusable selections

A group is a named list of refs (`pool.options` names or other group names).
Groups may reference other groups; cycles are an error. Groups are the
recommended way to share a set of options between a root and its subcommands.

```yaml
pool:
  groups:
    common: [Ignore case, Hidden]
    input:  [Pattern file, Pre command]
options: [common, input, PATTERN]
```

### 4.5 Bool, negative, and negative-short

- Bool without `negative`: `false` → emit nothing; `true` → emit `long`.
- Bool with `negative`: tri-state. Unset → nothing; on → `long`; off →
  `negative`. A `negative_short` (e.g. `-I`) is an accepted short spelling for
  the off state, and assembly still emits the long `negative` form.
- `negative` must start with `--` and differ from `long`. `negative_short` must
  start with a single `-` (not `--`), must differ from `short`, and requires a
  `negative`.

### 4.6 Pool inheritance and name resolution

`pool.options` and `pool.groups` are collected from the root down. A node sees
the union of its own and all ancestors' entries. When the same name is defined
more than once, the nearest definition wins.

Selection refs inside a node resolve against that node's **visible** pool
(own + ancestors). This is what lets a subcommand reference a definition that
lives at the root without redeclaring it:

```yaml
pool:
  options:
    - {name: PATH_TO_RC, kind: positional, type: string, required: true}
subcommands:
  - name: allow
    options: [PATH_TO_RC]     # resolves against the root pool
```

A subcommand may instead declare its own `pool.options`/`pool.groups` for
options unique to it, and still reference root entries for the rest.

---

## 5. Prefill and the UI

When you run `fp <command> <tokens...>`, the tokens are parsed against the tree:

1. At the current node, tokens are matched against the current shape's options:
   long/short/negative forms fill flags, non-flag tokens fill positionals in
   order (a trailing variadic positional collects the rest).
2. When the current node has `subcommands`, the first non-flag token must match
   a child `name` or `alias`; the parser descends into it and discards the verb
   from the body (it is re-emitted canonically at assembly time).
3. Matching is strict: an unknown flag, an unmatched verb, or too many
   positionals is an error — never a silent drop or a raw passthrough (use a
   `passthrough` positional when you actually want that, §6).

In the TUI, `modes` and `subcommands` both appear as selectable shapes; the
active path's `usage` and `description` are shown, and the preview is the fully
assembled, shell-quoted command.

---

## 6. Passthrough positionals

`type: passthrough` is a positional that swallows every remaining token
verbatim, including tokens that look like flags. It is always variadic and must
be the **last positional** of its shape.

Use it for commands that hand a tail to another program:

```yaml
pool:
  options:
    - {name: DIR,  kind: positional, type: path, required: true}
    - {name: ARGS, kind: positional, type: passthrough, required: true}
options: [DIR, ARGS]
usage: direnv exec DIR COMMAND [...ARGS]
```

Parsing `direnv exec ./proj rg --json -i foo` yields `DIR=./proj` and
`ARGS=[rg, --json, -i, foo]`, all emitted unchanged.

---

## 7. Validation rules

A config is rejected when any of these fail:

- `pool.options` entry `name`s are non-empty and unique within a node's own
  `pool.options`.
- Flag `long` is present; enum options (flags and positionals) have at least
  one `values` entry.
- `negative` starts with `--` and differs from `long`; `negative_short` starts
  with a single `-`, differs from `short`, and has a `negative`.
- Selection refs resolve to a visible pool entry or group.
- Group references resolve and contain no cycles.
- A shape has at most one variadic positional, and it is last.
- A `passthrough` positional is the last positional.
- Mode `name`s are unique among siblings.
- Subcommand `name`s and `aliases` are non-empty, do not start with `-`, and
  are unique among siblings (a name may not equal a sibling alias).
- A node has either `modes` or shape fields, not both.

---

## 8. Type-to-widget map

| `type`        | widget                                         |
| ------------- | ---------------------------------------------- |
| `bool`        | checkbox (tri-state when `negative` is set)    |
| `string`      | text input (flag value or positional)          |
| `int`         | numeric text input                             |
| `enum`        | value menu                                     |
| `size`        | text input `NUM[KMG]`                          |
| `count`       | repeated flag; space cycles 0–3                |
| `path`        | filesystem picker                              |
| `passthrough` | raw text rows, written through verbatim        |
| `variadic`    | one row per value plus an add row              |
| `repeatable`  | same as variadic, for flags                    |

---

## 9. Worked example: `direnv`

```yaml
# config/direnv.yaml
description: Manages environment variables per directory.

pool:
  options:
    - {name: PATH_TO_RC, kind: positional, type: string, required: true}
    - {name: DIR,        kind: positional, type: path,   required: true}
    - {name: ARGS,       kind: positional, type: passthrough, required: true}
    - {name: MESSAGE,    kind: positional, type: string, required: true}
    - {name: JSON,   kind: flag, type: bool, long: --json}
    - {name: Status, kind: flag, type: bool, long: --status, conflicts: [Error]}
    - {name: Error,  kind: flag, type: bool, long: --error,  conflicts: [Status]}

usage: direnv COMMAND [...ARGS]

subcommands:
  - name: allow
    aliases: [permit, grant]
    description: Grants direnv permission to load the given .envrc or .env file.
    usage: direnv allow [PATH_TO_RC]
    options: [PATH_TO_RC]

  - name: block
    aliases: [deny, disallow, revoke]
    description: Revokes the authorization of a given .envrc or .env file.
    usage: direnv block [PATH_TO_RC]
    options: [PATH_TO_RC]

  - name: edit
    description: Opens the .envrc in $EDITOR and allows it to load.
    usage: direnv edit [PATH_TO_RC]
    options: [PATH_TO_RC]

  - name: exec
    description: Executes a command after loading the first .envrc found in DIR.
    usage: direnv exec DIR COMMAND [...ARGS]
    options: [DIR, ARGS]

  - name: status
    description: Prints some debug status information.
    usage: direnv status [--json]
    options: [JSON]

  - name: log
    description: Logs a given message.
    usage: direnv log [--status | --error] <message>
    options: [Status, Error, MESSAGE]
```

The root is a dispatcher (`usage` + `subcommands`, no `modes`), so it needs no
wrapper node. Each subcommand is a single-shape node referencing the root pool.

---

## 10. Migrating `rg.yaml`

The existing `rg.yaml` predates this spec: it uses `options` for definitions
and `modes` for shapes. Under the new format it becomes roughly:

```yaml
pool:
  options:
    - {name: PATTERN, kind: positional, type: string, required: true}
    - {name: PATH,    kind: positional, type: path,   required: false, variadic: true}
    - {name: Regular expression, kind: flag, type: string, long: --regexp, short: "-e", repeatable: true}
    # ...every current option moves into pool.options...
  groups:
    common: [input, search, filter, output, output_modes, logging, other]
    input: [Pattern file, Pre command, Pre glob, Search zip]
    # ...the remaining groups are unchanged...
modes:
  - {name: Pattern,  usage: "rg [OPTIONS] PATTERN [PATH ...]", options: [common, PATTERN, PATH]}
  - {name: Regexp,   usage: "rg [OPTIONS] -e PATTERN ... [PATH ...]", options: [common, Regular expression, PATH]}
  - {name: Info,     usage: "rg -h | -V | --type-list | --pcre2-version", options: [info]}
  - {name: Generate, usage: "rg --generate KIND", options: [generate]}
```

The two changes are mechanical: definitions move from the old top-level
`options` into `pool.options`, and the old inline `options` list becomes
`options` selections on each mode.
