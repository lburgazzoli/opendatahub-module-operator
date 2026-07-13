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
- Keep option names explicit: `WithViper`, `WithFS`, `WithConfigPath`.
- Avoid sentinel `nil` values in public APIs when the same behavior can be
  expressed by omitting an option entirely.
- Prefer field-by-field copying in `Options.applyOption(...)` so presets remain
  easy to reason about as the struct grows.

## Config Loading

For config-loading APIs in this module:
- keep environment-based defaults implicit in the main loader
- use options only for explicit overrides or injected dependencies
- prefer one `Load(opts ...Option)` entry point over `LoadXxx` overloads
