#!/bin/bash

# Quantum Storage Failure Testing Framework
# Tests data integrity under various component failure scenarios

set -euo pipefail

# Configuration
CONTAINER_NAME="qsfs-test"
IMAGE_NAME="qsfs"
MOUNT_POINT="/mnt"
LOG_DIR="/tmp/qsfs-test-logs"

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
        log "Building quantum storage image"
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

# Start quantum storage container
start_container() {
    log "Starting quantum storage container"
    docker run -d \
        --cap-add SYS_ADMIN \
        --device /dev/fuse \
        --name "${CONTAINER_NAME}" \
        "${IMAGE_NAME}"

    # Wait for system to be ready
    log "Waiting for quantum storage system to initialize"
    local max_attempts=30
    local attempt=0

    while [ $attempt -lt $max_attempts ]; do
        if docker exec "${CONTAINER_NAME}" df | grep -q zdbfs; then
            log "Quantum storage system is ready!"
            return 0
        fi
        sleep 1
        attempt=$((attempt + 1))
    done

    error "Quantum storage system failed to initialize within ${max_attempts} attempts"
    return 1
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

# Simulate component failures
kill_zstor() {
    log "Stopping zstor service"
    docker exec "${CONTAINER_NAME}" zinit kill zstor SIGKILL
    docker exec "${CONTAINER_NAME}" zinit stop zstor
}

kill_backend_zdb() {
    local port="$1"
    local service_name
    case "$port" in
        9901) service_name="zdb-back1" ;;
        9902) service_name="zdb-back2" ;;
        9903) service_name="zdb-back3" ;;
        9904) service_name="zdb-back4" ;;
        *) error "Unknown port $port"; return 1 ;;
    esac
    log "Stopping backend ZDB service: ${service_name}"
    docker exec "${CONTAINER_NAME}" zinit kill "$service_name" SIGKILL
    docker exec "${CONTAINER_NAME}" zinit stop "$service_name"
}

kill_zdbfs() {
    log "Stopping zdbfs service"
    docker exec "${CONTAINER_NAME}" zinit kill zdbfs SIGKILL
    docker exec "${CONTAINER_NAME}" zinit stop zdbfs
}

kill_frontend_zdb() {
    log "Stopping frontend ZDB service"
    docker exec "${CONTAINER_NAME}" zinit kill zdb-front SIGKILL
    docker exec "${CONTAINER_NAME}" zinit stop zdb-front
}

# Restore components
restore_zstor() {
    log "Restarting zstor service"
    docker exec "${CONTAINER_NAME}" zinit start zstor
    sleep 2
}

restore_zdb_backend() {
    local port="$1"
    local service_name
    case "$port" in
        9901) service_name="zdb-back1" ;;
        9902) service_name="zdb-back2" ;;
        9903) service_name="zdb-back3" ;;
        9904) service_name="zdb-back4" ;;
        *) error "Unknown port $port"; return 1 ;;
    esac
    log "Restarting ZDB backend service: ${service_name}"
    docker exec "${CONTAINER_NAME}" zinit start "$service_name"
    sleep 2
}

restore_zdbfs() {
    log "Restarting zdbfs service"
    docker exec "${CONTAINER_NAME}" zinit start zdbfs
    sleep 2
}

restore_frontend_zdb() {
    log "Restarting frontend ZDB service"
    docker exec "${CONTAINER_NAME}" zinit start zdb-front
    sleep 2
}

# Baseline test to demonstrate that everything is working

test_baseline() {
    # This is a baseline test with the following steps:
    # - Data is generated and stored in the frontend
    # - Data is removed from the frontend automatically by zstor
    # - Data is restored from the backend
    # - Restored data is verified by comparing checksums with original data

    log "=== TEST: Baseline test ==="

    start_container
    generate_test_data 100 3 "baseline_test"

    log "Initial frontend storage layout"
    docker exec "${CONTAINER_NAME}" tree -h /data/data

    # Give some time for the frontend zdb to rotate and for zstor to work
    log "Waiting for data files to be removed"
    while docker exec "${CONTAINER_NAME}" [ -e "/data/data/zdbfs-data/d0" ]; do
        sleep 1
    done
    sleep 2

    log "Post removal frontend storage layout"
    docker exec "${CONTAINER_NAME}" tree -h /data/data

    verify_data_integrity "baseline_test"

    log "Final frontend storage layout"
    docker exec "${CONTAINER_NAME}" tree -h /data/data
}

test_recover_frontend() {
    # This test ensures that we can recover from a complete loss of the frontend
    # system. We simulate that by deleting the entire frontend zdb data
    # directory and then rebuild it manually.

    log "=== TEST: Baseline test ==="

    start_container

    generate_test_data 100 3 "baseline_test"

    log "Initial frontend storage layout"
    docker exec "${CONTAINER_NAME}" tree -h /data/

    # Give some time for the frontend zdb to rotate and for zstor to work
    # This also should give zstor enough time to store all data in backends,
    # though technically we haven't verified that is complete
    # log "Waiting for data files to be removed"
    while docker exec "${CONTAINER_NAME}" [ -e "/data/data/zdbfs-data/d0" ]; do
        sleep 1
    done
    # sleep 10

    log "Post removal frontend storage layout"
    docker exec "${CONTAINER_NAME}" tree -h /data/

    log "Killing frontend zdb"
    kill_frontend_zdb

    log "Killing zdbfs"
    kill_zdbfs

    log "Deleting frontend data"
    # Wait a bit for files to become free
    docker exec "${CONTAINER_NAME}" rm -rf /data/data
    docker exec "${CONTAINER_NAME}" mkdir /data/data
    docker exec "${CONTAINER_NAME}" rm -rf /data/index
    docker exec "${CONTAINER_NAME}" mkdir /data/index

    log "Restoring frontend zdb"
    restore_frontend_zdb

    log "Copying recover script into container"
    docker cp ../../pulumi/scripts/recover.sh "${CONTAINER_NAME}:/recover.sh"

    log "Running recovery script"
    docker exec "${CONTAINER_NAME}" bash -c "source /recover.sh && setup_temp_namespace && recover_metadata && recover_data"

    log "Unmounting /mnt"
    docker exec "${CONTAINER_NAME}" umount /mnt

    log "Restoring zdbfs"
    restore_zdbfs

    log "Verifying data integrity"
    verify_data_integrity "baseline_test"

    log "Final frontend storage layout"
    docker exec "${CONTAINER_NAME}" tree -h /data/data
}

# Test scenario: Kill zstor during data upload
test_zstor_failure_during_upload() {
    log "=== TEST: zstor failure during data upload ==="

    start_container
    generate_test_data 100 3 "zstor_test"

    # Start data generation in background
    (
        sleep 2
        generate_test_data 100 3 "zstor_test_bg"
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

    cleanup
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

    cleanup
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

    cleanup
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

    cleanup
}

# Run all tests
run_all_tests() {
    log "Starting Quantum Storage Failure Tests"

    setup

    test_zstor_failure_during_upload
    test_backend_zdb_failure
    test_zdbfs_failure
    test_multiple_failures

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
        ;;
    "recover_frontend")
        setup
        test_recover_frontend
        ;;
    "zstor")
        setup
        test_zstor_failure_during_upload
        ;;
    "backend")
        setup
        test_backend_zdb_failure
        ;;
    "zdbfs")
        setup
        test_zdbfs_failure
        ;;
    "multi")
        setup
        test_multiple_failures
        ;;
    "all"|*)
        run_all_tests
        ;;
esac
