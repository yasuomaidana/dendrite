package analysis

import (
	"Dendrite/pkg/directive"
	"Dendrite/pkg/fsutil"
	"Dendrite/pkg/rea/engine"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// maxFunctionDocs bounds the function documents written (largest functions first); each is indexed with the rest.
const maxFunctionDocs = 200

// maxNameChars bounds the part of a function document's name taken from the function.
const maxNameChars = 60

// EngineRunner runs a reverse-engineering engine over a target; *engine.Service is the real one.
type EngineRunner interface {
	Run(ctx context.Context, target, format string, p engine.Policy) (*engine.Result, error)
}

func (s *Service) enginePath(id string) string { return filepath.Join(s.Store.Dir(id), "engine.json") }

// EngineFunctions reads the functions the engine found for analysis id; nothing when no engine ran.
func (s *Service) EngineFunctions(id string) ([]engine.Function, error) {
	var res engine.Result
	err := readJSON(s.enginePath(id), &res) // a missing file (an analysis made before engines) reads as none
	return res.Functions, err
}

// explicitEngine reports whether the analysis was told which engine to use, which makes a failing engine an error.
func explicitEngine(requested string) bool { return requested != "" && requested != engine.EngineAuto }

// runEngine runs the engine the policy chose and saves what it found (engine.json and one document per large
// function). With no usable engine the analysis goes on with its structure and records why. An engine that fails
// while the engine was left to auto-selection is recorded too; one that was asked for by name is an error, and so is
// a refused policy (an untrusted binary on the host).
func (s *Service) runEngine(ctx context.Context, st *State) error {
	res, err := s.Engines.Run(ctx, st.Binary, st.Format, engine.Policy{Engine: st.Engine, Sandbox: st.Sandbox, Trusted: st.Trusted})
	var runErr *engine.RunError
	if err != nil {
		if !errors.As(err, &runErr) || explicitEngine(st.Engine) {
			return err
		}
		res = &engine.Result{Note: "the engine failed and the analysis went on without it: " + err.Error()}
	}
	if err := writeJSON(s.enginePath(st.ID), res); err != nil {
		return err
	}
	if err := writeFunctionDocs(s.Store.WorkspaceDir(st.ID), res.Functions); err != nil {
		return err
	}
	st.EngineRan, st.EngineMode, st.EngineNote, st.Stage = res.Engine, res.Mode, res.Note, StageEngine
	return nil
}

// functionDocName names the document of a function after its address (unique) and its name.
func functionDocName(f engine.Function) string {
	name := strings.Trim(unsafeName.ReplaceAllString(strings.ToLower(f.Name), "_"), "._-")
	if len(name) > maxNameChars {
		name = name[:maxNameChars]
	}
	if name == "" {
		name = "function"
	}
	return filepath.Join("functions", f.Address+"-"+name+".md")
}

// functionDoc is the document of one function: what the engine knows, so a search finds it like any other part of
// the analysis.
func functionDoc(f engine.Function) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "# Function %s\n\n- Address: %s\n- Size: %d bytes\n", f.Name, f.Address, f.Size)
	if f.Cyclomatic > 0 {
		fmt.Fprintf(&sb, "- Cyclomatic complexity: %d\n", f.Cyclomatic)
	}
	if len(f.Calls) > 0 {
		fmt.Fprintf(&sb, "- Calls: %s\n", strings.Join(f.Calls, ", "))
	}
	if f.Disassembly != "" {
		fmt.Fprintf(&sb, "\n## Disassembly\n\n```asm\n%s```\n", f.Disassembly)
	}
	if f.Pseudocode != "" {
		fmt.Fprintf(&sb, "\n## Pseudo-code\n\n```c\n%s\n```\n", f.Pseudocode)
	}
	return sb.String()
}

// writeFunctionDocs writes the documents of the largest functions (the engine lists them largest first).
func writeFunctionDocs(dir string, fns []engine.Function) error {
	for i, f := range fns {
		if i == maxFunctionDocs {
			break
		}
		if err := fsutil.WriteFileAtomic(filepath.Join(dir, functionDocName(f)), []byte(functionDoc(f))); err != nil {
			return err
		}
	}
	return nil
}

// stageAfterWritten reports whether stage a is the written stage or comes after it, the one the engine stage follows.
func stageAfterWritten(a Stage) bool {
	switch a {
	case StageWritten, StageEngine, StageTriaged, StageDone:
		return true
	}
	return false
}

// WithEngineDefaults fills the engine and sandbox that o leaves empty: from the environment, then the project's
// [rea] directive. Resuming an analysis does not use it: an empty choice there means "keep what the analysis has".
func (o Options) WithEngineDefaults(rule directive.Rea) Options {
	p := engine.ResolvePolicy(rule, engine.Policy{Engine: o.Engine, Sandbox: o.Sandbox})
	o.Engine, o.Sandbox = p.Engine, p.Sandbox
	return o
}

// retarget applies another engine, sandbox or trust to a resumed analysis. An analysis that is already past the
// written stage goes back to it, so the engine (and what follows) runs again with the new choice.
func (s *Service) retarget(st *State, o Options) {
	if o.Engine == "" && o.Sandbox == "" && !o.Trusted {
		return
	}
	st.Engine, st.Sandbox = orDefault(o.Engine, st.Engine), orDefault(o.Sandbox, st.Sandbox)
	st.Trusted = st.Trusted || o.Trusted
	if stageAfterWritten(st.Stage) {
		st.Stage, st.Gate = StageWritten, ""
	}
}
