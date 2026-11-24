package ticketparent

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"testing"

	"Dendrite/pkg/planesvc"
	"Dendrite/pkg/planesvc/planetest"
	"Dendrite/pkg/ticketlock"
	"Dendrite/pkg/tracker/plane"
)

// rig is a Plane stand-in with DEN-841 under DEN-840 and DEN-900 on its own.
func rig(t *testing.T) (*planetest.Board, ticketlock.Board) {
	t.Helper()
	t.Setenv("PLANE_API_KEY", "k")
	board := planetest.NewBoard(t, map[string]string{"DEN-840": "To Do", "DEN-841": "To Do", "DEN-900": "To Do"})
	board.SetParent("DEN-841", "DEN-840")
	return board, boardAt(t, board.URL, board.Client())
}

// boardAt is a ticket board on the Plane served at baseURL.
func boardAt(t *testing.T, baseURL string, httpClient *http.Client) ticketlock.Board {
	t.Helper()
	client, err := plane.NewClient(plane.Config{BaseURL: baseURL, WorkspaceSlug: planetest.BoardWorkspace, ProjectID: planetest.BoardProject, DirectToken: "k"}, httpClient)
	if err != nil {
		t.Fatal(err)
	}
	return ticketlock.Board{Svc: planesvc.Default(), Client: client}
}

// DEN-841 is under DEN-840, which has no parent: parenting under DEN-841 walks up one step and stops.
func TestSetMakesTheParentNormalizesRefsAndWalksUpTheChain(t *testing.T) {
	for _, c := range []struct{ child, parent, wantParent string }{
		{" den-900 ", "den-840", "DEN-840"},
		{"DEN-900", "DEN-841", "DEN-841"},
	} {
		board, b := rig(t)
		if err := Set(context.Background(), b, c.child, c.parent); err != nil {
			t.Fatal(err)
		}
		if got, want := board.ParentOf("DEN-900"), board.IDOf(c.wantParent); got != want {
			t.Errorf("%q under %q: parent = %q, want %q", c.child, c.parent, got, want)
		}
	}
}

func TestSetRefusesLoopsAndUnknownTickets(t *testing.T) {
	_, b := rig(t)
	for name, args := range map[string][2]string{
		"itself":         {"DEN-840", "DEN-840"},
		"its descendant": {"DEN-840", "DEN-841"},
		"unknown parent": {"DEN-900", "DEN-404"},
		"unknown child":  {"DEN-404", "DEN-840"},
	} {
		err := Set(context.Background(), b, args[0], args[1])
		if err == nil {
			t.Errorf("%s: expected an error", name)
		}
		if loop := name == "itself" || name == "its descendant"; loop != errors.Is(err, ErrLoop) {
			t.Errorf("%s: ErrLoop = %v for %v", name, !loop, err)
		}
	}
}

func TestClearWithoutAParentSendsNothing(t *testing.T) {
	_, b := rig(t)
	called := false
	_, had, err := clearWith(context.Background(), b, "DEN-900", func(context.Context) error { called = true; return nil })
	if err != nil || had || called {
		t.Errorf("had=%v called=%v err=%v, want a quiet no-op", had, called, err)
	}
}

func TestClearRemovesTheParentAndNamesIt(t *testing.T) {
	board, b := rig(t)
	old, had, err := clearWith(context.Background(), b, "den-841", func(context.Context) error { return nil })
	if err != nil || !had || old.ID != "DEN-840" {
		t.Fatalf("old=%+v had=%v err=%v", old, had, err)
	}
	if got := board.ParentOf("DEN-841"); got != "" {
		t.Errorf("parent = %q, want none", got)
	}
}

func TestClearRefusesAnOldSidecarBeforeTouchingPlane(t *testing.T) {
	board, b := rig(t)
	tooOld := errors.New("too old")
	_, _, err := clearWith(context.Background(), b, "DEN-841", func(context.Context) error { return tooOld })
	if !errors.Is(err, tooOld) {
		t.Fatalf("err = %v", err)
	}
	if board.ParentOf("DEN-841") == "" {
		t.Error("the parent was cleared although the sidecar is too old")
	}
}

func TestClearOfAnUnknownTicketIsAnError(t *testing.T) {
	_, b := rig(t)
	if _, _, err := Clear(context.Background(), b, "DEN-404"); err == nil {
		t.Fatal("an unknown ticket is an error")
	}
}

func TestClearReportsAPlaneThatRefusesTheWrite(t *testing.T) {
	board, _ := rig(t)
	target, err := url.Parse(board.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	refusing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			http.Error(w, "no", http.StatusInternalServerError)
			return
		}
		proxy.ServeHTTP(w, r)
	}))
	t.Cleanup(refusing.Close)
	b := boardAt(t, refusing.URL, refusing.Client())
	_, had, err := clearWith(context.Background(), b, "DEN-841", func(context.Context) error { return nil })
	if err == nil || !had {
		t.Fatalf("had=%v err=%v, want the refused write reported", had, err)
	}
	if board.ParentOf("DEN-841") == "" {
		t.Error("the parent was cleared although Plane refused the write")
	}
}
