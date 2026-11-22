package coreclient

import (
	"context"
	"testing"
)

func TestComponentProps(t *testing.T) {
	props, err := ComponentProps(context.Background(), "svelte", "prop:label prop:...rest other")
	if err != nil || len(props) != 2 || props[0].Name != "label" || props[0].Rest || !props[1].Rest {
		t.Fatalf("props = %+v, err = %v", props, err)
	}
	if props, err = ComponentProps(context.Background(), "tsx", "nothing here"); err != nil || len(props) != 0 {
		t.Fatalf("no props = %+v, err = %v", props, err)
	}
}

func TestComponentPropsSurfaceTheSidecarError(t *testing.T) {
	if _, err := ComponentProps(context.Background(), "jsx", "FAIL"); err == nil {
		t.Fatal("a sidecar error must not become an empty list")
	}
	var nilClient *Client
	if _, err := nilClient.ComponentProps(context.Background(), "tsx", ""); err == nil {
		t.Fatal("a nil client must be an error")
	}
}
