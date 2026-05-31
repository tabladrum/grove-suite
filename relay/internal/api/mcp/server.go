// Package mcp exposes Relay's certified-merge engine as a stdio JSON-RPC
// 2.0 server speaking the Model Context Protocol. Five tools are surfaced:
// relay_check, relay_certify, relay_submit, relay_policy, relay_explain.
//
// Each tool maps onto the same engine.Engine wiring the CLI uses; the
// stdio loop borrows the Content-Length framing already used by Grove.
//
// This server is designed for Pre-Flight Autopilot — an agent runs it on
// the laptop, edits files, calls relay_check / relay_submit, and reads
// next-action hints from the response.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"time"

	"github.com/tabladrum/grove-suite/relay/internal/admission"
	"github.com/tabladrum/grove-suite/relay/internal/core"
	"github.com/tabladrum/grove-suite/relay/internal/engine"
	"github.com/tabladrum/grove-suite/relay/internal/intent"
	"github.com/tabladrum/grove-suite/relay/internal/version"
)

// EngineBuilder constructs an *engine.Engine plus its cleanup hook for a
// given repository root. cli.BuildEngine is the canonical implementation.
type EngineBuilder func(repoRoot string) (*engine.Engine, func(), error)

// Server is a stateless MCP server: each tool call constructs a fresh
// Engine from the provided builder.
type Server struct {
	defaultRoot string
	build       EngineBuilder
}

// NewServer returns a Server that uses the given builder for engine wiring.
// defaultRoot is used when a tool call does not pass --repo.
func NewServer(defaultRoot string, build EngineBuilder) *Server {
	return &Server{defaultRoot: defaultRoot, build: build}
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func invalidParams(err error) *rpcError { return &rpcError{Code: -32602, Message: err.Error()} }

// Serve reads JSON-RPC messages from r and writes responses to w.
func (s *Server) Serve(r io.Reader, w io.Writer) error {
	reader := bufio.NewReader(r)
	for {
		message, err := readMessage(reader)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		var req request
		if err := json.Unmarshal(message, &req); err != nil {
			continue
		}
		if req.ID == nil {
			continue
		}
		result, rpcErr := s.handle(req.Method, req.Params)
		if err := writeMessage(w, req.ID, result, rpcErr); err != nil {
			return err
		}
	}
}

func (s *Server) handle(method string, params json.RawMessage) (any, *rpcError) {
	switch method {
	case "initialize":
		return map[string]any{
			"protocolVersion": "2024-11-05",
			"serverInfo":      map[string]string{"name": "relay", "version": version.Version},
			"capabilities":    map[string]any{"tools": map[string]any{}},
		}, nil
	case "tools/list":
		return map[string]any{"tools": toolDescriptors()}, nil
	case "tools/call":
		var call struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal(params, &call); err != nil {
			return nil, invalidParams(err)
		}
		value, err := s.callTool(call.Name, call.Arguments)
		if err != nil {
			return nil, &rpcError{Code: -32000, Message: err.Error()}
		}
		encoded, _ := json.MarshalIndent(value, "", "  ")
		return map[string]any{
			"content": []map[string]string{{"type": "text", "text": string(encoded)}},
		}, nil
	default:
		return nil, &rpcError{Code: -32601, Message: "method not found"}
	}
}

func (s *Server) callTool(name string, args map[string]any) (any, error) {
	switch name {
	case "relay_check":
		return s.runCheck(args, false)
	case "relay_certify":
		return s.runCheck(args, true)
	case "relay_submit":
		return s.runSubmit(args)
	case "relay_policy":
		return s.runPolicy(args)
	case "relay_explain":
		return s.runExplain(args)
	case "relay_intent_open":
		return s.runIntentOpen(args)
	case "relay_intent_update":
		return s.runIntentUpdate(args)
	case "relay_intent_close":
		return s.runIntentClose(args)
	case "relay_intent_list":
		return s.runIntentList(args)
	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
}

// resolveRepo returns the absolute repo root, defaulting to the server's
// configured default or the current working directory.
func (s *Server) resolveRepo(args map[string]any) (string, error) {
	root := stringArg(args, "repo", s.defaultRoot)
	if root == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		root = wd
	}
	return filepath.Abs(root)
}

func (s *Server) runIntentOpen(args map[string]any) (any, error) {
	title := stringArg(args, "title", "")
	if title == "" {
		return nil, errors.New("`title` is required")
	}
	description := stringArg(args, "description", "")
	if description == "" {
		return nil, errors.New("`description` is required")
	}
	root, err := s.resolveRepo(args)
	if err != nil {
		return nil, err
	}
	from := intent.OriginatedFrom{
		Agent: stringArg(args, "agent", ""),
		Model: stringArg(args, "model", ""),
	}
	if ts := stringArg(args, "conversation_ts", ""); ts != "" {
		if t, err := time.Parse(time.RFC3339, ts); err == nil {
			from.ConversationTS = t
		}
	}
	svc := intent.New(root)
	i, path, err := svc.Open(title, description, from)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"intent_id":  i.ID,
		"draft_path": path,
		"schema":     i.Schema,
		"status":     string(i.Status),
		"originated_from": map[string]any{
			"agent":           i.OriginatedFrom.Agent,
			"model":           i.OriginatedFrom.Model,
			"prompt_hash":     i.OriginatedFrom.PromptHash,
			"conversation_ts": i.OriginatedFrom.ConversationTS,
		},
	}, nil
}

