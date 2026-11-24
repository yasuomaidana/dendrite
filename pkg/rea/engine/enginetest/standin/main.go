// Command standin plays docker, rizin, radare2, objdump and Ghidra (analyzeHeadless) for the engine tests, chosen by the name it is run as.
// The docker it plays refuses a run that is not hardened and runs the command of the container on this machine, with
// /target/input replaced by the mounted file. Environment: STANDIN_DOCKER=down, STANDIN_IMAGES=<comma list>,
// STANDIN_MODE=r2broken|many|empty|r2hard|detailfail|objfail|buildfail|ghidranomarkers|ghidrabroken, STANDIN_LOG=<file> (one line per call).
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	args := os.Args[1:]
	logCall(args)
	switch filepath.Base(os.Args[0]) {
	case "docker":
		docker(args)
	case "rizin", "radare2", "r2":
		r2(args)
	case "objdump", "llvm-objdump":
		objdump(args)
	case "analyzeHeadless":
		ghidra(args)
	default:
		fmt.Fprintln(os.Stderr, "standin: run me as docker, rizin, radare2 or objdump")
		os.Exit(2)
	}
}

func logCall(args []string) {
	if path := os.Getenv("STANDIN_LOG"); path != "" {
		if f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
			fmt.Fprintf(f, "%s %s\n", filepath.Base(os.Args[0]), strings.Join(args, " "))
			_ = f.Close()
		}
	}
}

func docker(args []string) {
	if len(args) == 0 {
		os.Exit(2)
	}
	switch args[0] {
	case "info":
		if os.Getenv("STANDIN_DOCKER") == "down" {
			os.Exit(1)
		}
		fmt.Println("29.9.3")
	case "image":
		if len(args) < 3 || !strings.Contains(","+os.Getenv("STANDIN_IMAGES")+",", ","+args[2]+",") {
			os.Exit(1)
		}
	case "rm":
	case "build":
		dockerBuild(args[1:])
	case "run":
		dockerRun(args[1:])
	default:
		os.Exit(2)
	}
}

// valueFlags take a value in a `docker run` line.
var valueFlags = map[string]bool{"--name": true, "--cap-drop": true, "--security-opt": true, "--network": true, "--tmpfs": true, "-m": true, "--cpus": true, "-v": true, "-w": true, "-e": true}

// parseRun reads the flags of a `docker run` line: the flag values, the -v mounts and the index of the image.
func parseRun(args []string) (flags map[string]string, mounts []string, image int) {
	flags = map[string]string{}
	for ; image < len(args) && strings.HasPrefix(args[image], "-"); image++ {
		if !valueFlags[args[image]] {
			flags[args[image]] = ""
			continue
		}
		if args[image] == "-v" {
			mounts = append(mounts, args[image+1])
		}
		flags[args[image]] = args[image+1]
		image++
	}
	return flags, mounts, image
}

// hardened reports whether the flags are the ones a safe container run has.
func hardened(flags map[string]string) bool {
	_, readOnly := flags["--read-only"]
	return flags["--network"] == "none" && flags["--cap-drop"] == "ALL" && readOnly && flags["--security-opt"] == "no-new-privileges:true"
}

// targetMount is the host path mounted read-only at /target/input, or "".
func targetMount(mounts []string) string {
	for _, m := range mounts {
		if parts := strings.Split(m, ":"); len(parts) == 3 && parts[1] == "/target/input" && parts[2] == "ro" {
			return parts[0]
		}
	}
	return ""
}

// dockerRun refuses a container that is not hardened, then runs its command here with the mounted file in place.
func dockerRun(args []string) {
	flags, mounts, i := parseRun(args)
	if !hardened(flags) {
		fmt.Fprintln(os.Stderr, "standin docker: the container is not hardened")
		os.Exit(3)
	}
	host := targetMount(mounts)
	if host == "" || len(args[i:]) < 4 {
		fmt.Fprintln(os.Stderr, "standin docker: the target is not mounted read-only at /target/input")
		os.Exit(3)
	}
	script := strings.ReplaceAll(args[i+3], "/target/input", host) // args[i] is the image, then sh, -c, script
	cmd := exec.Command("sh", "-c", script)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	cmd.Env = append(os.Environ(), "PATH="+filepath.Dir(os.Args[0])+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := cmd.Run(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			os.Exit(exit.ExitCode())
		}
		os.Exit(1)
	}
}

// r2 answers rizin and radare2: aflj lists functions (with a warning on standard error), pdfj @ addr disassembles one.
func r2(args []string) {
	commands, target := "", args[len(args)-1]
	for i, a := range args {
		if a == "-c" && i+1 < len(args) {
			commands = args[i+1]
		}
	}
	if _, err := os.Stat(target); err != nil {
		fmt.Fprintln(os.Stderr, "ERROR: cannot open", target)
		os.Exit(1)
	}
	if os.Getenv("STANDIN_MODE") == "detailfail" && strings.Contains(commands, "pdfj @") {
		os.Exit(1)
	}
	for _, c := range strings.Split(commands, ";") {
		switch c = strings.TrimSpace(c); {
		case c == "aflj":
			fmt.Fprintln(os.Stderr, "WARN: this warning must not reach the answer")
			functions()
		case strings.HasPrefix(c, "pdfj @ "):
			disassembly(strings.TrimPrefix(c, "pdfj @ "))
		}
	}
}

