---
trigger: always_on
---

# Go Project Rules & Code Style

## General Archetype
- Follow the official Effective Go guidelines and Go Code Review Comments.
- Prioritize clear, plain, and readable code over highly clever optimizations.
- Keep the public API surface area as small as possible.

## Architecture & Layout
- Organize the project according to the Standard Go Project Layout.
- Place all business logic in internal directories to prevent external import.
- Keep `main.go` slim; use it only to wire dependencies and parse flags.

## Coding Style Rules
- **Indentation**: Always use native tabs for formatting, never spaces.
- **Naming**: Use `camelCase` for private names and `PascalCase` for public names.
- **Acronyms**: Keep acronyms consistently capitalized (e.g., use `userID`, not `userId`).
- **Short Variables**: Use short names for limited scopes (e.g., `r` for reader, `ctx` for context).

## Concurrency & Packages
- **Context**: Always make `context.Context` the very first parameter of a function.
- **Logging**: Use the standard `log/slog` package for structured telemetry and logging.
- **Goroutines**: Never start a goroutine without knowing exactly how it will exit.

## Error Handling
- **Return Style**: Return errors as the final return value from functions.
- **Wrapping**: Wrap upstream errors using `fmt.Errorf("context: %w", err)`.
- **Handling**: Never ignore errors using blank identifiers (`_`); handle them immediately.
