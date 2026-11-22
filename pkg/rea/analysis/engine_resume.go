package analysis

import (
	"Dendrite/pkg/directive"
	"Dendrite/pkg/rea/engine"
)

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
