package runtime

import (
	"math"

	"lunex/internal/ast"
	"lunex/internal/jit"
)

type loopValueProfile struct {
	samples        uint64
	numericSamples uint64
	bits           uint64
	value          float64
	hasNumeric     bool
	constant       bool
}

func recordLoopValueProfiles(state *jitLoopState, env *Environment) {
	if state == nil || env == nil {
		return
	}
	if state.profiles == nil {
		state.profiles = make(map[string]*loopValueProfile)
	}
	for _, name := range env.AllNames() {
		value, ok := env.Get(name)
		if !ok || value == nil {
			continue
		}
		profile := state.profiles[name]
		if profile == nil {
			profile = &loopValueProfile{constant: true}
			state.profiles[name] = profile
		}
		if profile.samples < ^uint64(0) {
			profile.samples++
		}
		if value.Tag != TypeNumber {
			continue
		}
		bits := math.Float64bits(value.NumVal)
		if !profile.hasNumeric {
			profile.hasNumeric = true
			profile.bits = bits
			profile.value = value.NumVal
		} else if profile.bits != bits {
			profile.constant = false
		}
		if profile.numericSamples < ^uint64(0) {
			profile.numericSamples++
		}
	}
}

func stableLoopValues(state *jitLoopState) map[string]float64 {
	values := make(map[string]float64)
	if state == nil {
		return values
	}
	for name, profile := range state.profiles {
		if profile == nil || !profile.hasNumeric || !profile.constant || profile.numericSamples < 8 || profile.samples-profile.numericSamples > profile.samples/20 {
			continue
		}
		values[name] = profile.value
	}
	return values
}

func inlineFunctionsForEnvironment(env *Environment) map[string]jit.InlineFunction {
	functions := make(map[string]jit.InlineFunction)
	if env == nil {
		return functions
	}
	for _, name := range env.AllNames() {
		value, ok := env.Get(name)
		if !ok || value == nil || value.Tag != TypeFunction || value.FnVal == nil {
			continue
		}
		function := value.FnVal
		if function.Native != nil {
			continue
		}
		body, ok := function.Body.(*ast.Node)
		if !ok || body == nil {
			continue
		}
		slots := make([]int, len(function.Params))
		valid := true
		for index, parameter := range function.Params {
			if parameter.Rest || fnParamHasDefault(parameter.Default) || parameter.Destructure != nil || parameter.ResolvedSlot < 0 {
				valid = false
				break
			}
			slots[index] = parameter.ResolvedSlot
		}
		if !valid {
			continue
		}
		functions[name] = jit.InlineFunction{Name: name, Body: body, ParamSlots: slots, Identity: function}
	}
	return functions
}

func jitProgramGuardsMatch(program *jit.Program, env *Environment) bool {
	if program == nil || env == nil {
		return false
	}
	for _, guard := range program.InlineGuards {
		value := env.GetSlotAddr(guard.Hops, guard.Slot)
		if value == nil || value.Tag != TypeFunction || value.FnVal == nil {
			return false
		}
		expected, ok := guard.Expected.(*Function)
		if !ok || value.FnVal != expected {
			return false
		}
	}
	for _, guard := range program.SpecializationGuards {
		value := env.GetSlotAddr(guard.Hops, guard.Slot)
		if value == nil || value.Tag != TypeNumber || math.Float64bits(value.NumVal) != math.Float64bits(guard.Value) {
			return false
		}
	}
	return true
}

func (interp *Interpreter) loopJITFallback(node *ast.Node, program *jit.Program, env *Environment) *jit.Program {
	if interp == nil || node == nil || program == nil || env == nil || len(program.SpecializationGuards) == 0 && len(program.InlineGuards) == 0 {
		return nil
	}
	interp.jitMu.Lock()
	state := interp.jitLoops[node]
	var fallback *jit.Program
	if state != nil && state.current == program {
		fallback = state.tier1
	}
	interp.jitMu.Unlock()
	if fallback != nil && jitProgramGuardsMatch(fallback, env) {
		return fallback
	}
	generic, err := jit.CompileLoopSpecialized(node, 1, nil, nil)
	if err != nil {
		return nil
	}
	interp.jitMu.Lock()
	state = interp.jitLoops[node]
	if state != nil && state.current == program {
		state.tier1 = generic
		interp.jitMu.Unlock()
		return generic
	}
	interp.jitMu.Unlock()
	_ = generic.Close()
	return nil
}
