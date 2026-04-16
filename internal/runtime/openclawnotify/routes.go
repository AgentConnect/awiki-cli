package openclawnotify

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
)

const routesFileName = "openclaw.host-notify.routes.json"

type Route struct {
	Channel string `json:"channel"`
	To      string `json:"to"`
}

type Registry struct {
	Routes []Route `json:"routes"`
}

func RoutesPath(paths appconfig.Paths) string {
	stateDir := strings.TrimSpace(paths.StateDir)
	if stateDir == "" {
		stateDir = filepath.Join(paths.WorkspaceHomeDir, "runtime")
	}
	return filepath.Join(stateDir, routesFileName)
}

func ResolveRouteInput(channel string, to string, sessionKey string) (Route, error) {
	channel = strings.TrimSpace(channel)
	to = strings.TrimSpace(to)
	sessionKey = strings.TrimSpace(sessionKey)

	switch {
	case sessionKey != "" && (channel != "" || to != ""):
		return Route{}, fmt.Errorf("provide either --session-key or --channel/--to, not both")
	case sessionKey != "":
		return ParseSessionKey(sessionKey)
	case channel == "" || to == "":
		return Route{}, fmt.Errorf("route requires either --session-key or both --channel and --to")
	default:
		return NormalizeRoute(Route{Channel: channel, To: to})
	}
}

func ParseSessionKey(sessionKey string) (Route, error) {
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" {
		return Route{}, fmt.Errorf("session key is required")
	}
	parts := strings.Split(sessionKey, ":")
	if len(parts) < 5 {
		return Route{}, fmt.Errorf("unsupported session key format %q", sessionKey)
	}
	if parts[0] != "agent" {
		return Route{}, fmt.Errorf("unsupported session key prefix %q", sessionKey)
	}
	switch parts[3] {
	case "direct", "group", "channel", "room":
	default:
		return Route{}, fmt.Errorf("unsupported session key route type %q", parts[3])
	}
	return NormalizeRoute(Route{
		Channel: parts[2],
		To:      strings.Join(parts[4:], ":"),
	})
}

func NormalizeRoute(route Route) (Route, error) {
	normalized := Route{
		Channel: strings.ToLower(strings.TrimSpace(route.Channel)),
		To:      strings.TrimSpace(route.To),
	}
	if normalized.Channel == "" {
		return Route{}, fmt.Errorf("route channel is required")
	}
	if normalized.To == "" {
		return Route{}, fmt.Errorf("route target is required")
	}
	return normalized, nil
}

func LoadRoutes(paths appconfig.Paths) ([]Route, error) {
	registryPath := RoutesPath(paths)
	raw, err := os.ReadFile(registryPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []Route{}, nil
		}
		return nil, fmt.Errorf("read route registry: %w", err)
	}
	var registry Registry
	if err := json.Unmarshal(raw, &registry); err != nil {
		return nil, fmt.Errorf("parse route registry: %w", err)
	}
	normalized := make([]Route, 0, len(registry.Routes))
	seen := make(map[string]struct{}, len(registry.Routes))
	for _, route := range registry.Routes {
		item, err := NormalizeRoute(route)
		if err != nil {
			return nil, fmt.Errorf("normalize route registry entry: %w", err)
		}
		key := routeKey(item)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, item)
	}
	return normalized, nil
}

func AddRoute(paths appconfig.Paths, route Route) (Route, bool, []Route, error) {
	normalized, err := NormalizeRoute(route)
	if err != nil {
		return Route{}, false, nil, err
	}
	routes, err := LoadRoutes(paths)
	if err != nil {
		return Route{}, false, nil, err
	}
	for _, existing := range routes {
		if routeKey(existing) == routeKey(normalized) {
			return normalized, false, routes, nil
		}
	}
	routes = append(routes, normalized)
	if err := WriteRoutes(paths, routes); err != nil {
		return Route{}, false, nil, err
	}
	return normalized, true, routes, nil
}

func RemoveRoute(paths appconfig.Paths, route Route) (Route, bool, []Route, error) {
	normalized, err := NormalizeRoute(route)
	if err != nil {
		return Route{}, false, nil, err
	}
	routes, err := LoadRoutes(paths)
	if err != nil {
		return Route{}, false, nil, err
	}
	filtered := make([]Route, 0, len(routes))
	removed := false
	for _, existing := range routes {
		if routeKey(existing) == routeKey(normalized) {
			removed = true
			continue
		}
		filtered = append(filtered, existing)
	}
	if !removed {
		return normalized, false, routes, nil
	}
	if err := WriteRoutes(paths, filtered); err != nil {
		return Route{}, false, nil, err
	}
	return normalized, true, filtered, nil
}

func WriteRoutes(paths appconfig.Paths, routes []Route) error {
	registryPath := RoutesPath(paths)
	if err := os.MkdirAll(filepath.Dir(registryPath), 0o700); err != nil {
		return fmt.Errorf("create route registry dir: %w", err)
	}
	normalized := make([]Route, 0, len(routes))
	seen := make(map[string]struct{}, len(routes))
	for _, route := range routes {
		item, err := NormalizeRoute(route)
		if err != nil {
			return err
		}
		key := routeKey(item)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, item)
	}
	raw, err := json.MarshalIndent(Registry{Routes: normalized}, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal route registry: %w", err)
	}
	if err := writeAtomicFile(registryPath, append(raw, '\n'), 0o600); err != nil {
		return fmt.Errorf("write route registry: %w", err)
	}
	return nil
}

func routeKey(route Route) string {
	return route.Channel + "\x00" + route.To
}

func writeAtomicFile(path string, content []byte, mode os.FileMode) error {
	tempFile, err := os.CreateTemp(filepath.Dir(path), ".routes-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp route registry: %w", err)
	}
	tempPath := tempFile.Name()
	cleanup := true
	defer func() {
		_ = tempFile.Close()
		if cleanup {
			_ = os.Remove(tempPath)
		}
	}()

	if _, err := tempFile.Write(content); err != nil {
		return fmt.Errorf("write temp route registry: %w", err)
	}
	if err := tempFile.Sync(); err != nil {
		return fmt.Errorf("sync temp route registry: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("close temp route registry: %w", err)
	}
	if err := os.Chmod(tempPath, mode); err != nil {
		return fmt.Errorf("chmod temp route registry: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replace route registry: %w", err)
	}
	cleanup = false

	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return fmt.Errorf("open route registry dir: %w", err)
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil {
		return fmt.Errorf("sync route registry dir: %w", err)
	}
	return nil
}
