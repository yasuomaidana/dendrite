package engine

import (
	"errors"
	"testing"

	"Dendrite/pkg/directive"
)

func TestResolvePolicyTakesTheFlagThenTheEnvironmentThenTheDirective(t *testing.T) {
	rule := directive.Defaults().Rea
	rule.Engine, rule.Sandbox = "radare2", "container"

	got := ResolvePolicy(rule, Policy{})
	if got.Engine != "radare2" || got.Sandbox != "container" || got.Trusted {
		t.Errorf("the directive: %+v", got)
	}
	t.Setenv(EnvEngine, "objdump")
	t.Setenv(EnvSandbox, "host")
	if got = ResolvePolicy(rule, Policy{}); got.Engine != "objdump" || got.Sandbox != "host" {
		t.Errorf("the environment beats the directive: %+v", got)
	}
	if got = ResolvePolicy(rule, Policy{Engine: "rizin", Sandbox: "container", Trusted: true}); got.Engine != "rizin" || got.Sandbox != "container" || !got.Trusted {
		t.Errorf("a flag beats both and trust is only what the caller says: %+v", got)
	}
	t.Setenv(EnvEngine, "")
	t.Setenv(EnvSandbox, "")
	if got = ResolvePolicy(directive.Rea{}, Policy{}); got.Engine != "" || got.Sandbox != "" {
		t.Errorf("nothing set stays empty so Select applies its defaults: %+v", got)
	}
}

func TestRunErrorNamesTheEngineAndKeepsTheCause(t *testing.T) {
	cause := errors.New("boom")
	e := &RunError{Engine: "rizin", Mode: ModeContainer, Err: cause}
	if e.Error() != "rizin (container): boom" || !errors.Is(e, cause) {
		t.Errorf("%q", e.Error())
	}
}
