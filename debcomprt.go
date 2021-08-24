package main

import (
	"bufio"
	"errors"
	"io"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/urfave/cli/v2"
)

const (
	comprtConfigFile    = "comprtconfig"
	comprtIncludeFile   = "comprtinc.txt"
	defaultDebianMirror = "http://ftp.us.debian.org/debian/"
	defaultUbuntuMirror = "http://archive.ubuntu.com/ubuntu/"
)

var defaultMirrorMappings = map[string]string{
	"buster":  defaultDebianMirror,
	"focal":   defaultUbuntuMirror,
	"hirsute": defaultUbuntuMirror,
}

// A type used to store command flag argument values and argument values.
type cmdArgs struct {
	passthrough            bool
	quiet                  bool
	useShellImplementation bool
	helpFlagPassedIn       bool
	comprtIncludesPath     string
	comprtConfigPath       string
	codeName               string
	target                 string
	mirror                 string
	passThroughFlags       []string
}

// A custom callback handler in the event improper cli
// flag/flag arguments/arguments are passed in.
var CustomOnUsageErrorFunc cli.OnUsageErrorFunc = func(context *cli.Context, err error, isSubcommand bool) error {
	cli.ShowAppHelp(context)
	log.Fatal(err)
	return err
}

// Copy the src file to dst. Any existing file will not be overwritten and will not
// copy file attributes.
func copy(src, dst string) error {
	// inspired from:
	// https://stackoverflow.com/questions/21060945/simple-way-to-copy-a-file#answer-21061062
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, syscall.O_CREAT|syscall.O_EXCL|syscall.O_WRONLY, fs.FileMode(0777))
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	if err != nil {
		return err
	}
	return out.Close()
}

// Looks to see if the string is in the string array.
func stringInArr(strArg string, arr []string) bool {
	for _, value := range arr {
		if value == strArg {
			return true
		}
	}
	return false
}

// Looks to see if the strings are in the string array.
func stringsInArr(strArgs []string, arr []string) bool {
	for _, value := range strArgs {
		if stringInArr(value, arr) {
			return true
		}
	}
	return false
}

// getComprtIncludes reads in the comprt includes file and adds the
// discovered packages into includePkgs.
func getComprtIncludes(includePkgs *[]string, args *cmdArgs) error {
	// inspired by:
	// https://stackoverflow.com/questions/8757389/reading-a-file-line-by-line-in-go/16615559#16615559
	file, err := os.Open(args.comprtIncludesPath)
	if err != nil {
		// the comprt includes is optional
		return nil
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	// optionally, resize scanner's capacity for lines over 64K, see next example
	for scanner.Scan() {
		*includePkgs = append(*includePkgs, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return err
	}
	return nil
}

// parseCmdArgs interprets the command arguments passed in. Saving particular
// flag/flag arguments of interest into 'args'.
func parseCmdArgs(args *cmdArgs) {
	var localOsArgs []string = os.Args
	var flagArg bool = false

	// parses out flags to pass to debootstrap
	for index, value := range localOsArgs {
		if index < 1 {
			continue
		} else if value == "--" {
			break
		} else if stringsInArr([]string{"-h", "-help", "--help"}, localOsArgs) {
			args.helpFlagPassedIn = true
		} else if stringInArr(value, []string{"-passthrough", "--passthrough"}) {
			// index + 1 to ignoring iterating over passthrough flag
			for passthroughIndex, passthroughValue := range localOsArgs[index+1:] {
				if strings.HasPrefix(passthroughValue, "-") {
					args.passThroughFlags = append(args.passThroughFlags, passthroughValue)
					flagArg = true
				} else if !flagArg {
					// index + 1 to keep passthrough flag
					// index + passthroughIndex + 1 to truncate final flag argument
					localOsArgs = append(localOsArgs[:index+1], localOsArgs[index+passthroughIndex+1:]...)
					break
				} else {
					args.passThroughFlags = append(args.passThroughFlags, passthroughValue)
					flagArg = false
				}
			}
			break
		}
	}

	app := &cli.App{
		Name:            "debcomprt",
		Usage:           "creates debian compartments, an undyling 'target' generated from debootstrap",
		UsageText:       "debcomprt [global options] CODENAME TARGET [MIRROR]",
		Description:     "[WARNING] this tool's cli is not fully POSIX compliant, so POSIX utility cli behavior may not always occur",
		HideHelpCommand: true,
		OnUsageError:    CustomOnUsageErrorFunc,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "passthrough",
				Value:       false,
				Usage:       "passes the rest of the flag/flag arguments to debootstrap",
				Destination: &args.passthrough,
			},
			&cli.BoolFlag{
				Name:        "quiet",
				Aliases:     []string{"q"},
				Value:       false,
				Usage:       "quiet (no output)",
				Destination: &args.quiet,
			},
			&cli.BoolFlag{
				Name:        "use-shell-implementation",
				Aliases:     []string{"s"},
				Value:       false,
				Usage:       "uses the shell implemented debootstrap",
				Destination: &args.useShellImplementation,
			},
			&cli.PathFlag{
				Name:        "includes-path",
				Aliases:     []string{"i"},
				Value:       args.comprtIncludesPath,
				Usage:       "alternative path to comprt includes file",
				Destination: &args.comprtIncludesPath,
			},
			&cli.PathFlag{
				Name:        "config-path",
				Aliases:     []string{"c"},
				Value:       args.comprtConfigPath,
				Usage:       "alternative path to comptr config file",
				Destination: &args.comprtConfigPath,
			},
		},
		Action: func(context *cli.Context) error {
			if context.NArg() < 1 { // CODENAME
				cli.ShowAppHelp(context)
				log.Fatal(errors.New("CODENAME argument is required"))
			}

			if context.NArg() < 2 { // TARGET
				cli.ShowAppHelp(context)
				log.Fatal(errors.New("TARGET argument is required"))
			} else if _, err := os.Stat(context.Args().Get(1)); errors.Is(err, fs.ErrNotExist) {
				log.Fatal(err)
			}

			if context.NArg() < 3 { // MIRROR
				if _, ok := defaultMirrorMappings[context.Args().Get(0)]; !ok {
					log.Fatal(errors.New("no default MIRROR could be determined"))
				}
				args.mirror = defaultMirrorMappings[context.Args().Get(0)]
			} else {
				args.mirror = context.Args().Get(2)
			}

			args.codeName = context.Args().Get(0)
			args.target = context.Args().Get(1)
			return nil
		},
	}
	app.Run(localOsArgs)
	// Because for some reason github.com/urfave/cli/v2@v2.3.0 does not have a way to
	// eject if a variant of 'help' is passed in!
	if args.helpFlagPassedIn {
		os.Exit(0)
	}
}

