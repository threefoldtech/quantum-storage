#!/bin/bash

# QuantumD Test Framework
# Tests data integrity for a local quantumd instance

set -euo pipefail

# Configuration
CONTAINER_NAME="quantumd-test"
IMAGE_NAME="quantumd"
MOUNT_POINT="/mnt/qsfs"
LOG_DIR="/tmp/quantumd-test-logs"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Test results
TESTS_PASSED=0
TESTS_FAILED=0

log() {
    echo -e "${GREEN}[$(date '+%Y-%m-%d %H:%M:%S')] $1${NC}"
}

warn() {
    echo -e "${YELLOW}[$(date '+%Y-%m-%d %H:%M:%S')] WARNING: $1${NC}"
}

error() {
    echo -e "${RED}[$(date '+%Y-%m-%d %H:%M:%S')] ERROR: $1${NC}"
}

# Cleanup function
cleanup() {
    log "Cleaning up test environment"
    docker rm -f "${CONTAINER_NAME}" 2>/dev/null || true
    rm -rf "${LOG_DIR}"
}

# Setup test environment
setup() {
    log "Setting up test environment"
    mkdir -p "${LOG_DIR}"

    # Build the image if it doesn't exist
    if ! docker image inspect "${IMAGE_NAME}" >/dev/null 2>&1; then
        log "Building quantumd image"
        cd "$(dirname "$0")/.."
        docker buildx build -t "${IMAGE_NAME}" .
        cd - >/dev/null
    fi
}

# Generate test data with checksums inside container
generate_test_data() {
    local size_mb=$1
    local file_count=${2:-1}
    local prefix="${3:-testfile}"

    log "Generating ${file_count} test files of ${size_mb}MB each"

    # Prepare checksum directory in container
    docker exec "${CONTAINER_NAME}" mkdir -p /tmp/checksums

    for i in $(seq 1 "$file_count"); do
        local temp_file="/tmp/${prefix}_${i}.dat"
        local filename="${MOUNT_POINT}/${prefix}_${i}.dat"
        local checksum_file="/tmp/checksums/${prefix}_${i}.sha256"

        # Generate file and compute checksum outside mount point
        docker exec "${CONTAINER_NAME}" bash -c "dd if=/dev/urandom of=\"$temp_file\" bs=1M count=$size_mb 2>/dev/null && sha256sum \"$temp_file\" | cut -d' ' -f1 > \"$checksum_file\""

        # Copy file to mount point
        docker exec "${CONTAINER_NAME}" mv "$temp_file" "$filename"
    done
}

# Start quantumd container
start_container() {
    log "Starting quantumd container"
    docker run -d \
        --privileged \
        --name "${CONTAINER_NAME}" \
        "${IMAGE_NAME}"

    log "Running quantumd setup"
    docker exec "${CONTAINER_NAME}" quantumd setup --local

    # Wait for system to be ready
    log "Waiting for quantumd system to initialize"
    local max_attempts=30
    local attempt=0

    while [ $attempt -lt $max_attempts ]; do
        if docker exec "${CONTAINER_NAME}" df | grep -q "${MOUNT_POINT}"; then
            log "Quantumd system is ready!"
            return 0
        fi
        sleep 1
        attempt=$((attempt + 1))
    done

    error "Quantumd system failed to initialize within ${max_attempts} attempts"
    docker logs "${CONTAINER_NAME}"
    return 1
}

# Simulate component failures
kill_zstor() {
    log "Stopping zstor service"
    docker exec "${CONTAINER_NAME}" systemctl stop zstor || true
    docker exec "${CONTAINER_NAME}" pkill -f zstor || true
}

kill_backend_zdb() {
    local port="$1"
    log "Stopping backend ZDB service on port ${port}"
    docker exec "${CONTAINER_NAME}" systemctl stop zdb-backend-${port} || true
    docker exec "${CONTAINER_NAME}" pkill -f "zdb.*${port}" || true
}

