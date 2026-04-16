package cli

import (
	"context"
	"fmt"
	"strings"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	"github.com/agentconnect/awiki-cli/internal/output"
	"github.com/agentconnect/awiki-cli/internal/runtime/openclawnotify"
	"github.com/spf13/cobra"
)

func (a *App) runRuntimeHostNotifyOpenClawRouteAdd(cmd *cobra.Command, args []string) error {
	resolved, err := a.resolveConfigForWorkspace()
	if err != nil {
		return a.runtimeExit(err, "Run `awiki-cli doctor` to inspect runtime configuration.")
	}
	format := normalizedFormat(runtimeFormat(resolved))
	route, err := resolveOpenClawRouteFromFlags(cmd)
	if err != nil {
		return output.NewExitError("invalid_argument", 2, err.Error(), "Use --channel and --to, or use --session-key.")
	}
	if a.globals.DryRun {
		data := map[string]any{
			"plan": map[string]any{
				"action":              "host_notify_openclaw_route_add",
				"route":               route,
				"route_registry_path": openclawnotify.RoutesPath(resolved.Paths),
			},
		}
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Dry run: OpenClaw route add planned", nil, identityMetaFromResolved(resolved))
	}

	addedRoute, added, routes, err := openclawnotify.AddRoute(resolved.Paths, route)
	if err != nil {
		return a.runtimeExit(err, "Check write permissions for the runtime route registry.")
	}
	data := map[string]any{
		"route":               addedRoute,
		"routes":              routes,
		"route_registry_path": openclawnotify.RoutesPath(resolved.Paths),
	}
	if !added {
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "OpenClaw route already exists", nil, identityMetaFromResolved(resolved))
	}

	confirmation, warning := sendOpenClawRouteConfirmation(context.Background(), resolved, addedRoute)
	if confirmation != nil {
		data["confirmation"] = confirmation
	}
	var warnings []string
	if warning != "" {
		warnings = append(warnings, warning)
	}
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "OpenClaw route added", warnings, identityMetaFromResolved(resolved))
}

func (a *App) runRuntimeHostNotifyOpenClawRouteList(cmd *cobra.Command, args []string) error {
	resolved, err := a.resolveConfigForWorkspace()
	if err != nil {
		return a.runtimeExit(err, "Run `awiki-cli doctor` to inspect runtime configuration.")
	}
	format := normalizedFormat(runtimeFormat(resolved))
	routes, err := openclawnotify.LoadRoutes(resolved.Paths)
	if err != nil {
		return a.runtimeExit(err, "Check the runtime route registry file.")
	}
	data := map[string]any{"routes": routes}
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "OpenClaw routes loaded", nil, identityMetaFromResolved(resolved))
}

func (a *App) runRuntimeHostNotifyOpenClawRouteRemove(cmd *cobra.Command, args []string) error {
	resolved, err := a.resolveConfigForWorkspace()
	if err != nil {
		return a.runtimeExit(err, "Run `awiki-cli doctor` to inspect runtime configuration.")
	}
	format := normalizedFormat(runtimeFormat(resolved))
	route, err := resolveOpenClawRouteFromFlags(cmd)
	if err != nil {
		return output.NewExitError("invalid_argument", 2, err.Error(), "Use --channel and --to, or use --session-key.")
	}
	if a.globals.DryRun {
		data := map[string]any{
			"plan": map[string]any{
				"action":              "host_notify_openclaw_route_remove",
				"route":               route,
				"route_registry_path": openclawnotify.RoutesPath(resolved.Paths),
			},
		}
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Dry run: OpenClaw route remove planned", nil, identityMetaFromResolved(resolved))
	}

	removedRoute, removed, routes, err := openclawnotify.RemoveRoute(resolved.Paths, route)
	if err != nil {
		return a.runtimeExit(err, "Check write permissions for the runtime route registry.")
	}
	data := map[string]any{
		"route":               removedRoute,
		"routes":              routes,
		"route_registry_path": openclawnotify.RoutesPath(resolved.Paths),
	}
	summary := "OpenClaw route removed"
	if !removed {
		summary = "OpenClaw route not found"
	}
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, summary, nil, identityMetaFromResolved(resolved))
}

func resolveOpenClawRouteFromFlags(cmd *cobra.Command) (openclawnotify.Route, error) {
	channel, _ := cmd.Flags().GetString("channel")
	to, _ := cmd.Flags().GetString("to")
	sessionKey, _ := cmd.Flags().GetString("session-key")
	return openclawnotify.ResolveRouteInput(channel, to, sessionKey)
}

func sendOpenClawRouteConfirmation(ctx context.Context, resolved *appconfig.Resolved, route openclawnotify.Route) (map[string]any, string) {
	settings, err := openclawnotify.ResolveSettings(resolved)
	if err != nil {
		return nil, fmt.Sprintf("route was added, but the confirmation message could not be prepared: %v", err)
	}
	client, err := openclawnotify.NewWebhookClient(settings.HookURL, settings.Token)
	if err != nil {
		return nil, fmt.Sprintf("route was added, but the confirmation message could not be prepared: %v", err)
	}
	response, err := client.Send(ctx, openclawnotify.HookRequest{
		Message:  buildOpenClawRouteConfirmationMessage(route),
		Name:     openclawnotify.FixedHookName,
		WakeMode: "now",
		Deliver:  true,
		Channel:  route.Channel,
		To:       route.To,
	})
	if err != nil {
		return nil, fmt.Sprintf("route was added, but the confirmation message was not accepted by OpenClaw: %v", err)
	}
	return map[string]any{
		"accepted": true,
		"run_id":   response.RunID,
	}, ""
}

func buildOpenClawRouteConfirmationMessage(route openclawnotify.Route) string {
	lines := []string{
		"AWiki notifications are now configured for this conversation.",
		"Future AWiki message notifications will be delivered here.",
		fmt.Sprintf("channel=%s", route.Channel),
		fmt.Sprintf("to=%s", route.To),
	}
	return strings.Join(lines, "\n")
}