func main() {
	var includePkgs, debootstrap []string
	var targetComprtConfigPath string = filepath.Join("/", comprtConfigFile)
	args := &cmdArgs{ // sets defaults
		passthrough:            false,
		quiet:                  false,
		useShellImplementation: false,
		comprtIncludesPath:     filepath.Join(".", comprtIncludeFile),
		comprtConfigPath:       filepath.Join(".", comprtConfigFile),
	}
	parseCmdArgs(args)

	debootstrap = append(debootstrap, "cdebootstrap")
	if args.useShellImplementation {
		debootstrap[0] = "debootstrap"
	}

	if err := getComprtIncludes(&includePkgs, args); err != nil {
		log.Fatal(err)
	}

	if includePkgs != nil {
		debootstrap = append(debootstrap, "--include="+strings.Join(includePkgs, ","))
	}

	if args.passThroughFlags != nil {
		debootstrap = append(debootstrap, args.passThroughFlags...)
	}

	debootstrap = append(debootstrap, args.codeName, args.target, args.mirror)

	debootstrapPath, err := exec.LookPath(debootstrap[0])
	if err != nil {
		log.Fatal(err)
	}

	if err := copy(args.comprtConfigPath, filepath.Join(args.target, targetComprtConfigPath)); err != nil {
		log.Fatal(err)
	}

	// inspired by:
	// https://stackoverflow.com/questions/39173430/how-to-print-the-realtime-output-of-running-child-process-in-go
	debootstrapCmd := exec.Command(debootstrapPath, debootstrap[1:]...)
	debootstrapCmd.Stdout = os.Stdout
	debootstrapCmd.Stderr = os.Stderr
	if err := debootstrapCmd.Start(); err != nil {
		log.Fatal(err)
	}
	if err := debootstrapCmd.Wait(); err != nil {
		log.Fatal(err)
	}

	if err := syscall.Chroot(args.target); err != nil {
		log.Fatal(err)
	}

	if err := syscall.Chdir("/"); err != nil { // makes sh happy, otherwise getcwd() for sh fails
		log.Fatal(err)
	}

	shPath, err := exec.LookPath("sh")
	if err != nil {
		log.Fatal(err)
	}
	comprtConfigFileCmd := exec.Command(shPath, targetComprtConfigPath)
	comprtConfigFileCmd.Stdout = os.Stdout
	comprtConfigFileCmd.Stderr = os.Stderr
	if err := comprtConfigFileCmd.Start(); err != nil {
		log.Fatal(err)
	}
	if err := comprtConfigFileCmd.Wait(); err != nil {
		log.Fatal(err)
	}

	os.Exit(0)
}
