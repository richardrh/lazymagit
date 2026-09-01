package keymap

import (
	"encoding/json"
	"fmt"
	"strings"
)

// RenderLedger is pure: it renders the embedded pinned manifest and registry
// without reading or writing the working tree.
func RenderLedger() (string, error) {
	var m manifest
	if err := json.Unmarshal(upstreamManifest, &m); err != nil {
		return "", err
	}
	var out strings.Builder
	fmt.Fprintf(&out, "# Magit v4.7 status keybinding ledger\n\nGenerated from the vendored manifest for Magit %s (`%s`, `%s`, clean checkout). Run `go run ./internal/keymap/cmd/keymapdoc` to update or add `-check` to verify drift.\n\n", m.Upstream.Version, m.Upstream.Tag, m.Upstream.Commit)
	out.WriteString("| # | Upstream key | Canonical input | Upstream command | Kind | Domain | Layer | Source | Classification | Current status |\n|---:|---|---|---|---|---|---|---|---|---|\n")
	count := renderTopLedger(&out, m)
	occurrences := 0
	for _, tr := range m.Transients {
		occurrences += len(tr.Entries)
	}
	fmt.Fprintf(&out, "\n## Manifest identity\n\n- Schema: **%s**\n- Mode: **%s**\n- Effective map chain: `%s`\n- Effective top-level status bindings: **%d**\n- Recursively reachable transients: **%d**\n- Transient entry occurrences: **%d**\n", m.SchemaVersion, m.Scope.Mode, strings.Join(m.Scope.MapChain, "` → `"), count, len(m.Transients), occurrences)
	out.WriteString("\n## Recursively reachable transients\n\nAll manifest transients are generated from effective status bindings and transient suffix edges. Every occurrence is retained, including infixes, conditional duplicate keys, provenance, and multi-token suffixes. Infixes are actionable only when static capability data declares a consumer.\n")
	renderTransientLedger(&out, m)
	return out.String(), nil
}

func renderTopLedger(out *strings.Builder, m manifest) int {
	byTop := map[string]Binding{}
	for _, b := range Registry() {
		if b.EffectiveTop {
			byTop[b.UpstreamKey] = b
		}
	}
	count := 0
	for _, top := range m.Top {
		if !top.Effective {
			continue
		}
		count++
		b := byTop[top.Key]
		status := b.Unavailable
		if b.Handler == HandlerExecute {
			status = "registered handler"
		} else if b.Handler == HandlerPrefix {
			status = "registered transient"
		}
		fmt.Fprintf(out, "| %d | `%s` | `%s` | `%s` | %s | %s | `%s` | `%s:%d (%s)` | `%s` | %s |\n", count, ledgerEscape(top.Key), ledgerEscape(strings.Join(b.Sequence, " ")), top.Command, top.Kind, top.Domain, top.Layer, top.Source.File, top.Source.Line, top.Source.Definition, b.Parity, status)
	}
	return count
}

func renderTransientLedger(out *strings.Builder, m manifest) {
	routes := transientRoutes(m)
	bindings := map[string]Binding{}
	for _, binding := range Registry() {
		if binding.Scheme == SchemeMagit && binding.Occurrence != "" {
			bindings[binding.Occurrence] = binding
		}
	}
	for _, tr := range m.Transients {
		prefix := strings.Join(routes[tr.Name], " ")
		fmt.Fprintf(out, "\n### `%s` — %s (%d occurrences)\n\n| Key | Command | Kind | Group | Conditions | Classification | Current status |\n|---|---|---|---|---|---|---|\n", prefix, tr.Name, len(tr.Entries))
		for index, row := range tr.Entries {
			group := ""
			if len(row.Groups) > 0 {
				group = row.Groups[0]
			}
			conditions := make([]string, len(row.Conditions))
			for i, c := range row.Conditions {
				conditions[i] = c.Type + ": " + c.Expression
			}
			binding := bindings[fmt.Sprintf("%s:%02d", tr.Name, index)]
			status := binding.Unavailable
			if binding.Handler == HandlerExecute {
				status = "TUI workflow handler (startup-validated)"
			} else if binding.Kind == KindInfix && len(OptionConsumerCommands(prefix, binding.UpstreamCommand)) > 0 {
				status = "actionable TUI option"
			} else if binding.Kind == KindInfix && strings.Contains(strings.Join(binding.Conditions, " "), "direct-configure") {
				status = "actionable in corresponding Configure dialog"
			}
			fmt.Fprintf(out, "| `%s` | `%s` | **%s** | %s | %s | `%s` | %s |\n", ledgerEscape(row.Key), ledgerEscape(row.Command), row.Kind, ledgerEscape(group), ledgerEscape(strings.Join(conditions, "; ")), binding.Parity, ledgerEscape(status))
		}
	}
}

func ledgerEscape(s string) string { return strings.ReplaceAll(s, "|", "\\|") }