kill_zdbfs() {
    log "Stopping zdbfs service"
    docker exec "${CONTAINER_NAME}" systemctl stop zdbfs || true
    docker exec "${CONTAINER_NAME}" pkill -f zdbfs || true
}

kill_frontend_zdb() {
    log "Stopping frontend ZDB service"
    docker exec "${CONTAINER_NAME}" systemctl stop zdb-frontend || true
    docker exec "${CONTAINER_NAME}" pkill -f "zdb.*9900" || true
}

# Restore components
restore_zstor() {
    log "Restarting zstor service"
    docker exec "${CONTAINER_NAME}" systemctl start zstor || true
    sleep 2
}

restore_zdb_backend() {
    local port="$1"
    log "Restarting ZDB backend service on port ${port}"
    docker exec "${CONTAINER_NAME}" systemctl start zdb-backend-${port} || true
    sleep 2
}

restore_zdbfs() {
    log "Restarting zdbfs service"
    docker exec "${CONTAINER_NAME}" systemctl start zdbfs || true
    sleep 2
}

restore_frontend_zdb() {
    log "Restarting frontend ZDB service"
    docker exec "${CONTAINER_NAME}" systemctl start zdb-frontend || true
    sleep 2
}

# Verify data integrity inside container
verify_data_integrity() {
    local test_name="$1"
    log "Verifying data integrity for: ${test_name}"

    local integrity_ok=true

    # Get list of checksum files
    local checksum_files
    checksum_files=$(docker exec "${CONTAINER_NAME}" find /tmp/checksums -type f -name "*.sha256")

    # Process each checksum file
    while IFS= read -r cf; do
        # Extract base filename without extension
        local base_in_container
        base_in_container=$(basename "$cf" .sha256)
        local data_file="${MOUNT_POINT}/${base_in_container}.dat"

        # Get expected hash from stored checksum
        local expected_hash
        expected_hash=$(docker exec "${CONTAINER_NAME}" cat "$cf")

        # Compute actual hash of data file in container
        local actual_hash
        actual_hash=$(docker exec "${CONTAINER_NAME}" sha256sum "$data_file" | cut -d' ' -f1)

        if [ "$expected_hash" = "$actual_hash" ]; then
            log "✓ File ${base_in_container} integrity verified"
        else
            error "✗ File ${base_in_container} integrity check FAILED!"
            error "  Expected: $expected_hash"
            error "  Actual:   $actual_hash"
            integrity_ok=false
        fi
    done <<< "$checksum_files"

    if [ "$integrity_ok" = true ]; then
        log "✓ All files passed integrity check for test: ${test_name}"
        TESTS_PASSED=$((TESTS_PASSED + 1))
        return 0
    else
        error "✗ Data integrity test FAILED for: ${test_name}"
        TESTS_FAILED=$((TESTS_FAILED + 1))
        return 1
    fi
}

# Baseline test to demonstrate that everything is working
test_baseline() {
    log "=== TEST: Baseline test ==="

    start_container
    generate_test_data 10 1 "baseline_test"
    verify_data_integrity "baseline_test"
}

