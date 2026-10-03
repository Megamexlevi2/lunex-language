package std

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"lunex/internal/runtime"
	shared "lunex/internal/std/shared"
	"math"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

var globalDBEngine = &dbEngine{databases: make(map[string]*lunexDB)}

type dbEngine struct {
	databases map[string]*lunexDB
	mu        sync.Mutex
}

func (e *dbEngine) open(name string) (*lunexDB, error) {
	if name == "" {
		name = "default"
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if db, ok := e.databases[name]; ok {
		return db, nil
	}
	store, err := openStore(name)
	if err != nil {
		return nil, err
	}
	db := &lunexDB{
		name:   name,
		store:  store,
		tables: make(map[string]*lunexTable),
	}
	e.databases[name] = db
	return db, nil
}

func (e *dbEngine) drop(name string) error {
	if name == "" {
		name = "default"
	}
	e.mu.Lock()
	delete(e.databases, name)
	e.mu.Unlock()
	return dropStoreFile(name)
}

func (e *dbEngine) close(db *lunexDB) error {
	if db == nil {
		return nil
	}
	e.mu.Lock()
	if current, ok := e.databases[db.name]; ok && current == db {
		delete(e.databases, db.name)
	}
	e.mu.Unlock()
	return db.store.close()
}

func (e *dbEngine) list() []string {
	names := listStoreFiles()
	if names == nil {
		return []string{}
	}
	sort.Strings(names)
	return names
}

type lunexDB struct {
	name   string
	store  *sqliteStore
	tables map[string]*lunexTable
	mu     sync.Mutex
}

func (db *lunexDB) table(name string) *lunexTable {
	db.mu.Lock()
	defer db.mu.Unlock()
	if t, ok := db.tables[name]; ok {
		return t
	}
	t := &lunexTable{
		name:    name,
		db:      db,
		handle:  genUUID(),
		schema:  make(map[string]*fieldDef),
		indexes: make(map[string]*tableIndex),
		loaded:  false,
	}
	db.tables[name] = t
	return t
}

func (db *lunexDB) tableNames() []string { return db.tableNamesTx(nil) }

func (db *lunexDB) tableNamesTx(tx *sql.Tx) []string {
	names, err := db.store.listTablesTx(tx)
	if err != nil {
		return nil
	}
	sort.Strings(names)
	return names
}

type lunexTable struct {
	name    string
	db      *lunexDB
	tx      *sql.Tx
	handle  string
	rows    []dbRow
	loaded  bool
	schema  map[string]*fieldDef
	indexes map[string]*tableIndex
	watches []*tableWatch
	mu      sync.RWMutex
}

type dbRow struct {
	id  string
	doc map[string]*runtime.Value
}

type fieldDef struct {
	Type       string
	Required   bool
	Unique     bool
	DefaultVal *runtime.Value
	DefaultFn  string
	Min        float64
	Max        float64
	MinLen     int
	MaxLen     int
	MaxLenSet  bool
	Enum       []string
	Index      bool
	Primary    bool
	OnUpdate   string
	Ref        string
}

type tableIndex struct {
	fields []string
	unique bool
}

type tableWatch struct {
	id     string
	filter *runtime.Value
	fn     *runtime.Value
}

func genUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%s-%s-%s-%s-%s",
		hex.EncodeToString(b[0:4]),
		hex.EncodeToString(b[4:6]),
		hex.EncodeToString(b[6:8]),
		hex.EncodeToString(b[8:10]),
		hex.EncodeToString(b[10:]))
}

func (t *lunexTable) ensureLoaded() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if err := t.db.store.ensureTableTx(t.tx, t.name); err != nil {
		return fmt.Errorf("db: could not create table '%s': %w", t.name, err)
	}
	if t.loaded {
		return nil
	}
	if schemaJSON, ok, err := t.db.store.getMetaTx(t.tx, "schema:"+t.name); err != nil {
		return fmt.Errorf("db: could not load schema for '%s': %w", t.name, err)
	} else if ok {
		schema, err := decodeSchema(schemaJSON)
		if err != nil {
			return fmt.Errorf("db: invalid persisted schema for '%s': %w", t.name, err)
		}
		t.schema = schema
	}
	if indexJSON, ok, err := t.db.store.getMetaTx(t.tx, "indexes:"+t.name); err != nil {
		return fmt.Errorf("db: could not load indexes for '%s': %w", t.name, err)
	} else if ok {
		indexes, err := decodeIndexes(indexJSON)
		if err != nil {
			return fmt.Errorf("db: invalid persisted indexes for '%s': %w", t.name, err)
		}
		t.indexes = indexes
	}
	raw, err := t.db.store.loadAllTx(t.tx, t.name)
	if err != nil {
		return fmt.Errorf("db: could not load table '%s': %w", t.name, err)
	}
	rows := make([]dbRow, len(raw))
	for i, r := range raw {
		rows[i] = dbRow{id: r.ID, doc: docFromNative(r.Doc)}
	}
	t.rows = rows
	t.loaded = true
	return nil
}

type persistedFieldDef struct {
	Type      string      `json:"type"`
	Required  bool        `json:"required"`
	Unique    bool        `json:"unique"`
	Default   interface{} `json:"default,omitempty"`
	DefaultFn string      `json:"defaultFn,omitempty"`
	Min       float64     `json:"min,omitempty"`
	Max       float64     `json:"max,omitempty"`
	MinLen    int         `json:"minLength,omitempty"`
	MaxLen    int         `json:"maxLength,omitempty"`
	MaxLenSet bool        `json:"maxLengthSet,omitempty"`
	Enum      []string    `json:"enum,omitempty"`
	Index     bool        `json:"index,omitempty"`
	Primary   bool        `json:"primary,omitempty"`
	OnUpdate  string      `json:"onUpdate,omitempty"`
	Ref       string      `json:"ref,omitempty"`
}

type persistedIndex struct {
	Name   string   `json:"name"`
	Fields []string `json:"fields"`
	Unique bool     `json:"unique"`
}

func normalizeSchemaType(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	switch name {
	case "bool", "boolean":
		return "boolean"
	case "int", "integer", "float", "double":
		return "number"
	case "dict", "map":
		return "object"
	case "list":
		return "array"
	case "any", "string", "number", "date", "array", "object", "function":
		return name
	default:
		return name
	}
}

func validateSchemaType(name string) error {
	switch name {
	case "", "any", "string", "number", "boolean", "date", "array", "object", "function":
		return nil
	default:
		return fmt.Errorf("unsupported schema type %q", name)
	}
}

func encodeSchema(schema map[string]*fieldDef) (string, error) {
	out := make(map[string]persistedFieldDef, len(schema))
	for name, def := range schema {
		item := persistedFieldDef{
			Type: def.Type, Required: def.Required, Unique: def.Unique, DefaultFn: def.DefaultFn,
			Min: def.Min, Max: def.Max, MinLen: def.MinLen, MaxLen: def.MaxLen, MaxLenSet: def.MaxLenSet,
			Enum: append([]string(nil), def.Enum...), Index: def.Index, Primary: def.Primary, OnUpdate: def.OnUpdate, Ref: def.Ref,
		}
		if def.DefaultVal != nil {
			item.Default = valueToNative(def.DefaultVal)
		}
		out[name] = item
	}
	data, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func decodeSchema(raw string) (map[string]*fieldDef, error) {
	var stored map[string]persistedFieldDef
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		return nil, err
	}
	out := make(map[string]*fieldDef, len(stored))
	for name, item := range stored {
		item.Type = normalizeSchemaType(item.Type)
		if err := validateSchemaType(item.Type); err != nil {
			return nil, err
		}
		def := &fieldDef{
			Type: item.Type, Required: item.Required, Unique: item.Unique, DefaultFn: item.DefaultFn,
			Min: item.Min, Max: item.Max, MinLen: item.MinLen, MaxLen: item.MaxLen, MaxLenSet: item.MaxLenSet,
			Enum: append([]string(nil), item.Enum...), Index: item.Index, Primary: item.Primary, OnUpdate: item.OnUpdate, Ref: item.Ref,
		}
		if item.Default != nil {
			def.DefaultVal = nativeToValue(item.Default)
		}
		if name == "id" {
			def.Unique = true
			def.Primary = true
		}
		out[name] = def
	}
	return out, nil
}

