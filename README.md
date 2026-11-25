# margin

A terminal markdown notes editor. Your notes stay plain `.md` files in a folder, so they work with git, grep, Obsidian, or any other editor.

```
╭──────────────────────╮╭────────────────────────────────────────────────────╮
│ ▾ ideas              ││    Welcome                                         │
│     Side projects    ││                                                    │
│ ▸ work               ││   This is margin, a terminal notes app.            │
│   Welcome            ││                                                    │
│                      ││   ## Features                                      │
│                      ││                                                    │
│                      ││   • Folder tree                                    │
│                      ││   • Markdown rendering                             │
│                      ││   • Autosave                                       │
│                      ││                                                    │
│                      ││     fmt.Println("hi")                              │
╰──────────────────────╯╰────────────────────────────────────────────────────╯
 VIEW  Welcome.md                       e edit · n new · ctrl+p find · ? help
```

## Features

- **Folder tree** of your notes, with folders you can collapse
- **Rendered markdown** by default ([Glamour](https://github.com/charmbracelet/glamour)); press `e` to edit the raw text
- **Autosave** while you type; every write is atomic (temp file + rename)
- **Safe with other editors**: if a note changes on disk while you edit it, margin asks before overwriting
- **Fuzzy finder** (`ctrl+p`) by note name and **full-text search** (`ctrl+f`) across all notes
- **Trash** instead of delete: removed notes go to `<notes>/.trash/`
- Undo/redo, rename, folders, light/dark themes

## Install

```sh
go install github.com/beladevo/margin@latest
```

Or download a binary from [Releases](https://github.com/beladevo/margin/releases).

## Usage

```sh
margin              # opens ~/notes (created if missing)
margin ~/vault      # opens another folder
```

### Keys

| Browse | |
|---|---|
| `↑/↓` `j/k` | move |
| `←/→` `h/l` | collapse / expand, focus the note |
| `enter` | open folder / edit note |
| `tab` | switch pane |
| `e` | edit note |
| `n` / `N` | new note / new folder |
| `r` | rename |
| `d` | move to trash |
| `ctrl+p` | find note by name |
| `ctrl+f` or `/` | search inside notes |
| `?` | help |
| `q` | quit |

| Edit | |
|---|---|
| `esc` | save and go back to the rendered view |
| `ctrl+s` | save now |
| `ctrl+z` / `ctrl+y` | undo / redo |

When a note changes on disk while you have unsaved edits, a banner appears: `ctrl+r` reloads the disk version, `ctrl+k` keeps yours.

New notes are named after their title: `Meeting notes` becomes `Meeting notes.md`, starting with `# Meeting notes`.

## Config

`~/.config/margin/config.toml` (or `$XDG_CONFIG_HOME/margin/config.toml`), all optional:

```toml
notes_dir = "~/notes"
theme = "auto"          # auto | dark | light
autosave_delay = "1s"
```

## Development

```sh
go test -race ./...
golangci-lint run
```

## License

[MIT](LICENSE)
