package ui

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/glamour/styles"
	"github.com/rakyll/ate-watch/internal/watcher"
	"github.com/rakyll/ate-watch/pkg/proto/ateapipb"
)

// RenderOptions configures table rendering.
type RenderOptions struct {
	Atespace      string
	Interval      time.Duration
	UseColor      bool
	SelectedIndex int // -1 for no cursor, >= 0 for selected row in interactive mode
}

// compactStyle returns a glamour StyleConfig with zero margins and minimal padding.
func compactStyle(useColor bool) ansi.StyleConfig {
	var s ansi.StyleConfig
	if useColor {
		s = styles.DarkStyleConfig
	} else {
		s = styles.NoTTYStyleConfig
	}
	zero := uint(0)
	s.Document.Margin = &zero
	s.Document.BlockPrefix = ""
	s.Document.BlockSuffix = ""
	s.Document.Prefix = ""
	s.Document.Suffix = ""

	s.Table.Margin = &zero
	s.Table.BlockPrefix = ""
	s.Table.BlockSuffix = ""
	s.Table.Prefix = ""
	s.Table.Suffix = ""

	s.Paragraph.Margin = &zero
	s.Paragraph.BlockPrefix = ""
	s.Paragraph.BlockSuffix = ""
	s.Paragraph.Prefix = ""
	s.Paragraph.Suffix = ""

	s.H2.Margin = &zero
	s.H2.BlockPrefix = ""
	s.H2.BlockSuffix = ""
	s.H2.Prefix = ""
	s.H2.Suffix = ""

	s.H3.Margin = &zero
	s.H3.BlockPrefix = ""
	s.H3.BlockSuffix = ""
	s.H3.Prefix = ""
	s.H3.Suffix = ""

	s.H4.Margin = &zero
	s.H4.BlockPrefix = ""
	s.H4.BlockSuffix = ""
	s.H4.Prefix = ""
	s.H4.Suffix = ""

	s.List.Margin = &zero
	s.List.BlockPrefix = ""
	s.List.BlockSuffix = ""
	s.List.Prefix = ""
	s.List.Suffix = ""

	s.Item.BlockPrefix = ""
	s.Item.BlockSuffix = ""
	s.Item.Prefix = ""
	s.Item.Suffix = ""

	return s
}

