package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/blackwell-systems/knowing/internal/daemon"
)

// cmdDaemon dispatches to daemon lifecycle subcommands.
func cmdDaemon(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: knowing daemon <start|stop|status|restart>")
	}
	switch args[0] {
	case "start":
		return cmdDaemonStart(args[1:])
	case "stop":
		return cmdDaemonStop(args[1:])
	case "status":
		return cmdDaemonStatus(args[1:])
	case "restart":
		return cmdDaemonRestart(args[1:])
	default:
		return fmt.Errorf("unknown daemon subcommand: %s", args[0])
	}
}

// cmdDaemonStart starts the knowing daemon. With --detach it forks into
// the background; otherwise it runs the serve command in the foreground.
func cmdDaemonStart(args []string) error {
	fs := flag.NewFlagSet("daemon start", flag.ExitOnError)
	detach := fs.Bool("detach", false, "Run as background daemon")
	dbPath := fs.String("db", defaultDB(), "Database path")
	addr := fs.String("addr", ":8080", "MCP server address")
	if err := fs.Parse(args); err != nil {
		return err
	}

	// Check if already running.
	if pid, running := daemon.IsDaemonRunning(); running {
		return fmt.Errorf("daemon already running (PID %d)", pid)
	}

	if !*detach {
		// Foreground mode: just run serve.
		fmt.Fprintln(os.Stderr, "Running in foreground. Use --detach to daemonize.")
		return cmdServe([]string{"-db", *dbPath, "-addr", *addr})
	}

	// Detach: re-exec ourselves with "serve" subcommand.
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("finding executable: %w", err)
	}
	cmd := exec.Command(exe, "serve", "-db", *dbPath, "-addr", *addr)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	// Redirect stdout/stderr to log file.
	logDir := filepath.Dir(daemon.PIDFilePath())
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("creating log directory: %w", err)
	}
	logPath := filepath.Join(logDir, "daemon.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("opening log file: %w", err)
	}
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		logFile.Close()
		return fmt.Errorf("starting daemon: %w", err)
	}
	logFile.Close()

	// Write the child's PID to the PID file.
	pid := cmd.Process.Pid
	pidPath := daemon.PIDFilePath()
	if err := os.MkdirAll(filepath.Dir(pidPath), 0755); err != nil {
		return fmt.Errorf("creating PID directory: %w", err)
	}
	if err := os.WriteFile(pidPath, []byte(fmt.Sprintf("%d\n", pid)), 0644); err != nil {
		return fmt.Errorf("writing PID file: %w", err)
	}

	fmt.Printf("Daemon started (PID %d)\n", pid)
	fmt.Printf("  Database: %s\n", *dbPath)
	fmt.Printf("  Address:  %s\n", *addr)
	fmt.Printf("  Log:      %s\n", logPath)
	return nil
}

// cmdDaemonStop sends SIGTERM to the running daemon and waits for it to exit.
// If the daemon does not exit within 5 seconds, SIGKILL is sent.
func cmdDaemonStop(args []string) error {
	pid, err := daemon.ReadPIDFile()
	if err != nil {
		fmt.Println("Daemon not running.")
		return nil
	}

	// Verify the process is actually alive.
	if err := syscall.Kill(pid, 0); err != nil {
		// Process not running; clean up stale PID file.
		daemon.RemovePIDFile()
		fmt.Println("Daemon not running (stale PID file removed).")
		return nil
	}

	// Send SIGTERM.
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		return fmt.Errorf("sending SIGTERM to PID %d: %w", pid, err)
	}

	// Wait up to 5 seconds for the process to exit.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(pid, 0); err != nil {
			// Process has exited.
			daemon.RemovePIDFile()
			fmt.Printf("Daemon stopped (PID %d).\n", pid)
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}

	// Process still alive after 5s; send SIGKILL.
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
		return fmt.Errorf("sending SIGKILL to PID %d: %w", pid, err)
	}
	daemon.RemovePIDFile()
	fmt.Printf("Daemon killed (PID %d, did not respond to SIGTERM).\n", pid)
	return nil
}

// cmdDaemonStatus prints the current daemon status: running or not, PID,
// and uptime derived from the PID file modification time.
func cmdDaemonStatus(args []string) error {
	pid, running := daemon.IsDaemonRunning()
	if !running {
		fmt.Println("Daemon not running.")
		os.Exit(1)
	}

	fmt.Printf("Daemon running (PID %d)\n", pid)

	// Calculate uptime from PID file modification time.
	pidPath := daemon.PIDFilePath()
	if info, err := os.Stat(pidPath); err == nil {
		uptime := time.Since(info.ModTime()).Truncate(time.Second)
		fmt.Printf("  Uptime: %s\n", uptime)
	}

	return nil
}

// cmdDaemonRestart stops the running daemon (if any) and starts a new one
// with the provided flags.
func cmdDaemonRestart(args []string) error {
	// Stop ignoring "not running" case.
	_ = cmdDaemonStop(nil)

	// Brief pause to let the port be released.
	time.Sleep(500 * time.Millisecond)

	return cmdDaemonStart(args)
}
