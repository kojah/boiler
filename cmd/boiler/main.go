// Command boiler checks and generates field-complete type maintenance code.
//
//	boiler check [--config boiler.yaml] [--output PATH]
//	boiler generate [--config boiler.yaml] [--output PATH] [--check]
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/kojah/boiler"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, out, stderr io.Writer) error {
	if len(args) == 0 || args[0] != "check" && args[0] != "generate" {
		return fmt.Errorf("usage: boiler {check|generate} [--config PATH] [--output PATH] [--check]")
	}
	cmd := args[0]
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(stderr)
	config := fs.String("config", "boiler.yaml", "YAML project config or standalone .boiler file")
	target := fs.String("output", "", "select one resolved output path (default all)")
	check := fs.Bool("check", false, "report stale or missing outputs without writing (generate only)")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	if cmd == "check" && *check {
		return fmt.Errorf("--check applies to generate")
	}
	c, err := boiler.Load(*config)
	if err != nil {
		return err
	}
	artifacts, err := c.Compile(*target)
	if err != nil {
		return err
	}
	if cmd == "generate" {
		if err := c.Publish(artifacts, *check); err != nil {
			return err
		}
	}
	fmt.Fprintf(out, "Boiler %s: %d output(s) valid\n", cmd, len(artifacts))
	return nil
}
