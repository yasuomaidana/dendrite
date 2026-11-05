package canvas

import (
	"context"
	"fmt"
)

func (h *relateTicketHandler) Execute(ctx context.Context, req ActionRequest) (*ActionResponse, error) {
	if h.bridge == nil {
		return nil, ErrNilBridge
	}

	ticketID, _ := req.Parameters["ticket_id"].(string)
	featureName, _ := req.Parameters["feature_name"].(string)
	componentPath, _ := req.Parameters["component_path"].(string)
	penpotFrameID, _ := req.Parameters["penpot_frame_id"].(string)

	if ticketID == "" {
		ticketID = req.Title
	}
	if featureName == "" {
		featureName = req.Prompt
	}

	relation := TicketFeatureRelation{
		TicketID:      ticketID,
		FeatureName:   featureName,
		Description:   req.Prompt,
		ComponentPath: componentPath,
		PenpotFrameID: penpotFrameID,
	}

	if err := h.bridge.RelateTicketToFeature(ctx, relation); err != nil {
		return nil, err
	}

	saved, _ := h.bridge.GetTicketFeatureRelation(ticketID)

	return &ActionResponse{
		Action:  ActionRelateTicket,
		Status:  "success",
		Title:   fmt.Sprintf("Ticket Relational Link: %s -> %s", ticketID, featureName),
		Content: fmt.Sprintf("Bound Plane ticket '%s' to project functionality '%s' (Penpot Frame: %s)", ticketID, featureName, saved.PenpotFrameURL),
		StructuredData: map[string]interface{}{
			"relation": saved,
		},
		UIBridgeResult: &UIBridgeResult{
			TicketID:       ticketID,
			FeatureName:    featureName,
			PenpotFrameURL: saved.PenpotFrameURL,
			ComponentPath:  componentPath,
			ProjectState:   saved.CurrentState,
		},
		Notes: []string{
			fmt.Sprintf("Ticket '%s' successfully bound to feature '%s'", ticketID, featureName),
			fmt.Sprintf("Penpot Frame Deep-Link: %s", saved.PenpotFrameURL),
		},
	}, nil
}

type updateStateHandler struct {
	bridge *PenpotUIBridge
}

func (h *updateStateHandler) ActionID() ActionID {
	return ActionUpdateState
}

func (h *updateStateHandler) Execute(ctx context.Context, req ActionRequest) (*ActionResponse, error) {
	if h.bridge == nil {
		return nil, ErrNilBridge
	}

	ticketID, _ := req.Parameters["ticket_id"].(string)
	targetState, _ := req.Parameters["target_state"].(string)
	comment, _ := req.Parameters["comment"].(string)
	penpotURL, _ := req.Parameters["penpot_frame_url"].(string)
	pushToPlane, _ := req.Parameters["push_to_plane"].(bool)

	if ticketID == "" {
		ticketID = req.Title
	}
	if targetState == "" {
		targetState = req.Prompt
	}

	updateReq := ProjectStateUpdateRequest{
		TicketID:       ticketID,
		TargetState:    targetState,
		Comment:        comment,
		PenpotFrameURL: penpotURL,
		PushToPlane:    pushToPlane,
	}

	res, err := h.bridge.UpdateProjectState(ctx, updateReq)
	if err != nil {
		return nil, err
	}

	return &ActionResponse{
		Action:  ActionUpdateState,
		Status:  "success",
		Title:   fmt.Sprintf("Project State Transition: %s (%s -> %s)", ticketID, res.PreviousState, res.NewState),
		Content: res.Message,
		StructuredData: map[string]interface{}{
			"update_result": res,
		},
		UIBridgeResult: &UIBridgeResult{
			TicketID:     ticketID,
			ProjectState: res.NewState,
			PlaneUpdated: res.PlaneSynced,
			Details:      res.Message,
		},
		Notes: []string{
			res.Message,
			fmt.Sprintf("Plane API Synchronization: %v", res.PlaneSynced),
		},
	}, nil
}
