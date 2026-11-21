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

# scene runs one scene between the start and the stop of the inner gmux.
scene() {
	inner_start && "$@"
	status=$?
	inner_stop
	return "$status"
}

windows_row=23
info_row=24

# row prints one row of what the inner gmux drew.
row() { gmux capture-pane -t outer | sed -n "${1}p"; }

# column prints the cell of a row where a text starts. The cell counts from one, as a pointer report does.
column() {
	line=$(row "$1")
	case $line in
	*"$2"*) ;;
	*)
		echo "row $1 does not hold $2: $line" >&2
		return 1
		;;
	esac
	before=${line%%"$2"*}
	echo $(($(printf %s "$before" | LC_ALL=C.UTF-8 wc -m) + 1))
}

# report types one pointer report the way a terminal sends it: a code, a cell, a row, and M or m.
report() { gmux send-keys -l -t outer "$(printf '\033[<%d;%d;%d%s' "$1" "$2" "$3" "$4")"; }

# click presses and releases a button on a text of a row.
click() {
	x=$(column "$2" "$3") || return 1
	report "$1" "$x" "$2" M && report "$1" "$x" "$2" m
}

# wheel turns the wheel one step over a text of a row.
wheel() {
	x=$(column "$2" "$3") || return 1
	case $1 in
	up) report 64 "$x" "$2" M ;;
	down) report 65 "$x" "$2" M ;;
	esac
}

# drag holds the left button on one text of a row and lets go on another.
drag() {
	from=$(column "$1" "$2") || return 1
	to=$(column "$1" "$3") || return 1
	report 0 "$from" "$1" M && report 32 $((from + 1)) "$1" M &&
		report 32 "$to" "$1" M && report 0 "$to" "$1" m
}

# size prints the size of the active pane, as the information row shows it.
size() { row "$info_row" | grep -o '[0-9]*×[0-9]*'; }

# marked types a command that prints a mark, and waits for the mark.
marked() { gmux send-keys -t outer "echo $1" Enter && shows outer "^$1" >/dev/null; }

add_window() { click 0 "$windows_row" "+" && shows outer " $1:sh " >/dev/null; }

click_adds_window() {
	add_window 1 && gmux capture-pane -t outer | tail -n 2
}

tab_menu_and_switch() {
	marked first-window && add_window 1 &&
		click 2 "$windows_row" "0:sh" && shows outer "Move right" | grep -c "Move right" &&
		gmux send-keys -t outer Escape && gone outer "Move right" &&
		click 0 "$windows_row" "0:sh" && shows outer "^first-window" | grep -c "^first-window"
}

# The buttons at the right of the window row: split right, split down, zoom, close the pane, help.
window_buttons() {
	whole=$(size) &&
		click 0 "$windows_row" "┃" && gone outer " $whole " && right=$(size) && echo "right $right" &&
		click 0 "$windows_row" "━" && gone outer " $right " && below=$(size) && echo "below $below" &&
		click 0 "$windows_row" "□" && shows outer " $whole " >/dev/null && echo "zoomed $(size)" &&
		click 0 "$windows_row" "□" && shows outer " $below " >/dev/null &&
		click 0 "$windows_row" "×" && gone outer " $below " && echo "closed $(size)" &&
		click 0 "$windows_row" "?" && shows outer "gmux keys" | grep -c "gmux keys"
}

wheel_turns_windows() {
	marked w-zero && add_window 1 && marked w-one &&
		wheel up "$windows_row" "0:sh" && shows outer "^w-zero" | grep -c "^w-zero" &&
		wheel down "$windows_row" "0:sh" && shows outer "^w-one" | grep -c "^w-one"
}

middle_click_closes_window() {
	add_window 1 && click 1 "$windows_row" "1:sh" && gone outer " 1:sh " && row "$windows_row"
}

# A window takes the name of the program in front, so both tabs can be told apart.
drag_moves_window() {
	add_window 1 && gmux send-keys -t outer "sleep 300" Enter && shows outer " 0:sh  1:sleep " >/dev/null &&
		drag "$windows_row" "1:sleep" "0:sh" && shows outer " 0:sleep  1:sh " | tail -n 2
}

# The information row: the clock shows the date, and the directory copies itself.
info_row_clicks() {
	clock=$(row "$info_row" | grep -o '[0-9][0-9]:[0-9][0-9]' | tail -n 1) &&
		click 0 "$info_row" "$clock" && shows outer "[A-Z][a-z][a-z] [0-9]* [A-Z][a-z][a-z] [0-9:]* *$" | tail -n 1 | grep -c ":" &&
		click 0 "$info_row" "/" && shows outer "copied [~/]" | tail -n 1 | grep -c "copied [~/]"
}