func encodeIndexes(indexes map[string]*tableIndex) (string, error) {
	out := make([]persistedIndex, 0, len(indexes))
	for name, idx := range indexes {
		out = append(out, persistedIndex{Name: name, Fields: append([]string(nil), idx.fields...), Unique: idx.unique})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	data, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func decodeIndexes(raw string) (map[string]*tableIndex, error) {
	var stored []persistedIndex
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		return nil, err
	}
	out := make(map[string]*tableIndex, len(stored))
	for _, item := range stored {
		out[item.Name] = &tableIndex{fields: append([]string(nil), item.Fields...), unique: item.Unique}
	}
	return out, nil
}

func (t *lunexTable) primaryField() string {
	if _, ok := t.schema["id"]; ok {
		return "id"
	}
	for field, def := range t.schema {
		if def.Primary {
			return field
		}
	}
	return "_id"
}

func (t *lunexTable) publicRowID(row dbRow) *runtime.Value {
	if field := t.primaryField(); field != "_id" {
		if value, ok := row.doc[field]; ok && value != nil && !value.IsNullish() {
			return value
		}
	}
	return runtime.StringVal(row.id)
}
func (t *lunexTable) idFilter(id *runtime.Value) *runtime.Value {
	field := t.primaryField()
	return runtime.ObjectVal(map[string]*runtime.Value{field: id})
}

func (t *lunexTable) rowToValue(row dbRow) *runtime.Value {
	out := make(map[string]*runtime.Value, len(row.doc)+1)
	out["_id"] = t.publicRowID(row)
	for k, v := range row.doc {
		out[k] = v
	}
	return runtime.ObjectVal(out)
}

func rowJSON(row dbRow) ([]byte, error) {
	return json.Marshal(docToNative(row.doc))
}

func (t *lunexTable) applySchema(doc map[string]*runtime.Value, isInsert bool) (map[string]*runtime.Value, error) {
	out := make(map[string]*runtime.Value, len(doc))
	for key, value := range doc {
		if key == "_id" {
			continue
		}
		out[key] = value
	}
	for field, def := range t.schema {
		if field == "_id" {
			continue
		}
		value, exists := out[field]
		missing := !exists || value == nil || value.IsNullish()
		if missing && isInsert {
			switch def.DefaultFn {
			case "$uuid":
				value = runtime.StringVal(genUUID())
			case "$now":
				value = runtime.NumberVal(float64(time.Now().UnixNano() / int64(time.Millisecond)))
			case "$seq":
				seq, err := t.db.store.nextSeqTx(t.tx, t.name+"_"+field)
				if err != nil {
					return nil, fmt.Errorf("db: could not generate sequence for '%s': %w", field, err)
				}
				value = runtime.NumberVal(float64(seq))
			default:
				if def.DefaultVal != nil {
					value = shared.DeepCopy(def.DefaultVal)
				} else if def.Required {
					return nil, fmt.Errorf("field '%s' is required", field)
				}
			}
			if value != nil {
				out[field] = value
			}
			missing = value == nil || value.IsNullish()
		}
		if missing {
			if isInsert && def.Required {
				return nil, fmt.Errorf("field '%s' is required", field)
			}
			continue
		}
		if !isInsert && def.OnUpdate == "$now" {
			value = runtime.NumberVal(float64(time.Now().UnixNano() / int64(time.Millisecond)))
			out[field] = value
		}
		if err := validateField(field, value, def); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func validateField(name string, val *runtime.Value, def *fieldDef) error {
	if val == nil || val.IsNullish() {
		if def.Required {
			return fmt.Errorf("field '%s' is required", name)
		}
		return nil
	}
	if def.Type != "" && def.Type != "any" {
		typeName := shared.GetTypeName(val)
		if def.Type == "date" {
			if typeName != "number" && typeName != "string" {
				return fmt.Errorf("field '%s' must be date-compatible, got %s", name, typeName)
			}
		} else if typeName != def.Type {
			return fmt.Errorf("field '%s' must be %s, got %s", name, def.Type, typeName)
		}
	}
	if def.Type == "number" && val.Tag == runtime.TypeNumber {
		n := val.ToNumber()
		if def.Min != 0 && n < def.Min {
			return fmt.Errorf("field '%s' must be >= %v", name, def.Min)
		}
		if def.Max != 0 && n > def.Max {
			return fmt.Errorf("field '%s' must be <= %v", name, def.Max)
		}
	}
	if def.Type == "string" && val.Tag == runtime.TypeString {
		length := len([]rune(val.ToString()))
		if def.MinLen > 0 && length < def.MinLen {
			return fmt.Errorf("field '%s' must be at least %d characters", name, def.MinLen)
		}
		if def.MaxLenSet && length > def.MaxLen {
			return fmt.Errorf("field '%s' must be at most %d characters", name, def.MaxLen)
		}
	}
	if len(def.Enum) > 0 {
		value := val.ToString()
		found := false
		for _, item := range def.Enum {
			if item == value {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("field '%s' must be one of: %s", name, strings.Join(def.Enum, ", "))
		}
	}
	return nil
}

func matchFilter(doc map[string]*runtime.Value, filter *runtime.Value) bool {
	if filter == nil || filter.IsNullish() {
		return true
	}
	if filter.Tag != runtime.TypeObject {
		return false
	}
	for key, cond := range filter.ObjVal {
		switch key {
		case "$and":
			if cond.Tag != runtime.TypeArray {
				return false
			}
			for _, f := range cond.ArrVal {
				if !matchFilter(doc, f) {
					return false
				}
			}
		case "$or":
			if cond.Tag != runtime.TypeArray {
				return false
			}
			any := false
			for _, f := range cond.ArrVal {
				if matchFilter(doc, f) {
					any = true
					break
				}
			}
			if !any {
				return false
			}
		case "$not":
			if matchFilter(doc, cond) {
				return false
			}
		case "$nor":
			if cond.Tag != runtime.TypeArray {
				return false
			}
			for _, f := range cond.ArrVal {
				if matchFilter(doc, f) {
					return false
				}
			}
		default:
			fieldVal := doc[key]
			if fieldVal == nil {
				fieldVal = runtime.Undefined
			}

			if cond != nil && cond.Tag == runtime.TypeObject {
				for op, opVal := range cond.ObjVal {
					if !matchOp(fieldVal, op, opVal) {
						return false
					}
				}
			} else {
				if !fieldVal.StrictEquals(cond) {
					return false
				}
			}
		}
	}
	return true
}

func (t *lunexTable) matchRowFilter(row dbRow, filter *runtime.Value) bool {
	if filter == nil || filter.IsNullish() {
		return true
	}
	if filter.Tag != runtime.TypeObject {
		return false
	}
	doc := make(map[string]*runtime.Value, len(row.doc)+1)
	doc["_id"] = t.publicRowID(row)
	for k, v := range row.doc {
		doc[k] = v
	}
	return matchFilter(doc, filter)
}

func matchOp(field *runtime.Value, op string, val *runtime.Value) bool {
	if field == nil {
		field = runtime.Undefined
	}
	switch op {
	case "$eq":
		return field.StrictEquals(val)
	case "$ne":
		return !field.StrictEquals(val)
	case "$gt":
		return field.Tag == runtime.TypeNumber && val != nil && val.Tag == runtime.TypeNumber && field.ToNumber() > val.ToNumber()
	case "$gte":
		return field.Tag == runtime.TypeNumber && val != nil && val.Tag == runtime.TypeNumber && field.ToNumber() >= val.ToNumber()
	case "$lt":
		return field.Tag == runtime.TypeNumber && val != nil && val.Tag == runtime.TypeNumber && field.ToNumber() < val.ToNumber()
	case "$lte":
		return field.Tag == runtime.TypeNumber && val != nil && val.Tag == runtime.TypeNumber && field.ToNumber() <= val.ToNumber()
	case "$in":
		if val == nil || val.Tag != runtime.TypeArray {
			return false
		}
		for _, item := range val.ArrVal {
			if field.StrictEquals(item) {
				return true
			}
		}
		return false
	case "$nin":
		if val == nil || val.Tag != runtime.TypeArray {
			return true
		}
		for _, item := range val.ArrVal {
			if field.StrictEquals(item) {
				return false
			}
		}
		return true
	case "$like":
		return matchLike(field.ToString(), val.ToString(), false)
	case "$ilike":
		return matchLike(field.ToString(), val.ToString(), true)
	case "$regex":
		re, err := regexp.Compile(val.ToString())
		if err != nil {
			return false
		}
		return re.MatchString(field.ToString())
	case "$exists":
		exists := field.Tag != runtime.TypeUndefined && field.Tag != runtime.TypeNull
		if val != nil && val.Tag == runtime.TypeBool {
			return exists == val.BoolVal
		}
		return exists
	case "$between":
		if field.Tag != runtime.TypeNumber || val == nil || val.Tag != runtime.TypeArray || len(val.ArrVal) < 2 {
			return false
		}
		if val.ArrVal[0] == nil || val.ArrVal[0].Tag != runtime.TypeNumber || val.ArrVal[1] == nil || val.ArrVal[1].Tag != runtime.TypeNumber {
			return false
		}
		n := field.ToNumber()
		return n >= val.ArrVal[0].ToNumber() && n <= val.ArrVal[1].ToNumber()
	case "$contains":
		if field.Tag == runtime.TypeArray {
			for _, item := range field.ArrVal {
				if item.StrictEquals(val) {
					return true
				}
			}
			return false
		}
		return strings.Contains(field.ToString(), val.ToString())
	case "$size":
		if field.Tag == runtime.TypeArray {
			return float64(len(field.ArrVal)) == val.ToNumber()
		}
		if field.Tag == runtime.TypeString {
			return float64(len(field.StrVal)) == val.ToNumber()
		}
		return false
	case "$type":
		return shared.GetTypeName(field) == val.ToString()
	case "$startsWith":
		return strings.HasPrefix(field.ToString(), val.ToString())
	case "$endsWith":
		return strings.HasSuffix(field.ToString(), val.ToString())
	default:
		return false
	}
}

func matchLike(s, pattern string, caseInsensitive bool) bool {
	if caseInsensitive {
		s = strings.ToLower(s)
		pattern = strings.ToLower(pattern)
	}
	re := "^"
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '%':
			re += ".*"
		case '_':
			re += "."
		case '.', '+', '*', '?', '(', ')', '[', ']', '{', '}', '^', '$', '|', '\\':
			re += "\\" + string(pattern[i])
		default:
			re += string(pattern[i])
		}
	}
	re += "$"
	matched, _ := regexp.MatchString(re, s)
	return matched
}

func (t *lunexTable) projectRow(row dbRow, fields []string) *runtime.Value {
	if len(fields) == 0 {
		return t.rowToValue(row)
	}
	out := make(map[string]*runtime.Value, len(fields))
	src := make(map[string]*runtime.Value, len(row.doc)+1)
	src["_id"] = t.publicRowID(row)
	for k, v := range row.doc {
		src[k] = v
	}
	for _, f := range fields {
		if v, ok := src[f]; ok {
			out[f] = v
		}
	}
	return runtime.ObjectVal(out)
}

func (t *lunexTable) execQuery(filter *runtime.Value, proj []string, sorts []sortEntry, limitN, offsetN int) []*runtime.Value {
	if err := t.ensureLoaded(); err != nil {
		return nil
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	var matched []dbRow
	for _, row := range t.rows {
		if t.matchRowFilter(row, filter) {
			matched = append(matched, row)
		}
	}
	if len(sorts) > 0 {
		sort.SliceStable(matched, func(i, j int) bool {
			for _, s := range sorts {
				a := matched[i].doc[s.field]
				b := matched[j].doc[s.field]
				if s.field == "_id" {
					a = t.publicRowID(matched[i])
					b = t.publicRowID(matched[j])
				}
				if a == nil {
					a = runtime.Undefined
				}
				if b == nil {
					b = runtime.Undefined
				}
				var cmp int
				if a.Tag == runtime.TypeNumber && b.Tag == runtime.TypeNumber {
					if a.ToNumber() < b.ToNumber() {
						cmp = -1
					} else if a.ToNumber() > b.ToNumber() {
						cmp = 1
					}
				} else {
					cmp = strings.Compare(a.ToString(), b.ToString())
				}
				if cmp != 0 {
					if s.desc {
						return cmp > 0
					}
					return cmp < 0
				}
			}
			return false
		})
	}
	if offsetN > 0 {
		if offsetN >= len(matched) {
			return nil
		}
		matched = matched[offsetN:]
	}
	if limitN > 0 && limitN < len(matched) {
		matched = matched[:limitN]
	}
	out := make([]*runtime.Value, len(matched))
	for i, row := range matched {
		out[i] = t.projectRow(row, proj)
	}
	return out
}

func (t *lunexTable) insertDoc(doc map[string]*runtime.Value) (*runtime.Value, error) {
	if err := t.ensureLoaded(); err != nil {
		return nil, err
	}
	resultDoc, err := t.applySchema(doc, true)
	if err != nil {
		return nil, err
	}
	if t.primaryField() == "id" {
		if value, ok := resultDoc["id"]; !ok || value == nil || value.IsNullish() {
			resultDoc["id"] = runtime.StringVal(genUUID())
		}
	}
	if err := t.validateDocument(resultDoc); err != nil {
		return nil, err
	}
	internalID := genUUID()
	t.mu.Lock()
	if err := t.checkUniqueLocked(internalID, resultDoc); err != nil {
		t.mu.Unlock()
		return nil, err
	}
	row := dbRow{id: internalID, doc: resultDoc}
	payload, err := rowJSON(row)
	if err != nil {
		t.mu.Unlock()
		return nil, fmt.Errorf("db: could not serialize document: %w", err)
	}
	if err := t.db.store.insertRowTx(t.tx, t.name, internalID, payload); err != nil {
		t.mu.Unlock()
		return nil, fmt.Errorf("db: insert failed: %w", err)
	}
	t.rows = append(t.rows, row)
	result := t.rowToValue(row)
	t.mu.Unlock()
	if t.tx == nil {
		t.notifyWatches("insert", row)
	}
	return result, nil
}
func validateDocumentWithSchema(doc map[string]*runtime.Value, schema map[string]*fieldDef) error {
	for field, def := range schema {
		value, ok := doc[field]
		if !ok || value == nil || value.IsNullish() {
			if def.Required {
				return fmt.Errorf("field '%s' is required", field)
			}
			continue
		}
		if err := validateField(field, value, def); err != nil {
			return err
		}
	}
	return nil
}

func (t *lunexTable) validateDocument(doc map[string]*runtime.Value) error {
	return validateDocumentWithSchema(doc, t.schema)
}

func (t *lunexTable) checkUniqueLocked(excludeID string, doc map[string]*runtime.Value) error {
	for field, def := range t.schema {
		if !def.Unique && !def.Primary {
			continue
		}
		value, ok := doc[field]
		if !ok || value == nil || value.IsNullish() {
			continue
		}
		for _, row := range t.rows {
			if row.id == excludeID {
				continue
			}
			if existing, ok := row.doc[field]; ok && existing.StrictEquals(value) {
				return fmt.Errorf("field '%s' must be unique, value already exists", field)
			}
		}
	}
	return nil
}

func (t *lunexTable) checkUnique(excludeID string, doc map[string]*runtime.Value, isUpdate bool) error {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.checkUniqueLocked(excludeID, doc)
}

func (t *lunexTable) applyChanges(doc map[string]*runtime.Value, changes map[string]*runtime.Value) (map[string]*runtime.Value, error) {
	out := make(map[string]*runtime.Value, len(doc)+len(changes))
	for key, value := range doc {
		out[key] = value
	}
	operatorMode := false
	if set, ok := changes["$set"]; ok {
		operatorMode = true
		if set == nil || set.Tag != runtime.TypeObject {
			return nil, fmt.Errorf("$set must be an object")
		}
		for key, value := range set.ObjVal {
			if key == "_id" {
				return nil, fmt.Errorf("field '_id' is reserved")
			}
			out[key] = value
		}
	}
	if unset, ok := changes["$unset"]; ok {
		operatorMode = true
		if unset == nil || unset.Tag != runtime.TypeObject {
			return nil, fmt.Errorf("$unset must be an object")
		}
		for key := range unset.ObjVal {
			if key == "_id" {
				return nil, fmt.Errorf("field '_id' is reserved")
			}
			delete(out, key)
		}
	}
	if inc, ok := changes["$inc"]; ok {
		operatorMode = true
		if inc == nil || inc.Tag != runtime.TypeObject {
			return nil, fmt.Errorf("$inc must be an object")
		}
		for key, amount := range inc.ObjVal {
			if key == "_id" {
				return nil, fmt.Errorf("field '_id' is reserved")
			}
			current := out[key]
			if current == nil || current.IsNullish() {
				current = runtime.NumberVal(0)
			}
			if current.Tag != runtime.TypeNumber || amount == nil || amount.Tag != runtime.TypeNumber {
				return nil, fmt.Errorf("$inc requires numeric values for field '%s'", key)
			}
			out[key] = runtime.NumberVal(current.ToNumber() + amount.ToNumber())
		}
	}
	if push, ok := changes["$push"]; ok {
		operatorMode = true
		if push == nil || push.Tag != runtime.TypeObject {
			return nil, fmt.Errorf("$push must be an object")
		}
		for key, value := range push.ObjVal {
			if key == "_id" {
				return nil, fmt.Errorf("field '_id' is reserved")
			}
			current := out[key]
			if current == nil || current.IsNullish() {
				current = runtime.ArrayVal(nil)
			}
			if current.Tag != runtime.TypeArray {
				return nil, fmt.Errorf("$push requires an array field '%s'", key)
			}
			items := append([]*runtime.Value(nil), current.ArrVal...)
			items = append(items, value)
			out[key] = runtime.ArrayVal(items)
		}
	}
	if operatorMode {
		for key := range changes {
			if strings.HasPrefix(key, "$") && key != "$set" && key != "$unset" && key != "$inc" && key != "$push" {
				return nil, fmt.Errorf("unsupported update operator '%s'", key)
			}
		}
	} else {
		for key, value := range changes {
			if key == "_id" {
				return nil, fmt.Errorf("field '_id' is reserved")
			}
			out[key] = value
		}
	}
	return out, nil
}

func (t *lunexTable) updateRows(filter *runtime.Value, changes map[string]*runtime.Value, onlyFirst bool) (int, error) {
	if err := t.ensureLoaded(); err != nil {
		return 0, err
	}
	if changes == nil {
		return 0, nil
	}
	changed, err := t.updateRowsLocked(filter, changes, onlyFirst)
	if err != nil {
		return 0, err
	}
	if t.tx == nil {
		for _, row := range changed {
			t.notifyWatches("update", row)
		}
	}
	return len(changed), nil
}

func (t *lunexTable) updateRowsLocked(filter *runtime.Value, changes map[string]*runtime.Value, onlyFirst bool) ([]dbRow, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	var updates []struct {
		index int
		row   dbRow
	}
	for i, row := range t.rows {
		if !t.matchRowFilter(row, filter) {
			continue
		}
		updatedDoc, err := t.applyChanges(row.doc, changes)
		if err != nil {
			return nil, err
		}
		updatedDoc, err = t.applySchema(updatedDoc, false)
		if err != nil {
			return nil, err
		}
		if err := t.validateDocument(updatedDoc); err != nil {
			return nil, err
		}
		updates = append(updates, struct {
			index int
			row   dbRow
		}{i, dbRow{id: row.id, doc: updatedDoc}})
		if onlyFirst {
			break
		}
	}
	if len(updates) == 0 {
		return nil, nil
	}
	var tx *sql.Tx
	var err error
	if t.tx == nil {
		tx, err = t.db.store.begin()
		if err != nil {
			return nil, fmt.Errorf("db: could not begin update transaction: %w", err)
		}
	}
	rollback := func(e error) ([]dbRow, error) {
		if tx != nil {
			_ = tx.Rollback()
			t.db.store.finishTransaction()
		}
		return nil, e
	}
	for _, item := range updates {
		payload, marshalErr := rowJSON(item.row)
		if marshalErr != nil {
			return rollback(fmt.Errorf("db: could not serialize updated document: %w", marshalErr))
		}
		storeTx := t.tx
		if storeTx == nil {
			storeTx = tx
		}
		if err := t.db.store.updateRowTx(storeTx, t.name, item.row.id, payload); err != nil {
			return rollback(fmt.Errorf("db: update failed: %w", err))
		}
	}
	if tx != nil {
		if err := tx.Commit(); err != nil {
			t.db.store.finishTransaction()
			return nil, fmt.Errorf("db: could not commit update transaction: %w", err)
		}
		t.db.store.finishTransaction()
	}
	changed := make([]dbRow, len(updates))
	for i, item := range updates {
		t.rows[item.index] = item.row
		changed[i] = item.row
	}
	return changed, nil
}

func (t *lunexTable) deleteRows(filter *runtime.Value, onlyFirst bool) (int, error) {
	if err := t.ensureLoaded(); err != nil {
		return 0, err
	}
	removed, err := t.deleteRowsLocked(filter, onlyFirst)
	if err != nil {
		return 0, err
	}
	if t.tx == nil {
		for _, row := range removed {
			t.notifyWatches("delete", row)
		}
	}
	return len(removed), nil
}

func (t *lunexTable) deleteRowsLocked(filter *runtime.Value, onlyFirst bool) ([]dbRow, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	var removed []dbRow
	kept := make([]dbRow, 0, len(t.rows))
	matched := false
	for _, row := range t.rows {
		if t.matchRowFilter(row, filter) && (!onlyFirst || !matched) {
			removed = append(removed, row)
			matched = true
			continue
		}
		kept = append(kept, row)
	}
	if len(removed) == 0 {
		return nil, nil
	}
	var tx *sql.Tx
	var err error
	if t.tx == nil {
		tx, err = t.db.store.begin()
		if err != nil {
			return nil, fmt.Errorf("db: could not begin delete transaction: %w", err)
		}
	}
	rollback := func(e error) ([]dbRow, error) {
		if tx != nil {
			_ = tx.Rollback()
			t.db.store.finishTransaction()
		}
		return nil, e
	}
	for _, row := range removed {
		storeTx := t.tx
		if storeTx == nil {
			storeTx = tx
		}
		if err := t.db.store.deleteRowTx(storeTx, t.name, row.id); err != nil {
			return rollback(fmt.Errorf("db: delete failed: %w", err))
		}
	}
	if tx != nil {
		if err := tx.Commit(); err != nil {
			t.db.store.finishTransaction()
			return nil, fmt.Errorf("db: could not commit delete transaction: %w", err)
		}
		t.db.store.finishTransaction()
	}
	t.rows = kept
	return removed, nil
}

func (t *lunexTable) notifyWatches(event string, row dbRow) {
	t.mu.RLock()
	watches := make([]*tableWatch, len(t.watches))
	copy(watches, t.watches)
	t.mu.RUnlock()
	if len(watches) == 0 {
		return
	}
	for _, w := range watches {
		if t.matchRowFilter(row, w.filter) {
			if runtime.CallFunction != nil {
				runtime.CallFunction(w.fn, []*runtime.Value{
					runtime.ObjectVal(map[string]*runtime.Value{
						"type": runtime.StringVal(event),
						"doc":  t.rowToValue(row),
					}),
				})
			}
		}
	}
}

func aggregationGroupValue(row *runtime.Value, expression *runtime.Value) *runtime.Value {
	if expression == nil || expression.IsNullish() {
		return runtime.Null
	}
	if expression.Tag == runtime.TypeString && strings.HasPrefix(expression.StrVal, "$") {
		field := strings.TrimPrefix(expression.StrVal, "$")
		if value, ok := row.ObjVal[field]; ok {
			return value
		}
		return runtime.Null
	}
	return expression
}

func aggregationValueKey(value *runtime.Value) string {
	if value == nil {
		return "null:null"
	}
	payload, _ := json.Marshal(valueToNative(value))
	return shared.GetTypeName(value) + ":" + string(payload)
}

func (t *lunexTable) aggregate(pipeline *runtime.Value) []*runtime.Value {
	if err := t.ensureLoaded(); err != nil {
		return nil
	}
	t.mu.RLock()
	current := make([]dbRow, len(t.rows))
	copy(current, t.rows)
	t.mu.RUnlock()
	if pipeline == nil || pipeline.Tag != runtime.TypeArray {
		return nil
	}
	var rows []*runtime.Value
	for _, r := range current {
		rows = append(rows, t.rowToValue(r))
	}
	for _, stage := range pipeline.ArrVal {
		if stage == nil || stage.Tag != runtime.TypeObject {
			continue
		}
		for op, conf := range stage.ObjVal {
			switch op {
			case "$match":
				var next []*runtime.Value
				for _, r := range rows {
					if matchFilter(r.ObjVal, conf) {
						next = append(next, r)
					}
				}
				rows = next
			case "$sort":
				if conf == nil || conf.Tag != runtime.TypeObject {
					break
				}
				type sf struct {
					field string
					dir   float64
				}
				var fields []sf
				for f, d := range conf.ObjVal {
					fields = append(fields, sf{f, d.ToNumber()})
				}
				sort.SliceStable(rows, func(i, j int) bool {
					for _, s := range fields {
						a := rows[i].ObjVal[s.field]
						b := rows[j].ObjVal[s.field]
						if a == nil {
							a = runtime.Undefined
						}
						if b == nil {
							b = runtime.Undefined
						}
						var cmp int
						if a.Tag == runtime.TypeNumber && b.Tag == runtime.TypeNumber {
							if a.ToNumber() < b.ToNumber() {
								cmp = -1
							} else if a.ToNumber() > b.ToNumber() {
								cmp = 1
							}
						} else {
							cmp = strings.Compare(a.ToString(), b.ToString())
						}
						if cmp != 0 {
							if s.dir < 0 {
								return cmp > 0
							}
							return cmp < 0
						}
					}
					return false
				})
			case "$limit":
				n := int(conf.ToNumber())
				if n < 0 {
					return nil
				}
				if n == 0 {
					rows = nil
				} else if n < len(rows) {
					rows = rows[:n]
				}
			case "$skip":
				n := int(conf.ToNumber())
				if n > 0 {
					if n >= len(rows) {
						rows = nil
					} else {
						rows = rows[n:]
					}
				}
			case "$project":
				if conf == nil || conf.Tag != runtime.TypeObject {
					break
				}
				var include []string
				for f, v := range conf.ObjVal {
					if v == nil {
						continue
					}
					includeField := false
					if v.Tag == runtime.TypeBool {
						includeField = v.BoolVal
					} else {
						includeField = v.ToNumber() != 0
					}
					if includeField {
						include = append(include, f)
					}
				}
				if len(include) > 0 {
					for idx, r := range rows {
						out := make(map[string]*runtime.Value, len(include))
						for _, f := range include {
							if v, ok := r.ObjVal[f]; ok {
								out[f] = v
							}
						}
						rows[idx] = runtime.ObjectVal(out)
					}
				}
			case "$group":
				if conf == nil || conf.Tag != runtime.TypeObject {
					break
				}
				idExpr, hasID := conf.ObjVal["_id"]
				byExpr, hasBy := conf.ObjVal["by"]
				groups := make(map[string][]*runtime.Value)
				groupValues := make(map[string]*runtime.Value)
				groupOrder := []string{}
				for _, r := range rows {
					var keyValue *runtime.Value
					if hasID {
						keyValue = aggregationGroupValue(r, idExpr)
					} else if hasBy {
						keyValue = aggregationGroupValue(r, byExpr)
					}
					key := aggregationValueKey(keyValue)
					if _, exists := groups[key]; !exists {
						groupOrder = append(groupOrder, key)
						groupValues[key] = keyValue
					}
					groups[key] = append(groups[key], r)
				}
				var next []*runtime.Value
				for _, key := range groupOrder {
					grp := groups[key]
					out := make(map[string]*runtime.Value)
					if hasID {
						out["_id"] = groupValues[key]
					} else if hasBy {
						byName := strings.TrimPrefix(byExpr.ToString(), "$")
						if byName != "" {
							out[byName] = groupValues[key]
						}
					}
					for aggField, aggDef := range conf.ObjVal {
						if aggField == "_id" || aggField == "by" {
							continue
						}
						if aggDef == nil {
							continue
						}
						if aggDef.Tag == runtime.TypeBool && aggDef.BoolVal {
							out[aggField] = runtime.NumberVal(float64(len(grp)))
							continue
						}
						if aggDef.Tag != runtime.TypeObject {
							continue
						}
						for aggOp, aggTarget := range aggDef.ObjVal {
							if aggOp == "$count" {
								out[aggField] = runtime.NumberVal(float64(len(grp)))
								continue
							}
							targetField := strings.TrimPrefix(aggTarget.ToString(), "$")
							switch aggOp {
							case "$sum":
								sum := 0.0
								for _, r := range grp {
									if v, ok := r.ObjVal[targetField]; ok && v.Tag == runtime.TypeNumber {
										sum += v.ToNumber()
									}
								}
								out[aggField] = runtime.NumberVal(sum)
							case "$avg":
								sum, n := 0.0, 0
								for _, r := range grp {
									if v, ok := r.ObjVal[targetField]; ok && v.Tag == runtime.TypeNumber {
										sum += v.ToNumber()
										n++
									}
								}
								if n == 0 {
									out[aggField] = runtime.NumberVal(0)
								} else {
									out[aggField] = runtime.NumberVal(sum / float64(n))
								}
							case "$min":
								min := math.MaxFloat64
								for _, r := range grp {
									if v, ok := r.ObjVal[targetField]; ok && v.Tag == runtime.TypeNumber && v.ToNumber() < min {
										min = v.ToNumber()
									}
								}
								if min == math.MaxFloat64 {
									out[aggField] = runtime.Null
								} else {
									out[aggField] = runtime.NumberVal(min)
								}
							case "$max":
								max := -math.MaxFloat64
								for _, r := range grp {
									if v, ok := r.ObjVal[targetField]; ok && v.Tag == runtime.TypeNumber && v.ToNumber() > max {
										max = v.ToNumber()
									}
								}
								if max == -math.MaxFloat64 {
									out[aggField] = runtime.Null
								} else {
									out[aggField] = runtime.NumberVal(max)
								}
							case "$first":
								if len(grp) > 0 {
									if v, ok := grp[0].ObjVal[targetField]; ok {
										out[aggField] = v
									}
								}
							case "$last":
								if len(grp) > 0 {
									if v, ok := grp[len(grp)-1].ObjVal[targetField]; ok {
										out[aggField] = v
									}
								}
							case "$push", "$addToSet":
								seen := make(map[string]bool)
								arr := make([]*runtime.Value, 0, len(grp))
								for _, r := range grp {
									v, ok := r.ObjVal[targetField]
									if !ok {
										continue
									}
									if aggOp == "$addToSet" {
										key := aggregationValueKey(v)
										if seen[key] {
											continue
										}
										seen[key] = true
									}
									arr = append(arr, v)
								}
								out[aggField] = runtime.ArrayVal(arr)
							}
						}
					}
					next = append(next, runtime.ObjectVal(out))
				}
				rows = next
			case "$unwind":
				field := conf.ToString()
				var next []*runtime.Value
				for _, r := range rows {
					arr, ok := r.ObjVal[field]
					if !ok || arr.Tag != runtime.TypeArray {
						next = append(next, r)
						continue
					}
					for _, item := range arr.ArrVal {
						clone := make(map[string]*runtime.Value, len(r.ObjVal))
						for k, v := range r.ObjVal {
							clone[k] = v
						}
						clone[field] = item
						next = append(next, runtime.ObjectVal(clone))
					}
				}
				rows = next
			case "$count":
				name := conf.ToString()
				if name == "" {
					name = "count"
				}
				rows = []*runtime.Value{runtime.ObjectVal(map[string]*runtime.Value{
					name: runtime.NumberVal(float64(len(rows))),
				})}
			}
		}
	}
	return rows
}

func (t *lunexTable) search(text string, fields []string) []*runtime.Value {
	if err := t.ensureLoaded(); err != nil {
		return nil
	}
	text = strings.ToLower(text)
	t.mu.RLock()
	defer t.mu.RUnlock()
	var out []*runtime.Value
	for _, row := range t.rows {
		searchFields := fields
		if len(searchFields) == 0 {
			for k := range row.doc {
				searchFields = append(searchFields, k)
			}
		}
		for _, f := range searchFields {
			if v, ok := row.doc[f]; ok {
				if strings.Contains(strings.ToLower(v.ToString()), text) {
					out = append(out, t.rowToValue(row))
					break
				}
			}
		}
	}
	return out
}

type sortEntry struct {
	field string
	desc  bool
}

func cloneTableForTx(t *lunexTable, tx *sql.Tx) *lunexTable {
	t.mu.RLock()
	defer t.mu.RUnlock()
	schema := make(map[string]*fieldDef, len(t.schema))
	for name, def := range t.schema {
		copyDef := *def
		copyDef.Enum = append([]string(nil), def.Enum...)
		if def.DefaultVal != nil {
			copyDef.DefaultVal = shared.DeepCopy(def.DefaultVal)
		}
		schema[name] = &copyDef
	}
	indexes := make(map[string]*tableIndex, len(t.indexes))
	for name, idx := range t.indexes {
		indexes[name] = &tableIndex{fields: append([]string(nil), idx.fields...), unique: idx.unique}
	}
	return &lunexTable{name: t.name, db: t.db, tx: tx, handle: genUUID(), schema: schema, indexes: indexes}
}

func (db *lunexDB) txTable(name string, tx *sql.Tx) *lunexTable {
	return cloneTableForTx(db.table(name), tx)
}

func (db *lunexDB) invalidateTables() {
	db.mu.Lock()
	tables := make([]*lunexTable, 0, len(db.tables))
	for _, t := range db.tables {
		tables = append(tables, t)
	}
	db.mu.Unlock()
	for _, t := range tables {
		t.mu.Lock()
		t.rows = nil
		t.loaded = false
		t.schema = make(map[string]*fieldDef)
		t.indexes = make(map[string]*tableIndex)
		t.mu.Unlock()
	}
}

var dbTableHandles sync.Map

func resolveTableHandle(value *runtime.Value) (*lunexTable, error) {
	if value == nil || value.Tag != runtime.TypeObject {
		return nil, fmt.Errorf("join: expected table object")
	}
	marker, ok := value.ObjVal["__lunex_table__"]
	if !ok || marker.Tag != runtime.TypeBool || !marker.BoolVal {
		return nil, fmt.Errorf("join: expected table object")
	}
	handle, ok := value.ObjVal["__lunex_handle"]
	if !ok || handle.Tag != runtime.TypeString {
		return nil, fmt.Errorf("join: invalid table handle")
	}
	valueRef, ok := dbTableHandles.Load(handle.StrVal)
	if !ok {
		return nil, fmt.Errorf("join: table handle is no longer available")
	}
	t, ok := valueRef.(*lunexTable)
	if !ok || t == nil {
		return nil, fmt.Errorf("join: invalid table handle")
	}
	return t, nil
}

func copyTableIndexes(indexes map[string]*tableIndex) map[string]*tableIndex {
	out := make(map[string]*tableIndex, len(indexes))
	for name, idx := range indexes {
		out[name] = &tableIndex{fields: append([]string(nil), idx.fields...), unique: idx.unique}
	}
	return out
}

func (t *lunexTable) persistSchemaDefinition(parsed map[string]*fieldDef) error {
	t.mu.RLock()
	rows := append([]dbRow(nil), t.rows...)
	oldSchema := make(map[string]*fieldDef, len(t.schema))
	for name, def := range t.schema {
		copyDef := *def
		copyDef.Enum = append([]string(nil), def.Enum...)
		if def.DefaultVal != nil {
			copyDef.DefaultVal = shared.DeepCopy(def.DefaultVal)
		}
		oldSchema[name] = &copyDef
	}
	indexes := copyTableIndexes(t.indexes)
	t.mu.RUnlock()
	for _, row := range rows {
		if err := validateDocumentWithSchema(row.doc, parsed); err != nil {
			return fmt.Errorf("schema validation failed for existing record: %w", err)
		}
	}
	var tx *sql.Tx
	started := false
	var err error
	if t.tx != nil {
		tx = t.tx
	} else {
		tx, err = t.db.store.begin()
		if err != nil {
			return fmt.Errorf("db: could not begin schema transaction: %w", err)
		}
		started = true
	}
	rollback := func(e error) error {
		if started {
			_ = tx.Rollback()
			t.db.store.finishTransaction()
		}
		return e
	}
	for name, idx := range indexes {
		if len(idx.fields) != 1 {
			continue
		}
		field := idx.fields[0]
		oldDef := oldSchema[field]
		newDef := parsed[field]
		oldManaged := oldDef != nil && (oldDef.Index || oldDef.Unique || oldDef.Primary)
		newManaged := newDef != nil && (newDef.Index || newDef.Unique || newDef.Primary)
		if oldManaged && !newManaged && name == field {
			if err := t.db.store.dropIndexTx(tx, t.name, name); err != nil {
				return rollback(fmt.Errorf("db: could not remove index '%s': %w", name, err))
			}
			delete(indexes, name)
		}
	}
	for field, def := range parsed {
		if !def.Index && !def.Unique && !def.Primary {
			continue
		}
		if err := t.db.store.createIndexTx(tx, t.name, field, []string{field}, def.Unique || def.Primary); err != nil {
			return rollback(fmt.Errorf("db: could not create index for '%s': %w", field, err))
		}
		indexes[field] = &tableIndex{fields: []string{field}, unique: def.Unique || def.Primary}
	}
	schemaJSON, err := encodeSchema(parsed)
	if err != nil {
		return rollback(fmt.Errorf("db: could not serialize schema: %w", err))
	}
	indexesJSON, err := encodeIndexes(indexes)
	if err != nil {
		return rollback(fmt.Errorf("db: could not serialize indexes: %w", err))
	}
	if err := t.db.store.setMetaTx(tx, "schema:"+t.name, schemaJSON); err != nil {
		return rollback(fmt.Errorf("db: could not persist schema: %w", err))
	}
	if err := t.db.store.setMetaTx(tx, "indexes:"+t.name, indexesJSON); err != nil {
		return rollback(fmt.Errorf("db: could not persist indexes: %w", err))
	}
	if started {
		if err := tx.Commit(); err != nil {
			t.db.store.finishTransaction()
			return fmt.Errorf("db: could not commit schema transaction: %w", err)
		}
		t.db.store.finishTransaction()
	}
	t.mu.Lock()
	t.schema = parsed
	t.indexes = indexes
	t.mu.Unlock()
	return nil
}

func (t *lunexTable) persistIndexDefinition(fields []string, unique bool) error {
	name := strings.Join(fields, "_")
	var tx *sql.Tx
	started := false
	var err error
	if t.tx != nil {
		tx = t.tx
	} else {
		tx, err = t.db.store.begin()
		if err != nil {
			return fmt.Errorf("db: could not begin index transaction: %w", err)
		}
		started = true
	}
	rollback := func(e error) error {
		if started {
			_ = tx.Rollback()
			t.db.store.finishTransaction()
		}
		return e
	}
	if err := t.db.store.createIndexTx(tx, t.name, name, fields, unique); err != nil {
		return rollback(fmt.Errorf("db: could not create index '%s': %w", name, err))
	}
	t.mu.RLock()
	indexes := copyTableIndexes(t.indexes)
	t.mu.RUnlock()
	indexes[name] = &tableIndex{fields: append([]string(nil), fields...), unique: unique}
	indexesJSON, err := encodeIndexes(indexes)
	if err != nil {
		return rollback(fmt.Errorf("db: could not serialize indexes: %w", err))
	}
	if err := t.db.store.setMetaTx(tx, "indexes:"+t.name, indexesJSON); err != nil {
		return rollback(fmt.Errorf("db: could not persist indexes: %w", err))
	}
	if started {
		if err := tx.Commit(); err != nil {
			t.db.store.finishTransaction()
			return fmt.Errorf("db: could not commit index transaction: %w", err)
		}
		t.db.store.finishTransaction()
	}
	t.mu.Lock()
	t.indexes = indexes
	t.mu.Unlock()
	return nil
}

func tableObject(t *lunexTable) *runtime.Value {
	dbTableHandles.Store(t.handle, t)
	makeQB := func(filter *runtime.Value) *runtime.Value {
		return newQueryBuilder(t, filter)
	}

	obj := runtime.ObjectVal(map[string]*runtime.Value{
		"name":            runtime.StringVal(t.name),
		"__lunex_table__": runtime.BoolVal(true),
		"__lunex_handle":  runtime.StringVal(t.handle),

		"schema": runtime.FuncVal(&runtime.Function{Name: "schema", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 || args[0] == nil || args[0].Tag != runtime.TypeObject {
				if err := t.ensureLoaded(); err != nil {
					return runtime.Null, err
				}
				return tableObject(t), nil
			}
			if err := t.ensureLoaded(); err != nil {
				return runtime.Null, err
			}
			parsed := make(map[string]*fieldDef, len(args[0].ObjVal))
			for field, definition := range args[0].ObjVal {
				if field == "_id" {
					return runtime.Null, fmt.Errorf("field '_id' is reserved")
				}
				fd := &fieldDef{}
				switch definition.Tag {
				case runtime.TypeString:
					fd.Type = normalizeSchemaType(definition.StrVal)
				case runtime.TypeObject:
					if value, ok := definition.ObjVal["type"]; ok && value != nil {
						fd.Type = normalizeSchemaType(value.ToString())
					}
					if value, ok := definition.ObjVal["required"]; ok {
						if value.Tag != runtime.TypeBool {
							return runtime.Null, fmt.Errorf("schema field '%s': required must be boolean", field)
						}
						fd.Required = value.BoolVal
					}
					if value, ok := definition.ObjVal["unique"]; ok {
						if value.Tag != runtime.TypeBool {
							return runtime.Null, fmt.Errorf("schema field '%s': unique must be boolean", field)
						}
						fd.Unique = value.BoolVal
					}
					if value, ok := definition.ObjVal["index"]; ok {
						if value.Tag != runtime.TypeBool {
							return runtime.Null, fmt.Errorf("schema field '%s': index must be boolean", field)
						}
						fd.Index = value.BoolVal
					}
					if value, ok := definition.ObjVal["primary"]; ok {
						if value.Tag != runtime.TypeBool {
							return runtime.Null, fmt.Errorf("schema field '%s': primary must be boolean", field)
						}
						fd.Primary = value.BoolVal
					}
					if value, ok := definition.ObjVal["min"]; ok {
						fd.Min = value.ToNumber()
					}
					if value, ok := definition.ObjVal["max"]; ok {
						fd.Max = value.ToNumber()
					}
					if value, ok := definition.ObjVal["minLength"]; ok {
						fd.MinLen = int(value.ToNumber())
					}
					if value, ok := definition.ObjVal["maxLength"]; ok {
						fd.MaxLen = int(value.ToNumber())
						fd.MaxLenSet = true
					}
					if value, ok := definition.ObjVal["ref"]; ok {
						fd.Ref = value.ToString()
					}
					if value, ok := definition.ObjVal["onUpdate"]; ok {
						fd.OnUpdate = "$" + strings.TrimPrefix(value.ToString(), "$")
					}
					if value, ok := definition.ObjVal["default"]; ok {
						if value.Tag == runtime.TypeString {
							switch value.StrVal {
							case "uuid", "$uuid":
								fd.DefaultFn = "$uuid"
							case "now", "$now":
								fd.DefaultFn = "$now"
							case "seq", "$seq":
								fd.DefaultFn = "$seq"
							default:
								fd.DefaultVal = shared.DeepCopy(value)
							}
						} else {
							fd.DefaultVal = shared.DeepCopy(value)
						}
					}
					if value, ok := definition.ObjVal["enum"]; ok {
						if value.Tag != runtime.TypeArray {
							return runtime.Null, fmt.Errorf("schema field '%s': enum must be an array", field)
						}
						for _, item := range value.ArrVal {
							fd.Enum = append(fd.Enum, item.ToString())
						}
					}
				default:
					return runtime.Null, fmt.Errorf("schema field '%s': expected string or object definition", field)
				}
				if err := validateSchemaType(fd.Type); err != nil {
					return runtime.Null, fmt.Errorf("schema field '%s': %w", field, err)
				}
				if fd.MinLen < 0 || (fd.MaxLenSet && fd.MaxLen < 0) {
					return runtime.Null, fmt.Errorf("schema field '%s': length limits cannot be negative", field)
				}
				if fd.MaxLenSet && fd.MinLen > fd.MaxLen {
					return runtime.Null, fmt.Errorf("schema field '%s': minLength cannot exceed maxLength", field)
				}
				if fd.Min != 0 && fd.Max != 0 && fd.Min > fd.Max {
					return runtime.Null, fmt.Errorf("schema field '%s': min cannot exceed max", field)
				}
				if field == "id" {
					fd.Unique = true
					fd.Primary = true
				}
				if fd.Primary {
					fd.Required = true
					fd.Unique = true
				}
				parsed[field] = fd
			}
			if err := t.persistSchemaDefinition(parsed); err != nil {
				return runtime.Null, err
			}
			return tableObject(t), nil
		}}),

		"index": runtime.FuncVal(&runtime.Function{Name: "index", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if err := t.ensureLoaded(); err != nil {
				return runtime.Null, err
			}
			var fields []string
			if len(args) > 0 {
				if args[0].Tag == runtime.TypeArray {
					for _, field := range args[0].ArrVal {
						fields = append(fields, field.ToString())
					}
				} else {
					fields = []string{args[0].ToString()}
				}
			}
			if len(fields) == 0 {
				return tableObject(t), nil
			}
			unique := false
			if len(args) > 1 && args[1] != nil && args[1].Tag == runtime.TypeObject {
				if value, ok := args[1].ObjVal["unique"]; ok {
					if value.Tag != runtime.TypeBool {
						return runtime.Null, fmt.Errorf("index option 'unique' must be boolean")
					}
					unique = value.BoolVal
				}
			}
			for _, field := range fields {
				if field != "_id" && !validFieldName.MatchString(field) {
					return runtime.Null, fmt.Errorf("db: invalid field name %q", field)
				}
			}
			if err := t.persistIndexDefinition(fields, unique); err != nil {
				return runtime.Null, err
			}
			return tableObject(t), nil
		}}),

		"insert": runtime.FuncVal(&runtime.Function{Name: "insert", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 || args[0].Tag != runtime.TypeObject {
				return runtime.Null, fmt.Errorf("insert: expected object")
			}
			doc := make(map[string]*runtime.Value, len(args[0].ObjVal))
			for k, v := range args[0].ObjVal {
				doc[k] = v
			}
			return t.insertDoc(doc)
		}}),

		"insertMany": runtime.FuncVal(&runtime.Function{Name: "insertMany", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 || args[0] == nil || args[0].Tag != runtime.TypeArray {
				return runtime.Null, fmt.Errorf("insertMany: expected array")
			}
			if t.tx != nil {
				var out []*runtime.Value
				for _, item := range args[0].ArrVal {
					if item == nil || item.Tag != runtime.TypeObject {
						return runtime.Null, fmt.Errorf("insertMany: every record must be an object")
					}
					doc := make(map[string]*runtime.Value, len(item.ObjVal))
					for k, v := range item.ObjVal {
						doc[k] = v
					}
					r, err := t.insertDoc(doc)
					if err != nil {
						return runtime.Null, err
					}
					out = append(out, r)
				}
				return runtime.ArrayVal(out), nil
			}
			if err := t.ensureLoaded(); err != nil {
				return runtime.Null, err
			}
			tx, err := t.db.store.begin()
			if err != nil {
				return runtime.Null, fmt.Errorf("insertMany: could not begin transaction: %w", err)
			}
			txTable := cloneTableForTx(t, tx)
			var out []*runtime.Value
			insertedRows := make([]dbRow, 0, len(args[0].ArrVal))
			for _, item := range args[0].ArrVal {
				if item == nil || item.Tag != runtime.TypeObject {
					_ = tx.Rollback()
					t.db.store.finishTransaction()
					return runtime.Null, fmt.Errorf("insertMany: every record must be an object")
				}
				doc := make(map[string]*runtime.Value, len(item.ObjVal))
				for k, v := range item.ObjVal {
					doc[k] = v
				}
				beforeLen := len(txTable.rows)
				r, insertErr := txTable.insertDoc(doc)
				if insertErr != nil {
					_ = tx.Rollback()
					t.db.store.finishTransaction()
					return runtime.Null, insertErr
				}
				out = append(out, r)
				if len(txTable.rows) > beforeLen {
					insertedRows = append(insertedRows, txTable.rows[len(txTable.rows)-1])
				}
			}
			if err := tx.Commit(); err != nil {
				t.db.store.finishTransaction()
				return runtime.Null, fmt.Errorf("insertMany: could not commit transaction: %w", err)
			}
			t.db.store.finishTransaction()
			t.db.invalidateTables()
			for _, row := range insertedRows {
				t.notifyWatches("insert", row)
			}
			return runtime.ArrayVal(out), nil
		}}),

		"upsert": runtime.FuncVal(&runtime.Function{Name: "upsert", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) < 2 || args[0] == nil || args[0].Tag != runtime.TypeObject || args[1] == nil || args[1].Tag != runtime.TypeObject {
				return runtime.Null, fmt.Errorf("upsert: expected query and object update")
			}
			rows := t.execQuery(args[0], nil, nil, 1, 0)
			if len(rows) > 0 {
				changes := make(map[string]*runtime.Value, len(args[1].ObjVal))
				for key, value := range args[1].ObjVal {
					changes[key] = value
				}
				if _, err := t.updateRows(args[0], changes, true); err != nil {
					return runtime.Null, err
				}
				rows = t.execQuery(args[0], nil, nil, 1, 0)
				if len(rows) == 0 {
					return runtime.Null, nil
				}
				return rows[0], nil
			}
			doc := make(map[string]*runtime.Value, len(args[0].ObjVal)+len(args[1].ObjVal))
			for key, value := range args[0].ObjVal {
				if strings.HasPrefix(key, "$") || value == nil || value.Tag == runtime.TypeObject {
					continue
				}
				doc[key] = value
			}
			for key, value := range args[1].ObjVal {
				if key == "$set" && value != nil && value.Tag == runtime.TypeObject {
					for field, item := range value.ObjVal {
						doc[field] = item
					}
					continue
				}
				if !strings.HasPrefix(key, "$") {
					doc[key] = value
				}
			}
			return t.insertDoc(doc)
		}}),

		"find": runtime.FuncVal(&runtime.Function{Name: "find", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			var filter *runtime.Value
			if len(args) > 0 {
				filter = args[0]
			}
			var proj []string
			var sorts []sortEntry
			limitN, offsetN := 0, 0
			if len(args) > 1 && args[1] != nil && args[1].Tag == runtime.TypeObject {
				opts := args[1].ObjVal
				if v, ok := opts["select"]; ok && v.Tag == runtime.TypeArray {
					for _, f := range v.ArrVal {
						proj = append(proj, f.ToString())
					}
				}
				if v, ok := opts["sort"]; ok && v.Tag == runtime.TypeObject {
					for f, d := range v.ObjVal {
						sorts = append(sorts, sortEntry{f, d.ToNumber() < 0})
					}
				}
				if v, ok := opts["orderBy"]; ok {
					sorts = append(sorts, sortEntry{v.ToString(), false})
				}
				if v, ok := opts["limit"]; ok {
					limitN = int(v.ToNumber())
				}
				if v, ok := opts["offset"]; ok {
					offsetN = int(v.ToNumber())
				}
				if v, ok := opts["skip"]; ok {
					offsetN = int(v.ToNumber())
				}
			}
			return runtime.ArrayVal(t.execQuery(filter, proj, sorts, limitN, offsetN)), nil
		}}),

		"findOne": runtime.FuncVal(&runtime.Function{Name: "findOne", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			var filter *runtime.Value
			if len(args) > 0 {
				filter = args[0]
			}
			rows := t.execQuery(filter, nil, nil, 1, 0)
			if len(rows) == 0 {
				return runtime.Null, nil
			}
			return rows[0], nil
		}}),

		"findById": runtime.FuncVal(&runtime.Function{Name: "findById", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 || args[0] == nil || args[0].IsNullish() {
				return runtime.Null, nil
			}
			if err := t.ensureLoaded(); err != nil {
				return runtime.Null, err
			}
			rows := t.execQuery(t.idFilter(args[0]), nil, nil, 1, 0)
			if len(rows) == 0 {
				return runtime.Null, nil
			}
			return rows[0], nil
		}}),

		"update": runtime.FuncVal(&runtime.Function{Name: "update", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) < 2 || args[1].Tag != runtime.TypeObject {
				return runtime.NumberVal(0), nil
			}
			changes := make(map[string]*runtime.Value, len(args[1].ObjVal))
			for k, v := range args[1].ObjVal {
				changes[k] = v
			}
			n, err := t.updateRows(args[0], changes, false)
			if err != nil {
				return runtime.NumberVal(0), err
			}
			return runtime.NumberVal(float64(n)), nil
		}}),

		"updateOne": runtime.FuncVal(&runtime.Function{Name: "updateOne", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) < 2 || args[1].Tag != runtime.TypeObject {
				return runtime.NumberVal(0), nil
			}
			changes := make(map[string]*runtime.Value, len(args[1].ObjVal))
			for k, v := range args[1].ObjVal {
				changes[k] = v
			}
			n, err := t.updateRows(args[0], changes, true)
			if err != nil {
				return runtime.NumberVal(0), err
			}
			return runtime.NumberVal(float64(n)), nil
		}}),

		"delete": runtime.FuncVal(&runtime.Function{Name: "delete", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			var filter *runtime.Value
			if len(args) > 0 {
				filter = args[0]
			}
			n, err := t.deleteRows(filter, false)
			if err != nil {
				return runtime.NumberVal(0), err
			}
			return runtime.NumberVal(float64(n)), nil
		}}),

		"deleteOne": runtime.FuncVal(&runtime.Function{Name: "deleteOne", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			var filter *runtime.Value
			if len(args) > 0 {
				filter = args[0]
			}
			n, err := t.deleteRows(filter, true)
			if err != nil {
				return runtime.NumberVal(0), err
			}
			return runtime.NumberVal(float64(n)), nil
		}}),

		"deleteById": runtime.FuncVal(&runtime.Function{Name: "deleteById", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.NumberVal(0), nil
			}
			n, err := t.deleteRows(t.idFilter(args[0]), true)
			if err != nil {
				return runtime.NumberVal(0), err
			}
			return runtime.NumberVal(float64(n)), nil
		}}),

		"count": runtime.FuncVal(&runtime.Function{Name: "count", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			var filter *runtime.Value
			if len(args) > 0 {
				filter = args[0]
			}
			if err := t.ensureLoaded(); err != nil {
				return runtime.NumberVal(0), err
			}
			t.mu.RLock()
			defer t.mu.RUnlock()
			n := 0
			for _, row := range t.rows {
				if t.matchRowFilter(row, filter) {
					n++
				}
			}
			return runtime.NumberVal(float64(n)), nil
		}}),

		"exists": runtime.FuncVal(&runtime.Function{Name: "exists", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			var filter *runtime.Value
			if len(args) > 0 {
				filter = args[0]
			}
			rows := t.execQuery(filter, nil, nil, 1, 0)
			return runtime.BoolVal(len(rows) > 0), nil
		}}),

		"distinct": runtime.FuncVal(&runtime.Function{Name: "distinct", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.ArrayVal(nil), nil
			}
			field := args[0].ToString()
			var filter *runtime.Value
			if len(args) > 1 {
				filter = args[1]
			}
			if err := t.ensureLoaded(); err != nil {
				return runtime.ArrayVal(nil), err
			}
			t.mu.RLock()
			defer t.mu.RUnlock()
			seen := make(map[string]bool)
			var out []*runtime.Value
			for _, row := range t.rows {
				if !t.matchRowFilter(row, filter) {
					continue
				}
				var v *runtime.Value
				if field == "_id" {
					v = t.publicRowID(row)
				} else {
					v = row.doc[field]
				}
				if v == nil {
					continue
				}
				keyBytes, _ := json.Marshal(valueToNative(v))
				key := shared.GetTypeName(v) + ":" + string(keyBytes)
				if !seen[key] {
					seen[key] = true
					out = append(out, v)
				}
			}
			return runtime.ArrayVal(out), nil
		}}),

		"where": runtime.FuncVal(&runtime.Function{Name: "where", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			filter, err := buildWhereFilter(args)
			if err != nil {
				return runtime.Null, err
			}
			return makeQB(filter), nil
		}}),

		"select": runtime.FuncVal(&runtime.Function{Name: "select", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			state := &qbState{table: t, limitN: 0, offsetN: 0}
			if len(args) > 0 && args[0] != nil && args[0].Tag == runtime.TypeArray {
				for _, field := range args[0].ArrVal {
					state.proj = append(state.proj, field.ToString())
				}
			}
			return qbObject(state), nil
		}}),

		"orderBy": runtime.FuncVal(&runtime.Function{Name: "orderBy", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			state := &qbState{table: t, limitN: 0, offsetN: 0}
			if len(args) > 0 {
				desc := len(args) > 1 && strings.EqualFold(args[1].ToString(), "desc")
				state.sorts = append(state.sorts, sortEntry{args[0].ToString(), desc})
			}
			return qbObject(state), nil
		}}),

		"limit": runtime.FuncVal(&runtime.Function{Name: "limit", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			state := &qbState{table: t, limitN: 0, offsetN: 0}
			if len(args) > 0 {
				state.limitN = int(args[0].ToNumber())
			}
			return qbObject(state), nil
		}}),

		"aggregate": runtime.FuncVal(&runtime.Function{Name: "aggregate", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.ArrayVal(nil), nil
			}
			return runtime.ArrayVal(t.aggregate(args[0])), nil
		}}),

		"sum": runtime.FuncVal(&runtime.Function{Name: "sum", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.NumberVal(0), nil
			}
			field := args[0].ToString()
			var filter *runtime.Value
			if len(args) > 1 {
				filter = args[1]
			}
			if err := t.ensureLoaded(); err != nil {
				return runtime.NumberVal(0), err
			}
			t.mu.RLock()
			defer t.mu.RUnlock()
			sum := 0.0
			for _, row := range t.rows {
				if !t.matchRowFilter(row, filter) {
					continue
				}
				if v, ok := row.doc[field]; ok {
					sum += v.ToNumber()
				}
			}
			return runtime.NumberVal(sum), nil
		}}),

		"avg": runtime.FuncVal(&runtime.Function{Name: "avg", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.NumberVal(0), nil
			}
			field := args[0].ToString()
			var filter *runtime.Value
			if len(args) > 1 {
				filter = args[1]
			}
			if err := t.ensureLoaded(); err != nil {
				return runtime.NumberVal(0), err
			}
			t.mu.RLock()
			defer t.mu.RUnlock()
			sum, n := 0.0, 0
			for _, row := range t.rows {
				if !t.matchRowFilter(row, filter) {
					continue
				}
				if v, ok := row.doc[field]; ok {
					sum += v.ToNumber()
					n++
				}
			}
			if n == 0 {
				return runtime.NumberVal(0), nil
			}
			return runtime.NumberVal(sum / float64(n)), nil
		}}),

		"min": runtime.FuncVal(&runtime.Function{Name: "min", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.Null, nil
			}
			field := args[0].ToString()
			var filter *runtime.Value
			if len(args) > 1 {
				filter = args[1]
			}
			if err := t.ensureLoaded(); err != nil {
				return runtime.Null, err
			}
			t.mu.RLock()
			defer t.mu.RUnlock()
			min := math.MaxFloat64
			for _, row := range t.rows {
				if !t.matchRowFilter(row, filter) {
					continue
				}
				if v, ok := row.doc[field]; ok {
					if v.ToNumber() < min {
						min = v.ToNumber()
					}
				}
			}
			if min == math.MaxFloat64 {
				return runtime.Null, nil
			}
			return runtime.NumberVal(min), nil
		}}),

		"max": runtime.FuncVal(&runtime.Function{Name: "max", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.Null, nil
			}
			field := args[0].ToString()
			var filter *runtime.Value
			if len(args) > 1 {
				filter = args[1]
			}
			if err := t.ensureLoaded(); err != nil {
				return runtime.Null, err
			}
			t.mu.RLock()
			defer t.mu.RUnlock()
			max := -math.MaxFloat64
			for _, row := range t.rows {
				if !t.matchRowFilter(row, filter) {
					continue
				}
				if v, ok := row.doc[field]; ok {
					if v.ToNumber() > max {
						max = v.ToNumber()
					}
				}
			}
			if max == -math.MaxFloat64 {
				return runtime.Null, nil
			}
			return runtime.NumberVal(max), nil
		}}),

		"join": runtime.FuncVal(&runtime.Function{Name: "join", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) < 3 {
				return runtime.Null, fmt.Errorf("join: expected table, local field, and foreign field")
			}
			other, err := resolveTableHandle(args[0])
			if err != nil {
				return runtime.Null, err
			}
			localField, foreignField := args[1].ToString(), args[2].ToString()
			if localField == "" || foreignField == "" {
				return runtime.Null, fmt.Errorf("join: field names are required")
			}
			as := other.name
			joinType, single := "left", false
			if len(args) > 3 {
				if args[3].Tag == runtime.TypeString {
					as = args[3].ToString()
				}
				if args[3].Tag == runtime.TypeObject {
					if v, ok := args[3].ObjVal["as"]; ok {
						as = v.ToString()
					}
					if v, ok := args[3].ObjVal["type"]; ok {
						joinType = strings.ToLower(v.ToString())
					}
					if v, ok := args[3].ObjVal["single"]; ok {
						if v.Tag != runtime.TypeBool {
							return runtime.Null, fmt.Errorf("join option 'single' must be boolean")
						}
						single = v.BoolVal
					}
				}
			}
			if joinType != "left" && joinType != "inner" {
				return runtime.Null, fmt.Errorf("join: type must be 'left' or 'inner'")
			}
			if err := t.ensureLoaded(); err != nil {
				return runtime.Null, err
			}
			if err := other.ensureLoaded(); err != nil {
				return runtime.Null, err
			}
			leftRows := t.execQuery(nil, nil, nil, 0, 0)
			rightRows := other.execQuery(nil, nil, nil, 0, 0)
			out := make([]*runtime.Value, 0, len(leftRows))
			for _, left := range leftRows {
				local := left.ObjVal[localField]
				matches := make([]*runtime.Value, 0)
				for _, right := range rightRows {
					foreign := right.ObjVal[foreignField]
					if local != nil && foreign != nil && local.StrictEquals(foreign) {
						matches = append(matches, right)
					}
				}
				if joinType == "inner" && len(matches) == 0 {
					continue
				}
				merged := make(map[string]*runtime.Value, len(left.ObjVal)+1)
				for key, value := range left.ObjVal {
					merged[key] = value
				}
				if single {
					if len(matches) == 0 {
						merged[as] = runtime.Null
					} else {
						merged[as] = matches[0]
					}
				} else {
					merged[as] = runtime.ArrayVal(matches)
				}
				out = append(out, runtime.ObjectVal(merged))
			}
			return runtime.ArrayVal(out), nil
		}}),

		"search": runtime.FuncVal(&runtime.Function{Name: "search", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.ArrayVal(nil), nil
			}
			text := args[0].ToString()
			var fields []string
			if len(args) > 1 && args[1].Tag == runtime.TypeArray {
				for _, f := range args[1].ArrVal {
					fields = append(fields, f.ToString())
				}
			}
			return runtime.ArrayVal(t.search(text, fields)), nil
		}}),

		"watch": runtime.FuncVal(&runtime.Function{Name: "watch", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			var filter *runtime.Value
			var fn *runtime.Value
			if len(args) >= 2 {
				filter = args[0]
				fn = args[1]
			} else if len(args) == 1 {
				fn = args[0]
			}
			if fn == nil || fn.Tag != runtime.TypeFunction {
				return runtime.Undefined, nil
			}
			watchID := genUUID()
			t.mu.Lock()
			t.watches = append(t.watches, &tableWatch{id: watchID, filter: filter, fn: fn})
			t.mu.Unlock()
			return runtime.FuncVal(&runtime.Function{Name: "unwatch", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
				t.mu.Lock()
				for i, w := range t.watches {
					if w.id == watchID {
						t.watches = append(t.watches[:i], t.watches[i+1:]...)
						break
					}
				}
				t.mu.Unlock()
				return runtime.Undefined, nil
			}}), nil
		}}),

		"clear": runtime.FuncVal(&runtime.Function{Name: "clear", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if err := t.db.store.ensureTableTx(t.tx, t.name); err != nil {
				return runtime.Undefined, fmt.Errorf("db: could not create table '%s': %w", t.name, err)
			}
			if err := t.db.store.clearRowsTx(t.tx, t.name); err != nil {
				return runtime.Undefined, fmt.Errorf("db: could not clear table '%s': %w", t.name, err)
			}
			t.mu.Lock()
			t.rows = nil
			t.loaded = true
			t.mu.Unlock()
			return runtime.Undefined, nil
		}}),

		"drop": runtime.FuncVal(&runtime.Function{Name: "drop", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if err := t.db.store.dropTableTx(t.tx, t.name); err != nil {
				return runtime.Undefined, fmt.Errorf("db: could not drop table '%s': %w", t.name, err)
			}
			t.mu.Lock()
			t.rows = nil
			t.loaded = false
			t.schema = make(map[string]*fieldDef)
			t.indexes = make(map[string]*tableIndex)
			t.watches = nil
			t.mu.Unlock()
			return runtime.Undefined, nil
		}}),

		"dump": runtime.FuncVal(&runtime.Function{Name: "dump", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			rows := t.execQuery(nil, nil, nil, 0, 0)
			return runtime.ArrayVal(rows), nil
		}}),

		"indexes": runtime.FuncVal(&runtime.Function{Name: "indexes", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if err := t.ensureLoaded(); err != nil {
				return runtime.ArrayVal(nil), err
			}
			t.mu.RLock()
			defer t.mu.RUnlock()
			var out []*runtime.Value
			for name, idx := range t.indexes {
				fields := make([]*runtime.Value, len(idx.fields))
				for i, f := range idx.fields {
					fields[i] = runtime.StringVal(f)
				}
				out = append(out, runtime.ObjectVal(map[string]*runtime.Value{
					"name":   runtime.StringVal(name),
					"fields": runtime.ArrayVal(fields),
					"unique": runtime.BoolVal(idx.unique),
				}))
			}
			return runtime.ArrayVal(out), nil
		}}),
	})
	schemaFn := obj.ObjVal["schema"]
	obj.ObjVal["define"] = schemaFn
	return obj
}

