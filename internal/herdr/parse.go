package herdr

import (
	"encoding/json"
	"path/filepath"
)

// rawList mirrors the `herdr agent list --json` envelope. Unknown fields are
// ignored so a herdr version bump that adds fields does not break parsing.
type rawList struct {
	Result struct {
		Agents []struct {
			Agent        string `json:"agent"`
			AgentStatus  string `json:"agent_status"`
			Cwd          string `json:"cwd"`
			Focused      bool   `json:"focused"`
			PaneID       string `json:"pane_id"`
			TabID        string `json:"tab_id"`
			WorkspaceID  string `json:"workspace_id"`
			TerminalID   string `json:"terminal_id"`
			AgentSession *struct {
				Value string `json:"value"`
			} `json:"agent_session"`
		} `json:"agents"`
	} `json:"result"`
}

// parseAgentList turns the list envelope into []Agent. Project is the basename
// of cwd. A nil agent_session (non-claude agents) yields an empty SessionID.
func parseAgentList(raw []byte) ([]Agent, error) {
	var rl rawList
	if err := json.Unmarshal(raw, &rl); err != nil {
		return nil, err
	}
	out := make([]Agent, 0, len(rl.Result.Agents))
	for _, a := range rl.Result.Agents {
		ag := Agent{
			Label:       a.Agent,
			Status:      a.AgentStatus,
			Cwd:         a.Cwd,
			Project:     filepath.Base(a.Cwd),
			Focused:     a.Focused,
			PaneID:      a.PaneID,
			TabID:       a.TabID,
			WorkspaceID: a.WorkspaceID,
			TerminalID:  a.TerminalID,
		}
		if a.AgentSession != nil {
			ag.SessionID = a.AgentSession.Value
		}
		out = append(out, ag)
	}
	return out, nil
}