func (s *Server) runIntentUpdate(args map[string]any) (any, error) {
	id := stringArg(args, "intent_id", "")
	if id == "" {
		return nil, errors.New("`intent_id` is required")
	}
	patch, _ := args["patch"].(map[string]any)
	if patch == nil {
		return nil, errors.New("`patch` (object) is required")
	}
	root, err := s.resolveRepo(args)
	if err != nil {
		return nil, err
	}
	svc := intent.New(root)
	i, err := svc.Update(id, patch)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"intent_id":  i.ID,
		"draft_path": filepath.Join(root, ".relay", ".cache", "intents", id+".draft.yaml"),
		"title":      i.Title,
		"status":     string(i.Status),
	}, nil
}

func (s *Server) runIntentClose(args map[string]any) (any, error) {
	id := stringArg(args, "intent_id", "")
	if id == "" {
		return nil, errors.New("`intent_id` is required")
	}
	root, err := s.resolveRepo(args)
	if err != nil {
		return nil, err
	}
	agentIdent := stringArg(args, "agent", "")
	svc := intent.New(root)
	committedPath, hash, trailer, err := svc.Close(id, agentIdent)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"intent_id":      id,
		"committed_path": committedPath,
		"intent_hash":    "sha256:" + hash,
		"trailer_block":  trailer,
		"next_action":    "Run `relay certify --intent " + id + "` (laptop) or `relay submit` (team) to produce the signed commit.",
	}, nil
}

func (s *Server) runIntentList(args map[string]any) (any, error) {
	root, err := s.resolveRepo(args)
	if err != nil {
		return nil, err
	}
	status := stringArg(args, "status", "all")
	svc := intent.New(root)
	entries, err := svc.List(status)
	if err != nil {
		return nil, err
	}
	return map[string]any{"intents": entries}, nil
}

// changeSetFromArgs assembles a *core.ChangeSet from the call arguments.
func (s *Server) changeSetFromArgs(args map[string]any) (*core.ChangeSet, string, error) {
	diff := stringArg(args, "diff", "")
	if diff == "" {
		if p := stringArg(args, "diff_path", ""); p != "" {
			data, err := os.ReadFile(p)
			if err != nil {
				return nil, "", fmt.Errorf("read diff_path: %w", err)
			}
			diff = string(data)
		}
	}
	if diff == "" {
		return nil, "", errors.New("either `diff` or `diff_path` is required")
	}
	root := stringArg(args, "repo", s.defaultRoot)
	if root == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, "", err
		}
		root = wd
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, "", err
	}
	cs := &core.ChangeSet{
		IntentID:    stringArg(args, "intent", ""),
		IntentBrief: stringArg(args, "brief", ""),
		Diff:        diff,
		Author:      stringArg(args, "author", ""),
		AgentModel:  stringArg(args, "model", ""),
		RepoRoot:    abs,
	}
	return cs, abs, nil
}

