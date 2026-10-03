package lunex

import (
	"fmt"
	"lunex/internal/adaptor"
	"lunex/internal/meta"
	"os"
	"strings"
)

type helpSection struct {
	Title string
	Rows  []string
}

type helpPage struct {
	Name     string
	Usage    string
	Summary  string
	Aliases  string
	Sections []helpSection
}

var helpPages = map[string]helpPage{
	"run": {
		Name:    "run",
		Usage:   "lunex run <file.lx|file.nax> [--emit ast|ir] [-- args...]",
		Summary: "Compile and run a Lunex source file or NAX archive.",
		Sections: []helpSection{
			{Title: "Arguments", Rows: []string{
				"<file.lx|file.nax>    Source file or compiled NAX archive to execute.",
				"--emit ast|ir         Print the generated AST or IR instead of running it.",
				"--                    Everything after this marker is passed to the Lunex program.",
			}},
			{Title: "Behavior", Rows: []string{
				"Lunex compiles .lx files before execution and can execute .nax archives directly.",
				"The normal cache may be used unless --no-cache is enabled.",
				"Use `lunex check` when you only want validation without execution.",
			}},
		},
	},
	"start": {
		Name:    "start",
		Usage:   "lunex start [args...]",
		Summary: "Run the project entry point declared by lunex.toml.",
		Sections: []helpSection{
			{Title: "Project", Rows: []string{
				"Lunex searches the current directory for lunex.toml and reads its entry setting.",
				"When no entry is configured, main.lx is used.",
			}},
			{Title: "Arguments", Rows: []string{
				"Any remaining arguments are forwarded to the project entry point.",
			}},
		},
	},
	"debug": {
		Name:    "debug",
		Usage:   "lunex debug <file.lx> [args...]",
		Summary: "Compile and run a source file with detailed execution diagnostics.",
		Sections: []helpSection{
			{Title: "Behavior", Rows: []string{
				"Enables debug output and keeps the full compile diagnostics visible.",
				"Runtime failures are reported together with the execution trace available to Lunex.",
			}},
			{Title: "Arguments", Rows: []string{
				"<file.lx>             Lunex source file to compile and run.",
				"[args...]             Arguments forwarded to the program.",
			}},
		},
	},
	"-e": {
		Name:    "-e / execute",
		Usage:   "lunex -e \"<code>\"",
		Summary: "Compile and run a Lunex snippet without creating a source file.",
		Aliases: "execute",
		Sections: []helpSection{
			{Title: "Input", Rows: []string{
				"The next argument is treated as the complete source text for one execution.",
			}},
			{Title: "Related", Rows: []string{
				"Use `lunex repl` for an interactive session with persistent definitions.",
			}},
		},
	},
	"execute": {
		Name:    "-e / execute",
		Usage:   "lunex execute \"<code>\"",
		Summary: "Alias of -e. Compile and run a Lunex snippet directly.",
		Aliases: "-e",
		Sections: []helpSection{
			{Title: "Input", Rows: []string{
				"The next argument is treated as the complete source text for one execution.",
			}},
		},
	},
	"repl": {
		Name:    "repl",
		Usage:   "lunex repl",
		Summary: "Start the interactive Lunex REPL.",
		Sections: []helpSection{
			{Title: "REPL commands", Rows: []string{
				".help                  Show REPL commands.",
				".exit, .quit          Leave the REPL.",
				".clear                Reset the current session.",
				".vars                 List defined names.",
				".history              Show session history.",
				".load <file>          Load and evaluate a .lx file.",
				".type <expr>          Show the type of an expression.",
				"Ctrl+D                Leave the REPL with EOF.",
			}},
		},
	},
	"check": {
		Name:    "check",
		Usage:   "lunex check <file.lx>",
		Summary: "Validate a Lunex source file without running it.",
		Sections: []helpSection{
			{Title: "Checks", Rows: []string{
				"Parses the source, resolves modules, performs semantic analysis, and reports diagnostics.",
				"The program is not executed and no runtime result is produced.",
			}},
		},
	},
	"see_errors": {
		Name:    "see_errors",
		Usage:   "lunex see_errors <file.lx>",
		Summary: "Show the compiler errors for a source file without running it.",
		Aliases: "see-errors, errors",
		Sections: []helpSection{
			{Title: "Behavior", Rows: []string{
				"Compiles the selected source and prints every compiler diagnostic in the standard Lunex format.",
				"When compilation succeeds, Lunex reports that the file has no errors.",
			}},
		},
	},
	"init": {
		Name:    "init",
		Usage:   "lunex init [name]",
		Summary: "Create a new Lunex project directory with a starter layout.",
		Sections: []helpSection{
			{Title: "Created", Rows: []string{
				"lunex.toml            Project manifest.",
				"main.lx               Project entry point.",
				"src/                  Local source module directory.",
				".gitignore            Default Lunex build and cache ignore rules.",
			}},
			{Title: "Templates", Rows: []string{
				"http_server           HTTP server project.",
				"database              SQLite-backed database project.",
				"website               Static website project.",
				"Use `lunex init <template> <name>` to select one.",
			}},
			{Title: "Name", Rows: []string{
				"When omitted, the current directory name is used as the project name.",
			}},
		},
	},
	"pack": {
		Name:    "pack",
		Usage:   "lunex pack <file.lx|directory> [--source] [-o output.nax]",
		Summary: "Validate and bundle Lunex source into a NAX archive.",
		Sections: []helpSection{
			{Title: "Input", Rows: []string{
				"A single .lx file or a project/source directory can be packed.",
			}},
			{Title: "Options", Rows: []string{
				"--source              Keep recoverable source information in the NAX archive.",
				"-o, --output <path>   Choose the output archive path.",
			}},
			{Title: "Validation", Rows: []string{
				"Lunex validates imports and compiler/checker invariants before writing the archive.",
				"Use `lunex unpack` to recover source entries from an archive when source data is present.",
			}},
		},
	},
	"unpack": {
		Name:    "unpack",
		Usage:   "lunex unpack <file.nax>",
		Summary: "Recover source entries from a NAX archive into a directory.",
		Sections: []helpSection{
			{Title: "Output", Rows: []string{
				"The output directory is created beside the archive using the archive base name.",
				"Only recoverable source entries can be restored from the archive.",
			}},
		},
	},
	"set": {
		Name:    "set",
		Usage:   "lunex set cache <dir>\nlunex set cache reset",
		Summary: "Configure the on-disk Lunex runtime cache directory.",
		Sections: []helpSection{
			{Title: "Cache", Rows: []string{
				"set cache <dir>      Set a custom cache directory and create it when needed.",
				"set cache reset      Return to the default cache location.",
			}},
			{Title: "Related", Rows: []string{
				"Use `lunex cache` to inspect the current disk and memory cache state.",
			}},
		},
	},
	"cache": {
		Name:    "cache",
		Usage:   "lunex cache [clear]",
		Summary: "Inspect or clear the on-disk runtime cache.",
		Sections: []helpSection{
			{Title: "Commands", Rows: []string{
				"cache                 Show cache location and cache statistics.",
				"cache clear           Remove the on-disk cache.",
			}},
			{Title: "Related", Rows: []string{
				"Use `lunex memcache` for the process-local in-memory bytecode cache.",
				"Use `lunex --no-cache` to compile fresh and store nothing for that run.",
			}},
		},
	},
	"memcache": {
		Name:    "memcache",
		Usage:   "lunex memcache [clear]",
		Summary: "Inspect or clear the in-process bytecode cache.",
		Sections: []helpSection{
			{Title: "Commands", Rows: []string{
				"memcache              Show in-memory cache entries and size.",
				"memcache clear        Clear the current process memory cache.",
			}},
			{Title: "Lifetime", Rows: []string{
				"This cache exists only while the current Lunex process is running.",
			}},
		},
	},
	"platform": {
		Name:    "platform",
		Usage:   "lunex platform",
		Summary: "Show platform and adapter diagnostics for the current build.",
		Sections: []helpSection{
			{Title: "Output", Rows: []string{
				"Reports the active platform adapter and related runtime capabilities.",
				"This is useful when checking behavior across Android, desktop, and other targets.",
			}},
		},
	},
	"runtimes": {
		Name:    "runtimes",
		Usage:   "lunex runtimes",
		Summary: "Show the execution engines available in this Lunex build.",
		Sections: []helpSection{
			{Title: "Output", Rows: []string{
				"Lists the execution engines that can be used by the current binary.",
			}},
		},
	},
	"bench": {
		Name:    "bench",
		Usage:   "lunex bench <file.lx>",
		Summary: "Compile and run a source file while reporting timing information.",
		Sections: []helpSection{
			{Title: "Output", Rows: []string{
				"Reports compilation time and runtime duration.",
				"A missing .lx suffix is accepted and appended automatically.",
			}},
		},
	},
	"env": {
		Name:    "env",
		Usage:   "lunex env",
		Summary: "Show Lunex module stores and current project state.",
		Sections: []helpSection{
			{Title: "Output", Rows: []string{
				"Global module store and installed version count.",
				"Local project module store and installed version count.",
				"Whether lunex.toml and lunex.lock were found.",
			}},
		},
	},
	"link": {
		Name:    "link",
		Usage:   "lunex link",
		Summary: "Expose the current project's declared executable commands globally.",
		Sections: []helpSection{
			{Title: "Behavior", Rows: []string{
				"Reads the project's [project.bin] declarations and creates command shims in ~/.lunex/bin.",
				"The project is linked for development instead of being installed first.",
			}},
			{Title: "PATH", Rows: []string{
				"Add ~/.lunex/bin to PATH to run linked commands directly.",
			}},
		},
	},
	"install": {
		Name:    "install",
		Usage:   "lunex install\nlunex install -g <url>[@version] [more urls...]\nlunex install -l <url>[@version] [more urls...]",
		Summary: "Install dependencies from lunex.toml or install a library directly.",
		Aliases: "i",
		Sections: []helpSection{
			{Title: "Modes", Rows: []string{
				"install               Install every library declared in lunex.toml into the local project store.",
				"install -g <url>      Install a library into the global ~/.lunex store.",
				"install -l <url>      Install a library into the current project's ./.lunex store.",
				"More URLs can be supplied in the same command.",
			}},
			{Title: "Versioning", Rows: []string{
				"A URL may include an @version selector when the package source supports version resolution.",
				"Installed versions remain isolated so different dependencies can use different versions.",
			}},
		},
	},
	"add": {
		Name:    "add",
		Usage:   "lunex add <url>[@version] [more urls...]",
		Summary: "Add libraries to lunex.toml and install them locally.",
		Sections: []helpSection{
			{Title: "Behavior", Rows: []string{
				"Requires a lunex.toml in the current project.",
				"Each supplied dependency is installed and recorded in the manifest.",
			}},
		},
	},
	"remove": {
		Name:    "remove",
		Usage:   "lunex remove <library> [more libraries...]",
		Summary: "Remove installed libraries by name.",
		Aliases: "uninstall, rm",
		Sections: []helpSection{
			{Title: "Arguments", Rows: []string{
				"One or more installed library names can be supplied.",
			}},
		},
	},
	"update": {
		Name:    "update",
		Usage:   "lunex update [library] [more libraries...]",
		Summary: "Re-resolve installed libraries against the current lunex.toml.",
		Aliases: "upgrade",
		Sections: []helpSection{
			{Title: "Modes", Rows: []string{
				"update                Re-resolve all installed libraries with matching lunex.toml entries.",
				"update <library>      Re-resolve only the named installed library.",
			}},
			{Title: "Notes", Rows: []string{
				"A library must have a lunex.toml declaration to be re-resolved by the project.",
				"The resolved lock information is refreshed by the package installation process.",
			}},
		},
	},
	"list": {
		Name:    "list",
		Usage:   "lunex list",
		Summary: "List installed libraries and their installation scope.",
		Aliases: "ls",
		Sections: []helpSection{
			{Title: "Output", Rows: []string{
				"Each installed library is shown with its version when available.",
				"The scope is shown as local or global.",
			}},
		},
	},
	"version": {
		Name:    "version",
		Usage:   "lunex version",
		Summary: "Print the current Lunex version.",
		Aliases: "--version, -v",
		Sections: []helpSection{
			{Title: "Output", Rows: []string{
				"Prints the version reported by the Lunex metadata package.",
			}},
		},
	},
	"help": {
		Name:    "help",
		Usage:   "lunex help [command]",
		Summary: "Show the command reference or detailed help for one command.",
		Sections: []helpSection{
			{Title: "Modes", Rows: []string{
				"help                  Show the main command reference.",
				"help <command>        Show detailed help for one command.",
			}},
			{Title: "Command names", Rows: []string{
				"Aliases are accepted when they have their own command help page.",
				"Use `lunex help` to see the complete list of available commands and groups.",
			}},
		},
	},
}

