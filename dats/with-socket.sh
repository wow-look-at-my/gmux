#!/bin/sh
# Runs one script against a gmux server of its own.
set -u

# The staged binary is read-only, and a portable executable has to be able to prepare itself where it runs.
work=$(mktemp -d)
cp "$GO_TOOLCHAIN_DATS_BUILD_DIR/gmux" "$work/gmux"
chmod +x "$work/gmux"

export GMUX_SOCKET="$work/s.sock"
export SHELL=/bin/sh
export PS1='$ '
unset GMUX

gmux() { "$work/gmux" "$@"; }

# shows waits until the active pane of a session holds a text, then prints the pane.
shows() {
	tries=0
	until gmux pane capture -t "$1" | grep -q -- "$2"; do
		tries=$((tries + 1))
		if [ "$tries" -gt 100 ]; then
			echo "the pane of $1 never showed $2" >&2
			gmux pane capture -t "$1" >&2
			return 1
		fi
		sleep 0.1
	done
	gmux pane capture -t "$1"
}

eval "$1"
status=$?
gmux server kill >/dev/null 2>&1
rm -rf "$work"
exit "$status"
