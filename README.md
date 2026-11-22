# gmux

A terminal multiplexer in Go, in the spirit of tmux. Sessions run on a server and outlive the terminal. Windows split into panes. The window row at the top and the information row at the bottom take the mouse, and show what the host does.

The interface is [tml](https://github.com/wow-look-at-my/tml). The host readings come from [simple-stats-api](https://github.com/wow-look-at-my/simple-stats-api), run inside the server.

## Use

```bash
gmux                      # start a session and attach: it takes the directory's name
gmux new -s work          # start a session with a name of your own
gmux attach -t work       # attach to a session that runs
gmux ls                   # list the sessions
gmux kill-session -t work
gmux kill-server
```

`ctrl+b` is the prefix, as in tmux. `ctrl+b ?` lists every key. The common ones:

| Key | Does |
| --- | --- |
| `c` `n` `p` `1`-`9` | new window, next, previous, by number |
| `%` `"` | split right, split down |
| arrows | focus the pane on that side |
| `z` `x` | zoom the pane, close the pane |
| `[` | scroll back |
| `d` | detach |

## The bars

The window row is at the top of the screen. It holds the menu, the session, a tab for each window, and the cells that act on your view. The information row is at the bottom. It holds the working directory, the running program, the pane size, the host's load and the clock.

```text
 ≡  work  1:sh ×  2:logs ×  +                                  □  ─
```

- `≡` opens the menu: new window, split right, split down, zoom, close the pane, sessions, detach, keys. The bars keep their clicks. The menu works when the program in a pane takes the mouse for itself.
- `×` on a tab closes that tab's window. `+` adds a window.
- `─` detaches: gmux leaves the terminal, and the session and its programs keep running until `gmux attach`.
- `□` zooms the pane in focus to the whole window, and `❐` brings the other panes back. The cell shows only when the window has more than one pane.
- Rest the pointer on any cell of a bar for a second, and the cell says what it does.
- Click a tab to show its window. Turn the wheel over the tabs to go through the windows.
- Drag a tab to move its window. Click it with the middle button to close it. Click it again to rename it.
- Right-click a tab, the session or a pane for a menu.
- Click the working directory to copy it. Click a reading or the clock for its longer form.
- Drag the line between a pair of panes to resize them. Click a pane to focus it.
- Drag in a pane to select text. The selection is copied when the button comes up, and the middle button pastes it.
- The wheel in a pane scrolls its history, and so do page up and page down. A pane with history shows a scrollbar at its right edge: drag the thumb, or click the track.
- A program that asks for the mouse gets it, and shift takes it back.
- A narrower pane rewraps its text, and a wider one joins the lines again.

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