func printHelp() {
	if adaptor.TerminalWidth() < 64 {
		printHelpCompact()
		return
	}

	fmt.Printf("Lunex %s\n", meta.Version())
	fmt.Println()
	fmt.Println("USAGE")
	fmt.Println("  lunex <command> [options]")
	fmt.Println("  lunex <file.lx|file.nax>")
	fmt.Println()
	fmt.Println("COMMANDS")
	printHelpGroup("Project and execution", []string{
		"run <file>                 Compile and run a source file or NAX archive",
		"start                      Run the project entry from lunex.toml",
		"debug <file>               Run with detailed compile and execution diagnostics",
		"-e \"<code>\"              Run a source snippet directly",
		"repl                       Start the interactive REPL",
		"check <file>               Check a source file without running it",
		"see_errors <file>          Show detailed compiler errors",
		"bench <file>               Run with timing output",
	})
	printHelpGroup("Project files", []string{
		"init [name]                Create a new project",
		"init <template> <name>     Create a project from a template",
		"pack <file|directory>      Validate and create a .nax archive",
		"unpack <file.nax>          Recover source entries from a NAX archive",
	})
	printHelpGroup("Dependencies", []string{
		"install                    Install lunex.toml dependencies",
		"install -g <url>           Install a library globally",
		"install -l <url>           Install a library locally",
		"add <url>                  Add and install a dependency",
		"remove <library>           Remove an installed library",
		"update [library]           Re-resolve installed libraries",
		"list                       List installed libraries",
	})
	printHelpGroup("Runtime and tools", []string{
		"cache [clear]              Show or clear the disk cache",
		"memcache [clear]           Show or clear the process memory cache",
		"set cache <dir>            Configure the disk cache location",
		"platform                   Show platform and adapter diagnostics",
		"runtimes                   Show available execution engines",
		"env                        Show module stores and project status",
		"link                       Link project bin commands",
		"version                    Print the Lunex version",
		"help [command]             Show general or command-specific help",
	})

	fmt.Println("GLOBAL OPTIONS")
	printHelpGroup("", []string{
		"--debug, -d               Enable debug output",
		"--verbose, -V             Enable verbose debug output",
		"--no-cache                Compile fresh and store nothing for that run",
	})

	fmt.Println("MODULES")
	printHelpGroup("", []string{
		"@import(\"std.io\")       Standard library module",
		"@import(\"pkg-name\")     Installed library",
		"@fimport(\"./file.nax\")  Local NAX archive",
		"@fimport(\"./file.lx\")   Local Lunex source",
	})

	fmt.Println("NATIVE FFI")
	printHelpGroup("", []string{
		"lunex ffi = on run app.lx     Enable native FFI for one process",
		"lunex ffi = off run app.lx    Explicitly disable native FFI",
		"FFI is disabled unless the CLI prefix is present.",
	})

	fmt.Println("STANDARD LIBRARY")
	fmt.Println("  io, fs, http, crypto, db, ws, jwt, json, math, datetime")
	fmt.Println("  os, regex, env, ffi, utils")
	fmt.Println()
	fmt.Println("DEPENDENCY STORAGE")
	fmt.Println("  Versions are isolated on disk. lunex.lock records resolved versions, sources, and hashes.")
	fmt.Println()
	fmt.Println("GETTING HELP")
	fmt.Println("  lunex help <command>       Read the full guide for one command")
	fmt.Println()
}