func newQueryBuilder(t *lunexTable, filter *runtime.Value) *runtime.Value {
	qb := &qbState{table: t, filter: filter, limitN: 0, offsetN: 0}
	return qbObject(qb)
}

func normalizeWhereOperator(operator string) (string, bool) {
	operator = strings.ToLower(strings.TrimSpace(operator))
	switch operator {
	case "=", "==", "$eq", "eq":
		return "$eq", true
	case "!=", "<>", "$ne", "ne":
		return "$ne", true
	case ">", "$gt", "gt":
		return "$gt", true
	case ">=", "$gte", "gte":
		return "$gte", true
	case "<", "$lt", "lt":
		return "$lt", true
	case "<=", "$lte", "lte":
		return "$lte", true
	default:
		return "", false
	}
}

func buildWhereFilter(args []*runtime.Value) (*runtime.Value, error) {
	switch len(args) {
	case 0:
		return nil, nil
	case 1:
		if args[0] == nil || args[0].IsNullish() {
			return nil, nil
		}
		if args[0].Tag != runtime.TypeObject {
			return nil, fmt.Errorf("where: expected an object filter or (field, operator, value)")
		}
		return args[0], nil
	case 3:
		if args[0] == nil || args[0].Tag != runtime.TypeString {
			return nil, fmt.Errorf("where: field must be a string")
		}
		if args[1] == nil || args[1].Tag != runtime.TypeString {
			return nil, fmt.Errorf("where: operator must be a string")
		}
		field := strings.TrimSpace(args[0].ToString())
		if field == "" || strings.HasPrefix(field, "$") {
			return nil, fmt.Errorf("where: field must be a non-empty field name")
		}
		operator, ok := normalizeWhereOperator(args[1].ToString())
		if !ok {
			return nil, fmt.Errorf("where: unsupported operator %q", args[1].ToString())
		}
		return runtime.ObjectVal(map[string]*runtime.Value{
			field: runtime.ObjectVal(map[string]*runtime.Value{
				operator: args[2],
			}),
		}), nil
	default:
		return nil, fmt.Errorf("where: expected a filter object or (field, operator, value)")
	}
}

