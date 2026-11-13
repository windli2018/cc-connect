// Package piweb connects cc-connect directly to a pi-web session as an ACP
// agent over a Unix socket. Unlike type=acp, no connector subprocess is
// spawned: the pi-web side (aiti-channels extension) runs the ACP server
// in-process and listens on <pi-web dataDir>/acp/<instance>.sock; this
// package dials that socket and delegates the whole ACP session machinery
// (handshake, session/update translation, permission handling) to
// agent/acp.NewSession.
//
// pi-web has no authentication concept and does not advertise ACP modes or
// session/list, so there is no auth_method / mode plumbing here — the IM
// /mode picker stays hidden and /ls returns empty for this agent type.
package piweb

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/chenhg5/cc-connect/agent/acp"
	"github.com/chenhg5/cc-connect/core"
)

// dialTimeout bounds how long StartSession waits for the Unix socket to
// accept. The ACP server lives inside the pi-web process; when pi-web is
// down the connect fails fast, but a hung accept would otherwise block the
// engine's session start indefinitely.
const dialTimeout = 5 * time.Second

func init() {
	core.RegisterAgent("piweb", New)
}

type Agent struct {
	mu          sync.RWMutex
	workDir     string
	socket      string
	displayName string
}

// New builds a piweb agent from project options.
// Required: options["socket"] — path of the pi-web ACP Unix socket
// (default instance: ~/.pi/web/data/aiti-channels/acp/default.sock).
// Optional: options["work_dir"], options["display_name"].
func New(opts map[string]any) (core.Agent, error) {
	workDir, _ := opts["work_dir"].(string)
	if workDir == "" {
		workDir = "."
	}
	socket, _ := opts["socket"].(string)
	socket = strings.TrimSpace(socket)
	if socket == "" {
		return nil, fmt.Errorf("piweb: agent option \"socket\" is required (path of the pi-web ACP Unix socket, e.g. ~/.pi/web/data/aiti-channels/acp/default.sock)")
	}
	displayName, _ := opts["display_name"].(string)
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		displayName = "pi-web"
	}
	return &Agent{
		workDir:     workDir,
		socket:      socket,
		displayName: displayName,
	}, nil
}

func (a *Agent) Name() string { return "piweb" }

func (a *Agent) SetWorkDir(dir string) {
	a.mu.Lock()
	a.workDir = dir
	a.mu.Unlock()
}

func (a *Agent) GetWorkDir() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.workDir
}

// WorkspaceAgentOptions snapshots the options needed to recreate this agent
// in another workspace; the engine overrides work_dir itself.
func (a *Agent) WorkspaceAgentOptions() map[string]any {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return map[string]any{
		"socket":       a.socket,
		"display_name": a.displayName,
	}
}

func (a *Agent) StartSession(ctx context.Context, sessionID string) (core.AgentSession, error) {
	a.mu.RLock()
	socket := a.socket
	workDir := a.workDir
	a.mu.RUnlock()

	conn, err := (&net.Dialer{Timeout: dialTimeout}).DialContext(ctx, "unix", socket)
	if err != nil {
		return nil, fmt.Errorf("piweb: dial pi-web ACP socket %s: %w (is pi-web running with its ACP server listening on this socket?)", socket, err)
	}
	return acp.NewSession(ctx, conn, acp.SessionConfig{
		WorkDir:         workDir,
		ResumeSessionID: sessionID,
	})
}

// ListSessions returns nil: the pi-web ACP server does not advertise
// sessionCapabilities.list, and cross-process /ls (if ever needed) belongs
// on pi-web's REST API rather than the ACP session surface.
func (a *Agent) ListSessions(ctx context.Context) ([]core.AgentSessionInfo, error) {
	return nil, nil
}

func (a *Agent) Stop() error { return nil }

// -- AgentDoctorInfo --

func (a *Agent) CLIBinaryName() string { return "pi-web" }

func (a *Agent) CLIDisplayName() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.displayName == "" {
		return "pi-web"
	}
	return a.displayName
}
