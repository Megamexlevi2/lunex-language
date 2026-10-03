# lune-xml

XML module for the [Lunex](https://github.com/Megamexlevi2) programming language.

---

## Installation

Copy the `lune-xml` folder into your project and import:

```lx
val xml = @fimport("./lune-xml/main.nax")   
val xml = @fimport("./lune-xml/main.lx")    
```

---

## Beginner Guide

If you are new to lune-xml, start here. These helpers cover the most
common tasks with a minimal number of calls.

### Create and serialize XML

```lx
val io  = @import("std.io")
val xml = @fimport("./lune-xml/main.nax")

fn main() {
  
  
  
  
  

  val title  = xml.tag("title", "Lunex Programming")
  val author = xml.tag("author", "David Dev")

  val book = xml.set(xml.tag("book"), "id", "1")
  xml.add(book, title, author)

  val catalog = xml.set(xml.tag("catalog"), "version", "1.0")
  xml.add(catalog, book)

  io.log(xml.toString(catalog))
}
```

Output:
```xml
<?xml version="1.0" encoding="UTF-8"?>
<catalog version="1.0">
  <book id="1">
    <title>Lunex Programming</title>
    <author>David Dev</author>
  </book>
</catalog>
```

### Parse XML and read values

```lx
fn main() {
  val src = "<users><user id=\"42\"><name>Alice</name></user></users>"

  val doc  = xml.fromString(src)        
  val user = xml.find(doc, "user")      
  val name = xml.find(doc, "name")

  io.log(xml.get(name))                 
  io.log(xml.attr(user, "id"))          
}
```

### Set multiple attributes at once

```lx
val img = xml.setAll(xml.tag("img"), { src: "photo.png", alt: "photo" })

```

### Find all matching elements

```lx
val items = xml.findAll(doc, "item")
each item in items {
  io.log(xml.get(item))
}
```

### Beginner API reference

| Function | Description |
|---|---|
| `xml.tag(name)` | Create an empty element |
| `xml.tag(name, text)` | Create an element with text content |
| `xml.set(node, key, value)` | Set one attribute; returns the node |
| `xml.setAll(node, attrs)` | Set multiple attributes from an object; returns the node |
| `xml.add(parent, a, b, ...)` | Append up to 5 children; returns parent |
| `xml.get(node)` | Get the text content of a node |
| `xml.attr(node, name)` | Get an attribute value |
| `xml.toString(root)` | Serialize to a pretty XML string |
| `xml.fromString(src)` | Parse an XML string, return root node |
| `xml.find(root, tag)` | Find the first element matching tag |
| `xml.findAll(root, tag)` | Find all elements matching tag |

---

## Full API

### Node Structure

Every XML element is an `XmlNode` struct:

| Field | Type | Description |
|---|---|---|
| `tag` | string | Element tag name |
| `attrs` | object | Key-value attribute map |
| `attrOrder` | array | Attribute names in insertion order |
| `children` | array | Child `XmlNode` structs |
| `text` | string | Text content (leaf nodes only) |
| `cdata` | bool | If `true`, text serializes as `<![CDATA[ ]]>` |

Methods on any node:

| Method | Description |
|---|---|
| `setAttribute(k, v)` | Set attribute |
| `getAttribute(k)` | Get attribute, or `null` |
| `hasAttr(k)` | `true` if attribute exists |
| `attributeNames()` | Attribute names in insertion order |
| `appendChild(child)` | Add a child node |
| `hasChildren()` | `true` if node has children |
| `childAt(i)` | Child at index `i` |
| `childCount()` | Number of children |
| `setText(t)` | Set text content |
| `getText()` | Get text content |
| `setCData(flag)` | Mark/unmark text as CDATA |
| `isCData()` | `true` if text serializes as CDATA |
| `getTag()` | Get tag name |
| `toString()` | Returns `"XmlNode<tagname>"` for debugging |

```lx
val n = xml.createElement("div")
n.setAttribute("class", "box")
io.log(n.getAttribute("class"))  
io.log(n.hasAttr("id"))          
io.log(n.childCount())           
```

---

### Building XML

#### `xml.createElement(tag)` → node
Creates an empty element.

```lx
val section = xml.createElement("section")
```

#### `xml.createTextElement(tag, text)` → node
Creates an element with text content.

```lx
val title = xml.createTextElement("title", "Hello World")

```

#### `xml.createElementWithAttrs(tag, attrs)` → node
Creates an element and sets multiple attributes. Keys are sorted
alphabetically for deterministic output. Use
`createElementWithAttrPairs` when exact order matters.

```lx
val img = xml.createElementWithAttrs("img", { src: "photo.png", alt: "photo" })

```

#### `xml.createElementWithAttrPairs(tag, pairs)` → node
Like `createElementWithAttrs`, but takes an ordered array of
`[key, value]` pairs to preserve exact attribute order.

```lx
val img = xml.createElementWithAttrPairs("img", [["src", "photo.png"], ["alt", "photo"]])

```

#### `xml.createCDataElement(tag, text)` → node
Creates an element whose text serializes as `<![CDATA[ ... ]]>` — handy
for embedding SQL, JSON, HTML, or other markup-like content.

```lx
val sql = xml.createCDataElement("query", "SELECT * FROM users WHERE age > 18")

```

#### `xml.appendChild(node, child)`
Appends a child node.

#### `xml.setAttribute(node, key, value)`
Sets an attribute.

#### `xml.getAttribute(node, key)` → string | null
Gets an attribute value, or `null` if missing.

#### `xml.build(root)` → string
Serializes to a pretty-printed XML string with `<?xml ...?>` header.

#### `xml.buildFragment(node)` → string
Serializes without the XML header.

#### `xml.buildCompact(root)` / `xml.buildFragmentCompact(node)` → string
Same as above but whitespace-free, for sending over the wire.

---

### Parsing

#### `xml.parse(src)` → node | null
Parses an XML string and returns the root `XmlNode`, or `null` if no
root element is found.

Handles attributes, text, self-closing tags, `<?xml?>` declarations,
comments, DOCTYPE, and `<![CDATA[ ]]>` sections.

```lx
val doc = xml.parse("<users><user id=\"1\"><name>Alice</name></user></users>")
io.log(doc.tag)          
io.log(doc.childCount()) 
```

---

### Querying

#### `xml.selectFirst(root, tag)` → node | null
Returns the first node anywhere in the tree matching `tag`.

#### `xml.selectAll(root, tag)` → array
Returns all nodes matching `tag`.

```lx
val items = xml.selectAll(doc, "item")
each item in items {
  io.log(xml.text(item))
}
```

#### `xml.selectByAttr(root, attrName, attrValue)` → array
Returns all nodes where `attrs[attrName] == attrValue`.

```lx
val admins = xml.selectByAttr(doc, "role", "admin")
```

#### `xml.selectPath(root, path)` → node | null
Navigates a dot-separated path from the root tag.

```lx
val price = xml.selectPath(doc, "catalog.book.price")
```

#### `xml.children(node)` → array
Direct children only (not recursive).

#### `xml.childrenByTag(node, tag)` → array
Direct children matching a specific tag.

#### `xml.text(node)` → string
Text content of a node.

#### `xml.attr(node, name)` → string | null
Shorthand for `getAttribute`.

---

### Transforming

#### `xml.clone(node)` → node
Deep-clones a node and all its descendants.

#### `xml.walk(root, fn)`
Visits every node depth-first, calling `fn(node)` on each.

```lx
xml.walk(doc, fn(n) {
  io.log(n.tag)
})
```

#### `xml.mapNodes(root, fn)` → node | null
Transforms every node through `fn(node)`. Return the node to keep it,
`null` to remove it.

```lx
val clean = xml.mapNodes(doc, fn(n) {
  if n.tag == "draft" { null } else { n }
})
```

#### `xml.filterNodes(root, predicate)` → node
Removes children for which `predicate(node)` returns `false`. Mutates
in place.

#### `xml.toObject(root)` → object
Converts an XML tree to a plain Lunex object. Text nodes become
strings; repeated sibling tags become arrays.

```lx
val obj = xml.toObject(doc)
```

#### `xml.fromObject(tag, obj)` → node
Converts a plain Lunex object to an XML tree.

```lx
val node = xml.fromObject("config", {
  host: "localhost"
  port: "3000"
})

```

---

### Validating

All validation functions return `{ ok: bool, error: string }`.

#### `xml.isWellFormed(node)` → result
Checks that the node and all descendants have non-empty tag names.

```lx
val res = xml.isWellFormed(doc)
unless res.ok {
  io.err(res.error)
}
```

#### `xml.validateSchema(node, schema)` → result
Validates a single node against a schema.

| Field | Type | Description |
|---|---|---|
| `tag` | string | Must match this tag name |
| `requiredAttrs` | array | These attribute names must be present |
| `requiredChildren` | array | These child tags must exist |
| `minChildren` | number | Minimum child count |
| `maxChildren` | number | Maximum child count |
| `requireText` | bool | Must have non-empty text |
| `noText` | bool | Must NOT have text |

```lx
val res = xml.validateSchema(node, {
  tag:              "book"
  requiredAttrs:    ["id"]
  requiredChildren: ["title", "author"]
})

if res.ok {
  io.log("valid!")
} else {
  io.err(res.error)
}
```

#### `xml.validateTree(root, schemas)` → result
Validates every node in the tree using a map of `tag → schema`. Tags
not in the map are skipped.

```lx
val schemas = {
  catalog: { requiredChildren: ["book"] }
  book:    { requiredAttrs: ["id"], requiredChildren: ["title"] }
  title:   { requireText: true }
}

val res = xml.validateTree(doc, schemas)
if res.ok {
  io.log("document is valid")
} else {
  io.err(res.error)
}
```

---

## Robustness Notes

- **Entity encoding is global and correct.** Every `&`, `<`, `>`,
  `"`, tab, newline, and CR is escaped, so values round-trip intact.
- **Numeric character references** (`&#169;`, `&#x2764;`) are preserved
  verbatim rather than resolved, and survive round-trips unchanged.
- **Comments, processing instructions, and DOCTYPE** are parsed
  correctly even when they contain `>` internally.
- **CDATA sections** are parsed as raw text and re-serialized back.
- **Attribute and element order is deterministic.** `XmlNode` tracks
  attribute insertion order in `attrOrder`, and `createElementWithAttrs`
  / `fromObject` sort keys alphabetically, so `build()` output is
  reproducible across runs.

---

## Running Tests

```bash
lunex run test/run_all.lx
```

7 suites: **node**, **builder**, **parser**, **query**, **transform**,
**validate**, **entities**.

---

## Project Structure

```
lune-xml/
├── main.lx            Public entry point (beginner API + full API)
├── main.nax           Compiled bundle — use this for @fimport
├── lunex.toml         Project manifest
├── README.md          This file
├── src/
│   ├── node.lx        XmlNode struct
│   ├── entities.lx    XML entity encode/decode
│   ├── builder.lx     XML serializer
│   ├── parser.lx      XML parser
│   ├── query.lx       Selectors
│   ├── transform.lx   clone, walk, map, filter, toObject, fromObject
│   └── validate.lx    isWellFormed, validateSchema, validateTree
└── test/
    ├── run_all.lx
    ├── test_node.lx
    ├── test_builder.lx
    ├── test_parser.lx
    ├── test_query.lx
    ├── test_transform.lx
    ├── test_validate.lx
    └── test_entities.lx
```

---

## License

Apache-2.0 — by David Dev · [github.com/Megamexlevi2](https://github.com/Megamexlevi2)