func printHelpCompact() {
	fmt.Printf("Lunex %s\n", meta.Version())
	fmt.Println("Usage: lunex <command> [options]")
	fmt.Println()
	fmt.Println("Commands:")
	compact := []string{
		"run <file>", "start", "debug <file>", "-e \"<code>\"", "repl", "check <file>",
		"see_errors <file>", "init", "pack <file|directory>", "unpack <file.nax>", "install",
		"add <url>", "remove <library>", "update [library]", "list", "cache [clear]", "memcache [clear]",
		"set cache <dir>", "platform", "runtimes", "bench <file>", "env", "link", "version", "help [command]",
	}
	for _, cmd := range compact {
		fmt.Printf("  %s\n", cmd)
	}
	fmt.Println()
	fmt.Println("Run `lunex help <command>` for the complete guide to a command.")
}

func printHelpGroup(title string, rows []string) {
	if title != "" {
		fmt.Printf("  %s\n", title)
	}
	for _, row := range rows {
		fmt.Printf("    %s\n", row)
	}
	fmt.Println()
}

func printCommandHelp(name string) {
	key := strings.ToLower(strings.TrimSpace(name))
	page, ok := helpPages[key]
	if !ok {
		fmt.Fprintf(os.Stderr, "Unknown help topic: %s\n", name)
		fmt.Fprintln(os.Stderr, "Run `lunex help` to see available commands.")
		return
	}

	fmt.Printf("Lunex %s\n", meta.Version())
	fmt.Printf("Command: %s\n", page.Name)
	fmt.Printf("%s\n", page.Summary)
	fmt.Println()
	fmt.Println("USAGE")
	for _, line := range strings.Split(page.Usage, "\n") {
		fmt.Printf("  %s\n", line)
	}
	fmt.Println()
	if page.Aliases != "" {
		fmt.Printf("ALIASES\n  %s\n\n", page.Aliases)
	}
	for _, section := range page.Sections {
		fmt.Println(strings.ToUpper(section.Title))
		for _, row := range section.Rows {
			fmt.Printf("  %s\n", row)
		}
		fmt.Println()
	}
	fmt.Println("RELATED")
	fmt.Printf("  Run `lunex help` for the full command list.\n")
}
