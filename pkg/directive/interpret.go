package directive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"Dendrite/pkg/llmchat"
)

const interpretPrompt = "You turn a rule about code quality into changes of these settings. Settings:\n%s\n" +
	"Answer with ONLY a JSON array, one object per setting to change: " +
	`[{"key":"coverage.min_percent","value":95,"rationale":"one short sentence"}]. ` +
	"Use only the keys listed, with a value of the kind shown (a number, true or false, a word or a list of text). " +
	"If the rule changes none of them, answer []."

// Interpret asks a model to read a rule Parse did not know. Every change it returns has been checked against
// the settings and their limits, so a wrong key or an out-of-range value is refused, never written.
func Interpret(ctx context.Context, chat llmchat.Completer, text string) ([]Change, error) {
	answer, err := chat.Complete(ctx, fmt.Sprintf(interpretPrompt, Describe()), text)
	if err != nil {
		return nil, fmt.Errorf("the model could not read the rule: %w", err)
	}
	raw, err := llmchat.JSON(answer, '[', ']')
	if err != nil {
		return nil, fmt.Errorf("the model's answer is not usable: %w", err)
	}
	var changes []Change
	if err := json.Unmarshal([]byte(raw), &changes); err != nil {
		return nil, fmt.Errorf("the model's answer is not valid JSON: %w", err)
	}
	if len(changes) == 0 {
		return nil, errors.New("no directive matches that rule; `dendrite directive list` shows the settings")
	}
	scratch := Defaults()
	for i, c := range changes {
		if err := scratch.Apply(c.Key, c.Value); err != nil {
			return nil, fmt.Errorf("the model proposed an invalid change (%s): %w", c, err)
		}
		changes[i].Value = valueOf(scratch, c.Key)
	}
	return changes, nil
}

// valueOf is the value a key holds in s, as the typed value Apply stored (a JSON 95 becomes 95.0).
func valueOf(s Set, key string) any {
	return keys[key].get(s)
}
