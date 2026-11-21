# gmux

A terminal multiplexer in Go, in the spirit of tmux. Sessions run on a server and outlive the terminal. Windows split into panes. The status bars at the bottom take the mouse, and show what the host does.

The interface is [tml](https://github.com/wow-look-at-my/tml). The host readings come from [simple-stats-api](https://github.com/wow-look-at-my/simple-stats-api), run inside the server.

## Use

```bash
gmux                      # start a session and attach
gmux new -s work          # start a session with a name
gmux attach -t work       # attach to a session that runs
gmux ls                   # list the sessions
gmux kill-session -t work
gmux kill-server
```

`ctrl+b` is the prefix, as in tmux. `ctrl+b ?` lists every key. The common ones:

| Key | Does |
| --- | --- |
| `c` `n` `p` `0-9` | new window, next, previous, by number |
| `%` `"` | split right, split down |
| arrows | focus the pane on that side |
| `z` `x` | zoom the pane, close the pane |
| `[` | scroll back |
| `d` | detach |

## The bars

The upper row holds the session, the window tabs and the buttons. The lower row holds the working directory, the running program, the pane size, the host's load and the clock.

- Click a tab to show its window. Turn the wheel over the tabs to go through the windows.
- Drag a tab to move its window. Click it with the middle button to close it. Click it again to rename it.
- Right-click a tab, the session or a pane for a menu.
- Click the working directory to copy it. Click a reading or the clock for its longer form.
- Drag the line between a pair of panes to resize them. Click a pane to focus it.
- Drag in a pane to select text. The selection is copied when the button comes up, and the middle button pastes it.
- The wheel in a pane scrolls its history. A program that asks for the mouse gets it, and shift takes it back.

## Scripting

```bash
gmux new -d -s build -- make watch
gmux send-keys -t build 'make test' Enter
gmux capture-pane -t build
```

## Platforms

The binary is one portable executable. It is tested on Linux. The macOS side has not been run, and the host readings need `/proc`, so the bars leave them out there. Windows has no pseudo-terminals: the binary starts there and reports that it cannot open one.

`GMUX_THEME=light` draws the bars for a terminal with a light background.

## Build

```bash
go-toolchain
```

`AGENTS.md` describes the code.