# Test scenario: Recovery from complete frontend loss
test_recover_frontend() {
    log "=== TEST: Recover from complete frontend loss ==="

    start_container

    generate_test_data 10 3 "recover_test"

    log "Initial storage layout"
    docker exec "${CONTAINER_NAME}" tree -h /data/

    # Give some time for data to be processed and stored in backends
    log "Waiting for data processing to complete"
    sleep 10

    log "Killing frontend zdb"
    kill_frontend_zdb

    log "Killing zdbfs"
    kill_zdbfs

    log "Deleting frontend data to simulate complete loss"
    # Wait a bit for files to become free
    docker exec "${CONTAINER_NAME}" rm -rf /data/data/* || true
    docker exec "${CONTAINER_NAME}" rm -rf /data/index/* || true

    log "Restoring frontend zdb"
    restore_frontend_zdb

    log "Unmounting mount point"
    docker exec "${CONTAINER_NAME}" umount "${MOUNT_POINT}" || true

    log "Restoring zdbfs"
    restore_zdbfs

    log "Verifying data integrity after recovery"
    verify_data_integrity "recover_frontend"

    log "Final storage layout"
    docker exec "${CONTAINER_NAME}" tree -h /data/
}

# Test scenario: Kill zstor during data upload
test_zstor_failure_during_upload() {
    log "=== TEST: zstor failure during data upload ==="

    start_container
    generate_test_data 10 3 "zstor_test"

    # Start data generation in background
    (
        sleep 2
        generate_test_data 10 3 "zstor_test_bg"
    ) &
    local copy_pid=$!

    # Kill zstor after 1 second
    sleep 1
    kill_zstor

    # Wait a bit then restore
    sleep 3
    restore_zstor

    # Wait for copy to complete
    wait $copy_pid || true

    sleep 5
    verify_data_integrity "zstor_failure_during_upload"
}

# Test scenario: Kill backend ZDB during operation
test_backend_zdb_failure() {
    log "=== TEST: Backend ZDB failure ==="

    start_container
    generate_test_data 5 2 "backend_test"

    # Kill one backend ZDB
    kill_backend_zdb 9901

    # Try to access data
    sleep 2

    # Restore the backend
    restore_zdb_backend 9901

    sleep 3
    verify_data_integrity "backend_zdb_failure"
}

# Test scenario: Kill zdbfs during operation
test_zdbfs_failure() {
    log "=== TEST: zdbfs failure ==="

    start_container
    generate_test_data 8 2 "zdbfs_test"

    # Kill zdbfs
    kill_zdbfs

    sleep 2

    # Restore zdbfs
    restore_zdbfs

    sleep 3
    verify_data_integrity "zdbfs_failure"
}

# Test scenario: Multiple component failures
test_multiple_failures() {
    log "=== TEST: Multiple component failures ==="

    start_container
    generate_test_data 15 4 "multi_test"

    # Sequential failures
    kill_zstor
    sleep 1
    kill_backend_zdb 9902
    sleep 1
    kill_zdbfs

    sleep 3

    # Restore in order
    restore_zdb_backend 9902
    sleep 2
    restore_zstor
    sleep 2
    restore_zdbfs

    sleep 5
    verify_data_integrity "multiple_failures"
}

# Run all tests
run_all_tests() {
    log "Starting Quantumd Tests"

    log "Building Docker image"
    make docker-build

    cleanup
    setup

    test_baseline
    cleanup

    test_recover_frontend
    cleanup

    test_zstor_failure_during_upload
    cleanup

    test_backend_zdb_failure
    cleanup

    test_zdbfs_failure
    cleanup

    test_multiple_failures
    cleanup

    log "=== TEST SUMMARY ==="
    log "Tests passed: ${TESTS_PASSED}"
    log "Tests failed: ${TESTS_FAILED}"

    if [ "$TESTS_FAILED" -eq 0 ]; then
        log "🎉 All tests passed!"
        exit 0
    else
        error "❌ Some tests failed!"
        exit 1
    fi
}

# Handle cleanup on script exit
trap cleanup EXIT

# Main execution
case "${1:-all}" in
    "baseline")
        setup
        test_baseline
        cleanup
        ;;
    "recover_frontend")
        setup
        test_recover_frontend
        cleanup
        ;;
    "zstor")
        setup
        test_zstor_failure_during_upload
        cleanup
        ;;
    "backend")
        setup
        test_backend_zdb_failure
        cleanup
        ;;
    "zdbfs")
        setup
        test_zdbfs_failure
        cleanup
        ;;
    "multi")
        setup
        test_multiple_failures
        cleanup
        ;;
    "all"|*)
        run_all_tests
        ;;
esac
