package runtime

import (
	"fmt"
	"lunex/internal/ast"
	"lunex/internal/errfmt"
	"sync"
)

type watcher struct {
	subject *ast.Node
	body    *ast.Node
	env     *Environment
	last    *Value
	mu      sync.Mutex
	running bool
}

func (interp *Interpreter) clearWatchers() {
	interp.watchMu.Lock()
	interp.watchers = nil
	interp.watchMu.Unlock()
}

func (interp *Interpreter) execWatch(node *ast.Node, env *Environment) (*Value, error) {
	if node == nil || node.Subject == nil || node.Body == nil {
		return nil, interp.runtimeError(errfmt.KindRuntime, "E0117", "invalid watch declaration", node, nil)
	}
	if node.Subject.Type != ast.Identifier && node.Subject.Type != ast.MemberExpr {
		return nil, interp.runtimeError(errfmt.KindRuntime, "E0117", "watch target must be a variable or member expression", node.Subject, nil)
	}
	last, err := interp.evalExpr(node.Subject, env)
	if err != nil {
		return nil, err
	}
	MarkEscaped(env)
	w := &watcher{subject: node.Subject, body: node.Body, env: env, last: last}
	interp.watchMu.Lock()
	interp.watchers = append(interp.watchers, w)
	interp.watchMu.Unlock()
	return Undefined, nil
}

func (interp *Interpreter) notifyWatchers() error {
	interp.watchMu.Lock()
	watchers := append([]*watcher(nil), interp.watchers...)
	interp.watchMu.Unlock()

	for _, w := range watchers {
		if w == nil {
			continue
		}
		w.mu.Lock()
		if w.running {
			w.mu.Unlock()
			continue
		}
		w.mu.Unlock()

		current, err := interp.evalExpr(w.subject, w.env)
		if err != nil {
			return err
		}

		w.mu.Lock()
		if w.running || valuesSame(w.last, current) {
			w.mu.Unlock()
			continue
		}
		w.last = current
		w.running = true
		w.mu.Unlock()

		_, err = interp.execNode(w.body, w.env)
		if err == nil {
			current, err = interp.evalExpr(w.subject, w.env)
		}

		w.mu.Lock()
		if err == nil {
			w.last = current
		}
		w.running = false
		w.mu.Unlock()

		if err != nil {
			return interp.wrapWatchError(err, w.subject)
		}
	}
	return nil
}

func valuesSame(a, b *Value) bool {
	if a == nil {
		a = Undefined
	}
	if b == nil {
		b = Undefined
	}
	return a.StrictEquals(b)
}

func (interp *Interpreter) wrapWatchError(err error, subject *ast.Node) error {
	if le, ok := err.(*errfmt.LunexError); ok {
		if le.File == "" {
			le.File = interp.filename
		}
		if len(le.Lines) == 0 {
			le.Lines = interp.sourceLines
		}
		return le
	}
	return interp.runtimeError(errfmt.KindRuntime, "E0118", fmt.Sprintf("watch callback failed: %v", err), subject, nil)
}
