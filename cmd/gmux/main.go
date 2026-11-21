// Command gmux is a terminal multiplexer.
package main

import (
	"fmt"
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

func main() {
	fmt.Println("GOOS", runtime.GOOS, "GOARCH", runtime.GOARCH)
	fmt.Println("stdout terminal:", term.IsTerminal(os.Stdout))
	w, h, err := term.Size(os.Stdout)
	fmt.Println("size:", w, h, err)

	ptmx, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		fmt.Println("ptmx:", err)
		return
	}
	var n uint32
	if err := ioctl(ptmx.Fd(), 0x80045430, unsafe.Pointer(&n)); err != nil {
		fmt.Println("TIOCGPTN:", err)
		return
	}
	var unlock int32
	if err := ioctl(ptmx.Fd(), 0x40045431, unsafe.Pointer(&unlock)); err != nil {
		fmt.Println("TIOCSPTLCK:", err)
		return
	}
	name := fmt.Sprintf("/dev/pts/%d", n)
	fmt.Println("pts:", name)
	tty, err := os.OpenFile(name, os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		fmt.Println("open pts:", err)
		return
	}
	fmt.Println("pts terminal:", term.IsTerminal(tty))
	pw, ph, perr := term.Size(tty)
	fmt.Println("pts size:", pw, ph, perr)
	cmd := exec.Command("/bin/sh", "-c", "echo hello from child; tty; stty size; ps -o pid,sid,tty -p $$")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = tty, tty, tty
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	if err := cmd.Start(); err != nil {
		fmt.Println("start:", err)
		return
	}
	tty.Close()
	buf := make([]byte, 4096)
	for {
		k, err := ptmx.Read(buf)
		if k > 0 {
			fmt.Printf("%q\n", buf[:k])
		}
		if err != nil {
			fmt.Println("read end:", err)
			break
		}
	}
	fmt.Println("wait:", cmd.Wait())
}
