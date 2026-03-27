# json5

A Go package for encoding and decoding [JSON5](https://spec.json5.org/) data. The API mirrors the standard library `encoding/json` package, making it a drop-in replacement for projects that need JSON5 support.

## What is JSON5?

JSON5 is a superset of JSON that adds several features from ECMAScript 5.1:

- **Comments** — single-line (`//`) and multi-line (`/* */`)
- **Trailing commas** — in objects and arrays
- **Unquoted object keys** — any valid ECMAScript identifier
- **Single-quoted strings** — `'like this'`
- **Multiline strings** — using backslash line continuation
- **Extended escape sequences** — `\x41`, `\v`, `\0`
- **Hexadecimal numbers** — `0xFF`
- **Infinity and NaN** — as number literals
- **Leading/trailing decimal points** — `.5` and `5.`
- **Positive sign on numbers** — `+42`
- **Extended whitespace** — vertical tab, form feed, BOM, non-breaking space, and Unicode Zs category

Every valid JSON document is also valid JSON5.

## Installation

```
go get github.com/alchemy/json5
```

## Usage

### Unmarshal

Parse JSON5 data into Go values:

```go
package main

import (
    "fmt"
    "github.com/alchemy/json5"
)

func main() {
    data := []byte(`{
        // Database configuration
        host: 'localhost',
        port: 5432,
        ssl: true,
        maxConns: 0xFF,
        timeout: .5,  // seconds
        tags: ['primary', 'fast',],
    }`)

    var config map[string]any
    if err := json5.Unmarshal(data, &config); err != nil {
        panic(err)
    }
    fmt.Println(config)
}
```

### Unmarshal into structs

Struct tags use `json5` (with fallback to `json`):

```go
type Config struct {
    Host     string   `json5:"host"`
    Port     int      `json5:"port"`
    SSL      bool     `json5:"ssl"`
    MaxConns int      `json5:"maxConns"`
    Timeout  float64  `json5:"timeout"`
    Tags     []string `json5:"tags"`
    Debug    bool     `json5:"debug,omitempty"`
}

var config Config
err := json5.Unmarshal(data, &config)
```

### Marshal

Encode Go values to JSON5:

```go
b, err := json5.Marshal(config)
// {"host":"localhost","port":5432,"ssl":true,"maxConns":255,"timeout":0.5,"tags":["primary","fast"]}
```

Output is valid JSON for standard types. For `float64` values of `Infinity` and `NaN`, the encoder outputs JSON5-specific literals:

```go
b, _ := json5.Marshal(math.Inf(1))   // Infinity
b, _ = json5.Marshal(math.NaN())     // NaN
```

### MarshalIndent

Pretty-print with indentation:

```go
b, err := json5.MarshalIndent(config, "", "  ")
```

### Streaming with Decoder/Encoder

```go
// Decode from a reader
dec := json5.NewDecoder(file)
dec.UseNumber()                // decode numbers as json5.Number
dec.DisallowUnknownFields()    // error on unknown struct fields

var config Config
err := dec.Decode(&config)

// Encode to a writer
enc := json5.NewEncoder(os.Stdout)
err = enc.Encode(config)
```

### Validation

```go
if json5.Valid(data) {
    fmt.Println("valid JSON5")
}
```

### RawMessage

Delay parsing of a JSON5 value:

```go
type Event struct {
    Type    string          `json5:"type"`
    Payload json5.RawMessage `json5:"payload"`
}

var event Event
json5.Unmarshal(data, &event)

// Parse payload later based on type
switch event.Type {
case "click":
    var click ClickPayload
    json5.Unmarshal(event.Payload, &click)
}
```

### Number type

Preserve exact number representation. The `Float64()` and `Int64()` methods handle all JSON5 number formats including hexadecimal, `Infinity`, and `NaN`:

```go
dec := json5.NewDecoder(reader)
dec.UseNumber()

var v any
dec.Decode(&v)

n := v.(json5.Number)
f, _ := n.Float64()    // as float64 (handles 0xFF, Infinity, NaN)
i, _ := n.Int64()      // as int64 (handles 0xFF)
s := n.String()        // original text
```

## Struct Tags

The package recognizes `json5` struct tags with the same syntax as `encoding/json`:

```go
type Example struct {
    Field1 string `json5:"name"`              // custom name
    Field2 int    `json5:"count,omitempty"`    // omit if zero value
    Field3 bool   `json5:",omitempty"`         // default name, omit if zero
    Field4 int    `json5:"val,string"`         // encode/decode as string
    Field5 string `json5:"-"`                  // skip this field
}
```

If no `json5` tag is present, the package falls back to the `json` tag, enabling compatibility with existing types.

## Interface Compatibility

The package checks for these interfaces in order:

1. `json5.Marshaler` / `json5.Unmarshaler` — JSON5-specific
2. `json.Marshaler` / `json.Unmarshaler` — standard library compatibility
3. `encoding.TextMarshaler` / `encoding.TextUnmarshaler`

This means types that already implement `json.Marshaler` or `json.Unmarshaler` work without modification.

## License

MIT
