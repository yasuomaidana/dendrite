package coreclient

import "context"

// ComponentProp is one prop of a UI component as dendrite-core reads it from the syntax tree.
type ComponentProp struct {
	Name     string `json:"name"`
	Type     string `json:"type,omitempty"`
	Optional bool   `json:"optional"`
	Default  string `json:"default,omitempty"`
	// Rest marks a `...rest` collector, which holds the remaining props rather than being one.
	Rest bool `json:"rest"`
	Line int  `json:"line"`
}

// ComponentProps lists the props of the component in source: Svelte 5 `$props()` and legacy `export let`
// (lang "svelte"), and React function components (lang "tsx", "ts", "jsx" or "js"). It needs protocol 1.2.
func (c *Client) ComponentProps(ctx context.Context, lang, source string) ([]ComponentProp, error) {
	var res struct {
		Props []ComponentProp `json:"props"`
	}
	if err := c.call(ctx, "ast.component_props", map[string]string{"lang": lang, "source": source}, &res); err != nil {
		return nil, err
	}
	return res.Props, nil
}

// ComponentProps calls Client.ComponentProps on the default client.
func ComponentProps(ctx context.Context, lang, source string) ([]ComponentProp, error) {
	return Default().ComponentProps(ctx, lang, source)
}
