package main

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/ansel1/merry"
	rollbarAPI "github.com/stvp/rollbar"
	"golang.org/x/crypto/ssh/terminal"
)

// Stdout is used to mock stdout for testing
var Stdout io.Writer = os.Stdout

// Stderr is to mock stderr for testing
var Stderr io.Writer = os.Stderr

// ExitFn is used to mock os.Exit
var ExitFn = os.Exit

// Debugging is HEROKU_DEBUG
var Debugging = isDebugging()

// Exit just calls os.Exit, but can be mocked out for testing
func Exit(code int) {
	ExitFn(code)
}

// Err just calls `fmt.Fprint(Stderr, a...)` but can be mocked out for testing.
func Err(a ...interface{}) {
	fmt.Fprint(Stderr, a...)
}

// Errf just calls `fmt.Fprintf(Stderr, a...)` but can be mocked out for testing.
func Errf(format string, a ...interface{}) {
	fmt.Fprintf(Stderr, format, a...)
}

// Errln just calls `fmt.Fprintln(Stderr, a...)` but can be mocked out for testing.
func Errln(a ...interface{}) {
	fmt.Fprintln(Stderr, a...)
}

// Print is used to replace `fmt.Print()` but can be mocked out for testing.
func Print(a ...interface{}) {
	fmt.Fprint(Stdout, a...)
}

// Printf is used to replace `fmt.Printf()` but can be mocked out for testing.
func Printf(format string, a ...interface{}) {
	fmt.Fprintf(Stdout, format, a...)
}

// Println is used to replace `fmt.Println()` but can be mocked out for testing.
func Println(a ...interface{}) {
	fmt.Fprintln(Stdout, a...)
}

// Debugln is used to print debugging information
// It will be added to the logfile in ~/.cache/heroku/error.log and stderr if HEROKU_DEBUG is set.
func Debugln(a ...interface{}) {
	if Debugging {
		fmt.Fprintln(Stderr, a...)
	}
}

// Debugf is used to print debugging information
// It will be added to the logfile in ~/.cache/heroku/error.log and stderr if HEROKU_DEBUG is set.
func Debugf(format string, a ...interface{}) {
	if Debugging {
		fmt.Fprintf(Stderr, format, a...)
	}
}

// WarnIfError is a helper that prints out formatted error messages
// it will emit to rollbar
// it does not exit
func WarnIfError(err error) {
	if err == nil {
		return
	}
	err = merry.Wrap(err)
	Warn(err.Error())
	Debugln(merry.Details(err))
	rollbar(err, "warning")
}

// Warn shows a message with excalamation points prepended to stderr
func Warn(msg string) {
	prefix := " " + yellow(ErrorArrow) + "    "
	msg = strings.TrimSpace(msg)
	msg = strings.Join(strings.Split(msg, "\n"), "\n"+prefix)
	Errln(prefix + msg)
}

// Error shows a message with excalamation points prepended to stderr
func Error(msg string) {
	prefix := " " + red(ErrorArrow) + "    "
	msg = strings.TrimSpace(msg)
	msg = strings.Join(strings.Split(msg, "\n"), "\n"+prefix)
	Errln(prefix + msg)
}

// ErrorArrow is the triangle or bang that prefixes errors
var ErrorArrow = errorArrow()

func errorArrow() string {
	if windows() {
		return "!"
	}
	return "▸"
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

// LogIfError logs out an error if one arises
func LogIfError(e error) {
	if e != nil {
		Debugln(e.Error())
		Debugln(string(debug.Stack()))
		rollbar(e, "info")
	}
}

// ONE is the string 1
const ONE = "1"

func isDebugging() bool {
	debug := strings.ToUpper(os.Getenv("HEROKU_DEBUG"))
	if debug == "TRUE" || debug == ONE {
		return true
	}
	return false
}

func yellow(s string) string {
	if supportsColor() && !windows() {
		return "\x1b[33m" + s + "\x1b[39m"
	}
	return s
}

func red(s string) string {
	if supportsColor() && !windows() {
		return "\x1b[31m" + s + "\x1b[39m"
	}
	return s
}

func windows() bool {
	return runtime.GOOS == WINDOWS
}

func istty() bool {
	return terminal.IsTerminal(int(os.Stdout.Fd()))
}

func supportsColor() bool {
	if !istty() {
		return false
	}
	for _, arg := range os.Args {
		if arg == "--no-color" {
			return false
		}
	}
	if os.Getenv("COLOR") == "false" {
		return false
	}
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	return true
}

func handleSignal(s os.Signal, fn func()) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, s)
	go func() {
		<-c
		fn()
	}()
}

var crashing = false

func handlePanic() {
	if crashing {
		// if already crashing just let the error bubble
		// or else potential fork-bomb
		return
	}
	crashing = true
	if rec := recover(); rec != nil {
		err, ok := rec.(error)
		if !ok {
			err = merry.New(rec.(string))
		}
		err = merry.Wrap(err)
		Error(err.Error())
		Debugln(merry.Details(err))
		rollbar(err, "error")
		Exit(1)
	}
}

func rollbar(err error, level string) {
	if os.Getenv("TESTING") == ONE {
		return
	}
	rollbarAPI.Platform = "client"
	rollbarAPI.Token = "d40104ae6fa8477dbb6907370231d7d8"
	rollbarAPI.Environment = Channel
	rollbarAPI.ErrorWriter = nil
	rollbarAPI.CodeVersion = GitSHA
	var cmd string
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	fields := []*rollbarAPI.Field{
		{"version", Version},
		{"os", runtime.GOOS},
		{"arch", runtime.GOARCH},
		{"command", cmd},
	}
	rollbarAPI.Error(level, err, fields...)
	rollbarAPI.Wait()
}