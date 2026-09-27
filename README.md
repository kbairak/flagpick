# flagpick

A TUI for building and running complex CLI commands.

You point `flagpick` at a command — `rg`, `direnv`, … — and it shows that
command's flags and positional arguments as a list. You pick and edit them
fuzzy-style, watch the assembled command preview update live, and then run it,
copy it, or print it. No more half-remembering whether it's `-A` or `--after`,
or how a subcommand nests.

```sh
fp rg          # build a ripgrep command in the TUI
fp rg -i foo   # open with -i checked and foo prefilled
fp direnv      # pick a subcommand, then its options
```

What each command supports is described by a small YAML file — one per command.
The bundled configs live in [`config/`](config/). The format is documented in
[`config/README.md`](config/README.md); it supports flags, positionals,
subcommands, modes, groups, and reusable option pools.

## Install

Requires Go 1.25+.

```sh
go install github.com/kbairak/flagpick@latest
```

Go names the binary after the module path, so this installs **`flagpick`** (not
`fp`). The rest of this document uses `fp`; either alias it or symlink it:

```sh
alias fp=flagpick
# or
ln -s "$(go env GOPATH)/bin/flagpick" /usr/local/bin/fp
```

Then download the configs:

```sh
fp --update
```

Configs are not embedded in the binary. `--update` fetches
`config/manifest.json` and the configs it lists into the data directory, hashing
each file so only new or changed configs are downloaded.

### Build from source

```sh
git clone https://github.com/kbairak/flagpick
cd flagpick
make build      # builds ./fp
```

## Usage

```
fp [flagpick-flags] <command> [tool-args...]
```

The first non-flag token is the wrapped command; everything after it prefills
the TUI. `flagpick`'s own flags must come before it:

| flag              | behavior                                             |
| ----------------- | ---------------------------------------------------- |
| `--update`        | download/refresh configs from the remote manifest    |
| `-l`, `--list`    | list known commands                                  |
| `-r`, `--resume`  | prefill with the last composition for `<command>`    |
| `-h`, `--help`    | usage                                                |
| `-V`, `--version` | version                                              |

On launch the TUI shows a live preview of the assembled command. The default
action is to run it; from the menu (or via a direct key) you can also copy it to
the clipboard or print it. Running replaces the `flagpick` process with the
wrapped command (`exec`), so after it exits you're back at your shell.

### Keymap

| key                             | action                                             |
| ------------------------------- | -------------------------------------------------- |
| any printable                   | fuzzy-filter the option list                        |
| `ctrl-j` / `ctrl-k`, `up`/`down`| move the selection                                  |
| `ctrl-d` / `ctrl-u`             | move half a page                                    |
| `pgup` / `pgdown`               | scroll the description pane                         |
| `space`                         | toggle a flag / edit a value / add a value           |
| `space` on path                 | open the filesystem picker                          |
| `space` on enum                 | open the value menu                                 |
| `space` on command              | open the subcommand menu                            |
| `space` on count                | cycle repeats (0–3)                                 |
| `enter`                         | open the action menu (run / copy / print / cancel)  |
| `ctrl-x` / `ctrl-shift-x`       | clear the selected entry / clear everything         |
| `ctrl-shift-j` / `ctrl-shift-k` | reorder the selected flag (`ctrl-up`/`ctrl-down`)   |
| `tab` / `shift-tab`             | cycle modes                                         |
| `ctrl-r`                        | run                                                 |
| `ctrl-y` / `ctrl-shift-y`       | copy and exit / copy and stay                       |
| `ctrl-p`                        | print after exiting                                 |
| `ctrl-w`                        | delete the last filter word                         |
| `esc`                           | clear filter, else open quit confirm                |
| `ctrl-c`                        | quit immediately                                    |
| `ctrl-?`                        | help overlay                                        |

## Configs

A config teaches `flagpick` how to build one command's argv. It is a single YAML
file per command, named after the command (`config/rg.yaml` → `fp rg`).

`flagpick` looks for `<command>.yaml` in:

1. the user-override dir — `$XDG_CONFIG_HOME/flagpick/`
   (fallback `~/.config/flagpick/`);
2. then the downloaded dir — `$XDG_DATA_HOME/flagpick/`
   (fallback `~/.local/share/flagpick/`).

`--update` writes to the downloaded dir; a file in the override dir shadows it.
`make install-config` copies the repo's configs into the override dir.

Bundled configs: [`rg`](config/rg.yaml) (ripgrep) and
[`direnv`](config/direnv.yaml).

The config format is documented in full in
[`config/README.md`](config/README.md). Read that before writing or changing a
config.

## Contributing

Contributions are mostly **new config files** for tools you use, and fixes to
existing ones. Both are welcome.

### Testing a config locally

```sh
make install-config   # copy config/*.yaml into $XDG_CONFIG_HOME/flagpick
./fp <command>        # or: go run . <command>
```

`make install-config` puts your working copies in the override dir, so they win
over anything downloaded. Edit `config/<command>.yaml`, re-run
`make install-config`, and try it.

Before opening a PR:

```sh
make manifest     # refresh config/manifest.json (hashes every config)
make test vet fmt
```

`TestManifestUpToDate` fails if you change a config without regenerating the
manifest.

### Proposing a change from the GitHub UI

You don't need to clone the repo:

- **Edit an existing config** — open the file in
  [`config/`](config/), click the pencil, make your change, then
  *Propose changes* → open a pull request.
- **Add a new config** — in [`config/`](config/) choose *Add file* →
  *Create new file*, name it `<command>.yaml`, and paste the config.

The manifest still has to match. A maintainer can run `make manifest` for you
(say so in the PR description), or you can update it by hand: compute the MD5 of
your file and set it in [`config/manifest.json`](config/manifest.json):

```sh
md5 -q config/<command>.yaml        # macOS
md5sum config/<command>.yaml        # Linux
```

```json
{
  "rg": "…",
  "<command>": "<the hash you just computed>"
}
```

### Writing a config with an LLM

`flagpick` configs are good LLM fodder: the format is a small, fully-specified
document, and the source of truth for a command is its `--help` / man page. Give
the model both and ask it to emit one YAML file. Always review the result and
try it locally before opening a PR — models confidently invent flags.

Prompts that work well:

> Read `config/README.md` (the flagpick config spec) and the output of
> `<command> --help` below. Write `config/<command>.yaml`: one config for
> `<command>`, every documented option mapped to the closest `type`, with
> `conflicts` for mutually exclusive flags and a `description` per option.
> Output only the YAML.
>
> ```
> <paste `command --help` here>
> ```

> Read `config/README.md` (the flagpick config spec) and `man <command>`.
> `<command>` is a dispatcher: model its subcommands with `subcommands`, its
> per-subcommand flags/positionals with each subcommand's own `options`, and
> put shared flags in the root `pool`/`groups`. Where a subcommand accepts a
> fixed set of words, use a `kind: positional, type: enum` with `values`. Output
> only the YAML.
>
> ```
> <paste `command --help` and/or the man page here>
> ```

For a starting point, look at [`config/direnv.yaml`](config/direnv.yaml) — it
exercises subcommands, aliases, an enum positional, and a `passthrough`
positional.

## Development

Everything is one Go module; the binary lives at the module root.

```
main.go        CLI entry point and action handling
config.go      config loading (dirs, decode, manifest-facing)
node.go        config tree resolution (nodes, pools, paths, validation)
option.go      flag/positional types and their assembly
assemble.go    argv assembly and token prefill
tui.go         the Bubble Tea TUI
picker.go      filesystem picker for `path` positionals
remote.go       --update / manifest fetching
state.go       --resume state
clipboard.go   native clipboard + OSC52 fallback
cmd/genmanifest/   manifest.json generator
config/        bundled configs + the config spec (README.md)
```

```sh
make all   # fmt, test, vet, manifest, install-config, build
make test
```

Unit tests cover config parsing/validation, assembly and prefill (golden), the
remote update path (via `httptest`), and TUI layout/interaction. There is no
network access in tests.
