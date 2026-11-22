# gmux

A tmux-like terminal multiplexer. The binary is `gmux` (`cmd/gmux`). The interface is tml. The host readings are simple-stats-api's sources run in-process.

## Build and test

```bash
go-toolchain
```

It tidies, vets, runs the Go tests with coverage, builds `build/gmux` and runs `dats/*.dats` against it. Never run a bare `go` command. Never pipe or redirect its output.

## Processes

- The **server** owns the sessions and the ptys. A client starts it on demand (`client.Start`), through `/bin/sh`, in a session of its own.
- A **client** connects to the server's unix socket and passes its terminal's file descriptors over it. The server then runs a `tml.Program` straight on that terminal. The client only waits, and reports a resize.
- They share the `mux.Mux`.

## Packages

- `vt/` — the terminal emulator: parser, grid, scrollback, modes. Not goroutine-safe: the pane locks.
- `pty/` — open a pty, start a program on it, resize it, read the foreground process.
- `layout/` — the split tree of one window and its geometry. Pure.
- `keys/` — tml input values back into the bytes a program reads.
- `mux/` — sessions, windows, panes. `Mux.mu` guards the structure and each `Pane` guards its screen. Take `Mux.mu` first.
- `stats/` — the host readings as the bar writes them.
- `ui/` — one attached client: `assets/screen.tml`, the pane widget, keys, pointer, menus.
- `server/` — the socket protocol, the requests, the attach.
- `client/` — find or start the server, hand the terminal over.
- `cli/` — cobra commands, one self-registering command per file.
- `internal/ttytest/` — a pty as a client's terminal, read back through `vt`, for tests.

## Invariants

- A watcher given to `Mux.Watch` must not block and must not call back into the multiplexer.
- A pane's input goes through its queue (`Pane.Input`). Nothing writes to a pty while it holds a lock.
- Pane output asks for at most one frame per `ui.minFrame`. Input and structure changes ask at once.
- tml reports the pointer only for widgets. A clickable piece of a bar is a `<Hit>` around its content.
- The window row is the first row of the screen and the information row the last (`ui.topRows`, `ui.bottomRows`). A pane's rows count from the top of the window area, so `Client.mouse` moves a pointer event up by `topRows` before it reaches a pane.
- The bars never give a click to a pane's program. Everything a pane's menu does is also in the menu of the window row (`Client.mainMenu`), for a program that takes the pointer.
- A colour is a `<Color>` of `ui/assets/theme.tml` with a `for` that says what it is used for. Nothing else names a colour: the pane widget reads its own from `View.Theme()`.
- Every cell of a bar has a `tooltip`.
- A control tml has is tml's: the menu is its `<Menu>`, and a pane's scrollbar is its `widget.Scrollbar`. A missing one is added there, not built here.
- A pane's terminal is narrower than the pane by `ui.Gutter`, the column of its scrollbar. The server sets `mux.Config.Gutter`.
- An element id in `screen.tml` is unique in the frame. A duplicate fails the frame, and that ends the program.
- The cli keeps flags in package variables. A cli test calls `t.Serial`.

## Tests

- Go tests start real shells on real ptys. `ui` tests drive a `Client` with no terminal and read frames back.
- `client` and `cli` tests attach through `internal/ttytest`.
- `dats/cli.dats` is the command-line contract. Each test gets a server of its own through `dats/with-socket.sh`.