type qbState struct {
	table   *lunexTable
	filter  *runtime.Value
	proj    []string
	sorts   []sortEntry
	limitN  int
	offsetN int
}

func qbObject(qb *qbState) *runtime.Value {
	return runtime.ObjectVal(map[string]*runtime.Value{
		"where": runtime.FuncVal(&runtime.Function{Name: "where", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			filter, err := buildWhereFilter(args)
			if err != nil {
				return runtime.Null, err
			}
			qb.filter = filter
			return qbObject(qb), nil
		}}),
		"and": runtime.FuncVal(&runtime.Function{Name: "and", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) > 0 && qb.filter != nil {
				qb.filter = runtime.ObjectVal(map[string]*runtime.Value{
					"$and": runtime.ArrayVal([]*runtime.Value{qb.filter, args[0]}),
				})
			} else if len(args) > 0 {
				qb.filter = args[0]
			}
			return qbObject(qb), nil
		}}),
		"or": runtime.FuncVal(&runtime.Function{Name: "or", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) > 0 && qb.filter != nil {
				qb.filter = runtime.ObjectVal(map[string]*runtime.Value{
					"$or": runtime.ArrayVal([]*runtime.Value{qb.filter, args[0]}),
				})
			} else if len(args) > 0 {
				qb.filter = args[0]
			}
			return qbObject(qb), nil
		}}),
		"select": runtime.FuncVal(&runtime.Function{Name: "select", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			qb.proj = nil
			if len(args) > 0 && args[0].Tag == runtime.TypeArray {
				for _, f := range args[0].ArrVal {
					qb.proj = append(qb.proj, f.ToString())
				}
			}
			return qbObject(qb), nil
		}}),
		"orderBy": runtime.FuncVal(&runtime.Function{Name: "orderBy", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) > 0 {
				desc := false
				if len(args) > 1 {
					desc = strings.ToLower(args[1].ToString()) == "desc"
				}
				qb.sorts = append(qb.sorts, sortEntry{args[0].ToString(), desc})
			}
			return qbObject(qb), nil
		}}),
		"limit": runtime.FuncVal(&runtime.Function{Name: "limit", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) > 0 {
				qb.limitN = int(args[0].ToNumber())
			}
			return qbObject(qb), nil
		}}),
		"offset": runtime.FuncVal(&runtime.Function{Name: "offset", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) > 0 {
				qb.offsetN = int(args[0].ToNumber())
			}
			return qbObject(qb), nil
		}}),
		"skip": runtime.FuncVal(&runtime.Function{Name: "skip", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) > 0 {
				qb.offsetN = int(args[0].ToNumber())
			}
			return qbObject(qb), nil
		}}),
		"page": runtime.FuncVal(&runtime.Function{Name: "page", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) >= 2 {
				n := int(args[0].ToNumber())
				size := int(args[1].ToNumber())
				if n < 1 {
					n = 1
				}
				qb.offsetN = (n - 1) * size
				qb.limitN = size
			}
			return qbObject(qb), nil
		}}),
		"find": runtime.FuncVal(&runtime.Function{Name: "find", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			return runtime.ArrayVal(qb.table.execQuery(qb.filter, qb.proj, qb.sorts, qb.limitN, qb.offsetN)), nil
		}}),
		"exec": runtime.FuncVal(&runtime.Function{Name: "exec", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			return runtime.ArrayVal(qb.table.execQuery(qb.filter, qb.proj, qb.sorts, qb.limitN, qb.offsetN)), nil
		}}),
		"first": runtime.FuncVal(&runtime.Function{Name: "first", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			rows := qb.table.execQuery(qb.filter, qb.proj, qb.sorts, 1, qb.offsetN)
			if len(rows) == 0 {
				return runtime.Null, nil
			}
			return rows[0], nil
		}}),
		"last": runtime.FuncVal(&runtime.Function{Name: "last", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			rows := qb.table.execQuery(qb.filter, qb.proj, qb.sorts, 0, 0)
			if len(rows) == 0 {
				return runtime.Null, nil
			}
			return rows[len(rows)-1], nil
		}}),
		"count": runtime.FuncVal(&runtime.Function{Name: "count", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			rows := qb.table.execQuery(qb.filter, nil, nil, 0, 0)
			return runtime.NumberVal(float64(len(rows))), nil
		}}),
		"exists": runtime.FuncVal(&runtime.Function{Name: "exists", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			rows := qb.table.execQuery(qb.filter, nil, nil, 1, 0)
			return runtime.BoolVal(len(rows) > 0), nil
		}}),
		"delete": runtime.FuncVal(&runtime.Function{Name: "delete", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			n, err := qb.table.deleteRows(qb.filter, false)
			if err != nil {
				return runtime.NumberVal(0), err
			}
			return runtime.NumberVal(float64(n)), nil
		}}),
		"update": runtime.FuncVal(&runtime.Function{Name: "update", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 || args[0].Tag != runtime.TypeObject {
				return runtime.NumberVal(0), nil
			}
			changes := make(map[string]*runtime.Value)
			for k, v := range args[0].ObjVal {
				changes[k] = v
			}
			n, err := qb.table.updateRows(qb.filter, changes, false)
			if err != nil {
				return runtime.NumberVal(0), err
			}
			return runtime.NumberVal(float64(n)), nil
		}}),
	})
}

