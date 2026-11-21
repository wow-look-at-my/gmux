// Command gmux is a terminal multiplexer.
package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"unsafe"

	"github.com/wow-look-at-my/tml/term"
)

func ioctl(fd uintptr, req uintptr, arg unsafe.Pointer) error {
	_, _, e := syscall.Syscall(syscall.SYS_IOCTL, fd, req, uintptr(arg))
	if e != 0 {
		return e
	}
	return nil
}

func main() { probe(os.Stdout) }

func probe(out io.Writer) {
	fmt.Fprintln(out,"GOOS", runtime.GOOS, "GOARCH", runtime.GOARCH)
	fmt.Fprintln(out,"stdout terminal:", term.IsTerminal(os.Stdout))
	w, h, err := term.Size(os.Stdout)
	fmt.Fprintln(out,"size:", w, h, err)

	ptmx, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		fmt.Fprintln(out,"ptmx:", err)
		return
	}
	var n uint32
	if err := ioctl(ptmx.Fd(), 0x80045430, unsafe.Pointer(&n)); err != nil {
		fmt.Fprintln(out,"TIOCGPTN:", err)
		return
	}
	var unlock int32
	if err := ioctl(ptmx.Fd(), 0x40045431, unsafe.Pointer(&unlock)); err != nil {
		fmt.Fprintln(out,"TIOCSPTLCK:", err)
		return
	}
	name := fmt.Sprintf("/dev/pts/%d", n)
	fmt.Fprintln(out,"pts:", name)
	tty, err := os.OpenFile(name, os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		fmt.Fprintln(out,"open pts:", err)
		return
	}
	fmt.Fprintln(out,"pts terminal:", term.IsTerminal(tty))
	pw, ph, perr := term.Size(tty)
	fmt.Fprintln(out,"pts size:", pw, ph, perr)
	cmd := exec.Command("/bin/sh", "-c", "echo hello from child; tty; stty size; ps -o pid,sid,tty -p $$")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = tty, tty, tty
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	if err := cmd.Start(); err != nil {
		fmt.Fprintln(out,"start:", err)
		return
	}
	tty.Close()
	buf := make([]byte, 4096)
	for {
		k, err := ptmx.Read(buf)
		if k > 0 {
			fmt.Fprintf(out,"%q\n", buf[:k])
		}
		if err != nil {
			fmt.Fprintln(out,"read end:", err)
			break
		}
	}
	fmt.Fprintln(out,"wait:", cmd.Wait())
}
