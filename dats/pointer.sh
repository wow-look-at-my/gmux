#!/bin/sh
# Scenes that drive the status bars of a real gmux with the pointer.
# dats/with-socket.sh sources this file, so `gmux`, `shows` and $work are its.

# inner_start attaches a gmux to the outer session's pane and waits for its bars.
inner_start() {
	gmux new -d -s outer || return 1
	gmux send-keys -t outer "GMUX= $work/gmux -S $work/inner.sock new -s demo" Enter || return 1
	shows outer " demo  0:sh " >/dev/null
}

inner_stop() { "$work/gmux" -S "$work/inner.sock" kill-server >/dev/null 2>&1; }

# gone waits until the active pane of a session no longer holds a text.
gone() {
	tries=0
	while gmux capture-pane -t "$1" | grep -q -- "$2"; do
		tries=$((tries + 1))
		if [ "$tries" -gt 100 ]; then
			echo "the pane of $1 still shows $2" >&2
			return 1
		fi
		sleep 0.1
	done
}

# pointer types one press and release at a cell of the window row. The cell counts from one, as a report does.
pointer() {
	gmux send-keys -l -t outer "$(printf '\033[<%d;%d;23M\033[<%d;%d;23m' "$1" "$2" "$1" "$2")"
}

# The button that adds a window is the first of the buttons at the right of the window row.
click_adds_window() {
	inner_start && pointer 0 64 && shows outer " 1:sh " | tail -n 2
	status=$?
	inner_stop
	return "$status"
}

# A right click on the first tab opens that window's menu. A left click on the tab shows the window again.
tab_menu_and_switch() {
	inner_start &&
		gmux send-keys -t outer "echo first-window" Enter &&
		pointer 0 64 && shows outer " 1:sh " >/dev/null &&
		pointer 2 9 && shows outer "Move right" | grep -c "Move right" &&
		gmux send-keys -t outer Escape &&
		pointer 0 9 && shows outer "^first-window" | grep -c "^first-window"
	status=$?
	inner_stop
	return "$status"
}