func dbObjectWithTx(db *lunexDB, tx *sql.Tx) *runtime.Value {
	return runtime.ObjectVal(map[string]*runtime.Value{
		"name": runtime.StringVal(db.name),
		"path": runtime.StringVal(db.store.path),
		"table": runtime.FuncVal(&runtime.Function{Name: "table", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.Null, fmt.Errorf("table: name required")
			}
			if tx != nil {
				return tableObject(db.txTable(args[0].ToString(), tx)), nil
			}
			return tableObject(db.table(args[0].ToString())), nil
		}}),
		"collection": runtime.FuncVal(&runtime.Function{Name: "collection", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.Null, fmt.Errorf("collection: name required")
			}
			if tx != nil {
				return tableObject(db.txTable(args[0].ToString(), tx)), nil
			}
			return tableObject(db.table(args[0].ToString())), nil
		}}),
		"tables": runtime.FuncVal(&runtime.Function{Name: "tables", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			names := db.tableNamesTx(tx)
			out := make([]*runtime.Value, len(names))
			for i, n := range names {
				out[i] = runtime.StringVal(n)
			}
			return runtime.ArrayVal(out), nil
		}}),
		"drop": runtime.FuncVal(&runtime.Function{Name: "drop", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) > 0 {
				name := args[0].ToString()
				if err := db.store.dropTableTx(tx, name); err != nil {
					return runtime.Undefined, fmt.Errorf("db: could not drop table '%s': %w", name, err)
				}
				if tx == nil {
					var oldTable *lunexTable
					db.mu.Lock()
					oldTable = db.tables[name]
					delete(db.tables, name)
					db.mu.Unlock()
					if oldTable != nil {
						oldTable.mu.Lock()
						oldTable.rows = nil
						oldTable.loaded = false
						oldTable.schema = make(map[string]*fieldDef)
						oldTable.indexes = make(map[string]*tableIndex)
						oldTable.watches = nil
						oldTable.mu.Unlock()
					}
				}
			}
			return runtime.Undefined, nil
		}}),
		"transaction": runtime.FuncVal(&runtime.Function{Name: "transaction", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 || args[0] == nil || args[0].Tag != runtime.TypeFunction {
				return runtime.Null, fmt.Errorf("transaction: expected function")
			}
			if tx != nil {
				return runtime.Null, fmt.Errorf("transaction: nested transactions are not supported")
			}
			transaction, err := db.store.begin()
			if err != nil {
				return runtime.Null, fmt.Errorf("transaction: begin failed: %w", err)
			}
			result, callErr := runtime.CallFunction(args[0], []*runtime.Value{dbObjectWithTx(db, transaction)})
			if callErr != nil {
				_ = transaction.Rollback()
				db.store.finishTransaction()
				return runtime.Null, callErr
			}
			if err := transaction.Commit(); err != nil {
				db.store.finishTransaction()
				return runtime.Null, fmt.Errorf("transaction: commit failed: %w", err)
			}
			db.store.finishTransaction()
			db.invalidateTables()
			return result, nil
		}}),
		"dump": runtime.FuncVal(&runtime.Function{Name: "dump", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			names := db.tableNamesTx(tx)
			out := make(map[string]*runtime.Value, len(names))
			for _, name := range names {
				var t *lunexTable
				if tx != nil {
					t = db.txTable(name, tx)
				} else {
					t = db.table(name)
				}
				rows := t.execQuery(nil, nil, nil, 0, 0)
				out[name] = runtime.ArrayVal(rows)
			}
			return runtime.ObjectVal(out), nil
		}}),
		"load": runtime.FuncVal(&runtime.Function{Name: "load", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 || args[0] == nil || args[0].Tag != runtime.TypeObject {
				return runtime.Null, fmt.Errorf("load: expected object")
			}
			if tx != nil {
				for tname, tdata := range args[0].ObjVal {
					t := db.txTable(tname, tx)
					if tdata == nil || tdata.Tag != runtime.TypeArray {
						return runtime.Null, fmt.Errorf("load: table '%s' must be an array", tname)
					}
					for _, item := range tdata.ArrVal {
						if item == nil || item.Tag != runtime.TypeObject {
							return runtime.Null, fmt.Errorf("load: table '%s' contains a non-object record", tname)
						}
						doc := make(map[string]*runtime.Value, len(item.ObjVal))
						for key, value := range item.ObjVal {
							if key != "_id" {
								doc[key] = value
							}
						}
						if _, err := t.insertDoc(doc); err != nil {
							return runtime.Null, err
						}
					}
				}
				return runtime.Undefined, nil
			}
			transaction, err := db.store.begin()
			if err != nil {
				return runtime.Null, fmt.Errorf("load: could not begin transaction: %w", err)
			}
			tables := make(map[string]*lunexTable)
			events := make(map[string][]dbRow)
			for tname, tdata := range args[0].ObjVal {
				if tdata == nil || tdata.Tag != runtime.TypeArray {
					_ = transaction.Rollback()
					db.store.finishTransaction()
					return runtime.Null, fmt.Errorf("load: table '%s' must be an array", tname)
				}
				t, ok := tables[tname]
				if !ok {
					t = db.txTable(tname, transaction)
					tables[tname] = t
				}
				for _, item := range tdata.ArrVal {
					if item == nil || item.Tag != runtime.TypeObject {
						_ = transaction.Rollback()
						db.store.finishTransaction()
						return runtime.Null, fmt.Errorf("load: table '%s' contains a non-object record", tname)
					}
					doc := make(map[string]*runtime.Value, len(item.ObjVal))
					for key, value := range item.ObjVal {
						if key != "_id" {
							doc[key] = value
						}
					}
					beforeLen := len(t.rows)
					if _, err := t.insertDoc(doc); err != nil {
						_ = transaction.Rollback()
						db.store.finishTransaction()
						return runtime.Null, err
					}
					if len(t.rows) > beforeLen {
						events[tname] = append(events[tname], t.rows[len(t.rows)-1])
					}
				}
			}
			if err := transaction.Commit(); err != nil {
				db.store.finishTransaction()
				return runtime.Null, fmt.Errorf("load: could not commit transaction: %w", err)
			}
			db.store.finishTransaction()
			db.invalidateTables()
			for name, rows := range events {
				if base := db.table(name); base != nil {
					for _, row := range rows {
						base.notifyWatches("insert", row)
					}
				}
			}
			return runtime.Undefined, nil
		}}),

		"close": runtime.FuncVal(&runtime.Function{Name: "close", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if tx != nil {
				return runtime.Undefined, fmt.Errorf("close: cannot close a database from inside a transaction")
			}
			return runtime.Undefined, globalDBEngine.close(db)
		}}),
	})
}

