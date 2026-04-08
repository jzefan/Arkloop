package formatter

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

const (
	OutputText = "text"
	OutputJSON = "json"
)

type StatusView struct {
	Host        string `json:"host"`
	Connected   bool   `json:"connected"`
	UserID      string `json:"user_id"`
	Username    string `json:"username"`
	AccountID   string `json:"account_id"`
	WorkEnabled bool   `json:"work_enabled"`
}

type ModelView struct {
	Model        string   `json:"model"`
	ProviderID   string   `json:"provider_id"`
	ProviderName string   `json:"provider_name"`
	IsDefault    bool     `json:"is_default"`
	ShowInPicker bool     `json:"show_in_picker"`
	Tags         []string `json:"tags"`
}

type PersonaView struct {
	PersonaKey    string `json:"persona_key"`
	SelectorName  string `json:"selector_name"`
	DisplayName   string `json:"display_name"`
	Model         string `json:"model"`
	ReasoningMode string `json:"reasoning_mode"`
	Source        string `json:"source"`
}

type SessionView struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Mode        string `json:"mode"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
	ActiveRunID string `json:"active_run_id"`
	IsPrivate   bool   `json:"is_private"`
}

type ChatStatusView struct {
	Host      string `json:"host"`
	SessionID string `json:"session_id"`
	Model     string `json:"model"`
	Persona   string `json:"persona"`
	WorkDir   string `json:"work_dir"`
	Timeout   string `json:"timeout"`
}

func PrintStatus(w io.Writer, outputFormat string, view StatusView) error {
	switch outputFormat {
	case OutputText:
		_, err := fmt.Fprintf(
			w,
			"host: %s\nconnected: %t\nuser_id: %s\nusername: %s\naccount_id: %s\nwork_enabled: %t\n",
			view.Host,
			view.Connected,
			displayText(view.UserID, "-"),
			displayText(view.Username, "-"),
			displayText(view.AccountID, "-"),
			view.WorkEnabled,
		)
		return err
	case OutputJSON:
		return writeJSON(w, view)
	default:
		return fmt.Errorf("unknown output format: %s", outputFormat)
	}
}

func PrintModels(w io.Writer, outputFormat string, views []ModelView) error {
	switch outputFormat {
	case OutputText:
		tw := newTabWriter(w)
		if _, err := fmt.Fprintln(tw, "MODEL\tPROVIDER\tDEFAULT\tHIDDEN\tTAGS"); err != nil {
			return err
		}
		for _, view := range views {
			if _, err := fmt.Fprintf(
				tw,
				"%s\t%s\t%t\t%t\t%s\n",
				displayText(view.Model, "-"),
				displayText(view.ProviderName, "-"),
				view.IsDefault,
				!view.ShowInPicker,
				strings.Join(view.Tags, ","),
			); err != nil {
				return err
			}
		}
		return tw.Flush()
	case OutputJSON:
		return writeJSON(w, views)
	default:
		return fmt.Errorf("unknown output format: %s", outputFormat)
	}
}

func PrintPersonas(w io.Writer, outputFormat string, views []PersonaView) error {
	switch outputFormat {
	case OutputText:
		tw := newTabWriter(w)
		if _, err := fmt.Fprintln(tw, "NAME\tPERSONA_KEY\tMODEL\tREASONING_MODE"); err != nil {
			return err
		}
		for _, view := range views {
			if _, err := fmt.Fprintf(
				tw,
				"%s\t%s\t%s\t%s\n",
				displayText(view.SelectorName, displayText(view.DisplayName, view.PersonaKey)),
				displayText(view.PersonaKey, "-"),
				displayText(view.Model, "-"),
				displayText(view.ReasoningMode, "-"),
			); err != nil {
				return err
			}
		}
		return tw.Flush()
	case OutputJSON:
		return writeJSON(w, views)
	default:
		return fmt.Errorf("unknown output format: %s", outputFormat)
	}
}

func PrintSessions(w io.Writer, outputFormat string, views []SessionView) error {
	switch outputFormat {
	case OutputText:
		tw := newTabWriter(w)
		if _, err := fmt.Fprintln(tw, "SESSION_ID\tTITLE\tMODE\tCREATED_AT\tUPDATED_AT\tACTIVE_RUN_ID"); err != nil {
			return err
		}
		for _, view := range views {
			if _, err := fmt.Fprintf(
				tw,
				"%s\t%s\t%s\t%s\t%s\t%s\n",
				displayText(view.ID, "-"),
				displayText(view.Title, "-"),
				displayText(view.Mode, "-"),
				displayText(view.CreatedAt, "-"),
				displayText(view.UpdatedAt, "-"),
				displayText(view.ActiveRunID, "-"),
			); err != nil {
				return err
			}
		}
		return tw.Flush()
	case OutputJSON:
		return writeJSON(w, views)
	default:
		return fmt.Errorf("unknown output format: %s", outputFormat)
	}
}

func PrintChatStatus(w io.Writer, view ChatStatusView) error {
	_, err := fmt.Fprintf(
		w,
		"host: %s\nsession id: %s\nmodel: %s\npersona: %s\nwork-dir: %s\ntimeout: %s\n",
		displayText(view.Host, "-"),
		displayText(view.SessionID, "new"),
		displayText(view.Model, "default"),
		displayText(view.Persona, "default"),
		displayText(view.WorkDir, "-"),
		displayText(view.Timeout, "none"),
	)
	return err
}

func newTabWriter(w io.Writer) *tabwriter.Writer {
	return tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
}

func displayText(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func writeJSON(w io.Writer, value any) error {
	enc := json.NewEncoder(w)
	return enc.Encode(value)
}