// BuildMarkdown constructs the Markdown representation of the actor snapshot.
func BuildMarkdown(snap *watcher.Snapshot, opts RenderOptions) string {
	var sb strings.Builder

	atespaceDisplay := opts.Atespace
	if atespaceDisplay == "" {
		atespaceDisplay = "all"
	}

	totalStr := fmt.Sprintf("%d actors", snap.TotalCount)
	if snap.TotalCount == 1 {
		totalStr = "1 actor"
	}

	var parts []string
	if snap.CountByStatus != nil {
		if c := snap.CountByStatus[ateapipb.Actor_STATUS_RUNNING]; c > 0 {
			parts = append(parts, fmt.Sprintf("%d running", c))
		}
		if c := snap.CountByStatus[ateapipb.Actor_STATUS_SUSPENDING]; c > 0 {
			parts = append(parts, fmt.Sprintf("%d suspending", c))
		}
		if c := snap.CountByStatus[ateapipb.Actor_STATUS_SUSPENDED]; c > 0 {
			parts = append(parts, fmt.Sprintf("%d suspended", c))
		}
		if c := snap.CountByStatus[ateapipb.Actor_STATUS_RESUMING]; c > 0 {
			parts = append(parts, fmt.Sprintf("%d resuming", c))
		}
		if c := snap.CountByStatus[ateapipb.Actor_STATUS_PAUSING]; c > 0 {
			parts = append(parts, fmt.Sprintf("%d pausing", c))
		}
		if c := snap.CountByStatus[ateapipb.Actor_STATUS_PAUSED]; c > 0 {
			parts = append(parts, fmt.Sprintf("%d paused", c))
		}
		if c := snap.CountByStatus[ateapipb.Actor_STATUS_CRASHED]; c > 0 {
			parts = append(parts, fmt.Sprintf("%d crashed", c))
		}
	}

	statusSummary := ""
	if len(parts) > 0 {
		statusSummary = " (" + strings.Join(parts, ", ") + ")"
	}

	if opts.Interval > 0 {
		fmt.Fprintf(&sb, "**Every %s: ate-watch (%s)** • %s%s • %s\n\n",
			opts.Interval, atespaceDisplay,
			totalStr, statusSummary,
			snap.Timestamp.Format("15:04:05 MST"))
	} else {
		fmt.Fprintf(&sb, "**ate-watch (%s)** • %s%s • %s\n\n",
			atespaceDisplay,
			totalStr, statusSummary,
			snap.Timestamp.Format("15:04:05 MST"))
	}

	if snap.LastError != nil {
		fmt.Fprintf(&sb, "> **Error:** %v\n\n", snap.LastError)
	}

	if len(snap.Actors) == 0 {
		if snap.LastError == nil {
			sb.WriteString("_No actors found._\n")
		}
	} else {
		hasCursor := opts.SelectedIndex >= 0
		if hasCursor {
			sb.WriteString("| | STATUS | AGE | ATESPACE | NAME | TEMPLATE |\n")
			sb.WriteString("| :--- | :--- | :--- | :--- | :--- | :--- |\n")
		} else {
			sb.WriteString("| STATUS | AGE | ATESPACE | NAME | TEMPLATE |\n")
			sb.WriteString("| :--- | :--- | :--- | :--- | :--- |\n")
		}

		for i, s := range snap.Actors {
			a := s.Actor
			atespace := a.GetMetadata().GetAtespace()
			if atespace == "" {
				atespace = "-"
			}
			name := a.GetMetadata().GetName()

			template := "-"
			if a.GetActorTemplateName() != "" {
				if a.GetActorTemplateNamespace() != "" {
					template = a.GetActorTemplateNamespace() + "/" + a.GetActorTemplateName()
				} else {
					template = a.GetActorTemplateName()
				}
			}

			status := formatStatusMarkdown(a.GetStatus())
			age := FormatAge(a.GetMetadata().GetCreateTime())

			if hasCursor {
				cursor := " "
				if i == opts.SelectedIndex {
					cursor = ">"
				}
				fmt.Fprintf(&sb, "| %s | %s | %s | %s | %s | %s |\n",
					cursor, status, age, atespace, name, template)
			} else {
				fmt.Fprintf(&sb, "| %s | %s | %s | %s | %s |\n",
					status, age, atespace, name, template)
			}
		}

		if hasCursor {
			sb.WriteString("\n_[↑/↓/j/k] Select • [d/Enter] Describe • [Esc] Exit_\n")
		}
	}

	return sb.String()
}

