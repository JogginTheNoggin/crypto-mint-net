// Package gui is the root of the poo GUI module: a visual layer over the
// headless poo command and nothing else (docs/gui.md).
//
// Every action the GUI takes is a poo command line run through cli.Main,
// the function the poo binary runs, so the GUI and the headless tool always
// share the same code. The GUI's packages may import only the standard
// library and pkg/cli from the core module; imports_test.go enforces it.
//
//   - shell splits a typed line into a command line.
//   - jobs runs command lines through cli.Main and keeps their output.
//   - web holds the page, embedded in the binary.
//   - server serves the page and the API on a loopback address.
//   - cmd/poo-gui is the program.
package gui
