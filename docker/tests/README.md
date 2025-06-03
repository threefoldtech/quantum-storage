# Quantum Storage Automated Testing Framework

This directory contains automated tests for validating data integrity under various failure scenarios in the Quantum Storage system.

## Overview

The testing framework simulates real-world failures of system components and verifies that data remains intact and uncorrupted. Each test follows this pattern:

1. **Generate random test data** with SHA256 checksums
2. **Copy data** into the quantum storage system mounted at `/mnt`
3. **Simulate component failures** (kill processes)
4. **Restore the system** (restart processes)
5. **Verify data integrity** by comparing checksums

## Test Scripts

### `test-framework.sh` - Core Testing Framework

The main testing framework with essential failure scenarios:

```bash
# Run all tests
./test-framework.sh

# Run specific test
./test-framework.sh zstor     # Test zstor failures
./test-framework.sh backend   # Test backend ZDB failures  
./test-framework.sh zdbfs     # Test zdbfs failures
./test-framework.sh multi     # Test multiple component failures
```

#### Test Scenarios Included:

- **zstor failure during upload**: Kills zstor while data is being uploaded
- **Backend ZDB failure**: Kills one of the 4 backend ZDB instances
- **zdbfs failure**: Kills the FUSE filesystem component
- **Multiple failures**: Sequential failures of multiple components

### `stress-test.sh` - Stress Testing

Advanced stress testing scenarios:

```bash
# Run all stress tests
./stress-test.sh

# Run specific stress test
./stress-test.sh concurrent   # Concurrent operations with failures
./stress-test.sh rapid        # Rapid failure/recovery cycles
./stress-test.sh large        # Large file operations under failure
```

#### Stress Test Scenarios:

- **Concurrent operations**: Multiple data operations running during failures
- **Rapid failure/recovery**: Quick succession of kill/restore cycles
- **Large files under failure**: 100MB+ file transfers interrupted by failures

## System Architecture

The quantum storage system consists of these components:

- **4 Backend ZDB instances** (ports 9901-9904): Store data and metadata
- **zstor**: Handles data distribution, redundancy, and erasure coding
- **zdbfs**: FUSE filesystem interface mounted at `/mnt`
- **Frontend ZDB** (port 9900): Interfaces with zdbfs via hooks

```
┌─────────────┐    ┌─────────────┐    ┌─────────────┐
│    zdbfs    │───▶│Frontend ZDB │───▶│zdb-hook.sh  │
│  (FUSE FS)  │    │  (port 9900)│    │             │
└─────────────┘    └─────────────┘    └──────┬──────┘
                                             │
                   ┌─────────────────────────▼─────────────────────────┐
                   │                  zstor                           │
                   │        (data distribution & redundancy)          │
                   └┬────────┬────────┬────────┬─────────────────────┘
                    │        │        │        │
            ┌───────▼──┐ ┌───▼───┐ ┌───▼───┐ ┌──▼────┐
            │Backend  │ │Backend│ │Backend│ │Backend│
            │ZDB:9901 │ │ZDB:9902│ │ZDB:9903│ │ZDB:9904│
            └─────────┘ └───────┘ └───────┘ └───────┘
```

## Configuration

### Redundancy Settings
- **Minimal shards**: 2 (minimum shards needed to reconstruct data)
- **Expected shards**: 4 (total shards created per file)
- **Backends**: 4 ZDB instances for redundancy

### Test Data
- Test files are generated in `/tmp/qsfs-test-data`
- File sizes range from 5MB to 100MB depending on test
- SHA256 checksums verify data integrity

## Prerequisites

- Docker with `buildx` support
- Linux system with FUSE support
- Sufficient disk space for test data (up to 1GB for stress tests)

## Running Tests

1. **Build the image** (if not already built):
   ```bash
   cd docker/
   docker buildx build -t qsfs .
   ```

2. **Run basic tests**:
   ```bash
   cd tests/
   ./test-framework.sh
   ```

3. **Run stress tests**:
   ```bash
   ./stress-test.sh
   ```

## Test Results

The framework provides colored output:
- 🟢 **Green**: Normal operations and successful tests
- 🟡 **Yellow**: Warnings
- 🔴 **Red**: Errors and failed tests

Example output:
```
[2025-01-01 12:00:00] ✓ File testfile_1.dat integrity verified
[2025-01-01 12:00:01] ✓ All files passed integrity check for test: zstor_failure_during_upload
[2025-01-01 12:00:02] === TEST SUMMARY ===
[2025-01-01 12:00:02] Tests passed: 4
[2025-01-01 12:00:02] Tests failed: 0
[2025-01-01 12:00:02] 🎉 All tests passed!
```

## Troubleshooting

### Common Issues

1. **Container startup failures**: Check Docker daemon and FUSE support
2. **Permission errors**: Ensure script has execute permissions (`chmod +x`)
3. **Insufficient disk space**: Tests require several GB of temporary space
4. **Port conflicts**: Ensure ports 9900-9904 are available

### Debug Mode

Enable debug output by setting:
```bash
export DEBUG=1
./test-framework.sh
```

### Logs

Container logs are available via:
```bash
docker logs qsfs-test
```

Component-specific logs inside container:
- `/var/log/zstor.log` - zstor logs
- `/var/log/zdb.log` - Frontend ZDB logs  
- `/var/log/zdb1.log` to `/var/log/zdb4.log` - Backend ZDB logs
- `/var/log/zdbfs.log` - zdbfs logs

## Contributing

To add new test scenarios:

1. Add test function to `test-framework.sh` or `stress-test.sh`
2. Follow the pattern: setup → failure → restore → verify
3. Use the existing helper functions for consistency
4. Update this README with new test descriptions

Example test structure:
```bash
test_new_scenario() {
    log "=== TEST: New scenario description ==="
    
    generate_test_data 10 2 "new_test"
    start_container
    copy_test_data
    
    # Simulate failure
    kill_component
    sleep 2
    
    # Restore system  
    restore_component
    sleep 3
    
    verify_data_integrity "new_scenario"
    cleanup
}
```