// BuildDescribeMarkdown constructs a detailed inspector view for an actor.
func BuildDescribeMarkdown(a *ateapipb.Actor) string {
	if a == nil {
		return "_No actor selected._\n"
	}
	var sb strings.Builder

	atespace := a.GetMetadata().GetAtespace()
	if atespace == "" {
		atespace = "default"
	}
	name := a.GetMetadata().GetName()
	statusStr := formatStatusMarkdown(a.GetStatus())

	fmt.Fprintf(&sb, "**%s/%s**\n\n", atespace, name)
	fmt.Fprintf(&sb, "* **Status:** %s\n", statusStr)

	template := "<none>"
	if a.GetActorTemplateName() != "" {
		if a.GetActorTemplateNamespace() != "" {
			template = a.GetActorTemplateNamespace() + "/" + a.GetActorTemplateName()
		} else {
			template = a.GetActorTemplateName()
		}
	}
	fmt.Fprintf(&sb, "* **Template:** `%s`\n", template)
	fmt.Fprintf(&sb, "* **Age:** %s\n", FormatAge(a.GetMetadata().GetCreateTime()))
	fmt.Fprintf(&sb, "* **Endpoint:** `%s.%s.atenet`\n", name, atespace)

	pod := "-"
	if a.GetAteomPodName() != "" {
		if a.GetAteomPodNamespace() != "" {
			pod = a.GetAteomPodNamespace() + "/" + a.GetAteomPodName()
		} else {
			pod = a.GetAteomPodName()
		}
	}
	fmt.Fprintf(&sb, "* **Worker Pod:** `%s`\n", pod)

	ip := a.GetAteomPodIp()
	if ip == "" {
		ip = "-"
	}
	fmt.Fprintf(&sb, "* **Worker IP:** %s\n", ip)

	pool := a.GetWorkerPoolName()
	if pool == "" {
		pool = "-"
	}
	fmt.Fprintf(&sb, "* **Worker Pool:** `%s`\n", pool)

	if a.GetAteomPodUid() != "" {
		fmt.Fprintf(&sb, "* **Worker Pod UID:** `%s`\n", a.GetAteomPodUid())
	}

	fmt.Fprintf(&sb, "* **Version:** %d\n", a.GetMetadata().GetVersion())
	if a.GetMetadata().GetUid() != "" {
		fmt.Fprintf(&sb, "* **UID:** `%s`\n", a.GetMetadata().GetUid())
	}
	if a.GetMetadata().GetCreateTime() != nil {
		fmt.Fprintf(&sb, "* **Created:** %s\n", a.GetMetadata().GetCreateTime().AsTime().Format(time.RFC3339))
	}
	if a.GetMetadata().GetUpdateTime() != nil {
		fmt.Fprintf(&sb, "* **Updated:** %s\n", a.GetMetadata().GetUpdateTime().AsTime().Format(time.RFC3339))
	}

	if inProgress := a.GetInProgressSnapshot(); inProgress != "" {
		fmt.Fprintf(&sb, "* **In-Progress Snapshot:** `%s`\n", inProgress)
	}
	if snapInfo := a.GetLatestSnapshotInfo(); snapInfo != nil {
		if ext := snapInfo.GetExternal(); ext != nil && ext.GetSnapshotUriPrefix() != "" {
			fmt.Fprintf(&sb, "* **Snapshot:** `%s`\n", ext.GetSnapshotUriPrefix())
		}
		if loc := snapInfo.GetLocal(); loc != nil && loc.GetSnapshotPrefix() != "" {
			fmt.Fprintf(&sb, "* **Local Snapshot Prefix:** `%s`\n", loc.GetSnapshotPrefix())
			if len(loc.GetNodeVmsWithLocalSnapshots()) > 0 {
				fmt.Fprintf(&sb, "* **Node VMs with Local Snapshots:** %s\n", strings.Join(loc.GetNodeVmsWithLocalSnapshots(), ", "))
			}
		}
	}

	sb.WriteString("\n_Press [Esc] or [d] to return to table_\n")
	return sb.String()
}

func formatStatusMarkdown(s ateapipb.Actor_Status) string {
	str := s.String()
	if len(str) > 7 && str[:7] == "STATUS_" {
		str = str[7:]
	}
	return "**" + str + "**"
}

// ColorizeRenderedOutput injects ANSI color sequences onto status words.
func ColorizeRenderedOutput(text string) string {
	replacements := []struct {
		word  string
		color string
	}{
		{"RUNNING", Green + Bold + "RUNNING" + Reset},
		{"SUSPENDING", Yellow + Bold + "SUSPENDING" + Reset},
		{"SUSPENDED", Cyan + "SUSPENDED" + Reset},
		{"RESUMING", Yellow + Bold + "RESUMING" + Reset},
		{"PAUSING", Yellow + Bold + "PAUSING" + Reset},
		{"PAUSED", Blue + "PAUSED" + Reset},
		{"CRASHED", Red + Bold + "CRASHED" + Reset},
	}
	for _, r := range replacements {
		text = strings.ReplaceAll(text, r.word, r.color)
	}
	return text
}

// RenderMarkdown renders Markdown to a styled, colorized terminal string.
func RenderMarkdown(md string, useColor bool) (string, error) {
	r, err := glamour.NewTermRenderer(
		glamour.WithStyles(compactStyle(useColor)),
		glamour.WithWordWrap(0),
	)
	if err != nil {
		return md, err
	}

	out, err := r.Render(md)
	if err != nil {
		return md, err
	}

	trimmed := strings.TrimSpace(out)
	if useColor {
		trimmed = ColorizeRenderedOutput(trimmed)
	}
	return trimmed + "\n", nil
}

// RenderTable renders a complete Markdown table snapshot directly to w.
func RenderTable(w io.Writer, snap *watcher.Snapshot, opts RenderOptions) error {
	md := BuildMarkdown(snap, opts)
	out, err := RenderMarkdown(md, opts.UseColor)
	if err != nil {
		_, _ = fmt.Fprint(w, out)
		return err
	}
	_, err = fmt.Fprint(w, out)
	return err
}
