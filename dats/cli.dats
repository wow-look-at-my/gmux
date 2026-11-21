# Command-line tests of the gmux binary. go-toolchain runs every *.dats under dats/ after the build, and
# $GO_TOOLCHAIN_DATS_BUILD_DIR holds a copy of the binary. Each test runs its script through dats/with-socket.sh,
# which gives it a server of its own. The sandbox has no terminal, so the pointer tests make one: a pane of an
# outer session. The Go tests hand a client a pseudo-terminal for the rest of what a terminal sees.

sandbox:
	network: false

tests:
	- desc: help lists the commands
	  cmd: dats/with-socket.sh 'gmux --help'
	  exit: 0
	  outputs:
		stdout:
			- "new-session"
			- "attach-session"
			- "list-sessions"
			- "kill-session"
			- "kill-server"
			- "send-keys"
			- "capture-pane"
			- "rename-session"
		"!stdout":
			- "  server "

	- desc: listing with no server says none is running
	  cmd: dats/with-socket.sh 'gmux ls'
	  exit: 1
	  outputs:
		stderr:
			- "no server is running"

	- desc: a detached session starts the server and shows in the listing
	  cmd: dats/with-socket.sh 'gmux new-session -d -s work && gmux ls'
	  exit: 0
	  outputs:
		stdout:
			- "work: 1 windows (created "
		"!stdout":
			- "(attached)"

	- desc: a session with no name takes the name of its directory, and a second one a number after it
	  cmd: dats/with-socket.sh 'mkdir "$work/site" && cd "$work/site" && gmux new -d && gmux new -d && cd / && gmux new -d && gmux ls'
	  exit: 0
	  outputs:
		stdout:
			- "site: 1 windows (created "
			- "site-2: 1 windows (created "
			- "main: 1 windows (created "
		"!stdout":
			- "(attached)"

	- desc: keys sent to a session reach its shell, and the pane can be read back
	  cmd: dats/with-socket.sh 'gmux new -d -s t && gmux send-keys -t t "echo dats-\$((40+2))" Enter && shows t dats-42'
	  exit: 0
	  outputs:
		stdout:
			- "dats-42"

	- desc: a pane's program sees a terminal and the multiplexer's variables
	  cmd: dats/with-socket.sh 'gmux new -d -s t && gmux send-keys -t t "test -t 0 && echo term=\$TERM pane=\$GMUX_PANE" Enter && shows t pane=1'
	  exit: 0
	  outputs:
		stdout:
			- "term=xterm-256color pane=1"

	- desc: a session runs the command it is given, and ends with it
	  cmd: dats/with-socket.sh 'gmux new -d -s once -- /bin/sh -c "sleep 0.3" && gmux ls && sleep 1; gmux ls'
	  exit: 1
	  outputs:
		stdout:
			- "once: 1 windows"
		stderr:
			- "no server is running"

	- desc: a second session cannot take a name in use
	  cmd: dats/with-socket.sh 'gmux new -d -s same && gmux new -d -s same'
	  exit: 1
	  outputs:
		stderr:
			- 'a session named "same" already exists'

	- desc: a session is renamed and then ended by its new name
	  cmd: dats/with-socket.sh 'gmux new -d -s old && gmux new -d -s keep && gmux rename-session -t old new && gmux ls && gmux kill-session -t new && echo after && gmux ls'
	  exit: 0
	  outputs:
		stdout:
			- "new: 1 windows"
			- "after\nkeep: 1 windows"

	- desc: a target that names no session is an error
	  cmd: dats/with-socket.sh 'gmux new -d -s real && gmux capture-pane -t ghost'
	  exit: 1
	  outputs:
		stderr:
			- 'no session named "ghost"'

	- desc: kill-server ends the server
	  cmd: dats/with-socket.sh 'gmux new -d && gmux kill-server && gmux ls'
	  exit: 1
	  outputs:
		stderr:
			- "no server is running"

	# The next tests run the binary on a real terminal: a pane of an outer session. The pointer reports are typed
	# into that pane the way a terminal sends them, and the pane's text is what the inner gmux drew.
	- desc: a click on the bar's new-window button adds a window
	  cmd: dats/with-socket.sh '. dats/pointer.sh && scene click_adds_window'
	  exit: 0
	  outputs:
		stdout:
			- " demo  1:sh  2:sh "
			- "MEM "

	- desc: a right click on a tab opens its menu, and a click on a tab shows its window
	  cmd: dats/with-socket.sh '. dats/pointer.sh && scene tab_menu_and_switch'
	  exit: 0
	  outputs:
		stdout:
			- "1\n1"

	- desc: the buttons of the window row split, zoom and close the pane, and open the help
	  cmd: dats/with-socket.sh '. dats/pointer.sh && scene window_buttons'
	  exit: 0
	  outputs:
		stdout:
			- "right 39×22\nbelow 39×10\nzoomed 80×22\nclosed 39×22\n1"

	- desc: the wheel over the tabs turns through the windows
	  cmd: dats/with-socket.sh '. dats/pointer.sh && scene wheel_turns_windows'
	  exit: 0
	  outputs:
		stdout:
			- "1\n1"

	- desc: the middle button on a tab closes its window
	  cmd: dats/with-socket.sh '. dats/pointer.sh && scene middle_click_closes_window'
	  exit: 0
	  outputs:
		stdout:
			- " demo  1:sh  "
		"!stdout":
			- "2:sh"

	- desc: a tab dragged over another moves its window there
	  cmd: dats/with-socket.sh '. dats/pointer.sh && scene drag_moves_window'
	  exit: 0
	  outputs:
		stdout:
			- " demo  1:sleep  2:sh "

	- desc: a right click in the window opens a menu as wide as its longest line
	  cmd: dats/with-socket.sh '. dats/pointer.sh && scene pane_menu_fits'
	  exit: 0
	  outputs:
		stdout:
			- "╭ pane ──────────╮\n│ Even out panes │"

	- desc: a click on the clock shows the date, and a click on the directory copies it
	  cmd: dats/with-socket.sh '. dats/pointer.sh && scene info_row_clicks'
	  exit: 0
	  outputs:
		stdout:
			- "1\n1"

	- desc: attaching without a terminal is refused
	  cmd: dats/with-socket.sh 'gmux new -d -s t && gmux attach -t t </dev/null'
	  exit: 1
	  outputs:
		stderr:
			- "attaching needs a terminal"

	- desc: an attach from inside a session is refused
	  cmd: dats/with-socket.sh 'gmux new -d -s t && GMUX=inside gmux attach -t t'
	  exit: 1
	  outputs:
		stderr:
			- "already inside a gmux session"
