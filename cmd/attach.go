package cmd

import (
	"leash/session"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/sys/unix"
)

// attachSession takes over the terminal to interact with a PTY session.
// Returns when the user presses Ctrl+S to detach, or the session exits.
func attachSession(ps *session.PTYSession) error {
	fd := int(os.Stdin.Fd())

	// Save terminal state
	old, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil {
		return err
	}

	// Set raw mode with 100ms read timeout
	raw := *old
	raw.Iflag &^= unix.IGNBRK | unix.BRKINT | unix.PARMRK | unix.ISTRIP |
		unix.INLCR | unix.IGNCR | unix.ICRNL | unix.IXON
	raw.Oflag &^= unix.OPOST
	raw.Lflag &^= unix.ECHO | unix.ECHONL | unix.ICANON | unix.ISIG | unix.IEXTEN
	raw.Cflag &^= unix.CSIZE | unix.PARENB
	raw.Cflag |= unix.CS8
	raw.Cc[unix.VMIN] = 0
	raw.Cc[unix.VTIME] = 1 // 100ms timeout
	if err := unix.IoctlSetTermios(fd, unix.TCSETS, &raw); err != nil {
		return err
	}
	defer unix.IoctlSetTermios(fd, unix.TCSETS, old)

	ptyFd := int(ps.PTY.Fd())

	// Sync terminal size to PTY (triggers Claude redraw via SIGWINCH)
	syncSize := func() {
		if ws, err := unix.IoctlGetWinsize(fd, unix.TIOCGWINSZ); err == nil {
			unix.IoctlSetWinsize(ptyFd, unix.TIOCSWINSZ, ws)
		}
	}

	// Handle SIGWINCH
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH)
	sigDone := make(chan struct{})
	go func() {
		for {
			select {
			case <-sigCh:
				syncSize()
			case <-sigDone:
				return
			}
		}
	}()
	defer func() {
		signal.Stop(sigCh)
		close(sigDone)
	}()

	// Attach output and sync size
	ps.Attach(os.Stdout)
	defer ps.Detach()
	syncSize()

	// I/O loop: read stdin with timeout, forward to PTY
	buf := make([]byte, 4096)
	for {
		// Check if session ended
		select {
		case <-ps.Done():
			os.Stdout.Write([]byte("\x1b[2J\x1b[H"))
			return nil
		default:
		}

		n, err := unix.Read(fd, buf)
		if err == unix.EINTR {
			continue
		}
		if err != nil && err != unix.EAGAIN {
			return err
		}
		if n == 0 {
			continue // Timeout, loop back to check done
		}

		// Scan for Ctrl+S (0x13)
		end := n
		detach := false
		for i := 0; i < n; i++ {
			if buf[i] == 0x13 {
				end = i
				detach = true
				break
			}
		}
		if end > 0 {
			ps.PTY.Write(buf[:end])
		}
		if detach {
			os.Stdout.Write([]byte("\x1b[2J\x1b[H"))
			return nil
		}
	}
}
