package openclawnotify

import (
	"os"
	"path/filepath"
	"testing"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
)

func TestResolveRouteInputSupportsChannelAndTo(t *testing.T) {
	t.Parallel()

	route, err := ResolveRouteInput(" FeiShu ", " ou_123 ", "")
	if err != nil {
		t.Fatalf("ResolveRouteInput() error = %v", err)
	}
	if route.Channel != "feishu" {
		t.Fatalf("route.Channel = %q, want feishu", route.Channel)
	}
	if route.To != "ou_123" {
		t.Fatalf("route.To = %q, want ou_123", route.To)
	}
}

func TestResolveRouteInputSupportsSessionKey(t *testing.T) {
	t.Parallel()

	route, err := ResolveRouteInput("", "", "agent:main:telegram:direct:123456")
	if err != nil {
		t.Fatalf("ResolveRouteInput() error = %v", err)
	}
	if route.Channel != "telegram" || route.To != "123456" {
		t.Fatalf("route = %#v, want telegram/123456", route)
	}
}

func TestAddAndRemoveRoutePersistRegistry(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	paths := appconfig.Paths{
		WorkspaceHomeDir: root,
		StateDir:         filepath.Join(root, "runtime"),
	}

	route, added, routes, err := AddRoute(paths, Route{Channel: "feishu", To: "ou_123"})
	if err != nil {
		t.Fatalf("AddRoute() error = %v", err)
	}
	if !added {
		t.Fatal("added = false, want true")
	}
	if route.Channel != "feishu" || route.To != "ou_123" {
		t.Fatalf("route = %#v", route)
	}
	if len(routes) != 1 {
		t.Fatalf("len(routes) = %d, want 1", len(routes))
	}

	_, added, routes, err = AddRoute(paths, Route{Channel: "Feishu", To: "ou_123"})
	if err != nil {
		t.Fatalf("second AddRoute() error = %v", err)
	}
	if added {
		t.Fatal("added = true, want false for duplicate route")
	}
	if len(routes) != 1 {
		t.Fatalf("len(routes) after duplicate add = %d, want 1", len(routes))
	}

	loaded, err := LoadRoutes(paths)
	if err != nil {
		t.Fatalf("LoadRoutes() error = %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("len(loaded) = %d, want 1", len(loaded))
	}

	_, removed, routes, err := RemoveRoute(paths, Route{Channel: "feishu", To: "ou_123"})
	if err != nil {
		t.Fatalf("RemoveRoute() error = %v", err)
	}
	if !removed {
		t.Fatal("removed = false, want true")
	}
	if len(routes) != 0 {
		t.Fatalf("len(routes) after remove = %d, want 0", len(routes))
	}
}

func TestLoadRoutesMissingFileReturnsEmpty(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	paths := appconfig.Paths{
		WorkspaceHomeDir: root,
		StateDir:         filepath.Join(root, "runtime"),
	}
	routes, err := LoadRoutes(paths)
	if err != nil {
		t.Fatalf("LoadRoutes() error = %v", err)
	}
	if len(routes) != 0 {
		t.Fatalf("len(routes) = %d, want 0", len(routes))
	}
	if _, err := os.Stat(RoutesPath(paths)); !os.IsNotExist(err) {
		t.Fatalf("route registry file should not be created on read, stat err = %v", err)
	}
}