func (s *Server) runCheck(args map[string]any, _ bool) (any, error) {
	cs, root, err := s.changeSetFromArgs(args)
	if err != nil {
		return nil, err
	}
	e, cleanup, err := s.build(root)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	res, err := e.Check(context.Background(), cs)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (s *Server) runSubmit(args map[string]any) (any, error) {
	cs, root, err := s.changeSetFromArgs(args)
	if err != nil {
		return nil, err
	}
	e, cleanup, err := s.build(root)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	e.Admit = admission.NewGitAdmitter()
	res, err := e.Submit(context.Background(), cs)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (s *Server) runPolicy(args map[string]any) (any, error) {
	root := stringArg(args, "repo", s.defaultRoot)
	if root == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		root = wd
	}
	e, cleanup, err := s.build(root)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	gates := e.Policies.Names()
	enabled := map[string]bool{}
	for name, cfg := range e.Config.Policies {
		enabled[name] = cfg.Enabled
	}
	return map[string]any{
		"registered_gates": gates,
		"enabled":          enabled,
		"build_gates":     e.BuildGates,
		"analysis_gates":     e.AnalysisGates,
		"config_source":    "merged: defaults + .relay/relay.yaml",
	}, nil
}

// helpTable maps "<gate>" or "<gate>:<rule>" to a longer-form explanation.
// The MCP `relay_explain` tool surfaces these so an agent can resolve a
// finding without scraping documentation.
var helpTable = map[string]string{
	"secrets": "A secret (API key, private key, token) was detected in the diff. " +
		"Remove the literal, replace it with an environment-variable read, and rotate the credential out-of-band.",
	"secrets:aws-access-key-id": "AWS access key ID detected. Rotate the key in IAM, " +
		"remove from the diff, and load via the SDK default credential chain (AWS_PROFILE / IRSA / metadata).",
	"secrets:github-token": "GitHub token detected. Revoke at https://github.com/settings/tokens, " +
		"replace with a fine-grained PAT loaded from $GITHUB_TOKEN or repo secrets.",
	"fileclass": "A denied file class was included in the diff (private key, certificate, " +
		"or other artefact listed in policy.fileclass.options). Remove the file or update the policy.",
	"deps": "A dependency vulnerability of High/Critical severity was reported by govulncheck. " +
		"Upgrade the affected module, pin transitive deps, or document a justified deferral.",
	"coverage": "Stage 1 coverage fell below the configured minimum. Add tests for the added " +
		"statements or annotate irreducible lines with `// relay:no-test-required`.",
	"stage1": "Stage 1 build or test failed. Read the analyzer output, reproduce locally with `go test ./...`, " +
		"fix the failure, and re-run `relay_check`.",
	"path": "A denied path was modified (e.g. .git/, vendor/, generated outputs). " +
		"Move the change to an allowed path or update policy.path.options.deny.",
	"size": "Diff exceeded the configured max_files or max_added_lines. Split into smaller intents.",
}

func (s *Server) runExplain(args map[string]any) (any, error) {
	gate := stringArg(args, "gate", "")
	if gate == "" {
		return nil, errors.New("`gate` is required")
	}
	rule := stringArg(args, "rule", "")
	key := gate
	if rule != "" {
		key = gate + ":" + rule
	}
	if text, ok := helpTable[key]; ok {
		return map[string]string{"key": key, "explanation": text, "source": "builtin"}, nil
	}
	if text, ok := helpTable[gate]; ok {
		return map[string]string{
			"key":         gate,
			"explanation": text,
			"source":      "builtin (fallback to gate-level)",
			"hint":        fmt.Sprintf("no specific entry for rule %q", rule),
		}, nil
	}
	return map[string]string{
		"key":         key,
		"explanation": "No builtin explanation. Re-run relay_check with --json and read the finding's `message` and `next_action` fields.",
		"source":      "fallback",
	}, nil
}

func stringArg(args map[string]any, key, fallback string) string {
	if v, ok := args[key].(string); ok && v != "" {
		return v
	}
	return fallback
}

func readMessage(reader *bufio.Reader) ([]byte, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(strings.ToLower(line), "content-length:") {
		return []byte(strings.TrimSpace(line)), nil
	}
	lengthText := strings.TrimSpace(strings.TrimPrefix(strings.ToLower(line), "content-length:"))
	length, err := strconv.Atoi(lengthText)
	if err != nil {
		return nil, err
	}
	for {
		line, err = reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(line) == "" {
			break
		}
	}
	payload := make([]byte, length)
	_, err = io.ReadFull(reader, payload)
	return payload, err
}

func writeMessage(w io.Writer, id any, result any, rpcErr *rpcError) error {
	resp := map[string]any{"jsonrpc": "2.0", "id": id}
	if rpcErr != nil {
		resp["error"] = rpcErr
	} else {
		resp["result"] = result
	}
	payload, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "Content-Length: %d\r\n\r\n%s", len(payload), payload)
	return err
}
