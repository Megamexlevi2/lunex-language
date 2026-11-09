package runtime

import (
	"lunex/internal/ast"
	"lunex/internal/jit"
)

const maxTrackedTypeFeedbackNodes = 65536
const maxTrackedTypeVariants = 8

type NodeTypeFeedback struct {
	samples        uint64
	numericSamples uint64
	variants       map[uint16]uint64
}

func (interp *Interpreter) recordBinaryTypeFeedback(node *ast.Node, left, right *Value) {
	if interp == nil || node == nil || left == nil || right == nil {
		return
	}
	interp.feedbackTicker.Add(1)
	interp.feedbackMu.Lock()
	if interp.typeFeedback == nil {
		interp.typeFeedback = make(map[*ast.Node]*NodeTypeFeedback)
	}
	feedback := interp.typeFeedback[node]
	if feedback == nil {
		if len(interp.typeFeedback) >= maxTrackedTypeFeedbackNodes {
			interp.feedbackMu.Unlock()
			return
		}
		feedback = &NodeTypeFeedback{variants: make(map[uint16]uint64, 4)}
		interp.typeFeedback[node] = feedback
	}
	if feedback.samples < ^uint64(0) {
		feedback.samples++
	}
	if left.Tag == TypeNumber && right.Tag == TypeNumber && feedback.numericSamples < ^uint64(0) {
		feedback.numericSamples++
	}
	key := uint16(left.Tag)<<8 | uint16(right.Tag)
	if _, exists := feedback.variants[key]; exists || len(feedback.variants) < maxTrackedTypeVariants {
		if feedback.variants[key] < ^uint64(0) {
			feedback.variants[key]++
		}
	}
	interp.feedbackMu.Unlock()
}

func (interp *Interpreter) stableNumericFeedback(program *jit.Program) bool {
	if interp == nil || program == nil {
		return false
	}
	interp.feedbackMu.RLock()
	defer interp.feedbackMu.RUnlock()
	for _, node := range program.Nodes {
		if !requiresNumericFeedback(node) {
			continue
		}
		feedback := interp.typeFeedback[node]
		if feedback == nil || feedback.samples == 0 || feedback.numericSamples < feedback.samples-feedback.samples/20 {
			return false
		}
	}
	return true
}

func requiresNumericFeedback(node *ast.Node) bool {
	if node == nil {
		return false
	}
	switch node.Type {
	case ast.BinaryExpr:
		switch node.Op {
		case "+", "-", "*", "/", "%", "**", "<", "<=", ">", ">=", "==", "!=":
			return true
		}
	case ast.AssignExpr:
		switch node.Op {
		case "+=", "-=", "*=", "/=":
			return true
		}
	}
	return false
}
