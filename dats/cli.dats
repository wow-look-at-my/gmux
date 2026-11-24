# Command-line tests of the gmux binary. go-toolchain runs every *.dats under dats/ after the build, and
# $GO_TOOLCHAIN_DATS_BUILD_DIR holds a copy of the binary. Each test runs its script through dats/with-socket.sh,
# which gives it a server of its own. The sandbox has no terminal, so the pointer tests make one: a pane of an
# outer session. The Go tests hand a client a pseudo-terminal for the rest of what a terminal sees.

sandbox:
	network: false

tests:
	- desc: help lists the command groups and the version command
	  cmd: dats/with-socket.sh 'gmux --help'
	  exit: 0
	  outputs:
		stdout:
			- "  session "
			- "  server "
			- "  pane "
			- "  version "
		"!stdout":
			- "new-session"
			- "attach-session"
			- "list-sessions"
			- "kill-session"
			- "kill-server"
			- "send-keys"
			- "capture-pane"
			- "rename-session"

	- desc: each group lists its commands, and the server group hides the one a client starts the server with
	  cmd: dats/with-socket.sh 'gmux session --help && gmux server --help && gmux pane --help'
	  exit: 0
	  outputs:
		stdout:
			- "  new "
			- "  attach "
			- "  rename "
			- "  kill "
			- "  list "
			- "  send "
			- "  capture "
		"!stdout":
			- "  run "

	- desc: a word that names no command of a group is an error
	  cmd: dats/with-socket.sh 'gmux session dance'
	  exit: 1
	  outputs:
		stderr:
			- 'unknown command "dance" for "gmux session"'

	- desc: a name the commands had before the groups is no command
	  cmd: dats/with-socket.sh 'gmux list-sessions'
	  exit: 1
	  outputs:
		stderr:
			- 'unknown command "list-sessions" for "gmux"'

	# A build outside CI has no release, so the binary has no version to print. Both builds run this test.
	- desc: version prints the release the binary was built as, or says the build has none
	  cmd: dats/with-socket.sh 'out=$(gmux version 2>&1); code=$?; case "$code $out" in "0 gmux version "[0-9]*) echo ok;; "1 gmux: this binary was not built for a release"*) echo ok;; *) echo "$code $out"; exit 1;; esac'
	  exit: 0
	  outputs:
		stdout:
			- "ok"

	- desc: listing with no server says none is running
	  cmd: dats/with-socket.sh 'gmux session list'
	  exit: 1
	  outputs:
		stderr:
			- "no server is running"

	- desc: a detached session starts the server and shows in the listing
	  cmd: dats/with-socket.sh 'gmux session new -d -s work && gmux session list'
	  exit: 0
	  outputs:
		stdout:
			- "work: 1 windows (created "
		"!stdout":
			- "(attached)"

	- desc: a session with no name takes the name of its directory, and a second one a number after it
	  cmd: dats/with-socket.sh 'mkdir "$work/site" && cd "$work/site" && gmux session new -d && gmux session new -d && cd / && gmux session new -d && gmux session list'
	  exit: 0
	  outputs:
		stdout:
			- "site: 1 windows (created "
			- "site-2: 1 windows (created "
			- "main: 1 windows (created "
		"!stdout":
			- "(attached)"

	- desc: keys sent to a session reach its shell, and the pane can be read back
	  cmd: dats/with-socket.sh 'gmux session new -d -s t && gmux pane send -t t "echo dats-\$((40+2))" Enter && shows t dats-42'
	  exit: 0
	  outputs:
		stdout:
			- "dats-42"

	- desc: a pane's program sees a terminal and the multiplexer's variables
	  cmd: dats/with-socket.sh 'gmux session new -d -s t && gmux pane send -t t "test -t 0 && echo term=\$TERM pane=\$GMUX_PANE" Enter && shows t pane=1'
	  exit: 0
	  outputs:
		stdout:
			- "term=xterm-256color pane=1"

	- desc: a session runs the command it is given, and ends with it
	  cmd: dats/with-socket.sh 'gmux session new -d -s once -- /bin/sh -c "sleep 0.3" && gmux session list && sleep 1; gmux session list'
	  exit: 1
	  outputs:
		stdout:
			- "once: 1 windows"
		stderr:
			- "no server is running"

	- desc: a second session cannot take a name in use
	  cmd: dats/with-socket.sh 'gmux session new -d -s same && gmux session new -d -s same'
	  exit: 1
	  outputs:
		stderr:
			- 'a session named "same" already exists'

	- desc: a session is renamed and then ended by its new name
	  cmd: dats/with-socket.sh 'gmux session new -d -s old && gmux session new -d -s keep && gmux session rename -t old new && gmux session list && gmux session kill -t new && echo after && gmux session list'
	  exit: 0
	  outputs:
		stdout:
			- "new: 1 windows"
			- "after\nkeep: 1 windows"

	- desc: a target that names no session is an error
	  cmd: dats/with-socket.sh 'gmux session new -d -s real && gmux pane capture -t ghost'
	  exit: 1
	  outputs:
		stderr:
			- 'no session named "ghost"'

	- desc: server kill ends the server
	  cmd: dats/with-socket.sh 'gmux session new -d && gmux server kill && gmux session list'
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
			- " demo  1:sh ×  2:sh × "
			- "MEM "

	- desc: the window row is the first row of the screen, and the information row the last
	  cmd: dats/with-socket.sh '. dats/pointer.sh && scene bars_at_both_ends'
	  exit: 0
	  outputs:
		stdout:
			- "top: 1\nbottom: 1"

	- desc: the close cell of a tab closes that tab's window, and each tab has one
	  cmd: dats/with-socket.sh '. dats/pointer.sh && scene tab_closes_its_window'
	  exit: 0
	  outputs:
		stdout:
			- "close cells: 3\nclose cells: 2\nthe tab beside it stays: 1"

	- desc: the cell at the right end of the window row detaches, and the session keeps running
	  cmd: dats/with-socket.sh '. dats/pointer.sh && scene detach_cell_leaves_the_session'
	  exit: 0
	  outputs:
		stdout:
			- "1\ndemo: 1 windows (created "

	- desc: the menu of the window row splits the window under a program that takes the pointer
	  cmd: dats/with-socket.sh '. dats/pointer.sh && scene menu_splits_under_a_program_with_the_pointer'
	  exit: 0
	  outputs:
		stdout:
			- "pane menu: 0\nsplit: 38×22"

	- desc: a cell of the window row shows its tooltip after the pointer rests on it, and a move takes it away
	  cmd: dats/with-socket.sh '. dats/pointer.sh && scene tooltip_after_a_rest'
	  exit: 0
	  outputs:
		stdout:
			- "at once: 0\nafter a rest: 1\nafter a move: gone"

	- desc: a right click on a tab opens its menu, and a click on a tab shows its window
	  cmd: dats/with-socket.sh '. dats/pointer.sh && scene tab_menu_and_switch'
	  exit: 0
	  outputs:
		stdout:
			- "1\n1"

	- desc: the menu of the window row splits and closes panes and opens the help, and the zoom cell zooms
	  cmd: dats/with-socket.sh '. dats/pointer.sh && scene window_buttons'
	  exit: 0
	  outputs:
		stdout:
			- "zoom cell with one pane: 0\nright 38×22\nbelow 38×10\nzoomed 78×22\nclosed 38×22\n1"

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
			- " demo  1:sh × "
		"!stdout":
			- "2:sh"

	- desc: a tab dragged over another moves its window there
	  cmd: dats/with-socket.sh '. dats/pointer.sh && scene drag_moves_window'
	  exit: 0
	  outputs:
		stdout:
			- " demo  1:sleep ×  2:sh × "

	- desc: a right click in the window opens a menu as wide as its longest line
	  cmd: dats/with-socket.sh '. dats/pointer.sh && scene pane_menu_fits'
	  exit: 0
	  outputs:
		stdout:
			- "╭ pane ──────────╮\n│ Even out panes │"

	- desc: a narrower pane cuts a long line, and a wider one joins it again
	  cmd: dats/with-socket.sh '. dats/pointer.sh && scene reflow_on_resize'
	  exit: 0
	  outputs:
		stdout:
			- "cut: 1 then 1\nwhole: 1"

	- desc: a pane with history to scroll back to shows a scrollbar
	  cmd: dats/with-socket.sh '. dats/pointer.sh && scene scrollbar_shows'
	  exit: 0
	  outputs:
		stdout:
			- "before 0\nafter: a thumb at the right edge"

	- desc: a click on the clock shows the date, and a click on the directory copies it
	  cmd: dats/with-socket.sh '. dats/pointer.sh && scene info_row_clicks'
	  exit: 0
	  outputs:
		stdout:
			- "1\n1"

	- desc: attaching without a terminal is refused
	  cmd: dats/with-socket.sh 'gmux session new -d -s t && gmux session attach -t t </dev/null'
	  exit: 1
	  outputs:
		stderr:
			- "attaching needs a terminal"

	- desc: an attach from inside a session is refused
	  cmd: dats/with-socket.sh 'gmux session new -d -s t && GMUX=inside gmux session attach -t t'
	  exit: 1
	  outputs:
		stderr:
			- "already inside a gmux session"
