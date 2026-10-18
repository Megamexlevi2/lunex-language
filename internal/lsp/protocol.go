package lsp

import "encoding/json"

type rpcMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

type position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type lspRange struct {
	Start position `json:"start"`
	End   position `json:"end"`
}

type location struct {
	URI   string   `json:"uri"`
	Range lspRange `json:"range"`
}

type textEdit struct {
	Range   lspRange `json:"range"`
	NewText string   `json:"newText"`
}

type diagnostic struct {
	Range              lspRange `json:"range"`
	Severity           int      `json:"severity"`
	Code               any      `json:"code,omitempty"`
	Source             string   `json:"source,omitempty"`
	Message            string   `json:"message"`
	Tags               []int    `json:"tags,omitempty"`
	RelatedInformation []any    `json:"relatedInformation,omitempty"`
}

type completionItem struct {
	Label               string     `json:"label"`
	Kind                int        `json:"kind,omitempty"`
	Detail              string     `json:"detail,omitempty"`
	Documentation       string     `json:"documentation,omitempty"`
	SortText            string     `json:"sortText,omitempty"`
	FilterText          string     `json:"filterText,omitempty"`
	InsertText          string     `json:"insertText,omitempty"`
	InsertTextFormat    int        `json:"insertTextFormat,omitempty"`
	TextEdit            *textEdit  `json:"textEdit,omitempty"`
	Deprecated          bool       `json:"deprecated,omitempty"`
	Preselect           bool       `json:"preselect,omitempty"`
	CommitCharacters    []string   `json:"commitCharacters,omitempty"`
	AdditionalTextEdits []textEdit `json:"additionalTextEdits,omitempty"`
}

type documentSymbol struct {
	Name           string           `json:"name"`
	Detail         string           `json:"detail,omitempty"`
	Kind           int              `json:"kind"`
	Tags           []int            `json:"tags,omitempty"`
	Deprecated     bool             `json:"deprecated,omitempty"`
	Range          lspRange         `json:"range"`
	SelectionRange lspRange         `json:"selectionRange"`
	Children       []documentSymbol `json:"children,omitempty"`
}

type semanticTokens struct {
	Data []uint32 `json:"data"`
}

type signatureHelp struct {
	Signatures      []signatureInformation `json:"signatures"`
	ActiveSignature int                    `json:"activeSignature"`
	ActiveParameter int                    `json:"activeParameter"`
}

type signatureInformation struct {
	Label           string                 `json:"label"`
	Documentation   string                 `json:"documentation,omitempty"`
	Parameters      []parameterInformation `json:"parameters,omitempty"`
	ActiveParameter int                    `json:"activeParameter,omitempty"`
}

type parameterInformation struct {
	Label string `json:"label"`
}

type workspaceEdit struct {
	Changes         map[string][]textEdit `json:"changes,omitempty"`
	DocumentChanges []any                 `json:"documentChanges,omitempty"`
}

type codeAction struct {
	Title       string         `json:"title"`
	Kind        string         `json:"kind,omitempty"`
	Diagnostics []diagnostic   `json:"diagnostics,omitempty"`
	Edit        *workspaceEdit `json:"edit,omitempty"`
	IsPreferred bool           `json:"isPreferred,omitempty"`
}