func functions() {
	switch os.Getenv("STANDIN_MODE") {
	case "r2broken":
		fmt.Println("this is not json")
	case "empty":
		fmt.Println("[]")
	case "r2hard":
		fmt.Println(`[{"offset":4198400,"name":"main","size":64,"cc":20,"callrefs":[]},{"offset":4198464,"name":"helper","size":16,"cc":2}]`)
	case "many":
		fmt.Print("[")
		for n := 0; n < 600; n++ {
			if n > 0 {
				fmt.Print(",")
			}
			fmt.Printf(`{"offset":%d,"name":"fn%d","size":%d,"cc":1}`, 0x500000+n*16, n, 10+n)
		}
		fmt.Println("]")
	default:
		fmt.Println(`[{"offset":4198400,"name":"main","size":64,"cc":3,"callrefs":[{"addr":4198464,"type":"CALL"},{"addr":4198500,"type":"DATA"}]},` +
			`{"offset":4198464,"name":"helper","size":16,"cc":1},{"offset":4198496,"name":"tiny","size":6,"cc":1}]`)
	}
}

func disassembly(addr string) {
	switch addr {
	case "0x401000":
		fmt.Println(`{"name":"main","size":64,"addr":4198400,"ops":[{"offset":4198400,"disasm":"push rbp"},{"offset":4198401,"disasm":"call 0x401040"}]}`)
	case "0x401040":
		fmt.Println(`{"name":"helper","size":16,"addr":4198464,"ops":[{"offset":4198464,"disasm":"ret"}]}`)
	}
}

func objdump(args []string) {
	if os.Getenv("STANDIN_MODE") == "objfail" {
		os.Exit(1)
	}
	if _, err := os.Stat(args[len(args)-1]); err != nil {
		os.Exit(1)
	}
	fmt.Print("/target/input:     file format elf64-x86-64\n\nDisassembly of section .text:\n\n" +
		"0000000000401000 <main>:\n  401000:\tpush   %rbp\n  401001:\tcall   401040 <helper>\n  401006:\tret\n\n" +
		"0000000000401040 <helper>:\n  401040:\tmov    %rsp,%rbp\n  401045:\tret\n")
}

// dockerBuild answers `docker build -t image dir`: it prints the first line of the Dockerfile it was given, or fails
// in mode buildfail.
func dockerBuild(args []string) {
	if os.Getenv("STANDIN_MODE") == "buildfail" {
		fmt.Println("ERROR: package not found")
		os.Exit(1)
	}
	data, err := os.ReadFile(filepath.Join(args[len(args)-1], "Dockerfile"))
	if err != nil {
		os.Exit(1)
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "FROM ") {
			fmt.Println("built from", strings.TrimPrefix(line, "FROM "), "as", args[1])
		}
	}
	if _, err := os.Stat(filepath.Join(args[len(args)-1], "ExportFunctions.java")); err == nil {
		fmt.Println("context holds ExportFunctions.java")
	}
}

// ghidraArgs reads the import, script path and script name of an analyzeHeadless line.
func ghidraArgs(args []string) (imported, scriptPath, script string) {
	for i, a := range args {
		switch a {
		case "-import":
			imported = args[i+1]
		case "-scriptPath":
			scriptPath = args[i+1]
		case "-postScript":
			script = args[i+1]
		}
	}
	return imported, scriptPath, script
}

// ghidraChecks exits like Ghidra does when the import or the script cannot be found.
func ghidraChecks(imported, scriptPath, script string) {
	if _, err := os.Stat(imported); err != nil || script != "ExportFunctions.java" {
		fmt.Println("ERROR REPORT: nothing to import or no script")
		os.Exit(1)
	}
	if info, err := os.Stat(scriptPath); err == nil && info.IsDir() {
		if _, err := os.Stat(filepath.Join(scriptPath, script)); err != nil {
			fmt.Println("ERROR GhidraScriptLoadException: script not found")
			os.Exit(1)
		}
	}
}

// ghidraResult prints what the export script prints, between its markers, or breaks it as the mode asks.
func ghidraResult() {
	switch os.Getenv("STANDIN_MODE") {
	case "ghidranomarkers":
	case "ghidrabroken":
		fmt.Println("---DENDRITE-JSON-BEGIN---\nthis is not json\n---DENDRITE-JSON-END---")
	default:
		fmt.Println(`---DENDRITE-JSON-BEGIN---
[{"name":"main","address":"0x401000","size":64,"cyclomatic":3,"calls":["0x401040"],"disassembly":"0x401000  PUSH RBP\n","pseudocode":"int main(void)\n{\n  return helper();\n}\n"},` +
			`{"name":"helper","address":"0x401040","size":16,"cyclomatic":1,"calls":[]},{"name":"FUN_00401060","address":"0x401060","size":6,"cyclomatic":1,"calls":[]}]
---DENDRITE-JSON-END---`)
	}
}

// ghidra answers analyzeHeadless: it checks the import and the script it was given, then prints Ghidra-style log lines
// around the JSON the export script prints between its markers.
func ghidra(args []string) {
	imported, scriptPath, script := ghidraArgs(args)
	ghidraChecks(imported, scriptPath, script)
	fmt.Println("INFO  HEADLESS Script Paths: " + scriptPath + " (HeadlessAnalyzer)")
	fmt.Println("INFO  ANALYZING all memory and code: " + imported + " (HeadlessAnalyzer)")
	ghidraResult()
	fmt.Println("INFO  REPORT: Analysis succeeded for file: " + imported + " (HeadlessAnalyzer)")
}