func dbObject(db *lunexDB) *runtime.Value { return dbObjectWithTx(db, nil) }

func DbModule() *runtime.Value {
	return runtime.ObjectVal(map[string]*runtime.Value{
		"create": runtime.FuncVal(&runtime.Function{Name: "create", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			name := "default"
			if len(args) > 0 && args[0] != nil && !args[0].IsNullish() {
				name = args[0].ToString()
			}
			db, err := globalDBEngine.open(name)
			if err != nil {
				return runtime.Null, fmt.Errorf("db.create: %w", err)
			}
			return dbObject(db), nil
		}}),
		"open": runtime.FuncVal(&runtime.Function{Name: "open", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			name := "default"
			if len(args) > 0 && !args[0].IsNullish() {
				name = args[0].ToString()
			}
			db, err := globalDBEngine.open(name)
			if err != nil {
				return runtime.Null, fmt.Errorf("db.open: %w", err)
			}
			return dbObject(db), nil
		}}),
		"connect": runtime.FuncVal(&runtime.Function{Name: "connect", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			name := "default"
			if len(args) > 0 && !args[0].IsNullish() {
				name = args[0].ToString()
			}
			db, err := globalDBEngine.open(name)
			if err != nil {
				return runtime.Null, fmt.Errorf("db.connect: %w", err)
			}
			return dbObject(db), nil
		}}),
		"drop": runtime.FuncVal(&runtime.Function{Name: "drop", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) > 0 {
				if err := globalDBEngine.drop(args[0].ToString()); err != nil {
					return runtime.Undefined, fmt.Errorf("db.drop: %w", err)
				}
			}
			return runtime.Undefined, nil
		}}),
		"list": runtime.FuncVal(&runtime.Function{Name: "list", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			names := globalDBEngine.list()
			out := make([]*runtime.Value, len(names))
			for i, n := range names {
				out[i] = runtime.StringVal(n)
			}
			return runtime.ArrayVal(out), nil
		}}),
		"table": runtime.FuncVal(&runtime.Function{Name: "table", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.Null, fmt.Errorf("table: name required")
			}
			db, err := globalDBEngine.open("default")
			if err != nil {
				return runtime.Null, fmt.Errorf("db.table: %w", err)
			}
			return tableObject(db.table(args[0].ToString())), nil
		}}),
		"collection": runtime.FuncVal(&runtime.Function{Name: "collection", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.Null, fmt.Errorf("collection: name required")
			}
			db, err := globalDBEngine.open("default")
			if err != nil {
				return runtime.Null, fmt.Errorf("db.collection: %w", err)
			}
			return tableObject(db.table(args[0].ToString())), nil
		}}),
	})
}
