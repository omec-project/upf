# Test framework for BESS-UPF

Currently, the implementation of BESS-UPF is only tested in automated
system integration tests. The direct goal of this project is to create a
simple, developer-friendly and fully-automated test infrastructure that
assesses BESS-UPF features at a component level.

## Run a test
```console
./run_tests -t [test-dir] [optional: filename/filename.class]
```

Example: `./run_tests -t unary test.BTest`
