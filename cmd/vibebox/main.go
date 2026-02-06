package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"vibebox/internal/app"
	"vibebox/internal/config"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	a := app.New(os.Stdout, os.Stderr)
	if len(args) == 0 {
		printRootHelp()
		return nil
	}

	switch args[0] {
	case "init":
		fs := flag.NewFlagSet("init", flag.ContinueOnError)
		var nonInteractive bool
		var imageID string
		var provider string
		var cpus int
		var ramMB int
		var diskGB int
		fs.BoolVar(&nonInteractive, "non-interactive", false, "disable TUI wizard")
		fs.StringVar(&imageID, "image-id", "", "official image id")
		fs.StringVar(&provider, "provider", string(config.ProviderAuto), "provider: off|apple-vm|docker|auto")
		fs.IntVar(&cpus, "cpus", 2, "vm CPU count")
		fs.IntVar(&ramMB, "ram-mb", 2048, "vm memory in MiB")
		fs.IntVar(&diskGB, "disk-gb", 20, "vm disk in GiB")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		return a.Init(ctx, app.InitOptions{
			NonInteractive: nonInteractive,
			ImageID:        imageID,
			Provider:       config.Provider(provider),
			CPUs:           cpus,
			RAMMB:          ramMB,
			DiskGB:         diskGB,
		})
	case "up":
		fs := flag.NewFlagSet("up", flag.ContinueOnError)
		var provider string
		fs.StringVar(&provider, "provider", "", "override provider: off|apple-vm|docker|auto")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		return a.Up(ctx, app.UpOptions{Provider: config.Provider(provider)})
	case "images":
		if len(args) == 1 {
			printImagesHelp()
			return nil
		}
		sub := args[1]
		switch sub {
		case "list":
			return a.ImagesList()
		case "upgrade":
			fs := flag.NewFlagSet("images upgrade", flag.ContinueOnError)
			var imageID string
			fs.StringVar(&imageID, "image-id", "", "image id to refresh")
			if err := fs.Parse(args[2:]); err != nil {
				return err
			}
			return a.ImagesUpgrade(ctx, app.UpgradeOptions{ImageID: imageID})
		default:
			printImagesHelp()
			return fmt.Errorf("unknown images subcommand: %s", sub)
		}
	case "help", "--help", "-h":
		printRootHelp()
		return nil
	default:
		printRootHelp()
		return fmt.Errorf("unknown command: %s", args[0])
	}
}

func printRootHelp() {
	fmt.Print(`vibebox - sandbox launcher for LLM agents

Usage:
  vibebox init [flags]           Initialize project sandbox
  vibebox up [--provider ...]    Start sandbox shell
  vibebox images list            List official VM images
  vibebox images upgrade         Refresh/download an image

Common flags:
  --provider off|apple-vm|docker|auto
`)
}

func printImagesHelp() {
	fmt.Print(`vibebox images commands:
  vibebox images list
  vibebox images upgrade [--image-id <id>]`)
}
