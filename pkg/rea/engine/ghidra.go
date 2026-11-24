package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// Markers the export script prints around its JSON, because Ghidra's own log is on the same output.
const (
	ghidraBegin = "---DENDRITE-JSON-BEGIN---"
	ghidraEnd   = "---DENDRITE-JSON-END---"
	// ghidraScript is the export script's file name, in the image at ghidraImageScripts and on the host in a temporary
	// directory.
	ghidraScript       = "ExportFunctions.java"
	ghidraImageScripts = "/scripts"
)

// ghidraArgv builds the headless command: import the target into a throw-away project in projectDir, run the export
// script and delete the project.
func ghidraArgv(bin, projectDir, scriptDir string) []string {
	return []string{bin, projectDir, "dendrite", "-import", TargetPath, "-scriptPath", scriptDir, "-postScript", ghidraScript, "-deleteProject"}
}

// parseGhidra reads the functions the export script printed between its markers; everything else is Ghidra's log.
func parseGhidra(out []byte) ([]Function, error) {
	start := bytes.Index(out, []byte(ghidraBegin))
	end := bytes.Index(out, []byte(ghidraEnd))
	if start < 0 || end < start {
		return nil, errors.New("the engine did not print its result between the markers (the export script did not run or failed)")
	}
	var fns []Function
	if err := json.Unmarshal(bytes.TrimSpace(out[start+len(ghidraBegin):end]), &fns); err != nil {
		return nil, errors.New("the engine printed a result that is not a function list: " + err.Error())
	}
	return fns, nil
}

// runGhidra lists the functions with Ghidra's headless analyzer. In the container the script is in the image; on the
// host it is written to a temporary directory first.
func (s *Service) runGhidra(ctx context.Context, c *Choice, target string) ([]Function, error) {
	argv := ghidraArgv("analyzeHeadless", "/tmp", ghidraImageScripts)
	if c.Mode == ModeHost {
		dir, err := os.MkdirTemp("", "rea-ghidra")
		if err != nil {
			return nil, err
		}
		defer os.RemoveAll(dir)
		script, _ := dockerfiles.ReadFile("dockerfiles/ghidra/" + ghidraScript) // embedded: always present (tested)
		_ = os.WriteFile(filepath.Join(dir, ghidraScript), script, 0o644)       // a script that was not written makes Ghidra say so
		argv = ghidraArgv(c.HostBin, dir, dir)
	}
	out, err := s.run(ctx, c, target, argv)
	if err != nil {
		return nil, err
	}
	return parseGhidra(out)
}
