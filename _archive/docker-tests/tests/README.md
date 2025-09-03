# Quantum Storage automated local tests

This area contains some automated tests that utilize the QSFS Docker image. The idea is to verify the combined function of all components without the complication of a multiple machine deployment. That is to say, these are `integration tests`.

The basic form of a test is:

1. Setup a QSFS container and seed the system with some random data
2. Present some challenge to the system, such as removing data or killing components
3. Verify that the original data is available and uncorrupted, by way or checksum comparisons

In addition to testing the normal unattended operation of the system, we can also test scenarios where manual intervention would be required.

## Running tests

Just run the script and pass the name of a test:

```
cd docker/tests
tests.sh baseline
```

Or run all the tests serially:

```
tests.sh all
```
