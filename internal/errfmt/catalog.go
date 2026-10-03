package errfmt

const (
	ENullDeref     uint16 = 4001
	EDivZero       uint16 = 4002
	EIndexOOB      uint16 = 4003
	EKeyNotFound   uint16 = 4004
	EStackOverflow uint16 = 4005
	EInvalidCast   uint16 = 4006
	EBadBytecode   uint16 = 4007
	EFileNotFound  uint16 = 4009
	EPermission    uint16 = 4010
	ENetwork       uint16 = 4011
	ETimeout       uint16 = 4012
	EAssertion     uint16 = 4013
	EUserPanic     uint16 = 4014
	EInvalidRegex  uint16 = 4015
	EIORead        uint16 = 7001
	EIOWrite       uint16 = 7002
	EBadBCFormat   uint16 = 7003
	EUnknown       uint16 = 9999
)

type entry struct{ note string }

var catalog = map[uint16]entry{
	ENullDeref:     {"Null dereference. Add a null check before accessing."},
	EDivZero:       {"Division by zero. Guard the divisor with an if-check."},
	EIndexOOB:      {"Array index out of bounds. Check length before indexing."},
	EKeyNotFound:   {"Key not found in object. Use keys(obj) to see what's available."},
	EStackOverflow: {"Stack overflow — likely infinite recursion. Add a base case."},
	EInvalidCast:   {"Type cast failed. Use typeof() to check the value first."},
	EBadBytecode:   {"Corrupted bytecode. Repack with 'lunex pack'."},
	EFileNotFound:  {"File not found. Check the path and working directory."},
	EPermission:    {"Permission denied. Check file/directory permissions."},
	ENetwork:       {"Network error. Check connectivity and try again."},
	ETimeout:       {"Operation timed out. Check for slow I/O or long loops."},
	EAssertion:     {"Assertion failed. Review the condition that triggered it."},
	EUserPanic:     {"Explicit panic. Read the panic message for context."},
	EInvalidRegex:  {"Invalid regex. Check the pattern for typos."},
	EIORead:        {"Read error. Check path, permissions, and disk space."},
	EIOWrite:       {"Write error. Check disk space and permissions."},
	EBadBCFormat:   {"Not a valid .nax file. Use 'lunex pack' to produce one."},
}

func Lookup(code uint16) string {
	if e, ok := catalog[code]; ok {
		return e.note
	}
	return ""
}
