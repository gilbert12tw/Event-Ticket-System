# Clean Code and Google Style Rules

Use the public Google style guides as the default style reference, then apply stricter local project tooling when it exists.

Reference sources:

- Google Style Guides: https://google.github.io/styleguide/
- Google Go Style Guide: https://google.github.io/styleguide/go/guide
- Google Engineering Practices: Code Review: https://google.github.io/eng-practices/review/
- Google Engineering Practices: Small CLs: https://google.github.io/eng-practices/review/developer/small-cls.html
- Google Developer Documentation: Code Samples: https://developers.google.com/style/code-samples

## Style

- Go code must follow Google Go Style Guide principles: clarity, simplicity, concision, maintainability, and consistency.
- Every Go source file must be formatted with `gofmt`.
- TypeScript, JavaScript, HTML, and CSS must follow the relevant Google style guides where applicable.
- Project formatter and linter configuration overrides generic style preferences.

## File and Function Size

- Prefer 200-300 lines for hand-written source files.
- A hand-written source file must not exceed 500 lines.
- When a file approaches 400 lines, split it before adding more behavior unless the split would make the code less clear.
- Generated files, lockfiles, migrations, schema snapshots, vendored code, and large static data fixtures are exempt.
- Do not bypass size limits by moving unrelated behavior into a generic dumping ground.

Keep functions and methods small enough that their intent, inputs, outputs, and failure behavior are clear at a glance. Each function should do one thing at one abstraction level.

Extract helpers when they improve names, reduce nesting, isolate business rules, or make tests more focused. Do not extract helpers only to hide complexity or satisfy a line-count target.

## Coupling and Cohesion

- Use explicit interfaces and dependency injection at adapter boundaries.
- Avoid hidden global state, package-level mutable state, cyclic dependencies, and broad shared utility packages.
- Prefer domain-specific helpers near the code that uses them before introducing shared abstractions.
- Keep data flow explicit so callers can see dependencies, idempotency keys, transactions, and authorization decisions.

Review code for design, functionality, complexity, tests, naming, comments, style, and documentation.
