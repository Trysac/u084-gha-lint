# Testing

## Running

```sh
go test -cover .          # tests with coverage summary
go run ./scripts/check    # every gate, as CI and the factory run it
```

## Policy

- Statement coverage of the library (every package except `scripts/`) must stay at or above 80 %; the gate fails below it.
- Tests live in package `ghalint_test` and exercise the public API. They assert return values, returned errors (`errors.Is`) and their messages.
- Table-driven tests use `t.Run` subtests. Usage is documented with runnable `Example` functions, which `go test` verifies through their `// Output:` comments.
- The race detector runs whenever cgo and a C compiler are available (always in Linux CI); the gate prints why when it has to run without it.
- Each bug fix starts with a failing test that reproduces it.
- Tests are deterministic: no network, no wall clock, no dependence on test order.

## Layout

`<file>_test.go` sits next to the file it tests. Fixtures and golden files go in `testdata/`.
