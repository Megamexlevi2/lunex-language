package lsp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const maxMessageSize = 32 << 20

type Server struct {
	reader     *bufio.Reader
	writer     io.Writer
	writeMu    sync.Mutex
	docs       map[string]*Document
	roots      []string
	shutdown   bool
	exiting    bool
	clientName string
}

func Run() error {
	s := &Server{reader: bufio.NewReader(os.Stdin), writer: os.Stdout, docs: map[string]*Document{}}
	return s.serve()
}

func (s *Server) serve() error {
	for !s.exiting {
		msg, err := s.readMessage()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		if msg.Method == "" {
			if len(msg.ID) > 0 && string(msg.ID) != "null" {
				s.respond(msg.ID, nil, &rpcError{Code: -32600, Message: "Invalid Request"})
			}
			continue
		}
		result, rpcErr := s.handle(msg.Method, msg.Params)
		if len(msg.ID) > 0 && string(msg.ID) != "null" {
			s.respond(msg.ID, result, rpcErr)
		}
	}
	return nil
}

func (s *Server) readMessage() (rpcMessage, error) {
	var msg rpcMessage
	contentLength := -1
	for {
		line, err := s.reader.ReadString('\n')
		if err != nil {
			return msg, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(key), "Content-Length") {
			var n int
			if _, err := fmt.Sscanf(strings.TrimSpace(value), "%d", &n); err == nil {
				contentLength = n
			}
		}
	}
	if contentLength < 0 || contentLength > maxMessageSize {
		return msg, fmt.Errorf("invalid LSP Content-Length: %d", contentLength)
	}
	body := make([]byte, contentLength)
	if _, err := io.ReadFull(s.reader, body); err != nil {
		return msg, err
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	if err := dec.Decode(&msg); err != nil {
		return msg, fmt.Errorf("invalid JSON-RPC message: %w", err)
	}
	return msg, nil
}

func (s *Server) write(value any) {
	body, err := json.Marshal(value)
	if err != nil {
		return
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	fmt.Fprintf(s.writer, "Content-Length: %d\r\n\r\n", len(body))
	s.writer.Write(body)
}

func (s *Server) respond(id json.RawMessage, result any, rpcErr *rpcError) {
	response := map[string]any{"jsonrpc": "2.0", "id": id}
	if rpcErr != nil {
		response["error"] = rpcErr
	} else {
		response["result"] = result
	}
	s.write(response)
}

func (s *Server) notify(method string, params any) {
	s.write(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

func (s *Server) handle(method string, raw json.RawMessage) (any, *rpcError) {
	var params map[string]json.RawMessage
	if len(raw) != 0 {
		if err := json.Unmarshal(raw, &params); err != nil {
			return nil, &rpcError{Code: -32602, Message: "Invalid params"}
		}
	}
	switch method {
	case "initialize":
		return s.initialize(params), nil
	case "initialized", "$/cancelRequest", "$/setTrace", "$/logTrace", "textDocument/didChangeConfiguration", "textDocument/didSave", "workspace/didChangeConfiguration":
		if method == "textDocument/didSave" {
			s.didSave(params)
		}
		return nil, nil
	case "shutdown":
		s.shutdown = true
		return nil, nil
	case "exit":
		s.exiting = true
		return nil, nil
	case "textDocument/didOpen":
		s.didOpen(params)
		return nil, nil
	case "textDocument/didChange":
		s.didChange(params)
		return nil, nil
	case "textDocument/didClose":
		s.didClose(params)
		return nil, nil
	case "textDocument/completion":
		return s.completion(params), nil
	case "completionItem/resolve":
		return s.resolveCompletion(params), nil
	case "textDocument/hover":
		return s.hover(params), nil
	case "textDocument/typeDefinition":
		return s.typeDefinition(params), nil
	case "textDocument/definition", "textDocument/declaration":
		return s.definition(params), nil
	case "textDocument/references":
		return s.references(params), nil
	case "textDocument/prepareRename":
		return s.prepareRename(params), nil
	case "textDocument/rename":
		return s.rename(params), nil
	case "textDocument/documentSymbol":
		return s.documentSymbols(params), nil
	case "workspace/symbol":
		return s.workspaceSymbols(params), nil
	case "textDocument/documentHighlight":
		return s.documentHighlights(params), nil
	case "textDocument/semanticTokens/full":
		return s.semanticTokens(params), nil
	case "textDocument/signatureHelp":
		return s.signatureHelp(params), nil
	case "textDocument/codeAction":
		return s.codeActions(params), nil
	case "textDocument/foldingRange":
		return s.foldingRanges(params), nil
	case "workspace/didChangeWorkspaceFolders":
		s.changeWorkspaceFolders(params)
		return nil, nil
	case "workspace/executeCommand":
		return nil, &rpcError{Code: -32601, Message: "Unknown command"}
	default:
		return nil, &rpcError{Code: -32601, Message: "Method not found: " + method}
	}
}

func decodeField[T any](m map[string]json.RawMessage, key string, out *T) {
	if raw, ok := m[key]; ok {
		_ = json.Unmarshal(raw, out)
	}
}

func (s *Server) initialize(params map[string]json.RawMessage) any {
	var rootURI string
	var rootPath string
	var folders []struct {
		URI  string `json:"uri"`
		Name string `json:"name"`
	}
	decodeField(params, "rootUri", &rootURI)
	decodeField(params, "rootPath", &rootPath)
	decodeField(params, "workspaceFolders", &folders)
	if len(folders) > 0 {
		for _, folder := range folders {
			if path := uriToPath(folder.URI); path != "" {
				s.addRoot(path)
			}
		}
	} else if rootURI != "" {
		s.addRoot(uriToPath(rootURI))
	} else if rootPath != "" {
		s.addRoot(rootPath)
	} else if cwd, err := os.Getwd(); err == nil {
		s.addRoot(cwd)
	}
	var client struct {
		Name string `json:"name"`
	}
	if raw, ok := params["clientInfo"]; ok {
		_ = json.Unmarshal(raw, &client)
	}
	s.clientName = client.Name
	s.scanWorkspace()
	return map[string]any{
		"serverInfo": map[string]any{"name": "Lunex Language Server", "version": "1.0.0"},
		"capabilities": map[string]any{
			"positionEncoding":          "utf-16",
			"textDocumentSync":          map[string]any{"openClose": true, "change": 2, "save": map[string]any{"includeText": true}},
			"completionProvider":        map[string]any{"resolveProvider": true, "triggerCharacters": []string{".", "@", "\"", "'", "/"}},
			"hoverProvider":             true,
			"definitionProvider":        true,
			"declarationProvider":       true,
			"typeDefinitionProvider":    true,
			"referencesProvider":        true,
			"renameProvider":            map[string]any{"prepareProvider": true},
			"documentSymbolProvider":    true,
			"workspaceSymbolProvider":   true,
			"documentHighlightProvider": true,
			"semanticTokensProvider":    map[string]any{"legend": map[string]any{"tokenTypes": semanticTokenTypes, "tokenModifiers": semanticTokenModifiers}, "full": true},
			"signatureHelpProvider":     map[string]any{"triggerCharacters": []string{"(", ","}, "retriggerCharacters": []string{","}},
			"codeActionProvider":        map[string]any{"codeActionKinds": []string{"quickfix"}},
			"foldingRangeProvider":      true,
			"workspace":                 map[string]any{"workspaceFolders": map[string]any{"supported": true, "changeNotifications": true}},
		},
	}
}

func (s *Server) addRoot(path string) {
	if path == "" {
		return
	}
	abs, err := filepath.Abs(path)
	if err == nil {
		path = abs
	}
	for _, root := range s.roots {
		if root == path {
			return
		}
	}
	s.roots = append(s.roots, path)
}

func (s *Server) changeWorkspaceFolders(params map[string]json.RawMessage) {
	var event struct {
		Added []struct {
			URI string `json:"uri"`
		} `json:"added"`
		Removed []struct {
			URI string `json:"uri"`
		} `json:"removed"`
	}
	var raw struct {
		Event json.RawMessage `json:"event"`
	}
	decodeField(params, "event", &raw.Event)
	_ = json.Unmarshal(raw.Event, &event)
	for _, item := range event.Added {
		s.addRoot(uriToPath(item.URI))
	}
	for _, item := range event.Removed {
		path := uriToPath(item.URI)
		for i, root := range s.roots {
			if root == path {
				s.roots = append(s.roots[:i], s.roots[i+1:]...)
				break
			}
		}
	}
	s.scanWorkspace()
}
