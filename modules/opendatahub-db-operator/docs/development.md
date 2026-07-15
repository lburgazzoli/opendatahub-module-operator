# Development Notes

This module prefers options-based APIs over overload-style helper functions
when a constructor or loader has optional dependencies or optional behavior.

## Options Pattern

Prefer this shape when an API should support both named helper functions and
struct-literal presets:

```go
type Option interface {
	applyOption(o *Options)
}

type Options struct {
	Dependency SomeType
}

func (o Options) applyOption(target *Options) {
	if o.Dependency != nil {
		target.Dependency = o.Dependency
	}
}

type optionFunc func(*Options)

func (fn optionFunc) applyOption(target *Options) {
	if fn == nil {
		return
	}
	fn(target)
}

func WithDependency(dep SomeType) Option {
	return optionFunc(func(options *Options) {
		if options == nil || dep == nil {
			return
		}
		options.Dependency = dep
	})
}
```

Use this pattern when callers benefit from both:
- `NewThing(WithDependency(dep))`
- `NewThing(Options{Dependency: dep})`

## Design Guidance

- Keep one primary entry point instead of multiple overload-style exported
  helpers that only shuffle optional inputs.
- Put defaults in the constructor or loader before applying options.
- Validate after all options are applied.
- Prefer an explicit `Options.Validate()` / `options.Validate()` method for
  constructor preconditions so validation stays centralized instead of being
  split across multiple call sites.
- Keep option names explicit: `WithViper`, `WithFS`, `WithConfigPath`.
- Avoid sentinel `nil` values in public APIs when the same behavior can be
  expressed by omitting an option entirely.
- Prefer field-by-field copying in `Options.applyOption(...)` so presets remain
  easy to reason about as the struct grows.

## Functional Options Layout

For non-trivial option-based code, prefer a small, predictable file split:

- `<name>.go`
  - primary constructor / loader
  - core lifecycle methods
  - defaults assembled close to the constructor
- `<name>_options.go`
  - `Option` interface
  - `options` / `Options` struct
  - `applyOption(...)`
  - `Validate()`
  - `WithXxx(...)` helpers
- `<name>_support.go`
  - helper functions that do not belong in the constructor or exported option
    surface

This keeps the constructor readable, makes the options surface easy to scan,
and prevents helper code from being mixed into API-shaping code.

## Functional Options Construction

When optional behavior is eventually expressed as another library's own option
type, prefer appending those translated options directly into a slice on the
local options struct instead of duplicating the same state in multiple fields.

Prefer this pattern:

```go
type options struct {
	CreateOptions []UpstreamOption
	LogFn         func(format string, args ...any)
}

func (o options) Validate() error {
	if o.LogFn == nil {
		return fmt.Errorf("log function is nil")
	}

	return nil
}

func WithThing(value string) Option {
	return optionFunc(func(opts *options) {
		if opts == nil || value == "" {
			return
		}

		opts.CreateOptions = append(opts.CreateOptions, upstream.WithThing(value))
	})
}
```

Instead of keeping both:
- a local `Thing string` field, and
- a translated upstream option derived later from that same field

That avoids duplicated state and keeps the constructor focused on applying the
already-built option slices.

## General File Layout

For package-internal code in this module, prefer grouping files by API role
rather than by arbitrary size:

- exported entry point / lifecycle: `<name>.go`
- options surface: `<name>_options.go`
- shared helpers: `<name>_support.go`
- focused tests: `<name>_test.go`

For package families with a shared facade plus backend-specific implementations,
keep the shared facade in the parent package and backend-specific behavior in a
subpackage. For example:

- `test/support/cluster/`
  - shared `Instance` abstraction
  - shared cross-backend options
- `test/support/cluster/kind/`
  - `kind` constructor and lifecycle
  - `kind_options.go`
  - `kind_support.go`

## Config Loading

For config-loading APIs in this module:
- keep environment-based defaults implicit in the main loader
- use options only for explicit overrides or injected dependencies
- prefer one `Load(opts ...Option)` entry point over `LoadXxx` overloads
